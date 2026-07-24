# Phase 0 Research: Epistemic Hygiene for Emergent Lore

Grounding: docs/wiki (nightly-consolidation, social-fabric, agent-mind — all pinned at 6eb8b60), spec 004/019
artifacts, direct source verification at `f7b5385`, and the TASK-73/spec-027 eval precedent.

## R1 — Direct-perception classification needs a memory origin stamp

**Finding**: `sim.Memory` (internal/sim/agents.go) has no origin field. The only origin signal in stored state is
`Conv != 0` (conversation-gist memories, spec 019). Omen/dream deliveries, own-action memories, witness memories,
and nightly day-gists are indistinguishable after landing.

**Decision**: add `Origin string json:"origin,omitempty"` to `Memory` and its `agent.memory_added` payload, stamped
at emission. Vocabulary (closed set): `action` (own executed act — situatedMemoryEvent/situatedMemoryToned sites),
`witness` (situatedMemoryAboutEvent sites — the rememberer stood there; EXCEPT the chest-owner any-distance memory,
which stamps `report` — the owner may not have seen the taking), `omen` (Metatron dream/omen deliveries), `gist`
(conversation-gist memories, alongside their existing Conv ref), `digest` (nightly day-gist). Direct perception =
{action, witness, omen}. Legacy memories (field absent) classify as NOT direct perception — the conservative
direction: hygiene can only under-grant "witnessed", never over-grant it (FR-002/FR-012 spirit).

**Rationale**: spec 019 removed all bare memory constructors, so every sim emission flows through three situated
constructors plus two driver injection sites (convo gist, day-gist) and the Metatron nudge path — a bounded,
auditable set of stamp sites. `omitempty` keeps every pre-030 memory byte-identical (the 019 precedent exactly).

**Alternatives considered**: inferring origin from salience/Conv/Subject heuristics — rejected: salience is mutable
(promotions), Subject conflates gossip-about with witnessed-about, and FR-002 demands a classifier with no judgment.

## R2 — Evidence citations extend the consolidation contract like promote/fade

**Decision**: the consolidation output's belief object gains `"evidence": [ordinal refs]` (same `m1..mN` labels as
promote/fade), pre-trimmed to `MaxBeliefEvidence = 4` (best-first, matching the absorb-slack philosophy), resolved
by the validator to durable `(tick, MemoryHash)` identities exactly as promote/fade refs are. The landed
`agent.belief_revised` payload carries the resolved evidence identities plus a derived `direct bool` (true iff ≥1
cited memory has a direct-perception origin) so replay needs no re-resolution.

**Validator rule** (mind/validate.go, extending the existing provenance switch): proposal says `witnessed` →
qualifying evidence keeps it; secondhand-only evidence coerces to `told`; no resolvable evidence coerces to
`inferred`. Coercions are recorded in the night's marker telemetry (existing rejection-reason channel, new
non-fatal `coerced` counter) — never a rejected night (FR-003). `told`/`inferred` proposals pass through
regardless of evidence (hygiene gates the strong claim only).

## R3 — Decay is computed-on-read, per the memory recency precedent

**Decision**: `Belief` gains `Reinforced int64 json:"reinforced,omitempty"` (game tick). Set at formation; refreshed
by a revision iff that revision's evidence is direct (R2's `direct` flag); refreshed by the new reinforcement event
(R5). Effective confidence is a pure function in `internal/sim`:

```
EffectiveConfidence(b, tick) = b.Confidence                              if b.Reinforced == 0 (legacy grandfather)
                             = round(b.Confidence × 0.5^(days/halflife)) otherwise, days = (tick − b.Reinforced)/86400
```

Constants (doctrine, recorded on TASK-79 per FR-006): `BeliefHalfLifeDays = 8` — a conviction unconfirmed for a week
halves; an order of magnitude slower than memory recency (half per game-day), because convictions outlive vividness —
and `BeliefConfidenceFloor = 20` — just under the rumor tellability floor (25), so a belief stops driving behavior
slightly before its rumor stops being tellable: the story outlives the conviction, which is the myth-not-fact goal.
Stored `Confidence` never mutates; no decay events exist; pause/speed change nothing (pure tick arithmetic).

**Legacy**: `Reinforced == 0` (absent in pre-030 snapshots) = grandfathered, no decay until a revision or
reinforcement first stamps it (spec US2-AC5). `Belief.Tick` (last revision) is NOT used as a fallback clock —
using it would retroactively crush old worlds' beliefs at upgrade.

## R4 — Read-site sweep for effective confidence

Sites that must read effective (not stored) confidence: the scribe's Beliefs section (internal/scribe/scribe.go —
renders hedged form below floor, e.g. "(half-remembered) …", and effective value above it), the consolidation
prompt's held-beliefs block (internal/mind/consolidate.go — the model sees effective confidence so its revisions
start from lived reality), and any planner-prompt belief surface. Below floor: excluded from prompts entirely
(FR-007), still stored, still revisable by a night that cites it.

## R5 — The reinforcement seam

**Decision**: new whitelisted injection event `agent.belief_reinforced{agent, belief_id}` with a total reducer arm:
sets `Reinforced = e.Tick` on the named belief; vanished belief no-ops (consolidation reducer doctrine). Documented
in the events contract as the consumer half of the future grounded-observation channel (perception-of-absence task);
tests emit it directly through the injection door. No producer ships in 030.

## R6 — Gist attribution is a prompt + eval change only

**Decision**: the conversation outcome prompt (internal/mind/convo.go, the gist/outcome call) gains attribution
instructions (unverified claims stay named; never assert unperformed actions); the output SHAPE is untouched (gist,
topics, tones, retold). Eval per the TASK-73/spec-027 template: `specs/030-epistemic-hygiene/eval/` with old.md /
new.md prompt variants, a fixture set of scripted scenes (≥2 target shapes: speculation-flattening,
action-confabulation; plus control scenes for gist quality), a runner script (scripts/eval-prompt-79.sh modeled on
eval-prompt-73.sh), decision.md with numbers, and the numbers recorded on TASK-79 before the prompt ships (FR-010).
Metric: judge-scored flattening/confabulation rate old vs new (target ≥50% reduction, SC-004) + no control
regression beyond recorded tolerance.

## R7 — Determinism and compatibility

All three mechanisms preserve byte-identical replay: validator coercion is deterministic pre-landing (recorded
events replay verbatim, model-free); decay is computed-on-read; reinforcement is recorded input. Shapes are
additive-`omitempty` (`Memory.Origin`, `Belief.Reinforced`, belief-payload evidence fields) → old snapshots/logs
byte-identical → **no format bump** (same spec-013 boundary argument as 028). Provenance labels on already-landed
beliefs are never rewritten (FR-012).

## R8 — Testing strategy

Unit: origin stamping per emission site; classifier table; validator coercion both directions (FR-004) incl.
no-evidence and vanished-ref cases; EffectiveConfidence curve table (formation, half-life boundary, floor crossing,
legacy grandfather, post-reinforcement reset). Reducer: belief_revised with evidence/direct, belief_reinforced,
replay determinism suite extended with a log containing coerced beliefs + reinforcement events (SC-003). Render:
scribe hedged/excluded forms; consolidation prompt shows effective values. Eval: fixture runner per R6. Live: a
multi-game-day sample for SC-005 recorded in quickstart-results.md (012/T045 precedent applies if impractical).
