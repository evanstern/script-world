---
id: TASK-57
title: 'Rename project: script-world → promptworld'
status: In Progress
assignee: []
created_date: '2026-07-22 13:01'
updated_date: '2026-07-22 13:10'
labels: []
dependencies: []
ordinal: 50000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Mechanical repo-wide rename per user request. Trivial-exemption task (surgical mechanical rename, full inventory pinned below, ACs on this task).

Inventory (grep, 2026-07-22):
- GitHub repo evanstern/script-world → evanstern/promptworld (gh repo rename; old URLs redirect)
- go.mod module github.com/evanstern/script-world → github.com/evanstern/promptworld (18 .go files import it)
- cmd/scriptworld/ → cmd/promptworld/ (binary name)
- docs/wiki/cli-scriptworld.md → cli-promptworld.md; 21 wiki notes mention the name (re-pin after merge)
- .gitignore /scriptworld → /promptworld; rebuild local binary
- ~239 tracked text files contain script-world (321×) / scriptworld (442×): README, CLAUDE.md, specs/, docs/, e2e, topics, research
- EXCLUDED: backlog/ (33 files; never hand-edit, historical record)
- NOT in-session: local folder ~/Claude/Code/script-world/… rename (would break running session cwd; handed to user)
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 GitHub repo renamed to evanstern/promptworld and origin remote URL updated
- [x] #2 Go module path is github.com/evanstern/promptworld; all imports updated; go build ./... and go test ./... green
- [ ] #3 cmd/scriptworld renamed to cmd/promptworld; local binary rebuilt as promptworld; .gitignore updated
- [x] #4 All tracked text references outside backlog/ renamed (script-world/scriptworld → promptworld)
- [ ] #5 Wiki notes updated and re-pinned post-merge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Branch task-57-rename-promptworld: rename commit be59870 + wiki note commit 8b324a8 (docs/wiki/project-rename.md, indexed). PR #34 open, full suite green incl. e2e. GitHub repo renamed & origin updated (AC1 done). PR merge blocked by session permission classifier — Evan merges #34. Post-merge remaining: rebuild binary as promptworld (AC3), wiki re-pin incl. project-rename note (AC5), stop daemon pid 53069 then mv ~/.scriptworld ~/.promptworld, rename local checkout folders.
<!-- SECTION:NOTES:END -->
