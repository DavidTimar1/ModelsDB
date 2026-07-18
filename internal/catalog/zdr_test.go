package catalog

import "testing"

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
