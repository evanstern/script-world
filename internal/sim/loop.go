package sim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/cognition"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// SnapshotEveryTicks is the cadence bound on recovery replay: 1 game hour.
const SnapshotEveryTicks = 3600

const snapshotsKept = 24

// degradeWindow is how long an overrun must sustain before the loop calls
// itself degraded (and how often recovery is re-evaluated).
const degradeWindow = 5 * time.Second

// Status is the clock section of the protocol status shape, snapshotted
// inside the loop goroutine so it is always coherent.
type Status struct {
	Tick          int64       `json:"tick"`
	GameTime      string      `json:"game_time"`
	Paused        bool        `json:"paused"`
	Speed         clock.Speed `json:"speed"`
	EffectiveRate float64     `json:"effective_rate"`
	Degraded      bool        `json:"degraded"`
	LastSeq       int64       `json:"last_seq"`
	// MetatronCharges surfaces the nudge bank (TASK-12) so clients can
	// display ⚡ without a state fetch.
	MetatronCharges int `json:"metatron_charges"`
}

// InjectArgs carries a planner decision into deterministic space.
type InjectArgs struct {
	Agent       int
	Goal        string
	TargetAgent int // for seek/talk_to; -1 otherwise
	Reason      string
	// Cognition-horizon landing metadata (TASK-32). Class empty means an
	// unmetered caller (tests, tooling): the ladder's staleness, generation,
	// and guard checks are skipped and no telemetry is emitted — the
	// pre-TASK-32 contract.
	Class           string
	JobID           string
	SnapshotTick    int64
	Generation      int64
	PredictedWallMs int64
	ActualWallMs    int64
	Guards          []Guard
	// Plan is a guarded conditional plan (US4) — mutually exclusive with
	// Goal; the same ladder applies, then agent.plan_set records the steps.
	Plan []PlanStep
}

type command struct {
	name   string // status | state | pause | resume | set_speed | inject_intent | inject_social
	speed  clock.Speed
	inject *InjectArgs
	social []store.Event
	reply  chan commandResult
}

type commandResult struct {
	status Status
	state  []byte // canonical State JSON; set only for "state" commands
	err    error
}

// Loop is the single goroutine that owns State and the write path to the
// store. All external inputs enter through the command channel and are
// applied only at tick boundaries; every applied command is recorded as an
// event, making the log the complete input record (determinism, R3).
type Loop struct {
	state    *State
	m        *worldmap.Map // static terrain; read-only context for stepEvents
	st       *store.Store
	notify   func([]store.Event) // called after commit; must not block
	commands chan command
	done     chan struct{}

	// scheduler bookkeeping (loop goroutine only)
	windowStart time.Time
	windowTicks int64
	measured    float64 // achieved ticks/sec over the last window
}

func NewLoop(state *State, m *worldmap.Map, st *store.Store, notify func([]store.Event)) *Loop {
	if notify == nil {
		notify = func([]store.Event) {}
	}
	return &Loop{
		state:    state,
		m:        m,
		st:       st,
		notify:   notify,
		commands: make(chan command),
		done:     make(chan struct{}),
	}
}

// Do submits a command to the loop and waits for the resulting status.
// Safe from any goroutine; fails cleanly if the loop has stopped.
func (l *Loop) Do(name string, speed clock.Speed) (Status, error) {
	res, err := l.do(name, speed)
	return res.status, err
}

// InjectIntent applies a planner decision at the next tick boundary: the
// goal is validated and resolved deterministically, then recorded as
// agent.intent_set (source planner) + agent.thought. Model output enters the
// sim ONLY through here — as recorded input.
//
// Deliberately pause-open (FR-018, decision-4): pause means "the world
// freezes and the minds catch up" — an in-flight thought completes on the
// wall clock and lands at the frozen tick, where its game-tick staleness is
// zero by construction. Cancelling completed thought was considered and
// rejected: it would discard work that is, by tick arithmetic, perfectly
// fresh.
func (l *Loop) InjectIntent(args InjectArgs) error {
	cmd := command{name: "inject_intent", inject: &args, reply: make(chan commandResult, 1)}
	select {
	case l.commands <- cmd:
	case <-l.done:
		return errors.New("simulation loop is not running")
	}
	select {
	case res := <-cmd.reply:
		return res.err
	case <-l.done:
		return errors.New("simulation loop stopped")
	}
}

// injectSocialWhitelist is the mind's injection door: every event type a
// model-driven driver (conversations, TASK-9 consolidation, TASK-21
// musings) may land. The whitelist IS the isolation — everything else
// about the world is unreachable from model output.
var injectSocialWhitelist = map[string]bool{
	"social.relation_changed":  true,
	"social.rumor_told":        true,
	"social.conversation_turn": true,
	"social.conversation":      true,
	"agent.memory_added":       true,
	"agent.memory_promoted":    true,
	"agent.memory_faded":       true,
	"agent.belief_revised":     true,
	"agent.narrative_set":      true,
	"agent.consolidated":       true,
	// Musings (TASK-21): interiority with no state effect — recorded
	// chronicle material only.
	"agent.thought": true,
	// The chronicle (TASK-11): the narrator's story entries.
	"chronicle.entry": true,
	// Metatron nudges (TASK-12): the spend + record; the dry-run enforces
	// charges/form/target/text validity before anything lands.
	"metatron.nudged": true,
	// Governance flavor (TASK-13): the ONLY injectable governance type —
	// re-texts an enacted norm in the proposer's voice; outcomes stay
	// executor-deterministic. The dry-run enforces norm existence + text cap.
	"meeting.proposal_rephrased": true,
	// Cognition telemetry (TASK-32): recorded observability, reducer no-ops.
	// Every thought's lifecycle lands here so no failure is ever silent
	// (FR-015) and thought chains are walkable from the log alone (FR-020).
	"cog.thought":                   true,
	"cog.outcome":                   true,
	"cog.recalibration_recommended": true,
}

// InjectSocial applies a batch of whitelisted social events atomically at
// the next tick boundary (all-or-nothing): ticks are re-stamped, payloads
// dry-run on a state copy first, then applied and recorded.
//
// Deliberately pause-open, like InjectIntent (FR-018): a conversation
// founded before a pause completes on the wall clock and lands its whole
// scene at the frozen tick. No new cognition starts while paused — the
// mind's scheduling is tick-driven and ticks stop — so pause is the one
// state where thought fidelity is perfect.
func (l *Loop) InjectSocial(events []store.Event) error {
	cmd := command{name: "inject_social", social: events, reply: make(chan commandResult, 1)}
	select {
	case l.commands <- cmd:
	case <-l.done:
		return errors.New("simulation loop is not running")
	}
	select {
	case res := <-cmd.reply:
		return res.err
	case <-l.done:
		return errors.New("simulation loop stopped")
	}
}

// DoState returns a coherent snapshot of the full world state (canonical
// JSON) plus the clock status captured in the same loop iteration — the
// last_seq in the status is exactly the log position the state reflects.
func (l *Loop) DoState() ([]byte, Status, error) {
	res, err := l.do("state", "")
	return res.state, res.status, err
}

func (l *Loop) do(name string, speed clock.Speed) (commandResult, error) {
	cmd := command{name: name, speed: speed, reply: make(chan commandResult, 1)}
	select {
	case l.commands <- cmd:
	case <-l.done:
		return commandResult{}, errors.New("simulation loop is not running")
	}
	select {
	case res := <-cmd.reply:
		return res, res.err
	case <-l.done:
		return commandResult{}, errors.New("simulation loop stopped")
	}
}

func (l *Loop) status() Status {
	s := l.state
	eff := l.measured
	if eff == 0 && !s.Paused {
		eff = s.Speed.TicksPerSecond()
	}
	if s.Paused {
		eff = 0
	}
	return Status{
		Tick:            s.Tick,
		GameTime:        clock.Format(s.Tick),
		Paused:          s.Paused,
		Speed:           s.Speed,
		EffectiveRate:   eff,
		Degraded:        s.Degraded,
		LastSeq:         l.st.LastSeq(),
		MetatronCharges: s.MetatronCharges,
	}
}

// Run drives the world until ctx is canceled, then takes a final snapshot.
func (l *Loop) Run(ctx context.Context) error {
	defer close(l.done)
	l.windowStart = time.Now()
	next := time.Now()

	for {
		if l.state.Paused {
			// Paused: no timer, just wait for commands or shutdown.
			select {
			case <-ctx.Done():
				return l.finalSnapshot()
			case cmd := <-l.commands:
				if err := l.handleCommand(cmd); err != nil {
					return err
				}
				next = time.Now() // pacing restarts on resume
				l.resetWindow()
			}
			continue
		}

		interval := l.state.Speed.Interval()
		if interval == 0 {
			// Max speed: spin, staying responsive to commands and shutdown.
			select {
			case <-ctx.Done():
				return l.finalSnapshot()
			case cmd := <-l.commands:
				if err := l.handleCommand(cmd); err != nil {
					return err
				}
			default:
				if err := l.runTick(); err != nil {
					return err
				}
				if l.state.Tick%1024 == 0 {
					runtime.Gosched()
				}
			}
			l.observeWindow(0)
			continue
		}

		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return l.finalSnapshot()
		case cmd := <-l.commands:
			timer.Stop()
			if err := l.handleCommand(cmd); err != nil {
				return err
			}
		case <-timer.C:
			if err := l.runTick(); err != nil {
				return err
			}
			next = next.Add(interval)
			// Behind by more than one interval: no debt catch-up bursts —
			// the world slows honestly instead of skipping (FR-012).
			if now := time.Now(); now.After(next.Add(interval)) {
				next = now
			}
			if err := l.observeWindow(interval); err != nil {
				return err
			}
		}
	}
}

// runTick advances exactly one tick: events are a pure function of
// (state, nextTick), applied through the reducer, committed as one batch,
// then published.
func (l *Loop) runTick() error {
	nextTick := l.state.Tick + 1
	events := stepEvents(l.state, l.m, nextTick)
	l.state.Tick = nextTick
	for _, e := range events {
		if err := l.state.Apply(e); err != nil {
			return fmt.Errorf("tick %d: %w", nextTick, err)
		}
	}
	if len(events) > 0 {
		if err := l.st.AppendEvents(events); err != nil {
			return fmt.Errorf("tick %d append: %w", nextTick, err)
		}
		l.notify(events)
	}
	l.windowTicks++
	if nextTick%SnapshotEveryTicks == 0 {
		return l.snapshot()
	}
	return nil
}

func (l *Loop) handleCommand(cmd command) error {
	var events []store.Event
	emit := func(typ string, payload any) {
		events = append(events, store.Event{Tick: l.state.Tick, Type: typ, Payload: mustPayload(payload)})
	}

	var err error
	var stateJSON []byte
	switch cmd.name {
	case "status":
		// Read-only.
	case "state":
		stateJSON = l.state.Marshal()
	case "pause":
		if !l.state.Paused {
			emit("clock.paused", struct{}{})
		}
	case "resume":
		if l.state.Paused {
			emit("clock.resumed", struct{}{})
		}
	case "set_speed":
		if _, perr := clock.ParseSpeed(string(cmd.speed)); perr != nil {
			err = perr
		} else if cmd.speed != l.state.Speed {
			emit("clock.speed_set", SpeedSetPayload{Speed: cmd.speed})
		}
	case "inject_social":
		batch := cmd.social
		if len(batch) == 0 {
			err = fmt.Errorf("empty social batch")
			break
		}
		for i := range batch {
			if !injectSocialWhitelist[batch[i].Type] {
				err = fmt.Errorf("event type %q not injectable", batch[i].Type)
				break
			}
			batch[i].Tick = l.state.Tick
			batch[i].Seq = 0
			batch[i].WallTime = ""
		}
		if err != nil {
			break
		}
		// Dry-run on a copy: the batch lands atomically or not at all.
		probe := &State{}
		if uerr := json.Unmarshal(l.state.Marshal(), probe); uerr != nil {
			err = uerr
			break
		}
		for _, e := range batch {
			if aerr := probe.Apply(e); aerr != nil {
				err = fmt.Errorf("social batch rejected: %w", aerr)
				break
			}
		}
		if err != nil {
			break
		}
		events = append(events, batch...)
	case "inject_intent":
		in := cmd.inject
		if in.Agent < 0 || in.Agent >= len(l.state.Agents) {
			err = fmt.Errorf("no such agent %d", in.Agent)
			break
		}
		a := &l.state.Agents[in.Agent]
		// The landing ladder (TASK-32, FR-010..FR-013): enforcement happens
		// against the world as it is NOW. Every metered rejection is
		// recorded atomically with the verdict — the rejection events land
		// even though err is set (silent failure is gone, FR-015).
		staleness := l.state.Tick - in.SnapshotTick
		if staleness < 0 {
			staleness = 0
		}
		reject := func(outcome, reason string) {
			err = fmt.Errorf("%s: %s", outcome, reason)
			if in.Class == "" {
				return
			}
			kind := RejectKindWorldChange
			if in.PredictedWallMs > 0 && in.ActualWallMs > PredictionMissFactor*in.PredictedWallMs {
				kind = RejectKindPredictionMiss
			}
			emit("agent.intent_rejected", IntentRejectedPayload{
				Agent: in.Agent, Goal: in.Goal, Reason: reason, StalenessTicks: staleness,
			})
			emit("cog.outcome", CogOutcomePayload{
				Job: in.JobID, Class: in.Class, Agent: in.Agent,
				Outcome: outcome, SnapshotTick: in.SnapshotTick,
				LandingTick: l.state.Tick, StalenessTicks: staleness,
				PredictedWallMs: in.PredictedWallMs, ActualWallMs: in.ActualWallMs,
				Kind: kind, Reason: reason,
			})
		}
		if a.Dead {
			reject(OutcomeUnavailable, a.Name+" is dead")
			break
		}
		if a.Asleep {
			reject(OutcomeUnavailable, a.Name+" is asleep")
			break
		}
		adapted := false
		if in.Class != "" {
			if in.Generation != a.Generation {
				reject(OutcomeSuperseded, fmt.Sprintf("generation %d, thought was %d", a.Generation, in.Generation))
				break
			}
			if dc, ok := cognition.ClassFor(in.Class); ok && staleness > dc.BudgetTicks {
				reject(OutcomeRejectedStale, fmt.Sprintf("staleness %d > budget %d", staleness, dc.BudgetTicks))
				break
			}
			failed := false
			for _, g := range in.Guards {
				ok, why := g.Eval(l.state, in.Agent)
				if !ok {
					reject(OutcomeRejectedGuard, why)
					failed = true
					break
				}
				// The adapt rung: a target_present guard that holds but
				// whose target moved means resolveGoal repaired the intent.
				if g.Type == GuardTargetPresent && (g.X != 0 || g.Y != 0) {
					t := &l.state.Agents[g.Target]
					if t.X != g.X || t.Y != g.Y {
						adapted = true
					}
				}
			}
			if failed {
				break
			}
		}
		if len(in.Plan) > 0 {
			// A guarded conditional plan (US4): validate at the door, then
			// record the steps — the executor evaluates guards per tick.
			if len(in.Plan) > PlanStepCap {
				reject(OutcomeRejectedGuard, fmt.Sprintf("plan has %d steps (cap %d)", len(in.Plan), PlanStepCap))
				break
			}
			for si := range in.Plan {
				if !planGoals[in.Plan[si].Goal] {
					reject(OutcomeRejectedGuard, fmt.Sprintf("plan step %d: unknown goal %q", si, in.Plan[si].Goal))
					break
				}
				if in.Plan[si].Until == 0 {
					in.Plan[si].Until = l.state.Tick + PlanDefaultWindowTicks
				}
			}
			if err != nil {
				break
			}
			if in.Reason != "" {
				emit("agent.thought", ThoughtPayload{Agent: in.Agent, Text: in.Reason, Source: "planner"})
			}
			emit("agent.plan_set", PlanSetPayload{Agent: in.Agent, Job: in.JobID, Steps: in.Plan})
		} else {
			intent, direct, rerr := resolveGoal(l.state, l.m, in.Agent, in.Goal, in.TargetAgent, l.state.Tick)
			if rerr != nil {
				// resolveGoal is the repair path; failing here means no
				// deterministic adaptation exists — a world change.
				reject(OutcomeRejectedGuard, rerr.Error())
				break
			}
			if in.Reason != "" {
				emit("agent.thought", ThoughtPayload{Agent: in.Agent, Text: in.Reason, Source: "planner"})
			}
			if direct == "agent.ate" {
				emit("agent.ate", AgentPayload{Agent: in.Agent})
			} else if intent != nil {
				emit("agent.intent_set", IntentSetPayload{
					Agent: in.Agent, Goal: intent.Goal,
					TargetX: intent.TargetX, TargetY: intent.TargetY,
					ResX: intent.ResX, ResY: intent.ResY,
					Source: "planner",
				})
			}
		}
		if in.Class != "" {
			outcome := OutcomeLanded
			if adapted {
				outcome = OutcomeAdapted
			}
			emit("cog.outcome", CogOutcomePayload{
				Job: in.JobID, Class: in.Class, Agent: in.Agent,
				Outcome: outcome, SnapshotTick: in.SnapshotTick,
				LandingTick: l.state.Tick, StalenessTicks: staleness,
				PredictedWallMs: in.PredictedWallMs, ActualWallMs: in.ActualWallMs,
			})
		}
	default:
		err = fmt.Errorf("unknown command %q", cmd.name)
	}

	// Events land whenever they were emitted — a rejected inject_intent sets
	// err AND emits its rejection record (the only command that pairs the
	// two); every other error path emits nothing.
	{
		for _, e := range events {
			if aerr := l.state.Apply(e); aerr != nil {
				return aerr
			}
		}
		if len(events) > 0 {
			if aerr := l.st.AppendEvents(events); aerr != nil {
				return aerr
			}
			l.notify(events)
			l.resetWindow()
			if l.state.Paused {
				// Pausing snapshots, so a stop-while-paused resumes instantly.
				if serr := l.snapshot(); serr != nil {
					return serr
				}
			}
		}
	}
	cmd.reply <- commandResult{status: l.status(), state: stateJSON, err: err}
	return nil
}

// observeWindow keeps the effective-rate measurement honest and emits
// clock.degraded / clock.recovered on sustained transitions (R7).
func (l *Loop) observeWindow(interval time.Duration) error {
	elapsed := time.Since(l.windowStart)
	if elapsed < degradeWindow {
		return nil
	}
	l.measured = float64(l.windowTicks) / elapsed.Seconds()
	defer l.resetWindow()

	if interval == 0 {
		return nil // max speed: whatever we achieve is the contract
	}
	requested := l.state.Speed.TicksPerSecond()
	var events []store.Event
	if !l.state.Degraded && l.measured < requested*0.9 {
		events = append(events, store.Event{
			Tick: l.state.Tick, Type: "clock.degraded",
			Payload: mustPayload(DegradedPayload{EffectiveRate: l.measured}),
		})
	} else if l.state.Degraded && l.measured >= requested*0.95 {
		events = append(events, store.Event{
			Tick: l.state.Tick, Type: "clock.recovered", Payload: mustPayload(struct{}{}),
		})
	}
	for _, e := range events {
		if err := l.state.Apply(e); err != nil {
			return err
		}
	}
	if len(events) > 0 {
		if err := l.st.AppendEvents(events); err != nil {
			return err
		}
		l.notify(events)
	}
	return nil
}

func (l *Loop) resetWindow() {
	l.windowStart = time.Now()
	l.windowTicks = 0
}

func (l *Loop) snapshot() error {
	if err := l.st.SaveSnapshot(l.state.Tick, l.st.LastSeq(), l.state.Marshal()); err != nil {
		return err
	}
	return l.st.PruneSnapshots(snapshotsKept)
}

func (l *Loop) finalSnapshot() error { return l.snapshot() }
