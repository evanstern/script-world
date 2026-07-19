# Contract: Consolidation Model Output

One cloud-tier call per agent per game night (`llm.KindConsolidation`, routed per
llm.json — Anthropic API or an OpenAI-compatible router). The model must reply with ONLY
this JSON object (first `{...}` extracted, same tolerance as planner/convo parsing):

```json
{
  "nature": "<the agent's temperament line, restated verbatim>",
  "gist": "<one-sentence memory of the day, in the agent's voice>",
  "promote": [ "m3" ],
  "fade":    [ "m7" ],
  "beliefs": [
    {
      "id": 0,
      "statement": "Cedar breaks his word.",
      "confidence": 80,
      "provenance": "witnessed",
      "source": -1,
      "subject": 2
    }
  ],
  "narrative": "<≤1200 chars, first person, the agent's voice>"
}
```

The prompt labels each buffer memory with an ordinal (`m1`..`m60`, newest-last) and the
model references those labels — live testing showed models mangle hashes but transcribe
ordinals reliably; the driver maps labels back to the durable `(tick, hash)` identity the
events carry, deduplicating repeats. `id` 0 creates a belief; a nonzero `id` (from the
"beliefs you hold" prompt section) revises one — an unknown nonzero `id` is coerced to 0
by the driver before validation (models routinely invent an ID for a belief they mean as
new; ID bookkeeping is ours).

## Validator (deterministic, mechanical — internal/mind/validate.go)

Rejection reasons are stable strings recorded in the `agent.consolidated` marker.

**Layer 1 — structure**
- parses as the schema above; unknown fields ignored
- every promote/fade label parses as `mN` with `1 ≤ N ≤ len(sent buffer)`
- `len(promote) ≤ 5`, `len(fade) ≤ 8`, `len(beliefs) ≤ 4` — the driver truncates
  overruns to the best-first prefix before validation (enthusiasm is not corruption);
  the validator caps remain as hard guards
- confidence ∈ [0,100]; provenance ∈ {witnessed, told, inferred}; `source`/`subject`
  ∈ [−1, AgentCount)
- gist non-empty, ≤ 300 chars (prompt asks < 200; headroom is deliberate); narrative non-empty, ≤ 1200 chars
- belief revisions with nonzero `id` must reference an existing belief of that agent

**Layer 2 — anchor echo**
- `nature` must equal `persona.Anchors[agent]` under normalization (lowercase,
  whitespace runs collapsed, trailing `.`/`!` stripped) — typography is tolerated,
  paraphrase is not

**Layer 3 — drift lexicon**
- no entry of `persona.DriftMarkers[agent]` may appear (case-insensitive, word-boundary
  match) in `narrative` or in any belief `statement` whose `subject` is the agent itself

Any layer failing rejects the WHOLE output: nothing lands except the marker
(`outcome: "rejected"`, reason = first failure). Malformed JSON is
`outcome: "rejected", reason: "unparseable"`.
