---
name: ipc-client
description: Attach-side protocol client — dial with fast failure, request/response correlation by id, push demux channel
kind: component
sources:
  - internal/ipc/client.go
verified_against: 65898835d02ec199456eb656ad9187aca3346fbf
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

The reader's scanner is sized to the protocol's reply ceiling (`maxReplyBytes`,
64 MiB — TASK-19), matching the bound the server enforces, so multi-MiB `state`
replies round-trip.

`Call(cmd, args)` marshals, writes one line (10 s write deadline), and blocks on its
reply channel; `ok:false` responses surface as errors. On connection death the reader
closes all pending channels and `pushes`, so both callers and push consumers unwind
deterministically.

**Fatal-reply classification** (TASK-19): the exported sentinel
`ErrReplyTooLarge` marks failures reconnecting cannot fix. `Call` wraps
server-refused oversized replies in it (recognized by the `replyTooLargePrefix`
on the error string), and the read loop maps a raw `bufio.ErrTooLong` (only
possible against a version-skewed daemon) into the same sentinel with an
actionable message. Callers check `errors.Is(err, ipc.ErrReplyTooLarge)` to
fail fast instead of retrying — the [[tui-client]] quits on it.

Conveniences: `Status(cmd, args)` unmarshals the shared `StatusData` shape;
`FetchState()` unmarshals the `state` command's `StateData` (full world state + the
log position it reflects); `Subscribe(since *int64)` issues the subscribe command —
read events from `Pushes()` and handle the `"dropped"` push by re-subscribing from
its `last_seq`; `MetatronChat(text)` / `MetatronStatus()` carry the console pair
([[metatron]]) — `MetatronChat` blocks for the angel's cloud round-trip.

## Connections

[[ipc-protocol]] defines the wire; [[cli-promptworld]]'s `status`/`attach`/`tail`/
time-control commands and the [[tui-client]] are the callers; [[ipc-server]] is the
peer.

## Operational notes

The client is safe for one concurrent `Call` per goroutine plus one push consumer;
`Call` correlation allows interleaved requests. A full `pushes` buffer would backpressure
the reader — consumers should drain promptly (the CLI streams straight to stdout).
