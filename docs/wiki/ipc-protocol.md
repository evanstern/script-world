---
name: ipc-protocol
description: The wire contract — JSON-lines over a Unix socket; Request/Response/Push envelopes and the shared StatusData shape
kind: concept
sources:
  - internal/ipc/protocol.go
  - specs/001-world-daemon/contracts/client-protocol.md
verified_against: 6eb8b60ceb65d760408051eadf50a789603efa18
---

# IPC protocol

Clients talk to a world's daemon over the Unix socket in its save directory, one JSON
object per newline-delimited line. The Go types in `internal/ipc/protocol.go` are the
single source for both sides of the wire; the prose contract lives in
`specs/001-world-daemon/contracts/client-protocol.md`.

## How it works

Three envelopes:

- `Request{id, cmd, args}` — client → daemon; `id` is client-chosen and echoed back.
- `Response{id, ok, data | error}` — daemon → client.
- `Push{push, event | last_seq}` — daemon → subscribed client: `push: "event"`
  carries a `store.Event`; `push: "dropped"` carries `last_seq` and means the
  subscription was canceled on buffer overflow (re-subscribe with `since: last_seq`).

Clients demux on the presence of `id` vs `push` (`wireMsg` is the union used by the
client reader). Responses and pushes may interleave.

Commands: `status`, `state` (returns `StateData{state, last_seq}` — the full
canonical world-state JSON plus the log position it reflects, captured coherently in
one loop iteration; subscribe with `since: last_seq` for a gapless live replica),
`subscribe` (`SubscribeArgs{since}` — replay after that seq, then live, gapless),
`unsubscribe`, `pause`, `resume`, `set_speed` (`SetSpeedArgs{speed}`), `llm_call`
(`LLMCallArgs{kind, system, prompt, max_tokens}` → an `llm.Response` with tier,
model, tokens, cost, latency — errors when the world has no orchestrator), and
`shutdown`, and the Metatron console pair (TASK-12, [[metatron]]): `metatron_chat`
(`MetatronChatArgs{text}` → a `metatron.TurnResult` with reply, optional landed
nudge, charge bank, surfaced moments — a long call, one cloud round-trip) and
`metatron_status` (no args → the model-free `metatron.Status` peek), and `miracle`
(spec 016, [[metatron-miracles]]): `MiracleArgs{kind, day?, time?, villager?,
item?, qty?, class?, x?, y?, to_x?, to_y?, gratis?}` where `kind` selects
`time_snap`/`give_item`/`move`/`remove` and the remaining fields are that kind's
arguments → `MiracleData{kind, charges, gratis, summary}`. `miracle` is the
**only** surface that accepts `gratis` (the CLI's `--force` sets it, waiving the
charge — the angel's turn path has no equivalent field); the handler needs only
the sim loop, no LLM/angel presence, so it works in a pure-sim world. `StatusData`
gains an optional `llm` section (tier health, queue depths,
monthly spend vs budget) when the orchestrator is enabled.

`StatusData` is the shared response shape for status/pause/resume/set_speed, with four
sections: `world` (name, seed, format_version), `clock` (tick, game_time, paused,
speed, effective_rate, degraded, metatron_charges — the ⚡ bank, so clients need no
state fetch — plus, since spec 028, three additive `omitempty` adaptive-throttle
fields: `requested_speed` (the player's ceiling from sim state, empty when
ungoverned), `governor_debt`/`governor_jobs` (the daemon governor sampler's latest
staleness-debt reading, folded in exactly like the `llm` section — [[cognition]],
[[daemon-lifecycle]]); all three are zero/absent for a no-LLM world or an inert
governor, so pre-028 status bytes are unchanged), `daemon` (pid, uptime_seconds,
subscribers), `log`
(last_seq). `set_speed`'s existing refusal of uncapped `max` while an LLM is
configured is retained unchanged (spec 028 FR-012) — the governor only ever
moves `speed`/`effective_rate` along the capped ladder these fields describe,
never `max`.

Line caps (TASK-19): request lines are capped at 1 MiB, reply/push lines at
64 MiB. The daemon never emits a line over the cap — a reply that would exceed
it is substituted with an `ok:false` response whose `error` starts with
`reply too large` (carrying the byte counts). Clients classify that prefix —
and any raw over-long line — as `ipc.ErrReplyTooLarge`, a fatal error retrying
cannot fix.

Failure semantics: unknown cmd or bad args → `ok:false`, connection stays open;
malformed JSON → connection closed; daemon absent → socket connect fails fast;
oversized reply → the substituted `reply too large` error above.

## Connections

[[ipc-server]] implements the daemon side; [[ipc-client]] the attach side;
[[event-types]] defines what rides inside event pushes; [[cli-promptworld]] renders
`StatusData` for humans. The [[tui-client]] consumes `state` + `subscribe` to run its
live replica. `miracle` is the CLI/IPC operator door into [[metatron-miracles]].

## Operational notes

No authentication — trusted single-operator host, filesystem permissions on the socket
are the boundary. The protocol is debuggable with `nc -U <dir>/daemon.sock` and raw
JSON lines. Changing any field name here is a breaking wire change and must update the
contract doc in the same commit.
