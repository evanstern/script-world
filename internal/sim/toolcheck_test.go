package sim

import (
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/tool"
)

// TestToolCoverageClean: the shipped registry + sim tables satisfy the
// coverage gate — every goal-door world tool resolves and has a duration,
// every expressive tool's events are whitelisted, and set_plan (a World tool
// outside the goal door, spec 017 R11) does not break the gate. This is the
// check daemon boot runs (FR-003); before T004b, registering set_plan made
// this fail and the real daemon fail to boot.
func TestToolCoverageClean(t *testing.T) {
	if err := ValidateToolCoverage(); err != nil {
		t.Fatalf("tool coverage failed: %v", err)
	}
}

// TestCoverageRejectsMissingResolver (T028, US3 scenario 3): a GOAL-DOOR World
// tool (PlanStep true) with no resolver arm (and no duration) is caught by the
// coverage check — a config error that would abort boot, never a tick-time
// failure. Exercised with a synthetic tool absent from the sim resolver/
// duration tables.
func TestCoverageRejectsMissingResolver(t *testing.T) {
	bogus := append(tool.All(), tool.Tool{Name: "ghost_verb", Effect: tool.World, PlanStep: true})
	err := validateCoverage(bogus)
	if err == nil {
		t.Fatal("expected coverage failure for a world tool with no resolver")
	}
	if !strings.Contains(err.Error(), "ghost_verb") || !strings.Contains(err.Error(), "resolver") {
		t.Errorf("error should name the uncovered tool and its missing resolver: %v", err)
	}
}

// TestCoverageExemptsNonGoalDoorWorldTools (spec 017 T004b): a World tool
// outside the goal-door vocabulary (PlanStep false — set_plan's shape: it
// grounds through injectPlan, never resolveGoal) is exempt from the
// resolver/duration requirement. Pinned two ways: against the real registry
// (set_plan itself) and against a synthetic non-goal-door fixture with no
// resolver or duration entry at all — proving the exemption is deliberate,
// not an accident of set_plan happening to have one.
func TestCoverageExemptsNonGoalDoorWorldTools(t *testing.T) {
	if err := validateCoverage(tool.All()); err != nil {
		t.Fatalf("real registry (including set_plan, a non-goal-door World tool) should validate clean: %v", err)
	}

	synthetic := append(tool.All(), tool.Tool{Name: "ghost_loop_tool", Effect: tool.World, PlanStep: false})
	if err := validateCoverage(synthetic); err != nil {
		t.Errorf("a non-goal-door World tool (PlanStep false) must be exempt from the resolver/duration check, got: %v", err)
	}
}

// TestExpressiveEventsWhitelisted (T022, FR-013): every event type a registry
// expressive tool declares is on the injection whitelist — the registry names
// only a slice of the door's backstop, never a type the door would reject.
func TestExpressiveEventsWhitelisted(t *testing.T) {
	for _, tl := range tool.All() {
		if tl.Effect != tool.Expressive {
			continue
		}
		for _, ev := range tl.Events {
			if !injectSocialWhitelist[ev] {
				t.Errorf("expressive tool %q declares non-whitelisted event %q", tl.Name, ev)
			}
		}
	}
}

// TestWhitelistDiffIdentical (T022, FR-013): the injection whitelist is
// byte-for-byte the pre-refactor set — the tool-registry layer adds and removes
// ZERO entries. The whitelist is the isolation boundary; the registry
// formalizes what passes through it without widening or narrowing it. If a
// future change touches the whitelist, this test must be updated deliberately.
//
// Spec 017 (agent tool loop) deliberately widens the boundary by exactly one
// entry, "cog.tool_call" (contracts/events.md: "the whitelist's ONLY
// change") — the tool-use loop's call trace, a cog.* type like the three
// above it.
func TestWhitelistDiffIdentical(t *testing.T) {
	want := map[string]bool{
		"social.relation_changed":  true,
		"social.rumor_told":        true,
		"social.conversation_turn": true,
		"social.conversation":      true,
		"agent.memory_added":       true,
		"agent.memory_promoted":    true,
		"agent.memory_faded":       true,
		"agent.belief_revised":     true,
		"agent.narrative_set":      true,
		"agent.consolidated":       true,
		"agent.thought":            true,
		"chronicle.entry":          true,
		"metatron.nudged":          true,
		// Spec 016 (metatron miracles) deliberately widens the isolation
		// boundary by four recorded miracle event types (contracts §4). They
		// land through the same InjectSocial door as the nudge; they are NOT
		// registry-tool events (see the registry-doctrine note below), so they
		// appear only here in the whitelist, not in any tool's Events set.
		"metatron.time_snapped":         true,
		"metatron.item_granted":         true,
		"metatron.entity_moved":         true,
		"metatron.entity_removed":       true,
		"meeting.proposal_rephrased":    true,
		"cog.thought":                   true,
		"cog.outcome":                   true,
		"cog.recalibration_recommended": true,
		"cog.tool_call":                 true,
		// Spec 019 (agent journal) deliberately widens the boundary by exactly
		// two entries — the journal mutations, landed through the same door and
		// declared as the two Expressive journal tools' Events (pinned ⊆ this
		// whitelist by ValidateToolCoverage).
		"journal.entry_written": true,
		"journal.entry_deleted": true,
		// Spec 029 (metatron agency) widens the boundary by the injected standing-
		// order events. order_placed / order_cancelled are the monitor_and_act /
		// cancel_order tool Events (pinned ⊆ this whitelist by ValidateToolCoverage).
		// order_triggered (the trigger worker's injection) joins in T004; order_
		// expired is EXECUTOR-emitted (like charge_regenerated) and is deliberately
		// never here — only injected types need the door.
		"metatron.order_placed":    true,
		"metatron.order_cancelled": true,
	}
	for typ := range want {
		if !injectSocialWhitelist[typ] {
			t.Errorf("whitelist is missing pre-refactor entry %q (an entry was REMOVED)", typ)
		}
	}
	for typ := range injectSocialWhitelist {
		if !want[typ] {
			t.Errorf("whitelist has unexpected entry %q (an entry was ADDED)", typ)
		}
	}
	if len(injectSocialWhitelist) != len(want) {
		t.Errorf("whitelist size = %d, want %d", len(injectSocialWhitelist), len(want))
	}
}

// TestWorldToolDurationsMatchSimConstants (ratified addition, TASK-53 polish
// phase, ADR-equivalent decision recorded on the board): every GOAL-DOOR World
// tool's registry Cost.DurationTicks must equal the sim duration constant it
// was hand-carried from at migration time (R7 accepted this as intentional
// duplication, not a derivation, because internal/tool is a leaf package that
// cannot import internal/sim). This test is the trip-wire against that
// duplication silently drifting — it lives in internal/sim (not internal/tool)
// precisely because only this package can see both sides: the registry via
// tool.All() and the unexported *Ticks constants in agents.go.
//
// set_plan (spec 017 R11) is a World tool outside the goal door (PlanStep
// false) — it carries no sim duration constant to drift against (it never
// reaches intentDuration by its own name), so it is deliberately skipped here,
// pinned by the trailing exemption check below.
func TestWorldToolDurationsMatchSimConstants(t *testing.T) {
	want := map[string]int64{
		"forage":        forageTicks,
		"chop":          chopTicks,
		"hunt":          huntTicks,
		"build_fire":    buildFireTicks,
		"build_shelter": buildShelterTicks,
		"eat":           0, // no constant pre-refactor either — instant, like sleep/wander
		"sleep":         0,
		"wander":        0,
		"goto_warmth":   0,
		"talk_to":       0,
		"quarry":        quarryTicks,
		"collect_water": collectWaterTicks,
		"cook":          cookFireTicks, // base fire-cook duration; oven override is executor-side (workDuration)
		"refuel_fire":   0,
		"craft_planks":  craftPlanksTicks,
		"craft_stone":   craftStoneTicks,
		"craft_spear":   craftSpearTicks, // hunt's spear-carry override (huntTicksSpear) is a separate constant
		"build_oven":    buildOvenTicks,
		"bathe":         batheTicks,
		"drop":          0,
		"pick_up":       0,
		"build_chest":   buildFireTicks, // recipes.go's build_chest recipe entry reuses buildFireTicks (600)
		"deposit":       0,
		"withdraw":      0,
	}

	seen := make(map[string]bool, len(want))
	sawSetPlan := false
	for _, tl := range tool.All() {
		if tl.Name == "set_plan" {
			sawSetPlan = true
		}
		if tl.Effect != tool.World || !tl.PlanStep {
			continue // non-goal-door World tools (set_plan) are exempt — see doc above.
		}
		seen[tl.Name] = true
		wantTicks, ok := want[tl.Name]
		if !ok {
			t.Errorf("world tool %q has no entry in this test's expected-duration table — add one so a new tool can't drift unnoticed", tl.Name)
			continue
		}
		if tl.Cost.DurationTicks != wantTicks {
			t.Errorf("%s: registry Cost.DurationTicks = %d, want %d (sim constant)", tl.Name, tl.Cost.DurationTicks, wantTicks)
		}
	}
	// The reverse check: every name in want must be a real World tool, so a
	// renamed or removed tool can't leave a stale, silently-passing entry.
	for name := range want {
		if !seen[name] {
			t.Errorf("expected-duration table names %q, which is not a registered World tool", name)
		}
	}
	// Pin the exemption itself: set_plan is a real registry entry that was
	// deliberately excluded above (not just absent from the registry).
	if !sawSetPlan {
		t.Fatal("set_plan is not registered — this test's exemption for it is untested")
	}
	if seen["set_plan"] {
		t.Error("set_plan should not be a goal-door World tool (PlanStep must be false)")
	}
	if _, wanted := want["set_plan"]; wanted {
		t.Error("set_plan must not appear in the expected-duration table (it is exempt, not zero-duration)")
	}
}
