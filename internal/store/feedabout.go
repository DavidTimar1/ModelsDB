// What each column made from the OpenRouter feed is ABOUT.
//
// A column the refresh makes for a feed path nothing else reads is created
// from the path alone, so it has a heading and no description. Anyone asked to
// fill such a column (the research brief, the details dialog) needs to know what
// the cell holds, so the known paths are described here, and a path this table
// has not met yet gets a sentence built from the path itself rather than none.

package store

// feedColumnAbout describes the columns the refresh has made from feed paths,
// by column key. The wording follows what the feed carries at the path, read from
// the sample values the refresh records.
var feedColumnAbout = map[string]string{
	"alias_target_name":                                 "The display name of the model this listing is an alias of, where it is one.",
	"allowed_passthrough_parameters":                    "Request parameters beyond the standard ones that OpenRouter passes through to the provider, such as prompt_enhancement.",
	"architecture_modality":                             "The input and output media as one OpenRouter string, e.g. text+image->text.",
	"benchmarks_artificial_analysis_agentic_index":      "Artificial Analysis's agentic score for the model, as OpenRouter lists it.",
	"benchmarks_artificial_analysis_coding_index":       "Artificial Analysis's coding score for the model, as OpenRouter lists it.",
	"benchmarks_artificial_analysis_intelligence_index": "Artificial Analysis's overall intelligence score for the model, as OpenRouter lists it.",
	"benchmarks_design_arena":                           "The model's Design Arena leaderboard rows (arena, category, rank, Elo, win rate), as OpenRouter lists them.",
	"creativity":                                        "The range of creativity settings the listing offers, as a list of numbers.",
	"default_parameters_frequency_penalty":              "The frequency penalty OpenRouter applies to this model when a request sets none.",
	"default_parameters_repetition_penalty":             "The repetition penalty OpenRouter applies to this model when a request sets none.",
	"default_parameters_temperature":                    "The sampling temperature OpenRouter applies to this model when a request sets none.",
	"default_parameters_top_k":                          "The top-k cutoff OpenRouter applies to this model when a request sets none.",
	"default_parameters_top_p":                          "The top-p cutoff OpenRouter applies to this model when a request sets none.",
	"endpoints":                                         "The OpenRouter API path that lists the providers serving this model.",
	"generate_audio":                                    "Whether a video model also produces an audio track.",
	"links_details":                                     "The OpenRouter API path of this model's endpoint details.",
	"pricing_image_token":                               "What one image token costs, in USD, where the model prices images by token.",
	"pricing_overrides":                                 "Price changes that apply in particular windows, as OpenRouter lists them.",
	"pricing_skus":                                      "The model's price list by billable unit, as OpenRouter lists it.",
	"reasoning_default_effort":                          "The reasoning effort the model uses when a request names none: low, medium or high.",
	"reasoning_default_enabled":                         "Whether the model reasons by default, when a request does not say.",
	"reasoning_supported_efforts":                       "The reasoning effort levels the model accepts, as a list.",
	"reasoning_supports_max_tokens":                     "Whether a request can cap the tokens the model spends reasoning.",
	"seed":                                              "Whether the model takes a seed to make its output repeatable.",
	"supported_aspect_ratios":                           "The aspect ratios an image or video model produces, as a list such as 16:9.",
	"supported_durations":                               "The clip lengths in seconds a video model produces, as a list.",
	"supported_parameters_aspect_ratio_type":            "Whether the aspect ratio request parameter takes one of a fixed set (enum) or any value.",
	"supported_parameters_aspect_ratio_values":          "The aspect ratios the aspect ratio request parameter accepts.",
	"supported_parameters_background_type":              "Whether the background request parameter takes one of a fixed set (enum) or any value.",
	"supported_parameters_background_values":            "The values the background request parameter accepts: auto, transparent, opaque.",
	"supported_parameters_input_references_min":         "The fewest reference images a request must carry.",
	"supported_parameters_input_references_type":        "Whether the reference image count is a range or a fixed set.",
	"supported_parameters_n_max":                        "The most images one request can ask for.",
	"supported_parameters_n_min":                        "The fewest images one request can ask for.",
	"supported_parameters_n_type":                       "Whether the image count request parameter is a range or a fixed set.",
	"supported_parameters_output_compression_max":       "The highest output compression level a request can set.",
	"supported_parameters_output_compression_min":       "The lowest output compression level a request can set.",
	"supported_parameters_output_compression_type":      "Whether the output compression request parameter is a range or a fixed set.",
	"supported_parameters_output_format_type":           "Whether the output format request parameter takes one of a fixed set (enum) or any value.",
	"supported_parameters_output_format_values":         "The output file formats a request can ask for, such as png or webp.",
	"supported_parameters_quality_type":                 "Whether the quality request parameter takes one of a fixed set (enum) or any value.",
	"supported_parameters_quality_values":               "The quality levels a request can ask for.",
	"supported_parameters_resolution_type":              "Whether the resolution request parameter takes one of a fixed set (enum) or any value.",
	"supported_parameters_resolution_values":            "The resolutions a request can ask for, such as 1K, 2K, 4K.",
	"supported_parameters_seed_type":                    "What kind of value the seed request parameter takes.",
	"supported_parameters_size_type":                    "What kind of value the size request parameter takes.",
	"supported_resolutions":                             "The output resolutions a video model produces, as a list such as 720p.",
	"supported_sizes":                                   "The pixel sizes an image or video model produces, as a list such as 1024x1024.",
	"supported_voices":                                  "The voices a speech model offers, as a list of names.",
	"supports_streaming":                                "Whether the model can stream its answer.",
	"top_provider_context_length":                       "The context length of the provider OpenRouter ranks first for this model.",
	"upscale_factor_max":                                "The largest factor an upscaling model enlarges an image by.",
	"upscale_factor_min":                                "The smallest factor an upscaling model enlarges an image by.",
}

// feedFieldAbout describes the column made for one feed path. A path the table
// above does not describe gets a sentence that names the path, so no column made
// from the feed is ever offered without a description.
func feedFieldAbout(key, path string) string {
	if about := feedColumnAbout[key]; about != "" {
		return about
	}
	if path == "" {
		return "A field of OpenRouter's feed for this model, as the listing states it."
	}
	return "The field " + path + " of OpenRouter's feed for this model, as the listing states it."
}
