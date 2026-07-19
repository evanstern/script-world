package sim

import (
	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// The placeholder simulation exists only to push real, deterministic events
// through the substrate (log, snapshots, protocol) until TASK-5 systems
// plug in. Two wanderers roam the generated village terrain; day/night at
// 06:00/22:00.
const (
	wandererCount = 2

	nightStartSecond = 22 * 3600 // 22:00
	dayStartSecond   = 6 * 3600  // 06:00
)

// stepEvents is a pure function of (state, map, next tick): the events the
// world produces when advancing to nextTick. It must not mutate s.
func stepEvents(s *State, m *worldmap.Map, nextTick int64) []store.Event {
	var events []store.Event
	emit := func(typ string, payload any) {
		events = append(events, store.Event{Tick: nextTick, Type: typ, Payload: mustPayload(payload)})
	}

	day, _, _, _ := clock.GameTime(nextTick)
	switch clock.SecondOfDay(nextTick) {
	case nightStartSecond:
		emit("sim.night_started", DayPayload{Day: day})
		for i := range s.Wanderers {
			emit("agent.slept", AgentPayload{Agent: i})
		}
	case dayStartSecond:
		emit("sim.day_started", DayPayload{Day: day})
	}

	// Awake wanderers step each game-minute boundary, respecting terrain:
	// water and trees block. The escape clause (any in-bounds step is legal
	// when the current tile is itself impassable) lets agents from saves
	// that predate terrain wade out instead of stranding forever.
	if nextTick%60 == 0 && clock.SecondOfDay(nextTick) != nightStartSecond {
		for i, w := range s.Wanderers {
			if w.Asleep {
				continue
			}
			r := rngAt(s.Seed, "move", nextTick, i)
			x := w.X + int(r.Uint64N(3)) - 1
			y := w.Y + int(r.Uint64N(3)) - 1
			legal := m.Passable(x, y) || (!m.Passable(w.X, w.Y) && m.InBounds(x, y))
			if legal && (x != w.X || y != w.Y) {
				emit("agent.moved", AgentMovedPayload{Agent: i, X: x, Y: y})
			}
		}
	}
	return events
}
