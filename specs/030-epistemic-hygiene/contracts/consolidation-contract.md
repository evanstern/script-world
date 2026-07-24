# Contract: Consolidation Belief Evidence + Provenance Enforcement

## Output contract extension (model-facing)

The nightly reflection's belief object (specs/004-nightly-consolidation contract, extended):

```json
{"id": 0, "statement": "...", "confidence": 0-100, "provenance": "witnessed|told|inferred",
 "source": -1, "subject": -1, "evidence": ["m3", "m7"]}
```

- `evidence`: ordinal labels from the sent buffer (`m1..mN`), the belief's supporting memories. Optional for
  `told`/`inferred`; REQUIRED in practice for `witnessed` to survive (see enforcement). Pre-trimmed to
  `MaxBeliefEvidence` (4) best-first before judging — over-long lists are absorbed, not punished.
- The prompt instructs: cite the memories the belief rests on; `witnessed` only for what you directly did or
  directly received (omens, dreams); claims from conversation are `told`.

## Validator enforcement (deterministic; mind/validate.go)

1. Resolve evidence ordinals exactly as promote/fade refs (dedupe, map to durable `(tick, hash)`); unresolvable
   refs are dropped silently.
2. Provenance gate (coerce-not-reject): `witnessed` + ≥1 resolved memory with direct-perception origin → stands;
   `witnessed` + only secondhand → `told`; `witnessed` + none resolved → `inferred`. `told`/`inferred` pass
   through. A night is NEVER rejected for provenance alone.
3. Coercion telemetry: the count of coerced beliefs rides the night's `agent.consolidated` marker payload
   (additive field, omitempty).

## Direct-perception classification (sim-owned, model-free)

`Memory.Origin` ∈ {`action`, `witness`, `omen`} → direct. {`report`, `gist`, `digest`} or absent (legacy) →
secondhand. The classifier is a pure function on the stored field; no heuristics, no text inspection.

## Compatibility

- A reflection output with NO `evidence` fields (old-shaped model output) is legal: its `witnessed` beliefs coerce
  to `inferred` — the conservative direction — and everything else lands as before.
- Landed pre-030 beliefs are never relabeled (FR-012).
