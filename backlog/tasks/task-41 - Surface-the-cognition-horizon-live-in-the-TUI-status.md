---
id: TASK-41
title: Surface the cognition horizon live in the TUI/status
status: To Do
assignee: []
created_date: '2026-07-21 13:47'
updated_date: '2026-07-24 02:42'
labels:
  - ux
  - tui
dependencies: []
priority: medium
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From TASK-39: suppression exists only in raw cog.outcome payloads; no TUI/status surface. At high speed even the narrator is router-gated (narrate.go:260-261), so the world goes silent with no explanation — 'nothing is happening' with no cause shown. Add a live per-class verdict at current speed (e.g. header/souls indicator: 'conversations suppressed at 32x — calibrate or slow down') and/or suppression counters. Natural home: the TASK-34 dock (new tab or status strip); pairs with the metatron cloud-tier-unreachable spinner fix noted on TASK-34.
<!-- SECTION:DESCRIPTION:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Sequencing: depends on TASK-34's dock landing (its natural home is a dock status surface) — open/merge the TASK-34 PR first. Scope note: pair with the metatron cloud-tier-unreachable spinner fix recorded on TASK-34 (same 'daemon knows, UI doesn't say' family).

Re-grounding 2026-07-22: narrate.go router gate holds (~:261). Blocker cleared — TASK-34's dock is Done and landed; the sequencing note about merging the TASK-34 PR first is obsolete. Unblocked.

TASK-66 / decision-6 (2026-07-23): horizon legibility is a PREREQUISITE for classroom mode — a suppressed planner without a visible verdict reads to a learner as 'my prompt did nothing.' The client-decided teaching posture (TASK-77 chain-completion, TASK-78 soft speed cap) folds its learner-facing legibility needs into this task rather than a new one; spec 024 US6 (routing legibility) is the adjacent surface.

Drift audit 2026-07-23: PARTIALLY overtaken — the TUI decisions view now marks suppressed chains ('didn't think': internal/tui/decisions.go:232, :358; rendered views.go:1711), so 'suppression exists only in raw cog.outcome payloads' is no longer true. Still missing (remaining scope): the live per-class horizon verdict at current speed (header/status 'conversations suppressed at 32x') and suppression counters. Narrate gate moved to internal/mind/narrate.go:266-269 (was ~:260).
<!-- SECTION:NOTES:END -->
