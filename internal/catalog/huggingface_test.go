package catalog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hfServer stands in for huggingface.co. Every test in this file is fully
// offline: the two package-level base URLs are pointed here and restored
// afterwards, so nothing reaches the real API.
func hfServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(mux)
	origAPI, origResolve := HFAPIBaseURL, HFResolveBaseURL
	HFAPIBaseURL = server.URL + "/api"
	HFResolveBaseURL = server.URL
	t.Cleanup(func() {
		HFAPIBaseURL = origAPI
		HFResolveBaseURL = origResolve
		server.Close()
	})
	return server
}

// TestQuantLabelIsReadFromTheEndOfTheName pins the rule that keeps a model name
// from being mistaken for a quantization label: the label is the LAST part of the
// filename that looks like one, so a name carrying its own numbers is safe.
func TestQuantLabelIsReadFromTheEndOfTheName(t *testing.T) {
	for _, c := range []struct {
		file, label string
		bit         int
	}{
		{"Qwen3-32B-Q4_K_M.gguf", "Q4_K_M", 4},
		{"Qwen3-32B-IQ2_M.gguf", "IQ2_M", 2},
		{"Qwen3-32B-Q8_0.gguf", "Q8_0", 8},
		{"BF16/Qwen3-32B-BF16-00001-of-00002.gguf", "BF16", 16},
		{"Qwen3-32B-F32.gguf", "F32", 32},
		{"gpt-oss-20b-MXFP4.gguf", "MXFP4", 4},
		// No label at all: the file is still real, and is offered under its own
		// name rather than dropped.
		{"model.gguf", "", 0},
	} {
		if got := quantLabelOf(c.file); got != c.label {
			t.Errorf("quantLabelOf(%q) = %q, want %q", c.file, got, c.label)
		}
		if got := bitsOfLabel(c.label); got != c.bit {
			t.Errorf("bitsOfLabel(%q) = %d, want %d", c.label, got, c.bit)
		}
	}
}

// TestGGUFBuildsGroupShardsAndMarkProjectors pins the shape of a GGUF
// repository: one build per quantization, a build split over shards counted ONCE
// with its sizes summed, and the mmproj files kept but marked as projectors -
// they are loaded alongside a quantization rather than being one, so counting
// them as builds would overstate what the repository offers.
func TestGGUFBuildsGroupShardsAndMarkProjectors(t *testing.T) {
	builds := buildsFrom([]hfSibling{
		{Path: ".gitattributes", Size: 3205},
		{Path: "README.md", Size: 900},
		{Path: "Qwen3-32B-Q4_K_M.gguf", Size: 20_000_000_000},
		{Path: "BF16/Qwen3-32B-BF16-00001-of-00002.gguf", Size: 49_871_764_512},
		{Path: "BF16/Qwen3-32B-BF16-00002-of-00002.gguf", Size: 15_659_811_424},
		{Path: "mmproj-F16.gguf", Size: 1_400_000_000},
	}, []string{"gguf"})

	if len(builds) != 3 {
		t.Fatalf("got %d builds, want 3 (Q4_K_M, BF16, and the projector): %+v", len(builds), builds)
	}
	byLabel := map[string]HFBuild{}
	for _, b := range builds {
		byLabel[b.Label] = b
	}

	bf16, ok := byLabel["BF16"]
	if !ok {
		t.Fatal("the two BF16 shards did not group into one build")
	}
	if len(bf16.Files) != 2 {
		t.Errorf("the BF16 build holds %d files, want both shards", len(bf16.Files))
	}
	if want := int64(49_871_764_512 + 15_659_811_424); bf16.Bytes != want {
		t.Errorf("the BF16 build is %d bytes, want the shards summed (%d)", bf16.Bytes, want)
	}
	if bf16.Bit != 16 {
		t.Errorf("the BF16 build reports %d bits, want 16", bf16.Bit)
	}

	q4 := byLabel["Q4_K_M"]
	if q4.Bit != 4 || q4.Format != "GGUF" || q4.Bytes != 20_000_000_000 {
		t.Errorf("the Q4_K_M build is %+v, want a 4-bit GGUF of 20000000000 bytes", q4)
	}

	proj, ok := byLabel["F16"]
	if !ok {
		t.Fatalf("the mmproj file is missing from %+v", builds)
	}
	if !proj.Projector {
		t.Error("the mmproj file must be marked as a projector, not offered as a quantization")
	}
	if proj.Bit != 0 {
		t.Errorf("a projector reports %d bits; it is not a quantization, so it states none", proj.Bit)
	}
	// Real quantizations sort ahead of projectors, so the list opens with what a
	// person actually came for.
	if builds[len(builds)-1].Label != "F16" {
		t.Errorf("the projector should sort last, got %+v", builds)
	}
}

// TestNonGGUFRepositoryOffersOneBuild pins the other shape: a repository holding
// one build spread over shards offers exactly one entry, summing its weight
// files and taking its format and bit width from the repository's own tags.
func TestNonGGUFRepositoryOffersOneBuild(t *testing.T) {
	builds := buildsFrom([]hfSibling{
		{Path: "config.json", Size: 800},
		{Path: "tokenizer.json", Size: 11_422_654},
		{Path: "model-00001-of-00002.safetensors", Size: 9_000_000_000},
		{Path: "model-00002-of-00002.safetensors", Size: 8_000_000_000},
	}, []string{"safetensors", "awq", "4-bit", "text-generation"})

	if len(builds) != 1 {
		t.Fatalf("got %d builds, want 1: %+v", len(builds), builds)
	}
	b := builds[0]
	if b.Format != "AWQ" || b.Bit != 4 {
		t.Errorf("build is %s at %d bits, want AWQ at 4 (from the repository's tags)", b.Format, b.Bit)
	}
	if b.Bytes != 17_000_000_000 {
		t.Errorf("build is %d bytes, want the two shards summed and the tokenizer left out", b.Bytes)
	}
	if b.ID != "" {
		t.Errorf("a whole-repository build has the empty id, got %q", b.ID)
	}
}

// TestFormatIsReadFromTheFilesWhenTheTagsAreSilent pins the rule that keeps a
// build's format honest: a repository whose tags name no format is described by
// the weight files it actually holds. Both cases here are real repositories -
// coqui/XTTS-v2 publishes .pth files and nothing else, and IndexTeam/IndexTTS-2
// mixes .pth and .pt with one nested safetensors - and both used to be recorded
// as safetensors builds, a fact their own file lists contradict.
func TestFormatIsReadFromTheFilesWhenTheTagsAreSilent(t *testing.T) {
	pytorch := buildsFrom([]hfSibling{
		{Path: "config.json", Size: 800},
		{Path: "model.pth", Size: 1_800_000_000},
		{Path: "dvae.pth", Size: 250_000_000},
	}, []string{"coqui", "text-to-speech"})
	if len(pytorch) != 1 {
		t.Fatalf("got %d builds, want 1: %+v", len(pytorch), pytorch)
	}
	if pytorch[0].Format != "PyTorch" || pytorch[0].Label != "PyTorch" {
		t.Errorf("a repository of .pth files is %+v, want a PyTorch build - it holds no safetensors at all", pytorch[0])
	}

	mixed := buildsFrom([]hfSibling{
		{Path: "gpt.pth", Size: 3_000_000_000},
		{Path: "feat1.pt", Size: 40_000_000},
		{Path: "qwen0.6bemo4-merge/model.safetensors", Size: 1_200_000_000},
	}, []string{"safetensors", "text-to-speech"})
	if len(mixed) != 1 {
		t.Fatalf("got %d builds, want 1: %+v", len(mixed), mixed)
	}
	if mixed[0].Format != "safetensors + PyTorch" {
		t.Errorf("a repository of both kinds is %q, want both named - the build is every one of those files", mixed[0].Format)
	}
}

// TestNoWeightFilesMeansNoBuilds pins that a repository with nothing to download
// offers nothing, rather than an entry of zero bytes.
func TestNoWeightFilesMeansNoBuilds(t *testing.T) {
	if builds := buildsFrom([]hfSibling{{Path: "README.md", Size: 10}}, []string{"gguf"}); len(builds) != 0 {
		t.Errorf("got %+v, want no builds at all", builds)
	}
}

// TestGatedIsReadFromEitherShape pins the field that decides whether a build can
// be downloaded at all. HuggingFace answers a plain false on an open repository
// and a STRING on a gated one, so a bool-only reading would miss exactly the
// repositories this has to catch.
func TestGatedIsReadFromEitherShape(t *testing.T) {
	for _, c := range []struct {
		in   interface{}
		want bool
	}{
		{false, false},
		{true, true},
		{"manual", true},
		{"auto", true},
		{"", false},
		{nil, false},
	} {
		if got := isGated(c.in); got != c.want {
			t.Errorf("isGated(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestNextPageURLReadsTheLinkHeader pins the pagination the listing needs: the
// address of the next page arrives in a Link header, and a header naming any
// other relationship is not a next page.
func TestNextPageURLReadsTheLinkHeader(t *testing.T) {
	h := http.Header{}
	h.Add("Link", `<https://example.test/page2>; rel="next"`)
	if got := nextPageURL(h); got != "https://example.test/page2" {
		t.Errorf("nextPageURL = %q", got)
	}

	h = http.Header{}
	h.Add("Link", `<https://example.test/page1>; rel="prev"`)
	if got := nextPageURL(h); got != "" {
		t.Errorf("nextPageURL on a prev-only header = %q, want empty", got)
	}
	if got := nextPageURL(http.Header{}); got != "" {
		t.Errorf("nextPageURL on no header = %q, want empty", got)
	}
}

// TestQuantizedReposFollowsPagesAndReportsTheCap drives the listing end to end
// against a local server that keeps offering another page. The result must carry
// every repository read AND say that it stopped short, because a capped list
// presented as complete would have the user conclude a build does not exist.
func TestQuantizedReposFollowsPagesAndReportsTheCap(t *testing.T) {
	mux := http.NewServeMux()
	page := 0
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, "http://"+r.Host+"/api/models?cursor="+fmt.Sprint(page)))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"id":"owner%d/repo","likes":3,"downloads":40,"gated":false,"tags":["gguf"]}]`, page)
	})
	hfServer(t, mux)

	repos, truncated, err := HFQuantizedRepos("Qwen/Qwen3-32B")
	if err != nil {
		t.Fatalf("HFQuantizedRepos: %v", err)
	}
	if len(repos) != hfMaxRepoPages {
		t.Errorf("read %d repositories, want one per page up to the cap (%d)", len(repos), hfMaxRepoPages)
	}
	if !truncated {
		t.Error("more pages existed, so the result must say it was cut short")
	}
	if repos[0].Format != "GGUF" {
		t.Errorf("format from tags = %q, want GGUF", repos[0].Format)
	}
}

// TestRepoBuildsReadsSizesAndTheRevision drives one repository end to end.
func TestRepoBuildsReadsSizesAndTheRevision(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/owner/repo-GGUF", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("blobs") != "true" {
			t.Errorf("the listing must ask for blobs=true, or no file carries a size; got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"owner/repo-GGUF","author":"owner","sha":"abc123","gated":false,
			"tags":["gguf","base_model:vendor/base","base_model:quantized:vendor/base"],
			"gguf":{"architecture":"qwen3","context_length":40960,"total":32762123264,"chat_template":"a very long template"},
			"siblings":[{"rfilename":"repo-Q4_K_M.gguf","size":19762138368},
			            {"rfilename":"README.md","size":900}]}`))
	})
	hfServer(t, mux)

	detail, err := HFRepoBuilds("owner/repo-GGUF")
	if err != nil {
		t.Fatalf("HFRepoBuilds: %v", err)
	}
	if detail.Revision != "abc123" {
		t.Errorf("revision = %q, want the commit main pointed at", detail.Revision)
	}
	if detail.BaseModel != "vendor/base" {
		t.Errorf("base_model = %q, want the plain tag rather than the qualified one", detail.BaseModel)
	}
	if _, unwanted := detail.GGUF["chat_template"]; unwanted {
		t.Error("the chat template is kilobytes of template text and must not be copied out of the gguf block")
	}
	if detail.GGUF["architecture"] != "qwen3" || detail.GGUF["context_length"] != "40960" {
		t.Errorf("the gguf facts are %v, want the small scalars copied", detail.GGUF)
	}
	if len(detail.Builds) != 1 || detail.Builds[0].Bytes != 19762138368 {
		t.Fatalf("builds = %+v, want the one quantization with its own size", detail.Builds)
	}
	if _, ok := detail.FindBuild("repo-Q4_K_M.gguf"); !ok {
		t.Error("the build must be findable by the id it was listed under")
	}
	if _, ok := detail.FindBuild("nothing-like-this.gguf"); ok {
		t.Error("an unknown build id must not resolve")
	}
}

// TestRepoBuildsSaysWhatWentWrong pins the two failures a user meets: a
// repository that is not there, and one that needs an account. Each has to come
// back as a sentence rather than as a status code.
func TestRepoBuildsSaysWhatWentWrong(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/gone/repo", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	mux.HandleFunc("/api/models/closed/repo", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	hfServer(t, mux)

	if _, err := HFRepoBuilds("gone/repo"); err == nil || !strings.Contains(err.Error(), "no HuggingFace repository") {
		t.Errorf("a missing repository said %v", err)
	}
	if _, err := HFRepoBuilds("closed/repo"); err == nil || !strings.Contains(err.Error(), "needs an account") {
		t.Errorf("a repository needing an account said %v", err)
	}
}

// TestConfigRefusesAnLFSPointer pins the silent-wrong-answer case. A file stored
// in Git LFS can come back as a three-line pointer with status 200, and writing
// that into the database as an architecture would be a fact nobody could later
// tell was wrong.
func TestConfigRefusesAnLFSPointer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/owner/lfs/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("version https://git-lfs.github.com/spec/v1\noid sha256:aeb1\nsize 11422654\n"))
	})
	hfServer(t, mux)

	cfg, err := HFConfig("owner/lfs")
	if cfg != nil {
		t.Error("an LFS pointer must never be stored as a config")
	}
	if err == nil || !strings.Contains(err.Error(), "Git LFS") {
		t.Errorf("the error said %v, want it to name what came back instead", err)
	}
}
