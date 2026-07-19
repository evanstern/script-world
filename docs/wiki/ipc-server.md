---
name: ipc-server
description: Daemon-side sessions — gapless subscribe with store gap-fill, bounded push buffers that drop subscriptions rather than block the loop, long-path socket workaround
kind: component
sources:
  - internal/ipc/server.go
  - internal/ipc/socket.go
verified_against: cee600e086a1be15868205c16c395ee33aaa397e
---

# IPC server

`ipc.Server` hosts the protocol for one world. Its governing invariant is FR-011:
session lifecycles are fully decoupled from the sim — a client can die mid-write,
spam garbage, or subscribe and stall, and the loop never notices.

## How it works

`Serve` accepts connections; each `session` runs its own reader goroutine with a
line-scanner (1 MiB max line). Malformed JSON closes that connection; unknown
commands return `ok:false` and keep it open. Time-control and status commands go
through `Loop.Do` and reply with the full `StatusData` (built by `statusData`, which
adds world/daemon/log sections around the loop's clock snapshot); `state` goes
through `Loop.DoState` and replies with `StateData` — the canonical world-state JSON
plus the `last_seq` it reflects. `llm_call` submits to the optional
[[llm-orchestrator]] (`SetLLM`; 2-minute timeout per call) — a slow or dead model
blocks only the calling session, never the loop; `statusDataFull` appends the
orchestrator's snapshot to status responses.

**Broadcast path**: the loop's notify callback is `Server.Broadcast`, which offers
committed events to each session under a non-blocking send into a
`pushBufferSize = 1024` channel. On overflow the subscription is canceled and a
`{"push":"dropped","last_seq":N}` is sent from a fresh goroutine — the loop is never
blocked by a slow client.

**Gapless delivery**: each subscription runs a pusher goroutine with a `cursor`. It
first fills from the store up to the log head at subscribe time (`subscribe{since}`
replay), then consumes the live channel; any seq jump ahead of `cursor+1` triggers a
store gap-fill (`EventsSince`) before delivery. This closes the race between opening
the live buffer and reading history — events are always delivered in seq order with
no gaps for the life of a subscription.

**Long socket paths** (`socket.go`): `sockaddr_un` caps paths (~104 bytes on darwin).
`listenUnix`/`dialUnix` transparently chdir into the socket's directory and use its
basename when the path exceeds `maxSockPath = 100`, serialized under a mutex with cwd
restored immediately — save directories can live at any depth.

`shutdown` replies ok, then invokes the daemon's cancel function. `Close` unwinds the
listener and every session and removes the socket file.

## Connections

[[sim-loop]] feeds `Broadcast` and receives `Do` calls; [[event-log]] backs replay and
gap-fill; [[ipc-protocol]] defines the wire shapes; [[daemon-lifecycle]] constructs the
server (with `SetLoop` breaking the mutual reference) and calls `Close` on exit.

## Operational notes

Writes carry a 10 s deadline; a dead client's connection is closed and its reader
unwinds. Multiple concurrent clients are allowed and equal. Subscriber count in status
counts subscribed sessions only, not mere connections.
