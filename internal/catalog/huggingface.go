// Reading a model's VARIATIONS off HuggingFace.
//
// A build is one quantization of a model: a smaller copy in which each weight is
// stored with fewer bits, published as its own repository by whoever made it. A
// model listed on OpenRouter is a hosted service; a build is a file somebody can
// download and run. The app stores each build the user picks as a variation row
// (see store.ImportVariation), and this file is the only thing that reads
// HuggingFace to find one - all knowledge of HuggingFace's response shapes lives
// in this package, the way NormalizeModel holds all knowledge of OpenRouter's,
// with hfmodel.go reading the same repositories as MODELS rather than as builds.
//
// Three addresses are read here, and no others:
//
//	GET <api>/models?filter=base_model:quantized:<repo>   which repositories publish builds of <repo>
//	GET <api>/models/<repo>?blobs=true                    that repository's files, with exact byte sizes
//	GET <resolve>/<repo>/resolve/main/config.json         the architecture behind the build
//
// Everything below is measured against the live API rather than assumed, and the
// awkward parts are all real:
//
//   - The results come from MANY OWNERS, and not all of them are quantizations.
//     The list is offered to a person to choose from; nothing here decides which
//     entry is "the right one".
//   - The listing PAGINATES with a `link: <...>; rel="next"` header. This reads a
//     bounded number of pages and reports plainly when more existed.
//   - `?blobs=true` is what adds `size` to each file. Without it a file entry
//     carries only its name, and the size is the whole point.
//   - A GGUF repository holds ONE FILE PER QUANTIZATION, so one repository offers
//     many builds; every other kind holds ONE build spread over shards.
//   - `gguf.totalFileSize` is one file's size, not the repository's total, so
//     every size here is summed from the file list instead.
//   - A GATED repository answers the metadata call in full and REFUSES the file,
//     so the two failures are separate and both are surfaced as messages.
//
// Every request goes out through shared.NewHTTPClient, so it carries the one
// plain browser User-Agent and nothing that names this program or its owner.

package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/shared"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HFAPIBaseURL and HFResolveBaseURL are the two HuggingFace hosts this file
// reads. They are package-level vars, not consts, for the same reason ApiUrl is:
// a test points them at a local httptest.Server and exercises the whole path
// with no request to huggingface.co.
var (
	HFAPIBaseURL     = "https://huggingface.co/api"
	HFResolveBaseURL = "https://huggingface.co"
)

const (
	hfHTTPTimeout = 25 * time.Second
	// hfRepoPageSize is the page size asked for, and hfMaxRepoPages caps how many
	// pages are followed. A popular model has hundreds of quantized mirrors and
	// nobody scrolls three hundred entries; the cap is reported to the caller
	// rather than hidden, so the list never silently claims to be complete.
	hfRepoPageSize = 100
	hfMaxRepoPages = 3
	// hfMaxBodyBytes bounds every response read. The largest thing fetched here is
	// a config.json (kilobytes) or a file listing of a few hundred entries.
	hfMaxBodyBytes = 8 << 20
)

// HFRepo is one repository that publishes builds of a model, as the picker's
// first level shows it.
type HFRepo struct {
	Repo      string `json:"repo"`
	Likes     int    `json:"likes"`
	Downloads int    `json:"downloads"`
	Gated     bool   `json:"gated"`
	Format    string `json:"format"`  // what the repository's own tags say it publishes, "" when they say nothing
	Updated   string `json:"updated"` // when it last changed, as HuggingFace reports it
}

// HFBuild is one variation inside a repository, as the picker's second
// level shows it and as the importer stores it.
//
// ID is what a caller passes back to import this build, and it is the build's
// own file path (or the empty string for a repository that holds a single build
// spread over shards). The server looks the ID up in a freshly fetched listing
// rather than trusting any size or label a client sends back.
type HFBuild struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`  // the quantization label, e.g. "Q4_K_M", or the format when the repository holds one build
	Format    string   `json:"format"` // GGUF, AWQ, GPTQ, FP8, bnb-4bit, MLX, EXL2, safetensors
	Bit       int      `json:"bit"`    // bits per weight; 0 when the build does not say
	Bytes     int64    `json:"bytes"`
	Files     []string `json:"files"`
	Projector bool     `json:"projector"` // a vision projector, loaded ALONGSIDE a quantization rather than being one
}

// HFRepoDetail is one repository read in full: what it holds and what it took to
// read it.
type HFRepoDetail struct {
	Repo      string            `json:"repo"`
	Revision  string            `json:"revision"` // the commit main pointed at when it was read; main moves
	Gated     bool              `json:"gated"`
	Author    string            `json:"author"`
	BaseModel string            `json:"base_model"` // the repository these builds were made from, from the plain base_model tag
	GGUF      map[string]string `json:"gguf"`       // the small scalar facts off a GGUF repository's own gguf block
	Builds    []HFBuild         `json:"builds"`
}

// hfGet performs one GET and returns the body, the response header and the
// status. A non-2xx status is NOT an error here: which status came back is the
// answer for several of the callers (404 means "no config.json, try the base
// repository"; 401 means "gated, say so"), so the decision is left to them.
func hfGet(rawURL string) ([]byte, http.Header, int, error) {
	client := shared.NewHTTPClient(hfHTTPTimeout)
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, hfMaxBodyBytes))
	if err != nil {
		return nil, resp.Header, resp.StatusCode, err
	}
	return body, resp.Header, resp.StatusCode, nil
}

// nextPageURL reads the address of the next page out of a Link header
// (`<https://...>; rel="next"`). It returns "" when there is no next page, which
// is how the paging loop below knows it has the lot.
func nextPageURL(h http.Header) string {
	for _, link := range h.Values("Link") {
		for _, part := range strings.Split(link, ",") {
			part = strings.TrimSpace(part)
			start, end := strings.Index(part, "<"), strings.Index(part, ">")
			if start < 0 || end < start {
				continue
			}
			if !strings.Contains(part[end:], `rel="next"`) {
				continue
			}
			return part[start+1 : end]
		}
	}
	return ""
}

// sameOrigin reports whether two addresses share a scheme and a host, so a Link
// header cannot send the paging loop to another site.
func sameOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	return errA == nil && errB == nil && ua.Scheme == ub.Scheme && strings.EqualFold(ua.Host, ub.Host)
}

// isGated turns HuggingFace's gated field into a yes or no. It is a JSON boolean
// false on an open repository and a STRING on a gated one ("manual" and "auto"
// are both used), so a plain bool decode would fail on exactly the repositories
// this has to catch.
func isGated(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s != "" && s != "false"
	}
	return false
}

// hfRepoListing is one entry of the repository search response.
type hfRepoListing struct {
	ID           string      `json:"id"`
	Likes        int         `json:"likes"`
	Downloads    int         `json:"downloads"`
	Private      bool        `json:"private"`
	Gated        interface{} `json:"gated"`
	Tags         []string    `json:"tags"`
	LastModified string      `json:"lastModified"`
}

// HFQuantizedRepos lists the repositories that publish builds of baseRepo,
// newest-and-most-liked first as HuggingFace orders them. The second return
// value reports that MORE pages existed than were read, so the caller can say so
// instead of presenting a capped list as the whole truth.
//
// The results are what the tag says they are and no more: they come from many
// owners, and re-uploads, merges and experiments carry the same tag as a real
// quantization. Choosing among them is a judgement, which is why this returns a
// list for a person rather than picking one.
func HFQuantizedRepos(baseRepo string) ([]HFRepo, bool, error) {
	baseRepo, err := NormalizeHFRepo(baseRepo)
	if err != nil {
		return nil, false, err
	}
	next := fmt.Sprintf("%s/models?filter=%s&limit=%d",
		HFAPIBaseURL, url.QueryEscape("base_model:quantized:"+baseRepo), hfRepoPageSize)

	var out []HFRepo
	seen := map[string]bool{}
	truncated := false
	for page := 0; next != ""; page++ {
		if page >= hfMaxRepoPages {
			truncated = true
			break
		}
		body, header, status, err := hfGet(next)
		if err != nil {
			return nil, false, fmt.Errorf("list the builds of %s: %w", baseRepo, err)
		}
		if status != http.StatusOK {
			return nil, false, fmt.Errorf("list the builds of %s: HuggingFace answered %d", baseRepo, status)
		}
		var listing []hfRepoListing
		if err := json.Unmarshal(body, &listing); err != nil {
			return nil, false, fmt.Errorf("read the build list of %s: %w", baseRepo, err)
		}
		for _, r := range listing {
			if r.ID == "" || r.Private || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			format, _ := formatFromTags(r.Tags)
			out = append(out, HFRepo{
				Repo:      r.ID,
				Likes:     r.Likes,
				Downloads: r.Downloads,
				Gated:     isGated(r.Gated),
				Format:    format,
				Updated:   r.LastModified,
			})
		}
		next = nextPageURL(header)
		if next != "" && !sameOrigin(next, HFAPIBaseURL) {
			log.Printf("Ignoring a next-page address that is not on the HuggingFace API host")
			next = ""
		}
	}
	return out, truncated, nil
}

// hfSibling is one file entry of the repository-info response. size arrives only
// because the request asks for ?blobs=true.
type hfSibling struct {
	Path string `json:"rfilename"`
	Size int64  `json:"size"`
}

// hfRepoInfo is the repository-info response, in the parts this package reads.
// The fields down to GGUF are what a repository's BUILDS are read from, here; the
// ones after them are what a repository is read as a MODEL from, in hfmodel.go.
// One struct holds the response shape either way, so there is one place to look
// when HuggingFace changes it.
type hfRepoInfo struct {
	ID       string                 `json:"id"`
	Author   string                 `json:"author"`
	SHA      string                 `json:"sha"`
	Gated    interface{}            `json:"gated"`
	Tags     []string               `json:"tags"`
	Siblings []hfSibling            `json:"siblings"`
	GGUF     map[string]interface{} `json:"gguf"`

	PipelineTag string                 `json:"pipeline_tag"` // the task it is filed under, e.g. "text-generation"; absent on plenty of repositories
	CreatedAt   string                 `json:"createdAt"`    // when the repository was created
	Safetensors *hfSafetensors         `json:"safetensors"`  // the stored-element counts; absent on a repository with no safetensors
	Config      map[string]interface{} `json:"config"`       // a small pre-read slice of config.json, carrying architectures and model_type

	// TransformersInfo carries the task the library metadata files the model
	// under, which can differ from pipeline_tag; CardData is the model card's
	// own front matter, where base_model is stated.
	TransformersInfo *hfTransformersInfo    `json:"transformersInfo"`
	CardData         map[string]interface{} `json:"cardData"`
}

// hfTransformersInfo is the part of the repository-info response's
// transformersInfo block this package reads.
type hfTransformersInfo struct {
	PipelineTag string `json:"pipeline_tag"`
}

// hfSafetensors is the element count HuggingFace reports per storage type, and
// their sum. It counts STORED ELEMENTS rather than parameters - see
// parametersOf, which is the only thing that reads it.
type hfSafetensors struct {
	Parameters map[string]int64 `json:"parameters"`
	Total      int64            `json:"total"`
}

// HFRepoBuilds reads one repository and returns the builds it offers, with each
// build's exact size in bytes.
//
// A GATED repository still answers this call in full - the refusal comes later,
// when a file is fetched - so Gated is reported on the result rather than
// returned as an error. Nothing here downloads a build; the app records where it
// is and how big it is, and the user downloads it themselves.
func HFRepoBuilds(repo string) (*HFRepoDetail, error) {
	repo, err := NormalizeHFRepo(repo)
	if err != nil {
		return nil, err
	}
	body, _, status, err := hfGet(fmt.Sprintf("%s/models/%s?blobs=true", HFAPIBaseURL, repo))
	if err != nil {
		return nil, fmt.Errorf("read the repository %s: %w", repo, err)
	}
	switch {
	case status == http.StatusNotFound:
		return nil, fmt.Errorf("no HuggingFace repository called %s", repo)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, fmt.Errorf("%s needs an account to read; ModelsDB signs in to nothing", repo)
	case status != http.StatusOK:
		return nil, fmt.Errorf("read the repository %s: HuggingFace answered %d", repo, status)
	}
	var info hfRepoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("read the repository %s: %w", repo, err)
	}

	detail := &HFRepoDetail{
		Repo:      repo,
		Revision:  info.SHA,
		Gated:     isGated(info.Gated),
		Author:    info.Author,
		BaseModel: baseModelFromTags(info.Tags),
		GGUF:      ggufFacts(info.GGUF),
		Builds:    buildsFrom(info.Siblings, info.Tags),
	}
	return detail, nil
}

// FindBuild returns the named build of a repository, and whether it was there.
// The importer re-reads the repository and looks the build up here rather than
// trusting a size, a format or a label sent back by a caller.
func (d *HFRepoDetail) FindBuild(id string) (HFBuild, bool) {
	for _, b := range d.Builds {
		if b.ID == id {
			return b, true
		}
	}
	return HFBuild{}, false
}

// ---- classifying what a repository holds -------------------------------------

// quantTokenRe matches one quantization label as it appears in a GGUF filename:
// the llama.cpp family (Q4_K_M, Q8_0, IQ2_M, TQ1_0) and the unquantized dumps
// alongside them (BF16, F16, F32, MXFP4).
var quantTokenRe = regexp.MustCompile(`(?i)^(IQ\d+[A-Z0-9_]*|Q\d+[A-Z0-9_]*|TQ\d+[A-Z0-9_]*|BF16|FP16|F16|FP32|F32|MXFP4)$`)

// shardSuffixRe matches the "-00001-of-00002" a multi-file build carries. One
// quantization split over several files is ONE build, so the suffix is stripped
// before the files are grouped.
var shardSuffixRe = regexp.MustCompile(`-\d{4,6}-of-\d{4,6}$`)

// weightFormats pairs the file extensions a model's weights arrive in with the
// format each extension IS. Everything else in a repository (the card, the
// tokenizer, the licence) is not a build.
//
// One table serves both questions asked of a filename - is this a weight file,
// and what format is it - so a file that counts towards a build always names the
// format of that build. That is what leaves no room for a default: a repository
// of .pth files is a PyTorch build because its files are PyTorch files, and
// nothing has to assume the format nobody stated.
var weightFormats = []struct{ ext, format string }{
	{".gguf", "GGUF"},
	{".safetensors", "safetensors"},
	{".bin", "PyTorch"},
	{".pt", "PyTorch"},
	{".pth", "PyTorch"},
	{".npz", "NumPy npz"},
}

// formatOfFile returns the format a filename's extension says it is, and "" for
// a file that is not a weight file at all.
func formatOfFile(path string) string {
	lower := strings.ToLower(path)
	for _, w := range weightFormats {
		if strings.HasSuffix(lower, w.ext) {
			return w.format
		}
	}
	return ""
}

func hasWeightExt(path string) bool {
	return formatOfFile(path) != ""
}

// formatOfFiles names the format a set of weight files is stored in. A
// repository holding more than one kind gets all of them named, in the table's
// order, because the build is every one of those files and picking one of them
// to report would be a claim about the rest.
func formatOfFiles(paths []string) string {
	var named []string
	for _, w := range weightFormats {
		if slices.Contains(named, w.format) {
			continue
		}
		for _, path := range paths {
			if formatOfFile(path) == w.format {
				named = append(named, w.format)
				break
			}
		}
	}
	return strings.Join(named, " + ")
}

// quantLabelOf reads the quantization label out of a GGUF file's name, by taking
// the LAST dash-separated part that looks like one. Reading from the end is what
// keeps a model name containing a number from being mistaken for a label.
// It returns "" when the name carries no label at all.
func quantLabelOf(path string) string {
	name := path
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".gguf")
	name = strings.TrimSuffix(name, ".GGUF")
	name = shardSuffixRe.ReplaceAllString(name, "")
	parts := strings.Split(name, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		if quantTokenRe.MatchString(parts[i]) {
			return strings.ToUpper(parts[i])
		}
	}
	return ""
}

// quantBitsRe takes the bit width off the front of a quantization label.
var quantBitsRe = regexp.MustCompile(`^(?:IQ|TQ|Q)(\d+)`)

// bitsOfLabel reads the bits-per-weight out of a quantization label: the digits
// after Q/IQ/TQ (Q4_K_M -> 4), or the width of an unquantized dump (BF16 -> 16).
// It returns 0 when the label does not say, which is stored as "no value" rather
// than as a guess.
func bitsOfLabel(label string) int {
	l := strings.ToUpper(strings.TrimSpace(label))
	switch l {
	case "BF16", "FP16", "F16":
		return 16
	case "F32", "FP32":
		return 32
	case "MXFP4":
		return 4
	}
	digits := quantBitsRe.FindStringSubmatch(l)
	if len(digits) == 2 {
		if n, err := strconv.Atoi(digits[1]); err == nil {
			return n
		}
	}
	return 0
}

// formatFromTags reads what a repository publishes out of its own tags. The tags
// are applied by whoever uploaded the repository, so this is a hint the picker
// shows, never the authority: the authority for a GGUF repository is each file's
// own name, read above.
func formatFromTags(tags []string) (format string, bit int) {
	for _, raw := range tags {
		t := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case t == "gguf":
			format = "GGUF"
		case t == "awq":
			format = "AWQ"
		case t == "gptq":
			format = "GPTQ"
		case t == "exl2":
			format = "EXL2"
		case t == "mlx":
			format = "MLX"
		case t == "fp8":
			format, bit = "FP8", 8
		case t == "bitsandbytes" || strings.HasPrefix(t, "bnb"):
			if format == "" {
				format = "bitsandbytes"
			}
		case t == "4-bit":
			bit = 4
		case t == "8-bit":
			bit = 8
		case t == "16-bit":
			bit = 16
		case t == "2-bit":
			bit = 2
		}
	}
	return format, bit
}

// baseModelFromTags returns the repository a build was made from, taken from the
// PLAIN `base_model:<owner>/<name>` tag. The tags carry a qualified twin
// (`base_model:quantized:<repo>`, `base_model:finetune:<repo>`) naming the
// relationship, and the plain one is the address to fetch. The chain can be
// longer than one hop - a build of a fine-tune of a base model - so this names
// the NEXT repository up, not the original.
func baseModelFromTags(tags []string) string {
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if !strings.HasPrefix(t, "base_model:") {
			continue
		}
		rest := strings.TrimPrefix(t, "base_model:")
		// The qualified form carries the relationship in the middle, so it has two
		// colons in total; the plain form is exactly "<owner>/<name>".
		if strings.Contains(rest, ":") {
			continue
		}
		if strings.Contains(rest, "/") {
			return rest
		}
	}
	return ""
}

// ggufFacts copies the small, scalar facts off a GGUF repository's own gguf
// block. It copies them one by one on purpose: that block also carries
// chat_template, which is kilobytes of template text and belongs in no column of
// ours. totalFileSize is deliberately not among them either - it is the size of
// ONE of the repository's files rather than a total, so it would be read as a
// number it is not.
func ggufFacts(g map[string]interface{}) map[string]string {
	if g == nil {
		return nil
	}
	out := map[string]string{}
	for _, k := range []string{"architecture", "context_length", "total", "bos_token", "eos_token"} {
		v, ok := g[k]
		if !ok || v == nil {
			continue
		}
		if f, isNum := v.(float64); isNum {
			out[k] = strconv.FormatInt(int64(f), 10)
			continue
		}
		if s, isStr := v.(string); isStr && s != "" {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildsFrom turns a repository's file list into the builds it offers.
//
// The two shapes are genuinely different, which is why there are two branches
// and not one general rule:
//
//   - A GGUF repository holds ONE FILE PER QUANTIZATION, so it offers many
//     builds: the files are grouped by their quantization label (a label split
//     over "-00001-of-00002" shards is one build), and each group is one entry.
//     The mmproj files are kept and marked: they are vision projectors, loaded
//     ALONGSIDE a quantization rather than being one, so listing them as builds
//     would be wrong and hiding them would leave a user hunting for a file the
//     repository plainly has.
//   - Every other repository holds ONE build, spread over shards, whose format
//     is what its tags say. All its weight files are summed into a single entry.
func buildsFrom(siblings []hfSibling, tags []string) []HFBuild {
	var gguf, other []hfSibling
	for _, s := range siblings {
		if s.Path == "" || !hasWeightExt(s.Path) {
			continue
		}
		if strings.HasSuffix(strings.ToLower(s.Path), ".gguf") {
			gguf = append(gguf, s)
			continue
		}
		other = append(other, s)
	}

	if len(gguf) > 0 {
		return ggufBuilds(gguf)
	}
	if len(other) == 0 {
		return nil
	}

	var total int64
	files := make([]string, 0, len(other))
	for _, s := range other {
		total += s.Size
		files = append(files, s.Path)
	}
	sort.Strings(files)

	// The tags name a quantization SCHEME - AWQ, GPTQ, FP8 - which is stored
	// inside safetensors files and which no filename reveals, so they are read
	// first. When they name nothing, the files themselves are the only evidence
	// of what the build is: a repository of .pth files is a PyTorch build, and
	// calling it safetensors would put a fact in the catalog that its own file
	// list contradicts.
	format, bit := formatFromTags(tags)
	if format == "" {
		format = formatOfFiles(files)
	}
	label := format
	if bit > 0 {
		label = fmt.Sprintf("%s %d-bit", format, bit)
	}
	return []HFBuild{{
		ID:     "",
		Label:  label,
		Format: format,
		Bit:    bit,
		Bytes:  total,
		Files:  files,
	}}
}

// ggufBuilds groups a GGUF repository's files into one entry per quantization.
func ggufBuilds(files []hfSibling) []HFBuild {
	type group struct {
		build HFBuild
		key   string
	}
	byKey := map[string]*group{}
	var order []string

	for _, f := range files {
		lower := strings.ToLower(f.Path)
		projector := strings.Contains(lower, "mmproj")
		label := quantLabelOf(f.Path)
		if label == "" {
			// A .gguf file whose name carries no label at all is still a real file
			// somebody may want, so it is offered under its own name rather than
			// dropped.
			label = strings.TrimSuffix(f.Path[strings.LastIndex(f.Path, "/")+1:], ".gguf")
		}
		// The key groups the shards of one quantization: the directory it sits in
		// plus its label. Two quantizations of the same name in two directories
		// stay two builds, which is what a repository publishing BF16/ and Q4/
		// subfolders means.
		dir := ""
		if i := strings.LastIndex(f.Path, "/"); i >= 0 {
			dir = f.Path[:i+1]
		}
		key := dir + label
		if projector {
			key = dir + "mmproj:" + label
		}
		g, ok := byKey[key]
		if !ok {
			g = &group{key: key, build: HFBuild{
				ID:        f.Path,
				Label:     label,
				Format:    "GGUF",
				Projector: projector,
			}}
			if !projector {
				g.build.Bit = bitsOfLabel(label)
			}
			byKey[key] = g
			order = append(order, key)
		}
		g.build.Bytes += f.Size
		g.build.Files = append(g.build.Files, f.Path)
		// The lowest-sorting path is the build's stable id, so re-reading the
		// repository names the same build however the listing is ordered.
		if f.Path < g.build.ID {
			g.build.ID = f.Path
		}
	}

	out := make([]HFBuild, 0, len(order))
	for _, key := range order {
		b := byKey[key].build
		sort.Strings(b.Files)
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Projector != out[j].Projector {
			return !out[i].Projector // real quantizations first, projectors after them
		}
		return out[i].Bytes < out[j].Bytes
	})
	return out
}

// ---- the config behind a repository ------------------------------------------

// HFConfig reads a repository's config.json.
//
// It is fetched through /resolve/, never /raw/. What git stores for an
// LFS-tracked file is a three-line pointer served with status 200, so /raw/ can
// hand back prose where JSON was expected; /resolve/ returns the file itself
// whether or not LFS is involved. The pointer is checked for anyway, because a
// silent wrong answer here would be written into the database as fact.
//
// A missing config is not an error: a GGUF repository often has none of its own,
// and the config only fills facts beside the ones the import is for. The answer
// is then nil with no error.
func HFConfig(repo string) (map[string]interface{}, error) {
	cfg, _, err := fetchConfig(repo)
	return cfg, err
}

// fetchConfig reads one repository's config.json. The bool reports whether a
// real config came back; a 404 is "not here", not a failure.
func fetchConfig(repo string) (map[string]interface{}, bool, error) {
	body, _, status, err := hfGet(fmt.Sprintf("%s/%s/resolve/main/config.json", HFResolveBaseURL, strings.Trim(repo, "/")))
	if err != nil {
		return nil, false, fmt.Errorf("read the architecture of %s: %w", repo, err)
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, false, fmt.Errorf("%s needs an account to read its architecture", repo)
	default:
		return nil, false, fmt.Errorf("read the architecture of %s: HuggingFace answered %d", repo, status)
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "version https://git-lfs") {
		return nil, false, fmt.Errorf("%s stores its config.json in Git LFS and the pointer came back instead of the file", repo)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, false, fmt.Errorf("read the architecture of %s: %w", repo, err)
	}
	return cfg, true, nil
}
