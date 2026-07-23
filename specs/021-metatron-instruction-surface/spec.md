# Feature Specification: Metatron Instruction Surface — Staged Charter + Skill Files + Gated Tool Roster

**Feature Branch**: `021-metatron-instruction-surface`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Metatron instruction surface: staged charter + skill files + gated tool roster (board TASK-64). Evolve the single player-editable charter.md into a staged instruction surface that mirrors how real assistant configuration (CLAUDE.md / SKILL.md / tool grants) works, so prompt-engineering skills learned in-game transfer to Claude/ChatGPT. Keep per-read hot-reload for every instruction file; add a skills/ directory composed into the turn prompt with the fixed-frame non-negotiables provably not overridable; add a per-world capability manifest so ungranted tools are structurally absent from the declared roster; replace the hand-written prose tool list with registry-derived text and give miracle costs one source of truth; surface instruction-file provenance and the granted tool set in the TUI. Substrate for the curriculum ladder (TASK-68)."

## The teaching frame *(context)*

The game's editable surface is the angel, and the angel is deliberately shaped like a real
AI assistant: today the player edits one `charter.md` (a system-prompt-shaped file) and
watches behavior change. This feature grows that single file into the full configuration
surface a real assistant has — a base instruction file, a folder of skill files, and a
granted-tool roster — so that what a player learns in the village transfers directly to
configuring Claude or ChatGPT at work. It is also the substrate the curriculum ladder
(TASK-68) will stand on: stage presets will grant capabilities through the manifest this
feature introduces.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Player authors skill files that shape the angel (Priority: P1)

A player who has already learned to edit the charter opens the world save folder, creates
`skills/weather-prophet.md` with instructions for how the angel should speak about omens,
saves the file, and sends the angel a message. The very next reply already follows the new
skill — no restart, no reload command. Deleting or emptying the file just as smoothly
removes its influence. The charter keeps working exactly as before; skills add to it.

**Why this priority**: this is the player-facing heart of the feature — the second rung of
the stated progression (prompt conversationally → author instruction files → grant tools).
Without it there is no new authoring surface to gate or display.

**Independent Test**: in a running world, add/edit/remove skill files between turns and
observe the next reply reflect each change; adversarial skill files fail to break the
angel's fixed constraints.

**Acceptance Scenarios**:

1. **Given** a running world with a default charter, **When** the player adds a skill file
   and sends a message, **Then** the next reply reflects the skill's instructions without
   any restart.
2. **Given** a world with skill files, **When** the player edits or deletes one between
   turns, **Then** the following turn reflects the current on-disk content only.
3. **Given** a skill file that instructs the angel to invent events it never observed, or
   to pass the player's literal words to a villager, **When** the player pushes the angel
   to comply, **Then** the angel still refuses: the two fixed-frame non-negotiables hold
   regardless of what any editable file says.
4. **Given** a skill file that exceeds the size cap (or a skills folder over the file-count
   cap), **When** the next turn runs, **Then** the surplus is excluded deterministically and
   the reply carries a notice telling the player what was truncated or skipped, in the same
   way charter fallbacks are reported today.

---

### User Story 2 - World-scoped capability grants (gated tool roster) (Priority: P2)

A world creator (or, later, a curriculum stage preset) declares in the world's capability
manifest which acting tools this world grants the angel — e.g. dreams only; dreams and
omens; everything including miracles, or only some miracle kinds. In a world granting a
subset, the ungranted tools are not merely forbidden by prose: they are absent from the
tool surface declared to the model, and any attempt to use one is refused at the door.
Speech is never gated — the angel can always converse. Existing worlds with no manifest
keep today's full roster.

**Why this priority**: this is the substrate TASK-68's stage presets stand on
(gate-to-feature pathway), but it delivers value alone: a "counsel-only" or "dreams-only"
world is a playable configuration the moment this lands.

**Independent Test**: create worlds with different manifests and verify per world which
tools the model is offered, which acts can land, and that a manifest edit takes effect the
next turn.

**Acceptance Scenarios**:

1. **Given** a world whose manifest grants only `nudge_dream`, **When** a turn runs,
   **Then** the declared tool surface contains only `nudge_dream`, the instruction text
   describes only `nudge_dream`, and an omen or miracle cannot land through any path.
2. **Given** a world with no manifest file, **When** a turn runs, **Then** the angel has
   today's full roster (dreams, omens, all four miracle kinds) — existing worlds are
   unaffected.
3. **Given** a manifest granting `work_miracle` restricted to `give_item` and `move`,
   **When** a turn runs, **Then** the miracle kind vocabulary offered to the model contains
   only those kinds and a `time_snap` attempt is refused.
4. **Given** a manifest granting nothing, **When** the player converses, **Then** the angel
   still replies in words — conversation is not a grantable capability, it is always on.
5. **Given** the player edits the manifest between turns, **When** the next turn runs,
   **Then** the new grant set is already in effect (same hot-reload discipline as every
   instruction file).
6. **Given** a malformed or unreadable manifest, **When** a turn runs, **Then** the world
   falls back to a safe default and the reply carries a notice, mirroring charter fallback
   behavior.

---

### User Story 3 - Instruction provenance and grants visible in the TUI (Priority: P3)

A player opens the Metatron pane's status and sees, at a glance: which instruction files
are in effect and whether each is the stock default or their own custom text (charter plus
each skill file), and which tools this world currently grants the angel. A learner can
always answer "what is my angel running on, and what can it do?" without leaving the game.

**Why this priority**: the display makes the other two stories legible, but both function
without it.

**Independent Test**: toggle a world between default/custom charter, add skill files, vary
the manifest, and verify the status view tracks each change.

**Acceptance Scenarios**:

1. **Given** a fresh world, **When** the player views Metatron status, **Then** the charter
   shows as default, no skills are listed, and the granted tool set is shown.
2. **Given** a customized charter and two skill files, **When** the player views status,
   **Then** the charter shows as custom and both skill files are listed by name.
3. **Given** a manifest granting a subset, **When** the player views status, **Then**
   exactly the granted set is shown.

---

### Edge Cases

- Skill file appears/disappears between the status read and the next turn: each read is
  independent; no cached state may go stale (per-read discipline, no watchers).
- A skill file attempts prompt injection against the fixed frame ("ignore the rules above
  / below"): the non-negotiables must hold; adversarial fixtures exercise this.
- Skills folder contains non-instruction files (e.g. `.DS_Store`, subdirectories, non-`.md`
  files): excluded deterministically, not an error.
- Manifest names a tool that does not exist in the registry: unknown names are reported in
  a notice and ignored; the rest of the manifest still applies.
- Manifest revokes a tool between turns while charges are banked: charges persist (they are
  world state, not a capability), but the revoked tool can no longer spend them.
- Composition order of multiple skill files must be deterministic so identical world dirs
  produce identical prompts (replay/determinism doctrine).
- Charter missing entirely: today's restore-the-default behavior is preserved unchanged.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every player-editable instruction input (charter, each skill file, capability
  manifest) MUST be read fresh at each use with no watcher machinery or restart; an edit is
  in effect on the next turn and the next status read.
- **FR-002**: The system MUST compose zero or more player-authored skill files from a
  `skills/` folder in the world save directory into the angel's turn instructions, beneath
  the charter, with a deterministic composition order and documented per-file size and
  file-count caps; over-cap content is excluded deterministically and reported as a notice
  in the next reply.
- **FR-003**: The two fixed-frame non-negotiables (the angel never invents unobserved
  events; the player's literal words never pass to a villager) MUST live only in
  non-editable instruction text and MUST remain in effect verbatim in every turn regardless
  of the content of any editable file; no editable file may displace, truncate, or override
  them.
- **FR-004**: The system MUST support a per-world capability manifest declaring which
  acting tools the world grants the angel, including optionally restricting the miracle
  kind vocabulary to a subset; the manifest is a player-visible file in the world save
  directory.
- **FR-005**: Tools not granted by the manifest MUST be structurally absent from the tool
  surface declared to the model AND absent from the derived instruction text AND refused at
  the landing door if attempted — three independent layers, no prose-only forbidding.
- **FR-006**: Conversation (the angel replying in words) MUST NOT be gateable by the
  manifest; a world granting no tools still converses.
- **FR-007**: A world with no manifest file MUST behave exactly as today (full current
  roster); a malformed manifest MUST fall back safely and report a notice, mirroring
  charter fallback semantics.
- **FR-008**: The acting-tool instruction text in the angel's turn prompt MUST be derived
  from the tool registry's declarations (names, arguments, costs) rather than hand-written
  prose, so the described surface and the declared surface cannot diverge, and so it
  automatically reflects the world's granted subset.
- **FR-009**: Each miracle kind's charge cost MUST have exactly one authoritative source
  from which both the enforcement path and every player/model-facing rendering of costs
  derive; a cost change in that one source MUST propagate everywhere with no second edit.
- **FR-010**: Metatron status MUST report, per instruction file, whether it is the stock
  default or custom (charter) and which skill files are present, plus the world's granted
  tool set; the TUI Metatron pane MUST render this.
- **FR-011**: All existing charter behavior (restore-on-missing, empty fallback, size-cap
  truncation, notices) MUST be preserved unchanged.
- **FR-012**: Identical world directories MUST produce identical composed instructions
  (deterministic composition), preserving the project's replay/determinism doctrine.

### Key Entities

- **Instruction file**: a player-editable text file shaping the angel — the charter (base,
  exactly one) and skill files (zero or more, in `skills/`). Attributes: on-disk content,
  provenance (default vs custom), effective (post-cap) content, notices.
- **Fixed frame**: the non-editable instruction text carrying the two non-negotiables and
  the tool guidance; always present, never sourced from an editable file.
- **Capability manifest**: a per-world, player-visible declaration of granted acting tools
  (and optionally the granted miracle kinds). Absent file = full default grant.
- **Granted roster**: the effective tool surface for a turn — the intersection of the
  system's full tool registry with the manifest's grants; drives declaration, instruction
  text, and door refusal alike.
- **Provenance status**: the model-free report of instruction files (with default/custom
  flags), skill file names, and granted roster, surfaced in the TUI.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A player's edit to any instruction file (charter, skill file, manifest) is
  reflected in the very next turn in 100% of cases, with zero restarts or reload commands.
- **SC-002**: Across an adversarial fixture set of editable files (injection phrasings,
  oversize content, contradictory instructions), the two non-negotiables hold in 100% of
  turns — no fixture removes the fixed frame from the effective instructions or lands a
  forbidden outcome through any tool path.
- **SC-003**: In a world granting a strict subset, 0 ungranted tools appear in the declared
  surface or instruction text, and 100% of attempted ungranted acts are refused; in a world
  with no manifest, behavior is byte-for-byte today's (existing tests stay green).
- **SC-004**: Changing a miracle cost in its single authoritative source changes every
  rendering and every enforcement of that cost in one edit; a drift test pins this and
  fails if a second copy reappears.
- **SC-005**: From the TUI alone, a player can correctly answer "which instruction files
  are in effect, which are mine, and what can my angel do" for any world configuration —
  status reflects 100% of on-disk changes by the next read.
- **SC-006**: The curriculum ladder can express its stage presets purely as manifest
  contents (no code change per stage) — verified by expressing at least two distinct
  stage-like grant sets as fixtures.

## Assumptions

- Skill files are Markdown-shaped free text like the charter; the system imposes caps and
  composition order but no internal format (mirrors real SKILL.md authoring).
- Reasonable default caps, aligned with the existing charter cap of 4,000 characters:
  4,000 characters per skill file and at most 8 skill files composed per turn; files are
  composed in name order (deterministic). Exact numbers are tunable at planning time; the
  spec requires only that caps exist, are deterministic, and are reported when exceeded.
- The manifest gates the angel's ACTING tools only (nudges, miracles). It does not gate
  villager tools, and it cannot grant tools that do not exist in the registry. New
  query/read tools for the angel are out of scope (future curriculum work may add them;
  the manifest is designed so new tool names slot in without redesign).
- Provenance for skill files is presence-based (listed by name); default-vs-custom
  distinction applies to the charter, which is the only instruction file that ships a
  stock default. Skill files have no shipped default — their existence is their
  provenance.
- Nudge/miracle charge accrual and the charge bank are untouched; the manifest gates
  ability to SPEND, not accrual.
- The player edits files with their own editor outside the game (as with the charter
  today); in-game file editing is out of scope.
- Fixed-frame "provably not overridable" is proven by tests (fixture-driven), not by a
  formal proof: the fixed text is always present regardless of editable content, and door
  enforcement backs it independently of prompt text.
- The existing Metatron console/turn flow, charge economy, and mediation doctrine are
  unchanged; this feature only restructures where instructions come from and which tools
  are offered.

## Dependencies

- Substrate for TASK-68 (curriculum ladder): stage presets will be expressed as manifest
  contents. Nothing here depends on TASK-68.
- Runs alongside TASK-63 (decision-trace view), which touches the TUI Metatron pane's
  transcript rendering; this feature touches the pane's status rendering. Coordination is
  a merge concern, not a spec dependency.
