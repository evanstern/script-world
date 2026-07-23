# Phase 0 Research: Decision-Trace View

All Technical Context entries were resolvable from the codebase and prior specs (017
agent-tool-loop, 018 chronicle-digest, 015 villagers-tab); no NEEDS CLARIFICATION
markers remained. Decisions below pin the design choices the spec left to planning.

## D1 — Projection keyed by job, indexed per agent

**Decision**: maintain `map[job]*decisionChain` plus a per-agent ordered slice of job
keys (append order = arrival order). A chain accumulates its `cog.thought` header, its
`cog.tool_call` records (ordinal-ordered), and its `cog.outcome` terminal. Ingest
happens in `applyEvent` (internal/tui/tui.go:925) after the replica fold, before ring
append — the one place every subscribed event already flows through.

**Rationale**: the job ID is the correlation key spec 017 designed for exactly this
(`{Job, Ordinal}` dense per job); `applyEvent` is the established single choke point;
the chronicle ring (cap 500, tui.go:57) cannot serve as the store because eviction
would amputate chains (spec SC-003).

**Alternatives considered**: (a) scanning the ring at render time — rejected, breaks at
eviction and does O(ring) work per frame; (b) daemon-side projection — rejected, spec
FR-009 forbids daemon changes and the client already receives every event.

## D2 — Attribution: thought first, job-ID parse as fallback

**Decision**: attribute a chain to the agent from `CogThoughtPayload.Agent` when the
thought was seen. For fragments (thought folded into the pre-connect snapshot), parse
the villager job shape `<class>-<agentIndex>-<snapshotTick>`
(internal/mind/telemetry.go:43) with a strict regexp; jobs with the
`turn-metatron-` prefix (internal/metatron/turn.go:125) attribute to Metatron; jobs
with the `conversation-` prefix (internal/mind/convo.go:100) are skipped (spec
Assumptions — no single-agent attribution exists).

**Rationale**: the thought is authoritative and always present in steady state; the
parse only rescues mid-cognition connects (spec FR-008). Both formats are pinned by
existing tests upstream.

**Alternatives considered**: joining via `cog.outcome.Agent` for outcome-first
fragments — adopted as a bonus source when available (outcome carries Agent); pure
regex-only attribution — rejected as needlessly lossy when payloads carry the index.

## D3 — Stimulus resolved at ingest, stored as text

**Decision**: when a `cog.thought` arrives with `TriggerSeq > 0`, look the trigger
event up in the chronicle ring by seq at that moment and store its digest line
(`formatChronicleLine` summary, flattened to plain text) inside the chain. `TriggerSeq
== 0` stores the cadence phrase; a seq not found in the ring stores a neutral
reference ("stimulus #N, before this view connected").

**Rationale**: the trigger fired at most moments before the thought event, so it is
near-certainly still within the 500-event ring at ingest; resolving once and storing
text makes the chain self-contained and immune to later ring eviction (SC-003).
Reuses the digest grammar (spec 018) rather than inventing a second voice (FR-005).

**Alternatives considered**: render-time lookup — rejected (eviction races); storing
the whole trigger event — rejected (unbounded payload retention; text line is capped
by nature).

## D4 — Verdict glossary as a single pure map

**Decision**: one `verdictPhrase(verdict string) (string, bool)` table in
`decisions.go` covering the full spec-017 verdict taxonomy (landed, rejected_gate,
rejected_cardinality, rejected_unknown, rejected_malformed, read_ok, read_error,
unlanded) plus the `cog.outcome` outcome vocabulary (internal/sim/cognition.go:14-29,
including suppressed/retired forms) — e.g. `rejected_cardinality` → "its one action
for this thought was already spent". Reasons render appended after an em-dash,
verbatim (they are already prose from gates/handlers). A sweep test imports the
`internal/toolloop` verdict constants and the `internal/sim` outcome constants and
fails if any lacks a phrase — the same catalog-sweep pattern digest_test.go uses.

**Rationale**: single authority satisfies FR-007/SC-002 mechanically; the sweep makes
"a new verdict appears without a phrase" a test failure, not a silent raw enum leak.

**Alternatives considered**: per-surface phrasing — rejected (drift between surfaces);
translating reason strings too — rejected (reasons are freeform handler prose; the
verdict phrase supplies the plain-language frame around them).

## D5 — Decisions sub-view state and key grammar

**Decision**: a `villDecisions bool` (plus scroll offset) on `Model`, meaningful only
while `villDetail` is true. `d` toggles decisions from the detail view; `j`/`k` scroll
chains while it is open; `esc` closes decisions → detail → roster, extending the
existing esc chain in `handleVillagersKey` (tui.go:759) one level deeper, ahead of the
solo-release chain per the focus contract. Body renders via a new
`villagerDecisionsBody`, dispatched from the same place `villagerDetailBody` is
(views.go:1436), clipped to the pane budget like every panel body.

**Rationale**: matches the established detail-view pattern (TASK-56) — layered key
handling gated on visibility, client-only state clamped defensively, esc unwinds one
level at a time (focus-contract.md rule 3).

**Alternatives considered**: a fourth dock tab — rejected, spec Assumptions pin the
sub-view inside villager detail; modal overlay — rejected, no precedent in this TUI
and the exact-height invariant favors pane bodies.

## D6 — Metatron inline rows appended to the transcript at ingest

**Decision**: in `applyEvent`, a `cog.tool_call` whose job has the `turn-metatron-`
prefix appends one styled row to `m.transcript` (the existing `[]string`, cap 200,
tui.go:360) via a distinguishing prefix that `classifyTranscriptLine` (views.go:1292)
maps to a dim/telemetry style: tool name + glossary phrase + reason. The existing
`⚡` miracle rows (from the RPC reply) remain; verdict rows are the complete
event-sourced trail, so a refused call now appears where before nothing did.

**Rationale**: tool_call events for a turn arrive over the subscription while the
console RPC is still in flight, so appended rows land immediately before that turn's
`angel:` reply row — the "inline at the turn" ordering falls out of the existing
message flow with no new data structure. The 200-row cap already bounds it.

**Alternatives considered**: rendering from the projection at view time interleaved by
turn — rejected, requires correlating transcript rows to jobs (transcript rows carry
no turn identity today) for no user-visible gain; changing the metatron RPC reply to
carry records — rejected, FR-009 (no daemon change).

## D7 — Reconnect and bounds semantics

**Decision**: the projection resets with the replica on reconnect (`connectedMsg`
swaps state wholesale) — same lifecycle as the raw feed. Bounds: `decisionChainCap =
20` chains per attributed agent (and 20 for Metatron), oldest evicted with their map
entries; per-call args stay in their upstream-capped form and the decisions renderer
shows args only in compact single-line form.

**Rationale**: spec Assumptions accept reconnect loss; a hard per-agent cap plus the
upstream 2 KiB arg cap gives SC-005's bounded-memory guarantee by construction.

**Alternatives considered**: persisting the projection across reconnects by replaying
the store — rejected, the client deliberately has no store access (replica pattern,
wiki tui-client); unbounded retention — rejected outright (SC-005).
