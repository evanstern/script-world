# script-world grounding wiki

Code-grounded corpus for the script-world daemon substrate. Every note is pinned to the
commit it was verified against; a change to any file in a note's `sources:` invalidates
that note (re-pin with `/grounding-wiki:wiki-update`).

## Orientation

- [[overview]] — the system's shape: always-on daemon, attachable clients, event-sourced world
- [[design-grounding]] — the TASK-1 grounded assumptions the code implements

## Time & simulation

- [[game-clock]] — 1 tick = 1 game second; speeds 1x–max; epoch day 1 06:00
- [[sim-loop]] — single-goroutine fixed-timestep loop; command intents; auto-slow
- [[sim-state-reducer]] — State + Apply: the single mutation path, live and replay
- [[deterministic-rng]] — per-decision PCG from (seed, purpose, tick, index); no RNG state
- [[executor]] — agent bodies: needs, intents, death, terrain overlays
- [[reflex-policy]] — survival decision ladder + deterministic BFS pathfinding
- [[event-types]] — the event taxonomy and payload shapes

## Persistence

- [[world-save-directory]] — one dir = one run; manifest, layout, separability
- [[worldmap-generation]] — seeded terrain (water/woods/forage/dens); regenerated, never stored
- [[event-log]] — append-only SQLite events table; seq contiguity; source of truth
- [[snapshots]] — hash-verified recovery accelerator; cadence and fallback chain

## Interface

- [[ipc-protocol]] — JSON-lines over UDS: requests, responses, pushes, status shape
- [[ipc-server]] — sessions, gapless subscribe-replay, overflow drop, long-path sockets
- [[ipc-client]] — dial, request correlation, push demux
- [[tui-client]] — Bubble Tea four-pane client over a live log-shipped replica
- [[cli-scriptworld]] — the single binary's subcommands and exit discipline

## Inference & minds

- [[llm-orchestrator]] — two-tier model traffic: routing, spend ceiling, degraded mode
- [[agent-mind]] — personas, souls, memory window, and the planner driver
- [[social-fabric]] — relationships, rumors, debts, secrets, conversations

## Lifecycle & quality

- [[daemon-lifecycle]] — recovery, pidfile, meta validation, signals, shutdown
- [[testing-strategy]] — determinism harness, integration, binary-level e2e scenarios
