# Cognition horizon vs learner iteration speed (classroom mode)

**Status:** options + recommendation, awaiting client decision (TASK-66, AC#3)
**Origin:** 2026-07-22 team review, new-ideas item 4; client agreed 2026-07-22 to discuss
**Doctrine touched:** decision-4 (cognition horizon), TASK-20 (speed ladder), spec 007
**Grounded against:** `internal/cognition/route.go`, `internal/cognition/registry.go`,
`internal/clock/clock.go`, `cmd/promptworld/calibrate.go:173` (horizonSummary),
decision-4 (incl. the 2026-07-20 pause-catch-up refinement)

## The tension

A learner's loop is *tweak the charter → speed the world up → watch the effect*.
The cognition horizon (decision-4) deterministically suppresses model calls whose
answers would land too stale at the current speed: `Route` allows a class iff
`points × seconds-per-point × speed ≤ budget-ticks` (route.go:23-40). The mechanism
that protects determinism and story-sensibility directly opposes fast pedagogical
feedback. This is a design decision, not a bug — doctrine says "speed is never
capped to protect cognition; cognition is scoped to survive speed."

## The arithmetic (what actually suppresses, and when)

1 tick = 1 game second; speed N× = N ticks per real second (game-clock). Registry
values (registry.go:36-43) and the max speed at which each class still routes to
the model, `maxSpeed = budget / (points × s/pt)`:

| class | pts | budget (ticks) | degrade | max speed @ 20 s/pt (bootstrap local) | @ 17 s/pt (wiki example) | @ 12.3 s/pt (decision-4 live run) |
|---|---|---|---|---|---|---|
| planner | 3 | 1,200 | reflex | 20× → **suppressed at 32×** | 23.5× → **suppressed at 32×** | 32.5× → OK at 32× (barely) |
| conversation | 13 | 7,200 | skip | 27.7× → **suppressed at 32×** | 32.6× → OK (barely) | 45× → OK |
| meeting | 2 | 3,600 | template | 90× → OK | 106× → OK | 146× → OK |
| consolidation | 5 | 28,800 | skip | 288× → OK | — | — |
| metatron | 5 | 86,400 | skip | 864× → OK | — | — |
| chronicle | 5 | 86,400 | skip | 864× → OK | — | — |

Three load-bearing facts fall out:

1. **Metatron itself never suppresses at watchable speeds.** Even at the
   pessimistic bootstrap 20 s/pt and 32×, a metatron thought predicts
   5 × 20 × 32 = 3,200 ticks of drift against an 86,400-tick budget. The learner's
   charter edits and angel turns always reach the model. What degrades at speed is
   the *downstream evidence* — villager planner thoughts (musing now rides the
   planner class, spec 017) and conversations — i.e. exactly the behavior change
   the learner sped up to observe.
2. **The pain is concentrated in planner + conversation near 16–32×**, and it is
   calibration-sensitive: a fast local model (12.3 s/pt) clears 32× for both; the
   uncalibrated bootstrap (20 s/pt) suppresses both at 32×. TASK-40 (uncalibrated
   worlds silently over-suppress) is the adjacent HIGH bug; `calibrate` already
   prints the per-class answer ("planner suppressed above 16x",
   calibrate.go:173-197), so the numbers any option needs are already computed.
3. **Suppression is invisible to a learner today.** Verdicts land in `cog.outcome`
   telemetry, but nothing in the TUI says "your villagers stopped thinking because
   of speed" (TASK-41 is the open surfacing task). Whatever option wins, a learner
   who speeds up and sees reflex-only behavior *must be told why*, or the lesson
   taught is "my prompt did nothing" — the exact anti-lesson.

Interaction with in-flight work: spec 024 (TASK-35, provider division of labor)
will route chatty classes to fast small models, lowering effective s/pt for
conversation-class calls — it partially *dissolves* the conversation row but not
the planner row (planner quality wants the slow model, ~20 s/pt under load). The
horizon numbers per class will therefore diverge per provider chain; options below
must not hard-code today's single-tier arithmetic.

## Options

### (a) Paused authoring sandbox

Pause is already zero-staleness by doctrine: in-flight thought lands at the frozen
tick, and a landing batch wakes one debounce-bounded, blessed catch-up round
(decision-4 refinement, 2026-07-20). An **authoring mode** builds on that: pause
the world, edit the charter, *trigger* a thought (operator-initiated), watch it
land at zero staleness, single-step or resume.

- Arithmetic: at a frozen tick, predicted and measured drift are 0 ≤ any budget —
  the horizon never gates it. No doctrine bending on budgets.
- Doctrine cost: "no new cognition starts while paused" needs a deliberate,
  doored exception for operator-triggered thoughts (same shape as the blessed
  catch-up round: bounded, snapshotted at the frozen tick).
- Pedagogy: the tightest possible loop (edit → thought → observe), but it trades
  away *ambient* observation — the learner sees one thought under a microscope,
  not the village living under the new charter.

### (b) Classroom/learner speed cap

Teaching-mode worlds cap their speed ladder at the highest speed the calibrated
host affords for planner-class thoughts — a number `calibrate` already computes.
At 17 s/pt the cap is 16×; at 12.3 s/pt it's 32×.

- Arithmetic: cap = `floor-to-ladder(budget / (points × s/pt))` for the planner
  row, recomputed from the world's calibration profile.
- Doctrine cost: decision-4 says speed is never capped *to protect cognition*.
  Resolution: this caps speed to protect **feedback legibility in a teaching
  posture** — a per-world config posture (like TASK-68's stage field), not an
  engine rule. The engine still refuses nothing; the world template does.
- Pedagogy: preserves ambient observation (the village visibly lives), at the cost
  of slower wall-clock iteration on slow hosts. Honest: the learner never sees a
  world that silently stopped thinking.

### (c) Per-class staleness-budget overrides in teaching worlds

Let teaching worlds widen `BudgetTicks` (e.g. planner 1,200 → 3,600) as a recorded
world-config posture, accepting more drift for faster iteration.

- Arithmetic: planner at 20 s/pt clears 32× once the budget passes
  3 × 20 × 32 = 1,920 ticks (32 game-minutes of drift).
- Doctrine cost: highest of the three. Registry values are doctrine ("changing one
  is a reviewed code change, never runtime tuning", registry.go:22; decision-4:
  budgets retuned by humans from telemetry, never self-adjusted). A widened budget
  loosens both the router *and* the landing door (`OutcomeRejectedStale` keys on
  the same budget) — intents land on a world 30+ game-minutes past their snapshot.
  The stories a learner studies become *less sensible* exactly when we want cause
  → effect to be legible. It also forks per-world doctrine, complicating replay
  audits and support ("which budget was this world running?").
- Pedagogy: fastest ambient iteration on paper, but it teaches against a degraded
  simulation — the effect the learner attributes to their prompt may be a
  staleness artifact.

### (d) Staged combination by curriculum level

The curriculum ladder (TASK-68, client's three-stage progression) changes the
answer per stage:

- **Stage 1 — prompt Metatron conversationally.** The metatron class never
  suppresses at watchable speeds (fact 1). No conflict exists; no mechanism
  needed. Run at any ladder speed.
- **Stage 2 — edit the charter (instruction files).** The learner iterates on
  villager-visible behavior → the planner row bites. This is where the chosen
  mechanism matters.
- **Stage 3 — tool/capability design.** Same exposure as stage 2 plus tool-loop
  effects; same mechanism applies.

So (d) is not a fourth mechanism — it's the observation that (a)/(b)/(c) only
need to hold for stages 2+, and stage presets (TASK-68 AC#2) are the natural
carrier for whichever is chosen.

## Recommendation

**(d) staging, carried by (a) + (b); reject (c).**

- **(a) paused authoring sandbox** as the core stage-2+ learning affordance: it is
  the only option that gives *instant* feedback, it's doctrine-aligned (pause
  catch-up is already blessed; the operator-trigger door is a bounded extension),
  and it scales to any host — a learner on a slow laptop gets the same tight loop
  as one on a fast one.
- **(b) classroom speed cap** as the teaching-world preset default for ambient
  running between authoring sessions: the world visibly lives, and the learner
  never unknowingly watches a reflex-only village. The cap is derived from the
  calibration profile per world, not hard-coded — which also survives spec 024's
  per-provider s/pt divergence.
- **Reject (c)**: it spends the project's determinism/sensibility doctrine to buy
  iteration speed that (a) already provides for free, and it teaches on a
  degraded sim.
- **Cross-cutting prerequisite (either way):** suppression must be legible in the
  learner-facing surface (TASK-41 / spec 024 US6). A speed cap without an
  explanation, or a suppressed planner without a visible verdict, both read as
  "the game is broken."

## Follow-up implementation tasks (to cut after the client picks — AC#4)

Sketch, assuming the recommendation stands:

1. Authoring mode: operator-triggered thought at the frozen tick (doored exception
   to pause's no-new-cognition rule, bounded like the catch-up round) + a
   single-step affordance. Needs its own spec (touches sim loop pause semantics).
2. Teaching-world preset: speed cap derived from the calibration profile's
   planner-row arithmetic, stored as world-config posture; consumed by TASK-68's
   stage presets. Interacts with TASK-40 (uncalibrated worlds must prompt
   calibrate before a cap can be honest).
3. Learner-facing horizon legibility: fold into TASK-41 rather than a new task.
