package mind

import (
	"testing"

	"github.com/evanstern/script-world/internal/sim"
)

// TestNextPhasePreservingDue is TASK-44: nextPhasePreservingDue is the
// extracted pure seam behind both the musing and planner re-arms. It must
// step a due schedule forward in whole cadence multiples from its own due
// — never from tick — landing strictly after tick while preserving due's
// phase (due mod cadence) exactly.
func TestNextPhasePreservingDue(t *testing.T) {
	tests := []struct {
		name               string
		due, tick, cadence int64
		want               int64
	}{
		{"not yet due: unchanged", 500, 100, 900, 500},
		{"overdue exactly at tick", 100, 100, 900, 1000},
		{"overdue by less than one cadence", 100, 500, 900, 1000},
		{"overdue by exactly one cadence", 100, 1000, 900, 1900},
		{"overdue by several cadences", 100, 2500, 900, 2800},
		{"one tick short of a second cadence", 100, 1899, 900, 1900},
		{"cadence non-multiple offset", 337, 33679, 900, 34537},
		{"zero cadence is a no-op guard", 100, 5000, 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextPhasePreservingDue(tt.due, tt.tick, tt.cadence)
			if got != tt.want {
				t.Errorf("nextPhasePreservingDue(%d, %d, %d) = %d, want %d",
					tt.due, tt.tick, tt.cadence, got, tt.want)
			}
			if tt.cadence > 0 {
				if got <= tt.tick {
					t.Errorf("result %d must land strictly after tick %d", got, tt.tick)
				}
				if mod := (got - tt.due) % tt.cadence; mod != 0 {
					t.Errorf("result %d drifted off due %d's phase (mod %d != 0)", got, tt.due, mod)
				}
			}
		})
	}
}

// TestMusingCadenceSurvivesSharedStall is TASK-44 AC#1: reproduces the
// reported collapse — a shared stall (busy tier) leaves every agent overdue
// at the identical tick. Draining and re-arming each agent (as muse's pick
// loop does, one per call) must leave every due pairwise distinct and must
// preserve each agent's boot offset (due mod cadence) — never collapse them
// onto the same schedule the way `tick + cadence` did.
func TestMusingCadenceSurvivesSharedStall(t *testing.T) {
	const cadence = museCadenceTicks

	// Mirrors New()'s boot stagger: museDue[i] = tick0 + cadence/2 +
	// i*(cadence/AgentCount), with tick0 == 0 here.
	boot := make([]int64, sim.AgentCount)
	for i := range boot {
		boot[i] = cadence/2 + int64(i)*(cadence/sim.AgentCount)
	}

	due := append([]int64(nil), boot...)
	tick := int64(50_000) // deep past every agent's original due: the stall

	for i := range due {
		if due[i] > tick {
			t.Fatalf("test setup: agent %d due %d is not overdue at tick %d", i, due[i], tick)
		}
		due[i] = nextPhasePreservingDue(due[i], tick, cadence)
	}

	seen := map[int64]int{}
	for i, d := range due {
		if d <= tick {
			t.Errorf("agent %d re-armed due %d must land after tick %d", i, d, tick)
		}
		if mod := (d - boot[i]) % cadence; mod != 0 {
			t.Errorf("agent %d due %d lost its boot phase %d (mod %d != 0)", i, d, boot[i], mod)
		}
		seen[d]++
	}
	for d, n := range seen {
		if n > 1 {
			t.Errorf("due %d shared by %d agents — phase collapse reproduced", d, n)
		}
	}
}

// TestNextPhasePreservingDueSkipsWithoutDrift is TASK-44 AC#2: a single
// agent stalled for many cadences jumps straight to the next open slot
// after tick, skipping whole cadences without ever drifting off its
// original boot phase.
func TestNextPhasePreservingDueSkipsWithoutDrift(t *testing.T) {
	const cadence = 900
	const boot = int64(337) // arbitrary phase offset within one cadence
	const stalledCadences = 37
	due := boot
	tick := boot + stalledCadences*cadence + 42 // deep into a stall, mid-cadence

	got := nextPhasePreservingDue(due, tick, cadence)

	if got <= tick {
		t.Fatalf("got %d, want strictly greater than tick %d", got, tick)
	}
	if mod := (got - boot) % cadence; mod != 0 {
		t.Errorf("phase drifted: due %d, boot offset %d, cadence %d, mod %d != 0", got, boot, cadence, mod)
	}
	if skips := (got - boot) / cadence; skips != stalledCadences+1 {
		t.Errorf("expected to skip forward %d cadences, landed at multiple %d", stalledCadences+1, skips)
	}
}
