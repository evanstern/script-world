# Ship-gate decision — TASK-79 gist attribution (spec 030 US3)

**Date**: 2026-07-24 | Spec 030 | Gate: FR-009/FR-010, SC-004. Contract:
[contracts/eval-protocol.md](../contracts/eval-protocol.md).

The conversation OUTCOME/gist prompt (`internal/mind/convo.go`) gains attribution
rules (`eval/new.md`) vs the current prompt (`eval/old.md`). The prompt ships
(T011) only if the numbers below clear the pre-stated bar; otherwise it does not
ship and this record says so.

## Pre-stated tolerance (fixed BEFORE inspecting any results)

Ship bar (all must hold for `new` to ship):

1. **Primary (SC-004)** — on the treatment fixtures (speculation + action,
   6 fixtures × N samples), the **defect rate under `new` ≤ 50% of the `old`
   defect rate** (i.e. ≥50% relative reduction). A defect is a gist the judge
   flags `flattened` OR `confabulated_action`.
2. **Control quality guard** — `new` control faithfulness rate ≥ `old` control
   faithfulness rate **− 10 percentage points** (no meaningful gist-quality
   regression on ordinary scenes).
3. **Control no-harm** — the attribution rules must not push the model to hedge
   ordinary facts into non-facts: `new` introduces at most **1 additional**
   control-scene defect vs `old` across all control samples.

Judge: the same standard local model scores every gist (common-mode bias
cancels in the old→new difference). Scoring is on parsed gists; parse failures
are reported separately and, if materially unequal across variants, noted.

Boundary note (pre-stated): if `old`'s treatment defect rate is already at or
near the floor, a ≥50% relative reduction is not meaningfully demonstrable — in
that case the gate is **NOT met on this model/fixture set** and the prompt does
not ship on this evidence (an honest null result, not a bar to be lowered).

## Run setup

Authoritative run (`results/`, mirrored in `results/gemma4-12b-mlx/`):

| field | value |
|-------|-------|
| generation model | `gemma4:12b-mlx` (the standard local tier default, `internal/llm/config.go` `defaultLocal`) |
| judge model | `gemma4:12b-mlx` (same-model judge; common-mode bias, see contract) |
| endpoint | `http://localhost:11434/v1` |
| N (samples/fixture) | 3 |
| gen temperature / max_tokens | 0.8 / 224 (the Ollama server default the daemon inherits; the outcome call's `MaxTokens`) |
| reasoning_effort | `none` (the daemon's gemma4 default — else the 224-token budget is spent on hidden CoT and content is empty) |
| judge temperature | 0 |
| fixtures | 10 (3 speculation, 3 action, 4 control) |
| git_sha | `33ef0049` |
| run_utc | 2026-07-24T04:17:34Z |

## Numbers

### Authoritative — `gemma4:12b-mlx` (the standard local model, the gate)

| variant | treatment n | defects | flattened | confab | defect rate | control faithful |
|---------|-------------|---------|-----------|--------|-------------|------------------|
| `old` | 18 | 0 | 0 | 0 | **0.00%** | 12/12 = 100% |
| `new` | 16* | 0 | 0 | 0 | **0.00%** | 12/12 = 100% |

\* `new` had 2 parse failures (both `spec-03-omen-sign`, the 3-participant fixture:
the added rules block plus the longest transcript overran the 224-token budget
before the JSON closed), so 16 parsed treatment gists rather than 18. The
daemon's tolerant outcome parser would still extract the gist in production; only
the strict-jq eval extractor drops them. Not defect-relevant.

**Treatment defect rate: `old` 0.00% → `new` 0.00%. Reduction: n/a (0% baseline).**
Both variants attribute every speculation claim and never assert an unperformed
action. Representative `old`/`new` gists on the Thornspire fixture:

- `old`: "Rowan claims to have seen supernatural glowing tendrils in a well, while Birch remains skeptical."
- `new`: "Rowan claims to have seen glowing tendrils in a well, while Birch expresses skepticism and attributes the sighting to moonlight."
- `new` (action fixture): "Rowan and Birch agreed to investigate the well's tendrils at first light the following day" — planned, not "after investigating".

Control faithfulness is 100% for both — the new rules cause no gist-quality
regression on ordinary scenes.

### Corroboration (NON-authoritative) — `cogito:3b` generation, `gemma4:12b-mlx` judge

`results/cogito-3b/`. Run because the authoritative baseline is 0% (below), so the
standard-model eval cannot show whether the prompt helps *where the failure
actually occurs*. `cogito:3b` is the TASK-73 runner's model and the weaker class
that produced the real world-01 Thornspire defects. This run is diagnostic only;
the gate is the standard model.

| variant | treatment n | defects | flattened | confab | defect rate | control faithful |
|---------|-------------|---------|-----------|--------|-------------|------------------|
| `old` | 18 | 3 | 2 | 1 | 16.67% | 9/12 = 75% |
| `new` | 18 | 5 | 5 | 1 | 27.78% | 10/12 = 83% |

**Treatment defect rate: `old` 16.67% → `new` 27.78%. "Reduction": −66.6% (defects
rose).** Reading the flagged gists, most `new` flags are the judge over-penalizing
*correctly-attributed but vivid* gists ("Rowan claimed he saw glowing tendrils
curling out of the old well… green and cold. Birch denied this") — the small model,
told to keep the claim attributed by name, reproduces the claim's vivid language,
which the judge reads as partial endorsement. There are also two genuine
small-model failures under `new`: a confabulated "Mira cut planks" (act-02, an
action only agreed to) and Thornspire content bleeding into an unrelated control
(ctrl-03). n is 18 treatment samples and the judge is noisy, so the increase is
within noise — but there is unambiguously **no ≥50% reduction** on `cogito:3b`
either.

## Verdict

**Ship bar NOT met. `internal/mind/convo.go` is NOT changed; T011 does not start.**

1. **Primary (SC-004) — FAILS to demonstrate.** On the standard local model the
   `old` treatment defect rate is **0%** — the failure mode this prompt targets
   does not occur on `gemma4:12b-mlx`. A ≥50% *relative* reduction is
   mathematically undefined against a 0% baseline (the pre-stated boundary
   condition). The `new` prompt is *safe* (0% defects, 100% control faithfulness,
   equivalent gists) but there is nothing to reduce.
2. **Corroboration** on the weaker `cogito:3b`, where the failure does appear
   (16.67%), shows the `new` prompt does **not** reduce it (27.78%) — so the bar
   is not met on that model either.
3. **Control guard**: satisfied on both models (gemma 100%→100%; cogito 75%→83%),
   but moot given (1)/(2).

Per the absolute gate, the prompt does not ship on this evidence. Iterating
`new.md` was considered and rejected as futile for the authoritative gate: no
wording can produce a ≥50% reduction against a 0% baseline, and the cogito signal
is noise-dominated, not a wording problem.

### Escalation (for the planning tier — a design decision above the implementer)

The eval as specified assumes the standard local model exhibits the confabulation
shapes. It does not: `gemma4:12b-mlx` already writes honest, attributed gists
unprompted. This leaves an open question the implementer must not decide alone:

- **Option A — do not ship US3.** The current model produces honest gists; the
  prompt change earns nothing measurable and slightly raises parse-failure and
  small-model-verbosity risk. Close US3 as "not needed on the current model".
- **Option B — ship `new.md` as cheap insurance** for future/weaker local models
  and as explicit doctrine, accepting that the gate cannot be *demonstrated* on
  the standard model and recording that exception. This requires lowering or
  reinterpreting the FR-010/SC-004 gate, which is the planning tier's call, not
  the implementer's.

Recommendation: **Option A** on this evidence (the laundering-pump concern US3
targets is a property of weak models; the standard tier is already clean, and the
cogito corroboration gives no signal that `new.md` helps weak models either). If
the orchestrator prefers B, the gate language in the spec/contract needs an
explicit amendment recording why a non-demonstrable gate was accepted.
