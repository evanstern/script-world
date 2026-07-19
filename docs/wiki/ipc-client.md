---
name: ipc-client
description: Attach-side protocol client — dial with fast failure, request/response correlation by id, push demux channel
kind: component
sources:
  - internal/ipc/client.go
verified_against: 08d8c70e23c104a4c61df1749c00cb315f5c643d
---

# IPC client

`ipc.Client` is the attach side of the protocol, used by every CLI subcommand that
talks to a live daemon, and intended as the transport for the TASK-3 TUI.

## How it works

`Dial(sockPath)` connects with a 2-second timeout via the long-path-safe `dialUnix`
([[ipc-server]] describes the workaround); failure is wrapped as "daemon not running"
so callers surface the offline case fast instead of hanging.

One reader goroutine decodes `wireMsg` lines and demuxes:

- messages with `id` resolve the matching entry in the `pending` map (each `Call`
  registers a buffered reply channel keyed by a monotonically increasing id);
- messages with `push` go to the `pushes` channel (buffer 1024), read via `Pushes()`.

`Call(cmd, args)` marshals, writes one line (10 s write deadline), and blocks on its
reply channel; `ok:false` responses surface as errors. On connection death the reader
closes all pending channels and `pushes`, so both callers and push consumers unwind
deterministically.

Conveniences: `Status(cmd, args)` unmarshals the shared `StatusData` shape;
`Subscribe(since *int64)` issues the subscribe command — read events from `Pushes()`
and handle the `"dropped"` push by re-subscribing from its `last_seq`.

## Connections

[[ipc-protocol]] defines the wire; [[cli-scriptworld]]'s `status`/`attach`/`tail`/
time-control commands are the current callers; [[ipc-server]] is the peer.

## Operational notes

The client is safe for one concurrent `Call` per goroutine plus one push consumer;
`Call` correlation allows interleaved requests. A full `pushes` buffer would backpressure
the reader — consumers should drain promptly (the CLI streams straight to stdout).
