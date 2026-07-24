---
name: nightly-consolidation
description: Sleep-triggered cloud-tier memory digestion — promotions/fades, evidence-gated belief provenance with a computed-on-read confidence decay, self-narrative — behind a deterministic persona firewall validator and a once-per-night event-sourced ledger
kind: component
sources:
  - internal/sim/consolidate.go
  - internal/mind/consolidate.go
  - internal/mind/validate.go
  - internal/persona/personas.go
verified_against: 2bc94f55c57880e07f0e52e5de20c9cd527ab340
---

# Nightly consolidation + persona firewall

TASK-9: when a villager sleeps, one cloud-tier call (`llm.KindConsolidation`)
digests the day's episodic buffer into durable soul updates. "Souls change,
natures don't" is mechanized: the consolidator has no write path to persona.md
(structural) and a deterministic validator rejects temperament drift (automated).

## How it works

**Trigger and ledger** (`internal/mind/consolidate.go`, `internal/sim/consolidate.go`):
the mind observes `agent.slept`; `Agent.ConsolidationDue` gates on
`NightIndex(tick) > LastConsolidatedNight` (1-based, 0 = never) plus a 12-game-hour
gap (`ConsolidationGapTicks`) that closes the post-midnight-doze double-dip. Both
fields — and `ConsolidatedUpTo`, the digested-buffer high-water mark — are reduced
from the recorded `agent.consolidated` marker, so once-per-night survives restarts
and replay is model-free. The episodic buffer is every memory with
`Tick > ConsolidatedUpTo`; empty buffers close the night with a `skipped_empty`
marker and spend nothing. Since TASK-32 `maybeConsolidate` also passes the
[[cognition]] router gate (`routeVerdict` with the `consolidation` class,
`llm.KindConsolidation`) before enqueueing: the night-scale staleness budget
passes at every watchable speed today, but a suppression (future faster
speeds) emits a `cog.outcome{suppressed}` record and skips the night with no
marker — the buffer stays intact and the next sleep retries. Due agents queue
FIFO through a single-flight worker
(the night is hours long; latency is irrelevant).

**The call**: prompt = persona + verbatim temperament anchor + the buffer presented as
ordinal-labeled memories `m1`..`mN`, with the model told to reference them only by
label (memories have no IDs and slice indexes are unstable); the `(tick, hash)`
identity (`sim.MemoryHash`, FNV-1a) is used only internally, to map accepted ordinal
refs back into landed events, + held beliefs by ID + the social context block. Held
beliefs are listed at their **effective** confidence (spec 030, below) rather than
the stored value, so the model revises against what a belief actually feels like
tonight; a below-floor held belief stays listed by ID (still revisable) with a
trailing `(faded)` marker rather than being dropped. The prompt also instructs the
model to cite, per belief, the ordinal labels its claim rests on, and states the
provenance rule verbatim: "witnessed" only for what the agent directly did or
directly received (an omen/dream), "told" for something only heard about in
conversation, "inferred" for a reasoned conclusion. Output contract
(`specs/004-nightly-consolidation/contracts/`): a single JSON object — `nature`
(anchor echo), `gist`, `promote`/`fade` refs, `beliefs` (statement, confidence
0–100, provenance witnessed/told/inferred, source, subject, and since spec 030
`evidence` — up to `maxBeliefEvidence` (4) ordinal labels, best-first),
`narrative`. The call's response budget is `md.consolidationTokens` (spec 025,
TASK-72: `llm.json` `max_tokens.consolidation`, threaded through `mind.New`;
default 1024, the former hardcode).

**The firewall validator** (`internal/mind/validate.go`), deterministic and
mechanical — no second model call, so rejection is a testable 100% guarantee.
Before judging, mechanical slack is absorbed rather than punished (night-177
telemetry: most rejections were bookkeeping, not drift): unknown belief IDs are
coerced to "new" (ID bookkeeping is ours, not the model's) and over-long lists —
including, since spec 030, a belief's `evidence` citations against
`maxBeliefEvidence` (4) — are truncated to their best-first prefix. Then:
1. structure — refs must resolve in the sent buffer (deduplicated, mapped back
   to durable tick+hash identity), caps as hard guards behind the pre-trim
   (≤5 promotes, ≤8 fades, ≤4 belief edits, narrative ≤1200 chars), bounds;
2. anchor echo — `nature` must equal `persona.Anchors[name]` under a normalized
   comparison (lowercase, trimmed, trailing `.`/`!` stripped, whitespace runs
   collapsed) — echo fidelity is the canary, not typography;
3. drift lexicon — authored `persona.DriftMarkers[name]` words (word-boundary,
   case-insensitive) in the narrative or any self-belief reject the night.
Any failure lands ONLY a `rejected` marker with a stable reason; the buffer stays
intact and the next night digests the backlog.

**The provenance gate** (spec 030, `enforceProvenance` in validate.go): runs
deterministically AFTER `validateConsolidation` passes, and never rejects — it
coerces. For each belief it resolves the `evidence` ordinals to durable
`(tick, hash)` `sim.MemoryRef`s exactly as promote/fade refs resolve (deduped,
unresolvable refs dropped silently), and asks [[agent-mind]]'s
`sim.DirectPerception(origin)` — the sole, text-free classifier over the
memory's emission-stamped `Origin` — whether any resolved memory is a direct
perception (an own act, a witnessed event, or a delivered omen; a
report/gist/digest/absent origin is secondhand). A `"witnessed"` claim survives
only when at least one cited memory is direct; otherwise it drops to
`"told"` (some secondhand evidence resolved) or `"inferred"` (nothing
resolved) — "told"/"inferred" claims pass through untouched. The resolved refs
and the direct flag are stashed on the belief for landing; the count of
coerced beliefs rides the night's `agent.consolidated` marker as `Coerced`,
non-fatal telemetry, never a rejection reason.

**Landing**: an accepted night is one atomic batch through the loop's whitelisted
injection door ([[sim-loop]]): promotes, fades, day-gist (`agent.memory_added`,
`SalDayGist`, stamped `Origin: "digest"`), belief revisions — each
`agent.belief_revised` now also carrying the resolved `Evidence` refs and the
`Direct` flag `enforceProvenance` computed — narrative replacement, marker.
Reducer cases (`internal/sim/consolidate.go`) are total — vanished targets
no-op. A newly formed belief always anchors its decay clock to formation
(`Reinforced = tick`, spec 030 US2); a revision refreshes that anchor ONLY when
`Direct` is true — a nightly retelling of pure hearsay changes the stored
confidence but must not keep the clock eternally fresh. Transport/tier failure
(circuit open, budget, timeout) lands **no marker**: the attempt never
happened, the next sleep retries, the world never blocks.

**Belief confidence decay** (spec 030 US2, `sim.EffectiveConfidence`,
`internal/sim/consolidate.go`): a belief's stored `Confidence` never mutates and
no decay event is ever logged — decay is computed purely on read, the same
precedent as memory recency (`SelectMemories` scores on read). Effective
confidence halves every `BeliefHalfLifeDays` (8) game-days since the belief's
`Reinforced` tick — an order of magnitude slower than a memory's own one-day
recency half-life, so convictions outlive vividness. A belief with
`Reinforced == 0` (any belief formed before spec 030) is a legacy grandfather:
it never decays until a revision or an `agent.belief_reinforced` event first
stamps an anchor. Below `BeliefConfidenceFloor` (20 — just under the rumor
tellability floor of 25, so the story keeps being retold after nobody stakes a
decision on it) a belief stops driving behavior: read sites should drop it from
model-facing prompts (`sim.PromptBeliefs` is the shared exclusion helper; the
nightly held-beliefs prompt above is the one documented exception, marking
rather than dropping so faded beliefs stay revisable) and the scribe renders it
hedged rather than as a live conviction ([[agent-mind]]). A separate,
currently producer-less event, `agent.belief_reinforced`
(`BeliefReinforcedPayload{Agent, BeliefID}`), re-anchors a held belief's clock
to now — the reducer arm and its tests ship in spec 030 as the seam for a
future grounded-observation channel; nothing in-tree emits it yet.

## Connections

[[agent-mind]] owns the replica, personas, and the sleep trigger surface (and
stamps the `Origin` every memory carries — the sole input to this note's
provenance gate); [[llm-orchestrator]] routes `consolidation` to the cloud tier
(Anthropic or an OpenAI-compatible router) under the budget meter;
[[sim-state-reducer]] holds beliefs/narrative/ledger; [[event-types]] catalogs
the six event types (promote, fade, belief revision, narrative, the
consolidated marker, and spec 030's producer-less belief-reinforcement); the
scribe ([[agent-mind]]) renders "Who I am becoming" and Beliefs (at effective,
decayed confidence) into soul.md; [[snapshots]] carry the new state
transparently.

## Operational notes

Proven by tests: reducer table + replay determinism (`internal/sim/consolidate_test.go`),
driver atomicity/dedupe/deferral with a scripted model and a persona-bytes canary
(`internal/mind/consolidate_test.go`), validator fixture table incl. every authored
drift marker for every villager (`internal/mind/validate_test.go`). Cost: ≈8 cloud
calls per game night (≈32/real day at 4x) — negligible against the $100 ceiling;
$0 marginal on the operator's LAN router. Honest limit, on the record: the lexicon
catches *stated* drift; subtle drift needs the parked model-judged validator.
