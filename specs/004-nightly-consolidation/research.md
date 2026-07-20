# Research: Nightly Consolidation + Persona Firewall

No NEEDS CLARIFICATION markers remained in the technical context; research resolved the
five design decisions below against the existing codebase (all verified by reading
current source on `task-9-consolidation`).

## D1 — Memory identity for promote/fade events

**Decision**: identify a memory by `(tick, FNV-1a hash of text)`; the reducer adjusts the
first matching entry in `Agent.Memories`, no-op if absent.

**Rationale**: `sim.Memory` has no ID field — memories live in an append-only slice.
Indexes are unstable across concurrent appends between the driver's snapshot and the
batch landing (the night keeps ticking while the call is in flight). `(tick, texthash)`
is stable, cheap, and deterministic; a vanished target (pruned, edge case) degrades to a
harmless no-op rather than corrupting a neighbor.

**Alternatives considered**: slice indexes (unstable under append/prune); adding IDs to
Memory (schema churn across snapshots and every existing emitter for no other consumer).

## D2 — Once-per-night ledger semantics

**Decision**: a recorded `agent.consolidated` marker event with `outcome`
(accepted | rejected | skipped_empty) bumps `LastConsolidatedNight` (= game-day index of
the sleep tick) and, on accept, `ConsolidatedUpTo` (tick high-water mark of the digested
buffer) — both reduced into state. The trigger guard requires `night > LastConsolidatedNight`
AND `sleepTick − lastMarkerTick ≥ 12 game-hours`. Transport/model failure records NO
marker (retry on the next sleep); rejection records a marker (retry next night, per spec
US3-3).

**Rationale**: state-reduced markers survive restarts (FR-001) and replay for free; the
12-hour secondary guard closes the post-midnight-sleep edge (a starving agent dozing at
01:00 would otherwise map to the *next* day index and double-dip). Rejected-bumps-night
matches the spec's "next night arrives → attempted again fresh"; failed-transport-doesn't
means a mid-night daemon restart retries the same night — the once-per-night guarantee is
about *successful or judged* consolidations, not attempts.

**Alternatives considered**: in-memory dedupe (dies with the process, violates FR-001);
deriving the buffer boundary from the marker's own tick (conflates injection-restamp time
with digested range — an explicit `up_to` field is exact).

## D3 — Landing door: extend the existing whitelist vs a new loop command

**Decision**: extend `inject_social`'s whitelisted-batch door with the five consolidation
event types (generalizing its role to "the mind's injection door"), keeping the existing
dry-run-on-copy validation and atomic apply+append+notify.

**Rationale**: the door's contract (whitelist, re-stamp, all-or-nothing) is exactly what
FR-005/FR-006 require; a second parallel command would duplicate ~40 lines of the most
safety-critical code in the loop for zero isolation benefit — the whitelist itself is the
isolation.

**Alternatives considered**: new `inject_consolidation` command (duplicated machinery);
direct store append from the mind (bypasses dry-run validation and the single-writer
loop — rejected outright).

## D4 — Validator design: deterministic and mechanical

**Decision**: three-layer mechanical validator, no second model call:
1. **Structural**: strict JSON schema; every promote/fade ref must resolve in the sent
   buffer; confidence ∈ [0,100]; caps (≤ 5 promotes, ≤ 8 fades, ≤ 4 belief changes,
   narrative ≤ 1200 chars, gist non-empty ≤ 240 chars).
2. **Anchor echo**: the prompt supplies the agent's authored temperament line
   (`persona.Anchor`); the output must restate it byte-identical in a fixed `nature`
   field. A model drifting from the persona reliably paraphrases here — verbatim echo is
   a cheap, deterministic canary.
3. **Drift lexicon**: per-agent authored list (`persona.DriftMarkers`) of trait words
   that contradict the nature (e.g. the gentle one: "cruel", "ruthless"); case-insensitive
   match in the narrative or any self-belief statement → reject with the matched marker
   as reason.

**Rationale**: the grounding decision mechanizes the firewall without spending a second
cloud call per agent per night; every check is reproducible in tests with fixtures
(SC-002 needs "rejected 100% of the time", only a deterministic validator can promise
that). Honest limitation, recorded: lexicon checks catch stated drift, not subtle drift —
a model-judged validator is the parked v2 refinement (same slot the grounding doc parks
rerank-personality in).

**Alternatives considered**: model-judged validation (non-deterministic, doubles cost,
un-testable as a 100% guarantee); embedding-distance checks (new dependency, same
non-determinism).

## D5 — Consolidation concurrency

**Decision**: one consolidation in flight at a time, FIFO queue of due agents (all 8
sleep within the same game hour); each call is latency-tolerant, the night is ~6 real
hours at 4x.

**Rationale**: the cloud tier's queue (cap 32) and the meter are shared with future
narrator/drama traffic; serializing keeps burst pressure trivial and mirrors the proven
convoBusy pattern. 8 sequential calls at even 60 s each finish in 8 minutes of a 6-hour
night.

**Alternatives considered**: parallel fan-out (pointless burst against a bounded queue);
per-agent goroutines with jitter (more machinery, same outcome).
