# Feature Specification: Chronicle Digest Grammar & Selection Detail

**Feature Branch**: `018-chronicle-digest`

**Created**: 2026-07-22

**Status**: Draft

**Board task**: TASK-60

**Input**: User description: "Chronicle digest grammar: readable per-event summaries, TICK column, selection detail pane. The TUI chronicle raw feed is hard to parse: only speech/scene/clock/narration event classes get readable treatment — every other event type (~65 of ~70 in the event catalog) falls through to a compact raw-JSON payload dump, a wall of noise at speed. Every event should render as 'TICK | timestamp | event.name | structured readable summary'; use highlights/underlines/color to keep the feed parseable as it flies by; on pause, navigate the chronicle item by item; the highlighted entry shows a detailed view with as much info as was logged, eventually a jumping-off point to other menus or controls."

## Clarifications

### Session 2026-07-22

- Q: Where should the selected entry's full detail render in paused inspect mode? → A: **Dedicated detail pane** — a persistent pane below the entry list showing the selected entry's full detail; the list keeps its rhythm, and the pane is the future home of jump-off actions.
- Q: What voice should digest summaries use? → A: **Hybrid by family** — natural phrases for narrative families (agent, social, gru, sim, world, meeting/norm, metatron, chronicle); compact labeled fields for telemetry families (cog.\*, clock, daemon) where numeric comparison matters more than prose.
- Q: How does the columnar layout degrade at narrow dock width? → A: **Drop the tick column in the dock** — dock shows time + short event name + summary (wrapping as today); the tick column appears at solo width and always in the detail pane.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reading the live feed without decoding JSON (Priority: P1)

An operator watches the chronicle while a world runs. Today most events scroll past as `type {"agent":2,"x":14,"y":9}` — a compact JSON dump that must be mentally decoded, and at speed it is a wall of noise. After this feature, every event renders as one digest line — tick, clock time, event name, and a readable summary of what happened built from the fields that matter for that event type, with agent names resolved (`Ash foraged at (14,9)` rather than `{"agent":2,...}`). The reader follows the life of the village from the feed alone.

**Why this priority**: This is the core complaint and the core value — the feed is the primary observability surface for a running world, and today it fails at its one job for ~65 of ~70 event types. Everything else in this feature builds on the digest line.

**Independent Test**: Run a world (or replay a saved event log) and observe the raw chronicle feed. Every event type that appears renders a digest summary, not a JSON payload dump. A per-type sweep over the full event catalog can verify coverage without a live world.

**Acceptance Scenarios**:

1. **Given** a running world emitting events across families (agent, social, sim, clock, cog, gru, meeting/norm, metatron), **When** the operator watches the raw chronicle feed, **Then** every entry shows tick, clock time, event name, and a readable per-type summary — no cataloged event type renders as a raw JSON payload dump.
2. **Given** an event whose payload references agents by index (speaker/listener, from/to, creditor/debtor, witnesses), **When** its digest line renders, **Then** every agent reference appears as the agent's name.
3. **Given** an event of a type unknown to the digest grammar (a future type added after this feature ships), **When** it renders, **Then** the feed falls back to the current compact resolved-name JSON treatment — never a blank or an error.
4. **Given** the feed at high time compression, **When** many events arrive per second, **Then** rendering keeps up (no perceptible feed lag attributable to formatting).

---

### User Story 2 - Inspecting an entry in full on pause (Priority: P2)

The operator pauses the clock to read closely. The chronicle enters inspect mode (as today): up/down moves a highlighted selection item by item, first/last jumps work, and the selection survives switching tabs. New in this feature: the highlighted entry's **full detail** — everything that was logged: seq, tick, type, the verbatim stored payload pretty-printed, with agent indices annotated with resolved names — is shown automatically for the selected entry, without pressing an extra key. The detail surface is structured so that future work can attach jump-off actions (e.g. open the referenced agent in the villagers tab), though no actions ship now.

**Why this priority**: The digest line (US1) deliberately elides fields; the detail view is the other half of the contract — the evidence behind the summary. Navigation already exists; the delta is detail-on-selection.

**Independent Test**: Pause a world with events in the log; move the selection with the existing keys and confirm the selected entry's full logged detail is visible at each step with no additional keypress.

**Acceptance Scenarios**:

1. **Given** a paused clock with the chronicle visible, **When** the operator moves the selection to any entry, **Then** that entry's full detail (seq, tick, type, complete verbatim payload, resolved-name annotations) is displayed automatically.
2. **Given** an entry whose payload is very large (e.g. a migration event embedding an entire world state), **When** it is selected, **Then** the detail view renders a bounded, navigable representation without freezing or destroying the layout.
3. **Given** the operator resumes the clock, **When** running mode returns, **Then** the feed snaps back to tail-follow exactly as today.
4. **Given** the detail view, **Then** its layout reserves an identified, documented place where future jump-off actions will live (no actions functional in this release).

---

### User Story 3 - Scanning the feed by eye at speed (Priority: P3)

The operator doesn't read every line — they scan. Columns (tick, time, event name, summary) align vertically so the eye can run down a single column; each event family has a consistent visual identity (color and emphasis), and the key tokens inside a summary (names, quoted speech, amounts, causes) carry emphasis (bold/underline/reverse) so high-salience events (deaths, attacks, conversations) pop out of the scroll without being read.

**Why this priority**: Valuable but layered on top of US1 — a readable summary with today's styling already fixes the core complaint; alignment and family styling multiply its scanability.

**Independent Test**: Render a mixed window of ≥50 events; verify columns align, each family renders with its documented visual role, and high-salience events are locatable by scan alone.

**Acceptance Scenarios**:

1. **Given** a feed window of mixed events, **When** rendered at solo width, **Then** the tick, time, and event-name columns align vertically across entries.
2. **Given** events from different families on screen together, **When** the operator scans without reading, **Then** family membership is distinguishable by the line's visual treatment alone, per a documented color-role contract.
3. **Given** a digest summary containing an agent name, quoted speech, or a death/attack cause, **When** it renders, **Then** those tokens carry visual emphasis distinct from the surrounding summary text.

---

### Edge Cases

- **Unknown event type** (added to the system after this feature): falls back to compact resolved-name JSON — the current default treatment — never blank/error (FR-002).
- **Oversized payload**: `world.migrated` embeds the entire world state; the detail view must bound what it renders and stay navigable (FR-011).
- **Agent references that cannot resolve**: index out of range of the replica roster, or events predating an agent list — the digest shows the raw index (e.g. `#2`) rather than failing.
- **Dead agents**: names still resolve (roster retains dead agents); death events render with the name.
- **Long free-text fields** (utterances, thoughts, memory text, narration): truncated with `…` in the digest line per existing width rules; full text always in the detail view.
- **Narrow widths** (dock tab): the tick column is dropped and the summary wraps per existing dock rules (see FR-004); the digest summary itself still applies.
- **Zero-agent events** (`sim.*`, `clock.*`, `daemon.*`): summaries read naturally without a subject slot.
- **`chronicle.entry` in the raw feed**: narration events also get a digest treatment (day/thread/prose gist) rather than a JSON dump; the narrated view (`r` toggle) is unchanged.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every event type in the event catalog (docs/wiki/event-types.md as of implementation) MUST render in the raw chronicle feed as a digest line containing: tick, clock time, event name, and a type-specific readable summary extracting that type's salient fields. No cataloged type may render its payload as an undigested JSON dump in the feed.
- **FR-002**: Event types not covered by the digest grammar (unknown/future types) MUST fall back to the current compact resolved-name JSON treatment.
- **FR-003**: Every agent reference in a digest summary MUST display as the agent's resolved name; unresolvable indices display as the raw index without error.
- **FR-004**: The digest line MUST include a tick column, and at solo width the tick / time / event-name columns MUST align vertically across entries. At narrow dock width the tick column is dropped: the dock renders time + short event name + summary, wrapping per existing dock rules; the tick remains available at solo width and always in the detail pane.
- **FR-005**: Each event family MUST have a documented visual role (color/emphasis), extending the existing color-role contract, and key tokens within summaries (agent names, quoted speech, amounts, causes) MUST carry emphasis distinct from surrounding text. High-salience events (deaths, attacks) MUST be visually prominent.
- **FR-006**: The summary portion of the digest line MUST be human-readable without JSON decoding, in a hybrid voice by family: natural phrases for narrative families (`Ash foraged at (14,9)`, `Birch died: exposure`), compact labeled fields for telemetry families — `cog.*`, `clock.*`, `daemon.*` — (`job=41 landed agent=Rowan stale=12t wall=830ms`). The grammar documents which voice each family uses.
- **FR-007**: Existing inspect-mode navigation MUST be preserved: item-by-item up/down selection, first/last jumps, selection remembered across tab switches while paused, automatic entry on pause and exit on resume.
- **FR-008**: When an entry is selected in inspect mode, its full detail MUST display automatically in a dedicated detail pane below the entry list — seq, tick, type, and the verbatim stored payload pretty-printed with resolved-name annotations (payload bytes never rewritten) — with no additional keypress. The list keeps a stable rhythm (no inline expansion reflow); the pane replaces the current ⏎-triggered inline inspector.
- **FR-009**: The detail surface MUST include a documented extension point where future jump-off actions (to related views/controls) will attach; no actions ship in this feature.
- **FR-010**: Existing feed behaviors MUST be preserved: tail auto-follow while running, `r` raw/narrated toggle, `a`/`t` agent/thread filters, and width-overflow rules (solo truncates to one line; dock wraps then truncates).
- **FR-011**: The detail view MUST render oversized payloads in a bounded, navigable way (no freeze, no layout destruction), with the full content reachable.
- **FR-012**: The two governing design documents (the chronicle grammar pattern and the chronicle panel doc) MUST be updated to describe the shipped grammar, visual roles, and inspect behavior; the digest grammar's per-type coverage MUST be verifiable by an automated sweep over all cataloged event types.

### Key Entities

- **Digest line**: the one-line feed rendering of a stored event — tick, clock time, event name, readable summary. A *view* of the event: fields elided, names resolved.
- **Digest grammar**: the per-event-type mapping from payload fields to summary — which fields are salient for each type, and how they read. Organized by event family; the extension point for future types.
- **Event family**: a namespace grouping of event types (agent, social, sim, clock, cog, gru, meeting/norm, metatron, daemon, world, chronicle) — the unit of visual identity.
- **Detail view**: the full-fidelity rendering of the selected entry — *evidence*, not view: verbatim stored bytes, pretty-printed, annotated with names, never rewritten. Carries the future jump-off extension point.
- **Color role**: a named visual treatment (existing contract: dim/type/name/speech/clock/selection) extended with family and emphasis roles.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of cataloged event types render a digest summary in the feed — verified by an automated sweep that fails if any cataloged type falls through to the JSON dump.
- **SC-002**: For any digest line at solo width, a reader can answer "who did what (and where/to whom)" from that line alone, without opening the detail view, for every event family.
- **SC-003**: On pause, the full logged detail of any entry is reachable by navigation alone — zero additional keypresses beyond moving the selection.
- **SC-004**: In a mixed window of ≥50 events, high-salience events (deaths, attacks, conversations) are locatable by visual scan without reading line text.
- **SC-005**: The feed sustains maximum time-compression event rates with no perceptible rendering lag attributable to digest formatting.

## Assumptions

- The digest line replaces the current `#seq` prefix with the tick column per the requested format; `seq` remains visible in the detail view. (The seq is bookkeeping; the tick is the world-meaningful coordinate.)
- Readable summaries supersede the current grammar doc's "everything stays JSON-shaped, never prose" line-format principle for the feed line; the principle's other half — the detail view shows verbatim evidence, payload bytes never rewritten — is preserved unchanged. The grammar doc is updated accordingly (FR-012).
- This feature is view-layer only: no change to stored event formats, emission, or the reducer. TASK-17 (event payloads carry agent names at the format level) remains separate and complementary; the digest grammar resolves names post-hoc via the existing replica mechanism until TASK-17 lands.
- The narrated chronicle view (`r` toggle) and its narrator pipeline are out of scope and unchanged; this feature governs the raw feed and inspect mode.
- Jump-off actions from the detail view (opening agents in the villagers tab, filtering by thread, etc.) are explicitly out of scope; only the documented extension point ships.
- The existing keybinding surface (j/k/g/G, r/a/t) is retained; the always-on detail pane relieves ⏎ of its expand/collapse role (⏎ becomes available for future jump-off actions), and no new keys are required.
