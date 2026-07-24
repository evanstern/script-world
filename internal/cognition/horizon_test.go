package cognition

import (
	"strings"
	"testing"
)

// TestHorizonSummaryBootstrap (spec 035 T003): at the bootstrap local seed
// (20 s/pt) the summary matches contracts/warnings.md §1's worked example —
// planner and conversation suppressed above 16x, meeting OK through 32x.
func TestHorizonSummaryBootstrap(t *testing.T) {
	got := HorizonSummary(BootstrapLocalSecPerPt)
	for _, want := range []string{
		"planner suppressed above 16x",
		"conversation suppressed above 16x",
		"meeting OK at 32x",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HorizonSummary(%.1f) = %q, missing %q", BootstrapLocalSecPerPt, got, want)
		}
	}
}

// TestHorizonSummaryCalibrated: a fast, calibrated rig (0.94 s/pt) clears
// every watched class at the ladder ceiling.
func TestHorizonSummaryCalibrated(t *testing.T) {
	got := HorizonSummary(0.94)
	for _, class := range []string{"planner", "conversation", "meeting"} {
		if !strings.Contains(got, class+" OK at 32x") {
			t.Errorf("HorizonSummary(0.94) = %q, want %q OK at 32x", got, class)
		}
	}
}

// TestHorizonSummaryAgreesWithRoute: the summary's suppressed/OK verdicts
// agree with Route across the full ladder, at both bootstrap and calibrated
// seconds-per-point — the property FR-006 demands structurally.
func TestHorizonSummaryAgreesWithRoute(t *testing.T) {
	for _, secPerPt := range []float64{BootstrapLocalSecPerPt, BootstrapCloudSecPerPt, 0.94, 5.0} {
		summary := HorizonSummary(secPerPt)
		for _, class := range watchedClasses {
			dc, ok := ClassFor(class)
			if !ok {
				t.Fatalf("watched class %q not registered", class)
			}
			allowedAt32 := Route(dc, 32, secPerPt).Allow
			saysOKAt32 := strings.Contains(summary, class+" OK at 32x")
			if allowedAt32 != saysOKAt32 {
				t.Errorf("secPerPt=%.2f class=%s: Route.Allow@32x=%v but summary OK-at-32x=%v (%q)",
					secPerPt, class, allowedAt32, saysOKAt32, summary)
			}
		}
	}
}

// TestSuppressedAtAgreesWithRoute (spec 035 contracts/warnings.md Test
// obligations): SuppressedAt says a class is suppressed exactly when Route
// disallows it at that (ticksPerSecond, secPerPt) point, across the ladder ×
// watched-class matrix.
func TestSuppressedAtAgreesWithRoute(t *testing.T) {
	perClassSecPerPt := map[string]float64{
		"planner":      BootstrapLocalSecPerPt,
		"conversation": BootstrapLocalSecPerPt,
		"meeting":      BootstrapLocalSecPerPt,
	}
	lookup := func(class string) (float64, bool) {
		sp, ok := perClassSecPerPt[class]
		return sp, ok
	}
	for _, tps := range horizonLadder {
		suppressed := SuppressedAt(tps, lookup)
		suppressedSet := make(map[string]bool, len(suppressed))
		for _, c := range suppressed {
			suppressedSet[c] = true
		}
		for _, class := range watchedClasses {
			dc, _ := ClassFor(class)
			wantSuppressed := !Route(dc, tps, perClassSecPerPt[class]).Allow
			if suppressedSet[class] != wantSuppressed {
				t.Errorf("tps=%g class=%s: SuppressedAt says suppressed=%v, Route says suppressed=%v",
					tps, class, suppressedSet[class], wantSuppressed)
			}
		}
	}
}

// TestSuppressedAtGateExcludesClass: secPerPtFor returning ok=false (the
// seed-state gate — a calibrated provider) removes the class from the result
// entirely, even at a speed where the bootstrap arithmetic would suppress it.
func TestSuppressedAtGateExcludesClass(t *testing.T) {
	got := SuppressedAt(32, func(class string) (float64, bool) {
		return 0, false // every class gated out (calibrated world)
	})
	if len(got) != 0 {
		t.Errorf("gated-out lookup must yield no suppressed classes, got %v", got)
	}
}
