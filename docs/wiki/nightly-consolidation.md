---
name: nightly-consolidation
description: Sleep-triggered cloud-tier memory digestion — promotions/fades, beliefs with provenance, self-narrative — behind a deterministic persona firewall validator and a once-per-night event-sourced ledger
kind: component
sources:
  - internal/sim/consolidate.go
  - internal/mind/consolidate.go
  - internal/mind/validate.go
  - internal/persona/personas.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
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

**The call**: prompt = persona + verbatim temperament anchor + the buffer with
per-memory `(tick, hash)` references (`sim.MemoryHash`, FNV-1a — memories have no
IDs and slice indexes are unstable) + held beliefs by ID + the social context
block. Output contract (`specs/004-nightly-consolidation/contracts/`): a single
JSON object — `nature` (anchor echo), `gist`, `promote`/`fade` refs, `beliefs`
(statement, confidence 0–100, provenance witnessed/told/inferred, source, subject),
`narrative`.

**The firewall validator** (`internal/mind/validate.go`), deterministic and
mechanical — no second model call, so rejection is a testable 100% guarantee.
Before judging, mechanical slack is absorbed rather than punished (night-177
telemetry: most rejections were bookkeeping, not drift): unknown belief IDs are
coerced to "new" (ID bookkeeping is ours, not the model's) and over-long lists
are truncated to their best-first prefix. Then:
1. structure — refs must resolve in the sent buffer (deduplicated, mapped back
   to durable tick+hash identity), caps as hard guards behind the pre-trim
   (≤5 promotes, ≤8 fades, ≤4 belief edits, narrative ≤1200 chars), bounds;
2. anchor echo — `nature` must equal `persona.Anchors[name]` byte-for-byte
   (paraphrase is the cheap canary for drift);
3. drift lexicon — authored `persona.DriftMarkers[name]` words (word-boundary,
   case-insensitive) in the narrative or any self-belief reject the night.
Any failure lands ONLY a `rejected` marker with a stable reason; the buffer stays
intact and the next night digests the backlog.

**Landing**: an accepted night is one atomic batch through the loop's whitelisted
injection door ([[sim-loop]]): promotes, fades, day-gist (`agent.memory_added`,
`SalDayGist`), belief revisions, narrative replacement, marker. Reducer cases
(`internal/sim/consolidate.go`) are total — vanished targets no-op. Transport/tier
failure (circuit open, budget, timeout) lands **no marker**: the attempt never
happened, the next sleep retries, the world never blocks.

## Connections

[[agent-mind]] owns the replica, personas, and the sleep trigger surface;
[[llm-orchestrator]] routes `consolidation` to the cloud tier (Anthropic or an
OpenAI-compatible router) under the budget meter; [[sim-state-reducer]] holds
beliefs/narrative/ledger; [[event-types]] catalogs the five event types; the
scribe ([[agent-mind]]) renders "Who I am becoming" and Beliefs into soul.md;
[[snapshots]] carry the new state transparently.

## Operational notes

Proven by tests: reducer table + replay determinism (`internal/sim/consolidate_test.go`),
driver atomicity/dedupe/deferral with a scripted model and a persona-bytes canary
(`internal/mind/consolidate_test.go`), validator fixture table incl. every authored
drift marker for every villager (`internal/mind/validate_test.go`). Cost: ≈8 cloud
calls per game night (≈32/real day at 4x) — negligible against the $100 ceiling;
$0 marginal on the operator's LAN router. Honest limit, on the record: the lexicon
catches *stated* drift; subtle drift needs the parked model-judged validator.
