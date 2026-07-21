# Quickstart: World Instance Manager — validation walkthrough

**Feature**: `008-instance-manager`. Proves the user stories end-to-end against a real
build. Contracts: [contracts/cli.md](contracts/cli.md); states/resolution:
[data-model.md](data-model.md).

## Prerequisites

```bash
go build -o /tmp/sw ./cmd/scriptworld
export SCRIPTWORLD_HOME=$(mktemp -d)   # isolated home so the walkthrough is hermetic
```

## US2 — create and address by name (P2)

```bash
/tmp/sw new aria                        # → creates $SCRIPTWORLD_HOME/worlds/aria
test -f "$SCRIPTWORLD_HOME/worlds/aria/world.json" && echo OK
/tmp/sw new aria                        # → exit 1, "already exists", world untouched
/tmp/sw new 'bad/name'; /tmp/sw new -- '-flag'   # → exit 1, validation messages
/tmp/sw start aria                      # name resolves from any CWD
/tmp/sw status aria                     # live status by name
/tmp/sw speed aria 8x && /tmp/sw pause aria && /tmp/sw resume aria
/tmp/sw stop aria && /tmp/sw stop aria  # second stop: "daemon not running", exit 0
```

## US1 — see everything running (P1)

```bash
/tmp/sw new harbor && /tmp/sw start harbor
mkdir -p /tmp/elsewhere && /tmp/sw new /tmp/elsewhere/custom --name custom
/tmp/sw start /tmp/elsewhere/custom     # path form — legacy behavior
cd / && /tmp/sw ps                      # both listed: name, state, pid, tick, game time, speed, LLM
/tmp/sw ps --json | jq 'length'        # → 2, machine-readable (FR-014)

# stale-record honesty (FR-002): SIGKILL one daemon, ps must not show it running
kill -9 "$(cat "$SCRIPTWORLD_HOME/worlds/harbor/daemon.pid")"
/tmp/sw ps                              # harbor absent (or --all: stopped) — never "running"
/tmp/sw ps --all                        # stopped worlds too, with last-known tick/time
/tmp/sw stop custom
/tmp/sw ps                              # → "no worlds running", exit 0
```

## US3 — custom-path worlds by name (P3)

```bash
/tmp/sw ps --all                        # "custom" appears by name (registered at start)
/tmp/sw start custom && /tmp/sw stop custom   # name now addresses the custom-path world
rm -rf /tmp/elsewhere/custom
/tmp/sw ps --all                        # custom shown as missing (or omitted) — never errors
/tmp/sw status custom                   # exit 1, helpful "not found/missing" error

# ambiguity: same name in home and registry
/tmp/sw new clash && mkdir -p /tmp/dup && /tmp/sw new /tmp/dup/clash --name clash
/tmp/sw start /tmp/dup/clash && /tmp/sw stop /tmp/dup/clash    # registers "clash" → /tmp/dup/clash
/tmp/sw status clash                    # exit 1: ambiguous, both paths listed
```

## Backward compatibility + self-containment (SC-003, SC-004)

```bash
go test ./... && go test ./e2e/         # full suite incl. pre-existing path-based e2e
cp -R "$SCRIPTWORLD_HOME/worlds/aria" /tmp/aria-copy
SCRIPTWORLD_HOME=$(mktemp -d) /tmp/sw start /tmp/aria-copy    # runs with zero manager state
SCRIPTWORLD_HOME=$(mktemp -d) /tmp/sw stop /tmp/aria-copy
```

## Expected outcomes checklist

- [ ] `ps` from any CWD lists every running world with name/state/pid/tick/time/speed/LLM, exit 0
- [ ] SIGKILLed daemon never shows as running (stale pidfile swept/ignored)
- [ ] `ps` with nothing running prints the empty message, exit 0, in < 2s
- [ ] `new <name>` creates under the worlds home; duplicate name refused untouched
- [ ] every per-world command works with a name from any directory
- [ ] path invocations behave exactly as before (full old suite green)
- [ ] copied world dir runs on a fresh `SCRIPTWORLD_HOME` with no registry present
- [ ] ambiguous name refused with both candidate paths printed
