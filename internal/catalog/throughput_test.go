package catalog

import (
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// readStatsFixture loads a recorded stats answer from testdata.
func readStatsFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// The recorded GLM answer holds endpoints at 137, 133, 204, 60.5 and 101 tokens
// per second and one with no stats at all: the speed is the highest, 204.
func TestFastestThroughputTakesTheHighestP50(t *testing.T) {
	got, err := fastestThroughput(readStatsFixture(t, "stats_endpoint_glm_standard.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != 204 {
		t.Errorf("fastestThroughput = %d, want 204", got)
	}
}

// An answer whose endpoints carry no stats leaves the model without a figure.
func TestFastestThroughputOfUnmeasuredEndpointsIsZero(t *testing.T) {
	got, err := fastestThroughput(readStatsFixture(t, "stats_endpoint_embedding_free.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("fastestThroughput = %d, want 0", got)
	}
}

func TestFastestThroughputReadsEveryShapeOfMissingStats(t *testing.T) {
	for name, tc := range map[string]struct {
		body    string
		want    int
		wantErr bool
	}{
		"null stats are skipped":             {`{"data":[{"stats":null},{"stats":{"p50_throughput":80}}]}`, 80, false},
		"null p50 is skipped":                {`{"data":[{"stats":{"p50_throughput":null}},{"stats":{"p50_throughput":12}}]}`, 12, false},
		"all null leaves nothing":            {`{"data":[{"stats":null},{"stats":{"p50_throughput":null}},{}]}`, 0, false},
		"no endpoints leaves nothing":        {`{"data":[]}`, 0, false},
		"a fraction rounds to a whole":       {`{"data":[{"stats":{"p50_throughput":60.5}}]}`, 61, false},
		"zero is no figure":                  {`{"data":[{"stats":{"p50_throughput":0}}]}`, 0, false},
		"a negative figure is no figure":     {`{"data":[{"stats":{"p50_throughput":-4}}]}`, 0, false},
		"no data list is a changed feed":     {`{"error":"gone"}`, 0, true},
		"a null data list is a changed feed": {`{"data":null}`, 0, true},
		"not JSON":                           {`<html>`, 0, true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := fastestThroughput([]byte(tc.body))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error: %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("fastestThroughput = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestVariantOfIDIsTheSuffixOrStandard(t *testing.T) {
	for id, want := range map[string]string{
		"z-ai/glm-5.2":                    "standard",
		"deepseek/deepseek-r1:free":       "free",
		"google/gemini-embedding-2:batch": "batch",
		"vendor/model:":                   "standard",
	} {
		if got := variantOfID(id); got != want {
			t.Errorf("variantOfID(%q) = %q, want %q", id, got, want)
		}
	}
}

// The permaslug is the canonical slug (the slug cell) and the variant comes
// from the id; both stay inside their own query value.
func TestStatsEndpointURLKeepsValuesInsideTheirParameters(t *testing.T) {
	raw := statsEndpointURL("z-ai/glm-5.2-20260616&variant=evil#x", "free")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("permaslug") != "z-ai/glm-5.2-20260616&variant=evil#x" || q.Get("variant") != "free" {
		t.Errorf("query = %v, want the slug intact and the variant free", q)
	}
	if q.Get("latencyMetric") != "latency" || q.Get("perfWorkload") != "text_generation" {
		t.Errorf("query = %v, want latency and text_generation", q)
	}
}

func TestThroughputJobsUseTheSlugAndTheVariantOfTheID(t *testing.T) {
	data := []map[string]interface{}{
		{"id": "z-ai/glm-5.2", "canonical_slug": "z-ai/glm-5.2-20260616"},
		{"id": "a/b:free", "canonical_slug": "a/b-20260101"},
		{"id": "c/d"}, // no canonical slug: the id stands in, as NormalizeModel stores it
		{"id": "not/stored"},
		{"id": "z-ai/glm-5.2", "canonical_slug": "z-ai/glm-5.2-20260616"}, // listed twice: asked once
	}
	idToName := map[string]string{"z-ai/glm-5.2": "GLM 5.2", "a/b:free": "B (free)", "c/d": "D"}
	want := []throughputJob{
		{"GLM 5.2", "z-ai/glm-5.2-20260616", "standard"},
		{"B (free)", "a/b-20260101", "free"},
		{"D", "c/d", "standard"},
	}
	got := throughputJobs(data, idToName)
	if len(got) != len(want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("job %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// DeriveThroughput writes the fastest speed, clears the cell of a model nobody
// measures any more, and leaves a model whose call failed exactly as it was.
func TestDeriveThroughputWritesClearsAndFailsSoft(t *testing.T) {
	catalogTestDB(t)
	for _, m := range []map[string]interface{}{
		{"name": "Measured", "id": "v/measured", "canonical_slug": "v/measured-1"},
		{"name": "Dropped", "id": "v/dropped", "canonical_slug": "v/dropped-1"},
		{"name": "Failing", "id": "v/failing", "canonical_slug": "v/failing-1"},
	} {
		if _, err := store.UpsertAPIModel(NormalizeModel(m)); err != nil {
			t.Fatal(err)
		}
	}
	store.RecordThroughput("Dropped", 50)
	store.RecordThroughput("Failing", 70)

	answers := map[string]string{
		"v/measured-1": string(readStatsFixture(t, "stats_endpoint_glm_standard.json")),
		"v/dropped-1":  string(readStatsFixture(t, "stats_endpoint_embedding_free.json")),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := answers[r.URL.Query().Get("permaslug")]
		if !ok {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	orig := StatsEndpointBase
	StatsEndpointBase = server.URL
	t.Cleanup(func() { StatsEndpointBase = orig })

	data := []map[string]interface{}{
		{"id": "v/measured", "canonical_slug": "v/measured-1"},
		{"id": "v/dropped", "canonical_slug": "v/dropped-1"},
		{"id": "v/failing", "canonical_slug": "v/failing-1"},
	}
	DeriveThroughput(data, map[string]string{"v/measured": "Measured", "v/dropped": "Dropped", "v/failing": "Failing"}, nil)

	for name, want := range map[string]string{"Measured": "204", "Dropped": "", "Failing": "70"} {
		cells, err := store.ModelCells(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := cells["throughput"]; got != want {
			t.Errorf("%s throughput = %q, want %q", name, got, want)
		}
	}
}
