---
name: metatron
description: The gatekeeper angel (TASK-12) — console AND system-authored turns driven through the bounded tool-use loop (spec 017), omen/vision influence and standing-order agency behind a structural prompt firewall (spec 029), event-sourced charge economy, charge-free clock-control meta tools, digests + drama moments, and the staged player-editable instruction surface (charter + skills/ + capabilities.json, spec 021)
kind: component
sources:
  - internal/metatron/metatron.go
  - internal/metatron/turn.go
  - internal/metatron/toolcalls.go
  - internal/metatron/orders.go
  - internal/metatron/charter.go
  - internal/metatron/digest.go
  - internal/metatron/miracle_batch.go
  - internal/sim/metatron.go
  - internal/persona/charter.go
verified_against: e9213e17e6e48cf30da802949d9b59e0e3d78370
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

**Turns** (`turn.go`): one directive = one `Turn`, driven through [[tool-loop]]'s
bounded loop (`toolloop.Run`, spec 017 T020) against `llm.KindMetatron` cloud calls
([[llm-orchestrator]]), serialized single-flight. Since spec 029 (TASK-27) the turn
body is extracted into the shared `runTurn`, and there are two origins
(`turnOrigin`): a **console** turn (`Turn`, the player's words) and a
**system-authored** turn (a triggered standing order — see [[metatron-orders]]).
Both run the identical body — same single-flight `turnBusy` guard, same roster/
handler/gate composition, same telemetry, same transcript append — differing only
in framing: the console path opens the transcript with the player's `> …` line and
uses the correlation id `turn-metatron-<tick>`; the system path opens with a
`[watch]` origin marker over the order's pre-authorized action (never a player-text
line — a triggered turn has no player text), uses `watch-metatron-<tick>`, and
suppresses moment consumption (the player-facing queue awaits the next console open;
the trigger worker queues the system turn's own outcome moment). The console CAS-fails
fast with `ErrTurnBusy`; a system turn WAITS bounded for the slot ([[metatron-orders]]).
The prompt stacks the charter (re-read every turn — edits are live by construction,
with restore/empty/truncate fallbacks and in-reply notices, `charter.go`), then the
skill files (spec 021: `loadSkills` composes eligible `skills/*.md` — regular `.md`
direct children, ascending bytewise filename order, ≤8 files, ≤4,000 chars each via
`persona.CharterMaxChars`, each under a `--- skill: <name> ---` separator, with the
same truncate/skip notice discipline as the charter), then a fixed frame appended
LAST as compile-time constants on every path — no editable byte can displace or
truncate it (adversarial battery + determinism tests in `metatron_test.go`). The
frame pins the two `metatronNonNegotiables` invariants beneath ANY editable text
(never invent unobserved events; never pass the player's words to a villager) plus,
since spec 029, the `metatronInitiativeFrame` (T019) that binds clock control and
standing orders to player-requested or pre-authorized action only — never the
angel's own initiative, with the door-side grant gate backing it independently. The
frame also carries the acting-tool guidance DERIVED from the registry
(`tool.MetatronToolGuidance` over the world's granted roster, [[tool-registry]]) —
the old hand-written prose tool list is gone, so described ≡ declared by
construction. The turn also stacks live status (clock, ⚡ bank, roster),
queued moments, the [[chronicle]] tail (the angel reads its village's story — this
grounds fresh reigns and upgraded worlds), its soul tail, and recent transcript. The
model may reply with words (**converse** — the transcript-only final-answer channel,
`toolloop.Result.Final`; `converse` is deliberately NOT a declared loop tool, so it
can never be rejected as unknown) or call exactly one acting tool. Since spec 029
the declared loop roster is `send_vision`/`send_omen`/`monitor_and_act`/
`cancel_order`/`work_miracle`/`pause`/`start`/`adjust_speed`
(`tool.LoopRosterMetatron`, in that order) — the retired `nudge_dream`/`nudge_omen`
forms are gone from the registry — or fewer: since spec 021 the world's
`capabilities.json` gates the roster per-read through three independent layers, one
`grantedRoster` feeding declaration, guidance prose, and the handler set, with
defense-in-depth `grant.allows` checks in the landers; an ungranted
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
can be retried instead of ending the turn outright. The per-round token budget
is `mt.turnTokens` (spec 025, TASK-72: the operator-tunable `llm.json`
`max_tokens.metatron_turn` knob, threaded through `metatron.New` like
`loopRounds`; default 1024, up from the pre-loop 700 — a tool-era round must
carry a `tool_use` block alongside any converse prose in the same round, so
the budget grew to keep a full charter-voiced reply from crowding out a
same-round act). When the loop ends with no text and no landed
act (model_done with nothing, cap exhaustion, or a soft error), the same
scattered-thoughts apology as before covers it.

**Influence: omens and visions** (spec 029, TASK-27): the two mediated forms that
replaced the retired `dream`/`omen` nudges. A **vision** (`send_vision`, `landVision`)
reaches exactly one living villager at ANY hour; an **omen** (`send_omen`, `landOmen`)
reaches one villager, a named comma-separated group, or the word `everyone` — but
only at NIGHT. Each landed act costs exactly ONE charge regardless of recipient
count, console-initiated or triggered. Validation (living target(s), non-empty text,
charges ≥ 1) downgrades failures to refusal-with-counsel — refusals are free, fed
back as a `rejected_gate` the model may repair in a later round. The `dream` form is
gone from the angel's vocabulary; the spec-014 `OnRoster(RosterMetatron, "nudge_"+form)`
check is replaced by an explicit form switch in the reducer (`vision`/`omen`/`dream`,
with `dream` grandfathered replay-only — no live tool can produce a new one). The
400-byte text cap is still a registry read, re-pointed at `send_vision`'s entry
(`nudgeTextMax` in turn.go, `sim.NudgeTextMax` reducer-side, from the same tool so
truncation and enforcer never diverge).

Both landers share `landNudgeBatch` — the text cap, the ONE atomic `InjectSocial`
batch, and the soul append, VERBATIM the pre-029 `landNudge` body (wrap, don't
rewrite): `metatron.nudged{form, targets, text}` (validating reducer spends the
charge and enforces the omen NIGHT gate at the door; `send_omen`'s day path never
reaches here) + one prefixed (`"You saw a vision: "` / `"You witnessed an omen: "`)
`agent.memory_added` per target at `SalDream` (8), each stamped `Origin: sim.OriginOmen`
(spec 030) — a direct-perception provenance class per `sim.DirectPerception` (same
standing as an own act or a witnessed event), which the villager interprets in
persona. `landVision`/`landOmen` differ only in target
resolution and the per-tool grant gate. The firewall is structural, not behavioral:
no code path exists from model output to any villager surface OR clock control
outside registered tools (sentinel-audited by `TestHandlerFirewallAudit`,
`metatron_test.go`, extended to the spec-029 surface, SC-007). A **daytime**
`send_omen` neither lands nor refuses — it defers to nightfall as a system-origin
standing order ([[metatron-orders]]).

**Miracles** (spec 016, [[metatron-miracles]]): the angel's other charge-priced
mediated act, spent from the same bank, a declared loop tool: `work_miracle`
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
perception-memory companions (each stamped `Origin: sim.OriginOmen`, spec 030,
identically to a nudge's memories), and lands it through `InjectSocial`. A rejection at
the reducer dry-run becomes a `rejected_gate` the loop feeds back (rather than an
immediate reply-suffix refusal, though the wording is the same in-fiction
counsel); a landed miracle also appends a soul-file line. `landMiracle`'s
validation/batch/soul-append logic is likewise UNCHANGED from the pre-loop path —
only the input moved from `turnReply.Miracle` to `work_miracle`'s tool-call
arguments.

**Standing orders** (spec 029, its own note [[metatron-orders]]): `monitor_and_act`
places an event-sourced watch-and-act order whose condition, compiled once, is
matched for free against the live event stream; when it fires, the angel wakes and
runs the pre-authorized action as a system-authored turn through this same door.
`cancel_order` retires one. `handleMonitor`/`handleCancelOrder` (`toolcalls.go`) wrap
the door helpers `placeOrder`/`cancelOrder` (`orders.go`); the turn prompt carries
active orders (`writeStandingOrders`, FR-017) and `Status.Orders` lists them
model-free (FR-016). The full lifecycle, event sourcing, matching, trigger execution,
fuzzy confirm, and daytime-omen deferral live in [[metatron-orders]].

**Meta tools** (spec 029 US5): `pause`/`start`/`adjust_speed` are charge-free
registered tools (`Effect` Expressive, EMPTY `Events`) that drive the world clock
through the `LoopControl` seam — the SAME `*sim.Loop.Do` the [[ipc-server]] uses, so
a metatron-issued control is indistinguishable from an operator one and lands the
loop's own `clock.paused`/`clock.resumed`/`clock.speed_set` records (no new event
vocabulary). The daemon injects the loop twice at `metatron.New` — as the `Injector`
and as `LoopControl` (the `mind.New(loop, loop)` precedent). Mapping (`controlLoop`):
`pause`→`Do("pause")`, `adjust_speed`→`Do("set_speed", speed)`, and `start` resumes —
a bare start is `Do("resume", "")`, but a `start` WITH a speed issues `Do("set_speed",
speed)` THEN `Do("resume", "")` for the one call (the loop's resume ignores its speed
argument). They inject nothing and spend nothing but consume the turn's one act; each
is grant-gated and structurally absent when the manifest withholds it. The
`metatronInitiativeFrame` (above) binds their use to player authority.

**Tool-call telemetry** (`toolcalls.go`, spec 017 FR-007/T018/T020): every model
tool call the turn's loop saw — landed, rejected, or otherwise — is buffered as a
`toolloop.CallRecord` (the `Job.Record` sink) and lands as one `cog.tool_call`
batch through `InjectSocial` on EVERY termination path (so a rejected or
never-grounded call is recorded even when nothing landed), via the same
`sim.NewCogToolCallPayload` constructor [[agent-mind]]'s mind uses — a converse-
only turn (no tool calls) emits no batch at all. The turn's handlers are exactly
the granted subset of `send_vision`/`send_omen`/`monitor_and_act`/`cancel_order`/
`work_miracle`/`pause`/`start`/`adjust_speed`; `converse` is deliberately absent
from the handler map (and from `tool.LoopRosterMetatron()`) since it is the
final-text channel, never a callable tool. Since spec 025 (TASK-72) the turn
also surfaces the loop's one in-loop transport retry: when
`toolloop.Result.Retried` is set, `emitRetried` (toolcalls.go) lands a
NON-terminal `cog.outcome` carrying `sim.OutcomeRetried` and the first
failure's reason through the same `InjectSocial` door — emitted BEFORE the
error return, so even a twice-failed turn's retry is countable from the trail.

**Charge economy** (`internal/sim/metatron.go`): `State.MetatronCharges` — genesis
1, cap 3, +1 per absolute 6-game-hour boundary emitted by the [[executor]]
(`metatron.charge_regenerated`, a pure function of state + tick), −1 per landed
omen or vision (a miracle spends its per-kind cost). Fully event-sourced: replay
reproduces the bank exactly; the field is
deliberately not `omitempty` so a spent-to-zero bank survives snapshots; pre-TASK-12
snapshots gain the genesis charge on upgrade.

**Watching** (`digest.go`): notable events collect per 6-game-hour window; each
non-empty window costs one summarization call appended to `metatron/soul.md`
(skip-empty is free; failures carry lines into the next window). The drama rule v1:
`agent.died`, `gru.attacked`, `social.promise_broken`, and (since spec 029)
`metatron.order_expired` append model-free **moment** lines immediately and queue for
the console — the next reply leads with them. Digests and moments themselves never
construct an act; the angel acts only when the player asks OR a standing order the
player placed authorizes it (spec 029 relaxed the old "acts only when told" contract
to admit pre-authorized triggered turns — see [[metatron-orders]]).

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
`work_miracle(move,give_item)` form when kinds are restricted),
`manifest_default` (no `capabilities.json` present), and since spec 029 the active
standing orders (`orders`, `OrderStatus{id, condition, origin, fuzzy, expires_day,
status}`, FR-016 — see [[metatron-orders]]).

## Connections

[[sim-loop]] whitelists `metatron.nudged`, the four `metatron.*` miracle types, and
the three injected order events (`order_placed`/`order_cancelled`/`order_triggered`);
[[sim-state-reducer]] holds the bank, the miracle reducer arms ([[metatron-miracles]]),
and the standing-order arms ([[metatron-orders]]); [[executor]] regenerates the bank
and emits `metatron.order_expired`; [[event-types]] catalogs all three families;
[[llm-orchestrator]] routes `KindMetatron` to the cloud tier and the fuzzy
`KindMetatronWatch` confirm to a cheap chain; [[chronicle]] feeds the angel's
grounding; [[agent-mind]] is how villagers interpret what lands; [[daemon-lifecycle]]
wires the component behind the LLM-config gate, passing the loop as both `Injector`
and `LoopControl`; [[ipc-server]]'s `handleMiracle` is the miracle's other door,
sharing `BuildMiracleBatch` with `landMiracle` here, and the clock commands the meta
tools reuse. [[tool-loop]] is the turn driver (console and system-authored) since spec
017: `runTurn` calls `toolloop.Run` with `tool.LoopRosterMetatron()` and the granted
handler subset; [[tool-registry]] declares those tools (and deliberately excludes
`converse`), derives the turn's tool guidance (`MetatronToolGuidance`), and holds the
single miracle cost source ([[metatron-miracles]]). Specs: `specs/005-metatron/`,
`specs/016-metatron-miracles/`, `specs/017-agent-tool-loop/`,
`specs/021-metatron-instruction-surface/`, `specs/029-metatron-agency/`.

## Operational notes

Live-proven on a fresh world (reign-test: judged dream landed atomically, exhaustion
refused with counsel, BRUTUS charter edit live next turn, digest + regen at the 12:00
boundary) and on the 14-day chronicle-proof world (upgrade granted the genesis
charge; the angel answered "what do you know of Fern and the voice at the well?"
from the chronicle ring, honestly bounded its knowledge, then landed an in-world
dream that wove in the smooth stone from the story). Live finding folded back: the
no-invention rule originally lived in the (replaceable) default charter — a surly
custom charter invited fabricated villager activity; both invariants now sit in the
fixed frame. (Those reign-tests predate spec 029, when the live influence form was
still `dream`; the vocabulary is now omen/vision, but the atomicity, exhaustion, and
charter-edit findings carry over unchanged.) Cost: ~4 digests/game-day + player turns
+ any triggered watch turns, noise against the ceiling. Spec 029 (TASK-27) shipped
standing orders and pre-authorized autonomous action ([[metatron-orders]]); still
parked for post-v1: world-tools, full regency, drama-based cloud escalation of
villager minds.
