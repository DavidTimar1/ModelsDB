// The video and image listings, read beside the general
// one and joined onto the rows it already made.
//
// The general listing (ApiUrl) carries none of a video or an image model's
// own fields: no supported_frame_images, no pricing_skus, no structured
// reference-image count. What such a model takes is reachable only through
// its own listing. This file reads those two listings and JOINS each
// record onto the row FetchModels' own pass over the general listing already
// made, by id - never minting a row of its own, and never touching a path
// the general listing already carries (see joinByID and FetchModels).

package catalog

import (
	"encoding/json"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"strconv"
	"strings"
)

// VideoModelsURL is the video listing the refresh reads after the general one. A
// package-level var, like ApiUrl, so a test can point it at a local
// httptest.Server.
var VideoModelsURL = "https://openrouter.ai/api/v1/videos/models"

// ImageModelsURL is the image listing the refresh reads after the video one. A
// package-level var for the same reason as VideoModelsURL.
var ImageModelsURL = "https://openrouter.ai/api/v1/images/models"

// readDedicatedListings reads the video listing then the image listing, joins
// each onto idToName, writes their three question columns
// (frame_images, max_reference_images, reference_images), and answers every
// joined record's own map and raw bytes for FetchModels to fold into the
// same feed-field walk the general listing goes through. A listing that
// cannot be read (network, non-200, bad JSON) costs only itself: it is
// logged as one phase failure and the rest of the refresh, the other listing
// included, goes on; the failure is also returned, for the caller to report.
func readDedicatedListings(idToName map[string]string, pc *pacer) (data []map[string]interface{}, raw []json.RawMessage, failure error) {
	var failures phaseFailures

	video, videoRaw, videoTotal, err := joinByID(VideoModelsURL, idToName, pc)
	if err != nil {
		failures.record("the video listing: %v", err)
	} else {
		filled := fillVideoFacts(video)
		log.Printf("  ok %d video model(s) read, %d matched a row from the general listing, %d frame-image value(s) written",
			videoTotal, len(video), filled)
		data = append(data, video...)
		raw = append(raw, videoRaw...)
	}

	image, imageRaw, imageTotal, err := joinByID(ImageModelsURL, idToName, pc)
	if err != nil {
		failures.record("the image listing: %v", err)
	} else {
		filled := fillImageFacts(image)
		log.Printf("  ok %d image model(s) read, %d matched a row from the general listing, %d reference-image value(s) written",
			imageTotal, len(image), filled)
		data = append(data, image...)
		raw = append(raw, imageRaw...)
	}

	failure = failures.asError()
	if failure != nil {
		log.Printf("Warning: %v", failure)
	}
	return data, raw, failure
}

// joinByID reads one dedicated listing and joins each record onto the row
// the general listing made this refresh, by id: a record whose id matches no
// row from the general listing (or carries none) is skipped and counted,
// never creating a row of its own. The record handed back has its
// "name" field set to the JOINED name, so every reader downstream addresses
// the row the general listing made, whatever the dedicated listing itself
// happened to call it - the two are identical today, but the join does not
// depend on that staying true.
func joinByID(url string, idToName map[string]string, pc *pacer) (joined []map[string]interface{}, joinedRaw []json.RawMessage, total int, err error) {
	data, raw, err := fetchFeedFrom(url, pc)
	if err != nil {
		return nil, nil, 0, err
	}
	var skippedIDs []string
	for i, m := range data {
		id := shared.GetStr(m, "id")
		name, ok := idToName[id]
		if id == "" || !ok {
			skippedIDs = append(skippedIDs, id)
			continue
		}
		rec := make(map[string]interface{}, len(m))
		for k, v := range m {
			rec[k] = v
		}
		rec["name"] = name
		joined = append(joined, rec)
		joinedRaw = append(joinedRaw, raw[i])
	}
	if len(skippedIDs) > 0 {
		log.Printf("  %s: %d record(s) matched no row from the general listing and were skipped: %s",
			url, len(skippedIDs), strings.Join(skippedIDs, ", "))
	}
	return joined, joinedRaw, len(data), nil
}

// fillVideoFacts writes frame_images for every joined video record whose
// supported_frame_images is a non-null, non-empty list. A null or absent
// value writes nothing, so the cell stays empty rather than reading as
// "takes no frame images".
func fillVideoFacts(records []map[string]interface{}) (filled int) {
	for _, rec := range records {
		name := shared.GetStr(rec, "name")
		v, present := rec["supported_frame_images"]
		if !present || v == nil {
			continue
		}
		list, ok := v.([]interface{})
		if !ok || len(list) == 0 {
			continue
		}
		text := dbcore.ToJSONArr(list)
		if text == "" {
			continue
		}
		if err := store.WriteCells(store.SourceOpenRouter, name, map[string]string{"frame_images": text}); err != nil {
			log.Printf("Warning: frame_images for %q was not stored: %v", name, err)
			continue
		}
		filled++
	}
	return filled
}

// fillImageFacts writes max_reference_images and reference_images for every
// joined image record, from supported_parameters.input_references.max - the
// one structured reference-image ceiling the image listing states.
// max_reference_images is written whenever the field is present, at
// whatever figure it names; reference_images is written yes alongside it
// only where that figure is at least one. An absent field writes neither cell,
// leaving both for the manual pass to weigh rather than a fetched "no".
func fillImageFacts(records []map[string]interface{}) (filled int) {
	for _, rec := range records {
		name := shared.GetStr(rec, "name")
		ceiling, ok := inputReferencesMax(rec)
		if !ok {
			continue
		}
		values := map[string]string{"max_reference_images": strconv.FormatFloat(ceiling, 'f', -1, 64)}
		if ceiling >= 1 {
			values["reference_images"] = "yes"
		}
		if err := store.WriteCells(store.SourceOpenRouter, name, values); err != nil {
			log.Printf("Warning: the reference-image facts for %q were not stored: %v", name, err)
			continue
		}
		filled++
	}
	return filled
}

// inputReferencesMax reads supported_parameters.input_references.max off one
// image-listing record, and false where the record states no such field at
// all.
func inputReferencesMax(rec map[string]interface{}) (float64, bool) {
	sp, ok := rec["supported_parameters"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	ir, ok := sp["input_references"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	ceiling, ok := ir["max"].(float64)
	if !ok {
		return 0, false
	}
	return ceiling, true
}
