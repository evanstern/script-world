package mind

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// The nightly consolidation driver (TASK-9): when a villager sleeps, one
// cloud-tier call digests the day's episodic buffer into promotions, fades,
// a day-gist, belief revisions, and a rewritten self-narrative. The output
// passes the deterministic firewall validator, then lands as ONE atomic
// whitelisted batch — or a rejection marker lands, or (transport failure)
// nothing lands and the next sleep retries. The world never waits.

const (
	// consolidateCallTimeout bounds one cloud call; the night is hours long,
	// so this is generous rather than interactive.
	consolidateCallTimeout = 3 * time.Minute
	// maxBufferSent caps prompt size; overflow is truncated oldest-first for
	// the call only (state keeps everything).
	maxBufferSent = 60
)

// consolJob is the immutable snapshot a consolidation runs against —
// everything is copied at enqueue time so the ticking replica can't race it.
type consolJob struct {
	agent     int
	name      string
	personaMD string
	anchor    string
	drift     []string
	night     int64
	sleepTick int64
	upTo      int64 // buffer high-water mark (whole buffer, sent or not)
	buffer    []sim.Memory
	held      []sim.Belief
	social    string
	narrative string
}

// maybeConsolidate is called from absorb on agent.slept. Guards are checked
// on the replica; due agents are snapshotted and queued for the single-
// flight worker.
func (md *Mind) maybeConsolidate(e store.Event) {
	if md.social == nil {
		return
	}
	var p sim.AgentPayload
	if json.Unmarshal(e.Payload, &p) != nil {
		return
	}
	if p.Agent < 0 || p.Agent >= sim.AgentCount {
		return
	}
	a := &md.replica.Agents[p.Agent]
	if !a.ConsolidationDue(e.Tick) || md.consolInFlight[p.Agent].Load() {
		return
	}
	night := sim.NightIndex(e.Tick)

	buffer := a.EpisodicBuffer()
	if len(buffer) == 0 {
		// Nothing to digest: close the night with a marker, spend no call.
		md.consolInFlight[p.Agent].Store(true)
		md.landMarker(consolJob{agent: p.Agent, name: a.Name, night: night, sleepTick: e.Tick},
			sim.ConsolidationSkippedEmpty, "", 0)
		return
	}

	job := consolJob{
		agent:     p.Agent,
		name:      a.Name,
		personaMD: md.personas[p.Agent],
		anchor:    persona.Anchors[a.Name],
		drift:     persona.DriftMarkers[a.Name],
		night:     night,
		sleepTick: e.Tick,
		upTo:      buffer[len(buffer)-1].Tick,
		buffer:    append([]sim.Memory(nil), buffer...),
		held:      append([]sim.Belief(nil), a.Beliefs...),
		social:    socialContext(md.replica, p.Agent),
		narrative: a.Narrative,
	}
	if len(job.buffer) > maxBufferSent {
		job.buffer = job.buffer[len(job.buffer)-maxBufferSent:] // newest kept
	}
	md.consolInFlight[p.Agent].Store(true)
	select {
	case md.consolQ <- job:
	default:
		// Queue full (should not happen with cap 8): drop the attempt; the
		// next sleep retries.
		md.consolInFlight[p.Agent].Store(false)
	}
}

// consolidateWorker drains the night's queue one call at a time.
func (md *Mind) consolidateWorker() {
	for {
		select {
		case <-md.done:
			return
		case job := <-md.consolQ:
			md.runConsolidation(job)
		}
	}
}

func (md *Mind) runConsolidation(job consolJob) {
	defer md.consolInFlight[job.agent].Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), consolidateCallTimeout)
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind:      llm.KindConsolidation,
		System:    consolidateSystemPrompt(job),
		Prompt:    consolidateUserPrompt(job),
		MaxTokens: 1024,
	})
	cancel()
	if err != nil {
		// Transport/tier failure: NO marker — the attempt never happened as
		// far as the ledger cares; the next sleep retries (FR-002).
		log.Printf("mind: consolidation %s night %d deferred: %v", job.name, job.night, err)
		return
	}

	out, err := parseConsolidation(resp.Text)
	if err != nil {
		md.landMarker(job, sim.ConsolidationRejected, "unparseable", resp.CostUSD)
		return
	}
	if verr := validateConsolidation(out, job.agent, job.buffer, job.held, job.anchor, job.drift); verr != nil {
		md.landMarker(job, sim.ConsolidationRejected, verr.Error(), resp.CostUSD)
		return
	}

	// Accepted: build the whole night as one atomic batch.
	var batch []store.Event
	add := func(typ string, payload any) {
		b, _ := json.Marshal(payload)
		batch = append(batch, store.Event{Type: typ, Payload: b})
	}
	for _, r := range out.Promote {
		add("agent.memory_promoted", sim.MemoryPromotedPayload{
			Agent: job.agent, MemTick: r.Tick, TextHash: r.Hash, Boost: 3})
	}
	for _, r := range out.Fade {
		add("agent.memory_faded", sim.MemoryFadedPayload{
			Agent: job.agent, MemTick: r.Tick, TextHash: r.Hash})
	}
	add("agent.memory_added", sim.MemoryAddedPayload{
		Agent: job.agent, Text: out.Gist, Salience: sim.SalDayGist, Subject: -1})
	for _, b := range out.Beliefs {
		add("agent.belief_revised", sim.BeliefRevisedPayload{
			Agent: job.agent, BeliefID: b.ID, Statement: b.Statement,
			Confidence: b.Confidence, Provenance: b.Provenance,
			Source: b.Source, Subject: b.Subject})
	}
	add("agent.narrative_set", sim.NarrativeSetPayload{Agent: job.agent, Text: out.Narrative})
	add("agent.consolidated", sim.ConsolidatedPayload{
		Agent: job.agent, Night: job.night, UpTo: job.upTo,
		Outcome:  sim.ConsolidationAccepted,
		Promoted: len(out.Promote), Faded: len(out.Fade), Beliefs: len(out.Beliefs),
		CostUSD: resp.CostUSD})

	if err := md.social.InjectSocial(batch); err != nil {
		log.Printf("mind: consolidation %s night %d injection rejected: %v", job.name, job.night, err)
		return
	}
	log.Printf("mind: consolidation %s night %d accepted (%d promoted, %d faded, %d beliefs, $%.4f)",
		job.name, job.night, len(out.Promote), len(out.Fade), len(out.Beliefs), resp.CostUSD)
}

// landMarker records a non-accepted outcome (rejected / skipped_empty) as a
// single-event batch. The buffer stays intact for the next night.
func (md *Mind) landMarker(job consolJob, outcome, reason string, cost float64) {
	defer md.consolInFlight[job.agent].Store(false)
	b, _ := json.Marshal(sim.ConsolidatedPayload{
		Agent: job.agent, Night: job.night, Outcome: outcome, Reason: reason, CostUSD: cost})
	if err := md.social.InjectSocial([]store.Event{{Type: "agent.consolidated", Payload: b}}); err != nil {
		log.Printf("mind: consolidation %s night %d marker rejected: %v", job.name, job.night, err)
		return
	}
	switch outcome {
	case sim.ConsolidationRejected:
		log.Printf("mind: consolidation %s night %d rejected (%s)", job.name, job.night, reason)
	case sim.ConsolidationSkippedEmpty:
		log.Printf("mind: consolidation %s night %d skipped (empty)", job.name, job.night)
	}
}

func consolidateSystemPrompt(job consolJob) string {
	return fmt.Sprintf(`You are the sleeping mind of %s, a villager. %s
Tonight you digest the day into durable memory. You may only: strengthen or
let fade the day's memories, keep one gist of the day, revise beliefs, and
rewrite your self-narrative — all strictly in %s's voice and nature.
Your nature is fixed: %s. You must restate it verbatim in the "nature" field.`,
		job.name, job.personaMD, job.name, job.anchor)
}

func consolidateUserPrompt(job consolJob) string {
	var b strings.Builder
	b.WriteString("Today's memories (reference them ONLY by their tick and hash):\n")
	for _, m := range job.buffer {
		fmt.Fprintf(&b, "- [tick %d #%s] (salience %d) %s\n", m.Tick, sim.MemoryHash(m.Text), m.Salience, m.Text)
	}
	if len(job.held) > 0 {
		b.WriteString("\nBeliefs you already hold:\n")
		for _, bl := range job.held {
			fmt.Fprintf(&b, "- [id %d] (confidence %d, %s) %s\n", bl.ID, bl.Confidence, bl.Provenance, bl.Statement)
		}
	}
	if job.social != "" {
		b.WriteString("\n" + job.social)
	}
	if job.narrative != "" {
		fmt.Fprintf(&b, "\nYour current self-narrative:\n%s\n", job.narrative)
	}
	fmt.Fprintf(&b, `
Reply with ONLY this JSON:
{"nature": "<your nature, restated verbatim>",
 "gist": "<one sentence remembering this day, your voice, max 240 chars>",
 "promote": [{"tick": <tick>, "hash": "<hash>"}],   // up to %d memories worth keeping sharp
 "fade": [{"tick": <tick>, "hash": "<hash>"}],      // up to %d trivial memories to let go
 "beliefs": [{"id": 0, "statement": "...", "confidence": 0-100, "provenance": "witnessed|told|inferred", "source": -1, "subject": -1}],  // up to %d; id 0 = new, or revise a held id; subject/source are villager numbers, -1 = none
 "narrative": "<who you are becoming, first person, your voice, max 1200 chars>"}`,
		maxPromotes, maxFades, maxBeliefEdits)
	return b.String()
}
