package sim

// Guards (TASK-32, FR-011): the assumptions an intent or plan step was
// formed under, re-validated deterministically against current State at
// landing/execution. The vocabulary is a closed set — the model may pick
// guards, never author predicates. Evaluation is a pure function of State:
// no model, no randomness, no wall clock.

import "fmt"

// Guard types (v1, closed set — contracts/events.md).
const (
	GuardTargetAlive   = "target_alive"
	GuardTargetPresent = "target_present" // adapt-rung repairable: resolveGoal re-resolves
	GuardNotSuperseded = "not_superseded" // generation equality (FR-014)
	GuardAfterTick     = "after_tick"
	GuardBeforeTick    = "before_tick"
)

// Guard is one deterministic assumption. Target is an agent index for the
// target_* types; Tick is the boundary for the timed types; Generation is
// the snapshot generation for not_superseded.
type Guard struct {
	Type       string `json:"type"`
	Target     int    `json:"target,omitempty"`
	Tick       int64  `json:"tick,omitempty"`
	Generation int64  `json:"generation,omitempty"`
	// X, Y record the target's position at snapshot time (target_present):
	// a landing whose target holds the guard but moved is reported adapted,
	// not landed — the repair was resolveGoal's re-resolution.
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

// presentRadius bounds target_present: the target must be within this
// Manhattan distance for the assumption "I can reach them" to hold. Wide on
// purpose — a target who merely walked off is the adapt rung's job
// (re-resolve), not a rejection; one who left the area is a world change.
const presentRadius = 16

// Eval checks one guard against current state for the acting agent.
// Returns (holds, reason) — reason is recorded on rejection.
func (g Guard) Eval(s *State, agent int) (bool, string) {
	switch g.Type {
	case GuardTargetAlive:
		if g.Target < 0 || g.Target >= len(s.Agents) {
			return false, fmt.Sprintf("target %d out of range", g.Target)
		}
		if s.Agents[g.Target].Dead {
			return false, s.Agents[g.Target].Name + " is dead"
		}
		return true, ""
	case GuardTargetPresent:
		if g.Target < 0 || g.Target >= len(s.Agents) {
			return false, fmt.Sprintf("target %d out of range", g.Target)
		}
		t := &s.Agents[g.Target]
		if t.Dead {
			return false, t.Name + " is dead"
		}
		a := &s.Agents[agent]
		if d := abs(a.X-t.X) + abs(a.Y-t.Y); d > presentRadius {
			return false, fmt.Sprintf("%s is gone (distance %d)", t.Name, d)
		}
		return true, ""
	case GuardNotSuperseded:
		if agent < 0 || agent >= len(s.Agents) {
			return false, "agent out of range"
		}
		if s.Agents[agent].Generation != g.Generation {
			return false, fmt.Sprintf("superseded (generation %d, was %d)",
				s.Agents[agent].Generation, g.Generation)
		}
		return true, ""
	case GuardAfterTick:
		if s.Tick >= g.Tick {
			return true, ""
		}
		return false, fmt.Sprintf("before tick %d", g.Tick)
	case GuardBeforeTick:
		if s.Tick < g.Tick {
			return true, ""
		}
		return false, fmt.Sprintf("past tick %d", g.Tick)
	default:
		return false, fmt.Sprintf("unknown guard %q", g.Type)
	}
}
