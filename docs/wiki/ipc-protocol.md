---
name: ipc-protocol
description: The wire contract — JSON-lines over a Unix socket; Request/Response/Push envelopes and the shared StatusData shape
kind: concept
sources:
  - internal/ipc/protocol.go
  - specs/001-world-daemon/contracts/client-protocol.md
verified_against: cee600e086a1be15868205c16c395ee33aaa397e
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
`shutdown`. `StatusData` gains an optional `llm` section (tier health, queue depths,
monthly spend vs budget) when the orchestrator is enabled.

`StatusData` is the shared response shape for status/pause/resume/set_speed, with four
sections: `world` (name, seed, format_version), `clock` (tick, game_time, paused,
speed, effective_rate, degraded), `daemon` (pid, uptime_seconds, subscribers), `log`
(last_seq).

Failure semantics: unknown cmd or bad args → `ok:false`, connection stays open;
malformed JSON → connection closed; daemon absent → socket connect fails fast.

## Connections

[[ipc-server]] implements the daemon side; [[ipc-client]] the attach side;
[[event-types]] defines what rides inside event pushes; [[cli-scriptworld]] renders
`StatusData` for humans. The [[tui-client]] consumes `state` + `subscribe` to run its
live replica.

## Operational notes

No authentication — trusted single-operator host, filesystem permissions on the socket
are the boundary. The protocol is debuggable with `nc -U <dir>/daemon.sock` and raw
JSON lines. Changing any field name here is a breaking wire change and must update the
contract doc in the same commit.
