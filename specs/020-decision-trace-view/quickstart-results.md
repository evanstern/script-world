# Quickstart Validation Results: Decision-Trace View

**Run**: 2026-07-23, by the orchestrator (gate pass for T019) against a fresh world
(seed 63) in the session scratchpad, daemon + real local tier (gemma4:12b-mlx via
Ollama) and real cloud tier (LAN 9router, `cc/claude-haiku-4-5-20251001`,
openai_compat). The real `promptworld ui` binary was driven through a PTY with
expect scripts; assertions were made against the captured terminal stream.

## §1 Unit-level proof — PASS

`go test ./internal/tui/ -count=1` → ok (123 tests). `go build ./...`,
`go vet ./internal/tui/` clean; `gofmt -l` clean for every file this branch touches
(5 pre-existing unformatted files on origin/main verified byte-identical, untouched).
`TestCatalogSweep` passes unchanged. Full `go test ./...` green including e2e
(implementer run, ~185s).

## §2 Live decisions sub-view — PASS

Driven live: `4` → roster (bulk gauges rendered) → `⏎` detail (footer shows
`d decisions`) → `d` → decisions sub-view rendered (`BIRCH · decisions` /
`ASH · decisions` headers), `j`/`k` scrolled, `esc` unwound decisions → detail →
roster (footer hints confirmed at each level). Observed live in the pane, rendered
from real events:

- chain headers `day 1 06:10 · planner`
- honest in-progress marker: `in progress — no outcome yet` (FR-008)
- plain-language terminal outcome with reason:
  `outcome: was refused as stale before it could run (staleness 16054 > …)` — no
  raw enum anywhere on screen (FR-007)
- neutral stimulus degradation for a pre-connect trigger:
  `stimulus: stimulus #4901 (before this view connected)` (FR-005)
- empty state before any post-connect cognition: `no decisions recorded yet this
  session` (R11)

Call-row rendering with a landed verdict was additionally proven against real
captured events (real `set_plan` landed by gemma, real `agent.built` trigger event)
replayed through the production ingest + render path during implementation:
`set_plan — went through`, stimulus resolved via the chronicle digest
(`stimulus: A5 built a fire at (23,5)`); the same path is unit-covered for every
verdict.

## §3 Ring-eviction survival — PASS

With the decisions sub-view attached, the world ran until the log advanced from seq
4796 to 12785 (~8,000 events — 16× the 500-event chronicle ring). Chains ingested
early in the session (stimulus referencing seq #4901) still rendered with their
stored stimulus text at the end of the run (SC-003).

## §4 Metatron inline verdicts — PASS

UI parked on the metatron tab while `promptworld metatron` sent console turns:

- Two prose-only turns (the angel counseled instead of acting): **no verdict rows
  appended** (R14).
- A miracle turn: the row `note  work_miracle — went through` appeared inline in
  the attached UI's transcript, sourced from the event stream — event
  `#13787 cog.tool_call {"job":"turn-metatron-29399","ordinal":1,
  "tool":"work_miracle",…,"verdict":"landed","tier":"cloud"}`, alongside
  `#13785 metatron.item_granted` (R12, SC-004; plain language per R15 — the raw
  `landed` enum never rendered).

## §5 Fragments and reconnect — PASS

Every UI attach in this run was a mid-run connect: cognitions whose `cog.thought`
predated the subscription rendered as honest fragments (neutral stimulus reference;
`unknown` class where the thought was never seen), attributed via the job-ID parse.
Pre-connect cognitions folded into the snapshot correctly never appeared (the
projection is subscription-scoped by design); post-connect chains built normally
after each fresh attach — the reconnect-reset path is also unit-covered.

## Verdict

All five quickstart sections validated; T019 checked on this evidence.
