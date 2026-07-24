# Contract: cog.recalibration_recommended (payload v2 — additive adoption fields)

The estimator's breach signal is the ONLY record of a breach episode, and — new with
spec 031 — of the adoption that now acts on it. One episode, one event.

## Emission condition (unchanged)

Emitted when a provider's live estimator's rolling spike rate first exceeds
`BreachRate` (0.3) over a full `WindowSize` (20) window; re-armed after the window
state resets (which, post-031, adoption itself performs). Fires at most once per
breach episode.

## Payload

```json
{
  "tier": "gemma",
  "estimate_s_per_pt": 11.8,
  "spike_rate": 1.0,
  "window": 20,
  "prior_s_per_pt": 0.524,
  "adopted_s_per_pt": 11.8
}
```

| Field | Type | Semantics |
|---|---|---|
| `tier` | string | Serving provider's NAME (field name kept for replay-schema stability, spec 024) |
| `estimate_s_per_pt` | number | The estimator's current estimate at emission. Post-031 emission happens after adoption, so this equals `adopted_s_per_pt`; the field's meaning ("current estimate") is unchanged |
| `spike_rate` | number | Rolling spike rate at the breaching sample (> 0.3 by construction) |
| `window` | int | WindowSize doctrine constant (20) |
| `prior_s_per_pt` | number, omitempty, NEW | Estimate immediately before adoption |
| `adopted_s_per_pt` | number, omitempty, NEW | Window median installed as the new estimate |

## Compatibility rules

- Additive only: pre-031 events lack the two new fields and MUST decode and replay
  identically (reducer no-op whitelist entry in internal/sim/loop.go unchanged).
- No new event type; no new whitelist entry; the digest catalog entry for this type
  is EXTENDED (render `prior→adopted` when present), guarded by TestCatalogSweep.
- Consumers counting breach episodes keep counting this one event type — adoption
  does not add a second record per episode.

## Doctrine cross-reference

specs/007-cognition-horizon/contracts/calibration.md gains the adoption semantics
(what happens AT breach); this file owns the wire shape. Estimator tuning constants
remain doctrine and are unchanged by 031.
