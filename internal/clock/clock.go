// Package clock owns the game-time substrate: 1 tick = 1 game second, with a
// speed multiplier mapping game time to real time. Game epoch is day 1, 06:00.
package clock

import (
	"fmt"
	"time"
)

// TickGameSeconds is fixed at 1 in format_version 1; recorded in the world
// manifest so it can never silently change under an existing run.
const TickGameSeconds = 1

const (
	secondsPerMinute = 60
	secondsPerHour   = 3600
	secondsPerDay    = 86400
	// EpochSecondOfDay is the time of day at tick 0: 06:00 on day 1.
	EpochSecondOfDay = 6 * secondsPerHour
)

// Speed is the requested compression: game seconds per real second.
// SpeedMax means "as fast as affordable" (uncapped).
type Speed string

const (
	Speed1x  Speed = "1x"
	Speed4x  Speed = "4x"
	Speed8x  Speed = "8x"
	Speed16x Speed = "16x"
	// Speed32x is the top of the watchable ladder: the fastest pace LLM
	// minds can be expected to keep up with (TASK-20).
	Speed32x Speed = "32x"
	// SpeedMax is reserved for pure-sim worlds (headless proving runs);
	// worlds with an LLM configured refuse it at the set_speed door.
	SpeedMax Speed = "max"

	// DefaultSpeed is 4x: 1 game minute per 15 real seconds.
	DefaultSpeed = Speed4x
)

var speeds = map[Speed]float64{
	Speed1x:  1,
	Speed4x:  4,
	Speed8x:  8,
	Speed16x: 16,
	Speed32x: 32,
	SpeedMax: 0, // sentinel: uncapped
}

// cappedLadder is the watchable speed ladder the adaptive-throttle governor
// (spec 028) moves the effective speed along: the five finite multipliers in
// ascending order, excluding the uncapped SpeedMax (which the governor never
// touches — FR-004/FR-012). 1x is the hard floor, 32x the ceiling.
var cappedLadder = []Speed{Speed1x, Speed4x, Speed8x, Speed16x, Speed32x}

// CappedLadder returns the governor's speed ladder in ascending order
// (1x…32x, SpeedMax excluded) — a fresh copy so callers cannot mutate the
// doctrine order.
func CappedLadder() []Speed {
	return append([]Speed(nil), cappedLadder...)
}

// LadderIndex is the position of s on the capped ladder (0 = the 1x floor,
// 4 = the 32x ceiling), or -1 when s is off the ladder (SpeedMax or an unknown
// value). Governor validation uses it to require decisions land exactly one
// notch apart in the implied direction.
func LadderIndex(s Speed) int {
	for i, sp := range cappedLadder {
		if sp == s {
			return i
		}
	}
	return -1
}

func ParseSpeed(s string) (Speed, error) {
	sp := Speed(s)
	if _, ok := speeds[sp]; !ok {
		return "", fmt.Errorf("invalid speed %q (want 1x, 4x, 8x, 16x, 32x, or max)", s)
	}
	return sp, nil
}

// TicksPerSecond is the requested tick rate; 0 means uncapped (max).
func (s Speed) TicksPerSecond() float64 { return speeds[s] }

// Interval is the real-time budget per tick; 0 means uncapped (max).
func (s Speed) Interval() time.Duration {
	tps := speeds[s]
	if tps == 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / tps)
}

// GameTime converts a tick ordinal to calendar coordinates.
// Days are 1-based; tick 0 is day 1, 06:00:00.
func GameTime(tick int64) (day int64, hour, min, sec int) {
	abs := tick*TickGameSeconds + EpochSecondOfDay
	day = abs/secondsPerDay + 1
	rem := abs % secondsPerDay
	return day, int(rem / secondsPerHour), int(rem % secondsPerHour / secondsPerMinute), int(rem % secondsPerMinute)
}

// SecondOfDay is the game time of day in seconds for a tick.
func SecondOfDay(tick int64) int64 {
	return (tick*TickGameSeconds + EpochSecondOfDay) % secondsPerDay
}

// Format renders a tick as the display form used everywhere: "day N HH:MM".
func Format(tick int64) string {
	day, h, m, _ := GameTime(tick)
	return fmt.Sprintf("day %d %02d:%02d", day, h, m)
}

// FormatTOD renders a second-of-day as the clock time "HH:MM".
func FormatTOD(sec int) string {
	return fmt.Sprintf("%02d:%02d", sec/secondsPerHour, sec%secondsPerHour/secondsPerMinute)
}

// TickAt is the inverse of GameTime: the tick ordinal for game-calendar
// coordinates (1-based day, hour, minute, second). The miracle snap doors use
// it to translate an operator/angel "day N HH:MM" target into the tick the
// reducer snaps to (spec 016 US3). It does not judge direction — the clock is
// monotonic and forward-only, and the reducer rejects a non-forward target.
func TickAt(day int64, hour, min, sec int) int64 {
	abs := (day-1)*secondsPerDay + int64(hour)*secondsPerHour + int64(min)*secondsPerMinute + int64(sec)
	return (abs - EpochSecondOfDay) / TickGameSeconds
}

// ParseTimeOfDay parses a "HH:MM" clock label into hour and minute, validating
// the 24-hour range. The miracle snap door pairs it with a day number and TickAt.
func ParseTimeOfDay(s string) (hour, min int, err error) {
	var h, m int
	if n, serr := fmt.Sscanf(s, "%d:%d", &h, &m); serr != nil || n != 2 {
		return 0, 0, fmt.Errorf("invalid time %q (want HH:MM)", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time %q (hour 0-23, minute 0-59)", s)
	}
	return h, m, nil
}
