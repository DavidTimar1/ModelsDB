// Reading a HuggingFace repository as a MODEL row.
//
// huggingface.go answers "which VARIATIONS has somebody published of
// this model". This file answers the other question: what IS the model in this
// repository, as a row of this catalog - who published it, what it takes in and
// gives back, how long a context it holds, and how much disk its weights take.
// It exists so the server can build a whole row from a repository address alone,
// with no number typed by whoever asked for it.
//
// Three addresses are read here, and no others:
//
//	GET <api>/models/<repo>?blobs=true                              the repository, with every file's exact size
//	GET <resolve>/<repo>/resolve/main/model.safetensors.index.json  which of those files the weights actually are
//	GET <resolve>/<repo>/resolve/main/config.json                   the architecture and the context length
//
// THE SHARD INDEX IS THE WHOLE POINT OF THE SECOND ADDRESS. A repository holds
// COPIES of its own weights, so summing every weight file it lists gives an
// answer that is two or three times too big while looking entirely plausible:
//
//	mistralai/Mistral-7B-Instruct-v0.3  index 14,496,080,928 bytes   every weight file 28,992,159,440
//	openai/gpt-oss-20b                  index 13,761,316,904 bytes   every weight file 41,273,504,288
//
// The first ships consolidated.safetensors beside its three shards, the second
// ships original/ and metal/ copies beside its own. So the size is ALWAYS the
// distinct files the index names, and where no index can be read the size is
// left unset rather than guessed - a column this app presents as a measurement
// holds a measurement or it holds nothing.
//
// Two more things are read cautiously rather than eagerly:
//
//   - PARAMETERS come from the repository's safetensors block, which counts
//     STORED ELEMENTS rather than parameters. A packed low-bit repository puts
//     several weights in one element, so the count is only used when every
//     storage type in it is a full-precision one (see parametersOf).
//   - A GATED repository answers the first address in full and refuses the other
//     two, so this stops there and reports it. The caller decides what a gated
//     repository means; nothing here half-reads one.
//
// Requests go out through hfGet, so they carry the one plain browser User-Agent
// and nothing that names this program or its owner.

package catalog

import (
	"encoding/json"
	"fmt"
	"modelsdb/internal/dbcore"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// HFModel is one HuggingFace repository read as a model. Every value on it was
// read off HuggingFace; a field the repository does not state is left at its
// zero value rather than filled with a guess.
type HFModel struct {
	Repo             string   // the <owner>/<name> repository id, which is what hf_slug stores
	Author           string   // who published it
	Revision         string   // the commit main pointed at when it was read; main moves
	Gated            bool     // its owner requires an account, so its files cannot be read here
	CreatedAt        string   // when the repository was created, as HuggingFace reports it
	PipelineTag      string   // the task HuggingFace files it under, e.g. "text-generation"
	Architecture     string   // the first entry of the config's architectures list
	Description      string   // one line built from what was read, never from a caller
	ModelType        string   // chat | image | video | embedding | rerank | tts | music | stt | decisions, or "" when the task does not say
	InputModalities  []string //
	OutputModalities []string //
	ContextLength    int      // max tokens; 0 when nothing read states one
	Parameters       float64  // total parameters in BILLIONS; 0 when the count cannot be trusted (see parametersOf)
	MoE              string   // "yes", "no", or "" when no config could be read
	WeightBytes      int64    // the exact size of the weight files; 0 when no index or single-file weight set was found
	WeightBasis      string   // which files that size was summed from, in words
	BaseModel        string   // the repository (or repositories) the card or the base_model: tag names this one as made from; "" when it names none
	GGUFArchitecture string   // the architecture name inside a GGUF repository's own gguf block; "" on any other repository
}

// DiskSizeGB is the weight size in decimal GB, the unit disk_size_gb holds and
// the one HuggingFace shows on its own Files tab. It is 0 when the weight set
// could not be established.
func (m *HFModel) DiskSizeGB() float64 {
	if m.WeightBytes <= 0 {
		return 0
	}
	return float64(m.WeightBytes) / 1e9
}

// hfHosts are the addresses a HuggingFace model URL is served from. A URL on any
// other host is not a HuggingFace repository, however similar its shape.
var hfHosts = map[string]bool{
	"huggingface.co":     true,
	"www.huggingface.co": true,
	"hf.co":              true,
}

// hfNonModelSections are the parts of the site that are not models. Each is a
// two-segment address exactly like a model's, so without this list
// huggingface.co/datasets/squad would be read as the repository "datasets/squad".
var hfNonModelSections = map[string]string{
	"datasets":      "dataset",
	"spaces":        "Space",
	"collections":   "collection",
	"papers":        "paper",
	"blog":          "blog post",
	"docs":          "documentation page",
	"organizations": "organization page",
}

// NormalizeHFRepo turns what a person actually pastes into the repository id
// <owner>/<name>. It accepts the full URL from a browser (with any trailing
// /tree/main, /blob/... path and any query string), the same URL without its
// scheme, and the bare owner/name a person types from memory.
//
// It refuses rather than guesses: an address on another host, a dataset or Space
// address, and anything that does not carry both an owner and a name.
func NormalizeHFRepo(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("a HuggingFace model address is required, e.g. https://huggingface.co/Qwen/Qwen3-32B")
	}
	// Drop the scheme, then anything a browser appended after the path.
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	segs := []string{}
	for _, part := range strings.Split(s, "/") {
		if part = strings.TrimSpace(part); part != "" {
			segs = append(segs, part)
		}
	}
	if len(segs) == 0 {
		return "", fmt.Errorf("%q names no repository", raw)
	}
	// A pasted URL opens with the host. A host that is not HuggingFace's is a
	// different site, and an owner name never carries a dot, so anything else
	// containing one is a host too.
	if hfHosts[strings.ToLower(segs[0])] {
		segs = segs[1:]
	} else if strings.Contains(segs[0], ".") {
		return "", fmt.Errorf("%q is not a huggingface.co address", raw)
	}
	if len(segs) > 0 {
		if section, isSection := hfNonModelSections[strings.ToLower(segs[0])]; isSection {
			return "", fmt.Errorf("%q addresses a %s rather than a model", raw, section)
		}
		// huggingface.co/models/<owner>/<name> is the same repository as
		// huggingface.co/<owner>/<name>.
		if strings.EqualFold(segs[0], "models") {
			segs = segs[1:]
		}
	}
	if len(segs) < 2 {
		return "", fmt.Errorf("%q names no owner and model; a repository address looks like Qwen/Qwen3-32B", raw)
	}
	owner, name := segs[0], segs[1]
	if owner == "." || owner == ".." || name == "." || name == ".." ||
		!hfRepoSegmentRe.MatchString(owner) || !hfRepoSegmentRe.MatchString(name) {
		return "", fmt.Errorf("%q is not a repository address", raw)
	}
	return owner + "/" + name, nil
}

// hfRepoSegmentRe is what an owner or a repository name may be made of on
// HuggingFace. Anything else would let a pasted address change which API path is
// read once it is spliced into one.
var hfRepoSegmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// HFModelRecord reads one repository and returns it as a model.
//
// A GATED repository comes back with Gated set and nothing else read: its
// metadata answers in full while its files refuse, so every remaining fact would
// be missing anyway. Deciding what that means is the caller's, which is why it
// is a field rather than an error.
func HFModelRecord(repo string) (*HFModel, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return nil, fmt.Errorf("a HuggingFace repository id is required")
	}
	body, _, status, err := hfGet(fmt.Sprintf("%s/models/%s?blobs=true", HFAPIBaseURL, repo))
	if err != nil {
		return nil, fmt.Errorf("read the repository %s: %w", repo, err)
	}
	switch {
	case status == http.StatusNotFound:
		return nil, fmt.Errorf("no HuggingFace repository called %s", repo)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// One status covers three situations here: HuggingFace answers 401 for a
		// repository that needs an account, for a private one, and for one that
		// does not exist at all. The message says all three rather than picking.
		return nil, fmt.Errorf("%s cannot be read: HuggingFace answered %d, which means it needs an account, is private, or is not there. ModelsDB signs in to nothing", repo, status)
	case status != http.StatusOK:
		return nil, fmt.Errorf("read the repository %s: HuggingFace answered %d", repo, status)
	}
	var info hfRepoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("read the repository %s: %w", repo, err)
	}

	m := &HFModel{
		Repo:        repo,
		Author:      strings.TrimSpace(info.Author),
		Revision:    info.SHA,
		Gated:       isGated(info.Gated),
		CreatedAt:   info.CreatedAt,
		PipelineTag: strings.TrimSpace(info.PipelineTag),
	}
	if m.Author == "" {
		m.Author = ownerOf(repo)
	}
	if m.Gated {
		return m, nil
	}

	m.WeightBytes, m.WeightBasis = weightBytesOf(repo, info.Siblings)

	// A missing or unreadable config is an absence, not a failure: the fields it
	// would have filled stay unset and the rest of the record still stands.
	config, _ := HFConfig(repo)
	gguf := ggufFacts(info.GGUF)
	m.ContextLength = contextLengthOf(config, gguf)
	m.MoE = moeOf(config)
	m.Architecture = architectureOf(config, info.Config)
	m.Parameters = parametersOf(info.Safetensors)
	if m.Parameters == 0 {
		m.Parameters = ggufParametersOf(gguf)
	}
	m.GGUFArchitecture = gguf["architecture"]
	m.BaseModel = baseModelOf(info.Tags, info.CardData)
	transformersTask := ""
	if info.TransformersInfo != nil {
		transformersTask = info.TransformersInfo.PipelineTag
	}
	m.ModelType, m.InputModalities, m.OutputModalities = capabilitiesOf(m.PipelineTag, transformersTask, config, info.Tags)
	card := repoCard(repo)
	if m.ModelType == "tts" {
		m.ModelType = audioTypeOf(m.ModelType, cardOpening(card))
	}
	if isDecisionModel(info.Tags, card) {
		m.ModelType = "decisions"
		m.OutputModalities = decisionOutputs(m.OutputModalities)
	}
	// An embedding model's window is the longest text its sentence-embedding
	// wrapper cuts to, which is far shorter than the position table of the
	// language model it was built on.
	if m.ModelType == "embedding" {
		if n := sentenceMaxSeqLength(repo); n > 0 {
			m.ContextLength = n
		} else if n := cardContextWindow(card); n > 0 {
			m.ContextLength = n
		}
	}
	m.Description = describeRepo(m)
	return m, nil
}

// ownerOf is the part of a repository id left of the slash.
func ownerOf(repo string) string {
	if i := strings.Index(repo, "/"); i > 0 {
		return repo[:i]
	}
	return repo
}

// weightBytesOf returns the exact size of the model's weight files and the words
// for where that size came from. It returns 0 and "" when the weight set cannot
// be established, which is the correct answer because the alternative is a
// number two or three times too large (see this file's opening comment).
//
// Three shapes are tried, in this order:
//
//  1. A SHARDED repository, which publishes model.safetensors.index.json. The
//     files a loader reads are the distinct values of its weight_map, and their
//     sizes come from the listing that was already fetched.
//  2. A repository holding ONE model.safetensors, which has no index at all.
//  3. A repository holding ONE pytorch_model.bin, the older single-file form.
//
// Anything else - a sharded .bin repository, a GGUF repository, an index naming
// a file the listing does not carry - leaves the size unset.
func weightBytesOf(repo string, siblings []hfSibling) (int64, string) {
	sizes := make(map[string]int64, len(siblings))
	for _, s := range siblings {
		if s.Path != "" {
			sizes[s.Path] = s.Size
		}
	}

	if shards, ok := shardIndexFiles(repo); ok {
		var total int64
		for _, file := range shards {
			size, listed := sizes[file]
			if !listed {
				// The index names a file the listing does not carry, so a sum here
				// would be short by an unknown amount. Report nothing rather than a
				// figure that cannot be checked.
				return 0, ""
			}
			total += size
		}
		if total > 0 {
			return total, fmt.Sprintf("the %d file(s) named in model.safetensors.index.json", len(shards))
		}
	}
	for _, single := range []string{"model.safetensors", "pytorch_model.bin"} {
		if size, listed := sizes[single]; listed && size > 0 {
			return size, "the single " + single + ", the whole weight set of an unsharded repository"
		}
	}
	return 0, ""
}

// hfShardIndex is model.safetensors.index.json in the part that matters: which
// file every tensor lives in.
type hfShardIndex struct {
	WeightMap map[string]string `json:"weight_map"`
}

// shardIndexFiles reads the shard index and returns the distinct files it names.
// The bool reports whether a real index came back: a 404 means the repository is
// simply not sharded, and anything unreadable means the size must be left unset
// rather than fall back to summing every file in the repository.
//
// It is fetched through /resolve/, never /raw/, and the answer is checked for a
// Git LFS pointer: /raw/ returns a three-line pointer with status 200 for an
// LFS-tracked file, and a body that is not JSON would otherwise read as "this
// repository has no index" - which is exactly the mistake that doubles the size.
func shardIndexFiles(repo string) ([]string, bool) {
	body, _, status, err := hfGet(fmt.Sprintf("%s/%s/resolve/main/model.safetensors.index.json", HFResolveBaseURL, strings.Trim(repo, "/")))
	if err != nil || status != http.StatusOK {
		return nil, false
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "version https://git-lfs") {
		return nil, false
	}
	var index hfShardIndex
	if err := json.Unmarshal(body, &index); err != nil || len(index.WeightMap) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	var files []string
	for _, file := range index.WeightMap {
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		files = append(files, file)
	}
	return files, len(files) > 0
}

// nestedConfig returns the object holding the language-model fields. On a
// multimodal model they sit inside text_config and nothing is at the top level,
// so a reader that only looks at the top level concludes a field is missing when
// it is one level down.
func nestedConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	if text, ok := config["text_config"].(map[string]interface{}); ok {
		return text
	}
	return config
}

// configNumber reads one numeric field from the config, looking at the top level
// and inside text_config.
func configNumber(config map[string]interface{}, key string) (float64, bool) {
	for _, level := range []map[string]interface{}{config, nestedConfig(config)} {
		if level == nil {
			continue
		}
		if n, ok := level[key].(float64); ok {
			return n, true
		}
	}
	return 0, false
}

// sentenceMaxSeqLength reads max_seq_length from the repository's
// sentence_bert_config.json: the number of tokens a sentence-embedding model cuts
// its input to, which is its real context window. It answers 0 when the file is
// absent, is a Git LFS pointer, or states no positive number.
func sentenceMaxSeqLength(repo string) int {
	body, _, status, err := hfGet(fmt.Sprintf("%s/%s/resolve/main/sentence_bert_config.json", HFResolveBaseURL, strings.Trim(repo, "/")))
	if err != nil || status != http.StatusOK {
		return 0
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return 0
	}
	if n, ok := cfg["max_seq_length"].(float64); ok && n > 0 {
		return int(n)
	}
	return 0
}

// cardWindowRes match a card stating its window in tokens: "Context Window | 8,192
// tokens" and "a single shared 8,192-token context window".
var cardWindowRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)context window\W{0,12}([0-9][0-9,]*)\s*tokens`),
	regexp.MustCompile(`(?i)([0-9][0-9,]*)[- ]tokens?\s+context window`),
}

// cardContextWindow reads the window a model card states in tokens, and answers
// 0 when it states none. It is the fallback for an embedding model whose
// repository has no sentence_bert_config.json max_seq_length.
func cardContextWindow(card string) int {
	for _, re := range cardWindowRes {
		if m := re.FindStringSubmatch(card); m != nil {
			if n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// ggufParametersOf reads the parameter count, in billions, off a GGUF
// repository's own gguf block, where "total" is the count of parameters and not
// of stored elements. It answers 0 when the block states none.
func ggufParametersOf(gguf map[string]string) float64 {
	n, err := strconv.ParseInt(gguf["total"], 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return float64(n) / 1e9
}

// baseModelOf names the repository this one was made from. The plain
// base_model:<owner>/<name> tag is read first, being the one machine-readable
// statement; the card's own base_model field is the fallback, and it may be one
// repository or a list of them, all of which are named.
func baseModelOf(tags []string, cardData map[string]interface{}) string {
	if fromTag := baseModelFromTags(tags); fromTag != "" {
		return fromTag
	}
	switch v := cardData["base_model"].(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		var named []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				named = append(named, strings.TrimSpace(s))
			}
		}
		return strings.Join(named, ", ")
	}
	return ""
}

// contextLengthOf reads the context window: max_position_embeddings from the
// architecture config, or a GGUF repository's own context_length when there is
// no config to read. For an embedding model the caller replaces this with the
// sentence-embedding wrapper's max_seq_length when the repository has one.
func contextLengthOf(config map[string]interface{}, gguf map[string]string) int {
	if n, ok := configNumber(config, "max_position_embeddings"); ok && n > 0 {
		return int(n)
	}
	if raw, ok := gguf["context_length"]; ok {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// moeExpertKeys are the field names a Mixture-of-Experts config states its
// expert count under. They differ per architecture, so several are checked.
var moeExpertKeys = []string{"num_experts", "num_local_experts", "n_routed_experts", "moe_num_experts", "num_experts_per_tok"}

// moeOf reports whether the config describes a Mixture-of-Experts model: "yes",
// "no", or "" when no config could be read at all, which is stored as unknown
// rather than as a no.
func moeOf(config map[string]interface{}) string {
	if config == nil {
		return ""
	}
	for _, key := range moeExpertKeys {
		if n, ok := configNumber(config, key); ok && n > 1 {
			return "yes"
		}
	}
	return "no"
}

// architectureOf names the model's architecture class, from the config file or
// from the small config block the repository listing already carries.
func architectureOf(config, listed map[string]interface{}) string {
	for _, source := range []map[string]interface{}{config, nestedConfig(config), listed} {
		if source == nil {
			continue
		}
		names, ok := source["architectures"].([]interface{})
		if !ok || len(names) == 0 {
			continue
		}
		if first, ok := names[0].(string); ok && first != "" {
			return first
		}
	}
	return ""
}

// fullPrecisionDTypes are the storage types that hold ONE parameter per stored
// element, so that a count of elements is a count of parameters.
var fullPrecisionDTypes = map[string]bool{
	"F64": true, "F32": true, "F16": true, "BF16": true,
	"F8_E4M3": true, "F8_E5M2": true,
}

// parametersOf returns the total parameter count in BILLIONS, and 0 when the
// repository does not state one that can be trusted.
//
// The safetensors block counts STORED ELEMENTS, not parameters, and a packed
// low-bit repository puts several weights into one element - mlx-community's
// 4-bit Qwen3-32B reports 5.1 billion elements for a 32-billion-parameter model,
// eight weights to each 32-bit element. There is no reliable way to tell from
// the block alone how many weights a packed element holds, so a repository
// carrying ANY storage type that is not full precision yields no count at all
// and the field is left for a person to research.
//
// A count UNDER a billion is kept like any other. The column stores decimals, so
// a 0.6-billion model is recorded as 0.6 rather than lost - which is the whole
// point of a measurement read off the repository.
func parametersOf(st *hfSafetensors) float64 {
	if st == nil || st.Total <= 0 {
		return 0
	}
	for dtype := range st.Parameters {
		if !fullPrecisionDTypes[strings.ToUpper(strings.TrimSpace(dtype))] {
			return 0
		}
	}
	return float64(st.Total) / 1e9
}

// hfCapability is what one HuggingFace task says a model does, in this catalog's
// own terms.
type hfCapability struct {
	modelType string
	in        []string
	out       []string
}

// hfPipelineCapabilities maps the HuggingFace task tag to a model_type and the
// two modality lists. The tag ids are HuggingFace's own, read from
// https://huggingface.co/api/models-tags-by-type; a task not listed here leaves
// the type empty, which stores the model as an ordinary chat row for a person to
// correct rather than inventing a capability for it (capabilitiesOf says what
// is still recorded for such a model).
//
// Every audio-out task maps to tts, INCLUDING text-to-audio, which is the tag a
// music generator usually carries. The task cannot separate a music model from
// a speech one - both are text in, audio out - so audioTypeOf reads the model
// card's opening for the audio-out tasks and files the model as music when it
// says it makes music (see dbcore.DescribesMusicGeneration).
var hfPipelineCapabilities = map[string]hfCapability{
	"text-generation":              {"chat", []string{"text"}, []string{"text"}},
	"image-text-to-text":           {"chat", []string{"text", "image"}, []string{"text"}},
	"visual-question-answering":    {"chat", []string{"text", "image"}, []string{"text"}},
	"document-question-answering":  {"chat", []string{"text", "image"}, []string{"text"}},
	"video-text-to-text":           {"chat", []string{"text", "video"}, []string{"text"}},
	"audio-text-to-text":           {"chat", []string{"text", "audio"}, []string{"text"}},
	"any-to-any":                   {"chat", []string{"text", "image", "audio"}, []string{"text", "image", "audio"}},
	"text-to-image":                {"image", []string{"text"}, []string{"image"}},
	"image-to-image":               {"image", []string{"image"}, []string{"image"}},
	"image-text-to-image":          {"image", []string{"text", "image"}, []string{"image"}},
	"text-to-video":                {"video", []string{"text"}, []string{"video"}},
	"image-to-video":               {"video", []string{"image"}, []string{"video"}},
	"image-text-to-video":          {"video", []string{"text", "image"}, []string{"video"}},
	"video-to-video":               {"video", []string{"video"}, []string{"video"}},
	"automatic-speech-recognition": {"stt", []string{"audio"}, []string{"text"}},
	"text-to-speech":               {"tts", []string{"text"}, []string{"audio"}},
	"text-to-audio":                {"tts", []string{"text"}, []string{"audio"}},
	"feature-extraction":           {"embedding", []string{"text"}, []string{"embedding"}},
	"sentence-similarity":          {"embedding", []string{"text"}, []string{"embedding"}},
	"image-feature-extraction":     {"embedding", []string{"image"}, []string{"embedding"}},
	"text-ranking":                 {"rerank", []string{"text"}, []string{"ranking"}},
}

// cardIntroBytes is how much of a model card's opening is read to tell music
// from speech. A model says what it is in its first paragraphs; a speech model
// that names music generation as something it must not be used for does so in
// a limitations section further down, which this window is meant to stop short
// of.
const cardIntroBytes = 4000

// repoCard returns the whole model card (README.md) without its front matter,
// or "" when no card can be read. A card that cannot be read is an absence and
// not a failure: the type then stays what the task said.
func repoCard(repo string) string {
	body, _, status, err := hfGet(fmt.Sprintf("%s/%s/resolve/main/README.md", HFResolveBaseURL, strings.Trim(repo, "/")))
	if err != nil || status != http.StatusOK {
		return ""
	}
	card := string(body)
	if strings.HasPrefix(card, "---") {
		if end := strings.Index(card[3:], "\n---"); end >= 0 {
			card = card[3+end+4:]
		}
	}
	return card
}

// cardOpening is the first cardIntroBytes of a card.
func cardOpening(card string) string {
	if len(card) > cardIntroBytes {
		return card[:cardIntroBytes]
	}
	return card
}

// decisionTags are the tags a repository carries to say its output is a typed
// decision rather than text.
var decisionTags = map[string]bool{
	"decision-model": true, "typed-decisions": true, "calibrated-probabilities": true,
}

// decisionOutputRe matches the two shapes a decision model states its own
// output in: "a (calibrated) probability for every option|action|choice" and
// "one logit|probability per (allowed) option|action|choice". The article, or
// "one", is what ties the figure to the model's own answer. A benchmark or
// reward-model sentence names a score or a logit "for each choice" with "the" or
// no article, and a classifier says "each", so none of those match.
var decisionOutputRe = regexp.MustCompile(
	`(?i)\b(?:an?\s+(?:[a-z-]+\s+)?probability\s+for\s+every|one\s+(?:logit|probability)\s+per)\s+(?:allowed\s+|possible\s+|candidate\s+|valid\s+)?(?:option|action|choice)s?\b`)

// evaluationWordRe marks a sentence that reports how a model is measured. A
// score per option there describes the test, not what the model returns.
var evaluationWordRe = regexp.MustCompile(
	`(?i)\b(?:benchmarks?|evals?|evaluat\w*|mmlu|accuracy|harness|argmax|metrics?|leaderboard|arena|reward|likelihood)\b`)

// sentenceBreakRe splits a card into the sentences and table rows that
// decisionOutputRe and evaluationWordRe are applied to one at a time.
var sentenceBreakRe = regexp.MustCompile(`[\r\n]+|[.!?]\s+`)

// cardStatesDecisionOutput reports whether some sentence of the card gives the
// model's own output as a probability or logit per option, outside a sentence
// that reports an evaluation.
func cardStatesDecisionOutput(card string) bool {
	for _, sentence := range sentenceBreakRe.Split(card, -1) {
		if decisionOutputRe.MatchString(sentence) && !evaluationWordRe.MatchString(sentence) {
			return true
		}
	}
	return false
}

// isDecisionModel reports whether a repository is a decision model: its tags
// say so, or its whole card states the output as a probability per option. The
// card is read in full because the output wording sits far down it.
func isDecisionModel(tags []string, card string) bool {
	for _, t := range tags {
		if decisionTags[strings.ToLower(strings.TrimSpace(t))] {
			return true
		}
	}
	return cardStatesDecisionOutput(card)
}

// decisionOutputs is the output list of a decision model: its decisions, and
// text as well when the task's own output holds text (the two-mode case, where
// the unchanged base model answers in text).
func decisionOutputs(taskOut []string) []string {
	out := []string{"decisions"}
	for _, m := range taskOut {
		if m == "text" {
			out = append(out, "text")
		}
	}
	return out
}

// audioTypeOf settles an audio-out model's type: music when its card's opening
// describes music generation, and the task's own type otherwise. It is the same
// test OpenRouter's descriptions go through, so one rule and one list decide
// what counts as music everywhere in this app.
func audioTypeOf(taskType, cardIntro string) string {
	if dbcore.DescribesMusicGeneration(cardIntro) {
		return "music"
	}
	return taskType
}

// capabilityOfTask looks one HuggingFace task tag up in the table.
func capabilityOfTask(tag string) (hfCapability, bool) {
	c, known := hfPipelineCapabilities[strings.ToLower(strings.TrimSpace(tag))]
	return c, known
}

// capabilitiesOf turns what the repository states into this catalog's model_type
// and modality lists.
//
// The task is the repository's pipeline_tag; when that is one the table does not
// know, the task its transformers metadata states is used (a decision model
// filed under text-classification still says image-text-to-text there). With no
// known task the type stays empty, but what the config and the tags state about
// what the model takes in is still recorded: a vision_config means images
// whatever the task says, an audio_config means audio, and a video token or
// video_config means video.
//
// Audio and video are added only to a model that takes text in (a chat,
// embedding or rerank model, or one whose task is unknown): a speech model's
// audio_config describes what it makes, not what it reads.
func capabilitiesOf(pipelineTag, transformersTag string, config map[string]interface{}, tags []string) (string, []string, []string) {
	capability, known := capabilityOfTask(pipelineTag)
	if !known {
		capability, known = capabilityOfTask(transformersTag)
	}
	var in, out []string
	if known {
		in = append(in, capability.in...)
		out = append(out, capability.out...)
	} else if _, hasText := config["text_config"]; hasText {
		in = []string{"text"}
	}
	if config != nil {
		if _, hasVision := config["vision_config"]; hasVision {
			in = withModality(in, "image")
		}
	}
	if textIn := !known || capability.modelType == "chat" || capability.modelType == "embedding" || capability.modelType == "rerank"; textIn {
		for _, m := range modalitiesStatedByConfig(config) {
			in = withModality(in, m)
		}
		for _, m := range modalitiesStatedByTags(tags) {
			in = withModality(in, m)
		}
	}
	// With no task, "text" alone is a guess from the shape of the config and
	// nothing the repository said, so it is not recorded.
	if !known && (len(in) == 0 || (len(in) == 1 && in[0] == "text")) {
		return "", nil, nil
	}
	return capability.modelType, in, out
}

// modalitiesStatedByConfig names the input modalities beyond text that a
// model's config.json states, looking at the top level and inside text_config:
// an audio_config is audio, and a video_config or a video token id is video.
func modalitiesStatedByConfig(config map[string]interface{}) []string {
	var out []string
	for _, level := range []map[string]interface{}{config, nestedConfig(config)} {
		if level == nil {
			continue
		}
		if _, has := level["audio_config"]; has {
			out = withModality(out, "audio")
		}
		for _, key := range []string{"video_config", "video_token_id", "video_token_index"} {
			if _, has := level[key]; has {
				out = withModality(out, "video")
			}
		}
	}
	return out
}

// modalitiesStatedByTags names the input modalities the repository's own tags
// state in so many words: the tags "audio", "video", "image" and "vision".
func modalitiesStatedByTags(tags []string) []string {
	var out []string
	for _, raw := range tags {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "image", "vision":
			out = withModality(out, "image")
		case "audio":
			out = withModality(out, "audio")
		case "video":
			out = withModality(out, "video")
		}
	}
	return out
}

// withModality adds one modality to a list unless it is already there.
func withModality(list []string, add string) []string {
	for _, v := range list {
		if v == add {
			return list
		}
	}
	return append(list, add)
}

// describeRepo writes the one line the table shows about the model. It is built
// from what was read and names nothing that was not: whoever imports the model
// supplies no text, so this can never carry a claim the repository did not make.
func describeRepo(m *HFModel) string {
	parts := []string{fmt.Sprintf("%s on HuggingFace", m.Repo)}
	if m.Author != "" {
		parts[0] += ", published by " + m.Author
	}
	if m.PipelineTag != "" {
		parts = append(parts, "HuggingFace files it under "+m.PipelineTag)
	}
	if m.Architecture != "" {
		parts = append(parts, "architecture "+m.Architecture)
	}
	return strings.Join(parts, ". ") + "."
}
