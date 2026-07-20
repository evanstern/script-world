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
				case "social.relation_changed":
					var p sim.RelationChangedPayload
					if json.Unmarshal(e.Payload, &p) == nil {
						dirty[p.A] = true
					}
				case "social.gave", "social.promise_broken":
					for i := range s.replica.Agents {
						dirty[i] = true
					}
				case "agent.memory_added", "agent.died",
					"agent.memory_promoted", "agent.memory_faded",
					"agent.belief_revised", "agent.narrative_set",
					"agent.consolidated":
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

	// Who I am becoming: the consolidated self-narrative (TASK-9), the
	// player's first read.
	if a.Narrative != "" {
		fmt.Fprintf(&b, "## Who I am becoming\n\n%s\n\n", a.Narrative)
	}

	if len(a.Memories) == 0 {
		b.WriteString("*No memories yet.*\n")
	}
	for _, m := range a.Memories {
		fmt.Fprintf(&b, "- **%s** (%d★) %s\n", clock.Format(m.Tick), m.Salience, m.Text)
	}

	// Beliefs: durable convictions with confidence and provenance (TASK-9).
	if len(a.Beliefs) > 0 {
		b.WriteString("\n## Beliefs\n\n")
		for _, bl := range a.Beliefs {
			src := ""
			switch {
			case bl.Provenance == sim.ProvenanceTold && bl.Source >= 0 && bl.Source < len(s.replica.Agents):
				src = fmt.Sprintf("told by %s", s.replica.Agents[bl.Source].Name)
			default:
				src = bl.Provenance
			}
			fmt.Fprintf(&b, "- %s *(%d%% sure — %s, %s)*\n", bl.Statement, bl.Confidence, src, clock.Format(bl.Tick))
		}
	}

	// Bonds: the social fabric as this soul feels it.
	var bonds []string
	for _, r := range s.replica.Relations {
		if r.From != idx || (r.Trust == 0 && r.Affection == 0) {
			continue
		}
		bonds = append(bonds, fmt.Sprintf("- %s: trust %+d, affection %+d",
			s.replica.Agents[r.To].Name, r.Trust, r.Affection))
	}
	var debts []string
	for _, d := range s.replica.Debts {
		if d.Status == "open" && d.Debtor == idx {
			debts = append(debts, fmt.Sprintf("- owes %s one %s (due %s)",
				s.replica.Agents[d.Creditor].Name, d.Kind, clock.Format(d.Due)))
		}
	}
	if len(bonds)+len(debts) > 0 {
		fmt.Fprintf(&b, "\n## Bonds\n\n*Reputation %d/1000.*\n\n", sim.Reputation(s.replica, idx))
		for _, l := range append(bonds, debts...) {
			b.WriteString(l + "\n")
		}
	}
	os.WriteFile(persona.SoulPath(s.worldDir, a.Name), []byte(b.String()), 0o644)
}
