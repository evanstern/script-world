package sim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
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
	Tick     int64       `json:"tick"`
	GameTime string      `json:"game_time"`
	Paused   bool        `json:"paused"`
	Speed    clock.Speed `json:"speed"`
	// RequestedSpeed (spec 028 US2) is the player's ceiling while the governor
	// holds Speed below it, empty when ungoverned — mirrors State.RequestedSpeed
	// so the daemon's governor sampler can read the ceiling (and paused) through
	// the loop's non-blocking status door without touching State directly.
	RequestedSpeed clock.Speed `json:"requested_speed,omitempty"`
	EffectiveRate  float64     `json:"effective_rate"`
	Degraded       bool        `json:"degraded"`
	LastSeq        int64       `json:"last_seq"`
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
	// Kind/Qty (spec 013 R4) argue the storage goals (drop/pick_up/deposit/
	// withdraw) when Goal is one of them; ignored otherwise. Additive —
	// pre-013 callers leave them zero.
	Kind string
	Qty  int
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
	name   string // status | state | pause | resume | set_speed | govern | inject_intent | inject_social
	speed  clock.Speed
	govern *governArgs
	inject *InjectArgs
	social []store.Event
	reply  chan commandResult
}

// governArgs carries a governor decision into the command channel: the target
// effective speed and the debt arithmetic to record on the resulting event.
type governArgs struct {
	to   clock.Speed
	debt float64
	jobs int
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

// Govern applies a governor decision at the next tick boundary: it records a
// clock.governor_shed or clock.governor_recovered event and paces at the new
// effective speed immediately, exactly the path a player set_speed takes. A
// decision that no longer applies — the world paused, the speed already moved,
// `to` off the capped ladder or not exactly one notch from the current speed,
// or a recover above the standing ceiling — emits NOTHING and returns cleanly;
// the daemon's controller simply re-samples next cadence, so there is never a
// merge to resolve (spec 028 contracts/internal-api.md, FR-005/FR-006/FR-014).
//
// Thread-safe from any goroutine (the daemon's sampler owns the only caller);
// fails cleanly if the loop has stopped, mirroring InjectIntent.
func (l *Loop) Govern(to clock.Speed, debt float64, jobs int) (Status, error) {
	cmd := command{name: "govern", govern: &governArgs{to: to, debt: debt, jobs: jobs}, reply: make(chan commandResult, 1)}
	select {
	case l.commands <- cmd:
	case <-l.done:
		return Status{}, errors.New("simulation loop is not running")
	}
	select {
	case res := <-cmd.reply:
		return res.status, res.err
	case <-l.done:
		return Status{}, errors.New("simulation loop stopped")
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
	// The grounded-observation seam (spec 030 US2, FR-008): re-anchors a held
	// belief's decay clock when the villager directly observes supporting
	// evidence. 030 ships the consumer only (whitelist + reducer arm + tests);
	// the perception-of-absence task is the intended future PRODUCER — nothing
	// emits this in-tree yet.
	"agent.belief_reinforced": true,
	// Musings (TASK-21): interiority with no state effect — recorded
	// chronicle material only.
	"agent.thought": true,
	// The chronicle (TASK-11): the narrator's story entries.
	"chronicle.entry": true,
	// Metatron nudges (TASK-12): the spend + record; the dry-run enforces
	// charges/form/target/text validity before anything lands.
	"metatron.nudged": true,
	// Metatron miracles (spec 016): the four charge-priced world edits; the
	// dry-run's reducer arms enforce presence/destination/charge before
	// anything lands, and the whitelist is the isolation boundary.
	"metatron.time_snapped":   true,
	"metatron.item_granted":   true,
	"metatron.entity_moved":   true,
	"metatron.entity_removed": true,
	// Metatron standing orders (spec 029): the injected order-lifecycle events.
	// order_placed (monitor_and_act) and order_cancelled (cancel_order) are the
	// two Expressive tools' Events; order_triggered is injected by the trigger
	// worker (Batch B). order_expired is EXECUTOR-emitted (a pure function of
	// state + tick, like charge_regenerated) and so needs no whitelist entry —
	// only model/worker-injected types pass this door.
	"metatron.order_placed":    true,
	"metatron.order_cancelled": true,
	"metatron.order_triggered": true,
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
	// The tool-use loop's call trace (spec 017, FR-007): one record per tool
	// call the loop saw (landed, rejected, read, or unlanded) — recorded
	// observability, reducer no-op, same isolation guarantees as the other
	// cog.* types above.
	"cog.tool_call": true,
	// Agent-authored journal (spec 019, US3): the two mind-injectable journal
	// mutations. Landed only through this door; the reducer dry-run enforces the
	// rune budget (written) and entry existence (deleted) before either lands.
	// sim.ValidateToolCoverage pins the two Expressive journal tools' Events ⊆
	// this whitelist at boot.
	"journal.entry_written": true,
	"journal.entry_deleted": true,
}

// InjectSocial applies a batch of whitelisted social events atomically at
// the next tick boundary (all-or-nothing): ticks are re-stamped, payloads
// dry-run on a state copy first, then applied and recorded.
//
// Deliberately pause-open, like InjectIntent (FR-018): a conversation
// founded before a pause completes on the wall clock and lands its whole
// scene at the frozen tick. Tick-driven scheduling freezes with the clock;
// a landing batch may wake one debounce-bounded round of catch-up thought
// at zero staleness before the mind quiesces (live finding, 2026-07-20) —
// pause is the one state where thought fidelity is perfect.
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
		RequestedSpeed:  s.RequestedSpeed,
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
	case "govern":
		// A governor decision lands as a recorded event exactly like set_speed,
		// or is dropped silently (no event, clean return) when it no longer
		// applies. Every drop path just returns — the controller re-samples next
		// cadence (contracts/internal-api.md). Pause and off-ladder are checked
		// first; direction and one-notch adjacency infer the event type.
		g := cmd.govern
		cur := l.state.Speed
		curIdx, toIdx := clock.LadderIndex(cur), clock.LadderIndex(g.to)
		switch {
		case l.state.Paused:
			// Never govern a paused world (FR-013): the clock and the governor
			// with it are frozen; in-flight thoughts drain debt under pause.
		case curIdx < 0 || toIdx < 0:
			// Off the capped ladder (e.g. max) — the governor never touches it.
		case g.to == cur:
			// Stale decision: the speed already moved to the target (a player
			// change or a prior govern landed first). No-op.
		case toIdx-curIdx == -1:
			// Shed one notch down. The ceiling is the standing RequestedSpeed, or
			// the current speed on the first shed of an ungoverned world.
			emit("clock.governor_shed", GovernorPayload{
				Requested: l.requestedCeiling(), From: cur, To: g.to, Debt: g.debt, Jobs: g.jobs,
			})
		case toIdx-curIdx == 1:
			// Recover one notch up — but never above the player's ceiling.
			req := l.requestedCeiling()
			if reqIdx := clock.LadderIndex(req); reqIdx >= 0 && toIdx > reqIdx {
				break // a recover above the requested ceiling is dropped
			}
			emit("clock.governor_recovered", GovernorPayload{
				Requested: req, From: cur, To: g.to, Debt: g.debt, Jobs: g.jobs,
			})
		default:
			// Not exactly one notch (a stale multi-notch decision) — dropped.
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
		// The probe is reconstructed from bytes and so carries no map
		// (unexported, unserialized); attach the loop's map so miracle
		// arms validate the terrain vocabulary in the dry-run exactly as
		// the real apply and replay will (spec 016).
		probe.SetMap(l.m)
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
		// The landing ladder lives in landing.go: the ordered doctrine rungs
		// (unavailable → superseded → stale → guards → plan/goal → outcome). A
		// rejected landing sets err AND emits its rejection record — the only
		// command that pairs the two.
		err = l.landIntent(cmd.inject, emit)
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

// requestedCeiling is the player's speed ceiling for a governor event payload:
// the standing RequestedSpeed while already governed, else the current
// effective Speed — the first shed of an ungoverned world records the speed the
// player actually asked for as the ceiling (contracts/internal-api.md).
func (l *Loop) requestedCeiling() clock.Speed {
	if l.state.RequestedSpeed != "" {
		return l.state.RequestedSpeed
	}
	return l.state.Speed
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
