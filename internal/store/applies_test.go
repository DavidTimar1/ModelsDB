package store

import (
	"strings"
	"testing"
)

// profile builds a row profile without a database: the rules read nothing else.
func profile(listed bool, modelType string, cells map[string]string) RowProfile {
	if cells == nil {
		cells = map[string]string{}
	}
	cells["model_type"] = modelType
	if listed {
		cells["id"] = "vendor/model"
	}
	return ProfileOf(ModelIdentity{Name: "x", Source: "manual"}, cells)
}

// TestWhichFieldsApplyToEachKindOfModel is the applicability rule, one case per
// model type: a field named in "applies" is offered on that kind of row, a field
// named in "foreign" is not.
func TestWhichFieldsApplyToEachKindOfModel(t *testing.T) {
	for _, c := range []struct {
		modelType       string
		applies         []string
		foreign         []string
		listedOnlyApply []string
	}{
		{typeChat,
			[]string{"supports_tools", "supports_reasoning", "default_parameters_temperature", "knowledge_cutoff", "parameters", "price_prompt"},
			[]string{"reference_images", "supported_voices", "frame_images", "upscale_factor_max", "build_files"},
			[]string{"tokenizer", "max_completion_tokens", "price_cache_read"}},
		{typeDecisions,
			[]string{"supports_tools", "reasoning_default_effort", "base_model"},
			[]string{"reference_images", "supported_voices"},
			[]string{"tokenizer"}},
		{typeImage,
			[]string{"reference_images", "max_reference_images", "supported_aspect_ratios", "supported_sizes", "upscale_factor_max", "parameters", "active_parameters"},
			[]string{"supports_tools", "supports_reasoning", "default_parameters_top_p", "supported_durations", "frame_images", "context_length", "moe", "supported_resolutions"},
			[]string{"price_image_output", "supported_parameters_quality_values"}},
		{typeVideo,
			[]string{"reference_images", "frame_images", "supported_durations", "generate_audio", "supported_resolutions"},
			[]string{"supports_tools", "upscale_factor_min", "supported_voices"},
			[]string{"supported_parameters_resolution_values"}},
		{typeEmbedding,
			[]string{"context_length", "parameters", "knowledge_cutoff"},
			[]string{"supports_tools", "supports_reasoning", "reference_images", "default_parameters_temperature"},
			nil},
		{typeRerank,
			[]string{"context_length"},
			[]string{"supports_tools", "reasoning_mandatory", "supported_sizes"},
			nil},
		{typeTTS,
			[]string{"supported_voices", "parameters"},
			[]string{"supports_tools", "reference_images", "supported_durations"},
			[]string{"price_audio_output"}},
		{typeSTT,
			[]string{"parameters"},
			[]string{"supports_tools", "supported_voices", "reference_images"},
			nil},
		{typeMusic,
			[]string{"parameters"},
			[]string{"supports_tools", "supported_voices"},
			[]string{"price_audio_output"}},
	} {
		unlisted := profile(false, c.modelType, nil)
		listed := profile(true, c.modelType, nil)
		for _, key := range c.applies {
			if !unlisted.Applies(key, false) {
				t.Errorf("%s: %s should apply to a row with no OpenRouter listing", c.modelType, key)
			}
		}
		for _, key := range c.foreign {
			if listed.Applies(key, false) {
				t.Errorf("%s: %s is foreign to this kind of model and must not be offered, even on a listed row", c.modelType, key)
			}
		}
		for _, key := range c.listedOnlyApply {
			if unlisted.Applies(key, false) {
				t.Errorf("%s: %s describes an OpenRouter listing and must not be offered without one", c.modelType, key)
			}
			if !listed.Applies(key, false) {
				t.Errorf("%s: %s should be offered on a listed row", c.modelType, key)
			}
		}
	}
}

// TestOpenRouterOnlyFieldsNeedAListing: every field that describes the listing
// is held back from a repository-only row, and a feed column nothing describes
// counts as one of them.
func TestOpenRouterOnlyFieldsNeedAListing(t *testing.T) {
	unlisted := profile(false, typeChat, nil)
	for _, key := range []string{"id", "slug", "vendor", "zdr", "moderated", "price_image", "alias_target", "endpoints", "pricing_skus", "benchmarks_design_arena", "supports_streaming"} {
		if unlisted.Applies(key, false) {
			t.Errorf("%s is offered on a row OpenRouter does not list", key)
		}
	}
	if unlisted.Applies("brand_new_feed_field", true) {
		t.Error("an undescribed feed column must be treated as OpenRouter-only")
	}
	if !unlisted.Applies("a_column_nobody_ruled_on", false) {
		t.Error("a column with no rule applies everywhere")
	}
	if !unlisted.Applies("price_prompt", false) || !unlisted.Applies("price_unit", false) {
		t.Error("the basic price columns stay offered for a model somebody runs themselves")
	}
}

// TestBuildAndGGUFFieldsNeedTheirFamily: a base repository is not asked for a
// build's files, and a GGUF repository is offered no weight size.
func TestBuildAndGGUFFieldsNeedTheirFamily(t *testing.T) {
	base := profile(false, typeChat, nil)
	for _, key := range []string{"build_files", "build_format", "gguf_architecture"} {
		if base.Applies(key, false) {
			t.Errorf("%s is offered on a base repository that holds nothing of its family", key)
		}
	}
	build := ProfileOf(ModelIdentity{Name: "b", VariantOf: "parent"}, map[string]string{"model_type": "chat"})
	for _, key := range []string{"build_files", "build_format", "gguf_architecture"} {
		if !build.Applies(key, false) {
			t.Errorf("%s should be offered on a build", key)
		}
	}
	gguf := profile(false, typeImage, map[string]string{"gguf_architecture": "qwen_image21"})
	if !gguf.IsGGUFRepository() {
		t.Error("a row stating a GGUF architecture is a GGUF repository")
	}
	if gguf.Applies("weights_size_gb", false) {
		t.Error("weights_size_gb is offered on a GGUF main row, where no file is the weights")
	}
	if !build.Applies("weights_size_gb", false) {
		t.Error("a build keeps its size fields")
	}
	if gguf.Applies("build_files", false) {
		t.Error("a GGUF main row is not a build")
	}
}

// TestAnEmptyModelTypeHoldsNothingBack: a row whose type nothing has stated is
// not narrowed by type, since the rule cannot know which side it falls on.
func TestAnEmptyModelTypeHoldsNothingBack(t *testing.T) {
	p := profile(false, "", nil)
	if !p.Applies("supports_tools", false) || !p.Applies("reference_images", false) {
		t.Error("with no model type stated, type rules must not exclude anything")
	}
}

// TestEveryFieldOfferedHasADescription: the brief offers a research column only
// with its description. This holds for every shipped column and for every column
// made from a feed path the app already knows, and a path it has not met gets a
// sentence naming the path.
func TestEveryFieldOfferedHasADescription(t *testing.T) {
	for _, c := range shippedColumns {
		if strings.TrimSpace(c.About) == "" {
			t.Errorf("shipped column %s has no description", c.Key)
		}
	}
	for key, about := range feedColumnAbout {
		if strings.TrimSpace(about) == "" {
			t.Errorf("feed column %s has an empty description", key)
		}
	}
	if got := feedFieldAbout("some_new_field", "some.new_field"); !strings.Contains(got, "some.new_field") {
		t.Errorf("an unknown feed path is described as %q, want a sentence naming the path", got)
	}
}
