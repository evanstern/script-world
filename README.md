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

Pre-design. The first task on the board is a Socratic Q/A session to ground the
assumptions above and produce a real first-pass task list. See `backlog/` for the
board and `.specify/` for spec-driven development artifacts.
