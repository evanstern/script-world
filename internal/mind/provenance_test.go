package mind

// Spec 030 (US1) tests: the consolidation belief-evidence contract (T003) and
// the deterministic provenance enforcement (T004). Model-free — enforcement
// makes no model calls; it coerces, never rejects.

import (
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/sim"
)

// TestConsolidationPromptCitesEvidence (T003): the user prompt instructs the
// model to cite evidence and reserve "witnessed" for direct perception, and the
// belief schema carries the evidence field.
func TestConsolidationPromptCitesEvidence(t *testing.T) {
	job := consolJob{
		name:   "Ash",
		anchor: persona.Anchors["Ash"],
		buffer: []sim.Memory{{Text: "Saw a wolf.", Salience: 7, Tick: 100, Subject: -1, Origin: sim.OriginAction}},
	}
	p := consolidateUserPrompt(job)
	for _, want := range []string{`"evidence"`, "witnessed", "directly", "told"} {
		if !strings.Contains(p, want) {
			t.Errorf("consolidation prompt missing %q:\n%s", want, p)
		}
	}
}

// TestConsolidationParsesEvidence (T003): parseConsolidation accepts the
// evidence ordinals into beliefChange.Evidence; a reply with no evidence field
// parses to a nil slice (old-shaped output stays legal).
func TestConsolidationParsesEvidence(t *testing.T) {
	withEvidence := `{"nature":"n","gist":"g","promote":[],"fade":[],
	 "beliefs":[{"id":0,"statement":"s","confidence":50,"provenance":"witnessed","source":-1,"subject":-1,"evidence":["m1","m3"]}],
	 "narrative":"nn"}`
	out, err := parseConsolidation(withEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Beliefs[0].Evidence; len(got) != 2 || got[0] != "m1" || got[1] != "m3" {
		t.Errorf("parsed evidence = %v, want [m1 m3]", got)
	}

	noEvidence := `{"nature":"n","gist":"g","promote":[],"fade":[],
	 "beliefs":[{"id":0,"statement":"s","confidence":50,"provenance":"told","source":-1,"subject":-1}],
	 "narrative":"nn"}`
	out, err = parseConsolidation(noEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if out.Beliefs[0].Evidence != nil {
		t.Errorf("old-shaped output should parse to nil evidence, got %v", out.Beliefs[0].Evidence)
	}
}
