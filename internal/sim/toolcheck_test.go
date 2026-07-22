package sim

import (
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/tool"
)

// TestToolCoverageClean: the shipped registry + sim tables satisfy the coverage
// gate — every world tool resolves and has a duration, every expressive tool's
// events are whitelisted. This is the check daemon boot runs (FR-003).
func TestToolCoverageClean(t *testing.T) {
	if err := ValidateToolCoverage(); err != nil {
		t.Fatalf("tool coverage failed: %v", err)
	}
}

// TestCoverageRejectsMissingResolver (T028, US3 scenario 3): a World tool with
// no resolver arm (and no duration) is caught by the coverage check — a config
// error that would abort boot, never a tick-time failure. Exercised with a
// synthetic tool absent from the sim resolver/duration tables.
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
func TestWhitelistDiffIdentical(t *testing.T) {
	want := map[string]bool{
		"social.relation_changed":       true,
		"social.rumor_told":             true,
		"social.conversation_turn":      true,
		"social.conversation":           true,
		"agent.memory_added":            true,
		"agent.memory_promoted":         true,
		"agent.memory_faded":            true,
		"agent.belief_revised":          true,
		"agent.narrative_set":           true,
		"agent.consolidated":            true,
		"agent.thought":                 true,
		"chronicle.entry":               true,
		"metatron.nudged":               true,
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
// phase, ADR-equivalent decision recorded on the board): every World tool's
// registry Cost.DurationTicks must equal the sim duration constant it was
// hand-carried from at migration time (R7 accepted this as intentional
// duplication, not a derivation, because internal/tool is a leaf package that
// cannot import internal/sim). This test is the trip-wire against that
// duplication silently drifting — it lives in internal/sim (not internal/tool)
// precisely because only this package can see both sides: the registry via
// tool.All() and the unexported *Ticks constants in agents.go.
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
	for _, tl := range tool.All() {
		if tl.Effect != tool.World {
			continue
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
}
