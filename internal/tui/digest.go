package tui

// Chronicle digest registry (TASK-60, specs/018-chronicle-digest). Every
// cataloged event type gets a digestFunc here, keyed by its full event type
// — the per-type template it implements is contracts/digest-grammar.md §3
// row-for-row; sample payloads for every row live in the catalog sweep
// fixture (digest_test.go, contract §7). A registry miss, or ok=false on
// unmarshal failure, is handled by formatChronicleLine's fallback
// (FR-002/FR-003) — never here.
//
// Each digestFunc is a pure function over the stored event + the replica's
// agent-name table (R1), returning the summary as ordered role-tagged
// segments (grammar.go's `seg`) — no lipgloss, no ANSI. Where the real
// payload struct (internal/sim) didn't carry a field the contract's
// illustrative template assumed, the digest below adapts to the actual
// struct and a doc comment says so; the implementer's report to the
// orchestrator lists every such row.

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
)

// digestFunc renders one event type's summary, or ok=false on unmarshal
// failure (data-model.md).
type digestFunc func(e store.Event, names []string) (segs []seg, ok bool)

// decode unmarshals an event's payload into T; ok=false on failure is the
// signal formatChronicleLine falls back on.
func decode[T any](e store.Event) (T, bool) {
	var p T
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		var zero T
		return zero, false
	}
	return p, true
}

// --- seg builders — small helpers keeping the registry table legible ---

func txt(s string) seg { return seg{Text: s, Role: segText} }
func nameOf(names []string, idx int) seg {
	return seg{Text: agentName(names, idx), Role: segName}
}
func speech(s string) seg { return seg{Text: fmt.Sprintf("%q", s), Role: segSpeech} }
func emph(s string) seg   { return seg{Text: s, Role: segEmphasis} }
func label(kv string) seg { return seg{Text: kv, Role: segLabel} }
func emphN(n int) seg     { return emph(strconv.Itoa(n)) }
func emphI64(n int64) seg { return emph(strconv.FormatInt(n, 10)) }
func coord(x, y int) seg  { return emph(fmt.Sprintf("(%d,%d)", x, y)) }

// labeled builds a space-separated run of "key=value" segLabel spans — the
// cog/clock/daemon telemetry voice (contract §2).
func labeled(pairs ...string) []seg {
	out := make([]seg, 0, len(pairs)*2-1)
	for i, p := range pairs {
		if i > 0 {
			out = append(out, txt(" "))
		}
		out = append(out, label(p))
	}
	return out
}

// truncateRunes bounds a free-text field the contract explicitly marks
// "truncating" (chronicle.entry's text, plan_set's goal list).
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// join concatenates seg slices — a small variadic append helper so a
// registry entry can compose fixed segs with a conditional tail.
func join(parts ...[]seg) []seg {
	var out []seg
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// debtPercent expresses a measured governor debt (a budget-fraction sum,
// spec 028 FR-001) as a whole percent of cognition.ShedThreshold, rounded to
// the nearest percent — the shared arithmetic behind both the header's
// governed-speed suffix (views.go) and the governor digest lines below.
func debtPercent(debt float64) int {
	return int(math.Round(debt / cognition.ShedThreshold * 100))
}

// gratisMark appends a visible " (forced)" annotation when a miracle's
// Gratis flag waived its charge (internal/sim/miracles.go) — the operator
// force spec 016 SC-004 requires stay enumerable must be visible in the
// digest line, not just inferable from the payload. nil (no segs) when the
// miracle was charge-priced, so the plain summary is unchanged for the
// common case.
func gratisMark(gratis bool) []seg {
	if !gratis {
		return nil
	}
	return []seg{txt(" ("), emph("forced"), txt(")")}
}

// digestRegistry is the ~80-entry per-type table (contract §3). A key
// absent here (or present in the fixture but not here) fails the catalog
// sweep test (digest_test.go, contract §7, SC-001).
var digestRegistry = map[string]digestFunc{
	// --- world / clock / daemon ---

	"world.created": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.WorldCreatedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("world "), emph(fmt.Sprintf("%q", p.Name)), txt(" created · seed "), emph(fmt.Sprintf("%d", p.Seed))}), true
	},
	// world.migrated elides the embedded sim.State entirely (FR-011) — the
	// detail pane (Phase 4) is where an oversized payload gets bounded, not
	// the feed line.
	"world.migrated": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.WorldMigratedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("migrated from format v"), emphN(p.FromFormat),
			txt(" · "), emphI64(p.SourceEvents), txt(" events @ tick "), emphI64(p.SourceTick),
		}), true
	},
	"clock.paused":  func(e store.Event, names []string) ([]seg, bool) { return []seg{txt("paused")}, true },
	"clock.resumed": func(e store.Event, names []string) ([]seg, bool) { return []seg{txt("resumed")}, true },
	"clock.speed_set": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.SpeedSetPayload](e)
		if !ok {
			return nil, false
		}
		return labeled("speed=" + string(p.Speed)), true
	},
	"clock.degraded": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DegradedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("degraded ")}, labeled(fmt.Sprintf("rate=%.2f", p.EffectiveRate))), true
	},
	"clock.recovered": func(e store.Event, names []string) ([]seg, bool) { return []seg{txt("recovered")}, true },
	// clock.governor_shed / clock.governor_recovered (spec 028 FR-008): the
	// governor's speed-ladder decisions, one line each in the clock.degraded
	// line's style — the notch transition plus the debt/jobs arithmetic that
	// justified it (contracts/status-protocol.md "TUI" §). requested is
	// omitted here (unlike the header) since from→to already carries the
	// interesting delta and every other clock.* digest row stays this terse.
	"clock.governor_shed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GovernorPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("governor shed "), emph(string(p.From) + "→" + string(p.To)), txt(" "),
		}, labeled(fmt.Sprintf("debt=%d%%", debtPercent(p.Debt)), fmt.Sprintf("jobs=%d", p.Jobs))), true
	},
	"clock.governor_recovered": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GovernorPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("governor recovered "), emph(string(p.From) + "→" + string(p.To)), txt(" "),
		}, labeled(fmt.Sprintf("debt=%d%%", debtPercent(p.Debt)), fmt.Sprintf("jobs=%d", p.Jobs))), true
	},
	// daemon.started/stopped: "labeled dump of payload fields" (contract §3)
	// — verified against internal/daemon/daemon.go's appendDaemonEvent calls.
	"daemon.started": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DaemonStartedPayload](e)
		if !ok {
			return nil, false
		}
		return labeled(fmt.Sprintf("tick=%d", p.Tick), fmt.Sprintf("recovery_ms=%d", p.RecoveryMs)), true
	},
	"daemon.stopped": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DaemonStoppedPayload](e)
		if !ok {
			return nil, false
		}
		return labeled(fmt.Sprintf("tick=%d", p.Tick)), true
	},

	// --- sim ---

	"sim.day_started": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DayPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("day "), emphI64(p.Day), txt(" begins")}), true
	},
	"sim.night_started": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DayPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("night falls on day "), emphI64(p.Day)}), true
	},
	"sim.forage_regrown": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.RegrownPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("forage regrew at "), coord(p.X, p.Y)}), true
	},
	"sim.fire_burned_out": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.FireBurnedOutPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("the fire at "), coord(p.X, p.Y), txt(" burned out")}), true
	},
	"sim.food_rotted": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.FoodRottedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{emphN(p.N), txt(" "), emph(p.Kind), txt(" rotted at "), coord(p.X, p.Y)}), true
	},
	"sim.gathering_observed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GatheringObservedPayload](e)
		if !ok {
			return nil, false
		}
		if p.X == 0 && p.Y == 0 && p.Start == 0 {
			return []seg{txt("gathering dispersed")}, true
		}
		return join([]seg{txt("gathering at "), coord(p.X, p.Y), txt(" since tick "), emphI64(p.Start)}), true
	},

	// --- agent: acts & needs ---

	"agent.intent_set": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.IntentSetPayload](e)
		if !ok {
			return nil, false
		}
		out := join([]seg{nameOf(names, p.Agent), txt(" intends "), emph(p.Goal), txt(" (" + p.Source + ")")})
		// Presence heuristic (implementer decision, no sentinel in the
		// payload): a nonzero target coordinate is treated as "target set".
		if p.TargetX != 0 || p.TargetY != 0 {
			out = join(out, []seg{txt(" → "), coord(p.TargetX, p.TargetY)})
		}
		return out, true
	},
	"agent.work_started": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.WorkStartedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" set to work")}), true
	},
	"agent.intent_done": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.AgentPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" finished")}), true
	},
	"agent.intent_rejected": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.IntentRejectedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			nameOf(names, p.Agent), txt("'s "), emph(p.Goal), txt(" refused: "), emph(p.Reason),
			txt(" ("), emphI64(p.StalenessTicks), txt("t stale)"),
		}), true
	},
	"agent.moved": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.AgentMovedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" → "), coord(p.X, p.Y)}), true
	},
	"agent.foraged": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HarvestPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" foraged at "), coord(p.X, p.Y)}), true
	},
	"agent.chopped": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HarvestPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" chopped wood at "), coord(p.X, p.Y)}), true
	},
	"agent.hunted": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HarvestPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" hunted at "), coord(p.X, p.Y)}), true
	},
	"agent.quarried": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HarvestPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" quarried stone at "), coord(p.X, p.Y)}), true
	},
	"agent.collected_water": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HarvestPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" drew water at "), coord(p.X, p.Y)}), true
	},
	"agent.crafted": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.CraftedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" crafted "), emph(p.Kind)}), true
	},
	"agent.built": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.BuiltPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" built a "), emph(p.Kind), txt(" at "), coord(p.X, p.Y)}), true
	},
	"agent.dropped": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DroppedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" dropped "), emphN(p.N), txt(" "), emph(p.Kind), txt(" at "), coord(p.X, p.Y)}), true
	},
	"agent.picked_up": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.PickedUpPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" picked up "), emphN(p.N), txt(" "), emph(p.Kind), txt(" at "), coord(p.X, p.Y)}), true
	},
	"agent.deposited": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.DepositedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			nameOf(names, p.Agent), txt(" stored "), emphN(p.N), txt(" "), emph(p.Kind),
			txt(" in the chest at "), coord(p.X, p.Y),
		}), true
	},
	"agent.withdrew": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.WithdrewPayload](e)
		if !ok {
			return nil, false
		}
		out := join([]seg{nameOf(names, p.Agent), txt(" took "), emphN(p.N), txt(" "), emph(p.Kind), txt(" from ")})
		if p.Owner == p.Agent {
			out = join(out, []seg{txt("their chest")})
		} else {
			out = join(out, []seg{nameOf(names, p.Owner), txt("'s chest")})
		}
		return out, true
	},
	"agent.cooked": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.CookedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			nameOf(names, p.Agent), txt(" cooked "), emphN(p.Produced), txt(" "), emph(p.Kind),
			txt(" at the "), emph(p.Station),
		}), true
	},
	"agent.bathed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.BathedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			nameOf(names, p.Agent), txt(" bathed · morale "), emphN(p.MoraleAfter),
			txt(" warmth "), emphN(p.WarmthAfter),
		}), true
	},
	"agent.refueled": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.RefueledPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" refueled the fire at "), coord(p.X, p.Y)}), true
	},
	"agent.spear_broke": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.SpearBrokePayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt("'s spear broke")}), true
	},
	"agent.ate": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.AtePayload](e)
		if !ok {
			return nil, false
		}
		var parts []string
		if p.Meals > 0 {
			parts = append(parts, fmt.Sprintf("%d meals", p.Meals))
		}
		if p.Cooked > 0 {
			parts = append(parts, fmt.Sprintf("%d cooked", p.Cooked))
		}
		if p.Raw > 0 {
			parts = append(parts, fmt.Sprintf("%d raw", p.Raw))
		}
		breakdown := "nothing"
		if len(parts) > 0 {
			breakdown = strings.Join(parts, ", ")
		}
		return join([]seg{nameOf(names, p.Agent), txt(" ate "), emph(breakdown), txt(" → food "), emphN(p.FoodAfter)}), true
	},
	"agent.slept": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.AgentPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" fell asleep")}), true
	},
	"agent.woke": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.AgentPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" woke")}), true
	},
	// agent.needs_changed: NeedsPayload's actual fields are health/food/rest/
	// warmth/morale (no "water" field — the contract's illustrative example
	// named one that isn't in the struct; this renders the real fields).
	"agent.needs_changed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.NeedsPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" ")}, labeled(
			fmt.Sprintf("health=%d", p.Health), fmt.Sprintf("food=%d", p.Food),
			fmt.Sprintf("rest=%d", p.Rest), fmt.Sprintf("warmth=%d", p.Warmth),
			fmt.Sprintf("morale=%d", p.Morale),
		)), true
	},
	"agent.died": func(e store.Event, names []string) ([]seg, bool) { // alert
		p, ok := decode[sim.DiedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" died: "), emph(p.Cause)}), true
	},
	"agent.talked": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.TalkedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.A), txt(" chatted with "), nameOf(names, p.B)}), true
	},

	// --- agent: mind & plans ---

	"agent.memory_added": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MemoryAddedPayload](e)
		if !ok {
			return nil, false
		}
		out := join([]seg{nameOf(names, p.Agent), txt(" remembers: "), speech(p.Text)})
		if p.Subject >= 0 { // sentinel -1 = no gossip subject (internal/sim/memory.go)
			out = join(out, []seg{txt(" · about "), nameOf(names, p.Subject)})
		}
		return out, true
	},
	"agent.thought": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ThoughtPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" thought: "), speech(p.Text), txt(" (" + p.Source + ")")}), true
	},
	// agent.memory_promoted / agent.memory_faded: the real payload carries
	// TextHash + MemTick, never the memory's text (internal/sim/consolidate.go)
	// — the contract's quoted "{text}" isn't renderable from this payload, so
	// these digests reference the memory by its tick instead.
	"agent.memory_promoted": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MemoryPromotedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt("'s memory (t"), emphI64(p.MemTick), txt(") reinforced")}), true
	},
	"agent.memory_faded": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MemoryFadedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" forgot a memory (t"), emphI64(p.MemTick), txt(")")}), true
	},
	"agent.belief_revised": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.BeliefRevisedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" now believes: "), speech(p.Statement)}), true
	},
	"agent.narrative_set": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.NarrativeSetPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt("'s story: "), speech(p.Text)}), true
	},
	"agent.consolidated": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ConsolidatedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" consolidated the night's memories")}), true
	},
	"agent.plan_set": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.PlanSetPayload](e)
		if !ok {
			return nil, false
		}
		goals := make([]string, len(p.Steps))
		for i, st := range p.Steps {
			goals[i] = st.Goal
		}
		return join([]seg{
			nameOf(names, p.Agent), txt(" planned "), emphN(len(p.Steps)), txt(" steps: "),
			emph(truncateRunes(strings.Join(goals, ", "), 60)),
		}), true
	},
	"agent.plan_step_started": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.PlanStepPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" began step "), emph(p.Step)}), true
	},
	"agent.plan_expired": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.PlanStepPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt("'s plan lapsed ("), emph(p.Reason), txt(")")}), true
	},

	// --- social ---

	"social.conversation_turn": func(e store.Event, names []string) ([]seg, bool) { // speech privilege
		p, ok := decode[sim.ConversationTurnPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Speaker), txt("→"), nameOf(names, p.Listener), txt(" "), speech(p.Text)}), true
	},
	"social.rumor_told": func(e store.Event, names []string) ([]seg, bool) { // speech privilege
		p, ok := decode[sim.RumorToldPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.From), txt("→"), nameOf(names, p.To), txt(" rumor: "), speech(p.Text)}), true
	},
	"social.conversation": func(e store.Event, names []string) ([]seg, bool) { // tones elided (detail pane)
		p, ok := decode[sim.ConversationPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{speech(p.Gist), txt(" · "), emphN(p.Turns), txt(" turns")}), true
	},
	// social.relation_changed: the payload carries two deltas (trust,
	// affection), not the contract's single "{delta:+}" — both render.
	"social.relation_changed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.RelationChangedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			nameOf(names, p.A), txt("→"), nameOf(names, p.B), txt(" "),
			emph(fmt.Sprintf("trust%+d/affection%+d", p.TrustDelta, p.AffectionDelta)),
			txt(" ("), emph(p.Reason), txt(")"),
		}), true
	},
	// social.gave: GavePayload has no amount field (internal/sim/social.go)
	// — the contract's "{n}" isn't renderable; the kind alone renders.
	"social.gave": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GavePayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.From), txt(" gave "), nameOf(names, p.To), txt(" "), emph(p.Kind)}), true
	},
	// social.promise_broken: PromiseBrokenPayload carries only an ID, no
	// from/to (internal/sim/social.go) — the contract's "{from} broke a
	// promise to {to}" isn't renderable from this payload; the id renders.
	"social.promise_broken": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.PromiseBrokenPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("a promise was broken (#"), emphN(p.ID), txt(")")}), true
	},
	"social.secret_seeded": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.SecretSeededPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("a secret took root with "), nameOf(names, p.Agent)}), true
	},
	"social.chest_taken": func(e store.Event, names []string) ([]seg, bool) { // alert
		p, ok := decode[sim.ChestTakenPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Taker), txt(" raided "), nameOf(names, p.Owner), txt("'s chest at "), coord(p.X, p.Y)}), true
	},
	"social.hailed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HailedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.From), txt(" hailed "), nameOf(names, p.To), txt(" (until t"), emphI64(p.Until), txt(")")}), true
	},
	"social.hail_met": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HailMetPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.From), txt(" met "), nameOf(names, p.To)}), true
	},
	"social.hail_expired": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.HailExpiredPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.From), txt("'s hail to "), nameOf(names, p.To), txt(" lapsed")}), true
	},

	// --- governance (meeting.* / norm.*) ---

	// meeting.convened: MeetingPlacePayload carries only the place, no
	// agents list (internal/sim/governance.go) — the contract's "+ agents
	// per payload" isn't renderable from this payload.
	"meeting.convened": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MeetingPlacePayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("meeting convened at "), coord(p.X, p.Y)}), true
	},
	"meeting.opened": func(e store.Event, names []string) ([]seg, bool) {
		return []seg{txt("meeting opened")}, true
	},
	"meeting.turn_taken": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.TurnTakenPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" spoke at the meeting")}), true
	},
	"meeting.proposal_tabled": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ProposalPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Proposer), txt(" proposed: "), speech(p.Text)}), true
	},
	"meeting.proposal_resolved": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ProposalResolvedPayload](e)
		if !ok {
			return nil, false
		}
		outcome := "failed"
		if p.Passed {
			outcome = "passed"
		}
		out := join([]seg{txt("proposal "), emph(outcome), txt(": "), speech(p.Text)})
		if len(p.Yeas)+len(p.Nays) > 0 {
			out = join(out, []seg{txt(" ("), emph(fmt.Sprintf("%d-%d", len(p.Yeas), len(p.Nays))), txt(")")})
		}
		return out, true
	},
	"meeting.proposal_rephrased": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ProposalRephrasedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("norm rephrased: "), speech(p.Text)}), true
	},
	"meeting.closed": func(e store.Event, names []string) ([]seg, bool) {
		return []seg{txt("meeting closed")}, true
	},
	"meeting.place_designated": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MeetingPlacePayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("meeting place set at "), coord(p.X, p.Y)}), true
	},
	"meeting.convention_established": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MeetingConventionPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("meeting convention: "), emph(clock.FormatTOD(p.OpenSecond)), txt(" at "), coord(p.X, p.Y),
			txt(" (" + p.Source + ")"),
		}), true
	},
	// norm.violated: NormViolatedPayload carries NormID, not the norm's text
	// (internal/sim/governance.go) — the contract's quoted "{norm text}"
	// isn't renderable from this payload; the norm id renders instead.
	"norm.violated": func(e store.Event, names []string) ([]seg, bool) { // alert
		p, ok := decode[sim.NormViolatedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Violator), txt(" violated a norm (#"), emphN(p.NormID), txt(")")}), true
	},

	// --- gru / chronicle / metatron ---

	"gru.emerged": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GruEmergedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("the gru emerged at "), coord(p.X, p.Y)}), true
	},
	"gru.moved": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GruMovedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("the gru prowls to "), coord(p.X, p.Y)}), true
	},
	"gru.sighted": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.GruSightedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{nameOf(names, p.Agent), txt(" sighted the gru")}), true
	},
	"gru.attacked": func(e store.Event, names []string) ([]seg, bool) { // alert
		p, ok := decode[sim.GruAttackedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{txt("the gru attacked "), nameOf(names, p.Agent), txt(" · health → "), emphN(p.Health)}), true
	},
	"gru.withdrew": func(e store.Event, names []string) ([]seg, bool) {
		return []seg{txt("the gru withdrew")}, true
	},
	"chronicle.entry": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ChronicleEntryPayload](e)
		if !ok {
			return nil, false
		}
		out := join([]seg{txt("day "), emphI64(p.Day)})
		if p.Thread != "" {
			out = join(out, []seg{txt(" · " + p.Thread)})
		}
		out = join(out, []seg{txt(": "), txt(truncateRunes(p.Text, 80))})
		return out, true
	},
	"metatron.charge_regenerated": func(e store.Event, names []string) ([]seg, bool) {
		return []seg{txt("a charge regenerated")}, true
	},
	"metatron.nudged": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.MetatronNudgedPayload](e)
		if !ok {
			return nil, false
		}
		targets := make([]seg, 0, len(p.Targets)*2)
		for i, t := range p.Targets {
			if i > 0 {
				targets = append(targets, txt(", "))
			}
			targets = append(targets, nameOf(names, t))
		}
		return join([]seg{txt("Metatron "), emph(p.Form), txt(" → ")}, targets, []seg{txt(": "), speech(p.Text)}), true
	},
	// metatron.time_snapped / item_granted / entity_moved / entity_removed
	// (TASK-59, spec 016) predate this contract (specs/018) — no template
	// row exists for them, so voice/style mirrors metatron.nudged's (natural
	// phrase, "Metatron" as subject); gratisMark surfaces the operator force
	// SC-004 requires be enumerable, never silently indistinguishable from a
	// charge-priced miracle.
	"metatron.time_snapped": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.TimeSnappedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("Metatron snapped time forward to "), emph(clock.Format(p.ToTick)),
		}, gratisMark(p.Gratis)), true
	},
	"metatron.item_granted": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.ItemGrantedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("Metatron granted "), nameOf(names, p.Agent), txt(" "), emphN(p.Qty), txt(" "), emph(p.Kind),
		}, gratisMark(p.Gratis)), true
	},
	// entity_moved: the payload identifies its target by class + source
	// coordinates only (internal/sim/miracles.go) — no agent index, so a
	// moved villager renders by its (pre-move) location rather than a
	// resolved name.
	"metatron.entity_moved": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.EntityMovedPayload](e)
		if !ok {
			return nil, false
		}
		return join([]seg{
			txt("Metatron moved the "), emph(p.Class), txt(" at "), coord(p.X, p.Y), txt(" to "), coord(p.ToX, p.ToY),
		}, gratisMark(p.Gratis)), true
	},
	// entity_removed: the payload carries the target's class only, never a
	// structure's Kind (internal/sim/miracles.go) — a removed chest renders
	// as "the structure", not "the chest". A terrain target is overlaid
	// (chop/forage/quarry vocabulary), not deleted, so it reads "cleared"
	// rather than "removed".
	"metatron.entity_removed": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.EntityRemovedPayload](e)
		if !ok {
			return nil, false
		}
		verb := "removed"
		if p.Class == "terrain" {
			verb = "cleared"
		}
		return join([]seg{
			txt("Metatron " + verb + " the "), emph(p.Class), txt(" at "), coord(p.X, p.Y),
		}, gratisMark(p.Gratis)), true
	},

	// --- cog (labeled) ---

	"cog.thought": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.CogThoughtPayload](e)
		if !ok {
			return nil, false
		}
		return labeled(
			"job="+p.Job, "class="+p.Class, "agent="+agentName(names, p.Agent),
			fmt.Sprintf("pts=%d", p.Points), fmt.Sprintf("pred=%dms", p.PredictedWallMs),
		), true
	},
	"cog.outcome": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.CogOutcomePayload](e)
		if !ok {
			return nil, false
		}
		pairs := []string{
			"job=" + p.Job, p.Outcome, "agent=" + agentName(names, p.Agent),
			fmt.Sprintf("stale=%dt", p.StalenessTicks), fmt.Sprintf("wall=%dms", p.ActualWallMs),
		}
		if p.Kind != "" {
			pairs = append(pairs, "kind="+p.Kind)
		}
		if p.Reason != "" {
			pairs = append(pairs, "reason="+p.Reason)
		}
		return labeled(pairs...), true
	},
	"cog.recalibration_recommended": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.RecalibrationPayload](e)
		if !ok {
			return nil, false
		}
		// Post-spec-031 events carry the adoption arithmetic; show
		// prior→adopted when present, else the legacy current estimate.
		est := fmt.Sprintf("est=%.2fs/pt", p.EstimateSPerPt)
		if p.AdoptedSPerPt != 0 || p.PriorSPerPt != 0 {
			est = fmt.Sprintf("est=%.2f→%.2fs/pt", p.PriorSPerPt, p.AdoptedSPerPt)
		}
		return labeled(
			"tier="+p.Tier, est,
			fmt.Sprintf("spikes=%.2f", p.SpikeRate), fmt.Sprintf("window=%d", p.Window),
		), true
	},
	// cog.tool_call: Args and SnapshotTick are deliberately elided — the
	// detail pane bounds them, same reasoning as world.migrated's elided
	// state.
	"cog.tool_call": func(e store.Event, names []string) ([]seg, bool) {
		p, ok := decode[sim.CogToolCallPayload](e)
		if !ok {
			return nil, false
		}
		pairs := []string{
			"job=" + p.Job, fmt.Sprintf("ord=%d", p.Ordinal), "tool=" + p.Tool,
			p.Verdict, "tier=" + p.Tier,
		}
		if p.Reason != "" {
			pairs = append(pairs, "reason="+p.Reason)
		}
		return labeled(pairs...), true
	},
}
