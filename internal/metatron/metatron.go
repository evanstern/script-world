// Package metatron is the gatekeeper angel (TASK-12): the player's sole
// influence channel. A daemon-hosted notify consumer with its own replica
// (the mind/scribe pattern), it converses with the player, digests the event
// stream into an accreting soul, flags dramatic moments, and mediates nudges
// — whose only path into the world is the InjectSocial door, carrying only
// Metatron's own rendering. Raw player text has exactly one sink: Metatron's
// prompt.
package metatron

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Submitter is the orchestrator surface Metatron needs (test seam).
type Submitter interface {
	Submit(ctx context.Context, req llm.Request) (llm.Response, error)
}

// Injector is the loop surface Metatron needs (test seam).
type Injector interface {
	InjectSocial(events []store.Event) error
}

const (
	// turnTimeout bounds one console turn's cloud call (SC-001 wants ≤30s
	// in practice; the cap covers slow routers without wedging the console).
	turnTimeout = 2 * time.Minute
	// soulTailBytes / transcriptTailTurns bound what rides the prompt.
	soulTailBytes       = 4000
	transcriptTailTurns = 6
)

type Metatron struct {
	orch     Submitter
	social   Injector
	worldDir string

	replica *sim.State
	m       *worldmap.Map

	turnBusy atomic.Bool // one console turn at a time

	// runLoop drives one console turn through the bounded tool-use loop (spec
	// 017, T020). Production wires it to toolloop.Run against the concrete
	// orchestrator; tests that stub the model install a scripted driver. It is
	// touched only by Turn (never the absorb/digest goroutines), so installing it
	// in New before those start is race-free. loopRounds is the hard iteration
	// cap (llm.json loop_max_rounds, normalized), threaded from the daemon.
	runLoop    func(ctx context.Context, j toolloop.Job) (toolloop.Result, error)
	loopRounds int
	// turnTokens is the normalized console-turn response budget (llm.json
	// max_tokens.metatron_turn, spec 025 US2), threaded from the daemon exactly
	// as loopRounds is. Absent resolves to the built-in default (1024) at the
	// config boundary, so this is always an effective value. It governs the
	// console-turn loop ONLY — the metatron digest budget (400) is a different
	// call and stays hardcoded (spec 025 Assumption 2).
	turnTokens int64

	// fileMu guards soul.md / transcript.md appends (turn worker vs absorb
	// goroutine both write).
	fileMu sync.Mutex

	// stateMu guards the small mirrors the turn worker reads while the
	// absorb goroutine owns the replica.
	stateMu sync.Mutex
	charges int
	clockAt int64
	night   bool // mirrored State.Night — the omen night gate reads it turn-side (spec 029 T005)
	alive   map[int]bool
	// agentXY mirrors each villager's tile (absorb-owned, refreshed per batch)
	// so a console turn can resolve a tile-addressed miracle's perception-memory
	// recipient without racing the replica the absorb goroutine owns (spec 016).
	agentXY [][2]int
	moments []string // queued, surfaced oldest-first at the next turn
	story   []string // recent chronicle entries (TASK-11), prompt grounding

	// Standing-order mirror (spec 029 US2/US3, data-model §5): the replica's
	// MetatronOrders is the authority; the turn worker reads this copy under
	// stateMu (like charges/alive), and the absorb path matches live events
	// against it. lastPlaceTick/lastPlaceSeq disambiguate same-tick order ids
	// (research R7) when the async mirror has not yet reflected a just-placed
	// order — placements are serialized under turnBusy, so a plain counter is safe.
	orders        []sim.MetatronOrder
	lastPlaceTick int64
	lastPlaceSeq  int

	// digest collection (US4) — absorb-owned.
	digLines []string
	digFrom  int64
	digQ     chan digJob
	digCarry chan []string

	events chan []store.Event
	done   chan struct{}
}

// New starts the angel from a state snapshot. The metatron/ dir and an
// empty soul are created on first flight; existing files are kept.
//
// loopRounds is the tool-use loop's iteration cap (llm.json loop_max_rounds,
// already normalized by the daemon). The variadic runLoopOverride is a test
// seam: it installs a scripted loop BEFORE any goroutine starts, for tests that
// stub the model rather than pass a real *llm.Orchestrator. Production omits it —
// New wires runLoop from the concrete orchestrator.
func New(orch Submitter, social Injector, m *worldmap.Map, seed uint64, stateJSON []byte, worldDir string, loopRounds int, turnTokens int64, runLoopOverride ...func(context.Context, toolloop.Job) (toolloop.Result, error)) (*Metatron, error) {
	replica := sim.NewState(seed, m)
	if err := json.Unmarshal(stateJSON, replica); err != nil {
		return nil, err
	}
	mt := &Metatron{
		orch:     orch,
		social:   social,
		worldDir: worldDir,
		replica:  replica,
		m:        m,
		alive:    map[int]bool{},
		digQ:     make(chan digJob, 4),
		digCarry: make(chan []string, 1),
		events:   make(chan []store.Event, 256),
		done:     make(chan struct{}),
	}
	mt.loopRounds = loopRounds
	mt.turnTokens = turnTokens
	// The tool-use loop needs the concrete orchestrator (toolloop.Run's contract
	// surface — Submit + ObserveCognition). Production passes it; test seams that
	// stub the model install their own runLoop via runLoopOverride.
	if o, ok := orch.(*llm.Orchestrator); ok {
		mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
			return toolloop.Run(ctx, o, j)
		}
	}
	if len(runLoopOverride) > 0 && runLoopOverride[0] != nil {
		mt.runLoop = runLoopOverride[0]
	}
	if err := os.MkdirAll(mt.metatronDir(), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(mt.soulPath()); os.IsNotExist(err) {
		header := "# The soul of Metatron\n\n*The reign begins. The angel has seen nothing yet.*\n"
		if err := os.WriteFile(mt.soulPath(), []byte(header), 0o644); err != nil {
			return nil, err
		}
	}
	mt.digFrom = replica.Tick / digestWindowTicks
	mt.mirrorState()
	go mt.run()
	go mt.digestWorker()
	return mt, nil
}

// Observe is the loop-notify path: never blocks.
func (mt *Metatron) Observe(events []store.Event) {
	select {
	case mt.events <- events:
	default:
	}
}

func (mt *Metatron) Close() { close(mt.done) }

func (mt *Metatron) metatronDir() string    { return filepath.Join(mt.worldDir, "metatron") }
func (mt *Metatron) soulPath() string       { return filepath.Join(mt.metatronDir(), "soul.md") }
func (mt *Metatron) transcriptPath() string { return filepath.Join(mt.metatronDir(), "transcript.md") }

func (mt *Metatron) run() {
	for {
		select {
		case <-mt.done:
			return
		case batch := <-mt.events:
			for _, e := range batch {
				mt.replica.Apply(e)
				if e.Tick > mt.replica.Tick {
					mt.replica.Tick = e.Tick
				}
				mt.observeMoment(e)
				mt.digestNote(e)
			}
			mt.mirrorState()
		}
	}
}

// mirrorState refreshes the tiny snapshot the turn worker may read while
// the absorb goroutine keeps ticking the replica.
func (mt *Metatron) mirrorState() {
	mt.stateMu.Lock()
	defer mt.stateMu.Unlock()
	mt.charges = mt.replica.MetatronCharges
	mt.clockAt = mt.replica.Tick
	mt.night = mt.replica.Night
	if len(mt.agentXY) != len(mt.replica.Agents) {
		mt.agentXY = make([][2]int, len(mt.replica.Agents))
	}
	for i := range mt.replica.Agents {
		mt.alive[i] = !mt.replica.Agents[i].Dead
		mt.agentXY[i] = [2]int{mt.replica.Agents[i].X, mt.replica.Agents[i].Y}
	}
	// The standing-order mirror (spec 029): the replica is the authority, copied
	// so the turn worker reads orders under stateMu without racing the replica.
	mt.orders = append(mt.orders[:0], mt.replica.MetatronOrders...)
	// The narrated chronicle (TASK-11) is the village's own story — the
	// angel reads its tail so conversation is grounded even before its
	// soul has accreted (fresh reigns, upgraded worlds).
	mt.story = mt.story[:0]
	ring := mt.replica.Chronicle
	for i := maxOf(0, len(ring)-8); i < len(ring); i++ {
		mt.story = append(mt.story, fmt.Sprintf("day %d [%s] %s", ring[i].Day, ring[i].Thread, ring[i].Text))
	}
}

func maxOf(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// appendFile appends a block to one of Metatron's files (soul/transcript).
func (mt *Metatron) appendFile(path, block string) {
	mt.fileMu.Lock()
	defer mt.fileMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprint(f, block)
}

// tailOfFile returns up to n trailing bytes of a file ("" on any error).
func tailOfFile(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return ""
	}
	return string(buf)
}
