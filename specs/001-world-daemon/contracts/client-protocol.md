# Contract: client attach/detach protocol

Transport: Unix domain socket `<savedir>/daemon.sock`. Framing: newline-delimited JSON
(one JSON object per line, UTF-8, no pretty-printing). Multiple concurrent clients
allowed; all equal (trusted localhost operator).

## Requests (client → daemon)

```json
{"id": 1, "cmd": "<name>", "args": { }}
```

`id` is client-chosen, echoed in the response; monotonically increasing per connection.

| cmd | args | effect |
|---|---|---|
| `status` | — | snapshot of clock + world status |
| `state` | — | full canonical world-state JSON + the `last_seq` it reflects; subscribe with `since: last_seq` to maintain a gapless live replica (added for the TASK-3 TUI) |
| `subscribe` | `{"since": <seq>?}` | start event pushes; with `since`, first replays log events after that seq, then goes live (gapless) |
| `unsubscribe` | — | stop pushes |
| `pause` | — | pause game time (idempotent) |
| `resume` | — | resume game time (idempotent) |
| `set_speed` | `{"speed": "1x"\|"4x"\|"8x"\|"16x"\|"max"}` | change requested rate |
| `shutdown` | — | graceful daemon stop (final snapshot; connection then closes) |

## Responses (daemon → client)

```json
{"id": 1, "ok": true,  "data": { }}
{"id": 1, "ok": false, "error": "human-readable message"}
```

`status` / `pause` / `resume` / `set_speed` all return the same `data` shape:

```json
{
  "world":   {"name": "…", "seed": 1234567, "format_version": 1},
  "clock":   {"tick": 86400, "game_time": "day 2 06:00", "paused": false,
              "speed": "4x", "effective_rate": 4.0, "degraded": false},
  "daemon":  {"pid": 4242, "uptime_seconds": 3600, "subscribers": 1},
  "log":     {"last_seq": 90210}
}
```

## Pushes (daemon → client, after `subscribe`)

```json
{"push": "event", "event": {"seq": 90211, "tick": 86401, "game_time": "day 2 06:00",
                            "type": "agent.moved", "payload": { }}}
```

Ordering: pushes arrive in `seq` order with no gaps for the lifetime of a
subscription. Responses and pushes may interleave on the wire; clients demux on the
presence of `id` vs `push`.

## Failure semantics

- Unknown `cmd` / malformed args → `ok:false` response; connection stays open.
- Malformed JSON line → connection closed (protocol error).
- Slow subscriber whose push buffer (1024 events) overflows → daemon sends
  `{"push":"dropped","last_seq":N}` and cancels the subscription; the client re-syncs
  with `subscribe {since: N}`. The sim loop never blocks on a client.
- Abrupt client disconnect → session cleaned up silently; zero effect on the sim
  (FR-011).
- Daemon not running → connect fails at the socket level; CLI surfaces "daemon not
  running" fast (edge case: no hang).
