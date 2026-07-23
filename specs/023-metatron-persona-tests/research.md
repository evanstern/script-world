# Research: Behavioral Test Coverage for Metatron and Persona Packages

**Feature**: 023-metatron-persona-tests | **Date**: 2026-07-23

Phase 0 resolves what to test (the gap analysis against the existing suites) and how
(the conventions already established in the two packages). No NEEDS CLARIFICATION
markers existed in the Technical Context; the research questions were empirical —
answered by reading the sources and inventorying the existing tests.

## R1 — Gap analysis: what the existing suites already prove

**Decision**: fill only the verified gaps below; do NOT re-test the TASK-64
instruction surface, which is already thoroughly covered.

**Rationale**: `internal/metatron/metatron_test.go` (1513 lines, 26 test functions,
post-TASK-64) already covers: turn converse/degraded/fallback paths, nudge and
miracle landing (charge decrement, atomicity, perception memories), zero-bank
refusal, the firewall sentinel, charter fallbacks and per-read reload, provenance
via `Status()`, skill-file eligibility/ordering/caps (`TestLoadSkills`,
`TestPromptDeterminism`), fixed-frame non-negotiables under an adversarial battery
(`TestFixedFrameHolds`), capability-manifest semantics and all three gating layers
(`TestLoadManifest`, `TestGatingLayers`, `TestNoManifestByteCompat`), digest
windows and carry-on-failure, and cog.tool_call telemetry. Duplicating any of that
violates FR-009 (behavioral, not redundant) and the spec assumption "fill gaps
rather than rewrite".

**Verified metatron gaps** (each confirmed absent from the existing suite):

| Gap | Seam | Evidence |
|-----|------|----------|
| Soul/transcript tail windows | `metatron.go` `tailOfFile`, `soulTail` (4000 bytes), `turn.go` `transcriptTail` (3000 bytes → last 6 whole turns) | No test calls or truncation-tests these readers; only content-writing is asserted |
| Charge cap + regeneration via the replica seam | `metatron.go` `run()`/`Observe`/`mirrorState` applying `metatron.charge_regenerated` (reducer arm caps at `sim.MetatronChargeCap`) | Neither test file references `MetatronChargeCap` accrual or `metatron.charge_regenerated`; only decrement and zero-refusal are covered |
| True concurrent turn serialization | `turn.go` `Turn` / `turnBusy` CAS | `TestTurnSingleFlight` flips the flag manually — no goroutine race under `-race` |
| Observe never blocks | `metatron.go` `Observe` (non-blocking send, drop on full channel) | Untested |
| Absorb pipeline end-to-end | `metatron.go` `run()` applying batches, refreshing mirrors (charges, alive, agentXY, chronicle story tail of 8) | `run()` goroutine path untested (tests call internals directly) |

**Verified persona gaps**:

| Gap | Seam | Evidence |
|-----|------|----------|
| Index-aligned map sweep | `personas.go` `Texts`/`Anchors`/`DriftMarkers`/`Secrets` vs `sim.AgentNames` | Only `Texts` completeness is tested; `Anchors`, `DriftMarkers`, `Secrets` never referenced by any test |
| Anchor ≡ Temperament line | `personas.go` comment: "Deliberately identical to the Temperament line in Texts" | Documented invariant, unenforced |
| Unreadable-file load degrade | `files.go` `Load` (any read error → empty string) | Only the missing-file case is tested |
| Genesis charter/journal seeding | `files.go` `Genesis` (seeds `charter.md` once, never overwrites; seeds `journal.md`) | Only persona.md/soul.md asserted |
| `SecretEvents` | `files.go` (one tick-0 `social.secret_seeded` per agent, tone −70, index-aligned Agent) | Untested |

**Alternatives considered**: re-testing the instruction surface "for depth" —
rejected (duplication, FR-009); testing the charge regen/cap reducer arm in the sim
package instead — rejected as out of scope (the arm itself is sim territory and
`internal/sim/miracles_test.go` covers charge doctrine; what is missing HERE is the
metatron-side replica/absorb seam that consumes those events).

## R2 — "Corrupt file" semantics for persona.Load

**Decision**: interpret "corrupt" as *unreadable* (permission-denied), asserting
`Load` degrades to an empty string exactly like the missing-file case.

**Rationale**: persona files are plain Markdown read with `os.ReadFile` — there is
no parse step, so no malformed-content failure mode exists; the only read-time
failure classes are missing and unreadable, and `Load`'s contract (`err != nil` →
skip, empty string) treats them identically. The test pins that contract.

**Alternatives considered**: inventing a content-validation layer to make
"corrupt" meaningful — rejected: FR-011 forbids production changes, and the
documented contract is degrade-don't-fail.

## R3 — How to test the charge cap/regeneration seam without the sim executor

**Decision**: drive the metatron replica through its own absorb path — deliver
`metatron.charge_regenerated` and `metatron.nudged` events (via `Observe` with the
`run()` goroutine alive, or `replica.Apply` + `mirrorState()` white-box where the
goroutine is closed) — and assert `Status().Charges` accrues, caps at
`sim.MetatronChargeCap`, and decrements. Do NOT spin the sim executor.

**Rationale**: the executor's emission cadence (`chargeRegenTicks`) is sim-package
territory; the metatron package's own contract is "the replica and its mirrors
faithfully reflect charge events". The existing suite already pokes unexported
state under `stateMu` (white-box `package metatron` tests), so both styles are
established.

## R4 — Concurrency test shape for ErrTurnBusy

**Decision**: install a scripted `runLoop` that blocks on a channel; launch a
second `Turn` while the first is parked; assert the second returns `ErrTurnBusy`
fast and the first completes normally after release. Run meaningfully under
`-race` (two real goroutines contending on the CAS).

**Rationale**: upgrades the manual-flag test into a genuine serialization proof
without timing flakiness (channel-gated, not sleep-gated) — the shape
`TestQueueBackpressure`'s flake (TASK-69) teaches us to avoid.

## R5 — Test conventions to reuse (established in the packages)

**Decision**: follow the existing white-box conventions verbatim:

- `newTestAngel(t, reply)` constructor (temp dir + `persona.Genesis` + deterministic
  `worldmap.Generate(42,64,64)` + `mockOrch` + `stateInjector`, goroutines closed);
  add a variant that keeps goroutines alive for the Observe/absorb tests.
- Scripted loop drivers: `converseLoop` / `actLoop` / bespoke closures via
  `mt.runLoop` reassignment; `mockOrch` for `Submit`-level stubbing.
- `stateInjector` for door landings; `landedBatches` / `cogToolCalls` extractors.
- `t.TempDir()` for all filesystem work; explicit tick values, no fake clock.
- Standard library `testing` only, `t.Run` table subtests, `t.Helper()` helpers,
  doc comment per test naming the spec/AC it proves.
- Persona tests stay in `package persona` (white-box), same file or a new
  `files_test.go`/`personas_test.go` split if size warrants.

**Rationale**: FR-008 (codebase style) and the constitution's grounding principle —
the conventions are already proven race-clean and hermetic (no network anywhere:
`mockOrch` cans replies; nothing dials out).

## R6 — Wiki re-pin scope

**Decision**: update and re-pin `docs/wiki/testing-strategy.md` only (add the two
packages' behavioral suites to its narrative and sources). No other note re-pins
are required by the freshness gate because no production source file changes; the
notes sourcing `internal/metatron/*.go` / `internal/persona/*.go`
(metatron, metatron-miracles, agent-mind, agent-journal, nightly-consolidation)
pin against unchanged files.

**Rationale**: Principle IV ties re-pinning to changes touching listed sources;
tests-only PRs touch test files and the testing-strategy note itself.
