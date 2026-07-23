// Package llm is the orchestrator for all model traffic (TASK-6): two
// tiers (local Ollama-style HTTP, cloud Anthropic), kind-based routing,
// bounded queues with backpressure feeding N concurrent local workers
// (configurable via local.parallel, default 1; cloud is always single-worker,
// TASK-45), a persisted monthly spend meter with a hard ceiling, and per-tier
// circuit breakers so unreachable inference degrades the AI layer — never the
// simulation.
//
// The orchestrator lives entirely OUTSIDE the deterministic sim loop. LLM
// results reach the world only as recorded inputs (TASK-7's job), so replay
// never re-calls a model.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evanstern/promptworld/internal/cognition"
)

// Kind classifies a call; routing to a tier follows the grounding decisions.
type Kind string

const (
	KindPlanner       Kind = "planner"
	KindConversation  Kind = "conversation"
	KindConsolidation Kind = "consolidation"
	KindNarrator      Kind = "narrator"
	KindDrama         Kind = "drama"
	// KindMetatron is the gatekeeper angel (TASK-12): console turns,
	// nudge judgment, and digests — premium cognition, tiny volume.
	KindMetatron Kind = "metatron"
	// KindMeeting is governance flavor (TASK-13): rephrasing a tabled
	// proposal in the proposer's voice. Best-effort, never outcome-bearing.
	KindMeeting Kind = "meeting"
)

type Tier string

const (
	TierLocal Tier = "local"
	TierCloud Tier = "cloud"
)

// routing: high-volume ambient cognition stays local (free, ~3800+ calls/day
// only viable self-hosted); low-volume high-quality work goes cloud.
var routing = map[Kind]Tier{
	KindPlanner:       TierLocal,
	KindConversation:  TierLocal,
	KindConsolidation: TierCloud,
	KindNarrator:      TierCloud,
	KindDrama:         TierCloud,
	KindMetatron:      TierCloud,
	KindMeeting:       TierLocal,
}

// Kinds returns every call kind the orchestrator accepts, sorted — the
// cognition registry's completeness gate (FR-002) iterates this at daemon
// start so an unregistered kind can never reach a model at runtime.
func Kinds() []Kind {
	out := make([]Kind, 0, len(routing))
	for k := range routing {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var (
	ErrUnknownKind     = errors.New("unknown call kind")
	ErrBudgetExhausted = errors.New("monthly cloud budget exhausted; call refused (raise monthly_budget_usd in llm.json or wait for the month to roll over)")
	ErrTierDown        = errors.New("tier is down (circuit open); the world keeps running degraded")
	ErrQueueFull       = errors.New("tier queue full; back off and retry")
	ErrTierBusy        = errors.New("tier busy; best-effort call dropped")
	ErrClosed          = errors.New("orchestrator closed")
)

type Request struct {
	Kind      Kind   `json:"kind"`
	System    string `json:"system,omitempty"`
	Prompt    string `json:"prompt"`
	MaxTokens int64  `json:"max_tokens,omitempty"`
	// BestEffort requests drop-when-busy admission: the call is refused
	// with ErrTierBusy when no worker slot is free — any queued work, or all
	// N slots in flight (TASK-45). Callers that may not displace real
	// cognition set this; their fairness floor is the caller's business, not
	// the orchestrator's. (Scheduled musing was its first user until spec 017
	// folded musing into the planner loop; the mechanism stays doctrine for any
	// future drop-when-busy kind.)
	BestEffort bool `json:"best_effort,omitempty"`
	// --- agent tool-use loop transport (TASK-52; all additive) ---
	// Tools declares the tools the model may call this round. nil = no tools
	// parameter is sent on the wire (today's behavior for every single-shot
	// kind, byte-identical).
	Tools []ToolDecl `json:"tools,omitempty"`
	// Turns is the multi-turn transcript. nil = the single Prompt user message
	// is sent (today's behavior, byte-identical); non-nil replaces Prompt as
	// the message source. The transcript is ephemeral — never persisted, never
	// replayed.
	Turns []Turn `json:"turns,omitempty"`
	// SkipObserve marks a loop-internal call: the worker feeds NO per-call
	// sample to the tier estimator (the loop reports one whole-cognition
	// observation via Orchestrator.ObserveCognition instead). Metering,
	// admission, and the circuit breaker are unaffected.
	SkipObserve bool `json:"skip_observe,omitempty"`
}

type Response struct {
	Text         string  `json:"text"`
	Tier         Tier    `json:"tier"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Millis       int64   `json:"ms"`
	// --- agent tool-use loop transport (TASK-52; additive) ---
	// ToolCalls holds the calls the model emitted this round, in emission
	// order; nil for a plain-text reply. Stop is the provider's stop reason,
	// letting the loop driver tell "model finished" from "model wants results".
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Stop      StopReason `json:"stop,omitempty"`
}

// --- tool-call transport model (TASK-52) ---
//
// These types are the wire-agnostic currency between the loop driver and the
// per-tier callers: a caller translates Request{Tools,Turns} out to its
// provider's shape and Response{ToolCalls,Stop} back. They are transport-only
// and ephemeral — never persisted and never replayed (data-model §3).

// Role labels a transcript turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Turn is one message in the transcript: a role and its content blocks.
type Turn struct {
	Role   Role    `json:"role"`
	Blocks []Block `json:"blocks"`
}

// Block is one content block in a turn — exactly one of the three is set.
type Block struct {
	Text       string           `json:"text,omitempty"`
	ToolUse    *ToolUseBlock    `json:"tool_use,omitempty"`    // assistant-side call echo
	ToolResult *ToolResultBlock `json:"tool_result,omitempty"` // user-side outcome
}

// ToolUseBlock echoes a prior assistant tool call in the transcript.
type ToolUseBlock struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// ToolResultBlock carries a tool's outcome back to the model, tied to the
// call it answers by ForID.
type ToolResultBlock struct {
	ForID   string `json:"for_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// ToolDecl is one declared tool on a Request: the schema the model calls against.
type ToolDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema object
}

// ToolCall is one call parsed from a Response.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// StopReason is why the model stopped this round.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"   // model reached a natural stop
	StopToolUse   StopReason = "tool_use"   // model wants tool results to continue
	StopMaxTokens StopReason = "max_tokens" // output token budget hit
	StopOther     StopReason = "other"      // any other/unmapped reason
)

// TierStatus and Status feed the protocol status shape and the TUI.
type TierStatus struct {
	Model    string `json:"model"`
	Endpoint string `json:"endpoint,omitempty"`
	Up       bool   `json:"up"`
	Queue    int    `json:"queue"`
}

type Status struct {
	Local  TierStatus `json:"local"`
	Cloud  TierStatus `json:"cloud"`
	Month  string     `json:"month"`
	Spent  float64    `json:"spent_usd"`
	Budget float64    `json:"budget_usd"`
}

const queueCap = 32

// workerCallCap bounds any single provider call at the worker, the last
// line of defense against a hung transport freezing a tier.
const workerCallCap = 2 * time.Minute

type job struct {
	ctx   context.Context
	req   Request
	reply chan result
}

type result struct {
	resp Response
	err  error
}

type tier struct {
	name   Tier
	caller caller
	health *tierHealth
	queue  chan job
	prio   chan job // interactive work (conversations) jumps the line
	// slots is the tier's concurrent capacity: the number of worker
	// goroutines draining its channels (TASK-45). Local is configurable
	// (LocalConfig.Workers()); cloud is always 1 (FR-008).
	slots int
	// inflight counts jobs a worker has dequeued and not yet replied to —
	// incremented at dequeue, decremented on every reply path. It drives
	// slot-aware best-effort admission: 0 ≤ inflight ≤ slots.
	inflight atomic.Int32
	// est is the live seconds-per-point estimate for this tier (TASK-32):
	// the worker is the one place every call's true duration is observed,
	// so it feeds the estimator; the mind reads it to route.
	est *cognition.Estimator
}

// TierFor exposes a kind's tier — the mind needs it to read the right
// estimator when routing.
func TierFor(kind Kind) (Tier, bool) {
	t, ok := routing[kind]
	return t, ok
}

// Orchestrator routes, queues, meters, and degrades. One per daemon.
type Orchestrator struct {
	cfg       Config
	meter     *Meter
	tiers     map[Tier]*tier
	done      chan struct{}
	closeOnce sync.Once

	// recalibrate is invoked (in its own goroutine) when a tier's estimator
	// first breaches the spike-rate threshold — the mind turns it into a
	// cog.recalibration_recommended telemetry event.
	recalMu     sync.Mutex
	recalibrate func(tier Tier, estimate, spikeRate float64)
}

func New(cfg Config, st MeterStore) (*Orchestrator, error) {
	meter, err := NewMeter(st, cfg.MonthlyBudgetUSD)
	if err != nil {
		return nil, err
	}
	// Local tier concurrency is operator-configurable (clamped in Workers());
	// the boot warning is surfaced by the daemon, not here. Cloud is pinned
	// at one worker (FR-008).
	localSlots, _ := cfg.Local.Workers()
	localToolMode, _ := cfg.Local.ToolModeResolved()
	o := &Orchestrator{
		cfg:   cfg,
		meter: meter,
		done:  make(chan struct{}),
		tiers: map[Tier]*tier{
			TierLocal: {name: TierLocal, caller: newOpenAICompat(cfg.Local.Endpoint, cfg.Local.Model, cfg.Local.APIKey,
				resolveReasoningEffort(cfg.Local.ReasoningEffort, "none"), localToolMode),
				health: &tierHealth{}, queue: make(chan job, queueCap), prio: make(chan job, queueCap),
				slots: localSlots,
				est:   cognition.NewEstimator(cognition.SeedFor(nil, string(TierLocal)))},
			TierCloud: {name: TierCloud, caller: newCloudCaller(cfg.Cloud),
				health: &tierHealth{}, queue: make(chan job, queueCap), prio: make(chan job, queueCap),
				slots: 1,
				est:   cognition.NewEstimator(cognition.SeedFor(nil, string(TierCloud)))},
		},
	}
	// One worker goroutine per slot: N identical copies of the existing loop
	// drain the same two channels, preserving every per-job invariant while
	// unlocking concurrency (TASK-45 R1).
	for _, t := range o.tiers {
		for i := 0; i < t.slots; i++ {
			go o.worker(t)
		}
	}
	return o, nil
}

func (o *Orchestrator) Close() { o.closeOnce.Do(func() { close(o.done) }) }

// SeedCalibration re-seeds both tiers' estimators from a calibration
// profile (nil = keep bootstrap defaults). Called once at daemon start,
// before traffic.
func (o *Orchestrator) SeedCalibration(p *cognition.Profile) {
	for name, t := range o.tiers {
		t.est = cognition.NewEstimator(cognition.SeedFor(p, string(name)))
	}
}

// SecondsPerPoint is the live estimate for a tier — the router's bridge
// from Fibonacci points to this deployment's wall clock.
func (o *Orchestrator) SecondsPerPoint(tierName Tier) float64 {
	if t, ok := o.tiers[tierName]; ok {
		return t.est.Estimate()
	}
	return cognition.SeedFor(nil, string(tierName))
}

// feedEstimate reports one completed-cognition wall time to a tier's
// seconds-per-point estimator, normalized by the kind's registered point cost,
// and fires the recalibrate hook (in its own goroutine) on a spike-rate breach.
// Shared by the per-call worker path (TASK-32) and the whole-loop
// ObserveCognition seam (TASK-52), so both feed the estimator identically.
func (o *Orchestrator) feedEstimate(t *tier, kind Kind, millis int64) {
	dc, ok := cognition.ClassForKind(string(kind))
	if !ok || dc.Points <= 0 {
		return
	}
	if t.est.Sample(float64(millis) / 1000 / float64(dc.Points)) {
		est, rate, _, _ := t.est.Stats()
		o.recalMu.Lock()
		hook := o.recalibrate
		o.recalMu.Unlock()
		if hook != nil {
			go hook(t.name, est, rate)
		}
	}
}

// ObserveCognition reports one whole-cognition wall-time observation to the
// tier estimator (TASK-52). It is the agent tool-use loop's replacement for
// per-call worker feeding: the loop marks every internal Submit SkipObserve so
// no fractional per-round samples reach the estimator, then reports the summed
// loop duration here exactly once. Unknown kinds are ignored (no tier). Safe
// for concurrent use — the estimator and hook lock are the same ones the
// worker uses.
func (o *Orchestrator) ObserveCognition(kind Kind, totalMillis int64) {
	tierName, ok := routing[kind]
	if !ok {
		return
	}
	o.feedEstimate(o.tiers[tierName], kind, totalMillis)
}

// SetRecalibrateHook installs the drift-signal consumer (the mind). The
// hook runs in its own goroutine and must be idempotent per breach episode
// — the estimator already fires once per breach.
func (o *Orchestrator) SetRecalibrateHook(fn func(tier Tier, estimate, spikeRate float64)) {
	o.recalMu.Lock()
	o.recalibrate = fn
	o.recalMu.Unlock()
}

// Submit routes a request to its tier and blocks until the result (or the
// caller's ctx expires). Admission control is immediate: budget ceiling,
// open circuit, and full queue all fail fast rather than piling work up —
// that backpressure is what lets local throughput cap sim speed later.
func (o *Orchestrator) Submit(ctx context.Context, req Request) (Response, error) {
	tierName, ok := routing[req.Kind]
	if !ok {
		return Response{}, fmt.Errorf("%w: %q", ErrUnknownKind, req.Kind)
	}
	t := o.tiers[tierName]

	if tierName == TierCloud && !o.meter.Allow() {
		return Response{}, ErrBudgetExhausted
	}
	if !t.health.admit() {
		return Response{}, ErrTierDown
	}

	// Conversations are interactive — a turn mid-dialogue must not wait
	// behind a backlog of planner thoughts (which tolerate staleness; the
	// reflex grace covers them). Everything else rides the normal queue.
	// Best-effort work is the opposite extreme: admitted only
	// when a slot is free, refused instantly otherwise. With N local
	// workers, "free slot" means no queued work AND at least one idle
	// worker (inflight < slots) — a non-empty queue already implies every
	// slot is busy, so the queue checks remain the fast-path refusal (R3).
	if req.BestEffort && (len(t.queue) > 0 || len(t.prio) > 0 || t.inflight.Load() >= int32(t.slots)) {
		return Response{}, ErrTierBusy
	}
	q := t.queue
	if req.Kind == KindConversation {
		q = t.prio
	}
	j := job{ctx: ctx, req: req, reply: make(chan result, 1)}
	select {
	case q <- j:
	default:
		return Response{}, ErrQueueFull
	}
	select {
	case res := <-j.reply:
		return res.resp, res.err
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-o.done:
		return Response{}, ErrClosed
	}
}

func (o *Orchestrator) worker(t *tier) {
	for {
		// Two-level priority: drain interactive work first.
		var j job
		select {
		case <-o.done:
			return
		case j = <-t.prio:
		default:
			select {
			case <-o.done:
				return
			case j = <-t.prio:
			case j = <-t.queue:
			}
		}
		// Count this job in-flight the instant it leaves the channel; the
		// deferred decrement fires on every reply path (stale-skip, provider
		// error, meter error, success), keeping 0 ≤ inflight ≤ slots for the
		// slot-aware best-effort admission check in Submit (TASK-45 R3).
		t.inflight.Add(1)
		func() {
			defer t.inflight.Add(-1)
			// A job whose caller already gave up (its ctx expired in the
			// queue) is starvation, not model failure: skip it without
			// touching the model or the circuit. Otherwise every planner
			// that times out behind a long conversation both wastes a
			// generation and strikes the breaker — a busy-but-healthy
			// model gets declared down.
			if j.ctx.Err() != nil {
				j.reply <- result{err: j.ctx.Err()}
				return
			}
			start := time.Now()
			// Worker-side hard cap: no single call may wedge the tier,
			// regardless of the caller's context or transport behavior.
			callCtx, cancel := context.WithTimeout(j.ctx, workerCallCap)
			cr, err := t.caller.call(callCtx, j.req)
			cancel()
			if err != nil {
				// The circuit counts the model's failures, never the
				// caller's impatience: if the caller's own ctx died
				// mid-call, the model may be merely slow.
				if j.ctx.Err() == nil {
					t.health.fail()
				}
				j.reply <- result{err: fmt.Errorf("%s tier: %w", t.name, err)}
				return
			}
			t.health.succeed()
			resp := Response{
				Text:         cr.text,
				ToolCalls:    cr.toolCalls,
				Stop:         cr.stop,
				Tier:         t.name,
				InputTokens:  cr.inTok,
				OutputTokens: cr.outTok,
				Millis:       time.Since(start).Milliseconds(),
			}
			// Cognition-horizon sampling (TASK-32): completed calls feed the
			// tier's seconds-per-point estimate, normalized by the kind's
			// registered point cost. Successes only — a fast failure is not
			// a latency observation of completed thought, and the estimator's
			// spike rejection only guards the high side. A loop-internal call
			// opts out (TASK-52): the loop driver reports one whole-cognition
			// observation via ObserveCognition instead of N per-call fractions,
			// so per-call feeding here would skew sec/pt low and mis-arm the
			// suppression gate. Metering, admission, and the breaker are
			// untouched by the opt-out.
			if !j.req.SkipObserve {
				o.feedEstimate(t, j.req.Kind, resp.Millis)
			}
			if t.name == TierCloud {
				resp.Model = o.cfg.Cloud.Model
				resp.CostUSD = float64(cr.inTok)*o.cfg.Cloud.InputUSDPerMTok/1e6 +
					float64(cr.outTok)*o.cfg.Cloud.OutputUSDPerMTok/1e6
				if merr := o.meter.Add(resp.CostUSD); merr != nil {
					// Metering must never lose money silently: surface it.
					j.reply <- result{err: fmt.Errorf("spend meter: %w", merr)}
					return
				}
			} else {
				resp.Model = o.cfg.Local.Model
			}
			j.reply <- result{resp: resp}
		}()
	}
}

// StatusSnapshot reports tier health, queue depths, and spend.
func (o *Orchestrator) StatusSnapshot() Status {
	month, spent, budget := o.meter.Snapshot()
	local, cloud := o.tiers[TierLocal], o.tiers[TierCloud]
	return Status{
		Local: TierStatus{
			Model: o.cfg.Local.Model, Endpoint: o.cfg.Local.Endpoint,
			Up: !local.health.down(), Queue: len(local.queue),
		},
		Cloud: TierStatus{
			Model: o.cfg.Cloud.Model,
			Up:    !cloud.health.down(), Queue: len(cloud.queue),
		},
		Month: month, Spent: spent, Budget: budget,
	}
}
