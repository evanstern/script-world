package e2e

// Spec 012 (resources/food/crafting v1) quickstart.md §2: the fresh-world
// smoke, deterministic and LLM-free. A real daemon, run unattended at max
// speed for a couple of game days, must show outcrops on the generated map,
// a fire lifecycle carried entirely by the reflex (built, burned out when
// unfueled, refueled), and zero crafting-economy events — there is no
// planner in this run to ever choose a planner-only goal (quarry,
// collect_water, craft_*, build_oven, build_shelter, bathe, cook). §3
// (planner-driven progression toward SC-003) is out of scope here per
// TASK-50 Phase 9 — the orchestrator runs that separately post-merge.
//
// Empirically, "max" speed on this machine advances ~50-60k ticks/sec, so
// reaching two full game days (172800 ticks) takes low single-digit seconds
// — comfortably inside the poll deadline below, which is set generously
// wide for slower CI hardware.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
	"github.com/evanstern/script-world/internal/worldmap"
)

func TestQuickstartResourcesFoodCrafting_FreshWorldSmoke(t *testing.T) {
	// Hermetic SCRIPTWORLD_HOME (research.md D4 / TASK-49's finding): the
	// daemon registers any out-of-home world into the advisory registry on
	// boot, so — even though the world itself lives at an explicit t.TempDir()
	// path, never under the real ~/.scriptworld — this keeps that registry
	// write off the developer's real state too. daemon_e2e_test.go's other
	// tests don't yet do this (TASK-49, filed separately); this one isolates
	// itself rather than depending on that fix landing first.
	isolatedHome(t)

	const seed = "42"
	dir := newWorld(t, seed) // creates, strips llm.json (no planner), starts, and registers stopHard cleanup
	run(t, "speed", dir, "max")

	const twoGameDays = 2 * 24 * 3600
	deadline := time.Now().Add(30 * time.Second)
	var s statusJSON
	for time.Now().Before(deadline) {
		s = status(t, dir)
		if s.Clock.Tick >= twoGameDays {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if s.Clock.Tick < twoGameDays {
		t.Fatalf("world only reached tick %d in 30s at max speed, want >= %d (two game days)", s.Clock.Tick, twoGameDays)
	}
	run(t, "stop", dir)

	// --- Map data: outcrops present (SC-001/US1) -----------------------
	// worldmap.Generate is a pure function of (seed, dims); world.Open reads
	// the persisted manifest for both rather than assuming CLI defaults.
	w, err := world.Open(dir)
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	gm := w.Map()
	rockTiles := 0
	for y := 0; y < gm.H; y++ {
		for x := 0; x < gm.W; x++ {
			if gm.At(x, y) == worldmap.Rock {
				rockTiles++
			}
		}
	}
	if rockTiles == 0 {
		t.Error("no rock outcrops on the generated map (SC-001)")
	}

	// --- Event log: fire lifecycle + zero crafting-economy events -------
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	counts := map[string]int{}
	if err := st.ReplayEvents(0, func(e store.Event) error {
		counts[e.Type]++
		return nil
	}); err != nil {
		t.Fatalf("ReplayEvents: %v", err)
	}

	t.Logf("quickstart §2 smoke over %d ticks (%d game-days): rock tiles=%d, "+
		"agent.built=%d sim.fire_burned_out=%d agent.refueled=%d agent.foraged=%d "+
		"agent.hunted=%d agent.chopped=%d agent.ate=%d agent.died=%d — "+
		"planner-only events: quarried=%d collected_water=%d crafted=%d cooked=%d bathed=%d",
		s.Clock.Tick, s.Clock.Tick/(24*3600), rockTiles,
		counts["agent.built"], counts["sim.fire_burned_out"], counts["agent.refueled"],
		counts["agent.foraged"], counts["agent.hunted"], counts["agent.chopped"],
		counts["agent.ate"], counts["agent.died"],
		counts["agent.quarried"], counts["agent.collected_water"],
		counts["agent.crafted"], counts["agent.cooked"], counts["agent.bathed"])

	if counts["agent.built"] == 0 {
		t.Error("no agent.built at all in two unattended game-days — the reflex never built a fire")
	}
	if counts["sim.fire_burned_out"] == 0 {
		t.Error("no sim.fire_burned_out — a fire never went cold across two unattended game-days")
	}
	if counts["agent.refueled"] == 0 {
		t.Error("no agent.refueled — the reflex's refuel rule never fired")
	}
	// No planner ever ran (llm.json was removed), so none of the
	// planner-only goals (research R5, FR-020) can ever have been chosen.
	for _, typ := range []string{
		"agent.quarried", "agent.collected_water", "agent.crafted",
		"agent.cooked", "agent.bathed",
	} {
		if counts[typ] != 0 {
			t.Errorf("%s occurred %d times with no planner running — should be impossible", typ, counts[typ])
		}
	}
	if counts["agent.thought"] != 0 {
		t.Errorf("agent.thought occurred %d times with no planner running", counts["agent.thought"])
	}
}
