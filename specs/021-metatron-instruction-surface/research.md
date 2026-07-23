# Phase 0 Research: Metatron Instruction Surface

All Technical Context items were resolvable from the existing codebase and prior specs;
no external research was required. Each decision below cites the artifact it derives from.

## R1 — Where the new files live

**Decision**: `skills/` directory and `capabilities.json` sit at the world-save-dir root,
beside `charter.md` (`<worldDir>/skills/*.md`, `<worldDir>/capabilities.json`).

**Rationale**: `charter.md` already lives at the world dir root (`charter.go:
filepath.Join(worldDir, "charter.md")`) and is the player's known editing surface; the
angel's PRIVATE files (soul, transcript) live under `<worldDir>/metatron/`
(`metatron.go metatronDir()`). The split is meaningful: root = player-authored
configuration, `metatron/` = angel-authored state. New player-editable files belong with
the charter.

**Alternatives considered**: under `metatron/` — rejected: mixes player-authored config
into the angel's own notebook space and hides the files players must find.

## R2 — Skill file composition: order, caps, notices

**Decision**: compose `skills/*.md` (regular files, `.md` extension only, no recursion)
in ascending lexicographic filename order (bytewise, `sort.Strings`). Caps: 4,000 chars
per file (reuse `persona.CharterMaxChars`), 8 files max. Over-cap file → truncated at the
cap with a notice; files beyond the 8th in sort order → skipped with a notice; unreadable
file → skipped with a notice. Notices join the existing charter-notice channel (prefixed
into the next reply, `turn.go` notice handling) — one combined notice line per issue.

**Rationale**: mirrors the charter's exact fallback discipline (`loadCharter`: missing →
restore, empty → fallback, oversize → truncate, each with a notice) so the player learns
ONE model of "the game tells you when your file didn't fully load". Lexicographic order
is the SKILL.md-world convention players can control (prefix `10-`, `20-`) and is
deterministic (FR-012, determinism doctrine).

**Alternatives considered**: modification-time order — rejected: nondeterministic across
copies (breaks identical-dir ⇒ identical-prompt; world copies are a proven e2e scenario).
Manifest-declared order — rejected: couples two files for v1 no player asked for.

## R3 — Prompt assembly order; fixed frame provably last

**Decision**: `turnSystemPrompt` becomes charter → skill files (in order, each under a
`--- skill: <name> ---` separator) → fixed frame LAST, always appended unconditionally
from a Go string constant. The fixed frame retains the two non-negotiables verbatim and
gains nothing editable. "Provably not overridable" = (a) the frame is a compile-time
constant appended after all editable content on every path — no editable byte can
displace or truncate it (truncation happens per-file BEFORE assembly); (b) an adversarial
fixture test asserts the assembled prompt ends with the frame for a battery of hostile
skill/charter contents; (c) the door checks (R5) enforce the same limits model-side
prompt text cannot.

**Rationale**: today's layout already puts the fixed frame after the charter ("fixed
frame, beneath the charter", `turn.go:390`) — last-word position is the established
authority order; skills slot in between. The task description's "beneath the fixed frame"
reads as authority-subordination, which position-last preserves and strengthens.

**Alternatives considered**: skills after the frame — rejected: gives editable text the
last word, weakening the strongest position; contradicts current charter precedent.

## R4 — Capability manifest format and fallback semantics

**Decision**: `capabilities.json`, shape:

```json
{"tools": ["nudge_dream", "nudge_omen", "work_miracle"],
 "miracle_kinds": ["move", "remove", "give_item", "time_snap"]}
```

Missing file → full default grant (today's `loopMetatronTools` + all four kinds) —
existing worlds byte-compatible. Malformed JSON / wrong types → full default grant + a
notice (mirrors charter fallback-to-default). Unknown tool or kind names → ignored with a
notice; the valid remainder applies. `miracle_kinds` omitted while `work_miracle` granted
→ all kinds. `tools: []` → conversation-only world (valid; converse is not gateable —
it is the final-answer text channel, not a roster tool, `roster.go loopMetatronTools`
doc). Read fresh at each turn and each status call (per-read discipline, FR-001).

**Rationale**: JSON parses safely with clear failure detection (a prose file cannot
distinguish "malformed" from "weird but intended"); the permissive fallback mirrors
`loadCharter` exactly, satisfying spec FR-007's "mirroring charter fallback semantics";
name-based grants slot new future tools in without redesign (spec assumption).

**Alternatives considered**: YAML/TOML — rejected: new dependency for zero gain.
Deny-by-default on malformed — rejected: inconsistent with the charter's established
permissive-fallback teaching model, and a typo bricking the angel mid-lesson is hostile
to learners; the notice makes the fallback legible.

## R5 — Gating enforcement: three independent layers

**Decision**:
1. **Declaration**: `Turn()` builds the granted roster per-read — filter
   `tool.LoopRosterMetatron()` by manifest `tools`; for `work_miracle`, restrict the
   `kind` Param's Enum to granted `miracle_kinds` via a new pure helper
   `tool.RestrictEnum(t, "kind", kinds)` (copy-on-write; registry untouched). Ungranted
   tools never enter `Job.Roster`, so they are never declared to the model
   (schema-level structural absence — `derive.go InputSchema` walks the restricted copy).
2. **Prose**: the derived guidance text (R6) is generated FROM the same granted roster,
   so ungranted tools/kinds are absent from instructions too.
3. **Door**: handlers (`turnHandlers`/`toolcalls.go`) are built only for granted tools —
   an ungranted call is `rejected_unknown` by the existing loop machinery; additionally
   `landNudge`/`landMiracle` check the granted set (not just static `RosterMetatron`),
   and `sim.spendMiracleCharge`/reducer dry-run remains the final authority. A granted
   check failing at the door refuses in-fiction exactly like today's refusals.

**Rationale**: matches the registry doctrine already in force for gratis ("structural
absence — not sanitizing a field out — is the guarantee", `registry.go work_miracle`
comment) and the roster doctrine ("capability as membership", `roster.go`). Layer 3 keeps
the guarantee even if a prompt-injected model hallucinates a call.

**Alternatives considered**: prose-only forbidding — explicitly ruled out by spec FR-005.
Registry-level world-parameterization — rejected: the registry is process-global and
world-agnostic; per-world state belongs in the per-turn read.

## R6 — Registry-derived tool guidance replaces the prose list

**Decision**: new `tool.MetatronToolGuidance(roster []Tool) string` in `derive.go`:
walks the granted roster and renders, per tool, its name, argument surface (from Params,
mirroring `InputSchema`'s source of truth), and charge cost (from R7's cost table for
miracle kinds; `Cost.Charges` for nudges). `turnSystemPrompt` calls it instead of
carrying the hand-written block (`turn.go:413-425`). The surrounding fixed-frame
doctrine prose (judge-first, one act per turn, refusal is free, no free miracles, no
villager removal) stays hand-written constant — it is doctrine, not tool description.
A drift test pins: every roster tool name appears in the guidance; every cost number in
the guidance equals the authoritative table; no non-roster tool name appears.

**Rationale**: same pattern as `VocabularyLine`/`PromptGlossBlock` (derive.go), which
exists precisely "to kill" hand-maintained-vocabulary drift; extends it to the metatron
surface. Deriving from the SAME roster that feeds `Job.Roster` makes described ≡ declared
by construction (spec FR-008).

**Alternatives considered**: keeping prose with a pinning test only — rejected: the test
catches drift but the duplicate edit remains; spec demands derivation. Rendering raw
JSON schemas into the prompt — rejected: the model already receives schemas natively via
tool declarations; the prompt needs the human-shaped guidance layer, not a second schema
dump.

## R7 — One source of truth for miracle costs

**Decision**: authoritative per-kind cost table moves INTO the leaf `internal/tool`:
`tool.MiracleCost(kind string) int` + `tool.MiracleCostsByEvent() map[string]int`
(kind ↔ event-type mapping declared beside `miracleKinds` in registry.go).
`sim.miracleCost` is replaced by derivation from `tool.MiracleCostsByEvent()` (the
import direction already exists: `internal/sim/toolcheck.go`, `journal.go`, `agents.go`,
`loop.go`, `metatron.go` import `internal/tool`; `internal/tool` imports no internal
package, staying a leaf). `MetatronToolGuidance` (R6) renders costs from the same table.
The existing mirror test (`TestMiracleKindsMirrorTool`) flips from "pin two copies equal"
to "assert derivation" (one copy remains, in tool).

**Rationale**: three copies exist today (prose `turn.go:406-411`, `Cost.Charges` gate
minimum, `sim.miracleCost` enforcement) — spec FR-009/SC-004 demands one edit propagating
everywhere. The leaf package is the only home all consumers can reach without cycles.

**Alternatives considered**: source of truth in sim with tool mirroring — rejected: tool
cannot import sim (leaf), so the mirror+pin-test pattern would persist, which is exactly
the drift surface being removed. `Cost.Charges` per-kind as separate registry tools (one
tool per miracle kind) — rejected: changes the model-facing tool surface for an internal
bookkeeping win; the kind enum is the established contract (spec 016/017).

## R8 — Status extension and TUI surface

**Decision**: extend `metatron.Status` (model-free peek, `turn.go:339-350`) with
`Skills []string` (effective skill filenames in composition order),
`GrantedTools []string` (granted roster names, with granted miracle kinds rendered as
`work_miracle(move,give_item)` when restricted), and `ManifestDefault bool` (no manifest
file present). Existing fields unchanged; IPC rides the same JSON marshal
(`ipc/client.go MetatronStatus`, `server.go` — no protocol change). TUI: extend the
`consoleStatusMsg` handler + console header line (`tui.go:335-343`) to render e.g.
`custom charter · 2 skills · tools: dream, omen` — the ONLY TUI region this feature
touches (TASK-63 owns digest/villager-detail/transcript regions).

**Rationale**: `Status()` is already the provenance channel (`CharterDefault` +
`charterIsDefault`); extending it keeps one model-free status path and confines the TUI
delta to minimize the TASK-63 merge surface (both tasks edit `tui.go`, disjoint regions).

**Alternatives considered**: new IPC verb — rejected: nothing needs a second round-trip.
Full provenance panel in the TUI — deferred: the header summary satisfies FR-010/SC-005;
a richer panel can ride TASK-63's pane work later.

## R9 — What is deliberately out of scope

- No new query/read tools for the angel (spec assumption; manifest design leaves room).
- No in-game file editor; players edit with their own tools (charter precedent).
- No per-villager gating, no stage presets themselves (TASK-68 consumes the manifest).
- No changes to charge accrual, the charge cap, or nudge/miracle landing semantics.
- No new store event types; replay/determinism untouched (LLM calls are recorded inputs;
  prompt composition is upstream of the recording boundary).
