// Which columns can apply to one row at all.
//
// The research brief offers only the columns that can describe the row. Most
// columns describe something a given row does not have: an OpenRouter listing's
// prices on a model that is only a HuggingFace repository, an image model's
// reference images on an embedding model, a quantized build's file list on the
// base repository it was made from. An assistant asked about a field that cannot
// apply either leaves it empty and wastes the answer, or invents one.
//
// THIS FILE IS THE ONE PLACE THAT DECIDES IT. Applies answers for one column on
// one row, from three things a row states about itself and nothing else: whether
// OpenRouter lists it, what kind of model it is, and which families of
// measurement it already holds. A column with no rule applies everywhere, so a
// column added later is asked about until a rule here says otherwise.

package store

import "strings"

// The model types a rule can name. They are the model_type values the app
// stores; a new kind of model is one more constant and the rules that mention it.
const (
	typeChat      = "chat"
	typeImage     = "image"
	typeVideo     = "video"
	typeEmbedding = "embedding"
	typeRerank    = "rerank"
	typeTTS       = "tts"
	typeSTT       = "stt"
	typeMusic     = "music"
	typeDecisions = "decisions"
)

// Families of measurement that exist on only some rows. A family applies to a
// row once the row holds a value from it, or, for a build, once the row IS a
// build.
const (
	familyNone  = ""
	familyBuild = "build"
	familyGGUF  = "gguf"
	// familyWeightFiles: the size and format of the weight files, which the
	// importer reads from a shard index or a single weights file. A GGUF
	// repository holds one file per quantization and none of them is "the"
	// weights, so a GGUF main row is offered none of these.
	familyWeightFiles = "weightfiles"
)

// fieldRule says what a column needs of the row it is offered on.
type fieldRule struct {
	// listing: the column describes OpenRouter's listing or endpoint, so the row
	// has to be one OpenRouter lists.
	listing bool
	// types: the model types the column describes. Nil means every type.
	types []string
	// family: the family of measurement the row has to hold or be.
	family string
}

var (
	// languageModels are the types a context window and a mixture-of-experts
	// layout describe.
	languageModels = []string{typeChat, typeDecisions, typeEmbedding, typeRerank}
	// generators are the types that make pictures or footage.
	generators = []string{typeImage, typeVideo}
	chatLike   = []string{typeChat, typeDecisions}
	onlyImage  = []string{typeImage}
	onlyVideo  = []string{typeVideo}
	onlySpeech = []string{typeTTS}
)

// fieldRules is the rule per column key. Keys not named here, and not caught by
// a prefix below, apply to every row.
var fieldRules = map[string]fieldRule{
	// OpenRouter's listing, whatever the model is.
	"id": {listing: true}, "slug": {listing: true}, "vendor": {listing: true},
	"moderated": {listing: true}, "expiration_date": {listing: true},
	"alias_target": {listing: true}, "alias_target_name": {listing: true},
	"zdr": {listing: true}, "eu_routing": {listing: true}, "supported_parameters": {listing: true},
	"top_provider_context_length": {listing: true}, "endpoints": {listing: true},
	"links_details": {listing: true}, "allowed_passthrough_parameters": {listing: true},
	"supports_streaming": {listing: true}, "architecture_modality": {listing: true},
	"creativity": {listing: true}, "seed": {listing: true},
	"price_image": {listing: true}, "price_audio": {listing: true},
	"price_audio_cache_read": {listing: true},

	// OpenRouter's listing of a language model.
	"tokenizer": {listing: true, types: chatLike}, "instruct_type": {listing: true, types: chatLike},
	"max_completion_tokens":    {listing: true, types: chatLike},
	"price_internal_reasoning": {listing: true, types: chatLike},
	"price_cache_read":         {listing: true, types: chatLike}, "price_cache_write": {listing: true, types: chatLike},
	"price_cache_write_1h": {listing: true, types: chatLike}, "price_web_search": {listing: true, types: chatLike},

	// OpenRouter's price lines for the media a model produces.
	"price_image_output": {listing: true, types: onlyImage},
	"price_audio_output": {listing: true, types: []string{typeTTS, typeMusic}},

	// What a language model can do, which the model itself states.
	"supports_tools": {types: chatLike}, "supports_reasoning": {types: chatLike},
	"reasoning_mandatory": {types: chatLike}, "reasoning_default_effort": {types: chatLike},
	"reasoning_default_enabled": {types: chatLike}, "reasoning_supported_efforts": {types: chatLike},
	"reasoning_supports_max_tokens": {types: chatLike},

	// What a picture or video model can do.
	"reference_images": {types: generators}, "max_reference_images": {types: generators},
	"supported_aspect_ratios": {types: generators}, "supported_sizes": {types: generators},
	"supported_resolutions": {types: onlyVideo},

	// A context window and a mixture-of-experts layout describe language models.
	"context_length": {types: languageModels}, "moe": {types: languageModels},
	"frame_images": {types: onlyVideo}, "supported_durations": {types: onlyVideo},
	"generate_audio":     {types: onlyVideo},
	"upscale_factor_min": {types: onlyImage}, "upscale_factor_max": {types: onlyImage},
	"supported_voices": {types: onlySpeech},

	"weights_size_gb": {family: familyWeightFiles},

	// One quantized build, and the GGUF file inside it.
	"build_bits_per_weight": {family: familyBuild}, "build_format": {family: familyBuild},
	"build_revision": {family: familyBuild}, "build_files": {family: familyBuild},
	"build_size_bytes":  {family: familyBuild},
	"gguf_architecture": {family: familyGGUF},
}

// prefixRules cover the groups of columns the refresh makes from feed paths,
// whose members come and go with the feed.
var prefixRules = []struct {
	prefix string
	rule   fieldRule
}{
	{"default_parameters_", fieldRule{types: chatLike}},
	{"supported_parameters_", fieldRule{listing: true, types: generators}},
	{"pricing_", fieldRule{listing: true}},
	{"benchmarks_", fieldRule{listing: true}},
}

// ruleFor is the rule for one column. A column made from the feed that nothing
// above describes is about OpenRouter's listing, so it needs one.
func ruleFor(key string, madeFromFeed bool) fieldRule {
	if rule, ok := fieldRules[key]; ok {
		return rule
	}
	for _, p := range prefixRules {
		if strings.HasPrefix(key, p.prefix) {
			return p.rule
		}
	}
	if madeFromFeed {
		return fieldRule{listing: true}
	}
	return fieldRule{}
}

// RowProfile is what a row states about itself that decides which columns can
// apply to it.
type RowProfile struct {
	// Listed: OpenRouter lists this model.
	Listed bool
	// ModelType is the row's own model_type, empty when nothing states one, in
	// which case no column is held back for its type.
	ModelType string
	// Build: the row is one quantized build filed under a model.
	Build bool

	holdsBuild, holdsGGUF bool
}

// ProfileOf reads a row's profile from its identity and its merged cells.
func ProfileOf(identity ModelIdentity, cells map[string]string) RowProfile {
	p := RowProfile{
		Listed:    identity.Source == "openrouter" || cells["id"] != "",
		ModelType: strings.ToLower(strings.TrimSpace(cells["model_type"])),
		Build:     identity.VariantOf != "",
	}
	for key, value := range cells {
		if value == "" {
			continue
		}
		switch ruleFor(key, false).family {
		case familyBuild:
			p.holdsBuild = true
			p.holdsGGUF = true
		case familyGGUF:
			p.holdsGGUF = true
		}
	}
	return p
}

// IsGGUFRepository reports a main row that is a GGUF repository: it holds GGUF
// facts and is not a build filed under a model.
func (p RowProfile) IsGGUFRepository() bool { return p.holdsGGUF && !p.Build }

// Applies reports whether a column can describe this row at all. madeFromFeed
// says the column is one the refresh made from a feed path.
func (p RowProfile) Applies(key string, madeFromFeed bool) bool {
	rule := ruleFor(key, madeFromFeed)
	if rule.listing && !p.Listed {
		return false
	}
	if len(rule.types) > 0 && p.ModelType != "" && !containsWord(rule.types, p.ModelType) {
		return false
	}
	switch rule.family {
	case familyBuild:
		return p.Build || p.holdsBuild
	case familyGGUF:
		return p.Build || p.holdsGGUF
	case familyWeightFiles:
		return !p.IsGGUFRepository()
	}
	return true
}

func containsWord(list []string, want string) bool {
	for _, w := range list {
		if w == want {
			return true
		}
	}
	return false
}
