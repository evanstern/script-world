package mind

import (
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
)

func validOutput() consolidationOutput {
	return consolidationOutput{
		Nature:    persona.Anchors["Ash"],
		Gist:      "A hard day, held together.",
		Promote:   []string{"m1"},
		Fade:      []string{"m2"},
		Beliefs:   []beliefChange{{ID: 0, Statement: "Wolves hunt the ridge.", Confidence: 70, Provenance: sim.ProvenanceWitnessed, Source: -1, Subject: -1}},
		Narrative: "I keep the village fed. Steady hands, steady fire.",
	}
}

func validBuffer() []sim.Memory {
	return []sim.Memory{
		{Text: "saw a wolf", Salience: 7, Tick: 100, Subject: -1},
		{Text: "ate berries", Salience: 1, Tick: 200, Subject: -1},
	}
}

// TestValidatorAcceptsClean: the happy path must pass all three layers.
func TestValidatorAcceptsClean(t *testing.T) {
	err := validateConsolidation(validOutput(), 0, validBuffer(), nil,
		persona.Anchors["Ash"], persona.DriftMarkers["Ash"])
	if err != nil {
		t.Fatalf("clean output rejected: %v", err)
	}
}

// TestValidatorStructuralTable: layer-1 violations, each with its stable
// reason.
func TestValidatorStructuralTable(t *testing.T) {
	held := []sim.Belief{{ID: 3, Statement: "old belief", Confidence: 50, Provenance: sim.ProvenanceInferred, Source: -1, Subject: -1}}
	cases := []struct {
		name   string
		mutate func(*consolidationOutput)
		reason string
	}{
		{"too many promotes", func(o *consolidationOutput) {
			o.Promote = make([]string, maxPromotes+1)
			for i := range o.Promote {
				o.Promote[i] = "m1"
			}
		}, "too_many_promotes"},
		{"unknown memory ref", func(o *consolidationOutput) {
			o.Fade = []string{"m99"}
		}, "unknown_memory_ref"},
		{"confidence out of range", func(o *consolidationOutput) {
			o.Beliefs[0].Confidence = 101
		}, "confidence_out_of_range"},
		{"bad provenance", func(o *consolidationOutput) {
			o.Beliefs[0].Provenance = "divined"
		}, "bad_provenance"},
		{"unknown belief id", func(o *consolidationOutput) {
			o.Beliefs[0].ID = 42
		}, "unknown_belief_id"},
		{"bad agent ref", func(o *consolidationOutput) {
			o.Beliefs[0].Subject = 99
		}, "bad_agent_ref"},
		{"empty gist", func(o *consolidationOutput) { o.Gist = "" }, "bad_gist"},
		{"oversized narrative", func(o *consolidationOutput) {
			o.Narrative = strings.Repeat("I remember. ", 200)
		}, "bad_narrative"},
	}
	for _, c := range cases {
		out := validOutput()
		c.mutate(&out)
		err := validateConsolidation(out, 0, validBuffer(), held,
			persona.Anchors["Ash"], persona.DriftMarkers["Ash"])
		if err == nil || err.Error() != c.reason {
			t.Errorf("%s: err = %v, want %s", c.name, err, c.reason)
		}
	}
}

// TestValidatorAnchorEcho: layer 2 — paraphrased nature is drift's canary.
func TestValidatorAnchorEcho(t *testing.T) {
	out := validOutput()
	out.Nature = "pretty calm and handy, doesn't anger fast" // paraphrase, not echo
	err := validateConsolidation(out, 0, validBuffer(), nil,
		persona.Anchors["Ash"], persona.DriftMarkers["Ash"])
	if err == nil || err.Error() != "anchor_mismatch" {
		t.Fatalf("err = %v, want anchor_mismatch", err)
	}
	// Leading/trailing whitespace alone is tolerated.
	out = validOutput()
	out.Nature = "  " + persona.Anchors["Ash"] + "\n"
	if err := validateConsolidation(out, 0, validBuffer(), nil,
		persona.Anchors["Ash"], persona.DriftMarkers["Ash"]); err != nil {
		t.Fatalf("trimmed echo rejected: %v", err)
	}
}

// TestValidatorDriftLexicon is SC-002: a deliberately temperament-drifting
// output is rejected 100% of the time — in the narrative or a self-belief;
// markers about OTHERS do not trigger it.
func TestValidatorDriftLexicon(t *testing.T) {
	// Every authored marker for every villager, in a narrative → rejected.
	for _, name := range sim.AgentNames {
		for _, marker := range persona.DriftMarkers[name] {
			out := validOutput()
			out.Nature = persona.Anchors[name]
			out.Narrative = "Something changed in me today. I have become " + marker + "."
			err := validateConsolidation(out, 0, validBuffer(), nil,
				persona.Anchors[name], persona.DriftMarkers[name])
			if err == nil || !strings.HasPrefix(err.Error(), "drift:") {
				t.Errorf("%s narrative with %q: err = %v, want drift rejection", name, marker, err)
			}
		}
	}

	// Drift in a SELF-belief → rejected.
	out := validOutput()
	out.Beliefs = []beliefChange{{ID: 0, Statement: "I am reckless now.", Confidence: 90, Provenance: sim.ProvenanceInferred, Source: -1, Subject: 0}}
	err := validateConsolidation(out, 0, validBuffer(), nil,
		persona.Anchors["Ash"], persona.DriftMarkers["Ash"])
	if err == nil || err.Error() != "drift:reckless" {
		t.Fatalf("self-belief drift: err = %v, want drift:reckless", err)
	}

	// The same word about ANOTHER villager is legitimate observation.
	out = validOutput()
	out.Beliefs = []beliefChange{{ID: 0, Statement: "Rowan is reckless.", Confidence: 80, Provenance: sim.ProvenanceWitnessed, Source: -1, Subject: 3}}
	if err := validateConsolidation(out, 0, validBuffer(), nil,
		persona.Anchors["Ash"], persona.DriftMarkers["Ash"]); err != nil {
		t.Fatalf("belief about another rejected: %v", err)
	}

	// Word-boundary: "recklessness" contains no standalone "reckless"... it
	// does at a boundary? \breckless\b matches inside "recklessness"? No —
	// the trailing 'n' is a word char, so \b fails. Substrings must NOT trip.
	out = validOutput()
	out.Narrative = "I weighed the recklessness of others and stayed steady."
	if err := validateConsolidation(out, 0, validBuffer(), nil,
		persona.Anchors["Ash"], persona.DriftMarkers["Ash"]); err != nil {
		t.Fatalf("substring tripped the lexicon: %v", err)
	}
}

// TestAnchorsAndMarkersAuthoredForAll: every villager has a firewall.
func TestAnchorsAndMarkersAuthoredForAll(t *testing.T) {
	for _, name := range sim.AgentNames {
		if persona.Anchors[name] == "" {
			t.Errorf("%s has no anchor", name)
		}
		if len(persona.DriftMarkers[name]) == 0 {
			t.Errorf("%s has no drift markers", name)
		}
	}
}
