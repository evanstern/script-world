package tool

import "strings"

// The derived surfaces: each is one walk of the registry, computed live so a
// tool added to the registry (or, in tests, appended via the registry hook)
// flows to all of them with no other edit. These replace the four
// hand-maintained vocabularies (the prompt goal line, the mind parse set, the
// sim plan-step map, and the per-verb gloss prose) whose drift this feature
// exists to kill.
//
// The single-walk invariant (TASK-55 AC#2): VocabularyLine names ≡ WorldGoals
// keys ≡ PlanStepGoals keys — divergence is impossible by construction because
// all three walk the same World-tool set, and every World tool carries
// PlanStep == true.

// VocabularyLine returns the comma-joined World-tool names in registration
// order — byte-identical to the old goal-vocabulary line.
func VocabularyLine() string {
	var names []string
	for _, t := range registry {
		if t.Effect == World {
			names = append(names, t.Name)
		}
	}
	return strings.Join(names, ", ")
}

// PromptGlossBlock returns the concatenated per-verb gloss lines in
// registration order, each terminated by a newline — byte-identical to the old
// hand-written prose block (the lines between "Goals:" and "For a short
// sequence" in internal/mind/prompt.go). Empty when no tool carries a gloss.
func PromptGlossBlock() string {
	var b strings.Builder
	for _, t := range registry {
		if t.PromptGloss != "" {
			b.WriteString(t.PromptGloss)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// WorldGoals returns the set of World-tool names — the mind-side parse accept
// set (replaces the mind's old hand-maintained goal map). A fresh map per call;
// callers on a hot path cache it once.
func WorldGoals() map[string]bool {
	m := make(map[string]bool)
	for _, t := range registry {
		if t.Effect == World {
			m[t.Name] = true
		}
	}
	return m
}

// PlanStepGoals returns the set of names with PlanStep == true — the sim-door
// plan-step accept set (replaces the sim's old hand-maintained plan-step map).
// The FR-012 drift cure lives in the catalog data (every World tool carries
// PlanStep: true), not in code here.
func PlanStepGoals() map[string]bool {
	m := make(map[string]bool)
	for _, t := range registry {
		if t.PlanStep {
			m[t.Name] = true
		}
	}
	return m
}
