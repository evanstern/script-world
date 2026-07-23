---
name: metatron
description: The gatekeeper angel (TASK-12) — console turn driven through the bounded tool-use loop (spec 017), dream/omen nudge mediation behind a structural prompt firewall, event-sourced charge economy, digests + drama moments, and the staged player-editable instruction surface (charter + skills/ + capabilities.json, spec 021)
kind: component
sources:
  - internal/metatron/metatron.go
  - internal/metatron/turn.go
  - internal/metatron/toolcalls.go
  - internal/metatron/charter.go
  - internal/metatron/digest.go
  - internal/metatron/miracle_batch.go
  - internal/sim/metatron.go
  - internal/persona/charter.go
verified_against: 8c44bf21ad22c0f1bad07ae7f2a08072a0cb5544
---

# Metatron

Metatron is the player's sole verb: a daemon-hosted gatekeeper (`internal/metatron`,
the mind/scribe notify-consumer pattern) that converses in the console, watches the
world, and mediates all influence. Raw player text has exactly one sink — Metatron's
own prompt; villagers can only ever receive Metatron's validated rendering, landed
through [[sim-loop]]'s injection door as recorded events. The meta-game is
prompt-engineering your angel through the staged instruction surface (spec 021,
TASK-64), shaped like real assistant configuration: `charter.md` (the
CLAUDE.md-shaped base), `skills/*.md` (player-authored SKILL.md-shaped files),
and `capabilities.json` (the per-world tool grant manifest) — all at the
save-dir root, all re-read fresh every turn.

## How it works

**Console turns** (`turn.go`): one player message = one console `Turn`, driven
through [[tool-loop]]'s bounded loop (`toolloop.Run`, spec 017 T020) against
`llm.KindMetatron` cloud calls ([[llm-orchestrator]]), serialized single-flight.
The prompt stacks the charter (re-read every turn — edits are live by construction,
with restore/empty/truncate fallbacks and in-reply notices, `charter.go`), then the
skill files (spec 021: `loadSkills` composes eligible `skills/*.md` — regular `.md`
direct children, ascending bytewise filename order, ≤8 files, ≤4,000 chars each via
`persona.CharterMaxChars`, each under a `--- skill: <name> ---` separator, with the
same truncate/skip notice discipline as the charter), then a fixed frame appended
LAST as a compile-time constant (`metatronNonNegotiables`) on every path — no
editable byte can displace or truncate it (adversarial battery + determinism tests
in `metatron_test.go`). The frame pins two invariants beneath ANY editable text
(never invent unobserved events; never pass the player's words to a villager) and
carries the acting-tool guidance DERIVED from the registry
(`tool.MetatronToolGuidance` over the world's granted roster, [[tool-registry]]) —
the old hand-written prose tool list is gone, so described ≡ declared by
construction. The turn also stacks live status (clock, ⚡ bank, roster),
queued moments, the [[chronicle]] tail (the angel reads its village's story — this
grounds fresh reigns and upgraded worlds), its soul tail, and recent transcript. The
model may reply with words (**converse** — the transcript-only final-answer channel,
`toolloop.Result.Final`; `converse` is deliberately NOT a declared loop tool, so it
can never be rejected as unknown) or call exactly one acting tool
(`nudge_dream`/`nudge_omen`/`work_miracle` — or fewer: since spec 021 the world's
`capabilities.json` gates the roster per-read through three independent layers, one
`grantedRoster` feeding declaration, guidance prose, and the handler set, with
defense-in-depth `grant.allows` checks in `landNudge`/`landMiracle`; an ungranted
tool is structurally absent from the declared schemas, never merely prose-forbidden;
missing manifest = full roster byte-compatibly, malformed = full roster + notice,
`tools: []` = conversation-only — converse is never gateable), which lands through
its existing door;
the driver's one-acting-call cardinality enforces "at most one mediated act per
turn" structurally, so the pre-loop nudge-wins-over-miracle precedence dissolves —
the model just picks its one act. The retired `turnReply`/`parseTurn` free-text
JSON contract (`{say, nudge|null, miracle|null}`) is gone: a door refusal now
becomes a `rejected_gate` fed back to the model within the loop's round cap — a
behavior upgrade over the old single-shot refusal, since a mistyped villager name
can be retried instead of ending the turn outright. `turnMaxTokens` is 1024 (up
from 700): a tool-era round must carry a `tool_use` block alongside any converse
prose in the same round, so the budget grew to keep a full charter-voiced reply
from crowding out a same-round act. When the loop ends with no text and no landed
act (model_done with nothing, cap exhaustion, or a soft error), the same
scattered-thoughts apology as before covers it.

**Nudges**: `dream` (one living villager) or `omen` (all living, recorded
explicitly). Validation (form, living target, ≤400 chars, charges ≥ 1) downgrades
failures to refusal-with-counsel — refusals are free, and (since spec 017) fed
back as a `rejected_gate` the model may repair in a later round rather than a
turn-ending refusal. Since spec 014 (TASK-53) the
form must be a nudge tool on the [[tool-registry]]'s `RosterMetatron` — checked
both turn-side (`landNudge`) and in the reducer dry-run, with the same
unknown-form reason as before — and the 400-byte text cap is a registry read
(`nudgeTextMax` in turn.go, `sim.NudgeTextMax` reducer-side, both from the same
`nudge_dream` entry so the truncation and the enforcer can never diverge). A landed nudge is ONE atomic
`InjectSocial` batch: `metatron.nudged{form, targets, text}` (validating reducer
spends the charge; the dry-run enforces it at the door) + one prefixed
(`"You dreamed: "` / `"You witnessed an omen: "`) `agent.memory_added` per target at
`SalDream` (8) — provenance-unknown memories the villager interprets in persona.
The firewall is structural, not behavioral: no code path exists from console input
to any villager surface (sentinel-audited in `metatron_test.go`). `landNudge`'s
validation/batch/soul-append logic is UNCHANGED from the pre-loop path (spec 017
T020: wrap, don't rewrite) — only its input moved from a parsed `turnReply.Nudge`
struct to the `nudge_dream`/`nudge_omen` tool call's arguments
(`internal/metatron/toolcalls.go`'s `handleNudge`).

**Miracles** (spec 016, [[metatron-miracles]]): the angel's other mediated act,
spent from the same charge bank, now the fourth declared loop tool: `work_miracle`
(`kind` ∈ `move`/`remove`/`give_item`/`time_snap`). The retired
`turnReply.Miracle` anonymous struct had **no gratis field** as its structural
guarantee; the replacement `miracleArgs` (`toolcalls.go`, the tool-call-parsed
mirror of the same flat surface) keeps that guarantee identically — nothing to
unmarshal `gratis` into, so a model-driven miracle can never waive its charge.
`landMiracle` resolves door-neutral `MiracleParams` (villager name → index,
day/`HH:MM` → tick via [[game-clock]]'s `ParseTimeOfDay`/`TickAt`) from an
`agentXY` snapshot the absorb goroutine mirrors per batch (so the turn worker
never races the live replica), then calls the shared `metatron.BuildMiracleBatch`
— the SAME builder the IPC `miracle` door uses — to compose the event and its
perception-memory companions, and lands it through `InjectSocial`. A rejection at
the reducer dry-run becomes a `rejected_gate` the loop feeds back (rather than an
immediate reply-suffix refusal, though the wording is the same in-fiction
counsel); a landed miracle also appends a soul-file line. `landMiracle`'s
validation/batch/soul-append logic is likewise UNCHANGED from the pre-loop path —
only the input moved from `turnReply.Miracle` to `work_miracle`'s tool-call
arguments.

**Tool-call telemetry** (`toolcalls.go`, spec 017 FR-007/T018/T020): every model
tool call the turn's loop saw — landed, rejected, or otherwise — is buffered as a
`toolloop.CallRecord` (the `Job.Record` sink) and lands as one `cog.tool_call`
batch through `InjectSocial` on EVERY termination path (so a rejected or
never-grounded call is recorded even when nothing landed), via the same
`sim.NewCogToolCallPayload` constructor [[agent-mind]]'s mind uses — a converse-
only turn (no tool calls) emits no batch at all. `nudge_dream`/`nudge_omen`/
`work_miracle` are the turn's only handlers; `converse` is deliberately absent
from the handler map (and from `tool.LoopRosterMetatron()`) since it is the
final-text channel, never a callable tool.

**Charge economy** (`internal/sim/metatron.go`): `State.MetatronCharges` — genesis
1, cap 3, +1 per absolute 6-game-hour boundary emitted by the [[executor]]
(`metatron.charge_regenerated`, a pure function of state + tick), −1 per landed
nudge. Fully event-sourced: replay reproduces the bank exactly; the field is
deliberately not `omitempty` so a spent-to-zero bank survives snapshots; pre-TASK-12
snapshots gain the genesis charge on upgrade.

**Watching** (`digest.go`): notable events collect per 6-game-hour window; each
non-empty window costs one summarization call appended to `metatron/soul.md`
(skip-empty is free; failures carry lines into the next window). The drama rule v1:
`agent.died`, `gru.attacked`, `social.promise_broken` append model-free **moment**
lines immediately and queue for the console — the next reply leads with them.
V1 contract: the angel acts only when told; digests and moments can never construct
a nudge.

**Files** (bound to the run, not event-sourced): `charter.md` at the save-dir root
(seeded by `persona.Genesis`, never overwritten), plus the optional player-created
`skills/` dir and `capabilities.json` manifest beside it (spec 021 — root =
player-authored configuration); `metatron/soul.md` (accreting notes, starts empty)
and `metatron/transcript.md` (console history) — restart survival comes free with
files, and world determinism never depends on them (prompt composition is upstream
of the recorded LLM inputs).

**Surfaces**: IPC `metatron_chat`/`metatron_status` ([[ipc-protocol]]), CLI
`promptworld metatron <dir> [message…]` ([[cli-promptworld]]), and the
[[tui-client]] metatron pane (the console). Protocol status (`metatron.Status`, the
model-free peek, computed fresh from disk per call) carries the ⚡ bank, charter
provenance (`charter_default`), and since spec 021 the effective skill filenames
(`skills`, composition order), the granted roster (`granted_tools`, registry order,
`work_miracle(move,give_item)` form when kinds are restricted), and
`manifest_default` (no `capabilities.json` present).

## Connections

[[sim-loop]] whitelists `metatron.nudged` and the four `metatron.*` miracle
types; [[sim-state-reducer]] holds the bank and the miracle reducer arms
([[metatron-miracles]]); [[executor]] regenerates it; [[event-types]] catalogs
both families; [[llm-orchestrator]] routes `KindMetatron` to the cloud tier;
[[chronicle]] feeds the angel's grounding; [[agent-mind]] is how villagers
interpret what lands; [[daemon-lifecycle]] wires the component behind the
LLM-config gate; [[ipc-server]]'s `handleMiracle` is the miracle's other door,
sharing `BuildMiracleBatch` with `landMiracle` here. [[tool-loop]] is the console
turn's driver since spec 017: `Turn` calls `toolloop.Run` with
`tool.LoopRosterMetatron()` and the `nudge_dream`/`nudge_omen`/`work_miracle`
handlers; [[tool-registry]] declares those tools (and deliberately excludes
`converse`), derives the turn's tool guidance (`MetatronToolGuidance`), and holds
the single miracle cost source ([[metatron-miracles]]). Specs: `specs/005-metatron/`,
`specs/016-metatron-miracles/`, `specs/017-agent-tool-loop/`,
`specs/021-metatron-instruction-surface/`.

## Operational notes

Live-proven on a fresh world (reign-test: judged dream landed atomically, exhaustion
refused with counsel, BRUTUS charter edit live next turn, digest + regen at the 12:00
boundary) and on the 14-day chronicle-proof world (upgrade granted the genesis
charge; the angel answered "what do you know of Fern and the voice at the well?"
from the chronicle ring, honestly bounded its knowledge, then landed an in-world
dream that wove in the smooth stone from the story). Live finding folded back: the
no-invention rule originally lived in the (replaceable) default charter — a surly
custom charter invited fabricated villager activity; both invariants now sit in the
fixed frame. Cost: ~4 digests/game-day + player turns, noise against the ceiling.
Parked for post-v1: world-tools, standing orders/regency, autonomous action,
drama-based cloud escalation of villager minds.
