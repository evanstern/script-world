# Feature Specification: Behavioral Test Coverage for Metatron and Persona Packages

**Feature Branch**: `task-74-metatron-persona-tests`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Behavioral test coverage for the metatron and persona packages (board TASK-74). internal/metatron has 1 test file against 6 source files; internal/persona 1 against 3. Add behavioral tests (not change-detectors) in the codebase style: scripted stubs, no network, -race clean. Metatron scope: turn dispatch and grant/charge accounting, charter provenance, fixed-frame composition, transcript/soul tail windows, ErrTurnBusy serialization; also cover the post-TASK-64 instruction-surface seams where TASK-64 left them thin. Persona scope: genesis-once semantics, load behavior on missing/corrupt files, anchor/drift-marker alignment with Texts including a sweep test over the index-aligned maps. Deliverable also updates the wiki testing-strategy note with the new coverage."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Metatron economy and turn dispatch are provably correct (Priority: P1)

A maintainer changing Metatron's turn handling or its miracle-charge economy (charge
decrement on use, banking cap, regeneration cadence) gets fast, deterministic feedback
from the package's own test suite when they break a contract — instead of discovering the
break later through a live world misbehaving or a distant integration test.

**Why this priority**: the charge economy and turn dispatch are the package's core
behavioral contracts, are gameplay-facing (the player's angel visibly stops working when
they break), and today are covered only indirectly from other packages.

**Independent Test**: run the metatron package test suite alone; economy and dispatch
scenarios pass without any other package's tests and without network access.

**Acceptance Scenarios**:

1. **Given** an angel with N charges banked, **When** a chargeable miracle is performed,
   **Then** the bank decrements by the miracle's cost and the decrement is observable.
2. **Given** an angel at the banking cap, **When** regeneration ticks accrue, **Then**
   the bank never exceeds the cap.
3. **Given** an angel below the cap, **When** the regeneration interval elapses,
   **Then** exactly the expected number of charges accrue (no drift, no double-credit).
4. **Given** a turn already in flight, **When** a second turn is requested,
   **Then** the caller receives the busy error and the in-flight turn is unaffected
   (serialization contract).
5. **Given** a completed turn, **When** the turn record is inspected, **Then** dispatch
   routed the request to the expected handler for its kind.

---

### User Story 2 - Instruction-surface composition is pinned by tests (Priority: P2)

A maintainer editing how the angel's operating instructions are assembled — the fixed
frame, the operator-editable charter, skill files, and the capability manifest (the
TASK-64 instruction surface) — can rely on tests that pin the non-negotiable parts and
the composition order, so a refactor cannot silently drop a safety frame or reorder
instruction precedence.

**Why this priority**: the instruction surface is doctrine-adjacent (the fixed frame
must survive beneath ANY operator charter/skill text) and newly built in TASK-64; it is
the seam most likely to be edited next and the cost of a silent regression is highest.

**Independent Test**: with synthetic charter and skill files on disk, composing the
instruction surface yields the fixed-frame invariants and documented ordering; provenance
distinguishes default from custom charters and reflects on-disk edits per read.

**Acceptance Scenarios**:

1. **Given** any custom charter and skill text, **When** the instruction surface is
   composed, **Then** both fixed-frame non-negotiables are present beneath the custom
   text.
2. **Given** no custom charter on disk, **When** provenance is inspected, **Then** it
   reports the default charter; **Given** a custom charter is written, **Then** the next
   read reports custom and reflects the new content (per-read reload).
3. **Given** multiple skill files, **When** the surface is composed, **Then** they appear
   in the documented composition order.
4. **Given** a capability manifest that gates a tool off, **When** the tool roster is
   assembled, **Then** the gated tool is absent from what the angel is offered.
5. **Given** transcript and soul histories longer than their tail windows, **When**
   context is assembled, **Then** only the configured tail is included.

---

### User Story 3 - Persona lifecycle guarantees are enforced by tests (Priority: P3)

A maintainer touching villager persona storage can prove the lifecycle guarantees:
genesis happens exactly once (a second genesis cannot overwrite an existing soul; persona
files are written read-only), loading degrades predictably on missing or corrupt files,
and the package's index-aligned text/anchor/drift-marker maps can never silently drift
out of alignment.

**Why this priority**: persona corruption destroys a villager's identity permanently
(the persona firewall depends on these files being immutable), but the failure modes are
rare in practice, so tests are the only realistic way they get exercised.

**Independent Test**: run the persona package test suite alone against temp-dir
fixtures; genesis-once, load-robustness, and map-alignment sweeps pass with no network.

**Acceptance Scenarios**:

1. **Given** a persona already generated for an agent, **When** genesis is invoked again,
   **Then** the existing persona survives unmodified (no-op or explicit error).
2. **Given** a freshly generated persona file, **When** its permissions are inspected,
   **Then** it is read-only (0444).
3. **Given** a missing persona file, **When** a load is attempted, **Then** the caller
   receives the documented missing-file behavior (not a crash or a fabricated persona).
4. **Given** a corrupt persona file, **When** a load is attempted, **Then** the failure
   is explicit and identifies the file.
5. **Given** the package's index-aligned maps (texts, anchors, drift markers), **When**
   the alignment sweep runs, **Then** every index present in one map is present in all,
   for all defined personas.

---

### Edge Cases

- Charge regeneration arriving in the same instant a miracle spends: accounting must not
  lose or double-count either side.
- A charter file deleted between two reads: provenance must fall back to default without
  crashing the in-flight turn.
- Zero skill files / empty manifest: composition still yields a valid surface containing
  the fixed frame.
- A persona directory that exists but is empty vs. one that does not exist at all: load
  behavior must be the documented one for each.
- Concurrent turn requests racing on the busy-serialization seam: exactly one proceeds
  (tests must be meaningful under the race detector).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The metatron package MUST have behavioral test coverage of the exported
  seams of every one of its source files (turn dispatch, charge economy, charter
  provenance, fixed-frame composition, digest/transcript windows, tool-call handling,
  miracle batching), asserting observable behavior rather than implementation structure.
- **FR-002**: Tests MUST explicitly cover the charge economy: decrement on chargeable
  use, the banking cap, and regeneration accrual.
- **FR-003**: Tests MUST pin the fixed-frame composition invariant: the two
  non-negotiable frame elements are present beneath any combination of charter and skill
  text.
- **FR-004**: Tests MUST cover charter provenance (default vs custom detection) and
  per-read reload semantics.
- **FR-005**: Tests MUST cover the post-TASK-64 instruction-surface seams — skill-file
  composition order and capability-manifest tool gating — wherever existing TASK-64
  tests left them uncovered (no duplication of already-proven scenarios).
- **FR-006**: Tests MUST cover transcript/soul tail-window truncation and the turn-busy
  serialization contract.
- **FR-007**: The persona package MUST have behavioral coverage of genesis-once
  semantics (second genesis is a no-op or error; files written read-only 0444), load
  behavior on missing and corrupt files, and anchor/drift-marker alignment with Texts —
  including a sweep test proving the index-aligned maps stay consistent across all
  defined personas.
- **FR-008**: All new tests MUST follow the codebase testing style: scripted stubs for
  any model interaction, no network access, deterministic fixtures (temp dirs), and
  green under the race detector.
- **FR-009**: New tests MUST be behavioral, not change-detectors: they assert contracts
  a maintainer relies on, not exact strings/structures that legitimate refactors would
  churn.
- **FR-010**: The wiki testing-strategy note MUST be updated to record the new coverage
  and re-pinned per the grounding-freshness gate.
- **FR-011**: The change MUST be test-and-docs only: no production source behavior may
  change. Any real defect the new tests expose MUST be reported on the board as its own
  task rather than silently fixed in this deliverable (unless the defect makes a
  required test impossible to land, in which case it is surfaced for a scope decision).

### Key Entities

- **Angel (Metatron)**: the operator-editable agent; owns a charter, skill files, a
  capability manifest, a miracle-charge bank, and a turn loop.
- **Charge bank**: per-angel miracle currency with a cost-per-miracle, a banking cap,
  and time-based regeneration.
- **Instruction surface**: the composed operating context — fixed frame (non-negotiable)
  + charter (operator-editable, provenance-tracked) + skill files (ordered) + capability
  manifest (tool gating).
- **Persona**: a villager's immutable identity artifact — generated once, stored
  read-only, loaded thereafter; internally index-aligned across texts, anchors, and
  drift markers.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every metatron source file's exported seams are exercised by at least one
  behavioral test in the package's own suite (6/6 files, up from 1 test file today).
- **SC-002**: Every persona lifecycle guarantee named in scope (genesis-once, file mode,
  missing/corrupt load, map alignment) has at least one dedicated test (up from minimal
  coverage today).
- **SC-003**: The full project test suite passes under the race detector with the new
  tests included, with zero network access required.
- **SC-004**: The alignment sweep test fails if any index-aligned persona map gains or
  loses an entry relative to its peers (verified by construction during review).
- **SC-005**: The wiki testing-strategy note reflects the new coverage and passes the
  freshness gate.

## Assumptions

- This is a coverage deliverable: production behavior is assumed correct as-built; tests
  document and pin the CURRENT contracts. Defects discovered en route are carded
  separately (see FR-011), keeping this PR pure.
- The existing metatron test file (post-TASK-64) already covers part of the instruction
  surface; this feature fills gaps rather than rewriting what is already proven.
- "The two non-negotiables" of the fixed frame are the ones established by TASK-64's
  instruction-surface design; the plan phase will name them precisely from the code.
- The codebase's established test conventions (scripted stubs, temp-dir fixtures, table
  tests, race-detector cleanliness) are the style baseline; no new test infrastructure
  is expected.
- Wiki work is limited to the testing-strategy note (plus any note whose sources list
  the touched files, per the freshness gate); no course re-render is in scope.
