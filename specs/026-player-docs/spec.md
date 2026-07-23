# Feature Specification: Player Docs — HTML user documentation generated from the wiki + docs-freshness skill

**Feature Branch**: `026-player-docs`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Player docs: HTML user documentation generated from the wiki, plus a docs-freshness project skill. A docs/player/ folder of self-contained, theme-aware HTML pages written for PLAYERS (non-engineers) of promptworld, generated from docs/wiki/ + README.md + docs/llm-providers.md as source of truth, with per-page provenance (source notes + pinned commit), and a project skill that regenerates stale pages and offers a --check mode. Board task TASK-82."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A new player gets from zero to a running world (Priority: P1)

A person who has never seen the codebase (and doesn't read Go) opens the player docs
index in a browser, follows the getting-started page, and ends up with a world created,
started, and attached — watching villagers live in their terminal. Along the way they
learn the handful of commands that matter (install/build → `new` → `start` → `attach` /
`ui`) in plain language, with no code internals or engineering vocabulary required.

**Why this priority**: onboarding is the entire point of player docs; if only one page
exists, it must be this one. Every other page assumes a running world.

**Independent Test**: open `docs/player/index.html` from disk in a browser, follow the
getting-started page top to bottom, and verify each described step matches what the
README and wiki document for those commands at the pinned commit.

**Acceptance Scenarios**:

1. **Given** a fresh checkout of the repo, **When** a reader opens the player docs index
   in a browser (double-click, no server), **Then** the index renders correctly, links to
   every player page, and each linked page opens and renders standalone.
2. **Given** the getting-started page, **When** a non-technical reader follows it,
   **Then** every command shown (install, new, start, attach/ui) matches the grounded
   corpus, and no step requires knowledge the page hasn't introduced.
3. **Given** any player page, **When** viewed in a browser set to dark mode and again in
   light mode, **Then** the page is legible in both without user action.

---

### User Story 2 - A player learns to actually play (Metatron, time, the story) (Priority: P2)

With a world running, the player reads the remaining pages to learn the game itself: how
all influence flows through Metatron and the player-editable charter; how game time,
speed, and pause work; how to read the chronicle/story to catch up on what happened; and
a plain-language explanation of "the AI behind the village" (what the villagers' minds
are, local vs cloud models) plus just enough `llm.json` orientation for a non-engineer
to get inference working — while operator-depth detail stays in the operator guide.

**Why this priority**: this is the substance of the docs, but it presupposes the P1
onboarding path exists.

**Independent Test**: read each page against its declared wiki source notes at the
pinned commit and confirm every player-facing claim is a restatement of the grounded
corpus, with no invented facts and no contradictions.

**Acceptance Scenarios**:

1. **Given** the full page set, **When** enumerated, **Then** it covers at minimum:
   getting started, playing via Metatron + charter, time/speed/pause, reading the
   chronicle/story, "the AI behind the village", and llm.json basics for non-engineers,
   plus a navigation index.
2. **Given** any player page, **When** its claims are spot-checked against the wiki
   notes it declares as sources (at the commit it declares), **Then** no claim
   contradicts the corpus, and the audit is recorded on the board task.
3. **Given** the llm.json basics page, **When** compared to the operator guide
   (docs/llm-providers.md), **Then** it stays at "get it working" depth and defers
   operator-level detail (registry reference, migration) to that guide by link/mention
   rather than duplicating it.

---

### User Story 3 - The operator keeps the docs provably fresh (Priority: P3)

After the wiki is re-pinned (post-merge `wiki-update`), the operator runs the
docs-freshness skill. In check mode it reports — without writing anything — exactly
which player pages are stale (a source note re-pinned past the page's recorded pin) and
which are current. In generate mode it refreshes only the stale pages, re-records their
provenance, and leaves fresh pages untouched; running it again immediately is a no-op.

**Why this priority**: freshness is what makes the docs trustworthy over time, but it
only matters once the pages (P1/P2) exist.

**Independent Test**: with all pages current, run check mode (expect "all fresh", no
writes) and generate mode (expect no changes). Re-pin one source note's recorded commit,
run check mode (expect exactly the pages sourcing that note reported stale, still no
writes), regenerate, then run check mode again (expect all fresh).

**Acceptance Scenarios**:

1. **Given** every page current with its sources, **When** check mode runs, **Then** it
   reports all pages fresh and modifies no files.
2. **Given** a source note re-pinned past a page's recorded pin, **When** check mode
   runs, **Then** that page (and only pages sourcing changed notes) is reported stale,
   and no files are modified.
3. **Given** stale pages reported, **When** the skill regenerates, **Then** only the
   stale pages change, their recorded provenance advances to the notes' current pins,
   and an immediate second run changes nothing (byte-identical no-op).
4. **Given** a repo where the skill has never run, **When** someone asks "are the player
   docs current?", **Then** the answer is derivable from artifacts (page provenance vs
   note pins) rather than from memory or assertion.

---

### Edge Cases

- A page declares a source note that no longer exists in `docs/wiki/` (renamed/deleted):
  check mode must surface this as a staleness/error condition, not silently pass.
- A brand-new wiki note appears that a page *should* cover: out of scope for the
  automated check (it compares declared sources only); coverage changes are editorial
  decisions made when regenerating.
- `docs/player/` doesn't exist yet or a page is missing: check mode reports missing
  pages rather than erroring out.
- README.md and docs/llm-providers.md are sources but are not pinned wiki notes: pages
  sourcing them record the commit they were read at, and staleness for them is judged by
  whether the file changed since that commit.
- The reader's browser blocks network access or they're offline: pages must still render
  fully (no external assets).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The project MUST contain a player documentation folder (`docs/player/`) of
  HTML pages written in plain language for players (non-engineers), distinct from the
  developer wiki (`docs/wiki/`) and from the codebase-to-course output (`docs/course/`).
- **FR-002**: The page set MUST cover at minimum: getting started (install → new world →
  start → attach/TUI), playing via Metatron and the charter, time/speed/pause, reading
  the chronicle/story, a plain-language "the AI behind the village" page, and llm.json
  basics for non-engineers.
- **FR-003**: A navigation index page MUST link every player page; every page MUST link
  back to the index.
- **FR-004**: Every page MUST be self-contained and offline-viewable: no external
  scripts, stylesheets, fonts, or images; legible in both light and dark color schemes
  without user action.
- **FR-005**: Every factual claim in the player docs MUST be a plain-language projection
  of the grounded corpus (docs/wiki/ notes, README.md, docs/llm-providers.md) — no
  independently asserted facts. Operator-depth content stays in docs/llm-providers.md
  and is referenced, not duplicated.
- **FR-006**: Every page MUST machine-readably record its provenance: the source
  documents it was rendered from and the commit(s) those sources were pinned to at
  render time.
- **FR-007**: A project skill MUST exist that regenerates the player docs from the
  corpus, refreshing only pages whose sources have moved past the page's recorded
  provenance; regenerating when nothing is stale MUST change nothing (idempotent no-op).
- **FR-008**: The skill MUST offer a check mode that reports each page's freshness
  (fresh / stale / missing / broken source reference) without writing any files, and the
  staleness determination MUST be executable by a script (machine-checkable, not
  judgment-based).
- **FR-009**: The skill MUST be planted in the project (`.claude/skills/`) and
  documented — its name and when to run it (as the recommended follow-on after a wiki
  re-pin) — in CLAUDE.md or the skill's own description. It MUST run standalone, not be
  invoked by other plugins.
- **FR-010**: A spot-audit confirming the player docs contain no facts contradicting the
  wiki at their pinned commits MUST be recorded on board task TASK-82.

### Key Entities

- **Player page**: one self-contained HTML document in `docs/player/` covering one
  player-facing topic; carries provenance (source list + pinned commit(s)) and links to
  the index.
- **Provenance record**: the machine-readable per-page statement of which sources
  (wiki notes / README / operator guide) the page was rendered from and at which commit
  each was pinned; the unit the freshness check compares.
- **Source note**: a docs/wiki/ note with its own `verified_against` pin (or a
  non-wiki source file, pinned by the commit it was read at); movement of a source's pin
  past a page's recorded pin is what makes the page stale.
- **Docs-freshness skill**: the project skill that generates/regenerates pages and
  drives the scriptable check; the mechanism that makes "docs are current" provable.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A non-technical reader can go from the docs index to a created, running,
  attached world following only the getting-started page — every command it shows works
  as written at the pinned commit.
- **SC-002**: 100% of player pages open and render fully from local disk with no network
  access, in both light and dark modes.
- **SC-003**: 100% of pages carry machine-readable provenance, and the check mode's
  fresh/stale verdict for any page is reproducible by re-running it (same inputs → same
  report, zero files written).
- **SC-004**: After a source note is re-pinned, the check flags exactly the pages that
  declare that note as a source — no false negatives, no unrelated pages flagged — and
  regeneration followed by an immediate re-run is a byte-identical no-op.
- **SC-005**: A spot-audit of player-facing claims against source notes at their pinned
  commits finds zero contradictions (audit recorded on TASK-82).

## Assumptions

- The wiki corpus (`docs/wiki/`, 36 notes pinned via `verified_against`) is current as
  of the commit this feature starts from; player docs render from it as-is and inherit
  its freshness model rather than re-verifying code directly.
- Page prose is authored by the skill's guided generation (model-written plain-language
  projection), not by a deterministic text transformer; determinism is required only of
  the freshness *check* and of the no-op property (regenerating a fresh page must not
  rewrite it).
- "Theme-aware" means responding to the reader's OS/browser color-scheme preference; no
  in-page theme toggle is required.
- The player docs ship in the repo (viewable from a checkout); publishing/hosting them
  elsewhere is out of scope.
- Screenshots/recordings of the TUI are out of scope for this iteration; pages may use
  text/ASCII depictions instead.
- docs/llm-providers.md remains the operator-level guide; the player llm.json page only
  covers what a player needs to get inference running.
