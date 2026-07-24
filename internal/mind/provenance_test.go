package mind

// Spec 030 (US1) tests: the consolidation belief-evidence contract (T003) and
// the deterministic provenance enforcement (T004). Model-free — enforcement
// makes no model calls; it coerces, never rejects.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
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

// provBuffer mixes a direct-perception memory (own act) and a secondhand one
// (conversation gist) so a belief can cite either.
func provBuffer() []sim.Memory {
	return []sim.Memory{
		{Text: "Built a fire.", Salience: 5, Tick: 100, Subject: -1, Origin: sim.OriginAction},                        // m1: direct
		{Text: "Talked with Rowan — he claims tendrils.", Salience: 4, Tick: 200, Subject: 3, Origin: sim.OriginGist}, // m2: secondhand
	}
}

// TestEnforceProvenanceTable (T004, FR-004 both directions): the coercion table
// — witnessed survives on direct evidence, coerces to told on secondhand-only,
// to inferred on no/unresolvable evidence; old-shaped (no evidence) witnessed
// coerces to inferred; told/inferred pass through regardless of evidence.
func TestEnforceProvenanceTable(t *testing.T) {
	cases := []struct {
		name        string
		provenance  string
		evidence    []string
		wantLands   string
		wantCoerced bool
		wantDirect  bool
		wantRefs    int
	}{
		{"witnessed + direct → witnessed", sim.ProvenanceWitnessed, []string{"m1"}, sim.ProvenanceWitnessed, false, true, 1},
		{"witnessed + direct among many → witnessed", sim.ProvenanceWitnessed, []string{"m2", "m1"}, sim.ProvenanceWitnessed, false, true, 2},
		{"witnessed + secondhand only → told", sim.ProvenanceWitnessed, []string{"m2"}, sim.ProvenanceTold, true, false, 1},
		{"witnessed + no evidence → inferred", sim.ProvenanceWitnessed, nil, sim.ProvenanceInferred, true, false, 0},
		{"witnessed + unresolvable only → inferred", sim.ProvenanceWitnessed, []string{"m99"}, sim.ProvenanceInferred, true, false, 0},
		{"told + direct passes through (not upgraded)", sim.ProvenanceTold, []string{"m1"}, sim.ProvenanceTold, false, true, 1},
		{"inferred + secondhand passes through", sim.ProvenanceInferred, []string{"m2"}, sim.ProvenanceInferred, false, false, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			beliefs := []beliefChange{{ID: 0, Statement: "s", Confidence: 60, Provenance: c.provenance, Source: -1, Subject: -1, Evidence: c.evidence}}
			coerced := enforceProvenance(beliefs, provBuffer())
			b := beliefs[0]
			if b.Provenance != c.wantLands {
				t.Errorf("provenance = %q, want %q", b.Provenance, c.wantLands)
			}
			if (coerced == 1) != c.wantCoerced {
				t.Errorf("coerced = %d, want coerced=%v", coerced, c.wantCoerced)
			}
			if b.direct != c.wantDirect {
				t.Errorf("direct = %v, want %v", b.direct, c.wantDirect)
			}
			if len(b.resolved) != c.wantRefs {
				t.Errorf("resolved refs = %d, want %d", len(b.resolved), c.wantRefs)
			}
		})
	}
}

// TestEnforceProvenanceDedupesAndResolves (T004): repeated ordinals dedupe and
// resolved refs carry the durable (tick, hash) identity — the promote/fade
// discipline. The Birch case (witnessed tendrils on gist-only evidence) cannot
// land witnessed (SC-001).
func TestEnforceProvenanceDedupesAndResolves(t *testing.T) {
	buf := provBuffer()
	beliefs := []beliefChange{{
		ID: 0, Statement: "Glowing tendrils lurk past the ridge.", Confidence: 68,
		Provenance: sim.ProvenanceWitnessed, Source: -1, Subject: -1,
		Evidence: []string{"m2", "m2", "m99"}, // gist twice + one unresolvable
	}}
	if n := enforceProvenance(beliefs, buf); n != 1 {
		t.Fatalf("coerced = %d, want 1 (the Birch case)", n)
	}
	b := beliefs[0]
	if b.Provenance != sim.ProvenanceTold {
		t.Errorf("Birch tendrils landed %q, want told (SC-001: never witnessed)", b.Provenance)
	}
	if len(b.resolved) != 1 {
		t.Fatalf("resolved = %v, want one deduped ref", b.resolved)
	}
	want := sim.MemoryRef{Tick: buf[1].Tick, Hash: sim.MemoryHash(buf[1].Text)}
	if b.resolved[0] != want {
		t.Errorf("resolved ref = %+v, want %+v", b.resolved[0], want)
	}
}

// TestConsolidationCoercionCounterOnMarker (T004, end-to-end): a scripted night
// proposing a "witnessed" belief evidenced only by conversation lands with the
// belief coerced to hearsay and the coercion counted on the marker — non-fatal
// (the night is accepted).
func TestConsolidationCoercionCounterOnMarker(t *testing.T) {
	// setupConsol's buffer (Ash): all three memories are pre-030 (Origin "" =
	// secondhand) since they are hand-seeded — a "witnessed" belief citing them
	// must coerce. The reply cites m1 (a secondhand-classified memory).
	reply := `{
  "nature": "` + persona.Anchors["Ash"] + `",
  "gist": "A day of wolves, thin meals, and a promise from Cedar.",
  "promote": ["m1"],
  "fade": ["m2"],
  "beliefs": [{"id": 0, "statement": "A wolf hunts the treeline.", "confidence": 70, "provenance": "witnessed", "source": -1, "subject": -1, "evidence": ["m1"]}],
  "narrative": "I keep this village fed and I keep my head."
}`
	h, md := setupConsol(t, &scriptedModel{replies: []string{reply}})
	md.maybeConsolidate(sleptEvent(t, 80000, 0))

	markers := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "agent.consolidated"
	})
	if len(markers) != 1 {
		t.Fatalf("markers = %d, want 1", len(markers))
	}
	var mp sim.ConsolidatedPayload
	if err := json.Unmarshal(markers[0].Payload, &mp); err != nil {
		t.Fatal(err)
	}
	if mp.Outcome != sim.ConsolidationAccepted {
		t.Fatalf("outcome = %q, want accepted (coercion is non-fatal)", mp.Outcome)
	}
	if mp.Coerced != 1 {
		t.Errorf("marker Coerced = %d, want 1", mp.Coerced)
	}

	// The landed belief_revised carries the coerced label, not "witnessed".
	all, _ := h.st.EventsSince(0, 0)
	var landed *sim.BeliefRevisedPayload
	for _, e := range all {
		if e.Type == "agent.belief_revised" {
			var p sim.BeliefRevisedPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			landed = &p
		}
	}
	if landed == nil {
		t.Fatal("no belief_revised landed")
	}
	if landed.Provenance == sim.ProvenanceWitnessed {
		t.Errorf("belief landed witnessed on secondhand evidence (SC-001 breach)")
	}
}
