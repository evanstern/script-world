package sim

import (
	"encoding/json"
	"fmt"
	"hash/fnv"

	"github.com/evanstern/promptworld/internal/store"
)

// Nightly consolidation substrate (TASK-9): the model-free half. Beliefs,
// narrative, and the once-per-night ledger are ordinary event-sourced state;
// the mind's consolidation driver lands its output exclusively through the
// whitelisted injection door as one atomic batch. Replay never consults a
// model — these reducer cases are total: a payload whose target has vanished
// degrades to a no-op, never an error.

// Belief is a durable conviction, revisable by later consolidations.
type Belief struct {
	ID         int    `json:"id"`
	Statement  string `json:"statement"`
	Confidence int    `json:"confidence"` // 0..100
	Provenance string `json:"provenance"` // "witnessed" | "told" | "inferred"
	Source     int    `json:"source"`     // teller for "told"; -1 otherwise
	Subject    int    `json:"subject"`    // agent the belief is about; -1 = world
	Tick       int64  `json:"tick"`       // last revision
	// Reinforced (spec 030) is the game tick the belief was last anchored to a
	// direct observation — set at formation, refreshed by a direct-evidence
	// revision or a reinforcement event. The decay clock (US2) reads it; 0 =
	// legacy grandfather (no decay). omitempty keeps pre-030 beliefs byte-stable.
	Reinforced int64 `json:"reinforced,omitempty"`
}

const (
	ProvenanceWitnessed = "witnessed"
	ProvenanceTold      = "told"
	ProvenanceInferred  = "inferred"
)

// ConsolidationGapTicks is the secondary once-per-night guard: 12 game-hours
// must separate markers, closing the post-midnight-sleep double-dip (a
// starving agent dozing at 01:00 maps to the next night index).
const ConsolidationGapTicks = 43200

// MaxSalience caps promotion boosts.
const MaxSalience = 10

// SalDayGist is the salience of the nightly day-gist memory: durable but
// below landmark events.
const SalDayGist = 6

// Consolidation outcomes recorded in the agent.consolidated marker.
const (
	ConsolidationAccepted     = "accepted"
	ConsolidationRejected     = "rejected"
	ConsolidationSkippedEmpty = "skipped_empty"
)

// NightIndex is the 1-based game night a tick belongs to (day 1 = night 1);
// 0 in LastConsolidatedNight therefore means "never consolidated", which
// keeps genesis and pre-TASK-9 snapshots (field absent → 0) correct.
func NightIndex(tick int64) int64 { return tick/86400 + 1 }

// MemoryHash identifies a memory by content for promote/fade references —
// memories have no IDs and slice indexes are unstable under append.
func MemoryHash(text string) string {
	h := fnv.New32a()
	h.Write([]byte(text))
	return fmt.Sprintf("%08x", h.Sum32())
}

// EpisodicBuffer is the un-consolidated tail: memories accumulated since the
// agent's last accepted consolidation, in tick order (Memories is append-
// ordered already).
func (a *Agent) EpisodicBuffer() []Memory {
	var out []Memory
	for _, m := range a.Memories {
		if m.Tick > a.ConsolidatedUpTo {
			out = append(out, m)
		}
	}
	return out
}

// ConsolidationDue reports whether sleeping at tick should trigger a
// consolidation attempt for this agent (night + gap guards; liveness and
// buffer emptiness are the caller's concern — an empty buffer is still "due"
// so the skipped_empty marker can close the night).
func (a *Agent) ConsolidationDue(tick int64) bool {
	return !a.Dead &&
		NightIndex(tick) > a.LastConsolidatedNight &&
		(a.LastConsolidateMark == 0 || tick-a.LastConsolidateMark >= ConsolidationGapTicks)
}

// --- event payloads (all landed via the whitelisted injection door) ---

type MemoryPromotedPayload struct {
	Agent    int    `json:"agent"`
	MemTick  int64  `json:"mem_tick"`
	TextHash string `json:"text_hash"`
	Boost    int    `json:"boost"`
}

type MemoryFadedPayload struct {
	Agent    int    `json:"agent"`
	MemTick  int64  `json:"mem_tick"`
	TextHash string `json:"text_hash"`
}

// MemoryRef is a memory's durable identity — the (tick, content-hash) pair that
// promote/fade references already resolve to. Spec 030 cites belief evidence
// with it so replay reads recorded identities and never re-resolves ordinals.
type MemoryRef struct {
	Tick int64  `json:"tick"`
	Hash string `json:"hash"`
}

type BeliefRevisedPayload struct {
	Agent      int    `json:"agent"`
	BeliefID   int    `json:"belief_id"` // 0 = new belief
	Statement  string `json:"statement"`
	Confidence int    `json:"confidence"`
	Provenance string `json:"provenance"`
	Source     int    `json:"source"`
	Subject    int    `json:"subject"`
	// Evidence (spec 030) is the resolved durable identities of the memories the
	// belief cites; Direct is whether >=1 of them is direct perception — both
	// derived by the validator BEFORE landing so replay never re-classifies.
	// Direct drives the revision-time reinforcement refresh (US2). Both omitempty
	// so a pre-030 belief_revised event replays byte-identically.
	Evidence []MemoryRef `json:"evidence,omitempty"`
	Direct   bool        `json:"direct,omitempty"`
}

type NarrativeSetPayload struct {
	Agent int    `json:"agent"`
	Text  string `json:"text"`
}

type ConsolidatedPayload struct {
	Agent    int    `json:"agent"`
	Night    int64  `json:"night"`
	UpTo     int64  `json:"up_to"` // buffer high-water mark; meaningful on accept
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason,omitempty"`
	Promoted int    `json:"promoted,omitempty"`
	Faded    int    `json:"faded,omitempty"`
	Beliefs  int    `json:"beliefs,omitempty"`
	// Coerced (spec 030) counts beliefs whose provenance the validator downgraded
	// from "witnessed" for lack of direct-perception evidence — non-fatal
	// telemetry, never a rejection. omitempty keeps pre-030 markers byte-stable.
	Coerced int     `json:"coerced,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// applyConsolidation is the reducer arm for the five consolidation event
// types, dispatched from State.Apply.
func (s *State) applyConsolidation(e store.Event) error {
	agent := func(i int) (*Agent, error) {
		if i < 0 || i >= len(s.Agents) {
			return nil, fmt.Errorf("apply %s: agent %d out of range", e.Type, i)
		}
		return &s.Agents[i], nil
	}
	switch e.Type {
	case "agent.memory_promoted":
		var p MemoryPromotedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		for i := range a.Memories {
			m := &a.Memories[i]
			if m.Tick == p.MemTick && MemoryHash(m.Text) == p.TextHash {
				m.Salience += p.Boost
				if m.Salience > MaxSalience {
					m.Salience = MaxSalience
				}
				break
			}
		} // vanished target: no-op

	case "agent.memory_faded":
		var p MemoryFadedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		for i := range a.Memories {
			if a.Memories[i].Tick == p.MemTick && MemoryHash(a.Memories[i].Text) == p.TextHash {
				a.Memories = append(a.Memories[:i], a.Memories[i+1:]...)
				break
			}
		} // vanished target: no-op

	case "agent.belief_revised":
		var p BeliefRevisedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		conf := p.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 100 {
			conf = 100
		}
		if p.BeliefID == 0 {
			if s.NextBeliefID == 0 {
				s.NextBeliefID = 1
			}
			// Formation always anchors the decay clock to now (spec 030 normative
			// note): the curve starts at formation for every new belief, direct or
			// not. Subsequent direct-evidence revisions refresh it (US2, T006).
			a.Beliefs = append(a.Beliefs, Belief{
				ID: s.NextBeliefID, Statement: p.Statement, Confidence: conf,
				Provenance: p.Provenance, Source: p.Source, Subject: p.Subject,
				Tick: e.Tick, Reinforced: e.Tick,
			})
			s.NextBeliefID++
		} else {
			for i := range a.Beliefs {
				if a.Beliefs[i].ID == p.BeliefID {
					a.Beliefs[i].Statement = p.Statement
					a.Beliefs[i].Confidence = conf
					a.Beliefs[i].Provenance = p.Provenance
					a.Beliefs[i].Source = p.Source
					a.Beliefs[i].Subject = p.Subject
					a.Beliefs[i].Tick = e.Tick
					break
				}
			} // unknown ID: no-op
		}

	case "agent.narrative_set":
		var p NarrativeSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Narrative = p.Text

	case "agent.consolidated":
		var p ConsolidatedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if p.Night > a.LastConsolidatedNight {
			a.LastConsolidatedNight = p.Night
		}
		a.LastConsolidateMark = e.Tick
		if p.Outcome == ConsolidationAccepted && p.UpTo > a.ConsolidatedUpTo {
			a.ConsolidatedUpTo = p.UpTo
		}
	}
	return nil
}
