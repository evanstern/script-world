# Contract: Metatron console (IPC + CLI + TUI)

## IPC request: `metatron_chat`

Synchronous request over the existing JSON-lines protocol (long-call pattern precedented
by `llm_call`; client read deadline extended for this request).

**Request**

```json
{"id": 7, "cmd": "metatron_chat", "args": {"text": "<player message, ≤ 2000 chars>"}}
```

**Response (success)**

```json
{"id": 7, "ok": true, "data": {
  "reply": "<Metatron's say, charter-voiced>",
  "nudge": {"form": "dream", "targets": ["Fern"], "text": "<rendering>"} ,
  "charges": 2,
  "moments": ["day 14 03:12 — Rowan was attacked by the gru"]
}}
```

- `nudge` is `null` when the turn was conversation/counsel/refusal (charge unchanged).
- `moments` lists the queued moments this reply surfaced (empty when none).
- `charges` is the post-turn bank.

**Response (failure)** — protocol `ok:false` with `error` string:

| Condition | Error behavior |
|---|---|
| No LLM config on the world | `"metatron is not present in this world (no llm config)"` |
| Cloud tier down / budget exhausted | honest reason from the orchestrator error; no charge lost |
| Turn already in flight | `"the angel is attending another matter"` |
| Empty/oversized `text` | rejected before any model call |
| Unusable model output | `ok:true` with an apologetic `reply`, `nudge:null` — a safe turn, not a protocol error |

## CLI: `scriptworld metatron <dir> <message…>`

One-shot console turn against the running daemon (the proof path for tests/scripts).
Prints the reply, any landed nudge line (`⚡ dream → Fern: …`), surfaced moments, and the
charge bank. Exit non-zero on protocol failure. With no message argument: prints charges
+ last soul.md entries (a status peek, no model call).

## TUI: metatron pane = the console

- Transcript viewport (session turns; `metatron/transcript.md` holds durable history)
  + input line; charges shown as `⚡⚡⚡`/`⚡⚡·` in the pane header; tier health + spend
  retained from the current pane.
- Key contract while pane 3 is active: printable keys → input; Enter → send (input
  disabled while a turn is in flight, spinner shown); Esc → return to map pane; pane
  footer documents this. Global keys (`1-4`, `tab`, space, `[`/`]`, `q`) apply only
  while the input is empty, so typing is never hijacked.

## Turn semantics (all surfaces)

- One player prompt = at most one mediated turn; turns serialized (single-flight).
- Conversation/counsel/refusal are free; exactly landed nudges cost 1 charge.
- The player's text reaches only Metatron's prompt (structural firewall — see
  research R5); villagers can only ever receive `nudge.text`.
- Charter is re-read at each turn start; edits are live next turn, no restart.
- Queued moments are surfaced at the start of the next reply, oldest first.
