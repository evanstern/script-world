# LLM providers & routing — operator guide

How model traffic is configured in `llm.json` (in each world's save directory), as of
spec 024 (multi-provider routing, TASK-35, PR #52), spec 025 (robustness knobs,
TASK-72), and spec 029 (metatron agency, TASK-27). Formal shapes live in
`specs/024-provider-routing/contracts/llm-config.md`; this is the operator-facing
reference.

## Is a migration required?

**No.** The legacy two-entry shape (`"local": {...}, "cloud": {...}`) loads forever and
behaves byte-identically — it is derived internally into a two-provider registry named
`local` and `cloud` with exactly the pre-024 routes. Existing worlds boot untouched,
keep their persisted spend, and their calibration profiles keep matching by name.

Two rules:

- One file cannot mix both shapes — declaring `providers` **and** `local`/`cloud`
  together is a boot error.
- Config is read at boot only: edit `llm.json`, then restart the daemon
  (`promptworld stop <world> && promptworld start <world>`).

New worlds (`promptworld new`) are written in the v2 shape with defaults semantically
identical to the old two-tier defaults.

## The v2 shape: providers + routes

```json
{
  "monthly_budget_usd": 100,
  "providers": {
    "gemma":  { "transport": "openai_compat", "endpoint": "http://localhost:11434/v1",
                "model": "gemma4:12b-mlx", "parallel": 2, "endpoint_capacity": 4 },
    "cogito": { "transport": "openai_compat", "endpoint": "http://localhost:11434/v1",
                "model": "cogito:3b", "parallel": 4, "endpoint_capacity": 4,
                "tool_mode": "json" },
    "cloud":  { "transport": "anthropic", "model": "claude-opus-4-8",
                "input_usd_per_mtok": 5, "output_usd_per_mtok": 25,
                "api_key_env": "ANTHROPIC_API_KEY" }
  },
  "routes": {
    "planner":       ["gemma"],
    "conversation":  ["cogito", "gemma"],
    "meeting":       ["cogito", "gemma"],
    "consolidation": ["cloud"],
    "narrator":      ["cloud"],
    "drama":         ["cloud"],
    "metatron":      { "chain": ["cloud"], "no_fallback": true }
  }
}
```

This example is the live-proven division of labor: high-volume structured kinds
(conversation turns, meeting flavor) on the small parallel model, prose kinds on the
quality model, the nightly/narrative tier on cloud.

### Providers

Each named entry declares one model source. Fields:

| Field | Meaning |
|-------|---------|
| `transport` | `"openai_compat"` (Ollama, LAN routers, 9router) or `"anthropic"` (official SDK) |
| `endpoint` | required for `openai_compat`; optional override for `anthropic` |
| `model` | model id at that endpoint |
| `input_usd_per_mtok` / `output_usd_per_mtok` | pricing; both 0 (or absent) = zero-priced |
| `api_key_env` | NAME of an env var holding the key (keys are never stored) |
| `api_key` | inline key — LAN-local routers only; wins over `api_key_env` |
| `parallel` | concurrent worker slots against this provider (1–16, warn-and-clamp) |
| `reasoning_effort` | hidden chain-of-thought posture; zero-priced default `"none"`, priced default omit |
| `tool_mode` | `"native"` (default) or `"json"` fallback envelope; `anthropic` transport ignores it. cogito:3b needs `"json"` — measured live (TASK-52): it never function-calls natively. Inert on non-tool kinds (conversation/meeting), so set it on the entry regardless — a future chain edit then can't trip the native-mode failure |
| `endpoint_capacity` | opt-in cross-world concurrency bound for the endpoint (see leases below) |

Every knob that used to be per-tier is now per-provider. Zero-priced providers are
"local-class": never refused for budget reasons, seeded with local-class latency
bootstraps.

### Routes: the chain IS the policy

Each call kind maps to an **ordered chain** of provider names. Membership means "meets
this kind's quality floor"; position means preference. There is no runtime scoring —
cost, latency, and quality are things **you** weigh when writing the chain.

At submission the chain is walked in order and the call goes to the first admissible
candidate. A candidate is skipped only for a mechanical, observable reason:

- `circuit-open` — its breaker is open (3 consecutive genuine failures),
- `wallet-exhausted` — the monthly ceiling is hit and this candidate is priced,
- `queue-full` / `busy` — its bounded queue is saturated.

Skips are recorded on the response (`skipped: name (reason)` in the one-shot output)
and visible in telemetry. If every candidate is inadmissible the call fails fast with
the head's reason and the sim's normal degrade paths take over (reflex layer,
conversation tolerance). Once a provider accepts a call, its failure is final — the
orchestrator never re-dispatches a failed call elsewhere.

`{"chain": ["x"], "no_fallback": true}` declares a kind that must fail rather than
substitute (chain must be a single entry).

**Continuity is automatic, not configured**: a conversation scene resolves its provider
once at scene start, and a planner/metatron tool-loop run pins its provider at run
start — no mid-scene or mid-thought model switches, including on the spec-025
in-loop retry.

### Validation

v2 configs are validated strictly at boot, and structural errors *fail the boot* with
the offender named: a route naming an undeclared provider, an accepted kind with no
route, an unknown kind key, a duplicate provider in a chain, an empty chain,
`no_fallback` with more than one entry, missing `transport`/`model`, `openai_compat`
without `endpoint`, or both shapes in one file. Tuning knobs (`parallel`, `tool_mode`,
`reasoning_effort`, `loop_max_rounds`, `max_tokens`) never fail the boot — out-of-range
values clamp with an operator warning.

### The `metatron_watch` kind (spec 029)

The angel's standing orders can be phrased fuzzily ("when Rowan seems
heartbroken…"), and confirming a fuzzy hit needs a model — but cheaply and rarely,
not through the metatron's premium conversational chain. `metatron_watch` is a
dedicated kind for exactly that: one bare yes/no call per confirm (16 tokens, no
tools, no tool loop), rate-capped per standing order so a chatty world can never
turn watching into spend. It routes and prices like any other kind — nothing about
it is a special case in the wallet or the breaker.

Default chain: `["local", "cloud"]` — cheap-first, with a reliable fallback so a
confirm still resolves when the local tier is down. Re-route it like any kind (e.g.
pin it to a small dedicated provider) by adding `"metatron_watch": [...]` under
`routes`.

**Upgrading an existing world**: a v2 `llm.json` written before this kind existed
has no route for it. Rather than failing the boot, the missing route is
backfilled from the default chain above with one boot log line naming the
backfill (`llm: route for call kind "metatron_watch" missing — backfilled from
defaults …`); add the route explicitly to silence the warning. This backfill is
scoped to kinds introduced after the v2 format shipped — a route missing for any
other kind is still a boot error, and an unknown route *key* (a typo) still fails
the boot exactly as before. Legacy (v1, two-tier) configs need no attention at
all: they resolve entirely through the same default table and pick the new kind
up for free.

## Money: one wallet, per-provider attribution

`monthly_budget_usd` remains a single global ceiling checked before any priced call.
Every call is priced by its serving provider's rates, and the month's spend is
attributed per provider (persisted; survives restarts; per-provider rows sum to the
global total). Months from before the upgrade show their total as `(unattributed)`.

## Kind-scoped knobs stay top-level

These describe the *thought class*, not the provider, and apply unchanged whichever
chain candidate serves:

- `loop_max_rounds` — tool-loop round cap (default 8, max 16).
- `max_tokens` (spec 025) — `{"planner": n, "metatron_turn": n, "consolidation": n}`;
  absent fields keep the built-in defaults (512/1024/1024), bound 4096, warn-and-clamp.

## Sharing an endpoint: `endpoint_capacity` (opt-in)

If two providers — or two *worlds* — drive the same endpoint (e.g. one Ollama serving
both gemma and cogito, or a proving world plus your own world), declare the endpoint's
true concurrent capacity on each provider that uses it. Participating daemons then
coordinate through advisory file leases (`~/.promptworld/endpoint-leases/`, flock-based,
crash-reclaimable — a killed daemon's slots free automatically), so combined load can
no longer push calls past the 2-minute worker cap and trip breakers on both sides
(the TASK-24 failure mode). A world waiting on a saturated endpoint reports
`contended` in status instead of striking its breaker. Leave the field off and
behavior is exactly as before — the mechanism only binds worlds that opt in.

## Observability

- `promptworld status <world>` (and the TUI's LLM pane) shows a per-provider table:
  name, model, endpoint, up/down, queue depth, inflight/slots, contended, spend share.
- `promptworld llm <world> <kind> "..."` (the one-shot proof path) prints the serving
  provider and any skipped candidates with reasons.
- `promptworld calibrate <world>` iterates the declared providers (each sample pinned
  to its provider) and writes one profile entry per provider name; `--provider <name>`
  narrows to one. `--tier local|cloud` still works as a deprecated alias
  (local→zero-priced, cloud→priced).

## Behavioral edges worth knowing

- A **zero-priced cloud router** is no longer budget-refused when the ceiling is hit
  (refusal keys on pricing, not tier identity). Priced providers behave as before.
- Routing chatty kinds to a small model raises empty-utterance risk; the conversation
  tolerance machinery (TASK-42: one bad utterance + one bad outcome absorbed per
  scene) is the shipped companion. Retune chains from telemetry — quality floors are
  never auto-degraded at runtime.
- Deleting `llm.json` still disables the orchestrator entirely; the world runs with
  reflex-only minds.
