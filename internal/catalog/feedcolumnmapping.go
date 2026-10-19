package catalog

import "sort"

// feedPathColumns maps a feed leaf path - as walkFeedRecord names it (see
// feedfields.go) - to the shipped column key that already reads it, for every
// path NormalizeModel or OpenRouterFacts (update.go) already writes into a
// column. The refresh consults it (feedPathIsRead) so a path a shipped column
// reads gets no second, redundant column made for it, and GET /api/columns
// consults it the other way round (FeedPathsByColumn) so a shipped column can
// be found by the feed's own name.
//
// A path absent here is one no shipped column reads, and the refresh makes a
// column for it - so this map has to cover EVERY path the code already reads,
// or the refresh makes a duplicate.
var feedPathColumns = map[string]string{
	"id":                                 "id",
	"canonical_slug":                     "slug",
	"hugging_face_id":                    "repository",
	"description":                        "description",
	"context_length":                     "context_length",
	"created":                            "created_at",
	"architecture.input_modalities":      "input_modalities",
	"architecture.output_modalities":     "output_modalities",
	"architecture.tokenizer":             "tokenizer",
	"architecture.instruct_type":         "instruct_type",
	"supported_parameters":               "supported_parameters",
	"top_provider.is_moderated":          "moderated",
	"top_provider.max_completion_tokens": "max_completion_tokens",
	"knowledge_cutoff":                   "knowledge_cutoff",
	"expiration_date":                    "expiration_date",
	"reasoning.mandatory":                "reasoning_mandatory",
	"alias_target.slug":                  "alias_target",
	"pricing.prompt":                     "price_prompt",
	"pricing.completion":                 "price_completion",
	"pricing.image":                      "price_image",
	"pricing.audio":                      "price_audio",
	"pricing.internal_reasoning":         "price_internal_reasoning",
	"pricing.input_cache_read":           "price_cache_read",
	"pricing.input_cache_write":          "price_cache_write",
	"pricing.input_cache_write_1h":       "price_cache_write_1h",
	"pricing.input_audio_cache":          "price_audio_cache_read",
	"pricing.image_output":               "price_image_output",
	"pricing.audio_output":               "price_audio_output",
	"pricing.web_search":                 "price_web_search",

	// The video listing's frame-image answer and the image listing's own
	// structured reference-image ceiling (dedicatedlistings.go). Both are
	// filled by explicit code there, not by the door's generic fill, but they
	// still have to be named here so the door makes no second, redundant
	// column for either path.
	"supported_frame_images":                    "frame_images",
	"supported_parameters.input_references.max": "max_reference_images",
}

// feedPathsOnTheRow are the feed paths read into the models row itself rather
// than into a column: "name" is models.name, the row's own key. No column can
// be made for one.
var feedPathsOnTheRow = map[string]bool{"name": true}

// feedPathIsRead reports whether something already reads path - a shipped
// column, or the model row - so the refresh makes no column for it.
func feedPathIsRead(path string) bool {
	_, mapped := feedPathColumns[path]
	return mapped || feedPathsOnTheRow[path]
}

// FeedPathsByColumn is the same table the other way round: for every shipped
// column key, the feed paths the refresh reads into it, sorted. It exists so
// the column list can name a column by the feed's own word for it as well as
// by its heading - "hugging_face_id" reaches the Repository column, which is
// not a word anyone types when looking for it.
func FeedPathsByColumn() map[string][]string {
	out := map[string][]string{}
	for path, key := range feedPathColumns {
		out[key] = append(out[key], path)
	}
	for key := range out {
		sort.Strings(out[key])
	}
	return out
}
