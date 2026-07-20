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

	"github.com/evanstern/script-world/internal/cognition"
	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
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

// estimating is the optional orchestrator surface for live
// seconds-per-point; test fakes without it fall back to bootstrap seeds.
type estimating interface {
	SecondsPerPoint(t llm.Tier) float64
}

func (md *Mind) secondsPerPoint(kind llm.Kind) float64 {
	tierName, _ := llm.TierFor(kind)
	if e, ok := md.orch.(estimating); ok {
		return e.SecondsPerPoint(tierName)
	}
	return cognition.SeedFor(nil, string(tierName))
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
		Reason: reason,
	})
	return store.Event{Type: "cog.outcome", Payload: b}
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
// daemon): the live estimator's spike rate breached threshold — record it.
func (md *Mind) RecalibrateSignal(tierName llm.Tier, estimate, spikeRate float64) {
	b, _ := json.Marshal(sim.RecalibrationPayload{
		Tier: string(tierName), EstimateSPerPt: estimate,
		SpikeRate: spikeRate, Window: cognition.WindowSize,
	})
	md.emitCog(store.Event{Type: "cog.recalibration_recommended", Payload: b})
}
