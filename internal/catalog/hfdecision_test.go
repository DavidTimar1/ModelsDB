package catalog

import (
	"net/http"
	"strings"
	"testing"
)

// TestTheDecisionPhraseTestMatchesTheCardsAndNotAChatCard pins the wording the
// detection must catch (both autotrust cards and the reader's own example) and
// the ordinary card text it must leave alone, including wordings that describe a
// benchmark, a classifier or a reward model rather than the model's own output.
func TestTheDecisionPhraseTestMatchesTheCardsAndNotAChatCard(t *testing.T) {
	for _, c := range []struct {
		what, card string
		want       bool
	}{
		{"JEV card", "System 1 | typed decisions | a calibrated probability for every option, in one forward pass", true},
		{"GEV card", "Their output: a probability for every action out. The logits are softmaxed.", true},
		{"his example", "Output: one logit per allowed option for each question", true},
		{"a statement after a benchmark sentence", "We evaluate on MMLU. The model returns a probability for every option.", true},

		{"chat model evaluation, score per option", "For MMLU we compute the log-likelihood score for each option and pick the highest.", false},
		{"multiple-choice harness", "A multiple-choice evaluation: the harness takes the logit for each choice and picks the argmax.", false},
		{"scoring logits over choices", "Instruct model; evaluated by scoring the logits over the choices A, B, C, D.", false},
		{"text classifier", "The classifier returns a probability for each action label.", false},
		{"reward model", "Our reward model assigns a score for every option shown in the arena comparison.", false},
		{"ranker", "The ranker gives a score for all options in the list.", false},
		{"reinforcement-learning policy", "The policy outputs probabilities over actions for the RL agent.", false},
		{"zero-shot scorer", "Zero-shot: the model emits a score per option.", false},
		{"benchmark sentence with the card's own wording", "On the harness, a probability for every option is compared with the argmax.", false},
		{"a chat card", "Qwen is a chat model. It generates text and was trained on 36 trillion tokens. The probability of each token is sampled with temperature 0.7. Benchmarks report a score for each task.", false},
		{"an empty card", "", false},
	} {
		if got := isDecisionModel(nil, c.card); got != c.want {
			t.Errorf("%s: isDecisionModel = %v, want %v", c.what, got, c.want)
		}
	}
	if !isDecisionModel([]string{"safetensors", "Typed-Decisions"}, "an ordinary card") {
		t.Error("a decision tag must be enough, whatever the card says")
	}
	if isDecisionModel([]string{"text-generation", "conversational"}, "an ordinary card") {
		t.Error("ordinary tags must not make a decision model")
	}
}

// TestADecisionRepositoryIsTypedDecisionsWithTextForTheTwoModes: the output of a
// repository whose task out holds text is decisions and text, and one that
// outputs an image stays an image.
func TestADecisionRepositoryIsTypedDecisionsWithTextForTheTwoModes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/vendor/judge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/judge","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"text-classification","transformersInfo":{"pipeline_tag":"image-text-to-text"},
			"tags":["decision-model"],"siblings":[]}`))
	})
	mux.HandleFunc("/api/models/vendor/picture", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"vendor/picture","author":"vendor","sha":"abc","gated":false,
			"pipeline_tag":"text-to-image","tags":["gguf"],"siblings":[]}`))
	})
	mux.HandleFunc("/vendor/picture/resolve/main/README.md", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("A diffusion model. It draws pictures."))
	})
	hfServer(t, mux)

	judge, err := HFModelRecord("vendor/judge")
	if err != nil {
		t.Fatal(err)
	}
	if judge.ModelType != "decisions" || strings.Join(judge.OutputModalities, ",") != "decisions,text" {
		t.Errorf("decision repository = %q -> %v, want decisions -> decisions,text", judge.ModelType, judge.OutputModalities)
	}
	if strings.Join(judge.InputModalities, ",") != "text,image" {
		t.Errorf("decision repository inputs = %v, want text,image", judge.InputModalities)
	}
	pic, err := HFModelRecord("vendor/picture")
	if err != nil {
		t.Fatal(err)
	}
	if pic.ModelType != "image" || strings.Join(pic.OutputModalities, ",") != "image" {
		t.Errorf("image repository = %q -> %v, want image", pic.ModelType, pic.OutputModalities)
	}
}

// TestACardStatesItsWindowInTokens: the wording of embeddinggemma-2's card, in
// both of its forms, and a card that states none.
func TestACardStatesItsWindowInTokens(t *testing.T) {
	for _, c := range []struct {
		card string
		want int
	}{
		{`| **Context Window** | 8,192 tokens | | **Native Output Dimension** | 768 |`, 8192},
		{"using a single shared 8,192-token context window.", 8192},
		{"It was trained on 2 trillion tokens.", 0},
	} {
		if got := cardContextWindow(c.card); got != c.want {
			t.Errorf("cardContextWindow(%.40q) = %d, want %d", c.card, got, c.want)
		}
	}
}
