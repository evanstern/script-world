# Quickstart Validation: Decision-Trace View

Prerequisites: repo builds (`go build ./...`), a world directory with LLM tiers
configured (local tier is enough), terminal ≥ 112 columns for the widescreen layout.

## 1. Unit-level proof (no world needed)

```sh
go test ./internal/tui/
```

Must pass, including the new tests:
- projection ingest joins/bounds/fragments (contract §1)
- glossary sweep over toolloop verdicts + sim outcomes (R16)
- decisions sub-view rendering + key routing (R7–R11)
- metatron inline verdict rows (R12–R14)
- existing digest catalog sweep unchanged (R18)

## 2. Live decisions sub-view (User Story 1)

```sh
promptworld daemon <world-dir>     # terminal A
promptworld ui <world-dir>        # terminal B
```

1. Let the world run until villagers think (watch `cog.*` lines in the raw feed: `2`
   to select chronicle, `r` for raw).
2. `4` (villagers tab) → select a villager that has acted → `⏎` (detail) → `d`
   (decisions).
3. Verify chains render most-recent-first: stimulus line, thought class, tool calls
   with plain-language verdicts + reasons, terminal outcome; `j`/`k` scroll; `esc`
   returns to detail, `esc` again to roster.
4. Verify at least one rejected call reads as plain language (no raw
   `rejected_*` string anywhere on screen).

## 3. Ring-eviction survival (SC-003)

Run at high speed (`]` to 32x) until well over 500 events accumulate, then open a
villager's decisions sub-view: chains older than the raw feed's oldest visible entry
must still render (stimulus text included).

## 4. Metatron inline verdicts (User Story 2)

1. `m` to focus the minibuffer; ask Metatron for something tool-shaped (e.g. a miracle:
   "light a fire near the camp") and something it should refuse.
2. `3` (metatron tab): each turn shows its tool calls inline — tool name +
   plain-language verdict (+ reason on refusals) — before the angel's reply row; a
   prose-only answer adds no verdict rows.

## 5. Fragments and reconnect (edge cases)

- Attach the UI mid-run (villagers already thinking): fragmentary chains (missed
  thought) must render with what is known, attributed via the job-ID parse.
- Kill and restart the daemon (or force a reconnect): the decisions view rebuilds
  from the new subscription onward — empty is acceptable, corruption is not.
