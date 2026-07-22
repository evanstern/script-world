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

// RosterVillager is the villager capability set: every World tool in
// registration order, then the villager expressive tools.
var RosterVillager = func() []string {
	out := make([]string, 0, len(registry))
	for _, t := range registry {
		if t.Effect == World {
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
