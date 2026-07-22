package tool

// Test-only registry manipulation. These swap the package-level registry (and
// its byName index and rosters) and return a restore closure, so a test can
// exercise the derivation/validation machinery against ad-hoc fixtures without
// any test-only hook in the production API. Restore is idempotent.

func rebuildIndex() {
	byName = make(map[string]Tool, len(registry))
	for _, t := range registry {
		byName[t.Name] = t
	}
}

// withExtraTool appends one tool to the live registry and returns a restore
// closure. The derived surfaces (VocabularyLine/WorldGoals/PlanStepGoals) walk
// the registry live, so they reflect the addition immediately.
func withExtraTool(extra Tool) func() {
	savedReg := registry
	savedIdx := byName

	registry = append(append([]Tool{}, savedReg...), extra)
	rebuildIndex()

	done := false
	return func() {
		if done {
			return
		}
		registry = savedReg
		byName = savedIdx
		done = true
	}
}

// swapRegistry replaces the registry and rosters wholesale (for malformed-case
// validation). Returns a restore closure.
func swapRegistry(reg []Tool, villager, metatron []string) func() {
	savedReg, savedIdx := registry, byName
	savedV, savedM := RosterVillager, RosterMetatron

	registry = reg
	rebuildIndex()
	RosterVillager = villager
	RosterMetatron = metatron

	return func() {
		registry, byName = savedReg, savedIdx
		RosterVillager, RosterMetatron = savedV, savedM
	}
}
