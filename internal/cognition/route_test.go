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
		{"musing", 32, true},       // 544 <= 3600
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

func TestRouteUncappedSuppresses(t *testing.T) {
	dc, _ := ClassFor("musing")
	if v := Route(dc, 0, 1.0); v.Allow {
		t.Error("uncapped speed must suppress")
	}
}
