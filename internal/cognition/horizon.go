package cognition

import (
	"fmt"
	"strings"
)

// horizonLadder is the watchable speed ladder (game ticks per real second)
// the operator-facing horizon surfaces summarize against — the same five
// multipliers as clock.CappedLadder's TicksPerSecond values, kept as plain
// float64s here so this leaf package needs no import of internal/clock.
var horizonLadder = []float64{1, 4, 8, 16, 32}

// watchedClasses is the subset of the registry the horizon surfaces name: the
// classes an operator actually watches at speed (spec 035 R5). Order is
// stable across calls (WatchedClasses returns a fresh copy).
var watchedClasses = []string{"planner", "conversation", "meeting"}

// WatchedClasses returns the decision classes the calibration-UX surfaces
// (HorizonSummary, SuppressedAt) summarize or gate on — a fresh copy so a
// caller cannot mutate the doctrine order (spec 035).
func WatchedClasses() []string {
	return append([]string(nil), watchedClasses...)
}

// HorizonSummary evaluates the registry against a fixed seconds-per-point
// across the watchable speed ladder: the operator sees the cognition horizon
// for their hardware before ever running a world. Moved verbatim from
// cmd/promptworld/calibrate.go (spec 035 R1) so the daemon boot warning, the
// set_speed warning, and `promptworld calibrate` all read one implementation
// — the warning may never disagree with the router (FR-006).
func HorizonSummary(secPerPt float64) string {
	parts := []string{}
	for _, class := range watchedClasses {
		dc, ok := ClassFor(class)
		if !ok {
			continue
		}
		maxOK := 0.0
		for _, sp := range horizonLadder {
			if Route(dc, sp, secPerPt).Allow {
				maxOK = sp
			}
		}
		switch {
		case maxOK == 0:
			parts = append(parts, class+" always suppressed")
		case maxOK >= 32:
			parts = append(parts, class+" OK at 32x")
		default:
			parts = append(parts, fmt.Sprintf("%s suppressed above %gx", class, maxOK))
		}
	}
	return strings.Join(parts, "; ")
}

// SuppressedAt returns, from the watched classes, those Route would suppress
// at ticksPerSecond given a per-class seconds-per-point lookup — the same
// arithmetic HorizonSummary applies at one fixed value, callable instead with
// a live per-class estimate (the set_speed warning, spec 035 FR-002/FR-006).
//
// secPerPtFor resolves one class to (secondsPerPoint, ok); ok=false EXCLUDES
// the class from consideration entirely — the seed-state gate (research R2):
// a class whose serving provider is calibrated must never contribute to the
// warning, not merely evaluate as never-suppressed. Order follows
// WatchedClasses.
func SuppressedAt(ticksPerSecond float64, secPerPtFor func(class string) (secPerPt float64, ok bool)) []string {
	var suppressed []string
	for _, class := range watchedClasses {
		dc, ok := ClassFor(class)
		if !ok {
			continue
		}
		secPerPt, ok := secPerPtFor(class)
		if !ok {
			continue
		}
		if !Route(dc, ticksPerSecond, secPerPt).Allow {
			suppressed = append(suppressed, class)
		}
	}
	return suppressed
}
