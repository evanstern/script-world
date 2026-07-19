package mind

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/evanstern/script-world/internal/sim"
)

// The persona firewall's automated half (TASK-9): a deterministic,
// mechanical validator — no second model call — that judges every
// consolidation output before anything lands. Any failure rejects the WHOLE
// night. Rejection reasons are stable strings recorded in the
// agent.consolidated marker.

// memRef is how the model references a buffer memory (contract: the prompt
// supplies tick+hash for every memory it is shown).
type memRef struct {
	Tick int64  `json:"tick"`
	Hash string `json:"hash"`
}

type beliefChange struct {
	ID         int    `json:"id"` // 0 = new
	Statement  string `json:"statement"`
	Confidence int    `json:"confidence"`
	Provenance string `json:"provenance"`
	Source     int    `json:"source"`
	Subject    int    `json:"subject"`
}

// consolidationOutput is the model's reply, per
// contracts/consolidation-output.md.
type consolidationOutput struct {
	Nature    string         `json:"nature"`
	Gist      string         `json:"gist"`
	Promote   []memRef       `json:"promote"`
	Fade      []memRef       `json:"fade"`
	Beliefs   []beliefChange `json:"beliefs"`
	Narrative string         `json:"narrative"`
}

// Validator caps (contract layer 1).
const (
	maxPromotes     = 5
	maxFades        = 8
	maxBeliefEdits  = 4
	maxNarrativeLen = 1200
	maxGistLen      = 240
)

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
	inBuffer := make(map[memRef]bool, len(buffer))
	for _, m := range buffer {
		inBuffer[memRef{Tick: m.Tick, Hash: sim.MemoryHash(m.Text)}] = true
	}
	for _, r := range append(append([]memRef{}, out.Promote...), out.Fade...) {
		if !inBuffer[r] {
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
	if strings.TrimSpace(out.Nature) != strings.TrimSpace(anchor) {
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
