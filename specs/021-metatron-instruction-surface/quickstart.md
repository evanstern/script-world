# Quickstart Validation: Metatron Instruction Surface

Prereqs: repo built (`go build ./...`), a scratch world (`./promptworld start <name>` or
the e2e helpers). Full contract details: [contracts/](./contracts/),
[data-model.md](./data-model.md).

## 1. Unit/fixture proof (fast path)

```bash
go test ./internal/tool/ ./internal/metatron/ ./internal/sim/ ./internal/tui/
go test ./...   # full sweep — no-manifest worlds must stay byte-compatible (SC-003)
```

Expected: adversarial fixture tests (SC-002 battery, contracts/instruction-surface.md)
green; guidance/cost drift tests green; prompt-composition determinism test green.

## 2. Skill hot-reload (US1, SC-001)

1. Start a world; open the TUI Metatron console; note a baseline reply.
2. `mkdir -p <worldDir>/skills && echo "Always answer in exactly three sentences." > <worldDir>/skills/10-three.md`
3. Send another message — the NEXT reply already obeys (no restart).
4. Delete the file; next reply reverts.
5. Write a 5,000-char skill file → next reply carries a truncation notice.

## 3. Gated roster (US2, SC-003)

1. `echo '{"tools":["nudge_dream"]}' > <worldDir>/capabilities.json`
2. Ask the angel for an omen → it must counsel/refuse; no omen can land (check the
   chronicle: no `metatron.nudged` omen event). `cog.tool_call` records for the turn show
   no omen declaration was even available.
3. `echo '{"tools":["work_miracle"],"miracle_kinds":["give_item"]}' > <worldDir>/capabilities.json`
   → next turn: a time_snap request is refused; a give_item can land.
4. `rm <worldDir>/capabilities.json` → full roster back next turn.
5. `echo 'not json' > capabilities.json` → next reply carries a notice; full roster
   (permissive fallback).

## 4. Cost single-source (SC-004)

Change `time_snap` to 3 in the ONE table (`internal/tool` registry.go) in a scratch
branch: `go test ./...` — the drift tests must FORCE agreement (prose render, sim
enforcement both reflect 3, no second edit needed). Revert.

## 5. Status/TUI provenance (US3, SC-005)

1. Fresh world → header: `default charter` (quiet tools part).
2. Edit charter + add two skills + subset manifest → header shows
   `custom charter · 2 skills · tools: dream` (matching contracts/status.md).
3. `./promptworld status <name>` (or IPC peek) → JSON carries `skills`,
   `granted_tools`, `manifest_default` per contract.

## 6. Curriculum-substrate smoke (SC-006)

Write two fixture manifests shaped like TASK-68 stages (stage-1 basics:
`{"tools":["nudge_dream"]}`; stage-3 full) and assert both load into the expected grant
sets — proves presets are pure data.
