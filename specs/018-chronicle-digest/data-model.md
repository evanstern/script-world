# Data Model: Chronicle Digest Grammar & Selection Detail

**Feature**: specs/018-chronicle-digest | **Date**: 2026-07-22

View-layer entities only — no stored formats change. Names are indicative; the implementer may adjust idiomatically within the contracts.

## Pure formatting layer (`internal/tui/grammar.go`, `digest.go`)

### seg — one styled span of a summary (R4)

| Field | Type | Notes |
|---|---|---|
| `Text` | `string` | plain text, no ANSI |
| `Role` | `segRole` | `segText` \| `segName` \| `segSpeech` \| `segEmphasis` \| `segLabel` |

Validation: concatenated `Text` of all segs is the plain summary; wrap/truncate operates on the concatenation, styles re-applied per span after wrapping.

### digestFunc & registry (R1)

```
type digestFunc func(e store.Event, names []string) (segs []seg, ok bool)
var digestRegistry map[string]digestFunc   // key: full event type
```

- `ok=false` (unmarshal failure) and registry miss both fall back to compact resolved-name JSON (`resolvePayloadNames`) rendered as a single `segText` — FR-002/FR-003 edge behavior.
- Agent index → name resolution inside digests uses the existing `agentName(names, idx)` (out-of-range → `#N`).

### chronicleLine v2 (replaces the current struct's Subject/Speech/Payload split)

| Field | Type | Notes |
|---|---|---|
| `Seq` | `int64` | detail pane only — leaves the feed line |
| `Tick` | `int64` | NEW column (solo; dropped in dock per FR-004) |
| `Time` | `string` | `clock.Format(e.Tick)`, width 5 |
| `Type` | `string` | full type (solo); last namespace segment (dock) |
| `Family` | `eventFamily` | namespace prefix; `meeting`+`norm` → governance (R2) |
| `Summary` | `[]seg` | digest output or fallback |

### eventFamily (R2)

Enum over: `world clock sim agent social governance gru chronicle metatron daemon cog` (+ `unknown`). Derivation: prefix before first `.`; override map merges `meeting`/`norm` → `governance`. Voice: labeled for `cog|clock|daemon`, phrase otherwise (contract §3 per-type).

## View layer (`internal/tui/views.go`)

### Style tokens (extends the existing token block)

| Token | Role |
|---|---|
| `styleFamilyAgent` / `Social` / `Sim` / `Governance` / `Gru` / `Metatron` / `Cog` / `World` | family tint applied to the type column (and summary base where the contract says so) |
| `styleFeedTick` | tick column (dim, like seq today) |
| `styleFeedEmphasis` | `segEmphasis` tokens (amounts, kinds, causes) |
| `styleFeedAlert` | high-salience lines (`agent.died`, `gru.attacked`, `social.chest_taken`, `norm.violated`) — bold + distinct foreground |
| existing `styleFeedType/Name/Speech/Clock/Select`, `styleDim` | retained; `styleFeedClock` becomes the `clock` family tint |

Exact colors chosen at implementation against the documented color-role contract (contract §4); roles, not raw colors, are the model.

### Column layout state (computed per render, not stored)

| Value | Rule (R5) |
|---|---|
| tick width | widest tick among visible rows, right-aligned |
| type width | longest type among visible rows, cap 26 (solo) / 10 short-name (dock) |
| summary width | remaining panel width; wraps (dock ≤3 lines) or truncates `…` (solo 1 line) |

## Interaction state (`internal/tui/tui.go` Model)

| Field | Change |
|---|---|
| `chronSelected int` | kept — selection base semantics unchanged (`chronSelectionBase`) |
| `chronExpanded bool` | **removed** (R7) |
| `chronExpIdx int` | **removed** (R7) |
| `chronDetailScroll int` | **new** — detail pane scroll offset; reset to 0 on selection move, pause exit, reconnect |

State transitions:
- pause + chronicle visible → inspect mode (unchanged trigger, `inspecting()`); detail pane always rendered for `chronSelectionBase()`.
- `j/k/g/G` move selection (unchanged) and reset `chronDetailScroll`.
- `J`/`K` scroll the detail pane within `[0, paneContentLines−paneRows]`.
- `⏎` no-op, reserved (FR-009 extension point).
- resume → running mode, tail-follow restored, `chronDetailScroll` reset (matches existing reset of selection state at reconnect, tui.go:325).

## Invariants

1. Payload bytes are never rewritten in the detail pane — annotations only (`formatInspector` contract preserved).
2. The pure layer emits no ANSI; all styling happens in views.go over segs.
3. Every type in the sweep fixture has a registry entry (SC-001 gate); registry keys not in the fixture fail the sweep too (no unlisted digests).
4. Digest fallback is total: no event, however malformed, renders blank or panics.
