# Contract: Consolidation Model Output

One cloud-tier call per agent per game night (`llm.KindConsolidation`, routed per
llm.json — Anthropic API or an OpenAI-compatible router). The model must reply with ONLY
this JSON object (first `{...}` extracted, same tolerance as planner/convo parsing):

```json
{
  "nature": "<the agent's temperament line, restated verbatim>",
  "gist": "<one-sentence memory of the day, in the agent's voice>",
  "promote": [ {"tick": 612001, "hash": "a1b2c3d4"} ],
  "fade":    [ {"tick": 611800, "hash": "99ffee00"} ],
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

The prompt supplies each buffer memory *with its `tick` and `hash`* so the model can only
reference what it was shown. `id` 0 creates a belief; a nonzero `id` (from the "beliefs
you hold" prompt section) revises one.

## Validator (deterministic, mechanical — internal/mind/validate.go)

Rejection reasons are stable strings recorded in the `agent.consolidated` marker.

**Layer 1 — structure**
- parses as the schema above; unknown fields ignored
- every promote/fade `(tick, hash)` resolves to a memory in the *sent* buffer
- `len(promote) ≤ 5`, `len(fade) ≤ 8`, `len(beliefs) ≤ 4`
- confidence ∈ [0,100]; provenance ∈ {witnessed, told, inferred}; `source`/`subject`
  ∈ [−1, AgentCount)
- gist non-empty, ≤ 240 chars; narrative non-empty, ≤ 1200 chars
- belief revisions with nonzero `id` must reference an existing belief of that agent

**Layer 2 — anchor echo**
- `nature` must equal `persona.Anchor[agent]` byte-for-byte (whitespace-trimmed at the
  ends only)

**Layer 3 — drift lexicon**
- no entry of `persona.DriftMarkers[agent]` may appear (case-insensitive, word-boundary
  match) in `narrative` or in any belief `statement` whose `subject` is the agent itself

Any layer failing rejects the WHOLE output: nothing lands except the marker
(`outcome: "rejected"`, reason = first failure). Malformed JSON is
`outcome: "rejected", reason: "unparseable"`.
