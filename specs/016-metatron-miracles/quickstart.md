# Quickstart Validation: Metatron Miracles

Runnable end-to-end scenarios proving the feature. Contracts:
[contracts/interfaces.md](contracts/interfaces.md); payload rules:
[data-model.md](data-model.md).

## Prerequisites

```sh
go build ./... && go test ./internal/sim ./internal/ipc ./internal/metatron
promptworld new miracle-qa            # throwaway world (pure-sim is fine)
promptworld start miracle-qa && promptworld pause miracle-qa
```

Genesis bank is 1 charge; regen is 1 per 6 game-hours (cap 3).

## Scenario A — rescue move (US1) + charge spend

```sh
promptworld miracle miracle-qa move villager 44,8 45,32
```

Expect: success line with remaining charges (bank − 1); `promptworld status` unchanged
otherwise. Verify via the state IPC/TUI: the villager stands at (45,32), their intent is
cleared, and they carry a new memory of being moved. A second identical call with an
empty bank must fail with a charge rejection.

Negative: `move villager 44,8 47,33` (impassable rock) → rejected, reason printed, no
charge spent. `remove villager 45,32` → rejected (v1 doctrine).

## Scenario B — gratis force door (US2)

With the bank at 0:

```sh
promptworld miracle miracle-qa give Ash food_raw 2 --force
```

Expect: success; bank still 0; event log shows `metatron.item_granted` with
`"gratis":true`; Ash's inventory +2 raw food; Ash has a grant memory.

Negative (validation survives gratis): `give Ash wood 999 --force` → rejected whole
(bulk cap), nothing delivered.

Adversarial (SC-005, in tests not CLI): a crafted model reply containing
`"miracle": {..., "gratis": true}` lands a *charged* event — covered by
`internal/metatron` unit test.

## Scenario C — time snap (US3)

Note the current game time from `promptworld status`, then:

```sh
promptworld miracle miracle-qa snap-time 2 11:30        # costs 2; --force if bank < 2
promptworld status miracle-qa
```

Expect: clock reads day 2, 11:30; no villager moved/ate/spoke for the skipped interval;
a lit fire retains its pre-snap remaining burn; the bank gained nothing from skipped
regen boundaries; every living villager carries a snap memory.

Negative: `snap-time 1 06:00` (past) → rejected, forward-only.

Drift proof (SC-003) runs in `go test ./internal/sim -run Drift`: whole-day snap vs
control, identical behavior modulo offset; per-field remaining-duration assertions for
arbitrary deltas.

## Scenario D — remove with spill

Build state with a stocked chest (or use a seeded test world), then:

```sh
promptworld miracle miracle-qa remove structure <x,y>
```

Expect: chest gone; a ground pile at (x,y) holding its former contents (nothing
silently destroyed).

## Scenario E — recovery replay (SC-002)

After scenarios A–D:

```sh
promptworld stop miracle-qa && promptworld start miracle-qa
```

Expect: daemon recovers cleanly (snapshot + replay applies the miracle events); state
matches pre-restart exactly — villager positions, inventories, clock, bank. The replay
byte-identity test (`go test ./internal/sim -run Miracle`) proves the same from genesis.

## Scenario F — angel-mediated miracle (US1 via fiction; needs llm.json)

```sh
promptworld metatron miracle-qa "please move Ash next to Cedar"
```

Expect: charter-voiced reply; on acceptance, the same `metatron.entity_moved` event as
Scenario A, charge spent; on an out-of-charges bank, an in-fiction refusal and no event.

## Cleanup

```sh
promptworld stop miracle-qa && rm -rf ~/.promptworld/worlds/miracle-qa
```
