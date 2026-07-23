package mind

// Cognition-horizon telemetry (TASK-32, specs/007-cognition-horizon): every
// thought the mind requests terminates in exactly one recorded outcome
// (FR-015), and prompts carry causality references (FR-020). Events ride the
// inject_social door as reducer no-ops; the loop's envelope re-stamp is the
// authoritative landing tick — payload ticks are the mind's own knowledge
// (its replica mirror), recorded for chain-walking.

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"unicode/utf8"

	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
)

// thoughtMeta is a job's identity + prediction, snapshotted at enqueue.
type thoughtMeta struct {
	job               string
	class             cognition.DecisionClass
	agent             int
	snapshotTick      int64
	generation        int64
	triggerSeq        int64
	predictedWallMs   int64
	predictedLandTick int64
}

// newMeta builds a job's telemetry identity from the replica's view at
// snapshot time (absorb goroutine only).
func (md *Mind) newMeta(class string, agent int, snapshotTick, triggerSeq int64, kind llm.Kind) thoughtMeta {
	dc, _ := cognition.ClassFor(class)
	spp := md.secondsPerPoint(kind)
	wallSec := float64(dc.Points) * spp
	m := thoughtMeta{
		job:             fmt.Sprintf("%s-%d-%d", class, agent, snapshotTick),
		class:           dc,
		agent:           agent,
		snapshotTick:    snapshotTick,
		triggerSeq:      triggerSeq,
		predictedWallMs: int64(wallSec * 1000),
	}
	if tps := md.replica.Speed.TicksPerSecond(); tps > 0 {
		m.predictedLandTick = snapshotTick + int64(wallSec*tps)
	}
	return m
}

// routeVerdict consults the deterministic router (FR-007) for a class at
// the replica's current speed. Absorb-goroutine only (reads the replica).
// Uncapped speed returns allow: production refuses max speed with an LLM
// configured at the door, so that branch exists only for pure-sim test
// harnesses — the pure Route() itself suppresses at uncapped.
func (md *Mind) routeVerdict(class string, kind llm.Kind) cognition.Verdict {
	dc, ok := cognition.ClassFor(class)
	if !ok {
		return cognition.Verdict{Allow: true, Class: class}
	}
	tps := md.replica.Speed.TicksPerSecond()
	if tps <= 0 {
		return cognition.Verdict{Allow: true, Class: class, Points: dc.Points, BudgetTicks: dc.BudgetTicks}
	}
	return cognition.Route(dc, tps, md.secondsPerPoint(kind))
}

// emitSuppressed records a router suppression: the single terminal record of
// a thought that was never attempted (no matching cog.thought). Fired from
// the absorb goroutine, so the injection detaches — telemetry must never
// block the absorb loop.
func (md *Mind) emitSuppressed(class string, agent int, snapshotTick int64, v cognition.Verdict) {
	b, _ := json.Marshal(sim.CogOutcomePayload{
		Job:   fmt.Sprintf("%s-%d-%d", class, agent, snapshotTick),
		Class: class, Agent: agent,
		Outcome: sim.OutcomeSuppressed, SnapshotTick: snapshotTick,
		PredictedWallMs: v.PredictedWallMs, Reason: v.Arithmetic,
	})
	e := store.Event{Type: "cog.outcome", Payload: b}
	go md.emitCog(e)
}

// estimating is the optional orchestrator surface for live per-provider
// seconds-per-point (spec 024 FR-013): the mind asks about a kind and gets the
// estimate of that kind's serving provider (its chain head), so a fast small
// model is never averaged with a slow quality model. Test fakes without the
// seam fall back to the bootstrap seed.
type estimating interface {
	EstimateForKind(kind llm.Kind) (string, float64, bool)
}

func (md *Mind) secondsPerPoint(kind llm.Kind) float64 {
	if e, ok := md.orch.(estimating); ok {
		if _, spp, ok := e.EstimateForKind(kind); ok {
			return spp
		}
	}
	// Fallback for a test fake lacking the seam: the pessimistic bootstrap seed
	// (the local/zero-priced constant is the slower of the two — fail toward
	// reflex, never toward stale action).
	return cognition.SeedFor(nil, "", true)
}

func cogThoughtEvent(m thoughtMeta) store.Event {
	b, _ := json.Marshal(sim.CogThoughtPayload{
		Job: m.job, Class: m.class.Class, Agent: m.agent,
		SnapshotTick: m.snapshotTick, Generation: m.generation,
		TriggerSeq: m.triggerSeq, Points: m.class.Points,
		PredictedWallMs: m.predictedWallMs, PredictedLandTick: m.predictedLandTick,
	})
	return store.Event{Type: "cog.thought", Payload: b}
}

func (md *Mind) cogOutcomeEvent(m thoughtMeta, outcome, reason string, actualWallMs int64) store.Event {
	return md.cogSceneOutcome(m, outcome, reason, actualWallMs, "", false)
}

// cogSceneOutcome is the conversation-scene variant (TASK-42): it carries the
// optional raw failed-reply text (parse failures only, bounded) and the
// retried flag (scene consumed ≥1 retry). The base cogOutcomeEvent delegates
// here with the extras zeroed, so every other call site is byte-identical.
func (md *Mind) cogSceneOutcome(m thoughtMeta, outcome, reason string, actualWallMs int64, raw string, retried bool) store.Event {
	landing := md.tick.Load()
	staleness := landing - m.snapshotTick
	if staleness < 0 {
		staleness = 0
	}
	b, _ := json.Marshal(sim.CogOutcomePayload{
		Job: m.job, Class: m.class.Class, Agent: m.agent,
		Outcome: outcome, SnapshotTick: m.snapshotTick,
		LandingTick: landing, StalenessTicks: staleness,
		PredictedWallMs: m.predictedWallMs, ActualWallMs: actualWallMs,
		Reason:  reason,
		Raw:     truncateRaw(raw),
		Retried: retried,
	})
	return store.Event{Type: "cog.outcome", Payload: b}
}

const (
	// rawReplyCap bounds a persisted failed reply (TASK-42): a 224-token reply
	// fits comfortably, and the marker keeps the durable record lean while
	// signalling the cut. The whole field (content + marker) stays ≤ cap.
	rawReplyCap    = 2048
	rawTruncMarker = "…[truncated]"
)

// truncateRaw bounds a raw model reply for persistence, cutting on a rune
// boundary so the stored text stays valid UTF-8 (data-model.md).
func truncateRaw(s string) string {
	if len(s) <= rawReplyCap {
		return s
	}
	cut := rawReplyCap - len(rawTruncMarker)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + rawTruncMarker
}

// verdictRequiresReason reports whether a verdict's cog.tool_call MUST carry a
// non-empty reason (contracts/events.md): every rejection and every read error
// is the queryable explanation AC#5 promises. The driver already sets Reason
// from the handler's ResultForModel for these; the emitter asserts the
// invariant so a blank explanation never reaches the durable log silently.
func verdictRequiresReason(v toolloop.Verdict) bool {
	switch v {
	case toolloop.VerdictRejectedGate, toolloop.VerdictRejectedCardinality,
		toolloop.VerdictRejectedUnknown, toolloop.VerdictRejectedMalformed,
		toolloop.VerdictReadError:
		return true
	default:
		return false
	}
}

// toolCallEvent converts one buffered CallRecord into its cog.tool_call event
// via the sim-side constructor (the shared payload authority — metatron converts
// its own CallRecord identically at T020). snapshotTick is the cognition's world
// tick (thoughtMeta.snapshotTick). The reason invariant is enforced here: an
// empty reason on a verdict that requires one is backfilled with the verdict
// name (so the AC#5 query never finds a blank explanation) and logged as the
// driver-contract violation it would be.
func (md *Mind) toolCallEvent(r toolloop.CallRecord, snapshotTick int64) store.Event {
	reason := r.Reason
	if reason == "" && verdictRequiresReason(r.Verdict) {
		reason = string(r.Verdict)
		log.Printf("mind: cog.tool_call %s ordinal %d verdict %s missing reason; backfilled", r.JobID, r.Ordinal, r.Verdict)
	}
	b, _ := json.Marshal(sim.NewCogToolCallPayload(
		r.JobID, r.Ordinal, r.Tool, r.Args,
		string(r.Verdict), reason, r.Tier, snapshotTick,
	))
	return store.Event{Type: "cog.tool_call", Payload: b}
}

// emitToolCalls lands a cognition's buffered CallRecords as cog.tool_call events
// (spec 017 FR-007, T018), one per record. Called on EVERY loop-termination path
// so a rejected / never-grounded call is recorded even when nothing landed. The
// records ride ONE all-or-nothing batch through the same telemetry door as
// cog.thought / cog.outcome (emitCog → InjectSocial) — a DEDICATED batch, so it
// neither reorders nor entangles with the grounding events the door already
// emitted during the loop, nor the terminal cog.outcome that follows. Events go
// out in ordinal order (the driver buffers them ordinal-dense already; sorted
// here so the mind's emission is correct independent of buffer order). An empty
// buffer emits nothing — no empty batch.
func (md *Mind) emitToolCalls(records []toolloop.CallRecord, snapshotTick int64) {
	if len(records) == 0 {
		return
	}
	ordered := append([]toolloop.CallRecord(nil), records...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Ordinal < ordered[j].Ordinal })
	events := make([]store.Event, 0, len(ordered))
	for _, r := range ordered {
		events = append(events, md.toolCallEvent(r, snapshotTick))
	}
	md.emitCog(events...)
}

// emitCog lands telemetry through the social door; a rejected batch is
// logged, never fatal (the world outlives its observability).
func (md *Mind) emitCog(events ...store.Event) {
	if md.social == nil || len(events) == 0 {
		return
	}
	if err := md.social.InjectSocial(events); err != nil {
		log.Printf("mind: telemetry rejected: %v", err)
	}
}

// RecalibrateSignal is the orchestrator's drift hook (installed by the
// daemon): the live estimator's spike rate breached threshold — record it. The
// hook is per provider now (spec 024 T009); the breaching provider's name rides
// the payload's Tier field, which stays named Tier because it is a recorded
// telemetry field (replay-relevant schema — untouched by the rename).
func (md *Mind) RecalibrateSignal(provider string, estimate, spikeRate float64) {
	b, _ := json.Marshal(sim.RecalibrationPayload{
		Tier: provider, EstimateSPerPt: estimate,
		SpikeRate: spikeRate, Window: cognition.WindowSize,
	})
	md.emitCog(store.Event{Type: "cog.recalibration_recommended", Payload: b})
}
