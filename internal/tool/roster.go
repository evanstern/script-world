package tool

// Rosters express capability as membership (spec 014 US3, research R4): each
// agent kind has an ordered list of the registry tool names it may use. An
// action naming a tool outside the acting agent's roster is rejected at the
// door exactly like an unknown action. Rosters are data, not code branches.
//
// The villager roster's world-verb portion is DERIVED from the registry (every
// World tool, in registration order) so that adding a world verb is a single
// registry edit — it joins the roster and every derived surface at once
// (SC-001). The expressive membership is explicit, because say/muse/gist and
// converse/nudge_* are all Effect Expressive and only the roster distinguishes
// which agent kind holds which.

// villagerExpressive are the expressive tools a villager may use, in roster
// order (say, muse, gist).
var villagerExpressive = []string{"say", "muse", "gist"}

// RosterVillager is the villager capability set: every legacy World tool
// (Effect World AND PlanStep true, isLegacyWorldTool in derive.go) in
// registration order, then the villager expressive tools. set_plan is
// deliberately excluded — it is Effect World but loop-only (PlanStep false);
// it appears only in LoopRosterVillager below.
var RosterVillager = func() []string {
	out := make([]string, 0, len(registry))
	for _, t := range registry {
		if isLegacyWorldTool(t) {
			out = append(out, t.Name)
		}
	}
	return append(out, villagerExpressive...)
}()

// RosterMetatron is the metatron capability set: its converse channel and the
// two nudge forms.
var RosterMetatron = []string{"converse", "nudge_dream", "nudge_omen"}

// OnRoster reports whether name is on roster — the door membership check.
func OnRoster(roster []string, name string) bool {
	for _, n := range roster {
		if n == name {
			return true
		}
	}
	return false
}

// LoopRosterVillager returns the ordered declared-tool list the villager
// tool-use loop presents to the model (spec 017 contracts/loop-api.md,
// data-model.md §2): every legacy World tool in registration order, then
// set_plan, then muse. Unlike RosterVillager (name-only membership, for the
// door's roster check), this returns full Tool values — InputSchema
// (derive.go) needs each tool's Params/InputSchemaJSON, not just its name.
//
// say/gist stay scene-gated and out of the loop roster this task (data-model
// §2): scenes remain driver-run, not model-initiated via the loop.
func LoopRosterVillager() []Tool {
	out := make([]Tool, 0, len(registry))
	for _, t := range registry {
		if isLegacyWorldTool(t) {
			out = append(out, t)
		}
	}
	if sp, ok := Lookup("set_plan"); ok {
		out = append(out, sp)
	}
	if muse, ok := Lookup("muse"); ok {
		out = append(out, muse)
	}
	return out
}

// LoopRosterMetatron returns the ordered declared-tool list the metatron
// tool-use loop presents to the model: converse, nudge_dream, nudge_omen —
// the same membership as RosterMetatron, resolved to full Tool values.
func LoopRosterMetatron() []Tool {
	out := make([]Tool, 0, len(RosterMetatron))
	for _, n := range RosterMetatron {
		if t, ok := Lookup(n); ok {
			out = append(out, t)
		}
	}
	return out
}
