// Package scribe renders each agent's soul.md from the event stream — the
// player-readable view over event-sourced memories. Always on (souls accrete
// whether or not a world has models); the file is a regenerable view, never
// a source of truth.
package scribe

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

type Scribe struct {
	worldDir string
	replica  *sim.State
	events   chan []store.Event
	done     chan struct{}
}

// New starts the scribe from a state snapshot (canonical JSON, as produced
// by State.Marshal at daemon startup) and renders every soul once.
func New(worldDir string, seed uint64, m *worldmap.Map, stateJSON []byte) (*Scribe, error) {
	replica := sim.NewState(seed, m)
	if err := json.Unmarshal(stateJSON, replica); err != nil {
		return nil, err
	}
	s := &Scribe{
		worldDir: worldDir,
		replica:  replica,
		events:   make(chan []store.Event, 256),
		done:     make(chan struct{}),
	}
	for i := range s.replica.Agents {
		s.render(i)
	}
	go s.run()
	return s, nil
}

// Observe is the loop's notify callback path: never blocks. On overflow,
// batches are dropped — souls lag until the next memory event re-renders
// from the (complete) replica... which requires the replica to be complete,
// so overflow instead marks the batch lost and the replica is rebuilt lazily
// via the full memory list carried in later renders. In practice the 256
// buffer far exceeds burst sizes.
func (s *Scribe) Observe(events []store.Event) {
	select {
	case s.events <- events:
	default:
	}
}

func (s *Scribe) Close() { close(s.done) }

func (s *Scribe) run() {
	for {
		select {
		case <-s.done:
			return
		case batch := <-s.events:
			dirty := map[int]bool{}
			for _, e := range batch {
				s.replica.Apply(e)
				if e.Tick > s.replica.Tick {
					s.replica.Tick = e.Tick
				}
				switch e.Type {
				case "agent.memory_added", "agent.died":
					var p struct {
						Agent int `json:"agent"`
					}
					if json.Unmarshal(e.Payload, &p) == nil {
						dirty[p.Agent] = true
					}
				}
			}
			for idx := range dirty {
				s.render(idx)
			}
		}
	}
}

// render writes one agent's soul.md from replica state.
func (s *Scribe) render(idx int) {
	if idx < 0 || idx >= len(s.replica.Agents) {
		return
	}
	a := s.replica.Agents[idx]
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — soul\n\n", a.Name)
	status := "Alive"
	if a.Dead {
		status = "Dead"
	}
	fmt.Fprintf(&b, "*Born day 1. %s. %d memories.*\n\n", status, len(a.Memories))
	if len(a.Memories) == 0 {
		b.WriteString("*No memories yet.*\n")
	}
	for _, m := range a.Memories {
		fmt.Fprintf(&b, "- **%s** (%d★) %s\n", clock.Format(m.Tick), m.Salience, m.Text)
	}
	os.WriteFile(persona.SoulPath(s.worldDir, a.Name), []byte(b.String()), 0o644)
}
