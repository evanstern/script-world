# Quickstart validation — spec 034

Prerequisites: Go 1.26.4; for live scenarios an Ollama at `localhost:11434`.
Contracts: [provider-conditions.md](contracts/provider-conditions.md),
[fresh-world-defaults.md](contracts/fresh-world-defaults.md).

## V0 — unit/integration suite

```sh
go test ./internal/llm/... ./internal/daemon/... ./internal/tui/... ./cmd/...
```

Expected: preflight probe classification, tool-silence detector thresholds,
status wire fields, default-config goldens all green.

## V1 — dead tier is loud (US1 / SC-001)

With Ollama running but WITHOUT the default model pulled (or with llm.json
edited to a bogus model):

```sh
promptworld new deadworld && promptworld start deadworld
sleep 5
grep "WARNING llm provider" ~/.promptworld/worlds/deadworld/daemon.log   # boot log surface
promptworld status deadworld            # WARNING line names provider/model/endpoint/remedy
promptworld status deadworld --json | jq '.llm.providers[] | {name, condition, condition_remedy}'
promptworld attach deadworld            # TUI: red [llm: …] header badge
```

Then `ollama pull cogito:3b`, wait ≤60s (re-probe) — warning clears from
status/TUI without restart; a `daemon.llm_warning` event with `active:false`
appears in the event stream.

## V2 — endpoint unreachable (US1 scenario 4)

Stop Ollama, start the world: same surfaces show the distinct
`endpoint-unreachable` wording; world boots and runs regardless.

## V3 — tool-silent provider (US2 / SC-005)

Edit llm.json: a pulled model known to never function-call natively (e.g.
`cogito:3b`) with `tool_mode` removed (native). Start, unpause, let villagers
plan for a few minutes:

```sh
promptworld status toolsilent   # WARNING … suggests tool_mode "json"
```

Fix `tool_mode: "json"`, restart: warning never fires (SC-003 negative check
rides any healthy soak).

## V4 — fresh world works out of the box (US3 / SC-002)

On a machine with `cogito:3b` pulled and nothing else:

```sh
promptworld new freshworld && promptworld start freshworld
# new's output names cogito:3b and the pull command
# after a few sim-minutes:
sqlite3 ~/.promptworld/worlds/freshworld/world.db \
  "select count(*) from events where type='cog.tool_call'"   # > 0, no config edits
```

(Event-log table/query per existing store layout; any equivalent evidence of
landed planner tool calls passes.)

## V5 — docs alignment (SC-004)

```sh
grep -n "cogito:3b" internal/llm/config.go docs/llm-providers.md README.md
grep -n "gemma4:12b-mlx" internal/llm/config.go README.md   # gone from default/README-default contexts
```

Default model + tool mode identical across code and both docs.
