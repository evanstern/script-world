package mind

import (
	"encoding/json"
	"testing"
)

// TestPlannerReplySchema (TASK-58): the generated structured-output schema is
// valid JSON, its goal enum is exactly the validGoals set (not a hand-copied
// drift), and its plan array cap is planStepCap — the single-source-of-truth
// guarantee that lets the sampler constraint and parseReply's gate agree.
func TestPlannerReplySchema(t *testing.T) {
	raw := plannerReplySchema()
	if !json.Valid(raw) {
		t.Fatalf("schema is not valid JSON: %s", raw)
	}

	var schema struct {
		Properties struct {
			Goal struct {
				Enum []string `json:"enum"`
			} `json:"goal"`
			Plan struct {
				MaxItems int `json:"maxItems"`
				Items    struct {
					Properties struct {
						Goal struct {
							Enum []string `json:"enum"`
						} `json:"goal"`
					} `json:"properties"`
					Required []string `json:"required"`
				} `json:"items"`
			} `json:"plan"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	// Top-level goal enum == validGoals, exactly.
	assertEnumIsGoals(t, "top-level goal", schema.Properties.Goal.Enum)
	// Plan step goal enum == validGoals, exactly (steps carry the same vocab).
	assertEnumIsGoals(t, "plan step goal", schema.Properties.Plan.Items.Properties.Goal.Enum)

	if schema.Properties.Plan.MaxItems != planStepCap {
		t.Errorf("plan maxItems = %d, want planStepCap %d", schema.Properties.Plan.MaxItems, planStepCap)
	}

	// goal+reason are required up top; plan is optional and, when present,
	// parseReply prefers it (discarding the top-level goal) so the plan form
	// still parses. Each plan step requires only "goal". (Requiring goal — vs.
	// the brief's reason-only — is what drives cogito:3b's reason-only replies
	// to zero; see plannerReplySchema's doc comment.)
	if got := schema.Required; len(got) != 2 || got[0] != "goal" || got[1] != "reason" {
		t.Errorf("top-level required = %v, want [goal reason]", got)
	}
	if got := schema.Properties.Plan.Items.Required; len(got) != 1 || got[0] != "goal" {
		t.Errorf("plan step required = %v, want [goal]", got)
	}
}

func assertEnumIsGoals(t *testing.T, label string, enum []string) {
	t.Helper()
	if len(enum) != len(validGoals) {
		t.Errorf("%s enum has %d values, want %d (validGoals)", label, len(enum), len(validGoals))
	}
	seen := make(map[string]bool, len(enum))
	for _, g := range enum {
		if !validGoals[g] {
			t.Errorf("%s enum contains %q, not in validGoals", label, g)
		}
		seen[g] = true
	}
	for g := range validGoals {
		if !seen[g] {
			t.Errorf("%s enum missing validGoals entry %q", label, g)
		}
	}
}
