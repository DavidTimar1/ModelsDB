package catalog

import (
	"net/http"
	"strings"
	"testing"
)

// The repository every test below reads, and the two figures that make the size
// question real: what its shard index names, against what every weight file in
// it adds up to. The gap is a second copy of the same weights, which is what a
// real repository ships (mistralai/Mistral-7B-Instruct-v0.3's
// consolidated.safetensors, openai/gpt-oss-20b's original/ and metal/ folders).
const (
	modelRepo     = "vendor/model-7b"
	shardIndexSum = 14_496_080_928
	everyFileSum  = 28_992_161_856
)

// modelRepoServer stands in for huggingface.co with one full, unsharded-index
// repository. Every test here is offline.
func modelRepoServer(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/"+modelRepo, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("blobs") != "true" {
			t.Errorf("the listing must ask for blobs=true, or no file carries a size; got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"` + modelRepo + `","author":"vendor","sha":"5eaf00d","gated":false,
			"createdAt":"2026-01-04T09:00:00.000Z","pipeline_tag":"text-generation",
			"tags":["safetensors","text-generation"],
			"safetensors":{"parameters":{"BF16":7248023552},"total":7248023552},
			"siblings":[{"rfilename":"model-00001-of-00002.safetensors","size":9000000000},
			            {"rfilename":"model-00002-of-00002.safetensors","size":5496080928},
			            {"rfilename":"consolidated.safetensors","size":14496080928},
			            {"rfilename":"model.safetensors.index.json","size":25000},
			            {"rfilename":"config.json","size":700}]}`))
	})
	mux.HandleFunc("/"+modelRepo+"/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"metadata":{"total_size":14496047104},
			"weight_map":{"model.embed_tokens.weight":"model-00001-of-00002.safetensors",
			              "model.layers.0.self_attn.q_proj.weight":"model-00001-of-00002.safetensors",
			              "lm_head.weight":"model-00002-of-00002.safetensors"}}`))
	})
	mux.HandleFunc("/"+modelRepo+"/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"architectures":["VendorForCausalLM"],"max_position_embeddings":32768,"num_local_experts":8}`))
	})
	hfServer(t, mux)
}

// TestNormalizeHFRepoTakesWhatAPersonPastes pins the addresses this has to
// accept, and the ones it must refuse rather than read as a repository. A
// /datasets/ address is the dangerous one: it has the same two-segment shape as a
// model's, so without a rule it would be read as the repository "datasets/squad".
func TestNormalizeHFRepoTakesWhatAPersonPastes(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://huggingface.co/Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
		{"https://huggingface.co/Qwen/Qwen3-32B/", "Qwen/Qwen3-32B"},
		{"https://huggingface.co/Qwen/Qwen3-32B/tree/main", "Qwen/Qwen3-32B"},
		{"https://huggingface.co/Qwen/Qwen3-32B?library=transformers", "Qwen/Qwen3-32B"},
		{"https://huggingface.co/Qwen/Qwen3-32B#files", "Qwen/Qwen3-32B"},
		{"http://huggingface.co/Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
		{"https://www.huggingface.co/Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
		{"https://hf.co/Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
		{"huggingface.co/Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
		{"https://huggingface.co/models/Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
		{"  Qwen/Qwen3-32B  ", "Qwen/Qwen3-32B"},
		{"Qwen/Qwen3-32B", "Qwen/Qwen3-32B"},
	} {
		got, err := NormalizeHFRepo(c.in)
		if err != nil {
			t.Errorf("NormalizeHFRepo(%q) refused it: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeHFRepo(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, in := range []string{
		"",
		"   ",
		"Qwen",
		"https://huggingface.co/",
		"https://huggingface.co/datasets/vendor/squad",
		"https://huggingface.co/spaces/vendor/demo",
		"https://example.test/Qwen/Qwen3-32B",
		"https://github.com/Qwen/Qwen3-32B",
	} {
		if got, err := NormalizeHFRepo(in); err == nil {
			t.Errorf("NormalizeHFRepo(%q) = %q, want a refusal", in, got)
		}
	}

	// The repository id is case-sensitive on HuggingFace, so nothing here may
	// lower-case it on the way through.
	if got, _ := NormalizeHFRepo("https://huggingface.co/MistralAI/Mistral-7B"); got != "MistralAI/Mistral-7B" {
		t.Errorf("NormalizeHFRepo lower-cased the repository id: %q", got)
	}
}

// TestModelSizeIsTheShardIndexNotEveryFile is the measurement test. The
// repository ships a second copy of its own weights, so summing every weight
// file gives twice the truth - a figure that looks entirely plausible in a
// column this app presents as a measurement, which is why the index is read
// rather than the file list.
func TestModelSizeIsTheShardIndexNotEveryFile(t *testing.T) {
	modelRepoServer(t)

	m, err := HFModelRecord(modelRepo)
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.WeightBytes != shardIndexSum {
		t.Errorf("weight bytes = %d, want %d - the two shards the index names", m.WeightBytes, shardIndexSum)
	}
	if m.WeightBytes == everyFileSum {
		t.Errorf("weight bytes = %d, which is every weight file summed; the repository's second copy of its own weights must not be counted", m.WeightBytes)
	}
	if got, want := m.DiskSizeGB(), float64(shardIndexSum)/1e9; got != want {
		t.Errorf("DiskSizeGB = %v, want %v", got, want)
	}
	if !strings.Contains(m.WeightBasis, "model.safetensors.index.json") {
		t.Errorf("the size basis reads %q, want it to name where the size came from", m.WeightBasis)
	}
}

// TestModelRecordReadsTheRestOfTheRepository covers what else one read fills in,
// so the route above it stores facts rather than blanks.
func TestModelRecordReadsTheRestOfTheRepository(t *testing.T) {
	modelRepoServer(t)

	m, err := HFModelRecord(modelRepo)
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.Author != "vendor" || m.Revision != "5eaf00d" {
		t.Errorf("author %q at revision %q, want vendor at 5eaf00d", m.Author, m.Revision)
	}
	if m.ContextLength != 32768 {
		t.Errorf("context length = %d, want the config's max_position_embeddings", m.ContextLength)
	}
	if m.ModelType != "chat" {
		t.Errorf("model type = %q, want chat from the text-generation task", m.ModelType)
	}
	if strings.Join(m.InputModalities, ",") != "text" || strings.Join(m.OutputModalities, ",") != "text" {
		t.Errorf("modalities are %v -> %v, want text -> text", m.InputModalities, m.OutputModalities)
	}
	if m.MoE != "yes" {
		t.Errorf("moe = %q, want yes from the config's expert count", m.MoE)
	}
	if m.Architecture != "VendorForCausalLM" {
		t.Errorf("architecture = %q", m.Architecture)
	}
	if m.CreatedAt != "2026-01-04T09:00:00.000Z" {
		t.Errorf("created at %q, want what the repository reports", m.CreatedAt)
	}
	if m.Parameters < 7.2 || m.Parameters > 7.3 {
		t.Errorf("parameters = %v billion, want about 7.25", m.Parameters)
	}
	if !strings.Contains(m.Description, modelRepo) {
		t.Errorf("description = %q, want it to name the repository", m.Description)
	}
}

// TestUnshardedRepositoryTakesItsSingleWeightFile pins the second shape: a
// repository with no index at all holds one model.safetensors, and the copies
// beside it - an ONNX or OpenVINO export, an old pytorch dump - are not part of
// the weight set.
func TestUnshardedRepositoryTakesItsSingleWeightFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/small", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/small","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"sentence-similarity",
			"safetensors":{"parameters":{"F32":22713216,"I64":512},"total":22713728},
			"siblings":[{"rfilename":"model.safetensors","size":90868376},
			            {"rfilename":"pytorch_model.bin","size":90888945},
			            {"rfilename":"openvino/openvino_model.bin","size":90883512}]}`))
	})
	mux.HandleFunc("/vendor/small/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	mux.HandleFunc("/vendor/small/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"architectures":["BertModel"],"max_position_embeddings":512}`))
	})
	hfServer(t, mux)

	m, err := HFModelRecord("vendor/small")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.WeightBytes != 90868376 {
		t.Errorf("weight bytes = %d, want the single model.safetensors and nothing beside it", m.WeightBytes)
	}
	if m.ModelType != "embedding" {
		t.Errorf("model type = %q, want embedding from the sentence-similarity task", m.ModelType)
	}
	// No count, and the SIZE of the model is not the reason - 0.0227 billion is a
	// figure the column stores perfectly well. The block carries an I64 buffer
	// beside the F32 weights, and I64 is not one of the storage types that holds
	// exactly one parameter per element, so the total cannot be read as a count of
	// parameters at all.
	if m.Parameters != 0 {
		t.Errorf("parameters = %v, want none: an I64 buffer in the block means its total is not a parameter count", m.Parameters)
	}
	if m.MoE != "no" {
		t.Errorf("moe = %q, want no - a config was read and it names no experts", m.MoE)
	}
}

// TestPackedRepositoryStatesNoParameterCount pins the trap the skill records:
// the safetensors block counts STORED ELEMENTS, and a packed 4-bit repository
// puts eight weights in each 32-bit element. Reading its total as a parameter
// count turns a 32-billion-parameter model into a 5-billion one, so a repository
// storing anything below full precision yields no count at all.
func TestPackedRepositoryStatesNoParameterCount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/model-4bit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/model-4bit","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"text-generation",
			"safetensors":{"parameters":{"U32":4095180800,"BF16":1024472064},"total":5119652864},
			"siblings":[{"rfilename":"model.safetensors","size":18429850874}]}`))
	})
	mux.HandleFunc("/vendor/model-4bit/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	mux.HandleFunc("/vendor/model-4bit/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	hfServer(t, mux)

	m, err := HFModelRecord("vendor/model-4bit")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.Parameters != 0 {
		t.Errorf("parameters = %v billion, want none: 5.1 billion packed elements are not 5.1 billion parameters", m.Parameters)
	}
	if m.WeightBytes != 18429850874 {
		t.Errorf("weight bytes = %d, want the one weight file's size", m.WeightBytes)
	}
	// No config anywhere means unknown, which is not the same as "no".
	if m.MoE != "" {
		t.Errorf("moe = %q, want it unset: no config could be read, so nothing is known", m.MoE)
	}
	if m.ContextLength != 0 {
		t.Errorf("context length = %d, want none: no config could be read", m.ContextLength)
	}
}

// TestASubBillionCountIsRead pins the other half of the parameter rule: the
// count is discarded when the repository's own block cannot be READ as a count of
// parameters, and never merely because the model is small. A 0.6-billion model is
// a real model, and the column holds decimals - so 0.6 is recorded, rather than
// left empty for somebody to research a number the repository already stated.
func TestASubBillionCountIsRead(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/tiny", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/tiny","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"text-generation",
			"safetensors":{"parameters":{"BF16":596049920},"total":596049920},
			"siblings":[{"rfilename":"model.safetensors","size":1192099840}]}`))
	})
	for _, p := range []string{"/vendor/tiny/resolve/main/model.safetensors.index.json", "/vendor/tiny/resolve/main/config.json"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Entry not found", http.StatusNotFound)
		})
	}
	hfServer(t, mux)

	m, err := HFModelRecord("vendor/tiny")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.Parameters != 0.59604992 {
		t.Errorf("parameters = %v, want 0.59604992 billion - a small model still has a count", m.Parameters)
	}
}

// TestAnLFSPointerLeavesTheSizeUnset pins the silent-wrong-answer case. A shard
// index served as a Git LFS pointer parses as nothing, and the tempting fallback
// - sum every weight file - is exactly the doubled figure this whole file exists
// to avoid. No size is the right answer.
func TestAnLFSPointerLeavesTheSizeUnset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/lfs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/lfs","author":"vendor","sha":"abc","gated":false,
			"siblings":[{"rfilename":"model-00001-of-00002.safetensors","size":9000000000},
			            {"rfilename":"model-00002-of-00002.safetensors","size":5496080928},
			            {"rfilename":"consolidated.safetensors","size":14496080928}]}`))
	})
	mux.HandleFunc("/vendor/lfs/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("version https://git-lfs.github.com/spec/v1\noid sha256:aeb1\nsize 25000\n"))
	})
	mux.HandleFunc("/vendor/lfs/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	hfServer(t, mux)

	m, err := HFModelRecord("vendor/lfs")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if m.WeightBytes != 0 {
		t.Errorf("weight bytes = %d, want none: the index could not be read, and summing every file would be %d",
			m.WeightBytes, everyFileSum)
	}
}

// TestGatedRepositoryStopsAfterItsMetadata pins what a gated repository does
// here: its metadata answers in full, its files refuse, and nothing pretends to
// have read them.
func TestGatedRepositoryStopsAfterItsMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/closed/model", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"closed/model","author":"closed","sha":"abc","gated":"manual",
			"pipeline_tag":"text-generation",
			"siblings":[{"rfilename":"model.safetensors","size":14000000000}]}`))
	})
	mux.HandleFunc("/closed/model/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, r *http.Request) {
		t.Error("a gated repository's files must not be fetched; the metadata already says they cannot be read")
	})
	hfServer(t, mux)

	m, err := HFModelRecord("closed/model")
	if err != nil {
		t.Fatalf("HFModelRecord: %v", err)
	}
	if !m.Gated {
		t.Fatal("the repository is gated and the record does not say so")
	}
	if m.WeightBytes != 0 || m.ContextLength != 0 {
		t.Errorf("a gated repository reported %d bytes and a %d context; nothing there could have been read",
			m.WeightBytes, m.ContextLength)
	}
}

// TestUnreadableRepositorySaysWhichWay pins the two answers a person meets, both
// as a sentence rather than a status code. HuggingFace answers 401 for a
// repository that needs an account, for a private one AND for one that does not
// exist, so the message says all three rather than picking one.
func TestUnreadableRepositorySaysWhichWay(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/gone/model", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	mux.HandleFunc("/api/models/private/model", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	hfServer(t, mux)

	if _, err := HFModelRecord("gone/model"); err == nil || !strings.Contains(err.Error(), "no HuggingFace repository") {
		t.Errorf("a missing repository said %v", err)
	}
	if _, err := HFModelRecord("private/model"); err == nil || !strings.Contains(err.Error(), "needs an account") {
		t.Errorf("an unreadable repository said %v", err)
	}
}

// TestCapabilitiesComeFromTheTask pins the mapping from HuggingFace's own task
// tags to this catalog's model_type and modality lists. The tag ids are
// HuggingFace's, and a task not in the table leaves everything unset rather than
// inventing a capability.
func TestCapabilitiesComeFromTheTask(t *testing.T) {
	for _, c := range []struct {
		tag, modelType, in, out string
	}{
		{"text-generation", "chat", "text", "text"},
		{"image-text-to-text", "chat", "text,image", "text"},
		{"text-to-image", "image", "text", "image"},
		{"text-to-video", "video", "text", "video"},
		{"automatic-speech-recognition", "stt", "audio", "text"},
		{"text-to-speech", "tts", "text", "audio"},
		{"text-to-audio", "tts", "text", "audio"},
		{"feature-extraction", "embedding", "text", "embedding"},
		{"text-ranking", "rerank", "text", "ranking"},
		{"object-detection", "", "", ""},
		{"", "", "", ""},
	} {
		modelType, in, out := capabilitiesOf(c.tag, "", nil, nil)
		if modelType != c.modelType || strings.Join(in, ",") != c.in || strings.Join(out, ",") != c.out {
			t.Errorf("capabilitiesOf(%q) = %q %v -> %v, want %q %q -> %q",
				c.tag, modelType, in, out, c.modelType, c.in, c.out)
		}
	}

	// A config carrying a vision_config takes images whatever its task says, so a
	// vision model filed under plain text-generation still records that it sees.
	_, in, _ := capabilitiesOf("text-generation", "", map[string]interface{}{
		"vision_config": map[string]interface{}{"hidden_size": float64(1024)},
	}, nil)
	if strings.Join(in, ",") != "text,image" {
		t.Errorf("input modalities = %v, want image added from the config's vision_config", in)
	}
}

// TestContextLengthComesFromEitherLevelOfTheConfig pins the nesting trap: on a
// multimodal model the language-model fields sit inside text_config and there is
// nothing at the top level, so a reader that looks only at the top concludes the
// field is missing when it is one level down.
func TestContextLengthComesFromEitherLevelOfTheConfig(t *testing.T) {
	top := map[string]interface{}{"max_position_embeddings": float64(8192)}
	if got := contextLengthOf(top, nil); got != 8192 {
		t.Errorf("context length from the top level = %d, want 8192", got)
	}
	nested := map[string]interface{}{
		"text_config": map[string]interface{}{"max_position_embeddings": float64(262144)},
	}
	if got := contextLengthOf(nested, nil); got != 262144 {
		t.Errorf("context length from text_config = %d, want 262144", got)
	}
	// A GGUF repository has no config at all and states its own context length.
	if got := contextLengthOf(nil, map[string]string{"context_length": "40960"}); got != 40960 {
		t.Errorf("context length from the gguf block = %d, want 40960", got)
	}
	if got := contextLengthOf(nil, nil); got != 0 {
		t.Errorf("context length with nothing to read = %d, want none", got)
	}
}

// TestAudioModelIsMusicWhenItsCardSaysItMakesMusic pins the split the task tag
// cannot make: HuggingFace files a music generator and a speech model under the
// same task, so the model card's opening decides. A card with no README, and a
// speech card that rules music generation out only far below its opening, both
// stay speech.
func TestAudioModelIsMusicWhenItsCardSaysItMakesMusic(t *testing.T) {
	cards := map[string]string{
		"vendor/song-model":    "---\nlicense: mit\n---\n# Song model\n\nAn open music generation model that writes songs from a prompt.\n",
		"vendor/voice-model":   "---\nlicense: mit\n---\n# Voice model\n\nA text-to-speech model with six voices.\n",
		"vendor/careful-tts":   "# Careful TTS\n\nA speech model.\n" + strings.Repeat("Notes on voices. ", 400) + "\nNot intended for music generation.\n",
		"vendor/no-card-tts":   "",
		"vendor/music-in-yaml": "---\ntags: [music generation]\n---\n# Voice\n\nA speech model.\n",
	}
	mux := http.NewServeMux()
	for repo, card := range cards {
		repo, card := repo, card
		mux.HandleFunc("/api/models/"+repo, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"` + repo + `","author":"vendor","sha":"abc","gated":false,"pipeline_tag":"text-to-audio","siblings":[]}`))
		})
		if card != "" {
			mux.HandleFunc("/"+repo+"/resolve/main/README.md", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(card))
			})
		}
	}
	hfServer(t, mux)

	for repo, want := range map[string]string{
		"vendor/song-model":    "music",
		"vendor/voice-model":   "tts",
		"vendor/careful-tts":   "tts",
		"vendor/no-card-tts":   "tts",
		"vendor/music-in-yaml": "tts",
	} {
		m, err := HFModelRecord(repo)
		if err != nil {
			t.Fatalf("%s: %v", repo, err)
		}
		if m.ModelType != want {
			t.Errorf("%s: model_type = %q, want %q", repo, m.ModelType, want)
		}
		if got := strings.Join(m.OutputModalities, ","); got != "audio" {
			t.Errorf("%s: output modalities = %q, want audio whichever type it got", repo, got)
		}
	}
}
