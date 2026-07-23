# Quickstart: validating multi-provider routing

Runnable scenarios proving the feature end-to-end. Contracts:
[llm-config](contracts/llm-config.md), [status](contracts/status.md),
[endpoint-lease](contracts/endpoint-lease.md).

## Prerequisites

- `go test -race ./...` green on the branch
- For live scenarios: Ollama at `localhost:11434` with `gemma4:12b-mlx` and `cogito:3b`
  loaded (`ollama ps`)

## 1. Legacy equivalence (US1 / SC-001) — automated

```sh
go test -race ./internal/llm/ -run 'Legacy|Equivalence|Config'
```

Expected: legacy-shape config derives providers `local`+`cloud` with today's routes;
routing/admission/metering/status tests that pinned old behavior still pass unmodified
(the suite's mock-provider tests are the regression gate).

## 2. Boot validation matrix (US1 / SC-008) — automated

```sh
go test ./internal/llm/ -run 'Validation|ValidV2|LoadConfig'
```

Expected: every row of the [validation matrix](contracts/llm-config.md#load-time-validation-matrix-boot-errors)
fails loading with an error naming the offending entry.

## 3. Division of labor, live (US2 / SC-002, SC-003)

```sh
promptworld new /tmp/pw-routing-demo            # writes v2 default llm.json
# edit /tmp/pw-routing-demo/llm.json: add cogito provider; route conversation+meeting → ["cogito","gemma"]
promptworld start /tmp/pw-routing-demo
promptworld llm /tmp/pw-routing-demo --kind conversation "say hi"   # one-shot proof path
```

Expected: the one-shot response names `"provider": "cogito"`; a planner-kind call names
`gemma`; status shows both providers with their own queue/inflight/slots.

## 4. Fallback chain-walk (US3 / SC-004) — automated

```sh
go test -race ./internal/llm/ -run 'Chain|Fallback|Skip|Pin'
```

Expected: circuit-open/wallet-empty/queue-full each skip to the next candidate with the
reason recorded in `Response.Skipped`; all-inadmissible returns the head's error;
`no_fallback` refuses instead of walking; `Request.Provider` pin honors the pinned
provider's admission and never walks.

## 5. Scene pinning (US3 / SC-005) — automated

```sh
go test -race ./internal/mind/ -run 'Scene.*Provider|Pin'
```

Expected: a scene's every turn carries the same provider even when a better candidate
frees up mid-scene; killing the pinned provider mid-scene routes the failure into the
TASK-42 tolerance path (one bad outcome absorbed), never a provider switch.

## 6. One wallet, attribution (US4 / SC-007) — automated

```sh
go test -race ./internal/llm/ -run 'Meter|Attribution|Budget'
```

Expected: per-provider spend keys sum to the total; ceiling refusal skips priced
candidates but serves zero-priced ones; attribution survives store reopen.

## 7. Endpoint lease contention (US5 / SC-006)

Automated (in-process pools contend via flock):

```sh
go test -race ./internal/llm/ -run 'Lease|Contended'
```

Live reproduction of TASK-24 (optional, two terminals):

```sh
# both worlds' llm.json: same endpoint, endpoint_capacity: 4
promptworld start /tmp/pw-a && promptworld start /tmp/pw-b
# drive both at speed; then:
promptworld status /tmp/pw-a   # expect: no breaker-open thrash; contended: true while saturated
kill -9 <daemon-b pid>         # slot reclaim: world A's throughput recovers with no action
```

## 8. Operator surface (US6) — TUI

Open the TUI against the demo world: the LLM pane shows the provider table (name, model,
up, queue, inflight/slots, contended, spend) matching `promptworld status` JSON; force a
fallback (stop Ollama) and observe the skip reason on the next call's telemetry and the
provider marked down.
