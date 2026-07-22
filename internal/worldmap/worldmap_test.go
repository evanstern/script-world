package worldmap

import "testing"

func TestSameSeedSameMap(t *testing.T) {
	a := Generate(42, 64, 64)
	b := Generate(42, 64, 64)
	if a.Hash() != b.Hash() {
		t.Fatal("same seed produced different maps (AC#3)")
	}
}

func TestDifferentSeedsDifferentMaps(t *testing.T) {
	if Generate(1, 64, 64).Hash() == Generate(2, 64, 64).Hash() {
		t.Fatal("different seeds produced identical maps")
	}
}

// TestVillageHasAllResources is AC#1 across a spread of seeds: every world
// must come with wood, water, forage, and animal territory.
func TestVillageHasAllResources(t *testing.T) {
	for _, seed := range []uint64{1, 7, 42, 99, 12345, 987654321} {
		m := Generate(seed, 64, 64)
		if n := m.CountKind(Water); n == 0 {
			t.Errorf("seed %d: no water", seed)
		}
		if n := m.CountKind(Tree); n == 0 {
			t.Errorf("seed %d: no trees (wood)", seed)
		}
		if n := m.CountKind(Forage); n == 0 {
			t.Errorf("seed %d: no forage", seed)
		}
		if n := m.CountKind(Rock); n == 0 {
			t.Errorf("seed %d: no rock outcrops (SC-001)", seed)
		}
		if len(m.Dens) == 0 {
			t.Errorf("seed %d: no animal dens", seed)
		}
	}
}

// TestColdStart is AC#2: no structures exist at generation (the type system
// has no structure tile kind), and buildable open ground is plentiful.
func TestColdStart(t *testing.T) {
	for _, seed := range []uint64{1, 7, 42, 99, 12345} {
		m := Generate(seed, 64, 64)
		buildable := 0
		for y := 0; y < m.H; y++ {
			for x := 0; x < m.W; x++ {
				if m.Buildable(x, y) {
					buildable++
				}
			}
		}
		// A village needs real room: at least 25% open grass.
		if buildable < m.W*m.H/4 {
			t.Errorf("seed %d: only %d buildable tiles of %d", seed, buildable, m.W*m.H)
		}
	}
}

func TestProportionsRoughlyHold(t *testing.T) {
	m := Generate(42, 64, 64)
	total := float64(m.W * m.H)
	if f := float64(m.CountKind(Water)) / total; f < 0.10 || f > 0.30 {
		t.Errorf("water fraction %.2f outside sane band", f)
	}
	if f := float64(m.CountKind(Tree)) / total; f < 0.10 || f > 0.35 {
		t.Errorf("tree fraction %.2f outside sane band", f)
	}
	// Rock is ~6% of dry grass remaining after trees, so a few percent of the
	// whole map (research R1: ~150-200 tiles on 64x64) — not the raw 6%.
	if f := float64(m.CountKind(Rock)) / total; f < 0.01 || f > 0.10 {
		t.Errorf("rock fraction %.3f outside sane band", f)
	}
}

// TestRockOutcropsAcrossSeeds is FR-001/SC-001: outcrops appear on every
// tested seed, identical generation is stable (Hash), and the ≥25% buildable
// floor holds with rock now claiming part of dry grass.
func TestRockOutcropsAcrossSeeds(t *testing.T) {
	for _, seed := range []uint64{1, 7, 42, 99, 12345, 987654321} {
		a := Generate(seed, 64, 64)
		b := Generate(seed, 64, 64)
		if a.Hash() != b.Hash() {
			t.Fatalf("seed %d: same-seed maps differ (AC#3) once outcrops are generated", seed)
		}
		if n := a.CountKind(Rock); n == 0 {
			t.Errorf("seed %d: no rock outcrops", seed)
		}
		if n := a.CountKind(Water); n == 0 {
			t.Errorf("seed %d: no water", seed)
		}
		if n := a.CountKind(Tree); n == 0 {
			t.Errorf("seed %d: no trees", seed)
		}
		if n := a.CountKind(Forage); n == 0 {
			t.Errorf("seed %d: no forage", seed)
		}
		if len(a.Dens) == 0 {
			t.Errorf("seed %d: no dens", seed)
		}
		buildable := 0
		for y := 0; y < a.H; y++ {
			for x := 0; x < a.W; x++ {
				if a.Buildable(x, y) {
					buildable++
				}
			}
		}
		if buildable < a.W*a.H/4 {
			t.Errorf("seed %d: only %d buildable tiles of %d (SC-001 floor)", seed, buildable, a.W*a.H)
		}
	}
}

func TestPassability(t *testing.T) {
	m := Generate(42, 64, 64)
	if m.Passable(-1, 0) || m.Passable(0, -1) || m.Passable(64, 0) || m.Passable(0, 64) {
		t.Error("out of bounds must be impassable")
	}
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			k := m.At(x, y)
			want := k == Grass || k == Forage
			if m.Passable(x, y) != want {
				t.Fatalf("(%d,%d) kind %d passable=%v", x, y, k, m.Passable(x, y))
			}
		}
	}
	for _, d := range m.Dens {
		if m.At(d.X, d.Y) != Grass {
			t.Errorf("den at (%d,%d) not on grass", d.X, d.Y)
		}
	}
}

func TestZeroDimsDefault(t *testing.T) {
	m := Generate(42, 0, 0)
	if m.W != DefaultSize || m.H != DefaultSize {
		t.Errorf("zero dims should default to %d, got %dx%d", DefaultSize, m.W, m.H)
	}
}
