# script-world

A terminal UI, open-world, top-down game on a procedurally generated map — where the
inhabitants are small **AI-programmable agents**.

## The idea

You don't play a character. You populate the world with 10–20 agents, each with its own
avatar, and each fully programmable **via AI prompting**. Give an agent a job, a
personality, a set of priorities — in plain language — and let it loose. Then watch the
world run: agents farm, build, trade, wander, argue, and improvise around each other.

Think **Dwarf Fortress** or **RimWorld**, except instead of indirect management through
menus and zones, you tweak your dwarf by talking to it. The prompt *is* the behavior.

## Core pillars

- **Terminal UI** — the whole game renders in the terminal, top-down.
- **Open world** — a procedurally generated map to explore and settle.
- **Prompt-programmed agents** — every agent's behavior is authored and re-authored
  through AI prompting; the fun is in the scripting.
- **Emergence over scripting** — the game's stories come from agents' AI-driven
  decisions colliding, not from authored quests.

## Status

Grounded and building. The design was interrogated in a Socratic grounding session
(see `docs/design/grounded-assumptions.md`) — script-world is an **ambient, always-on
world**: a daemon simulates the village 24/7; the terminal UI is an attachable client;
all player influence flows through **Metatron**, an editable intermediary agent. The
board lives in `backlog/`; specs in `specs/`.

### What runs today

The time substrate (TASK-2) and the village on top of it: a Go daemon with a
deterministic tick loop (1 tick = 1 game second), an append-only SQLite event log
with snapshots, per-world save directories, and a Unix-socket client protocol —
carrying eight villagers with needs, LLM planner minds, conversations, rumors and
debts, nightly memory consolidation, the nocturnal gru, a cloud-narrated chronicle
(the catch-up mechanism), **Metatron** (TASK-12): the player's sole influence
channel, conversing in the TUI console, mediating dreams and omens on a regenerating
charge economy, governed by `charter.md` — the game's only player-editable prompt —
and **norms and votes** (TASK-13): the village legislates itself at a daily noon
meeting, proposals and votes resolving deterministically off the relationship graph,
with the agreed law living in `village_charter.md` and witnessed violations feeding
memories, grudges, and gossip.

```sh
go build ./cmd/scriptworld

scriptworld new ~/worlds/demo --seed 42   # create a world
scriptworld start ~/worlds/demo           # detached daemon; the world now runs 24/7
scriptworld status ~/worlds/demo          # tick, game time, speed
scriptworld attach ~/worlds/demo          # watch events live; pause/resume/speed/quit
scriptworld pause ~/worlds/demo           # pause is a player verb (detaching is not)
scriptworld speed ~/worlds/demo max       # real-time up to as-fast-as-affordable
scriptworld tail ~/worlds/demo --follow   # stream the event log
scriptworld ui ~/worlds/demo              # full-screen TUI: map, chronicle, metatron, souls
scriptworld metatron ~/worlds/demo "who thrives, who struggles?"   # converse with your angel
scriptworld stop ~/worlds/demo            # graceful stop; kill -9 also resumes lossless
```

Default speed is 4x: 1 game minute per 15 real seconds; the watchable ladder tops at
32x. `go test -race ./...` covers determinism (same seed → byte-identical history),
crash recovery, the client protocol, and the model-output firewalls.
