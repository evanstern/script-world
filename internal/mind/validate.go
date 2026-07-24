package mind

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/evanstern/promptworld/internal/sim"
)

// The persona firewall's automated half (TASK-9): a deterministic,
// mechanical validator — no second model call — that judges every
// consolidation output before anything lands. Any failure rejects the WHOLE
// night. Rejection reasons are stable strings recorded in the
// agent.consolidated marker.

// Buffer memories are shown to the model with ordinal labels ("m1".."m60")
// and referenced back the same way — models transcribe short ordinals
// reliably where they mangle hashes. parseMemRef maps a label to its
// buffer index, or -1.
func parseMemRef(ref string, bufferLen int) int {
	if len(ref) < 2 || (ref[0] != 'm' && ref[0] != 'M') {
		return -1
	}
	n := 0
	for _, c := range ref[1:] {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > bufferLen {
		return -1
	}
	return n - 1
}

type beliefChange struct {
	ID         int      `json:"id"` // 0 = new
	Statement  string   `json:"statement"`
	Confidence int      `json:"confidence"`
	Provenance string   `json:"provenance"`
	Source     int      `json:"source"`
	Subject    int      `json:"subject"`
	Evidence   []string `json:"evidence,omitempty"` // spec 030: ordinal buffer refs ("m3") the belief rests on
}

// consolidationOutput is the model's reply, per
// contracts/consolidation-output.md.
type consolidationOutput struct {
	Nature    string         `json:"nature"`
	Gist      string         `json:"gist"`
	Promote   []string       `json:"promote"` // ordinal refs, "m3"
	Fade      []string       `json:"fade"`
	Beliefs   []beliefChange `json:"beliefs"`
	Narrative string         `json:"narrative"`
}

// Validator caps (contract layer 1).
const (
	maxPromotes     = 5
	maxFades        = 8
	maxBeliefEdits  = 4
	maxNarrativeLen = 1200
	// maxBeliefEvidence (spec 030) caps a belief's evidence citations. Over-long
	// lists are pre-trimmed best-first before judging — absorbed, not punished
	// (contracts/consolidation-contract.md), so there is no matching reject.
	maxBeliefEvidence = 4
	// Prompt asks for <200 chars; the cap allows overrun headroom (live
	// finding: hard-failing a whole night on a wordy sentence is waste).
	maxGistLen = 300
)

var anchorSpaces = regexp.MustCompile(`\s+`)

func normalizeAnchor(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, ".!")
	return anchorSpaces.ReplaceAllString(s, " ")
}

// validateConsolidation runs the three firewall layers against the exact
// buffer and belief set that were sent to the model. A nil error means the
// whole output may land.
func validateConsolidation(out consolidationOutput, agent int, buffer []sim.Memory, held []sim.Belief, anchor string, drift []string) error {
	// --- layer 1: structure ---
	if len(out.Promote) > maxPromotes {
		return fmt.Errorf("too_many_promotes")
	}
	if len(out.Fade) > maxFades {
		return fmt.Errorf("too_many_fades")
	}
	if len(out.Beliefs) > maxBeliefEdits {
		return fmt.Errorf("too_many_beliefs")
	}
	if out.Gist == "" || len(out.Gist) > maxGistLen {
		return fmt.Errorf("bad_gist")
	}
	if out.Narrative == "" || len(out.Narrative) > maxNarrativeLen {
		return fmt.Errorf("bad_narrative")
	}
	for _, r := range append(append([]string{}, out.Promote...), out.Fade...) {
		if parseMemRef(r, len(buffer)) < 0 {
			return fmt.Errorf("unknown_memory_ref")
		}
	}
	heldIDs := make(map[int]bool, len(held))
	for _, b := range held {
		heldIDs[b.ID] = true
	}
	for _, b := range out.Beliefs {
		if b.Confidence < 0 || b.Confidence > 100 {
			return fmt.Errorf("confidence_out_of_range")
		}
		switch b.Provenance {
		case sim.ProvenanceWitnessed, sim.ProvenanceTold, sim.ProvenanceInferred:
		default:
			return fmt.Errorf("bad_provenance")
		}
		if b.Source < -1 || b.Source >= sim.AgentCount || b.Subject < -1 || b.Subject >= sim.AgentCount {
			return fmt.Errorf("bad_agent_ref")
		}
		if b.ID != 0 && !heldIDs[b.ID] {
			return fmt.Errorf("unknown_belief_id")
		}
		if b.Statement == "" {
			return fmt.Errorf("empty_belief")
		}
	}

	// --- layer 2: anchor echo ---
	// Normalized comparison (case, whitespace runs, trailing punctuation):
	// echo fidelity is the canary, not typography — live testing showed
	// models add a period or capitalize while restating faithfully. A
	// paraphrase still fails.
	if normalizeAnchor(out.Nature) != normalizeAnchor(anchor) {
		return fmt.Errorf("anchor_mismatch")
	}

	// --- layer 3: drift lexicon (narrative + self-beliefs) ---
	selfTexts := []string{out.Narrative}
	for _, b := range out.Beliefs {
		if b.Subject == agent {
			selfTexts = append(selfTexts, b.Statement)
		}
	}
	for _, marker := range drift {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(marker) + `\b`)
		for _, text := range selfTexts {
			if re.MatchString(text) {
				return fmt.Errorf("drift:%s", strings.ToLower(marker))
			}
		}
	}
	return nil
}
