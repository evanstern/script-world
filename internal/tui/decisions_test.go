package tui

// Decision-trace projection + glossary tests (spec 020, TASK-63): ingest
// joins/bounds/fragments/attribution (contract §1), the verdict glossary
// sweep (§4), decisions sub-view rendering (§2, alongside villagers_test.go's
// key-routing tests), and the Metatron inline-verdict transcript path (§3).

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
)

// --- fixtures: build the three cog.* event types without going through a
// real mind/metatron cognition ---

func thoughtEvent(seq, tick int64, job, class string, agent int, triggerSeq int64) store.Event {
	b, _ := json.Marshal(sim.CogThoughtPayload{
		Job: job, Class: class, Agent: agent, SnapshotTick: tick, TriggerSeq: triggerSeq,
	})
	return store.Event{Seq: seq, Tick: tick, Type: "cog.thought", Payload: b}
}

func toolCallEvent(seq int64, job string, ordinal int, tool, verdict, reason string) store.Event {
	b, _ := json.Marshal(sim.CogToolCallPayload{
		Job: job, Ordinal: ordinal, Tool: tool, Verdict: verdict, Reason: reason, Tier: "cheap",
	})
	return store.Event{Seq: seq, Type: "cog.tool_call", Payload: b}
}

func outcomeEvent(seq int64, job, class string, agent int, outcome, reason string) store.Event {
	b, _ := json.Marshal(sim.CogOutcomePayload{
		Job: job, Class: class, Agent: agent, Outcome: outcome, Reason: reason,
	})
	return store.Event{Seq: seq, Type: "cog.outcome", Payload: b}
}

// --- T007: ingest joins/bounds/fragments ---

func TestIngestJoinsThoughtToolCallOutcomeByJob(t *testing.T) {
	dt := newDecisionTraces()
	names := []string{"Ash"}
	dt.ingest(thoughtEvent(1, 100, "planner-0-100", "planner", 0, 0), names, nil)
	// Out-of-order arrival (ordinal 2 before ordinal 1) — R2: calls must
	// still land ordinal-ordered regardless.
	dt.ingest(toolCallEvent(3, "planner-0-100", 2, "gather", "landed", ""), names, nil)
	dt.ingest(toolCallEvent(2, "planner-0-100", 1, "speak", "rejected_gate", "stale"), names, nil)
	dt.ingest(outcomeEvent(4, "planner-0-100", "planner", 0, "landed", ""), names, nil)

	chains := dt.chainsFor(0)
	if len(chains) != 1 {
		t.Fatalf("want 1 chain for agent 0, got %d", len(chains))
	}
	c := chains[0]
	if len(c.Calls) != 2 || c.Calls[0].Ordinal != 1 || c.Calls[1].Ordinal != 2 {
		t.Fatalf("calls not ordinal-ordered despite out-of-order arrival: %+v", c.Calls)
	}
	if c.Calls[0].Tool != "speak" || c.Calls[1].Tool != "gather" {
		t.Errorf("wrong call landed at each ordinal: %+v", c.Calls)
	}
	if c.Outcome != "landed" || c.Class != "planner" {
		t.Errorf("outcome/class not joined onto the chain: %+v", c)
	}
}

func TestIngestFragmentToolCallFirstAttributesViaJobIDParse(t *testing.T) {
	dt := newDecisionTraces()
	// No cog.thought ever arrives — folded into a pre-connect snapshot
	// (research D2, FR-008): attribution falls back to the job-ID parse.
	dt.ingest(toolCallEvent(1, "reflex-2-500", 1, "gather", "landed", ""), nil, nil)
	chains := dt.chainsFor(2)
	if len(chains) != 1 {
		t.Fatalf("expected job-ID parse to attribute to agent 2, chains(2)=%d", len(chains))
	}
	if len(chains[0].Calls) != 1 {
		t.Errorf("the triggering tool_call itself should still be recorded: %+v", chains[0])
	}
}

func TestIngestFragmentOutcomeFirstUsesOutcomePayloadAgent(t *testing.T) {
	dt := newDecisionTraces()
	dt.ingest(outcomeEvent(1, "reflex-5-500", "reflex", 5, "landed", ""), nil, nil)
	chains := dt.chainsFor(5)
	if len(chains) != 1 {
		t.Fatalf("expected the outcome payload's own Agent to attribute the chain, chains(5)=%d", len(chains))
	}
}

func TestIngestSkipsConversationJobs(t *testing.T) {
	dt := newDecisionTraces()
	dt.ingest(thoughtEvent(1, 100, "conversation-7", "conversation", 0, 0), nil, nil)
	dt.ingest(toolCallEvent(2, "conversation-7", 1, "speak", "landed", ""), nil, nil)
	dt.ingest(outcomeEvent(3, "conversation-7", "conversation", 0, "landed", ""), nil, nil)
	if _, ok := dt.byJob["conversation-7"]; ok {
		t.Error("a conversation-prefixed job must never be ingested (spec Assumptions)")
	}
}

func TestIngestMetatronToolCallAttributesToSentinel(t *testing.T) {
	dt := newDecisionTraces()
	dt.ingest(toolCallEvent(1, "turn-metatron-1000", 1, "grant_item", "landed", ""), nil, nil)
	chains := dt.chainsFor(metatronAgent)
	if len(chains) != 1 {
		t.Fatalf("expected turn-metatron- attribution to the Metatron sentinel, chains=%d", len(chains))
	}
	if len(dt.chainsFor(-1)) != 0 {
		t.Error("a metatron job must not also appear unattributed")
	}
}

func TestIngestSuppressionOutcomeOnlyNoThoughtNoCalls(t *testing.T) {
	dt := newDecisionTraces()
	dt.ingest(outcomeEvent(1, "meeting-0-900", "meeting", 0, sim.OutcomeSuppressed, "budget exhausted"), nil, nil)
	chains := dt.chainsFor(0)
	if len(chains) != 1 || !chains[0].Suppressed {
		t.Fatalf("expected a suppressed chain: %+v", chains)
	}
}

func TestIngestOutcomeAfterCallsIsNotSuppressed(t *testing.T) {
	dt := newDecisionTraces()
	dt.ingest(toolCallEvent(1, "reflex-0-100", 1, "gather", "landed", ""), nil, nil)
	dt.ingest(outcomeEvent(2, "reflex-0-100", "reflex", 0, "landed", ""), nil, nil)
	chains := dt.chainsFor(0)
	if len(chains) != 1 || chains[0].Suppressed {
		t.Fatalf("a chain with calls must never read as a router suppression: %+v", chains)
	}
}

func TestIngestCapEvictsOldestPerAgentFromBothIndexes(t *testing.T) {
	dt := newDecisionTraces()
	for i := 0; i < decisionChainCap+1; i++ {
		job := fmt.Sprintf("reflex-0-%d", i)
		dt.ingest(thoughtEvent(int64(i+1), int64(i), job, "reflex", 0, 0), nil, nil)
	}
	chains := dt.chainsFor(0)
	if len(chains) != decisionChainCap {
		t.Fatalf("want %d chains (cap), got %d", decisionChainCap, len(chains))
	}
	if _, ok := dt.byJob["reflex-0-0"]; ok {
		t.Error("the evicted (oldest) job must be gone from byJob too")
	}
	if want := fmt.Sprintf("reflex-0-%d", decisionChainCap); chains[0].Job != want {
		t.Errorf("chains must be most-recent-first: chains[0].Job = %q, want %q", chains[0].Job, want)
	}
}

func TestResolveStimulusCadence(t *testing.T) {
	got := resolveStimulus(0, nil, nil)
	if !strings.Contains(got, "cadence") {
		t.Errorf("triggerSeq 0 should read cadence-driven, got %q", got)
	}
}

func TestResolveStimulusRingHit(t *testing.T) {
	names := []string{"Ash"}
	ring := []store.Event{
		{Seq: 5, Tick: 60, Type: "agent.moved", Payload: json.RawMessage(`{"agent":0,"x":7,"y":8}`)},
	}
	got := resolveStimulus(5, ring, names)
	if !strings.Contains(got, "Ash") || !strings.Contains(got, "(7,8)") {
		t.Errorf("a ring hit should render the chronicle digest line, got %q", got)
	}
}

func TestResolveStimulusRingMiss(t *testing.T) {
	got := resolveStimulus(999, nil, nil)
	if !strings.Contains(got, "999") {
		t.Errorf("a ring miss should name the seq neutrally, got %q", got)
	}
}

func TestTracesResetOnReconnect(t *testing.T) {
	m := testModel(t)
	m.applyEvent(thoughtEvent(1, 100, "reflex-0-100", "reflex", 0, 0))
	if len(m.traces.chainsFor(0)) == 0 {
		t.Fatal("setup: expected a chain before reconnect")
	}
	mdl, _ := m.Update(connectedMsg{replica: sim.NewState(42, m.gameMap)})
	mm := mdl.(Model)
	if len(mm.traces.chainsFor(0)) != 0 {
		t.Error("the projection must reset wholesale on reconnect (contract R5)")
	}
}

// --- T008: glossary sweep (contract R16, mechanical proof of SC-002) ---

func TestVerdictGlossarySweepCoversEveryToolloopVerdict(t *testing.T) {
	for _, v := range []toolloop.Verdict{
		toolloop.VerdictLanded, toolloop.VerdictRejectedGate, toolloop.VerdictRejectedCardinality,
		toolloop.VerdictRejectedUnknown, toolloop.VerdictRejectedMalformed,
		toolloop.VerdictReadOK, toolloop.VerdictReadError, toolloop.VerdictUnlanded,
	} {
		phrase := verdictPhrase(string(v))
		if phrase == "" || phrase == string(v) || phrase == verdictFallback {
			t.Errorf("toolloop verdict %q: no distinct glossary phrase (got %q)", v, phrase)
		}
	}
}

func TestVerdictGlossarySweepCoversEverySimOutcome(t *testing.T) {
	for _, o := range []string{
		sim.OutcomeLanded, sim.OutcomeAdapted, sim.OutcomeRejectedStale, sim.OutcomeRejectedGuard,
		sim.OutcomeSuperseded, sim.OutcomeExpired, sim.OutcomeUnavailable, sim.OutcomeUnusable,
		sim.OutcomeSuppressed, sim.OutcomeRetried,
	} {
		phrase := verdictPhrase(o)
		if phrase == "" || phrase == o || phrase == verdictFallback {
			t.Errorf("sim outcome %q: no distinct glossary phrase (got %q)", o, phrase)
		}
	}
}

func TestVerdictGlossaryUnknownFallsBackSafely(t *testing.T) {
	if got := verdictPhrase("some_future_verdict_nobody_registered_yet"); got != verdictFallback {
		t.Errorf("an unrecognized verdict should fall back to the safe generic phrase, got %q", got)
	}
}

// --- T012: US1 rendering (villagerDecisionsBody, contract R9–R11) ---

func TestVillagerDecisionsBodyEmptyState(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	body := m.villagerDecisionsBody(m.width-6, m.height-6)
	if !strings.Contains(body, "no decisions recorded") {
		t.Errorf("empty state should say so plainly, not render a blank pane: %q", body)
	}
}

// TestVillagerDecisionsBodyOrderAndInProgress covers R9/FR-008: chains
// render most-recent-first, and a cognition with no cog.outcome yet reads
// as visibly non-terminal rather than pretending it landed.
func TestVillagerDecisionsBodyOrderAndInProgress(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	m.applyEvent(thoughtEvent(1, 100, "reflex-0-100", "reflex", 0, 0))
	m.applyEvent(thoughtEvent(2, 200, "reflex-0-200", "reflex", 0, 0))
	m.applyEvent(outcomeEvent(3, "reflex-0-100", "reflex", 0, "landed", "")) // only the OLDER job resolves

	body := m.villagerDecisionsBody(m.width-6, m.height-6)
	if !strings.Contains(body, "in progress — no outcome yet") {
		t.Errorf("the in-flight cognition must render honestly as non-terminal: %q", body)
	}
	idxNew := strings.Index(body, "in progress")
	idxOld := strings.Index(body, "outcome: went through")
	if idxNew < 0 || idxOld < 0 || idxNew > idxOld {
		t.Errorf("chains not most-recent-first (tick 200 should precede tick 100):\n%s", body)
	}
}

func TestVillagerDecisionsBodySuppressionRow(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	m.applyEvent(outcomeEvent(1, "meeting-0-900", "meeting", 0, sim.OutcomeSuppressed, "budget exhausted"))
	body := m.villagerDecisionsBody(m.width-6, m.height-6)
	if !strings.Contains(body, "didn't think because budget exhausted") {
		t.Errorf("a router suppression should read plainly, US3 AC2: %q", body)
	}
}

// TestVillagerDecisionsBodyDeadVillagerKeepsChains is the spec's "Dead
// villagers" edge case: the trail of how it died is prime teaching material.
func TestVillagerDecisionsBodyDeadVillagerKeepsChains(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	m.replica.Agents[0].Dead = true
	m.applyEvent(thoughtEvent(1, 100, "reflex-0-100", "reflex", 0, 0))
	m.applyEvent(outcomeEvent(2, "reflex-0-100", "reflex", 0, "landed", ""))
	body := m.villagerDecisionsBody(m.width-6, m.height-6)
	if !strings.Contains(body, "went through") {
		t.Errorf("a dead villager's chains must still render: %q", body)
	}
}

// TestVillagerDecisionsBodyNoOverflowAtTightBudgets is R10/the exact-height
// invariant: the body must never exceed its handed height, at any width.
func TestVillagerDecisionsBodyNoOverflowAtTightBudgets(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	for i := 0; i < 20; i++ {
		job := fmt.Sprintf("reflex-0-%d", i)
		m.applyEvent(thoughtEvent(int64(i*2+1), int64(i), job, "reflex", 0, 0))
		m.applyEvent(toolCallEvent(int64(i*2+2), job, 1, "gather", "landed", ""))
	}
	for _, sz := range []struct{ w, h int }{{15, 3}, {60, 5}, {8, 8}, {60, 40}} {
		body := m.villagerDecisionsBody(sz.w, sz.h)
		if got := len(strings.Split(body, "\n")); got > sz.h {
			t.Errorf("decisions body at %dx%d = %d lines, want <= %d:\n%s", sz.w, sz.h, got, sz.h, body)
		}
	}
}

// TestVillagerDecisionsBodyScrollRevealsLaterContent is R10: a scroll offset
// past what a short pane can show reveals different (later) chain content
// rather than clamping silently to the same view.
func TestVillagerDecisionsBodyScrollRevealsLaterContent(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	for i := 0; i < 10; i++ {
		job := fmt.Sprintf("reflex-0-%d", i)
		m.applyEvent(thoughtEvent(int64(i+1), int64(i), job, "reflex", 0, 0))
		m.applyEvent(outcomeEvent(int64(i+100), job, "reflex", 0, "landed", ""))
	}
	unscrolled := m.villagerDecisionsBody(60, 6)
	m.villDecisionsScroll = 3
	scrolled := m.villagerDecisionsBody(60, 6)
	if unscrolled == scrolled {
		t.Error("a nonzero scroll offset should reveal different content at a short pane budget")
	}
	m.villDecisionsScroll = 10_000 // far past the end — must clamp, not panic or go blank
	clamped := m.villagerDecisionsBody(60, 6)
	if clamped == "" {
		t.Error("an out-of-range scroll must clamp defensively, not blank the pane")
	}
}
