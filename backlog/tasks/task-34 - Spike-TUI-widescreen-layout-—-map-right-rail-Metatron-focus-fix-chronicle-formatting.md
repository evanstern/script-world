---
id: TASK-34
title: >-
  Spike: TUI widescreen layout — map + right rail, Metatron focus fix, chronicle
  formatting
status: In Progress
assignee: []
created_date: '2026-07-21 02:05'
updated_date: '2026-07-21 13:02'
labels:
  - spike
dependencies: []
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Interactive spike with mockups. Scope: (1) widescreen two-column layout: map keeps aspect, right side becomes a rail for controls/displays; (2) fix Metatron swallowing all keys (tui.go:305-309) — input should be escapable/discoverable without leader-key gymnastics; (3) chronicle visible in right rail as scrolling feed alongside map, solo view preserved; (4) chronicle formatting: readable one-line events, conversation turns as resolved-name JSON (speaker/listener/text), expandable pretty-JSON inspection while paused.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Widescreen composite: at width ≥112 cols the TUI renders the home page per docs/design/tui/pages/home.md — map left, dock right with tabs chronicle|metatron|souls (chronicle default), minibuffer above footer; map and dock update live simultaneously
- [x] #2 Narrow fallback: below 112 cols today's single-pane UI renders unchanged; resizing across the breakpoint swaps layouts live without losing state (pages/solo-views.md)
- [x] #3 Solo zoom state machine per pages/solo-views.md: tab key selects dock tab, same key again zooms solo, same key/1/esc returns home; tab state (scroll, filters, expansion) survives the round trip
- [x] #4 Focus contract per patterns/focus-contract.md: viewing never captures typing; m focuses the minibuffer; focused state draws amber border + 'esc release · ⏎ send' hint; esc always releases; no silent key swallows; all three acceptance checks in that doc pass, in widescreen AND narrow fallback
- [x] #5 Metatron flow per panels/minibuffer.md + panels/dock.md: send via minibuffer, non-blocking busy state, replies land in the dock metatron tab (stream if visible, badge 'metatron •' if not); transcript shows you/angel rows with input history on ↑/↓
- [x] #6 Chronicle grammar per patterns/chronicle-grammar.md: speech events render {"Speaker"→"Listener"} + quoted utterance in bright text; agent indices resolved to names in all feed payloads; class table (speech/scene/narration/clock/default) implemented; r/a/t toggles preserved
- [x] #7 Inspect mode per panels/chronicle.md: pause + visible chronicle enables j/k/g/G selection and ⏎ expand; expansion shows the stored event verbatim, pretty-printed with // name annotations; resume collapses and returns to tail-follow
- [x] #8 Keymap per patterns/keymap.md including per-mode footer hints; ctrl+c quits from any state
- [x] #9 Tests: unit coverage for breakpoint/width math, chronicle line formatting per event class, and focus-contract key routing; go build ./... and go test ./... green
- [x] #10 Design reference updated in the same branch wherever implementation deviated (INDEX.md ground rule 4)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Spike round 1: grounded in internal/tui (tui.go, views.go). Key-stealing root cause: tui.go:305-309 routes all keys to console when Metatron pane is active. Produced 3 layout mockups (A right-rail stack, B tabbed dock, C chronicle rail + Metatron minibuffer), Metatron focus contract (explicit/visible/self-documenting focus), and chronicle format (speech-first one-liners, paused j/k+enter JSON inspector). Mockup artifact: https://claude.ai/code/artifact/dfb04194-b379-4733-a586-9882b5e0746e — recommendation: C, with B's dock as growth path. Awaiting user direction.

Decision: B+C hybrid (tabbed dock + Metatron minibuffer; angel replies land in the dock's metatron tab). Durable design reference stamped at docs/design/tui/ — INDEX.md (decision record + anatomy), pages/ (home, solo-views+narrow-fallback), panels/ (map, dock, chronicle, minibuffer), patterns/ (focus-contract, chronicle-grammar, keymap, layout). Commit 9988fb1 on branch task-34-tui-widescreen-spike (.worktrees/task-34). Reference is written to be handed to the spec-implementer agent; next step: spec/tasks + implementation, PR from this branch.

ACs #1–#10 drafted from docs/design/tui/. Implementation delegated to the implementing tier (Sonnet subagent, spec-implementer instructions inlined — agent registry didn't expose the custom type this session) working in .worktrees/task-34 on branch task-34-tui-widescreen-spike. Orchestrator will review the agent's report, verify gates, tick ACs, and open the single TASK-34 PR.

Implementation complete on branch task-34-tui-widescreen-spike: commits 6f7c497 (layout math), 7c95793 (chronicle grammar pure functions), 8636edb (composite + focus contract + solo zoom + inspect), 959a215 (doc reconciliation). Orchestrator independently verified in worktree: go build/vet green, go test ./... green all packages, internal/tui green uncached, branch files gofmt-clean (4 gofmt-flagged files in internal/metatron+scribe predate branch on main). Agent-reported deviations folded into docs: solo(k)+k2 switches solo'd tab; 1 never solo-zooms map; breakpoint arithmetic corrected in layout.md (dock shrinks 118→112, not map); inspect operates over raw feed only. Open review items: souls-tab condensing drops columns coarsely below 40 cols (no priority order specified); tab/shift+tab now pure dock-tab cycling. PR not yet opened.

Live tmux test (140x40 + 100-col fallback) against a real daemon: composite, dock tabs, focus contract, metatron flow, solo zoom, narrow fallback, inspect expansion all verified working on screen. Three defects found: (B1) widescreen View renders taller than the terminal — header row pushed off-screen in all widescreen modes (row budget doesn't account for panel border rows); (B2) j/k are silent no-ops while an event is expanded — must collapse before moving, contradicts panels/chronicle.md and the no-silent-swallow rule; (B3) cosmetic: focused minibuffer hint wraps, orphaning 'send' to a second line at 140 cols. ACs #1 and #7 unchecked pending fixes; returned to implementer agent.

User feedback round 2: (B4) column split changes to 50/50 map/dock — planning decision, layout.md to be rewritten by implementer in same commit; (B5) resize robustness — chronicle growth was scrolling the whole frame (same root cause as B1 height overflow), plus re-clamp geometry state on WindowSizeMsg; (#4 investigated) readable/color-coded chronicle IS implemented and live — ANSI dim confirmed on default-class rows, names resolved; the gray wall was a newborn world with zero speech events (84 moved / 32 needs_changed, no conversation_turn yet). Test world resumed at 32x (max reserved for LLM-less worlds) to accumulate conversations + narrator entries for retest demo. B4/B5 sent to implementer alongside B1-B3.

B1-B5 fix batch landed (393b492 50/50 split + viewport padding; b6dce6c row-budget bounding across all panel bodies; 6b321cb doc reconciliation). Independently verified: build/test/gofmt/vet green, internal/tui 46/46. Live retest at 140x40: header visible, frame exactly 40 lines, 50/50 split, j/k/g/G reliable while expanded (B2 was a rendering bug — marker drawn off-panel — not state), resize 100x30 round trip preserves selection and exact height, long minibuffer input truncates to tail without wrapping. ACs #1 and #7 re-checked. Finding for the demo: no conversations ever occurred in the test world — cognition governor suppresses conversation jobs at 32x (13pt x ~25s/pt x 32 > 7200-tick budget); world set to 4x to let real dialogue land for the speech-formatting demo.

Follow-up item from live testing: metatron tab shows an eternal '⋮ thinking…' spinner when the cloud tier can't be reached — daemon logs 'metatron: digest deferred: cloud tier: no Anthropic credentials found' but the console protocol never surfaces it, so the UI can't distinguish slow from dead. Proposed: daemon pushes a console error/deferred status; metatron tab renders it as a dim transcript row (e.g. 'angel unreachable — cloud tier deferred'). Observed against a credential-less test daemon; user separately reported an unresponsive metatron in their own world (note: two daemon processes were attached to ~/worlds/myworld at the time — possible stale-daemon cause on their side, left for user to check). Scope call needed: this touches daemon+IPC, may be its own TASK rather than TASK-34.

Speech-formatting demo delegated to an Opus 4.8 subagent per user request: fresh disposable world (world-demo, seed 7) at 16x (fits the conversation governor budget; drops to 8x/4x if suppressions appear), local gemma tier only, polls for social.conversation_turn, captures plain+ANSI feed rows and the paused inspector view, mandatory teardown (no test worlds left running; user's myworld untouched).
<!-- SECTION:NOTES:END -->
