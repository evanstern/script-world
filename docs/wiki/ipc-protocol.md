---
name: ipc-protocol
description: The wire contract — JSON-lines over a Unix socket; Request/Response/Push envelopes and the shared StatusData shape
kind: concept
sources:
  - internal/ipc/protocol.go
  - specs/001-world-daemon/contracts/client-protocol.md
verified_against: 08d8c70e23c104a4c61df1749c00cb315f5c643d
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

Commands: `status`, `subscribe` (`SubscribeArgs{since}` — replay after that seq, then
live, gapless), `unsubscribe`, `pause`, `resume`, `set_speed`
(`SetSpeedArgs{speed}`), `shutdown`.

`StatusData` is the shared response shape for status/pause/resume/set_speed, with four
sections: `world` (name, seed, format_version), `clock` (tick, game_time, paused,
speed, effective_rate, degraded), `daemon` (pid, uptime_seconds, subscribers), `log`
(last_seq).

Failure semantics: unknown cmd or bad args → `ok:false`, connection stays open;
malformed JSON → connection closed; daemon absent → socket connect fails fast.

## Connections

[[ipc-server]] implements the daemon side; [[ipc-client]] the attach side;
[[event-types]] defines what rides inside event pushes; [[cli-scriptworld]] renders
`StatusData` for humans. The TASK-3 Bubble Tea TUI is the intended next consumer.

## Operational notes

No authentication — trusted single-operator host, filesystem permissions on the socket
are the boundary. The protocol is debuggable with `nc -U <dir>/daemon.sock` and raw
JSON lines. Changing any field name here is a breaking wire change and must update the
contract doc in the same commit.
