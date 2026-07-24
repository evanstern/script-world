package cognition

import "testing"

// TestRouteSanityTable pins the contract's speed ladder at the measured
// ~17 s/pt local baseline (contracts/registry.md).
func TestRouteSanityTable(t *testing.T) {
	const secPerPt = 17.0
	cases := []struct {
		class string
		speed float64
		allow bool
	}{
		{"planner", 1, true},
		{"planner", 4, true},
		{"planner", 16, true},      // 816 <= 1200
		{"planner", 32, false},     // 1632 > 1200
		{"conversation", 32, true}, // 7072 <= 7200
		{"meeting", 32, true},      // 1088 <= 3600
		{"consolidation", 32, true},
		{"chronicle", 32, true},
	}
	for _, c := range cases {
		dc, ok := ClassFor(c.class)
		if !ok {
			t.Fatalf("class %q missing", c.class)
		}
		v := Route(dc, c.speed, secPerPt)
		if v.Allow != c.allow {
			t.Errorf("Route(%s, %gx, %g) = allow %v (%s), want %v",
				c.class, c.speed, secPerPt, v.Allow, v.Arithmetic, c.allow)
		}
	}
}

func TestRouteIsPure(t *testing.T) {
	dc, _ := ClassFor("planner")
	a := Route(dc, 32, 17.0)
	for i := 0; i < 100; i++ {
		if b := Route(dc, 32, 17.0); b != a {
			t.Fatalf("verdict varied: %+v vs %+v", a, b)
		}
	}
}

func TestRouteSuppressionCarriesArithmetic(t *testing.T) {
	dc, _ := ClassFor("planner")
	v := Route(dc, 32, 17.0)
	if v.Allow {
		t.Fatal("expected suppression")
	}
	if v.PredictedDriftTicks != 1632 || v.BudgetTicks != 1200 {
		t.Errorf("arithmetic fields: drift %d budget %d", v.PredictedDriftTicks, v.BudgetTicks)
	}
	if v.Arithmetic == "" {
		t.Error("empty arithmetic string")
	}
}

// TestRouteTruthfulAfterAdoption (spec 031 SC-003, unit level): once the
// estimator has adopted the world-01 load regime (~12 s/pt) the router's
// arithmetic is honest, where the frozen seed (0.52 s/pt) hid every cost. For
// the planner class (3pt, budget 1200 ticks) the predicted drift is
// points x s/pt x ticksPerSecond; verified here against route.go directly.
func TestRouteTruthfulAfterAdoption(t *testing.T) {
	dc, ok := ClassFor("planner")
	if !ok {
		t.Fatal("planner class missing")
	}
	const adopted = 12.0
	// Frozen seed: predicts ~1.6 s wall (3 x 0.52) and admits everything —
	// the defect. Adopted: predicts 36 s wall, the truth.
	if v := Route(dc, 32, 0.52); v.PredictedWallMs != 1560 || !v.Allow {
		t.Errorf("frozen seed verdict: wall=%dms allow=%v (%s)", v.PredictedWallMs, v.Allow, v.Arithmetic)
	}
	// Adopted, admitted: 3pt x 12 s/pt x 8 t/s = 288 ticks <= 1200.
	if v := Route(dc, 8, adopted); !v.Allow || v.PredictedDriftTicks != 288 {
		t.Errorf("adopted @8t/s: allow=%v drift=%d (%s); want admit at 288 ticks", v.Allow, v.PredictedDriftTicks, v.Arithmetic)
	}
	// Adopted, suppressed: at a speed where true drift exceeds the budget the
	// thought is suppressed PRE-dispatch. 3pt x 12 s/pt = 36 s wall; drift
	// crosses 1200 ticks above ~33.3 t/s, so 40 t/s -> 1440 ticks > 1200.
	if v := Route(dc, 40, adopted); v.Allow || v.PredictedDriftTicks != 1440 {
		t.Errorf("adopted @40t/s: allow=%v drift=%d (%s); want suppress at 1440 ticks", v.Allow, v.PredictedDriftTicks, v.Arithmetic)
	}
}

func TestRouteUncappedSuppresses(t *testing.T) {
	dc, _ := ClassFor("planner")
	if v := Route(dc, 0, 1.0); v.Allow {
		t.Error("uncapped speed must suppress")
	}
}

// TestNoLowSpeedRegression (SC-006): at 1x and the default 4x, every
// registered class routes to the model under both the pessimistic bootstrap
// (20 s/pt) and the measured local baseline (~17 s/pt) — the horizon changes
// nothing at watchable-low speeds.
func TestNoLowSpeedRegression(t *testing.T) {
	for name := range registry {
		dc, _ := ClassFor(name)
		for _, spp := range []float64{BootstrapLocalSecPerPt, 17.0} {
			for _, speed := range []float64{1, 4} {
				if v := Route(dc, speed, spp); !v.Allow {
					t.Errorf("class %s suppressed at %gx / %g s/pt (%s)", name, speed, spp, v.Arithmetic)
				}
			}
		}
	}
}
