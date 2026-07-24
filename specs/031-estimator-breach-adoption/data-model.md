# Data Model: Estimator Breach Adoption

## Estimator (internal/cognition, per provider, process-lifetime)

Existing entity, extended. All fields guarded by the existing mutex; no
persistence — restarts re-seed from the calibration profile as today.

| Field | Today | After |
|---|---|---|
| `estimate` | EWMA seconds-per-point | unchanged meaning; additionally REPLACED by the window median at adoption |
| `window` | `[]bool` ring, WindowSize=20, spike flags | ring of {value seconds-per-point, spike bool} — retains the observed value alongside the flag (FR-001) |
| `wi, wn` | ring cursor / fill count | unchanged; both zeroed at adoption (fresh window must fill before any further breach) |
| `samples, spikes` | lifetime counters | unchanged; NOT reset at adoption (telemetry, not decision state) |
| `breached` | armed/re-arm state for the breach signal | unchanged semantics; cleared at adoption (empty window ⇒ rate 0) |

**Invariants**
- `estimate > 0` always: constructed from a positive seed; adoption value is a
  median of observed positive durations.
- Breach — and therefore adoption — is impossible before `wn == WindowSize`
  (existing doctrine preserved).
- Adoption is atomic with the breach-detecting `Sample` call (single mutex hold);
  concurrent completions cannot double-adopt one episode.
- Doctrine constants (`EWMAAlpha` 0.2, `SpikeFactor` 3.0, `WindowSize` 20,
  `BreachRate` 0.3) are untouched and remain non-tunable at runtime (FR-007).

**State transitions (Sample)**
1. Non-spike sample → EWMA update (unchanged).
2. Spike sample, window not breaching → excluded from estimate, counted (unchanged).
3. Sample completing a breach (rate > BreachRate over full window, first time) →
   adopt median of window values; zero ring; clear `breached`; return adoption
   evidence (prior, adopted, spike rate).
4. Post-adoption: fresh window fills against the adopted estimate; re-arm is
   structural (empty ring).

## Adoption evidence (return value, internal/cognition)

Plain-values struct returned by `Sample` (nil when no breach), keeping the package
leaf — no event or JSON types here.

| Field | Meaning |
|---|---|
| `Prior` | estimate immediately before adoption (seconds-per-point) |
| `Adopted` | window median installed as the new estimate |
| `SpikeRate` | rolling spike rate at the breaching sample |

## RecalibrationPayload (internal/sim/cognition.go — event `cog.recalibration_recommended`)

Existing recorded schema; ADDITIVE fields only (replay-compatible, omitempty).

| Field | JSON | Status |
|---|---|---|
| Tier | `tier` | existing — the breaching provider's name |
| EstimateSPerPt | `estimate_s_per_pt` | existing — the estimator's current estimate at emission (post-adoption, so its meaning is preserved) |
| SpikeRate | `spike_rate` | existing |
| Window | `window` | existing |
| PriorSPerPt | `prior_s_per_pt,omitempty` | NEW — estimate before adoption |
| AdoptedSPerPt | `adopted_s_per_pt,omitempty` | NEW — window median installed |

Historical events lack the new fields and replay identically (reducer no-op either
way). The digest renderer shows `prior→adopted` when the fields are present.

## Relationships

```
provider.est (Estimator)
   └─ Sample() ──returns──▶ Adoption evidence (plain values)
         └─ llm.feedEstimate ──hook(provider, evidence)──▶ Mind.RecalibrateSignal
               └─ sim.RecalibrationPayload ──emitCog──▶ events table
                     └─ tui digest renderer (catalog entry, TestCatalogSweep-guarded)
```
