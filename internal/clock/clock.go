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
	SpeedMax Speed = "max"

	// DefaultSpeed is 4x: 1 game minute per 15 real seconds.
	DefaultSpeed = Speed4x
)

var speeds = map[Speed]float64{
	Speed1x:  1,
	Speed4x:  4,
	Speed8x:  8,
	Speed16x: 16,
	SpeedMax: 0, // sentinel: uncapped
}

func ParseSpeed(s string) (Speed, error) {
	sp := Speed(s)
	if _, ok := speeds[sp]; !ok {
		return "", fmt.Errorf("invalid speed %q (want 1x, 4x, 8x, 16x, or max)", s)
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
