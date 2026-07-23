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
	// Journal tools (spec 019, US3): the villager's private notebook — two
	// acting (Expressive) and two Read. Appended after muse so no existing
	// declared tool's position shifts; villager-only (the metatron roster is
	// untouched, journals are private).
	for _, n := range []string{"write_journal_entry", "delete_from_journal", "search_journal", "read_journal"} {
		if t, ok := Lookup(n); ok {
			out = append(out, t)
		}
	}
	return out
}

// loopMetatronTools is the ordered declared-tool list the metatron tool-use
// loop presents to the model (spec 017 T020): the two nudge forms, then
// work_miracle (the R13 post-#38 amendment). It is NOT RosterMetatron:
// converse is DELIBERATELY excluded. converse is the final-answer channel, not
// a callable tool — the angel speaks by replying with text (toolloop Result
// Final), and the loop ends naturally (model_done) when it does. Declaring
// converse would trap a converse call as rejected_unknown (metatron installs no
// converse handler, by design: "converse is the transcript, not a door"), so it
// is offered only as the implicit text channel, never as a tool the model can
// call. work_miracle rides at the end so no existing tool's declared position
// shifts.
var loopMetatronTools = []string{"nudge_dream", "nudge_omen", "work_miracle"}

// LoopRosterMetatron returns the ordered declared-tool list the metatron
// tool-use loop presents to the model (loopMetatronTools), resolved to full
// Tool values — InputSchema (derive.go) needs each tool's Params, not just its
// name. RosterMetatron stays the pre-loop, name-only DOOR roster (landNudge's
// OnRoster check); this is the loop's DECLARED surface, which differs (converse
// excluded, work_miracle included).
func LoopRosterMetatron() []Tool {
	out := make([]Tool, 0, len(loopMetatronTools))
	for _, n := range loopMetatronTools {
		if t, ok := Lookup(n); ok {
			out = append(out, t)
		}
	}
	return out
}
