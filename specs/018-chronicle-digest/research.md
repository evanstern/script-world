# Research: Chronicle Digest Grammar & Selection Detail

**Feature**: specs/018-chronicle-digest | **Date**: 2026-07-22

All Technical Context unknowns resolved. Decisions below are grounded in the current code (`internal/tui/grammar.go`, `views.go`, `tui.go` read at planning time) and the event catalog (`docs/wiki/event-types.md` + reducer literals in `internal/sim/*.go`).

## R1 — Digest architecture: registry of pure per-type functions

**Decision**: a package-level `digestRegistry map[string]digestFunc` in a new `internal/tui/digest.go`, where `digestFunc(e store.Event, names []string) ([]seg, bool)` returns the summary as ordered segments (see R4) or `ok=false` on unmarshal failure. `formatChronicleLine` consults the registry first; a miss or `!ok` falls back to today's `resolvePayloadNames` compact JSON (FR-002). The existing `classifyEvent` speech/scene privileges are folded into registry entries so there is one dispatch mechanism.

**Rationale**: matches the package's established pattern — pure functions over `store.Event` + names, table-driven-testable without a Bubble Tea program (the header comment of `grammar.go` states this as a design rule). A map registry makes coverage mechanically checkable (R3) and gives future event types one obvious place to land.

**Alternatives considered**: (a) one giant switch inside `formatChronicleLine` — same behavior but coverage can't be introspected for the sweep test without parsing source; (b) reflection over payload structs to auto-render fields — breaks the hybrid-voice requirement (FR-006) and produces labeled dumps, which is the problem being fixed.

## R2 — Family classification by namespace prefix

**Decision**: family = the event type's namespace (text before the first `.`), giving 12 families: `world`, `clock`, `sim`, `agent`, `social`, `meeting`, `norm`, `gru`, `chronicle`, `metatron`, `daemon`, `cog`. `meeting` and `norm` share one visual role (governance). Voice assignment (FR-006): labeled fields for `cog`, `clock`, `daemon`; natural phrase for the rest; `agent.needs_changed` renders phrase-prefixed labeled needs (it is a numeric heartbeat).

**Rationale**: the namespace is already the emitter-side family convention (event-types.md "Conventions" section); deriving it needs no new metadata and automatically classifies future types.

**Alternatives considered**: hand-maintained family map per type — more control, but one more table to drift; prefix derivation plus a small override map (governance merge) gets the same result.

## R3 — Coverage sweep test: explicit catalog fixture, cross-checked against the wiki doc

**Decision**: `digest_test.go` carries an explicit `catalogTypes` fixture — every type with a representative sample payload (the ~70 enumerated in contracts/digest-grammar.md). The sweep asserts (1) every fixture type has a registry entry and digests without fallback, and (2) every backticked `x.y` type name parsed from `docs/wiki/event-types.md`'s table appears in the fixture — so the doc and the fixture cannot drift silently. Family rows in the doc (`meeting.*`/`norm.*`) are covered by the fixture's explicit enumeration from reducer literals.

**Rationale**: SC-001 demands a mechanical gate. Sample payloads are needed anyway for digest unit tests; making the fixture the catalog kills two birds. Parsing the wiki table keeps Principle IV honest without making the test depend on prose.

**Alternatives considered**: parsing the reducer's switch in `internal/sim/state.go` via go/ast — robust but heavyweight and still misses types emitted outside the switch; runtime registration from `internal/sim` — invasive to the sim package for a view-layer feature.

## R4 — Styled segments keep the pure layer ANSI-free

**Decision**: digest output is `[]seg{Text string; Role segRole}` with roles `segText`, `segName`, `segSpeech`, `segEmphasis` (amounts/causes/kinds), `segLabel` (telemetry `key=`). The pure layer never touches lipgloss; `renderChronicleRow` maps roles → style tokens. Plain-text assembly (`plainSeg s`) feeds the existing `wrapOrTruncatePlain` width logic, and tests assert on plain text + role spans.

**Rationale**: preserves the load-bearing property that grammar tests run without ANSI concerns (`plainChronicleLine`'s comment cites exactly this), while giving the view layer enough structure for FR-005 token emphasis. Styling wrapped lines segment-wise avoids the classic ANSI-mid-truncation bug.

**Alternatives considered**: digest returns pre-styled strings — breaks purity and makes width math wrong (ANSI codes count as runes); digest returns plain string only — loses token emphasis (FR-005 fails).

## R5 — Column layout: fixed-ish widths computed per visible window

**Decision**: solo width renders `TICK  HH:MM  type  summary` with the tick column right-aligned and padded to the widest tick in the visible window, time fixed at 5, and the type column left-padded to the longest type among visible rows, capped at 26 runes (`social.conversation_turn` = 24; longer future names truncate with `…`). Dock width (FR-004 clarification) drops the tick column and shows the type's last segment (`conv_turn`, `foraged`) padded to the window max, capped at 10; summary wraps per existing dock rules (≤3 lines). The `#seq` prefix leaves the feed line (it remains in the detail pane).

**Rationale**: per-window padding gives strict vertical alignment (SC-002/US3-AS1) without a global fixed budget that either wastes width or truncates everything; caps bound pathological names.

**Alternatives considered**: global fixed columns — simple but wastes ~8 columns at tick<10⁴ and misaligns past cap; no type column cap — one long type name would push every summary off-screen.

## R6 — Detail pane: bottom split with reserved rows and its own scroll

**Decision**: in inspect mode the panel body splits: the entry list keeps `max(5, rows−paneRows)` rows and the detail pane takes `paneRows = min(rows/2, 14)` at the bottom, separated by a rule line carrying `DETAIL · seq … · [future: actions]`. The pane renders `formatInspector` output (unchanged contract: verbatim payload, `// name` annotations); content taller than the pane scrolls with `J`/`K` (shift-j/k), and a `… (+N more lines — J to scroll)` footer marks truncation. Oversized payloads (FR-011, `world.migrated`) render lazily: only the visible slice of the pretty-printed payload is styled per frame, and scrolling reaches all of it. Selection change resets pane scroll to top.

**Rationale**: a persistent pane (clarified FR-008) with a stable list rhythm — the current inline expansion's reflow was the UX complaint the clarification resolved. Reserving pane rows out of the same `rows` budget follows the existing B1/B2 budget discipline documented in `chronicleInspectBody`. `J`/`K` mirrors j/k one layer up and collides with nothing in `handleInspectKey` or the global map.

**Alternatives considered**: side-by-side split — dock/narrow widths can't afford it, bottom split works at every width; viewport component from bubbles — new dependency for a 20-line windowing function the package already knows how to write.

## R7 — ⏎ freed; expansion state replaced by pane state

**Decision**: `chronExpanded`/`chronExpIdx` state and `chronToggleExpand` are removed; ⏎ in inspect mode becomes a no-op reserved for the future jump-off actions bar (the contract documents this as the extension point, FR-009). New state: `chronDetailScroll int`, reset on selection move and on pause exit.

**Rationale**: the pane is always-on (FR-008), so toggle state is dead weight; keeping ⏎ unbound-but-reserved is the cheapest honest extension point — documented in keymap.md and the contract rather than left to archaeology.

**Alternatives considered**: keep ⏎ as pane show/hide — contradicts "no additional keypress" (FR-008) and adds a mode users must discover.

## R8 — Performance: format visible window only; no memoization yet

**Decision**: move the existing "format everything then window" in `chronicleRawBody` to "window first, then format" (the event ring is capped at 256, but digesting ~250 events per frame at high push rates is avoidable waste); no per-seq memoization in this feature. Measure only if SC-005 shows lag.

**Rationale**: per-frame work becomes O(visible rows) ≈ 40 digests, each one small JSON unmarshal — comfortably under frame budget; memoization adds cache-invalidation surface (names change on reconnect) for no demonstrated need.

**Alternatives considered**: memoize digest by (seq, names-generation) — ready escape hatch, documented here so the implementer reaches for it instead of inventing one if profiling demands.
