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
