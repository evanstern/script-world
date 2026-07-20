package sim

import (
	"encoding/json"
	"fmt"

	"github.com/evanstern/script-world/internal/store"
)

// The chronicle (TASK-11): narrated entries distilled from world events by
// the cloud-tier narrator. Entries are events like everything else — the
// narrator's output enters the world only through the inject_social door —
// and the reducer keeps a bounded ring on State so any attaching client
// receives readable history in the state snapshot. That ring IS the
// catch-up mechanism for the ambient world.

// ChronicleEntryPayload is the chronicle.entry event payload.
type ChronicleEntryPayload struct {
	Day      int64  `json:"day"`
	FromTick int64  `json:"from_tick"`
	ToTick   int64  `json:"to_tick"`
	Text     string `json:"text"`
	Thread   string `json:"thread,omitempty"`
	Agents   []int  `json:"agents,omitempty"`
}

// ChronicleEntry is one narrated entry in the State ring.
type ChronicleEntry struct {
	Tick     int64  `json:"tick"` // when the entry landed
	Day      int64  `json:"day"`
	FromTick int64  `json:"from_tick"`
	ToTick   int64  `json:"to_tick"`
	Text     string `json:"text"`
	Thread   string `json:"thread,omitempty"`
	Agents   []int  `json:"agents,omitempty"`
}

// chronicleCap bounds State.Chronicle; older entries fall off. At the
// narrator's ~2 chapters per game day this holds weeks of story — the event
// log keeps everything forever.
const chronicleCap = 256

func (s *State) applyChronicle(e store.Event) error {
	var p ChronicleEntryPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("apply %s: %w", e.Type, err)
	}
	s.Chronicle = append(s.Chronicle, ChronicleEntry{
		Tick: e.Tick, Day: p.Day, FromTick: p.FromTick, ToTick: p.ToTick,
		Text: p.Text, Thread: p.Thread, Agents: p.Agents,
	})
	if len(s.Chronicle) > chronicleCap {
		s.Chronicle = append(s.Chronicle[:0], s.Chronicle[len(s.Chronicle)-chronicleCap:]...)
	}
	return nil
}

// Mentions reports whether the entry names the agent.
func (c ChronicleEntry) Mentions(agent int) bool {
	for _, a := range c.Agents {
		if a == agent {
			return true
		}
	}
	return false
}
