# Data Model: Behavioral Test Coverage for Metatron and Persona Packages

**Feature**: 023-metatron-persona-tests | **Date**: 2026-07-23

A tests-only feature has no new persistent entities; the "data model" is the map of
contracts under test and the fixtures that exercise them.

## 1. Contracts under test (metatron)

| Contract | Seam | Invariant pinned |
|----------|------|------------------|
| Tail windows | `tailOfFile(path, n)`, `soulTail()`, `transcriptTail()` | `tailOfFile`: last-n-bytes; whole file when shorter; `""` on missing file. `soulTail`: 4000-byte window (`soulTailBytes`). `transcriptTail`: 3000-byte read trimmed to the last 6 whole `[`-delimited turns (`transcriptTailTurns`), newest-last |
| Charge mirror | `Observe` → `run()` → `replica.Apply` → `mirrorState` → `Status().Charges` | `metatron.charge_regenerated` accrues +1, never above `sim.MetatronChargeCap` (3); `metatron.nudged` decrements; the turn-visible snapshot tracks the replica |
| Turn serialization | `Turn` / `turnBusy` CAS / `ErrTurnBusy` | Two concurrent `Turn` calls: exactly one proceeds, the loser fails fast with `ErrTurnBusy`, the winner completes after release; race-clean |
| Notify backpressure | `Observe` | Non-blocking: with the events channel full, `Observe` returns immediately (drops), never wedges the caller |
| Absorb mirrors | `run()` / `mirrorState` | An observed batch refreshes charges/alive/agentXY and the chronicle story tail (last 8 entries) visible to the next turn |

## 2. Contracts under test (persona)

| Contract | Seam | Invariant pinned |
|----------|------|------------------|
| Index-aligned maps | `Texts`, `Anchors`, `DriftMarkers`, `Secrets` | For every `sim.AgentNames` entry: present and non-empty in all four maps; no stray keys beyond `AgentNames`; `DriftMarkers` lists non-empty |
| Anchor ≡ Temperament | `Anchors` vs `Texts` | Each anchor string appears verbatim in its persona's `**Temperament:**` line (documented "deliberately identical" invariant) |
| Load degrade | `Load` | Unreadable persona file (mode 0000) → empty string for that agent, others unaffected; already-tested missing-file case stays green |
| Genesis seeding | `Genesis` | Seeds `journal.md` (with rune-budget header) and `charter.md` (= `DefaultCharter`) on first run; a pre-existing `charter.md` is NEVER overwritten; second `Genesis` errors (existing test) |
| Secret genesis | `SecretEvents` | One tick-0 `social.secret_seeded` event per agent, `Agent` field index-aligned with `sim.AgentNames`, tone −70, text from `Secrets` |

## 3. Fixtures

| Fixture | Construction | Used by |
|---------|--------------|---------|
| Test angel (goroutines closed) | existing `newTestAngel(t, reply)` | tail windows, concurrency (white-box mirror pokes) |
| Test angel (goroutines alive) | `newTestAngel` variant that skips `Close()` and registers `t.Cleanup(mt.Close)` | charge mirror via `Observe`, absorb mirrors, notify backpressure |
| Oversized soul/transcript | write > 4000-byte soul / > 6-turn transcript into the temp `metatron/` dir | tail windows |
| Blocking loop script | `mt.runLoop = func(...) { <-release; ... }` | turn serialization |
| Genesis'd world | `persona.Genesis(t.TempDir())` | persona lifecycle tests |
| Unreadable persona | `os.Chmod(PersonaPath(...), 0o000)` + cleanup restore | load degrade |

## 4. State transitions

None — no production state changes. The only mutated repository artifacts are new
`_test.go` files and `docs/wiki/testing-strategy.md`.
