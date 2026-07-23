# Quickstart Validation: Grounded Memories & Agent Journal

Runnable scenarios proving the feature end-to-end. Contracts: [memory-context.md](./contracts/memory-context.md), [journal-tools.md](./contracts/journal-tools.md); shapes: [data-model.md](./data-model.md).

## Prerequisites

- Go 1.26; repo at the feature branch (worktree `.worktrees/task-16`)
- For live scenarios: a local model reachable per `llm.json` (Ollama/9router local tier)

## 1. Gates & unit surface (no model needed)

```sh
go build ./... && go vet ./...
go test ./internal/tool/ ./internal/sim/ ./internal/mind/ ./internal/scribe/
```

**Expect**: boot coverage gates pass with the four new tools (two Expressive tools' events on the whitelist); reducer tests cover budget rejection, delete-unknown-id rejection, id stability; template grammar tests pin situated texts; scribe tests pin soul.md/journal.md renders (including a pre-019-shaped memory rendering byte-identically to the old format).

## 2. Determinism / replay (no model needed)

```sh
go test ./... -run 'Replay|Determinism'
```

**Expect**: the extended fixture (situated memories with reasons, a conversation, journal writes + a delete + a rejected over-budget write) replays to identical `State`, byte-identical soul.md and journal.md, zero orchestrator calls (SC-003); the pre-019 fixture log replays unchanged (SC-007).

## 3. Live smoke — situated memories (SC-001, SC-006)

```sh
promptworld new /tmp/pw-019 && promptworld run /tmp/pw-019 &
# let agents act (build/forage/talk), then:
cat /tmp/pw-019/agents/*/soul.md
```

**Expect**: new memory lines carry `· at <desc> (x,y)`; planner-driven actions also carry `· why: <reason>`; reflex actions show place but never a why; conversation memories show `[conv <id>]`.

## 4. Live smoke — transcript retrieval (SC-002)

Take any `[conv <id>]` from a soul; query the event log:

```sh
sqlite3 /tmp/pw-019/events.db \
  "select json_extract(payload,'$.speaker'), json_extract(payload,'$.text')
   from events where type='social.conversation_turn'
   and json_extract(payload,'$.conv')=<id> order by seq"
```

**Expect**: the full ordered dialogue, speakers + verbatim text — from the log alone.

## 5. Live smoke — journal (SC-004, SC-005)

Run a world long enough for planner cognitions; watch for journal tool calls:

```sh
sqlite3 /tmp/pw-019/events.db \
  "select json_extract(payload,'$.tool'), json_extract(payload,'$.verdict')
   from events where type='cog.tool_call' and json_extract(payload,'$.tool') like '%journal%'"
cat /tmp/pw-019/agents/*/journal.md
```

**Expect**: `write_journal_entry` calls with `landed` verdicts and journal.md files growing agent-authored entries; any over-budget attempt shows `rejected_gate` with the budget reason and an unchanged journal. (Whether/how agents journal is the experiment — the contract only requires the tools work when called; a scripted-driver unit test covers the full write→search→read→delete cycle deterministically.)

## 6. Restart & replay survival

```sh
# stop the daemon, restart on the same world dir
promptworld run /tmp/pw-019
```

**Expect**: souls and journals intact after restart (regenerated views over the log); no model calls during catch-up replay.

## Sign-off map

| Scenario | Proves |
|----------|--------|
| 1 | FR-004/009/010/013 mechanics, boot gates |
| 2 | FR-007/011/014 — SC-003, SC-007 |
| 3 | FR-001/002/006 — SC-001, SC-006 |
| 4 | FR-003/005 — SC-002 |
| 5 | FR-008/009/010/012 — SC-004, SC-005 |
| 6 | FR-007/008 durability |
