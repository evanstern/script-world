# Quickstart: validating spec 014 (Tool Registry)

How to prove the feature works, end to end. Prerequisites: Go toolchain (repo builds
with `go build ./...`), a checkout where TASK-51 (spec 013) has already merged
(sequencing clarified 2026-07-22).

## 0. Capture the byte-identity anchors BEFORE the refactor

From the pre-refactor commit (implementation step one, on the task branch before any
registry code):

```sh
go test ./internal/mind -run TestGoldenPrompt -update-golden   # creates the fixture
go test ./... > /tmp/pre-refactor-tests.txt                    # all green baseline
```

(The golden-prompt test is new — written first, against the OLD code, so the fixture
records today's exact prompt bytes.)

## 1. Registry exists and validates

```sh
go test ./internal/tool
```

Expected: registration/validation tests pass — unique names, class coherence, rosters
resolve, the single-walk invariant (vocabulary ≡ parse set ≡ plan-step set), and
malformed-fixture cases each fail `Validate()`.

## 2. Derived surfaces replace the maps (SC-004)

```sh
grep -rn "goalVocabulary\|validGoals\|planGoals" internal/ && echo "DRIFT SITES REMAIN" || echo OK
```

Expected: `OK` — the three names are gone from non-test code (test files pinning
history may reference them in comments only).

## 3. Behavior identity (SC-002, SC-003)

```sh
go test ./internal/mind -run TestGoldenPrompt   # byte-identical prompt from registry
go test ./internal/sim -run 'Replay|Determinism'  # replay byte-identity suite
go test ./...                                    # full suite green
```

Expected: the golden fixture captured in step 0 passes UNCHANGED against the derived
prompt; every pre-existing replay/determinism test passes unmodified.

## 4. The one permitted delta (FR-012 / TASK-55)

```sh
go test ./internal/sim -run TestPlanStepVocabulary
```

Expected: a plan step naming each of the 9 spec-012 verbs (quarry, collect_water, cook,
refuel_fire, craft_planks, craft_stone, craft_spear, build_oven, bathe) is ACCEPTED at
the door (was: rejected). This is the drift cure — the only behavioral difference.

## 5. Rosters enforced (SC-005)

```sh
go test ./internal/sim -run OutOfRoster
go test ./internal/metatron
```

Expected: villager action naming a metatron tool → rejected, no event lands; metatron
attempting a world verb form → rejected; both rejections non-fatal, same shape as
today's unknown-goal path. Metatron charge/validation tests pass unmodified.

## 6. Doors and ladder untouched (FR-013/FR-014)

```sh
go test ./internal/sim -run 'Whitelist|Cognition|Governance|Nudge|Charge'
```

Expected: all pre-existing whitelist, generation, staleness, and guard tests pass with
zero edits; `injectSocialWhitelist` diff against main is empty.

## 7. Live smoke (optional but recommended before PR)

```sh
go run ./cmd/promptworld new /tmp/sw-014 && go run ./cmd/promptworld run /tmp/sw-014
```

Watch one planner cadence: villagers plan/speak/muse as before; a multi-step plan
containing `cook` or `quarry` now lands (step 4's delta, visible live). Then
`promptworld tail` shows the same event vocabulary as a pre-refactor world.

## Done means

All of 1–6 green + `go vet ./...` clean + wiki re-pin (`/grounding-wiki:wiki-update` —
touched sources: agent-mind, sim-loop, reflex-policy, executor, metatron, cognition,
event-types notes) + spec-bridge sync moves TASK-53 per artifacts.
