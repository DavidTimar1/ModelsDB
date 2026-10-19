package store

import (
	"reflect"
	"testing"
)

func TestHasErrorPrice(t *testing.T) {
	if !hasErrorPrice(map[string]interface{}{"prompt": -1}) {
		t.Errorf("negative price should be an error price")
	}
	if !hasErrorPrice(map[string]interface{}{"image": -1000000}) {
		t.Errorf("negative sentinel should be an error price")
	}
	if hasErrorPrice(map[string]interface{}{"prompt": "0.5", "completion": "1"}) {
		t.Errorf("positive prices should not be error prices")
	}
	if hasErrorPrice(map[string]interface{}{"prompt": nil}) {
		t.Errorf("nil price should be ignored")
	}
}

func TestToStrList(t *testing.T) {
	if got := toStrList([]interface{}{"a", 1}); !reflect.DeepEqual(got, []string{"a", "1"}) {
		t.Errorf("toStrList([]interface{}) = %v", got)
	}
	if got := toStrList([]string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("toStrList([]string) = %v", got)
	}
	if got := toStrList(123); got != nil {
		t.Errorf("toStrList(unsupported) = %v, want nil", got)
	}
}

func TestPricingOf(t *testing.T) {
	norm := map[string]interface{}{"endpoint": map[string]interface{}{
		"pricing": map[string]interface{}{"prompt": "0.1"},
	}}
	if got := pricingOf(norm); got["prompt"] != "0.1" {
		t.Errorf("pricingOf should return the nested pricing map, got %v", got)
	}
	if got := pricingOf(map[string]interface{}{}); len(got) != 0 {
		t.Errorf("pricingOf without endpoint should be empty, got %v", got)
	}
}

func TestPriceStr(t *testing.T) {
	pr := map[string]interface{}{"prompt": 0.5, "nil": nil}
	if priceStr(pr, "prompt") != "0.5" {
		t.Errorf("priceStr present")
	}
	if priceStr(pr, "nil") != "" {
		t.Errorf("priceStr nil should be ''")
	}
	if priceStr(pr, "absent") != "" {
		t.Errorf("priceStr absent should be ''")
	}
}

// TestDeriveType covers the modality mapping and, for audio, the one case
// modality cannot decide: music against speech. The two descriptions below are
// the real ones OpenRouter serves for Google's Lyria 3 Pro (music) and OpenAI's
// GPT Audio (speech), and the two models carry IDENTICAL output modalities - so
// the pair is the whole reason the description is read at all.
func TestDeriveType(t *testing.T) {
	const lyriaDesc = "Full-length songs are priced at $0.08 per song. Lyria 3 is Google's " +
		"family of music generation models, available through the Gemini API."
	const gptAudioDesc = "The gpt-audio model is OpenAI's first generally available audio model. " +
		"The new snapshot features an upgraded decoder for more natural sounding voices."

	cases := []struct {
		what string
		out  []string
		desc string
		want string
	}{
		{"image output", []string{"image"}, "", "image"},
		{"video output", []string{"video"}, "", "video"},
		{"audio output, nothing said", []string{"audio"}, "", "tts"},
		{"text output", []string{"text"}, "", "chat"},
		{"no output at all", nil, "", "chat"},

		// OpenRouter's own spellings for the media that are not text, image,
		// video or audio.
		{"embeddings output", []string{"embeddings"}, "", "embedding"},
		{"rerank output", []string{"rerank"}, "", "rerank"},
		{"speech output", []string{"speech"}, "", "tts"},
		{"transcription output", []string{"transcription"}, "", "stt"},
		// A HuggingFace import's spellings of two of them.
		{"embedding output", []string{"embedding"}, "", "embedding"},
		{"ranking output", []string{"ranking"}, "", "rerank"},
		{"spelling is case blind", []string{"Embeddings"}, "", "embedding"},
		// Speech is not audio, so a description about music cannot re-label it.
		{"speech that talks about music", []string{"speech"}, lyriaDesc, "tts"},
		{"an embedding model that talks about music", []string{"embeddings"}, lyriaDesc, "embedding"},

		{"a music model", []string{"text", "audio"}, lyriaDesc, "music"},
		{"a speech model", []string{"text", "audio"}, gptAudioDesc, "tts"},

		// The music answer is reachable only through the audio branch, so a
		// description about music on a model that outputs something else cannot
		// re-label it.
		{"an image model that talks about music", []string{"image"}, lyriaDesc, "image"},
		{"a chat model that talks about music", []string{"text"}, lyriaDesc, "chat"},

		// A speech model that merely MENTIONS the word music stays speech.
		{"speech mentioning music in passing", []string{"audio"},
			"A text-to-speech voice model. It is not intended for music.", "tts"},
	}
	for _, c := range cases {
		if got := deriveType(c.out, c.desc); got != c.want {
			t.Errorf("%s: deriveType(%v, %.40q...) = %q, want %q", c.what, c.out, c.desc, got, c.want)
		}
	}
}
