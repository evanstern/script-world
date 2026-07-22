package clock

import (
	"testing"
	"time"
)

func TestParseSpeed(t *testing.T) {
	for _, valid := range []string{"1x", "4x", "8x", "16x", "32x", "max"} {
		if _, err := ParseSpeed(valid); err != nil {
			t.Errorf("ParseSpeed(%q): unexpected error %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "2x", "fast", "0", "MAX"} {
		if _, err := ParseSpeed(invalid); err == nil {
			t.Errorf("ParseSpeed(%q): expected error", invalid)
		}
	}
}

func TestInterval(t *testing.T) {
	cases := []struct {
		speed Speed
		want  time.Duration
	}{
		{Speed1x, time.Second},
		{Speed4x, 250 * time.Millisecond},
		{Speed8x, 125 * time.Millisecond},
		{Speed16x, 62500 * time.Microsecond},
		{Speed32x, 31250 * time.Microsecond},
		{SpeedMax, 0},
	}
	for _, c := range cases {
		if got := c.speed.Interval(); got != c.want {
			t.Errorf("%s.Interval() = %v, want %v", c.speed, got, c.want)
		}
	}
	if DefaultSpeed != Speed4x {
		t.Errorf("DefaultSpeed = %s, want 4x (1 game-min per 15 real-sec)", DefaultSpeed)
	}
}

func TestGameTime(t *testing.T) {
	cases := []struct {
		tick    int64
		day     int64
		h, m, s int
	}{
		{0, 1, 6, 0, 0},                     // genesis: day 1 06:00
		{59, 1, 6, 0, 59},                   // sub-minute
		{60, 1, 6, 1, 0},                    // first minute boundary
		{16 * 3600, 1, 22, 0, 0},            // night start day 1
		{18 * 3600, 2, 0, 0, 0},             // midnight rollover
		{24 * 3600, 2, 6, 0, 0},             // one full game day
		{29*24*3600 + 24*3600, 31, 6, 0, 0}, // day 31 (a 30-day run's end)
	}
	for _, c := range cases {
		day, h, m, s := GameTime(c.tick)
		if day != c.day || h != c.h || m != c.m || s != c.s {
			t.Errorf("GameTime(%d) = day %d %02d:%02d:%02d, want day %d %02d:%02d:%02d",
				c.tick, day, h, m, s, c.day, c.h, c.m, c.s)
		}
	}
}

func TestSecondOfDayBoundaries(t *testing.T) {
	if SecondOfDay(16*3600) != 22*3600 {
		t.Error("tick 16h should land on 22:00 (night start)")
	}
	if SecondOfDay(24*3600) != 6*3600 {
		t.Error("tick 24h should land on 06:00 (day start)")
	}
}

func TestFormat(t *testing.T) {
	if got := Format(0); got != "day 1 06:00" {
		t.Errorf("Format(0) = %q, want %q", got, "day 1 06:00")
	}
	if got := Format(16*3600 + 90); got != "day 1 22:01" {
		t.Errorf("Format = %q, want %q", got, "day 1 22:01")
	}
}

func TestTickAtInvertsGameTime(t *testing.T) {
	// TickAt is the exact inverse of GameTime for the display resolution
	// (day/hour/minute; seconds round-trip too).
	for _, tick := range []int64{0, 59, 60, 16 * 3600, 18 * 3600, 24 * 3600, 106200} {
		day, h, m, s := GameTime(tick)
		if got := TickAt(day, h, m, s); got != tick {
			t.Errorf("TickAt(GameTime(%d)) = %d, want %d", tick, got, tick)
		}
	}
	// Known coordinates: genesis and "day 2 11:30" (the quickstart target).
	if got := TickAt(1, 6, 0, 0); got != 0 {
		t.Errorf("TickAt(day1 06:00) = %d, want 0 (genesis)", got)
	}
	if got := TickAt(2, 11, 30, 0); got != 106200 {
		t.Errorf("TickAt(day2 11:30) = %d, want 106200", got)
	}
}

func TestParseTimeOfDay(t *testing.T) {
	cases := []struct {
		in   string
		h, m int
	}{
		{"11:30", 11, 30}, {"00:00", 0, 0}, {"23:59", 23, 59}, {"6:05", 6, 5},
	}
	for _, c := range cases {
		h, m, err := ParseTimeOfDay(c.in)
		if err != nil || h != c.h || m != c.m {
			t.Errorf("ParseTimeOfDay(%q) = %d:%d, %v; want %d:%d", c.in, h, m, err, c.h, c.m)
		}
	}
	for _, bad := range []string{"", "1130", "24:00", "11:60", "-1:00", "noon", "11:"} {
		if _, _, err := ParseTimeOfDay(bad); err == nil {
			t.Errorf("ParseTimeOfDay(%q): expected error", bad)
		}
	}
}
