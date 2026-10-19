package catalog

import "testing"

// An id from the feed is escaped part by part, so it cannot change the path or
// add a query to the address it ends up in.
func TestModelEndpointsURLKeepsAnIdInsideItsSegments(t *testing.T) {
	for id, want := range map[string]string{
		"openai/gpt-5":      "https://openrouter.ai/api/v1/models/openai/gpt-5/endpoints",
		"a/b:free":          "https://openrouter.ai/api/v1/models/a/b:free/endpoints",
		"x/y?z=1#frag":      "https://openrouter.ai/api/v1/models/x/y%3Fz=1%23frag/endpoints",
		"x/../../admin":     "https://openrouter.ai/api/v1/models/x/%2E%2E/%2E%2E/admin/endpoints",
		"vendor/model name": "https://openrouter.ai/api/v1/models/vendor/model%20name/endpoints",
	} {
		if got := modelEndpointsURL(id); got != want {
			t.Errorf("modelEndpointsURL(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestIsZDR(t *testing.T) {
	policies := map[string]bool{
		"openai":        true,  // retains prompts -> not ZDR on its own
		"together":      false, // retains nothing -> ZDR
		"google-vertex": true,
	}
	tests := []struct {
		name      string
		providers []string
		want      bool
	}{
		{"any non-retaining provider -> ZDR", []string{"openai", "together"}, true},
		{"all retaining -> not ZDR", []string{"openai", "google-vertex"}, false},
		{"unknown provider is treated as retaining", []string{"mystery-host"}, false},
		{"no providers -> not ZDR", nil, false},
		{"single non-retaining -> ZDR", []string{"together"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isZDR(tt.providers, policies); got != tt.want {
				t.Errorf("isZDR(%v) = %v, want %v", tt.providers, got, tt.want)
			}
		})
	}
}
