---
name: project-rename
description: The 2026-07-22 project rename script-world → promptworld — what changed (repo, module path, binary, env var, data dir) and what deliberately kept the old name
kind: concept
sources:
  - go.mod
  - README.md
  - internal/worlds/home.go
  - .gitignore
verified_against: be38288fa137064174eedbfb3b8a94cc5b1fb0b9
---

# Project rename: script-world → promptworld

On 2026-07-22 the project was renamed from **script-world** to **promptworld**
(TASK-57, PR #34) — a mechanical, repo-wide rename with no behavior change beyond
the identifiers themselves. The new name reflects the core pillar better: agents are
programmed via **AI prompting**, not scripts.

## What changed

- **GitHub repository**: `evanstern/script-world` → `evanstern/promptworld`
  (renamed via `gh repo rename`; GitHub redirects old URLs, but remotes should
  point at the new one).
- **Go module path**: `module github.com/evanstern/promptworld` in `go.mod`;
  every internal import path moved with it.
- **CLI binary / package**: `cmd/scriptworld` → `cmd/promptworld`; the build
  artifact is `promptworld` (gitignored as `/promptworld`).
- **Home env var**: `SCRIPTWORLD_HOME` → `PROMPTWORLD_HOME`
  (`internal/worlds/home.go`).
- **Default worlds home**: `~/.scriptworld` → `~/.promptworld`. Existing installs
  must move their data (`mv ~/.scriptworld ~/.promptworld`) after stopping any
  running daemons — the code does not auto-migrate the old directory.
- **Docs and corpus**: README, specs, this wiki (including
  [[cli-promptworld]], formerly `cli-scriptworld`), and the educate topic
  `topics/promptworld-design/` all use the new name.

## What deliberately kept the old name

- **`backlog/` board history**: task files are CLI-owned historical record and are
  never hand-edited; pre-rename tasks still say "script-world"/"scriptworld" in
  their titles and notes. That is intentional, not drift.
- **Old on-disk worlds**: world save directories under a not-yet-migrated
  `~/.scriptworld` keep working only with pre-rename binaries; the renamed binary
  looks in `~/.promptworld` (or `PROMPTWORLD_HOME`).

## Connections

- [[overview]] — the system this name now refers to
- [[cli-promptworld]] — the renamed CLI entrypoint
- [[world-save-directory]] — the per-world layout that lives under the renamed home
