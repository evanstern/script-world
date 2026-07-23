package mind

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// TestRouterEvaluatesAtGovernedSpeed (spec 028 FR-010, US2-AC3): the cognition
// router evaluates against the EFFECTIVE speed the replica reports, so when the
// governor sheds a notch — recorded as a clock.governor_shed the replica applies
// like any event — the router admits a class at the governed speed that it
// refused at the requested speed. Shedding speed widens what the model may own.
func TestRouterEvaluatesAtGovernedSpeed(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	state.Speed = clock.Speed32x
	state.EffectiveRate = clock.Speed32x.TicksPerSecond()

	// A bare Mind is enough for the router: routeVerdict reads only the replica's
	// speed and the seconds-per-point estimate (bootstrap 20 s/pt with no orch).
	md := &Mind{replica: state, m: m}

	// At the requested 32x a planner (3 pt × 20 s/pt × 32 = 1920 ticks) blows its
	// 1200-tick budget, so the router refuses it.
	if v := md.routeVerdict("planner", llm.KindPlanner); v.Allow {
		t.Fatalf("planner admitted at requested 32x: %s", v.Arithmetic)
	}

	// The governor sheds one notch: apply the recorded event exactly as the loop
	// and replay do. The replica now reports the governed effective speed.
	shed := store.Event{Tick: 1, Type: "clock.governor_shed", Payload: governorPayload(t,
		sim.GovernorPayload{Requested: clock.Speed32x, From: clock.Speed32x, To: clock.Speed16x, Debt: 1.4, Jobs: 3})}
	if err := state.Apply(shed); err != nil {
		t.Fatalf("apply governor_shed: %v", err)
	}
	if state.Speed != clock.Speed16x {
		t.Fatalf("replica Speed = %q, want governed 16x", state.Speed)
	}
	if state.RequestedSpeed != clock.Speed32x {
		t.Fatalf("replica RequestedSpeed = %q, want ceiling 32x", state.RequestedSpeed)
	}

	// The router now evaluates at the EFFECTIVE 16x (3 pt × 20 × 16 = 960 ticks
	// ≤ 1200) and admits the planner that 32x refused (FR-010).
	if v := md.routeVerdict("planner", llm.KindPlanner); !v.Allow {
		t.Fatalf("planner still refused at governed 16x: %s", v.Arithmetic)
	}
}

func governorPayload(t *testing.T, p sim.GovernorPayload) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
