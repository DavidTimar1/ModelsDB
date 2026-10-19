package dbcore

import "testing"

// TestDescribesMusicGeneration pins both directions of the one test that
// separates a music model from a speech model, since modality cannot. The
// negatives are the ones that matter: a false positive re-labels a speech model
// and no refresh ever puts it back.
func TestDescribesMusicGeneration(t *testing.T) {
	yes := []string{
		"Lyria 3 is Google's family of music generation models, available through the Gemini API.",
		"A TEXT-TO-MUSIC model.",
		"Give it a prompt and it will generate music in any style.",
		"Song generation from a lyric sheet.",
	}
	for _, d := range yes {
		if !DescribesMusicGeneration(d) {
			t.Errorf("DescribesMusicGeneration(%.60q) = false, want true", d)
		}
	}

	no := []string{
		"",
		"The gpt-audio model is OpenAI's first generally available audio model, with more natural sounding voices.",
		"A text-to-speech model. It is not intended for music.",
		"Transcribes speech recorded over background music.",
		"A chat model that can discuss music theory.",
	}
	for _, d := range no {
		if DescribesMusicGeneration(d) {
			t.Errorf("DescribesMusicGeneration(%.60q) = true, want false", d)
		}
	}
}
