package tui

// Decision-trace projection (spec 020, TASK-63): a bounded per-agent record
// of "why did my villager do that" built incrementally from the same event
// stream applyEvent already folds into the replica and the chronicle ring
// (tui.go), joining cog.thought / cog.tool_call / cog.outcome on their
// shared job ID (research D1, contracts/decision-trace-ui.md §1). Pure
// event-sourced logic — no lipgloss, no Model — in the grammar.go/digest.go
// style; villagerDecisionsBody (views.go) renders it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
)

// decisionChainCap bounds the projection per agent (data-model.md, spec
// FR-002/SC-005): the oldest chain is evicted from both indexes once a
// 21st arrives. metatronAgent is the dedicated sentinel Metatron's own
// turn-metatron-* jobs attribute to — never a real replica agent index, and
// distinct from -1 ("not yet attributed").
const (
	decisionChainCap = 20
	metatronAgent    = -2

	conversationJobPrefix = "conversation-" // shared two-agent scenes — no single-agent attribution (spec Assumptions); never ingested
	metatronJobPrefix     = "turn-metatron-"
)

// decisionChain is one cognition's causal record for display (data-model.md),
// keyed by job ID and attributed to one villager (or Metatron). attributed/
// HasThought are ingest-only bookkeeping (research D1/D2) — neither is
// rendered directly; they exist so a job is attributed at most once (R3) and
// so a router suppression (outcome with no thought, no calls) can be told
// apart from an ordinary in-flight fragment.
type decisionChain struct {
	Job           string
	Agent         int // -1 unattributed (invisible in the villager surface, R3); metatronAgent for Metatron
	Class         string
	Tick          int64
	TriggerSeq    int64
	Stimulus      string
	Calls         []decisionCall
	Outcome       string
	OutcomeReason string
	Suppressed    bool

	attributed bool
	HasThought bool
}

// decisionCall is one projected tool call (data-model.md). Verdict is the
// raw toolloop.Verdict/outcome string, kept as-is — it must be rendered
// ONLY through verdictPhrase (FR-007); nothing in this package ever prints
// it directly. Args is the compact single-line display form of the
// already-capped upstream arguments (research D7); the current per-call row
// (contract R9) doesn't surface it, but it's kept for parity with
// data-model.md and future use.
type decisionCall struct {
	Ordinal int
	Tool    string
	Verdict string
	Reason  string
	Args    string
}

// decisionTraces is the projection (data-model.md): byJob is the ingest join
// point, byAgent indexes job keys per agent in arrival order — the order
// decisionChainCap eviction and most-recent-first rendering (R4/R9) both
// walk.
type decisionTraces struct {
	byJob   map[string]*decisionChain
	byAgent map[int][]string
}

// newDecisionTraces returns an empty projection — used at Model construction
// and reset wholesale on every reconnect (contract R5), matching the
// replica's own reconnect lifecycle.
func newDecisionTraces() decisionTraces {
	return decisionTraces{byJob: map[string]*decisionChain{}, byAgent: map[int][]string{}}
}

func (dt *decisionTraces) ensureMaps() {
	if dt.byJob == nil {
		dt.byJob = map[string]*decisionChain{}
	}
	if dt.byAgent == nil {
		dt.byAgent = map[int][]string{}
	}
}

// chainFor returns job's chain, creating an unattributed one (Agent -1) if
// this is the first event seen for it.
func (dt *decisionTraces) chainFor(job string) *decisionChain {
	if c, ok := dt.byJob[job]; ok {
		return c
	}
	c := &decisionChain{Job: job, Agent: -1}
	dt.byJob[job] = c
	return c
}

// attribute resolves c's agent at most once (R3: thought's explicit Agent >
// outcome's explicit Agent > a villager job-ID parse — whichever source
// reaches this first for a given job, since thought is always the earliest
// of the three event types when present at all). A resolution indexes the
// job into byAgent, evicting the oldest entry past decisionChainCap from
// both indexes (contract R4). ok=false (an unrecognized/unparseable job with
// no explicit Agent yet) leaves the chain unattributed — invisible in the
// villager surface until a later event resolves it (contract R3).
func (dt *decisionTraces) attribute(c *decisionChain, agent int, ok bool) {
	if !ok || c.attributed {
		return
	}
	c.Agent = agent
	c.attributed = true
	keys := append(dt.byAgent[agent], c.Job)
	if len(keys) > decisionChainCap {
		delete(dt.byJob, keys[0])
		keys = keys[1:]
	}
	dt.byAgent[agent] = keys
}

// chainsFor returns agent's chains most-recent-first (contract R9) — a
// fresh slice built from the arrival-ordered index, never the index itself.
func (dt decisionTraces) chainsFor(agent int) []*decisionChain {
	keys := dt.byAgent[agent]
	out := make([]*decisionChain, 0, len(keys))
	for i := len(keys) - 1; i >= 0; i-- {
		if c, ok := dt.byJob[keys[i]]; ok {
			out = append(out, c)
		}
	}
	return out
}

// ingest folds one subscribed event into the projection (contract R1): only
// the three cog.* record types mutate it, each exactly once. Called from
// applyEvent (tui.go) after the seq-skip guard, before the chronicle ring
// append (research D1) — ring is that ring as it stood before this event,
// which is what stimulus resolution (resolveStimulus) needs.
func (dt *decisionTraces) ingest(e store.Event, names []string, ring []store.Event) {
	switch e.Type {
	case "cog.thought":
		dt.ingestThought(e, names, ring)
	case "cog.tool_call":
		dt.ingestToolCall(e)
	case "cog.outcome":
		dt.ingestOutcome(e)
	}
}

func (dt *decisionTraces) ingestThought(e store.Event, names []string, ring []store.Event) {
	p, ok := decode[sim.CogThoughtPayload](e)
	if !ok || strings.HasPrefix(p.Job, conversationJobPrefix) {
		return
	}
	dt.ensureMaps()
	c := dt.chainFor(p.Job)
	c.Class = p.Class
	c.Tick = p.SnapshotTick
	c.TriggerSeq = p.TriggerSeq
	c.Stimulus = resolveStimulus(p.TriggerSeq, ring, names)
	c.HasThought = true
	if strings.HasPrefix(p.Job, metatronJobPrefix) {
		dt.attribute(c, metatronAgent, true)
	} else {
		dt.attribute(c, p.Agent, true)
	}
}

func (dt *decisionTraces) ingestToolCall(e store.Event) {
	p, ok := decode[sim.CogToolCallPayload](e)
	if !ok || strings.HasPrefix(p.Job, conversationJobPrefix) {
		return
	}
	dt.ensureMaps()
	c := dt.chainFor(p.Job)
	if c.Tick == 0 {
		c.Tick = p.SnapshotTick
	}
	c.Calls = insertOrdinal(c.Calls, decisionCall{
		Ordinal: p.Ordinal, Tool: p.Tool, Verdict: p.Verdict, Reason: p.Reason, Args: compactArgs(p.Args),
	})
	if strings.HasPrefix(p.Job, metatronJobPrefix) {
		dt.attribute(c, metatronAgent, true)
	} else if agent, ok := parseVillagerJobAgent(p.Job); ok {
		// Fragment rescue (research D2, FR-008): this cognition's cog.thought
		// was folded into the pre-connect snapshot and never seen, so the
		// job-ID's own agent index is the only source left.
		dt.attribute(c, agent, true)
	}
}

func (dt *decisionTraces) ingestOutcome(e store.Event) {
	p, ok := decode[sim.CogOutcomePayload](e)
	if !ok || strings.HasPrefix(p.Job, conversationJobPrefix) {
		return
	}
	// OutcomeRetried is a NON-TERMINAL marker (contracts/telemetry.md rule 1):
	// a transport retry was consumed (spec 025 FR-004), emitted by the tool-loop
	// consumers AFTER the door may already have recorded the run's real terminal
	// outcome. This projection tracks only terminal outcomes, so the marker is
	// skipped here — it never overwrites the earned terminal — while remaining in
	// the event log for trail-level retry counting (SC-003). Same disregard the
	// conversation retried marker gets via the conversationJobPrefix guard above.
	if p.Outcome == sim.OutcomeRetried {
		return
	}
	dt.ensureMaps()
	c := dt.chainFor(p.Job)
	c.Outcome = p.Outcome
	c.OutcomeReason = p.Reason
	if c.Class == "" {
		c.Class = p.Class
	}
	if c.Tick == 0 {
		c.Tick = p.SnapshotTick
	}
	// Suppression (FR-007): a router suppression's cog.outcome is the ONLY
	// record of its thought — no cog.thought, no cog.tool_call ever exists
	// for the job (mind/telemetry.go emitSuppressed). Any calls already
	// ingested (fragment) rule it out.
	c.Suppressed = !c.HasThought && len(c.Calls) == 0
	if strings.HasPrefix(p.Job, metatronJobPrefix) {
		dt.attribute(c, metatronAgent, true)
	} else {
		dt.attribute(c, p.Agent, true)
	}
}

// villagerJobRe matches the villager job-ID shape `<class>-<agentIndex>-<tick>`
// (internal/mind/telemetry.go newMeta: `fmt.Sprintf("%s-%d-%d", class, agent,
// snapshotTick)`) — the fallback attribution source when a thought was
// missed (research D2). Class is letters/underscore only (the cognition
// registry's Class strings, e.g. "planner"), so this never mistakes
// conversation-<n> or turn-metatron-<n> for a villager job — those two are
// routed by explicit prefix check before this is ever consulted.
var villagerJobRe = regexp.MustCompile(`^[A-Za-z_]+-(\d+)-\d+$`)

func parseVillagerJobAgent(job string) (int, bool) {
	m := villagerJobRe.FindStringSubmatch(job)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// insertOrdinal inserts one call into ordinal order regardless of arrival
// order (contract R2) — a plain insertion sort; a chain accumulates at most
// a handful of calls per cognition (toolloop's MaxRounds bound), so this
// stays O(1) amortized per event (R19). A duplicate ordinal (should never
// happen — {Job, Ordinal} is the driver's own correlation key) replaces
// rather than duplicates, keeping re-ingest idempotent.
func insertOrdinal(calls []decisionCall, c decisionCall) []decisionCall {
	i := sort.Search(len(calls), func(i int) bool { return calls[i].Ordinal >= c.Ordinal })
	if i < len(calls) && calls[i].Ordinal == c.Ordinal {
		calls[i] = c
		return calls
	}
	calls = append(calls, decisionCall{})
	copy(calls[i+1:], calls[i:])
	calls[i] = c
	return calls
}

// resolveStimulus is research D3: fixed once at thought-ingest time and
// stored as plain text, so the chain survives the trigger event's later
// eviction from the chronicle ring (SC-003). triggerSeq 0 is the cadence
// sentinel (no stimulus event armed this thought); a ring hit reuses the
// chronicle's own digest voice (formatChronicleLine, grammar.go) rather than
// inventing a second one (FR-005); a miss (the trigger predates this
// client's subscription, or — vanishingly rarely — was already evicted)
// degrades to a neutral reference naming the seq.
func resolveStimulus(triggerSeq int64, ring []store.Event, names []string) string {
	if triggerSeq == 0 {
		return "cadence — the villager's own rhythm, not a triggering event"
	}
	if e, ok := findRingEvent(ring, triggerSeq); ok {
		return plainSegs(formatChronicleLine(e, names).Summary)
	}
	return fmt.Sprintf("stimulus #%d (before this view connected)", triggerSeq)
}

// findRingEvent binary-searches ring (append-ordered, strictly increasing
// Seq — tui.go's chronicle invariant) for the event at seq.
func findRingEvent(ring []store.Event, seq int64) (store.Event, bool) {
	i := sort.Search(len(ring), func(i int) bool { return ring[i].Seq >= seq })
	if i < len(ring) && ring[i].Seq == seq {
		return ring[i], true
	}
	return store.Event{}, false
}

// compactArgs renders a call's already-capped raw arguments (toolloop
// capArgs, 2 KiB) as a compact single-line display form (research D7):
// valid JSON compacts cleanly; anything that fails to parse (defensive only
// — capArgs upstream always emits valid JSON) falls back to the raw bytes
// rather than dropping the field.
func compactArgs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// --- verdict glossary (research D4, contract §4) ---

// verdictGlossary maps every toolloop verdict and sim outcome string to a
// plain-language phrase — the single authority both the decisions sub-view
// and the Metatron transcript render through (FR-007). Sweep-tested against
// internal/toolloop's Verdict constants and internal/sim's Outcome*
// constants (decisions_test.go) so a value added to either taxonomy without
// a phrase here fails the build rather than silently falling through to its
// raw enum string (contract R16, SC-002). None of these phrases is chosen to
// literally embed its own raw key as a substring (deliberately reworded
// where the raw string is itself a plain English word, e.g. "adapted") —
// that keeps a substring sweep over the raw taxonomy meaningful (T016).
var verdictGlossary = map[string]string{
	// toolloop verdicts (internal/toolloop/record.go)
	"landed":               "went through",
	"rejected_gate":        "the gate refused it",
	"rejected_cardinality": "its one action for this thought was already spent",
	"rejected_unknown":     "named a tool that wasn't available to it",
	"rejected_malformed":   "the arguments were malformed",
	"read_ok":              "read the data successfully",
	"read_error":           "the read failed",
	"unlanded":             "never got the chance to run",

	// sim outcome vocabulary (internal/sim/cognition.go) — "landed" is
	// shared with the table above (same word, same meaning in both domains).
	"adapted":              "changed its plan to fit what actually happened",
	"rejected-stale":       "was refused as stale before it could run",
	"rejected-guard":       "was refused by a guard",
	"superseded":           "was overtaken by a newer thought",
	"expired":              "ran out of time before it could land",
	"rejected-unavailable": "found nothing available to act on",
	"unusable":             "produced nothing usable",
	// OutcomeSuppressed (data-model.md, FR-007's last bullet): rendered by
	// renderDecisionChain (views.go) as "didn't think because <reason>" —
	// this phrase is that entry's verb.
	"suppressed": "didn't think",
	// OutcomeRetried is non-terminal (contracts/telemetry.md rule 1) — never
	// a chain's actual terminal Outcome in practice, but still glossaried
	// defensively so a caller that renders it anyway never sees the raw enum.
	"retried": "needed a retry to parse the reply",
}

// verdictFallback is R17's safe generic phrase for a verdict/outcome string
// this glossary doesn't recognize (a future taxonomy addition that landed
// before its phrase did) — never the raw enum, never a panic, never a
// dropped row.
const verdictFallback = "did something the client doesn't have a plain-language phrase for yet"

// verdictPhrase resolves a raw verdict/outcome string to its plain-language
// phrase (FR-007, contract R15).
func verdictPhrase(v string) string {
	if p, ok := verdictGlossary[v]; ok {
		return p
	}
	return verdictFallback
}

// callLine renders one call/verdict as plain language — "tool — phrase",
// with the reason parenthesized when present — the one place a tool name +
// raw verdict + reason becomes prose (FR-007), shared by the decisions
// sub-view (renderDecisionChain, views.go, contract R9) and the Metatron
// inline transcript row below (R12).
func callLine(tool, verdict, reason string) string {
	line := tool + " — " + verdictPhrase(verdict)
	if reason != "" {
		line += " (" + reason + ")"
	}
	return line
}

// --- Metatron inline verdict rows (research D6, contract §3) ---

// transcriptVerdictPrefix marks a verdict row appended to Model.transcript at
// ingest; classifyTranscriptLine (views.go) strips it and styles the row as
// telemetry, distinct from you:/angel: rows (contract R12).
const transcriptVerdictPrefix = "» "

// metatronVerdictRow builds the transcript row one turn-metatron- cog.tool_call
// appends (contract R12–R14): ok is false for every other event — including
// a villager's own cog.tool_call — so applyEvent (tui.go) can gate on it
// directly without re-deriving the prefix check.
func metatronVerdictRow(e store.Event) (string, bool) {
	if e.Type != "cog.tool_call" {
		return "", false
	}
	p, ok := decode[sim.CogToolCallPayload](e)
	if !ok || !strings.HasPrefix(p.Job, metatronJobPrefix) {
		return "", false
	}
	return transcriptVerdictPrefix + callLine(p.Tool, p.Verdict, p.Reason), true
}
