package dbcore

import "strings"

// musicGenerationPhrases are the phrases that mark a model as one whose audio
// output is MUSIC rather than speech. Every one of them names the act of making
// music, so it can only appear in the description of a model that does it -
// which is what keeps the test conservative. A bare mention of the word "music"
// is deliberately NOT in the list: a speech model's description can name music
// in passing ("not intended for music", "over background music"), and a wrong
// answer here re-labels a speech model on every install that ever reads it.
//
// The list is matched case-insensitively against the whole description.
var musicGenerationPhrases = []string{
	"music generation",
	"music generator",
	"music-generation",
	"text-to-music",
	"text to music",
	"generate music",
	"generates music",
	"generating music",
	"song generation",
	"generate songs",
	"generates songs",
	"generating songs",
}

// DescribesMusicGeneration reports whether a model description says the model
// makes music.
//
// It exists because MODALITY CANNOT ANSWER THE QUESTION. A music generator and
// a speech model are the same shape upstream - both take text and answer with
// audio - so the two are indistinguishable until something reads what the model
// says it produces. Google's Lyria and OpenAI's GPT Audio are the pair that
// proves it: identical input and output modality lists, and nothing but the
// description between them.
//
// It lives in this package rather than beside store.deriveType because the
// migration that corrects rows already stored under the wrong type applies the
// SAME test, and dbcore sits below store in the import DAG. One rule, one list,
// two callers, no chance of the two drifting apart.
func DescribesMusicGeneration(description string) bool {
	d := strings.ToLower(description)
	for _, p := range musicGenerationPhrases {
		if strings.Contains(d, p) {
			return true
		}
	}
	return false
}
