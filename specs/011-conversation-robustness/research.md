# Phase 0 Research: Conversation Robustness

No NEEDS CLARIFICATION markers survived the spec; research resolves the four design
choices the spec delegated and pins the code-level facts the design depends on.

## R1 — Utterance tolerance: retry vs skip-turn

**Decision**: retry the same speaker once; if the retry also fails, abandon the scene.
Do NOT skip turns.

**Rationale**: the transcript is consumed by the outcome prompt as an alternating
round-robin (`sp := t % n`, convo.go:191); silently skipping a speaker produces a
transcript where consecutive lines share a speaker, a shape the summary model has
never seen in this system and the round-robin bookkeeping does not represent. A
single retry keeps the transcript invariant intact and matches the outcome-site
mechanism (one design, two sites). The spec's FR-002 explicitly allows either;
"two consecutive failures abandon" maps to: first failure → retry; retry failure →
abandon (the two failures are necessarily consecutive, same turn).

**Alternatives considered**: skip-turn (rejected: breaks round-robin transcript
invariant and speaker attribution in `social.conversation_turn` replay); reduce
scene turn count on failure (rejected: changes scene shape doctrine mid-flight).

## R2 — Outcome retry mechanics

**Decision**: on `parseOutcome` failure (parse/validation error, not transport
error), re-submit the identical outcome request exactly once. Transport/admission
errors (`Submit` returning err — queue full, tier down, ctx expired) are NOT
retried: admission control's fail-fast doctrine stays authoritative.

**Rationale**: parse failures are stochastic formatting slips (observed: 4/4 were
the unquoted-gist shape; a re-sample very likely succeeds). Admission failures are
backpressure signals the system is designed to respect — retrying them would fight
the orchestrator (FR-007). The retry rides the same `KindConversation` request on
the same ctx, so the `convoDeadline` (10 min, convo.go:184) and the stale-at-landing
check (convo.go:213-218, runs AFTER outcome) both still bound the scene — a scene
that overruns its staleness budget because of a retry is still rejected stale, which
FR-007/edge cases require.

**Alternatives considered**: retry with a corrective suffix ("your last reply was
not valid JSON") — rejected for v1: changes the prompt surface, complicates the
golden-path test, and the lenient parser (R3) already targets the dominant shape.

## R3 — Lenient parse fallback for the observed failure shape

**Decision**: when `json.Unmarshal` fails in `parseOutcome`, attempt one lenient
recovery targeted at the observed shape — an unquoted string value for `gist`
(and `retold`): scan the raw span and quote the bare value(s), then re-unmarshal.
If lenient recovery also fails, report the original error. Applied in
`parseOutcome` only; `parseSay`'s `{"say": ...}` shape gets the same treatment via
a shared helper ONLY if it falls out naturally — it is not a requirement (spec
marks it SHOULD).

**Rationale**: recovers the scene with zero extra model calls in the dominant case
(4/4 observed failures). Bounded and shape-targeted — this is not a general JSON5
parser; anything beyond the known shape falls through to the retry (R2).

**Alternatives considered**: full tolerant JSON library (rejected: new dependency,
over-broad for one observed shape); regex field extraction (rejected: quoting the
bare value and re-using `encoding/json` keeps escaping/UTF-8 handling correct).

## R4 — Raw-reply persistence on parse failure

**Decision**: extend the existing `cog.outcome` telemetry payload
(`telemetry.go:109` `cogOutcomeEvent`) with an optional `raw` field, populated only
when outcome/reason is a parse failure, truncated to 2048 bytes with a trailing
`…[truncated]` marker. Both sites (utterance, outcome) pass the offending
`resp.Text` through to their existing `OutcomeUnusable` emissions (including the
double-failure abandon path, which carries the RETRY's raw text).

**Rationale**: `cog.outcome` is already the per-thought terminal record, already
emitted at every failure site, and already carries a free-form `reason` — adding
`raw` there needs no new event type (event-log doctrine: `events` is append-only,
new fields in JSON payloads are non-breaking). 2048 bytes covers a 224-token reply
comfortably. Failure-only population keeps the durable record lean (spec assumption).

**Alternatives considered**: new `cog.parse_failed` event type (rejected: new type
for data that is an attribute of the existing terminal record); log-only
persistence (rejected: daemon.log is not the durable, queryable record — SC-003
wants one query).

## R5 — Outcome prompt hardening

**Decision**: tighten the outcome instruction (convo.go:358-365) to state that
`gist` and `retold` MUST be JSON strings in double quotes, and change the `retold`
null-instruction to an explicit `"" (empty string)` alternative — observed models
handle `"retold": ""` more reliably than bare `null` next to prose instructions.
Keep the reply-shape example as the single source of format truth.

**Rationale**: the observed failure is the model starting the gist value with a
bare participant name; an explicit "quoted string" instruction attacks the base
rate. Measured before/after rate is an SC-005 deliverable recorded on the task.

**Alternatives considered**: few-shot example of a bad vs good reply (rejected:
utterance/outcome prompts are deliberately terse for the 224-token budget).

## R6 — MLX reasoning_effort probe (FR-008)

**Decision**: a small shell/curl script (kept in the spec dir or scratch, not
shipped) that hits the live local endpoint (`http://localhost:11434/v1`) with the
utterance-shaped request at `max_tokens=128`, with `reasoning_effort` set to
`none` vs unset vs `low`, N=10 each, recording reply length and empty-reply rate.
Findings (honored / ignored / empty-say correlation) are appended to board TASK-42
via `backlog task edit --append-notes` and referenced from the PR.

**Rationale**: the orchestrator already sends `reasoning_effort` for the local
tier (`resolveReasoningEffort(cfg.Local.ReasoningEffort, "none")`, llm.go:186) —
the open question is endpoint behavior, which only a live probe answers. It is an
investigation with a recorded answer, not shipped code (spec FR-008).

## Pinned code facts (verified 2026-07-21 against main @ cdb454b)

| Fact | Anchor |
|---|---|
| Utterance abandon site (no retry) | internal/mind/convo.go:194-199 |
| Outcome call + abandon site | internal/mind/convo.go:204-210 |
| Stale-at-landing check runs after outcome | internal/mind/convo.go:213-218 |
| Atomic landing batch (turns/memories/relations/rumor + terminal cog.outcome) | internal/mind/convo.go:225-292 |
| Utterance request: MaxTokens 128, KindConversation | internal/mind/convo.go:330-338 |
| Outcome request: MaxTokens 224, KindConversation | internal/mind/convo.go:358-374 |
| parseSay / parseOutcome / firstJSON | internal/mind/parse.go:77-96 / :98-130 / :57-76 |
| Terminal telemetry constructor | internal/mind/telemetry.go:109 |
| Test seam (fake Submitter) | internal/mind/mind.go:23-26; convo_test.go:65,265 |
| KindConversation → TierLocal, prio lane | internal/llm/llm.go:57; llm.go:246-256 |
| Scene wall-clock bound | convoDeadline = 10min, convo.go:178-184 |
