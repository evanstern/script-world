# Contract: `scriptworld calibrate`

```
scriptworld calibrate <world-dir> [--tier local|cloud|all] [--samples N]
```

(dir-first like every other subcommand; flags may also precede the dir)

- Requires `llm.json` in the world dir (no orchestrator → error explaining there is
  nothing to calibrate). The daemon may be running or stopped; calibrate opens its
  own orchestrator with an in-memory spend meter and never touches `world.db`.
- Reference workload per tier: `--samples` (default 5) calls per shape —
  `musing-1pt` (situation-sized prompt, MaxTokens 48) and `planner-3pt`
  (persona+window-sized prompt, MaxTokens 256) — fixed canned prompts, so runs are
  comparable across hosts and models.
- Cloud tier calls spend real money through the existing meter/ceiling; calibrate
  prints the estimated cost and requires `--tier cloud` or `--tier all` explicitly
  (default is `local`).
- Output: writes `calibration.json` (full-file replace) and prints a human summary:

```
tier local  (gemma4:12b-mlx @ localhost:11434)
  musing-1pt   median 17.2s   [16.1 17.8 17.2 16.9 18.0]
  planner-3pt  median 17.0s/pt [16.8 17.4 17.0 16.6 17.8]
  seconds_per_point: 17.1
  cognition at this profile: planner suppressed above 16x; musing, conversation OK at 32x
```

The final line evaluates the registry against the fresh profile at each speed — the
operator sees the cognition horizon for their hardware before ever running a world.

- Exit codes: 0 success; non-zero on any failure (unusable tier — profile not
  written for that tier; no `llm.json`; unwritable profile), with the reason on
  stderr.
- Failure mode: a tier whose provider is down is reported and skipped; an existing
  profile for a skipped tier is preserved on rewrite.
- Cloud calibration spends real money through real provider calls; the in-memory
  meter means calibrate spend is not counted against the world's monthly ceiling —
  it is the operator's explicit choice via `--tier cloud|all`, announced with the
  call count before running.
