# Contract: Decision-Class Registry (initial values)

Static Go data in `internal/cognition/registry.go`, versioned with `format_version`.
Changing a value is a reviewed code change, never runtime tuning (decision-4).

| Class | llm.Kind(s) | Points | Budget (game time) | BudgetTicks | Degrade | FutureDated |
|---|---|---|---|---|---|---|
| `planner` | planner | 3 | 20 min | 1200 | reflex | yes |
| `musing` | musing | 1 | 1 h | 3600 | skip | no |
| `conversation` | conversation | 13 | 2 h | 7200 | skip | no |
| `meeting` | meeting | 2 | 1 h | 3600 | template | no |
| `consolidation` | consolidation | 5 | 8 h (the night) | 28800 | skip | no |
| `chronicle` | narrator, drama | 5 | 1 day | 86400 | skip | no |
| `metatron` | metatron | 5 | 1 day | 86400 | skip | no |

**Routing arithmetic** (pure): `predictedWallSec = Points × secondsPerPoint(tier)`;
`predictedDriftTicks = predictedWallSec × Speed.TicksPerSecond()`; route to model iff
`predictedDriftTicks ≤ BudgetTicks`. `SpeedMax` (0 = uncapped) never reaches the
router — the existing refusal to run max speed with an LLM configured stands.

**Sanity table** (local tier at the measured ~17 s/point baseline):

| Class | Predicted wall | 1x drift | 4x | 16x | 32x | verdict at 32x |
|---|---|---|---|---|---|---|
| planner (3 pt) | ~51 s | 51 s | 204 s | 816 s | 1632 s | **suppressed** (>1200) |
| musing (1 pt) | ~17 s | 17 s | 68 s | 272 s | 544 s | allowed |
| conversation (13 pt) | ~221 s | 221 s | 884 s | 3536 s | 7072 s | allowed (≤7200) |
| meeting (2 pt) | ~34 s | 34 s | 136 s | 544 s | 1088 s | allowed |

At 32x on a slow local model, planners degrade to the reflex floor while musings and
scenes survive — high speed changes what agents think about, not whether the sim is
correct. A faster tier (or model) re-admits planners at high speed with no code
change: only the calibrated seconds-per-point differs.

**Completeness rule**: daemon startup iterates every `llm.Kind` accepted by the
orchestrator; an unmapped kind is a fatal startup error naming the kind (FR-002).
Fibonacci membership (1, 2, 3, 5, 8, 13) and positive budgets are validated at init.
