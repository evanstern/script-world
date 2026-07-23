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

// Tier is the retiring routing concept (decision-5): pricing (zero vs nonzero)
// is now the only local-vs-cloud distinction. The type survives as the
// estimator/calibration key and the legacy provider names ("local"/"cloud"),
// and rides Response.Tier plus the recalibrate hook until those consumers move
// per provider in later slices of spec 024.
type Tier string

const (
	TierLocal Tier = "local"
	TierCloud Tier = "cloud"
)

// acceptedKinds is the static set of call kinds the orchestrator accepts. The
// routes table is boot-validated against it in both directions (FR-003
// completeness): every accepted kind must have a route, and every route must
// name an accepted kind — so an unregistered kind can never reach a model, and
// a config typo dies at boot rather than at runtime.
var acceptedKinds = map[Kind]struct{}{
	KindPlanner:       {},
	KindConversation:  {},
	KindConsolidation: {},
	KindNarrator:      {},
	KindDrama:         {},
	KindMetatron:      {},
	KindMeeting:       {},
}

// Kinds returns every call kind the orchestrator accepts, sorted — the
// cognition registry's completeness gate (FR-002) iterates this at daemon
// start so an unregistered kind can never reach a model at runtime.
func Kinds() []Kind {
	out := make([]Kind, 0, len(acceptedKinds))
	for k := range acceptedKinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var (
	ErrUnknownKind     = errors.New("unknown call kind")
	ErrUnknownProvider = errors.New("unknown provider (config drift: a pin named an undeclared provider)")
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
	// Provider optionally pins the call to a named declared provider (spec 024
	// R3), bypassing chain routing while honoring that provider's admission
	// (breaker, wallet if priced, queue). Empty = route by the kind's chain
	// (every existing caller unchanged). An unknown name → ErrUnknownProvider.
	// The conversation layer stamps this to keep a scene on one provider (US3);
	// tests and the CLI one-shot use it to force a provider.
	Provider string `json:"provider,omitempty"`
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
	Text string `json:"text"`
	// Provider names the serving provider — always set (FR-011). Tier is the
	// legacy alias (= provider name), retained so telemetry/CLI consumers that
	// still read it keep working until they move per provider in a later slice.
	Provider     string  `json:"provider"`
	Tier         Tier    `json:"tier"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Millis       int64   `json:"ms"`
	// Skipped records, in chain order, the candidates the dispatch walk passed
	// over before the serving provider accepted the call (spec 024 US3): each is
	// a {Provider, Reason} pair with a mechanical, observable reason. Empty on a
	// head dispatch (the common case) and on a pinned / no_fallback route (which
	// never walk). Operator-facing only — routing carries no game-state meaning.
	Skipped []RouteSkip `json:"skipped,omitempty"`
	// --- agent tool-use loop transport (TASK-52; additive) ---
	// ToolCalls holds the calls the model emitted this round, in emission
	// order; nil for a plain-text reply. Stop is the provider's stop reason,
	// letting the loop driver tell "model finished" from "model wants results".
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Stop      StopReason `json:"stop,omitempty"`
}

// RouteSkip is one candidate the dispatch walk passed over and why (spec 024
// US3, data-model "Chain-walk admission"). Reasons are mechanical and
// observable — never a score or a judgment: a chain is the operator's ordered
// ruling and a skip is a deterministic admission fact.
type RouteSkip struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

// Chain-walk skip reasons (data-model.md): the only four conditions that make a
// candidate inadmissible at dispatch. circuit-open and queue-full apply to every
// call; wallet-exhausted applies to priced candidates only; busy applies to
// best-effort calls only (they additionally require an idle slot and empty
// queues).
const (
	SkipCircuitOpen     = "circuit-open"
	SkipWalletExhausted = "wallet-exhausted"
	SkipQueueFull       = "queue-full"
	SkipBusy            = "busy"
)

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

// ProviderStatus is one provider's operator-facing row (FR-012,
// contracts/status.md): one shape for legacy and v2 worlds alike. Contended and
// SpentUSD are wired in later slices (US5 leases, US4 attribution); this slice
// reports them zero.
type ProviderStatus struct {
	Name      string  `json:"name"`
	Model     string  `json:"model"`
	Endpoint  string  `json:"endpoint,omitempty"`
	Up        bool    `json:"up"`
	Queue     int     `json:"queue"`
	Inflight  int     `json:"inflight"`
	Slots     int     `json:"slots"`
	Contended bool    `json:"contended"`
	SpentUSD  float64 `json:"spent_usd"`
}

// Status feeds the protocol status shape and the TUI. The per-provider table
// replaces the fixed local/cloud pair — legacy worlds simply show two rows
// (named local, cloud). Providers is sorted by Name for a deterministic marshal.
type Status struct {
	Providers []ProviderStatus `json:"providers"`
	Month     string           `json:"month"`
	Spent     float64          `json:"spent_usd"`
	Budget    float64          `json:"budget_usd"`
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

// provider is one declared model source and its private machinery (FR-005):
// each owns a full instance of what every tier owned before — bounded queue +
// interactive priority lane, N worker slots, a circuit breaker, and a live
// latency estimator — with unchanged semantics, now per named provider rather
// than per fixed tier (decision-5).
type provider struct {
	name   string
	cfg    ProviderConfig
	caller caller
	health *tierHealth
	queue  chan job
	prio   chan job // interactive work (conversations) jumps the line
	// slots is the provider's concurrent capacity: the number of worker
	// goroutines draining its channels (TASK-45), from its clamped parallel.
	slots int
	// inflight counts jobs a worker has dequeued and not yet replied to —
	// incremented at dequeue, decremented on every reply path. It drives
	// slot-aware best-effort admission: 0 ≤ inflight ≤ slots.
	inflight atomic.Int32
	// est is the live seconds-per-point estimate for this provider (TASK-32):
	// the worker is the one place every call's true duration is observed,
	// so it feeds the estimator; the mind reads it to route.
	est *cognition.Estimator
}

// priced reports whether a provider bills for traffic — the surviving budget
// distinction (decision-5). Zero-priced providers are never budget-refused.
func (p *provider) priced() bool {
	return p.cfg.InputUSDPerMTok > 0 || p.cfg.OutputUSDPerMTok > 0
}

// route is one call kind's resolved ordered chain plus the no-fallback flag,
// built once at New() from validated config and immutable thereafter. This
// slice dispatches to chain[0] (the head) only; the admissible-head walk and
// fallback arrive in US3.
type route struct {
	chain      []*provider
	noFallback bool
}

// Orchestrator routes, queues, meters, and degrades. One per daemon.
type Orchestrator struct {
	cfg       Config
	meter     *Meter
	providers map[string]*provider
	routes    map[Kind]route
	done      chan struct{}
	closeOnce sync.Once

	// recalibrate is invoked (in its own goroutine) when a provider's estimator
	// first breaches the spike-rate threshold — the mind turns it into a
	// cog.recalibration_recommended telemetry event. It carries the serving
	// provider's name (spec 024 T009: the hook is per provider, not per tier).
	recalMu     sync.Mutex
	recalibrate func(provider string, estimate, spikeRate float64)
}

func New(cfg Config, st MeterStore) (*Orchestrator, error) {
	// resolveRegistry normalizes both config shapes (legacy local/cloud or the
	// v2 registry) into the validated provider set + kind→chain routes. A direct
	// caller that hands New() a malformed registry gets the same boot error
	// LoadConfig would raise — New() never builds machinery from an invalid map.
	pcs, rcs, err := cfg.resolveRegistry()
	if err != nil {
		return nil, err
	}
	// The meter learns the declared provider roster so it can reload each one's
	// persisted per-provider attribution at open and enumerate them in the
	// snapshot (spec 024 US4). The total key still governs the one wallet.
	names := make([]string, 0, len(pcs))
	for name := range pcs {
		names = append(names, name)
	}
	meter, err := NewMeter(st, cfg.MonthlyBudgetUSD, names)
	if err != nil {
		return nil, err
	}
	o := &Orchestrator{
		cfg:       cfg,
		meter:     meter,
		done:      make(chan struct{}),
		providers: make(map[string]*provider, len(pcs)),
		routes:    make(map[Kind]route, len(rcs)),
	}
	// Each provider owns a full instance of the per-tier machinery (FR-005),
	// with worker slots from its clamped parallel (warn discarded here — the
	// daemon surfaces boot warnings; the clamp itself is the invariant).
	for name, pc := range pcs {
		slots, _ := pc.workers(name)
		o.providers[name] = &provider{
			name: name, cfg: pc, caller: newProviderCaller(pc),
			health: &tierHealth{},
			queue:  make(chan job, queueCap), prio: make(chan job, queueCap),
			slots: slots,
			// Cold-start seed by pricing class (spec 024 R5): a zero-priced
			// provider bootstraps local, a priced one cloud (SeedCalibration
			// re-seeds from a recorded profile before traffic when present).
			est: cognition.NewEstimator(cognition.SeedFor(nil, name, pc.zeroPriced())),
		}
	}
	// Resolve each route's provider names to the live provider instances; the
	// config was validated so every name is present.
	for kind, rc := range rcs {
		chain := make([]*provider, 0, len(rc.Chain))
		for _, name := range rc.Chain {
			chain = append(chain, o.providers[name])
		}
		o.routes[kind] = route{chain: chain, noFallback: rc.NoFallback}
	}
	// One worker goroutine per slot: N identical copies of the existing loop
	// drain the same two channels, preserving every per-job invariant while
	// unlocking concurrency (TASK-45 R1).
	for _, p := range o.providers {
		for i := 0; i < p.slots; i++ {
			go o.worker(p)
		}
	}
	return o, nil
}

func (o *Orchestrator) Close() { o.closeOnce.Do(func() { close(o.done) }) }

// SeedCalibration re-seeds every provider's estimator from a calibration
// profile (nil = keep bootstrap defaults), keyed by provider name — legacy
// tier-keyed profiles keep matching the derived local/cloud providers by name.
// A provider with no recorded entry bootstraps by its pricing class (zero-priced
// → local constant, priced → cloud constant, spec 024 R5). Called once at daemon
// start, before traffic.
func (o *Orchestrator) SeedCalibration(p *cognition.Profile) {
	for name, pv := range o.providers {
		pv.est = cognition.NewEstimator(cognition.SeedFor(p, name, pv.cfg.zeroPriced()))
	}
}

// ProviderNames returns every declared provider name, sorted — the operator-
// facing roster for status/telemetry surfaces.
func (o *Orchestrator) ProviderNames() []string {
	out := make([]string, 0, len(o.providers))
	for name := range o.providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// admissibleHead returns the chain candidate a kind currently resolves to: the
// first provider whose breaker is closed and (if priced) whose wallet has not
// hit the ceiling — the same stable admission the dispatch walk applies, minus
// the transient queue/busy checks so the answer is a routing statement, not a
// snapshot of momentary load. When every candidate is inadmissible it falls back
// to the chain head (a routing seam must always name a provider). This is a pure
// read: it uses the non-mutating breaker check (down()), never admit(), so the
// mind may call it on every routing decision without consuming a half-open probe.
func (o *Orchestrator) admissibleHead(rt route) *provider {
	for _, p := range rt.chain {
		if p.priced() && !o.meter.Allow() {
			continue
		}
		if p.health.down() {
			continue
		}
		return p
	}
	return rt.chain[0]
}

// ResolveProvider names the provider a kind currently resolves to (FR-013): a
// dry chain-walk returning the current admissible head (research R3). The
// conversation layer resolves a scene's provider once through this seam and pins
// every turn to it, so a scene never switches voices mid-dialogue.
func (o *Orchestrator) ResolveProvider(kind Kind) (string, error) {
	rt, ok := o.routes[kind]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	return o.admissibleHead(rt).name, nil
}

// EstimateForKind returns the serving provider's name and its live seconds-per-
// point estimate for a kind — the cognition horizon's provider-granular seam
// (FR-013), so a fast small model is never averaged with a slow quality model.
// It reports the CURRENT ADMISSIBLE chain head (deterministic: the first
// candidate passing the breaker/wallet checks, falling back to the chain head
// when none is admissible), so the mind budgets against the provider that would
// actually serve the next call. ok is false for an unknown kind.
func (o *Orchestrator) EstimateForKind(kind Kind) (string, float64, bool) {
	rt, ok := o.routes[kind]
	if !ok {
		return "", 0, false
	}
	head := o.admissibleHead(rt)
	return head.name, head.est.Estimate(), true
}

// feedEstimate reports one completed-cognition wall time to a tier's
// seconds-per-point estimator, normalized by the kind's registered point cost,
// and fires the recalibrate hook (in its own goroutine) on a spike-rate breach.
// Shared by the per-call worker path (TASK-32) and the whole-loop
// ObserveCognition seam (TASK-52), so both feed the estimator identically.
func (o *Orchestrator) feedEstimate(p *provider, kind Kind, millis int64) {
	dc, ok := cognition.ClassForKind(string(kind))
	if !ok || dc.Points <= 0 {
		return
	}
	if p.est.Sample(float64(millis) / 1000 / float64(dc.Points)) {
		est, rate, _, _ := p.est.Stats()
		o.recalMu.Lock()
		hook := o.recalibrate
		o.recalMu.Unlock()
		if hook != nil {
			// The hook fires per provider: the breaching provider's own name
			// rides the drift signal (observational telemetry, not routing).
			go hook(p.name, est, rate)
		}
	}
}

// ObserveCognition reports one whole-cognition wall-time observation to a
// provider's estimator (TASK-52). It is the agent tool-use loop's replacement
// for per-call worker feeding: the loop marks every internal Submit SkipObserve
// so no fractional per-round samples reach the estimator, then reports the summed
// loop duration here exactly once. The observation feeds the NAMED serving
// provider's estimator (spec 024 T009: the loop passes Response.Provider, so a
// fallback that served a different provider than the chain head is measured on
// the estimator that actually did the work); an empty name (or one that no
// longer resolves) falls back to the kind's chain head. Unknown kinds are
// ignored. Safe for concurrent use — the estimator and hook lock are the same
// ones the worker uses.
func (o *Orchestrator) ObserveCognition(kind Kind, provider string, totalMillis int64) {
	rt, ok := o.routes[kind]
	if !ok {
		return
	}
	p := rt.chain[0]
	if provider != "" {
		if pv, ok := o.providers[provider]; ok {
			p = pv
		}
	}
	o.feedEstimate(p, kind, totalMillis)
}

// SetRecalibrateHook installs the drift-signal consumer (the mind). The
// hook runs in its own goroutine and must be idempotent per breach episode
// — the estimator already fires once per breach.
func (o *Orchestrator) SetRecalibrateHook(fn func(provider string, estimate, spikeRate float64)) {
	o.recalMu.Lock()
	o.recalibrate = fn
	o.recalMu.Unlock()
}

// Submit routes a request along its kind's chain and blocks until the serving
// provider replies (or the caller's ctx expires). Admission control is immediate
// per candidate — budget ceiling, open circuit, best-effort busy, and full queue
// all fail fast rather than piling work up — that backpressure is what lets local
// throughput cap sim speed. Fallback is chain-walking (spec 024 US3): the walk
// visits each candidate in the operator's declared order and dispatches to the
// first admissible one, recording the ordered skips it passed over.
func (o *Orchestrator) Submit(ctx context.Context, req Request) (Response, error) {
	// Resolve the candidate set. An explicit Request.Provider pin (R3) names a
	// declared provider and bypasses chain-walking (scene pinning, the CLI, and
	// tests use it) while honoring ALL of that provider's admission checks; a
	// no_fallback route considers only its head; otherwise the whole chain walks.
	var candidates []*provider
	if req.Provider != "" {
		p, ok := o.providers[req.Provider]
		if !ok {
			return Response{}, fmt.Errorf("%w: %q", ErrUnknownProvider, req.Provider)
		}
		candidates = []*provider{p}
	} else {
		rt, ok := o.routes[req.Kind]
		if !ok {
			return Response{}, fmt.Errorf("%w: %q", ErrUnknownKind, req.Kind)
		}
		if rt.noFallback {
			candidates = rt.chain[:1]
		} else {
			candidates = rt.chain
		}
	}

	// Walk the candidates in order. A candidate is skipped ONLY for a mechanical
	// admission fact, in the data-model's order: wallet ceiling (priced only),
	// open circuit, best-effort busy, then full queue. The first admissible
	// candidate is dispatched to; every skip that preceded it is recorded on the
	// response. When none is admissible the walk returns the HEAD's refusal error
	// (the first skip is the head's, since the walk records in chain order).
	var skipped []RouteSkip
	for _, t := range candidates {
		// Budget throttles priced providers before the call (never after the
		// money is spent); zero-priced providers are never budget-refused
		// (decision-5).
		if t.priced() && !o.meter.Allow() {
			skipped = append(skipped, RouteSkip{Provider: t.name, Reason: SkipWalletExhausted})
			continue
		}
		if !t.health.admit() {
			skipped = append(skipped, RouteSkip{Provider: t.name, Reason: SkipCircuitOpen})
			continue
		}
		// Best-effort work is admitted only when a slot is free, refused
		// instantly otherwise. With N workers, "free slot" means no queued work
		// AND at least one idle worker (inflight < slots) — a non-empty queue
		// already implies every slot is busy, so the queue checks are the
		// fast-path refusal (R3).
		if req.BestEffort && (len(t.queue) > 0 || len(t.prio) > 0 || t.inflight.Load() >= int32(t.slots)) {
			skipped = append(skipped, RouteSkip{Provider: t.name, Reason: SkipBusy})
			continue
		}
		// Conversations are interactive — a turn mid-dialogue must not wait
		// behind a backlog of planner thoughts (which tolerate staleness; the
		// reflex grace covers them). Everything else rides the normal queue.
		q := t.queue
		if req.Kind == KindConversation {
			q = t.prio
		}
		j := job{ctx: ctx, req: req, reply: make(chan result, 1)}
		select {
		case q <- j:
		default:
			skipped = append(skipped, RouteSkip{Provider: t.name, Reason: SkipQueueFull})
			continue
		}
		// Accepted: this candidate serves the call. Post-dispatch failure is
		// FINAL — a provider error here is never re-dispatched down the chain
		// (the model may have already done partial work; re-running it would
		// double-spend and double-act). The recorded skips ride the response.
		select {
		case res := <-j.reply:
			if res.err != nil {
				return res.resp, res.err
			}
			res.resp.Skipped = skipped
			return res.resp, nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-o.done:
			return Response{}, ErrClosed
		}
	}

	// Every candidate was inadmissible: refuse with the head's reason (skipped[0]
	// is the head — the walk records in chain order, and a pin/no_fallback set has
	// exactly one candidate, its own head).
	return Response{}, refusalFor(skipped[0].Reason)
}

// refusalFor maps a chain-head skip reason onto the admission-ladder sentinel
// the caller (and the toolloop's terminationForSubmitErr) already switch on, so
// an all-inadmissible walk refuses exactly as a single-tier refusal did before
// fallback existed — legacy single-entry chains are byte-identical.
func refusalFor(reason string) error {
	switch reason {
	case SkipWalletExhausted:
		return ErrBudgetExhausted
	case SkipCircuitOpen:
		return ErrTierDown
	case SkipBusy:
		return ErrTierBusy
	default: // SkipQueueFull
		return ErrQueueFull
	}
}

func (o *Orchestrator) worker(t *provider) {
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
				Text:      cr.text,
				ToolCalls: cr.toolCalls,
				Stop:      cr.stop,
				// Provider always names the serving provider (FR-011); Tier is
				// the legacy alias (= provider name) for consumers not yet moved.
				Provider:     t.name,
				Tier:         Tier(t.name),
				Model:        t.cfg.Model,
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
			// Every call is priced by its serving provider's rates; a priced
			// provider bills the wallet (zero-priced providers cost nothing and
			// never touch the meter — byte-identical to the local tier today).
			// Per-provider attribution keys land in US4; this slice writes only
			// the total via the provider-scoped Add signature.
			if t.priced() {
				resp.CostUSD = float64(cr.inTok)*t.cfg.InputUSDPerMTok/1e6 +
					float64(cr.outTok)*t.cfg.OutputUSDPerMTok/1e6
				if merr := o.meter.Add(t.name, resp.CostUSD); merr != nil {
					// Metering must never lose money silently: surface it.
					j.reply <- result{err: fmt.Errorf("spend meter: %w", merr)}
					return
				}
			}
			j.reply <- result{resp: resp}
		}()
	}
}

// StatusSnapshot reports each provider's health, queue depth, and worker
// occupancy plus its attributed spend and the global total (FR-012). Rows are
// sorted by name for a deterministic marshal; SpentUSD is the provider's share
// of the month's spend (spec 024 US4) — the total minus Σ(rows) is the
// (unattributed) remainder a surface renders. Contended is wired in US5.
func (o *Orchestrator) StatusSnapshot() Status {
	month, spent, budget, perProvider := o.meter.Snapshot()
	rows := make([]ProviderStatus, 0, len(o.providers))
	for _, p := range o.providers {
		rows = append(rows, ProviderStatus{
			Name: p.name, Model: p.cfg.Model, Endpoint: p.cfg.Endpoint,
			Up: !p.health.down(), Queue: len(p.queue),
			Inflight: int(p.inflight.Load()), Slots: p.slots,
			Contended: false, SpentUSD: perProvider[p.name],
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return Status{Providers: rows, Month: month, Spent: spent, Budget: budget}
}
