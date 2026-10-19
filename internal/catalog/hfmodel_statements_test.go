package catalog

import (
	"net/http"
	"strings"
	"testing"
)

// TestAnUnknownTaskStillRecordsWhatTheConfigStates: a decision model is filed
// under text-classification, which the task table does not know. What its config
// states must survive that, and the transformers task must be used when the
// pipeline tag is not one the table knows.
func TestAnUnknownTaskStillRecordsWhatTheConfigStates(t *testing.T) {
	vision := map[string]interface{}{
		"text_config":   map[string]interface{}{"hidden_size": float64(2816)},
		"vision_config": map[string]interface{}{"hidden_size": float64(1152)},
	}
	modelType, in, out := capabilitiesOf("text-classification", "", vision, nil)
	if modelType != "" || strings.Join(in, ",") != "text,image" || len(out) != 0 {
		t.Errorf("unknown task with a vision_config = %q %v -> %v, want no type, text and image in, nothing out", modelType, in, out)
	}

	modelType, in, out = capabilitiesOf("text-classification", "image-text-to-text", nil, nil)
	if modelType != "chat" || strings.Join(in, ",") != "text,image" || strings.Join(out, ",") != "text" {
		t.Errorf("unknown pipeline tag with a known transformers task = %q %v -> %v, want chat, text and image in, text out", modelType, in, out)
	}

	// A config that says nothing about modalities records nothing under an
	// unknown task: "text" alone would be a guess from the shape of the config.
	modelType, in, out = capabilitiesOf("text-classification", "", map[string]interface{}{
		"text_config": map[string]interface{}{"hidden_size": float64(768)},
	}, nil)
	if modelType != "" || in != nil || out != nil {
		t.Errorf("unknown task with a plain text config = %q %v -> %v, want nothing", modelType, in, out)
	}
}

// TestAudioAndVideoInputsComeFromTheConfigAndTheTags: embeddinggemma states
// text, image, video and audio, and a config carrying an audio_config, a video
// token id and tags naming them says so.
func TestAudioAndVideoInputsComeFromTheConfigAndTheTags(t *testing.T) {
	config := map[string]interface{}{
		"vision_config": map[string]interface{}{},
		"audio_config":  map[string]interface{}{},
		"text_config":   map[string]interface{}{"video_token_id": float64(262144)},
	}
	_, in, out := capabilitiesOf("feature-extraction", "", config, nil)
	if strings.Join(in, ",") != "text,image,audio,video" || strings.Join(out, ",") != "embedding" {
		t.Errorf("embedding model with audio and video in its config = %v -> %v, want text,image,audio,video -> embedding", in, out)
	}

	_, in, _ = capabilitiesOf("feature-extraction", "", nil, []string{"sentence-transformers", "Audio", "video"})
	if strings.Join(in, ",") != "text,audio,video" {
		t.Errorf("tags naming audio and video = %v, want text,audio,video", in)
	}

	// A speech model's audio_config describes what it MAKES, so it is not an
	// input, and a tag naming audio on it says the same thing.
	_, in, _ = capabilitiesOf("text-to-speech", "", map[string]interface{}{"audio_config": map[string]interface{}{}}, []string{"audio"})
	if strings.Join(in, ",") != "text" {
		t.Errorf("a speech model's inputs = %v, want text alone", in)
	}
}

// TestAGGUFRepositoryStatesItsParametersAndArchitecture: the gguf block's total
// is a count of parameters, and its architecture and the card's base model are
// recorded on a model import, not only on a build.
func TestAGGUFRepositoryStatesItsParametersAndArchitecture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/model-gguf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/model-gguf","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"text-to-image",
			"tags":["gguf","base_model:vendor/model-base","base_model:quantized:vendor/model-base"],
			"cardData":{"base_model":"vendor/some-other-name"},
			"gguf":{"total":7115124736,"architecture":"qwen_image21","context_length":4096,"totalFileSize":4000000000},
			"siblings":[{"rfilename":"model-Q4_K_M.gguf","size":4000000000}]}`))
	})
	hfServer(t, mux)

	m, err := HFModelRecord("vendor/model-gguf")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.Parameters != 7.115124736 {
		t.Errorf("parameters = %v, want 7.115124736 from gguf.total", m.Parameters)
	}
	if m.GGUFArchitecture != "qwen_image21" {
		t.Errorf("gguf architecture = %q, want qwen_image21", m.GGUFArchitecture)
	}
	if m.BaseModel != "vendor/model-base" {
		t.Errorf("base model = %q, want the plain base_model: tag's repository", m.BaseModel)
	}
}

// TestTheBaseModelComesFromTheCardWhenNoTagNamesIt pins the fallback and the
// shape of it: a card may name one repository or a list.
func TestTheBaseModelComesFromTheCardWhenNoTagNamesIt(t *testing.T) {
	for _, c := range []struct {
		what string
		tags []string
		card map[string]interface{}
		want string
	}{
		{"a tag wins over the card", []string{"base_model:a/b"}, map[string]interface{}{"base_model": "c/d"}, "a/b"},
		{"the qualified twin is not the address", []string{"base_model:quantized:a/b"}, nil, ""},
		{"a card string", nil, map[string]interface{}{"base_model": " c/d "}, "c/d"},
		{"a card list", nil, map[string]interface{}{"base_model": []interface{}{"c/d", "e/f"}}, "c/d, e/f"},
		{"nothing stated", nil, map[string]interface{}{"license": "mit"}, ""},
	} {
		if got := baseModelOf(c.tags, c.card); got != c.want {
			t.Errorf("%s: baseModelOf = %q, want %q", c.what, got, c.want)
		}
	}
}

// TestAnEmbeddingModelsWindowIsItsSentenceTransformerLength: the position table
// of the language model under an embedding model is 32 times longer than the
// text the embedding model is built to read.
func TestAnEmbeddingModelsWindowIsItsSentenceTransformerLength(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/embed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/embed","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"sentence-similarity","siblings":[{"rfilename":"model.safetensors","size":1000}]}`))
	})
	mux.HandleFunc("/vendor/embed/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"architectures":["VendorModel"],"max_position_embeddings":262144}`))
	})
	mux.HandleFunc("/vendor/embed/resolve/main/sentence_bert_config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"max_seq_length":8192,"do_lower_case":false}`))
	})
	mux.HandleFunc("/api/models/vendor/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/chat","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"text-generation","siblings":[{"rfilename":"model.safetensors","size":1000}]}`))
	})
	mux.HandleFunc("/vendor/chat/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"architectures":["VendorModel"],"max_position_embeddings":262144}`))
	})
	mux.HandleFunc("/vendor/chat/resolve/main/sentence_bert_config.json", func(w http.ResponseWriter, r *http.Request) {
		t.Error("only an embedding model has its sentence-transformer config read")
	})
	hfServer(t, mux)

	embed, err := HFModelRecord("vendor/embed")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if embed.ContextLength != 8192 {
		t.Errorf("embedding context length = %d, want 8192 from sentence_bert_config.json, not the 262144 position table", embed.ContextLength)
	}
	chat, err := HFModelRecord("vendor/chat")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if chat.ContextLength != 262144 {
		t.Errorf("chat context length = %d, want 262144 from the config", chat.ContextLength)
	}
}
