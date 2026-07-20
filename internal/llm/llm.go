// Package llm is the orchestrator for all model traffic (TASK-6): two
// tiers (local Ollama-style HTTP, cloud Anthropic), kind-based routing,
// bounded queues with backpressure, a persisted monthly spend meter with a
// hard ceiling, and per-tier circuit breakers so unreachable inference
// degrades the AI layer — never the simulation.
//
// The orchestrator lives entirely OUTSIDE the deterministic sim loop. LLM
// results reach the world only as recorded inputs (TASK-7's job), so replay
// never re-calls a model.
package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
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
	// KindMusing is best-effort interiority (TASK-21): admitted only when
	// the local tier is otherwise quiet, dropped without retry when not.
	KindMusing Kind = "musing"
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
	KindMusing:        TierLocal,
	KindMeeting:       TierLocal,
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
	// with ErrTierBusy whenever its tier has work waiting. Callers that
	// may not displace real cognition (musings) set this; their fairness
	// floor is the caller's business, not the orchestrator's.
	BestEffort bool `json:"best_effort,omitempty"`
}

type Response struct {
	Text         string  `json:"text"`
	Tier         Tier    `json:"tier"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Millis       int64   `json:"ms"`
}

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
}

// Orchestrator routes, queues, meters, and degrades. One per daemon.
type Orchestrator struct {
	cfg       Config
	meter     *Meter
	tiers     map[Tier]*tier
	done      chan struct{}
	closeOnce sync.Once
}

func New(cfg Config, st MeterStore) (*Orchestrator, error) {
	meter, err := NewMeter(st, cfg.MonthlyBudgetUSD)
	if err != nil {
		return nil, err
	}
	o := &Orchestrator{
		cfg:   cfg,
		meter: meter,
		done:  make(chan struct{}),
		tiers: map[Tier]*tier{
			TierLocal: {name: TierLocal, caller: newOpenAICompat(cfg.Local.Endpoint, cfg.Local.Model, cfg.Local.APIKey),
				health: &tierHealth{}, queue: make(chan job, queueCap), prio: make(chan job, queueCap)},
			TierCloud: {name: TierCloud, caller: newCloudCaller(cfg.Cloud),
				health: &tierHealth{}, queue: make(chan job, queueCap), prio: make(chan job, queueCap)},
		},
	}
	for _, t := range o.tiers {
		go o.worker(t)
	}
	return o, nil
}

func (o *Orchestrator) Close() { o.closeOnce.Do(func() { close(o.done) }) }

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
	// Best-effort work (musings) is the opposite extreme: admitted only
	// when nothing else is waiting, refused instantly otherwise.
	if req.BestEffort && (len(t.queue) > 0 || len(t.prio) > 0) {
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
		func() {
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
			text, inTok, outTok, err := t.caller.call(callCtx, j.req)
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
				Text:         text,
				Tier:         t.name,
				InputTokens:  inTok,
				OutputTokens: outTok,
				Millis:       time.Since(start).Milliseconds(),
			}
			if t.name == TierCloud {
				resp.Model = o.cfg.Cloud.Model
				resp.CostUSD = float64(inTok)*o.cfg.Cloud.InputUSDPerMTok/1e6 +
					float64(outTok)*o.cfg.Cloud.OutputUSDPerMTok/1e6
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
