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

### What runs today: the world daemon (TASK-2)

The time substrate is real: a Go daemon with a deterministic tick loop (1 tick =
1 game second), an append-only SQLite event log with snapshots, per-world save
directories, and a Unix-socket client protocol.

```sh
go build ./cmd/scriptworld

scriptworld new ~/worlds/demo --seed 42   # create a world
scriptworld start ~/worlds/demo           # detached daemon; the world now runs 24/7
scriptworld status ~/worlds/demo          # tick, game time, speed
scriptworld attach ~/worlds/demo          # watch events live; pause/resume/speed/quit
scriptworld pause ~/worlds/demo           # pause is a player verb (detaching is not)
scriptworld speed ~/worlds/demo max       # real-time up to as-fast-as-affordable
scriptworld tail ~/worlds/demo --follow   # stream the event log
scriptworld stop ~/worlds/demo            # graceful stop; kill -9 also resumes lossless
```

Default speed is 4x: 1 game minute per 15 real seconds. Two placeholder wanderers
generate events until the real village systems land (TASK-4+). `go test -race ./...`
covers determinism (same seed → byte-identical history), crash recovery, and the
client protocol.
