# Interface Contracts: Metatron Miracles

Three doors, one vocabulary. The event payloads in [data-model.md](../data-model.md) are
the replay contract; this file covers the caller-facing surfaces.

## 1. Angel turn contract (model output, `internal/metatron/turn.go`)

`turnReply` gains an optional `miracle` member; at most one of `nudge` / `miracle` per
turn (existing "one mediated act" rule):

```json
{
  "say": "<charter-voiced reply>",
  "nudge": { ... } | null,
  "miracle": {
    "kind": "time_snap" | "give_item" | "move" | "remove",

    // time_snap
    "day": 2, "time": "11:30",

    // give_item
    "villager": "Ash", "item": "food_raw", "qty": 2,

    // move / remove
    "class": "villager" | "structure" | "pile" | "terrain",
    "x": 44, "y": 8,
    "to_x": 45, "to_y": 32          // move only
  } | null
}
```

Contract rules:

- **No gratis field exists in this schema.** Unknown members (including `"gratis"`) are
  dropped at unmarshal — structural stripping (FR-007, SC-005).
- The system prompt documents the cost table (snap 2; others 1) and instructs the angel
  to refuse in-fiction when the bank is insufficient.
- `landMiracle` validation failures do not consume charges and are reported in the reply
  suffix, exactly like `landNudge` ("(No miracle landed: <why>)").
- Villagers are named by name; the door resolves names to indices (`agentIndexByName`).

## 2. IPC command (`internal/ipc`)

Request:

```json
{"id": 7, "cmd": "miracle", "args": {
  "kind": "time_snap" | "give_item" | "move" | "remove",
  "day": 2, "time": "11:30",
  "villager": "Ash", "item": "food_raw", "qty": 2,
  "class": "villager", "x": 44, "y": 8, "to_x": 45, "to_y": 32,
  "gratis": false
}}
```

Response (OK): `{"id": 7, "ok": true, "data": {"kind": "...", "charges": <bank after>,
"gratis": <bool>, "summary": "<one-line human rendering>"}}`

Response (rejected): `{"id": 7, "ok": false, "error": "<door/reducer reason>"}`

Contract rules:

- **This is the ONLY surface that accepts `gratis`.** The server passes it into the
  payload verbatim; the angel path has no equivalent input.
- Available without an LLM configured (pure-sim worlds included) — the handler needs
  only the sim loop, not `srv.llm` / `srv.metatron`.
- The handler and `landMiracle` share one batch-builder (miracle event + FR-018 memory
  events) so the two doors cannot drift.
- Day/HH:MM → tick conversion uses `internal/clock`; a target ≤ current tick errors.

## 3. CLI (`cmd/promptworld/main.go`)

```
promptworld miracle <world> snap-time <day> <HH:MM>        [--force]
promptworld miracle <world> give <villager> <item> <qty>   [--force]
promptworld miracle <world> move <class> <x,y> <x1,y1>     [--force]
promptworld miracle <world> remove <class> <x,y>           [--force]
```

- `--force` sets `gratis: true`; otherwise the console spends charges like the angel.
- `<class>` ∈ `villager|structure|pile|terrain` (`terrain` valid for remove only;
  `villager` invalid for remove — the CLI surfaces the reducer's rejection).
- Exit 0 on landed miracle (prints the summary + remaining charges); exit 1 with the
  rejection reason otherwise.
- Usage text lands in the top-level help block alongside the existing subcommands.

## 4. Whitelist delta (`internal/sim/loop.go`)

`injectSocialWhitelist` gains exactly four entries: `metatron.time_snapped`,
`metatron.item_granted`, `metatron.entity_moved`, `metatron.entity_removed`.
Nothing else about the isolation boundary changes.
