package sim

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/clock"
)

// TestGruNocturnalOnly is AC#1's frame: the gru exists only between nightfall
// and dawn. Driven from genesis for two full days, every gru.* event must
// land inside night, emergence must match the seeded per-night roll, and the
// gru must be gone (state nil) whenever it is day.
func TestGruNocturnalOnly(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	log := driveTicks(t, s, m, 2*86400, nil)

	emerged := 0
	for _, e := range log {
		switch e.Type {
		case "gru.emerged":
			emerged++
			if sec := clock.SecondOfDay(e.Tick); sec != nightStartSecond {
				t.Errorf("gru.emerged at second-of-day %d, want %d", sec, nightStartSecond)
			}
		case "gru.withdrew":
			if sec := clock.SecondOfDay(e.Tick); sec != dayStartSecond {
				t.Errorf("gru.withdrew at second-of-day %d, want %d", sec, dayStartSecond)
			}
		case "gru.moved", "gru.sighted", "gru.attacked":
			sec := clock.SecondOfDay(e.Tick)
			if sec < nightStartSecond && sec >= dayStartSecond {
				t.Errorf("%s at second-of-day %d — daytime", e.Type, sec)
			}
		}
	}
	// The roll is a pure function of (seed, night): the log must agree.
	want := 0
	for night := int64(1); night <= 2; night++ {
		if rngAt(uint64(seed), "gru-emerge", night, 0).Uint64N(1000) < gruEmergePerMille {
			want++
		}
	}
	if emerged != want {
		t.Errorf("emerged %d nights, seeded roll says %d", emerged, want)
	}
	if want == 0 {
		t.Fatal("seed 42 never rolls an emergence in 2 nights — pick a seed that exercises the gru")
	}
	if !s.Night && s.Gru != nil {
		t.Error("daytime state still carries a gru")
	}
}

// gruTestState builds a controlled night scene: all agents parked and dead
// except the ones a test raises, the gru placed by hand.
func gruTestState(seed uint64) (*State, int64) {
	m := testMap(seed)
	s := NewState(seed, m)
	for i := range s.Agents {
		s.Agents[i].Dead = true
	}
	s.Night = true
	// 23:00 day 1: night, not a boundary, not a heartbeat/social tick.
	return s, int64(17*3600 + 1)
}

// findOpenArea returns a grass tile whose 4-neighborhood is passable — room
// to stage the gru beside an agent beside a fire.
func findOpenArea(t *testing.T, s *State) (int, int) {
	t.Helper()
	m := testMap(s.Seed)
	for y := 2; y < m.H-2; y++ {
	next:
		for x := 2; x < m.W-2; x++ {
			for dx := -2; dx <= 2; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if !passable(m, s, x+dx, y+dy) {
						continue next
					}
				}
			}
			return x, y
		}
	}
	t.Fatal("no open area on test map")
	return 0, 0
}

// TestGruLightAndShelterProtect is AC#2's safety half: an agent inside fire
// light (or on a shelter) is invisible — never attacked, never stalked onto.
func TestGruLightAndShelterProtect(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s, tick := gruTestState(seed)
	x, y := findOpenArea(t, s)

	// Fire at (x,y); agent 0 beside it (lit, warm); gru right next to the agent.
	// FuelUntil in the far future so warmAt keeps the agent put (T019): a cold
	// fire would send the reflex off to chop, vacating the protected tile.
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: x, Y: y, FuelUntil: tick + 24*3600})
	a := &s.Agents[0]
	a.Dead = false
	a.X, a.Y = x+1, y
	a.Needs = Needs{Health: 1000, Food: 600, Rest: 600, Warmth: 600, Morale: 600}
	// The gru starts beyond the light (fire light radius is 3).
	s.Gru = &Gru{X: x + 5, Y: y}

	for i := int64(0); i < 400; i++ {
		s.Tick = tick + i
		for _, e := range stepEvents(s, m, tick+i+1) {
			if e.Type == "gru.attacked" {
				t.Fatal("gru attacked an agent inside fire light")
			}
			if err := s.Apply(e); err != nil {
				t.Fatal(err)
			}
		}
		if s.Gru != nil && gruProtected(s, s.Gru.X, s.Gru.Y) {
			t.Fatal("gru stepped into a protected tile")
		}
	}
	if s.Agents[0].Needs.Health != 1000 {
		t.Errorf("protected agent lost health: %d", s.Agents[0].Needs.Health)
	}

	// Shelter: standing on it is equally absolute.
	s2, tick2 := gruTestState(seed)
	x2, y2 := findOpenArea(t, s2)
	s2.Structures = append(s2.Structures, Structure{Kind: "shelter", X: x2, Y: y2})
	b := &s2.Agents[1]
	b.Dead = false
	b.X, b.Y = x2, y2
	b.Needs = Needs{Health: 1000, Food: 600, Rest: 600, Warmth: 600, Morale: 600}
	s2.Gru = &Gru{X: x2 + 1, Y: y2}
	for _, e := range stepEvents(s2, m, tick2+1) {
		if e.Type == "gru.attacked" {
			t.Fatal("gru attacked an agent on a shelter")
		}
	}
}

// TestGruWoundsNotExecutes is AC#2's teeth half: an unprotected agent is
// wounded — absolute health drop, floored above zero, victim woken and
// intent cleared — and the cooldown spaces the wounds out. The gru is never
// a proximate cause of death.
func TestGruWoundsNotExecutes(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s, tick := gruTestState(seed)
	x, y := findOpenArea(t, s)

	a := &s.Agents[0]
	a.Dead = false
	a.Asleep = true // asleep in the open: exactly who the night should scare
	a.X, a.Y = x, y
	a.Needs = Needs{Health: 1000, Food: 600, Rest: 600, Warmth: 600, Morale: 600}
	a.Intent = &Intent{Goal: "sleep", TargetX: x, TargetY: y}
	s.Gru = &Gru{X: x + 1, Y: y}

	var attacked *GruAttackedPayload
	for _, e := range stepEvents(s, m, tick+1) {
		if e.Type == "gru.attacked" {
			var p GruAttackedPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			attacked = &p
		}
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if attacked == nil {
		t.Fatal("adjacent unprotected agent was not attacked")
	}
	if attacked.Health != 1000-gruWound {
		t.Errorf("wound: health %d, want %d", attacked.Health, 1000-gruWound)
	}
	if a.Needs.Health != 1000-gruWound || a.Asleep || a.Intent != nil {
		t.Errorf("victim after wound: health=%d asleep=%v intent=%v", a.Needs.Health, a.Asleep, a.Intent)
	}
	if s.Gru.LastAttack != tick+1 || s.Gru.LastVictim != 0 {
		t.Errorf("gru attack ledger: last=%d victim=%d", s.Gru.LastAttack, s.Gru.LastVictim)
	}

	// Cooldown: the very next tick must not wound again.
	for _, e := range stepEvents(s, m, tick+2) {
		if e.Type == "gru.attacked" {
			t.Fatal("second wound inside the cooldown")
		}
	}

	// The floor: a wound can bring an agent to the brink, never over it.
	a.Needs.Health = 50
	s.Gru.LastAttack = 0
	for _, e := range stepEvents(s, m, tick+2) {
		if e.Type == "agent.died" {
			t.Fatal("gru attack killed directly")
		}
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if a.Needs.Health != gruWoundFloor {
		t.Errorf("floored wound: health %d, want %d", a.Needs.Health, gruWoundFloor)
	}
	if a.Dead {
		t.Error("the gru must wound, not execute")
	}
}

// TestGruStoryFuel is AC#3: encounters leave rumor seeds and omen material —
// the victim's memory, a witness memory about the victim that TellableFor
// serves as gossip, and a once-per-night sighting per agent.
func TestGruStoryFuel(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s, tick := gruTestState(seed)
	x, y := findOpenArea(t, s)

	victim := &s.Agents[0]
	victim.Dead = false
	victim.X, victim.Y = x, y
	victim.Needs = Needs{Health: 1000, Food: 600, Rest: 600, Warmth: 600, Morale: 600}

	// Witness: awake, near the victim, but protected by firelight (fire far
	// enough that the victim stays outside the light) so the gru keeps its
	// eyes on the victim.
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: x - 5, Y: y})
	witness := &s.Agents[1]
	witness.Dead = false
	witness.X, witness.Y = x-4, y

	s.Gru = &Gru{X: x + 1, Y: y}

	sighted := 0
	for _, e := range stepEvents(s, m, tick+1) {
		if e.Type == "gru.sighted" {
			sighted++
		}
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if sighted != 2 {
		t.Errorf("both awake agents in range should sight the gru once, got %d", sighted)
	}
	// Latch: no re-sighting the same night.
	for _, e := range stepEvents(s, m, tick+2) {
		if e.Type == "gru.sighted" {
			t.Fatal("gru.sighted repeated within one night")
		}
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}

	var victimMemory, witnessMemory bool
	for _, mem := range victim.Memories {
		if mem.Salience == salGruAttack {
			victimMemory = true
		}
	}
	for _, mem := range witness.Memories {
		if mem.Subject == 0 && mem.Salience == salGruWitness && mem.Tone == toneGruAttack {
			witnessMemory = true
		}
	}
	if !victimMemory {
		t.Error("victim carries no attack memory")
	}
	if !witnessMemory {
		t.Error("witness carries no gossip-seed memory about the victim")
	}

	// The witness memory must be servable gossip: raise a listener the
	// fabric can tell it to.
	s.Agents[2].Dead = false
	tell, ok := TellableFor(s, 1, 2)
	if !ok || tell.Subject != 0 {
		t.Fatalf("witness memory is not tellable gossip: ok=%v subject=%d", ok, tell.Subject)
	}
}
