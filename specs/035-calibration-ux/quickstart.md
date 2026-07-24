# Quickstart: validating Calibration UX (spec 035)

Prerequisites: built binaries (`go build ./...`), an Ollama-style local endpoint for the
calibrate step (only step 5 needs a live model; steps 1–4 work with any llm.json present).

## 1. Uncalibrated boot warns (US2)

```sh
promptworld new demo-ux            # world with llm.json, no calibration.json
promptworld daemon demo-ux
```

**Expect**: boot output contains the UNCALIBRATED warning block — pessimistic-defaults statement,
per-class horizon summary at bootstrap values, and `promptworld calibrate demo-ux` suggestion
(contracts/warnings.md §1). A world with calibration.json prints the existing seeded line only.

## 2. Raising speed into suppression warns, but still applies (US1)

```sh
promptworld set-speed demo-ux 32x
```

**Expect**: reply shows speed now 32x AND the warning naming the suppressed classes + calibrate
suggestion. Then `promptworld set-speed demo-ux 4x` → no warning. On a calibrated world, 32x →
no warning. On a no-llm world, any speed → no warning (and `max` still errors on LLM worlds —
unchanged gate).

## 3. Status shows calibration state persistently (US3)

```sh
promptworld status demo-ux
```

**Expect**: per-provider rows marked `uncalibrated (bootstrap)`. After step 5 + daemon restart:
rows show the profile's `calibrated_at` timestamp. Wire check: `calibrated_at` field absent for
bootstrap providers, present for calibrated ones (contracts/warnings.md §3).

## 4. No behavior change (FR-007 / SC-003)

```sh
go test ./...
```

**Expect**: all green, including untouched routing/estimator/governor suites. No test asserting
routing decisions needed modification (only output/reply-shape tests are new or extended).

## 5. Calibrate discloses the sequential floor (US4)

```sh
promptworld calibrate demo-ux
```

**Expect**: per-provider results + horizon summary as today, plus the sequential-floor disclosure
adjacent to the summary (contracts/warnings.md §4). Re-run step 1: boot warning gone; step 2 at
32x: warning gone (this rig's measured rate does not suppress) or truthfully present per the
measured rate.

## Sign-off checklist

- [ ] Boot warning: absent-profile AND unreadable-profile worlds warn; calibrated world byte-identical
- [ ] set_speed warning: fires only on (uncalibrated ∧ suppressing speed); speed always applies
- [ ] Status: `calibrated_at` per provider truthful, incl. partial profiles
- [ ] Calibrate: disclosure present in legacy and v2 output paths
- [ ] `go test ./...` green; gofmt clean on touched files (TASK-83 pre-existing drift excluded)
