package sim

import "testing"

// TestShelterPlankCost is spec 012 US5 AC#1 + T036: building a shelter now
// consumes planks (8), not raw wood — the plank economy's first
// re-costed structure.
func TestShelterPlankCost(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.Planks = shelterPlankCost
	a.Inv.Wood = 5 // untouched — shelter no longer spends wood
	site, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) })
	if !ok {
		t.Fatal("no build site reachable")
	}
	a.X, a.Y = site.X, site.Y
	a.Intent = &Intent{Goal: "build_shelter", TargetX: a.X, TargetY: a.Y, WorkStart: 1 - buildShelterTicks}

	log := driveTicks(t, s, m, 5, nil)
	var built bool
	for _, e := range log {
		if e.Type == "agent.built" {
			var p BuiltPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Kind == "shelter" {
				built = true
			}
		}
	}
	if !built {
		t.Fatal("no agent.built{shelter} event emitted")
	}
	if a.Inv.Planks != 0 {
		t.Errorf("Planks after building a shelter = %d, want 0", a.Inv.Planks)
	}
	if a.Inv.Wood != 5 {
		t.Errorf("Wood after building a shelter = %d, want 5 (unchanged — shelter no longer spends wood)", a.Inv.Wood)
	}
}

// TestShelterRestBonus is spec 012 US5 AC#2 + T037: sleeping exactly on a
// shelter tile regenerates rest at restRegenShelter (6/min) instead of
// restRegenSleep (4/min) sleeping rough — and shelters are communal (no
// ownership check).
func TestShelterRestBonus(t *testing.T) {
	rough := decayNeeds(Needs{Health: 1000, Food: 500, Rest: 500, Warmth: 500, Morale: 500}, true, false, false, false)
	sheltered := decayNeeds(Needs{Health: 1000, Food: 500, Rest: 500, Warmth: 500, Morale: 500}, true, false, false, true)
	if rough.Rest-500 != restRegenSleep {
		t.Errorf("rough-sleep rest gain = %d, want %d", rough.Rest-500, restRegenSleep)
	}
	if sheltered.Rest-500 != restRegenShelter {
		t.Errorf("sheltered rest gain = %d, want %d", sheltered.Rest-500, restRegenShelter)
	}
	if sheltered.Rest <= rough.Rest {
		t.Error("sleeping on a shelter should regenerate rest faster than sleeping rough")
	}

	// End-to-end: an asleep agent standing on a shelter tile picks up the
	// boosted rate via the executor's per-minute heartbeat (any agent — no
	// ownership).
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)
	a := &s.Agents[0]
	a.Dead = false
	a.Asleep = true
	a.Needs = Needs{Health: 1000, Food: 500, Rest: 500, Warmth: 500, Morale: 500}
	sx, sy := a.X, a.Y
	s.Structures = append(s.Structures, Structure{Kind: "shelter", X: sx, Y: sy})

	driveTicks(t, s, m, 60, nil) // one game-minute heartbeat
	if a.Needs.Rest != 500+restRegenShelter {
		t.Errorf("rest after one minute asleep on a shelter = %d, want %d", a.Needs.Rest, 500+restRegenShelter)
	}
}
