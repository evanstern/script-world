# Research: Villager Prompt Quality

**Date**: 2026-07-23 | **Plan**: [plan.md](plan.md)

All Technical Context unknowns resolved. Five decisions.

## D1 — Rejection rates come from the durable event log, not new telemetry

**Decision**: compute `rejected_malformed` / `rejected_cardinality` rates and the
acting-tool distribution by tallying `cog.tool_call` events from the world's event
log, dumped with `promptworld tail <world> --since 0` (line format:
`#seq tick time type payload-json`; payload carries `tool`, `verdict`, `tier`,
`job`). Rate denominator = all `cog.tool_call` events for villager planner jobs;
Metatron/conversation jobs are excluded by joining against `cog.thought` events'
`class` field (same `job` id).

**Rationale**: the log already records every call with its verdict
(`internal/mind/telemetry.go`, `internal/toolloop/record.go`); the spec assumption
"no new telemetry" holds. `tail` needs no running daemon for history.

**Alternatives considered**: (a) new counters + status endpoint — rejected, adds
production surface for a one-task eval; (b) TUI decisions view — rejected, not
scriptable.

## D2 — Variants are git commits; the eval driver builds per ref

**Decision**: the three prompt variants are plain commits, and the soak driver
builds the daemon binary from a named git ref into a temp dir:

- `old` = `origin/main` (merge-base of the task branch)
- `new` = task-branch commit with the rewrite, no exemplar
- `new+exemplar` = task-branch commit adding the worked exemplar

The shipped tip of the branch is whichever rewrite variant wins (FR-004); the
loser remains in history with its numbers recorded.

**Rationale**: no runtime prompt-variant toggle ever enters production code; the
prompt stays a pure function of `(name, personaText)` (FR-005). Git refs are the
natural artifact-grounded variant registry.

**Alternatives considered**: env-var / flag prompt switch — rejected, leaks eval
machinery into the cacheable prefix's purity story and into shipped code.

## D3 — Soak shape: same seed, same game-time window, local tier, ≥200 decisions

**Decision**: per variant, create a fresh world with a fixed seed
(`promptworld new eval73-<variant> --seed 4242`), start the daemon, set speed to
the highest sustainable multiplier, and run until the world clock passes a fixed
game-time window (target: **6 game-hours**, identical for every variant), then
stop and tally per D1. If any variant collects fewer than **200 villager acting
decisions** in the window, extend the window equally for all variants and rerun.
All variants use the same machine, same local provider registry (Ollama
`cogito:3b` per the current v2 registry), run serially, nothing else on the box.

**Rationale**: same seed + same game-time window makes world-state pressure
comparable across variants; the 200-decision floor keeps rate deltas out of the
noise (at 200 decisions, one extra rejection moves a rate by 0.5 pp). Serial runs
avoid contending for the local model.

**Alternatives considered**: wall-clock-bounded soak — rejected, decision counts
then depend on model latency jitter, not world pressure; shared long-lived world —
rejected, later variants would inherit different world states.

## D4 — Token counts: deterministic approximation, applied identically

**Decision**: a Go test/helper renders the system prompt for a fixed sample agent
(name + representative persona) per variant and records: byte length, word count,
and an approximate token count (`len(bytes)/4`, the standard rough BPE estimate).
Numbers for all three variants go into the eval record and onto TASK-73.

**Rationale**: the spec explicitly allows "a documented approximation applied
identically to all variants"; relative deltas are the decision input. No
provider-side counting path exists for the exact villager prompt without a live
call, and provider-reported usage varies by tools-roster serialization, which the
prompt change doesn't touch.

**Alternatives considered**: Anthropic count-tokens API — rejected, measures the
cloud tokenizer while the prompt runs thousands/day on the *local* tier;
Ollama-reported `prompt_eval_count` from soak calls — recorded opportunistically
in the eval notes if trivially extractable, but not the gating number (includes
tools + user prompt, so it drowns the frame delta).

## D5 — Exemplar design constraints (if it ships)

**Decision**: the candidate exemplar is one short worked example of *tool
selection reasoning*, placed in the static frame (after task framing), written to
be situation-generic: it names a hypothetical situation shape ("hungry, carrying
raw food, oven nearby") and shows choosing one acting tool for a reason — it does
NOT use any real villager's name, does NOT show literal JSON arguments, and does
NOT feature `muse` (to avoid teaching thinking-as-default). Ship/reject is decided
purely by D1/D3 numbers per FR-004; distribution collapse toward the exemplar's
verb is an explicit reject signal (SC-003).

**Rationale**: exemplars anchor small models hard; the design minimizes the
anchoring surface (no literal args to copy, no name) so the eval measures the
teaching value, not the parroting value.

**Alternatives considered**: multi-shot exemplars — rejected, token cost on a 3B
local model; exemplar in the user prompt — rejected, out of scope (system prefix
only) and would break per-agent cache stability.

## Out-of-scope confirmations

- `internal/mind/meeting.go:89` (law-phrasing prompt, `KindMeeting`) repeats the
  old identity sentence but is a separate best-effort surface — untouched.
- The user prompt (`userPrompt`, situation/needs/memories) — untouched.
- The Metatron prompt — shipped separately in TASK-64.
