package world

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// --- v1 fixture shapes: the frozen legacy state as the v1 binary wrote it
// (inv carries `food`; no fuel_until/quarried). Built as raw legacy JSON so the
// test exercises the real migration decode path, not sim's internal structs.

type v1Inv struct {
	Wood int `json:"wood"`
	Food int `json:"food"`
}

type v1Agent struct {
	Name      string       `json:"name"`
	X         int          `json:"x"`
	Y         int          `json:"y"`
	Needs     sim.Needs    `json:"needs"`
	Inv       v1Inv        `json:"inv"`
	Dead      bool         `json:"dead"`
	Intent    *sim.Intent  `json:"intent,omitempty"`
	Memories  []sim.Memory `json:"memories,omitempty"`
	NearDeath bool         `json:"near_death,omitempty"`
	LastTalk  int64        `json:"last_talk"`
}

type v1State struct {
	Tick            int64           `json:"tick"`
	Seed            uint64          `json:"seed"`
	Speed           clock.Speed     `json:"speed"`
	Night           bool            `json:"night"`
	Agents          []v1Agent       `json:"agents"`
	Structures      []sim.Structure `json:"structures,omitempty"`
	Relations       []sim.Relation  `json:"relations,omitempty"`
	Debts           []sim.Debt      `json:"debts,omitempty"`
	MetatronCharges int             `json:"metatron_charges"`
}

// v1StateJSON builds a representative v1 covering-snapshot state: eight souls
// (one carrying wood+legacy-food with a mid-flight intent, one near-death), a
// standing fire, an open debt and a relation, and Metatron charges.
func v1StateJSON(t *testing.T, seed uint64, tick int64) []byte {
	t.Helper()
	agents := make([]v1Agent, sim.AgentCount)
	names := []string{"Ash", "Birch", "Cedar", "Rowan", "Fern", "Hazel", "Oak", "Sage"}
	for i := range agents {
		agents[i] = v1Agent{
			Name:     names[i],
			X:        i,
			Y:        i,
			Needs:    sim.Needs{Health: 800, Food: 500, Rest: 600, Warmth: 700, Morale: 550},
			Inv:      v1Inv{Wood: 1, Food: 2},
			Memories: []sim.Memory{{Text: "we survived the frost", Salience: 5, Tick: 1000, Subject: -1}},
			LastTalk: 400,
		}
	}
	// Agent 0 carries more and is mid-chop — the intent must be wiped.
	agents[0].Inv = v1Inv{Wood: 7, Food: 4}
	agents[0].Intent = &sim.Intent{Goal: "chop", TargetX: 3, TargetY: 4, ResX: 3, ResY: 5, WorkStart: tick - 10}
	agents[1].NearDeath = true

	vs := v1State{
		Tick:            tick,
		Seed:            seed,
		Speed:           clock.Speed4x,
		Night:           true,
		Agents:          agents,
		Structures:      []sim.Structure{{Kind: "fire", X: 10, Y: 10}},
		Relations:       []sim.Relation{{From: 0, To: 1, Trust: 250, Affection: 150}},
		Debts:           []sim.Debt{{ID: 1, Debtor: 0, Creditor: 1, Kind: "food", Due: tick + 3600, Status: "open"}},
		MetatronCharges: 3,
	}
	b, err := json.Marshal(vs)
	if err != nil {
		t.Fatalf("marshal v1 state: %v", err)
	}
	return b
}

// writeV1World lays down a v1 world directory: a format_version-1 manifest, a
// contiguous event log of nEvents rows, and a valid snapshot. When covering,
// the snapshot's seq equals the max event seq (a clean shutdown); otherwise two
// events land after it (an unclean stop with un-snapshotted tail).
func writeV1World(t *testing.T, dir string, seed uint64, tick int64, nEvents int, covering bool) {
	t.Helper()
	manifest := `{"name":"fixture","seed":` + strconv.FormatUint(seed, 10) +
		`,"created_at":"2026-07-21T00:00:00Z","format_version":1,"tick_game_seconds":1,"map_width":64,"map_height":64}`
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	appendDummy(t, st, nEvents)
	if err := st.SaveSnapshot(tick, st.LastSeq(), v1StateJSON(t, seed, tick)); err != nil {
		t.Fatal(err)
	}
	if !covering {
		appendDummy(t, st, 2) // tail beyond the snapshot
	}
}

// appendTail opens an already-written world.db and appends events past its
// snapshot — used to reproduce the trailing tail a real v1 daemon leaves.
func appendTail(t *testing.T, dir string, evs ...store.Event) {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AppendEvents(evs); err != nil {
		t.Fatal(err)
	}
}

func appendDummy(t *testing.T, st *store.Store, n int) {
	t.Helper()
	evs := make([]store.Event, n)
	for i := range evs {
		evs[i] = store.Event{Tick: int64(i), Type: "agent.foraged", Payload: json.RawMessage(`{}`)}
	}
	if err := st.AppendEvents(evs); err != nil {
		t.Fatal(err)
	}
}

// TestMigrateHappyPath is spec 012 US6 AC#1/#2/#3 (FR-023): a cleanly-stopped
// v1 world migrates; people carry, the land resets, the archive appears, and
// the manifest is v2.
func TestMigrateHappyPath(t *testing.T) {
	dir := t.TempDir()
	const seed = uint64(42)
	const tick = int64(257400)
	writeV1World(t, dir, seed, tick, 6, true)

	res, err := Migrate(dir)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if res.AgentsCarried != sim.AgentCount || res.Tick != tick || res.SourceEvents != 6 {
		t.Errorf("result = %+v, want %d agents / tick %d / 6 events", res, sim.AgentCount, tick)
	}
	if _, err := os.Stat(res.ArchivePath); err != nil {
		t.Errorf("archive %s missing: %v", res.ArchivePath, err)
	}

	// The manifest is now v2 and Open succeeds.
	w, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after migrate: %v", err)
	}
	if w.Manifest.FormatVersion != FormatVersion {
		t.Errorf("manifest format_version = %d, want %d", w.Manifest.FormatVersion, FormatVersion)
	}

	// The fresh log's covering snapshot holds the migrated state.
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	snap, err := st.LatestValidSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("no covering snapshot after migrate: %v", err)
	}
	if snap.Tick != tick {
		t.Errorf("snapshot tick = %d, want %d (continuity)", snap.Tick, tick)
	}
	var s sim.State
	if err := json.Unmarshal(snap.State, &s); err != nil {
		t.Fatal(err)
	}

	m := w.Map()
	if len(s.Agents) != sim.AgentCount {
		t.Fatalf("agents = %d, want %d", len(s.Agents), sim.AgentCount)
	}
	for i := range s.Agents {
		a := &s.Agents[i]
		if len(a.Memories) != 1 || a.Memories[0].Text != "we survived the frost" {
			t.Errorf("agent %d memories not carried: %+v", i, a.Memories)
		}
		if a.Intent != nil {
			t.Errorf("agent %d intent should be wiped, got %+v", i, a.Intent)
		}
		if !m.Passable(a.X, a.Y) {
			t.Errorf("agent %d on impassable v2 tile (%d,%d)", i, a.X, a.Y)
		}
	}
	// Agent 0: wood 1:1, food×3 → meals.
	if s.Agents[0].Inv.Wood != 7 || s.Agents[0].Inv.Meals != 4*3 {
		t.Errorf("agent 0 inv = %+v, want wood 7 / meals 12", s.Agents[0].Inv)
	}
	if !s.Agents[1].NearDeath {
		t.Error("agent 1 NearDeath latch should carry")
	}
	// Map-bound state reset; fabric carried.
	if len(s.Structures) != 0 {
		t.Errorf("structures should reset, got %+v", s.Structures)
	}
	if len(s.Relations) != 1 || len(s.Debts) != 1 || s.MetatronCharges != 3 {
		t.Errorf("fabric not carried: rel=%v debt=%v charges=%d", s.Relations, s.Debts, s.MetatronCharges)
	}
	// Outcrops exist in the reborn land (the format break's whole point).
	if m.CountKind(worldmap.Rock) == 0 {
		t.Error("v2 map has no rock outcrops")
	}
}

// TestMigrateReplayDeterminismNoSnapshots is the determinism half of SC-007
// (FR-026): delete every snapshot from the migrated world and rebuild from
// genesis (world.created → world.migrated) — byte-identical to the
// post-migration snapshot.
func TestMigrateReplayDeterminismNoSnapshots(t *testing.T) {
	dir := t.TempDir()
	const seed = uint64(7)
	const tick = int64(120000)
	writeV1World(t, dir, seed, tick, 4, true)
	if _, err := Migrate(dir); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snap, err := st.LatestValidSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("snapshot: %v", err)
	}
	var want sim.State
	if err := json.Unmarshal(snap.State, &want); err != nil {
		t.Fatal(err)
	}

	// Delete ALL snapshots, then recover from genesis exactly as the daemon does
	// with an empty snapshot table.
	if err := st.PruneSnapshots(0); err != nil {
		t.Fatal(err)
	}
	if s, err := st.LatestValidSnapshot(); err != nil || s != nil {
		t.Fatalf("snapshots should be gone: snap=%v err=%v", s, err)
	}

	got := sim.NewState(w.Manifest.Seed, w.Map())
	if err := st.ReplayEvents(0, func(e store.Event) error {
		if err := got.Apply(e); err != nil {
			return err
		}
		if e.Tick > got.Tick {
			got.Tick = e.Tick
		}
		return nil
	}); err != nil {
		t.Fatalf("replay: %v", err)
	}

	if got.Hash() != want.Hash() {
		t.Fatalf("zero-snapshot replay diverged:\nwant %s\ngot  %s", want.Hash(), got.Hash())
	}
}

// TestMigrateRefusesUncoveredTail is spec 012 US6 AC#4 (FR-024): a world whose
// log runs past its latest snapshot is refused untouched, with the start+stop
// remedy.
func TestMigrateRefusesUncoveredTail(t *testing.T) {
	dir := t.TempDir()
	writeV1World(t, dir, 42, 5000, 4, false) // snapshot at seq 4, log runs to 6

	_, err := Migrate(dir)
	if err == nil {
		t.Fatal("Migrate should refuse an uncovered tail")
	}
	if !strings.Contains(err.Error(), "start and stop") {
		t.Errorf("error should name the start+stop remedy, got: %v", err)
	}
	assertUntouched(t, dir)
}

// TestMigrateToleratesDaemonTail is the real-world shape (spec amendment,
// FR-024): a v1 daemon appends `daemon.stopped` AFTER its shutdown snapshot, so
// a cleanly-stopped world has a one-event daemon.* tail past the covering
// snapshot. That tail is process bookkeeping (a reducer no-op, zero sim state);
// migration tolerates it and drops it. source_events still reflects the full v1
// log (the archive's true size), tail included.
func TestMigrateToleratesDaemonTail(t *testing.T) {
	dir := t.TempDir()
	const seed = uint64(42)
	const tick = int64(257400)
	writeV1World(t, dir, seed, tick, 6, true) // snapshot at seq 6
	appendTail(t, dir, store.Event{Tick: tick, Type: "daemon.stopped", Payload: json.RawMessage(`{"tick":257400}`)})

	res, err := Migrate(dir)
	if err != nil {
		t.Fatalf("Migrate should tolerate a daemon.* tail: %v", err)
	}
	// source_events counts the whole v1 log, the trailing daemon.stopped
	// included (the archive holds all 7 events).
	if res.SourceEvents != 7 {
		t.Errorf("SourceEvents = %d, want 7 (full v1 log incl. daemon tail)", res.SourceEvents)
	}
	if res.Tick != tick {
		t.Errorf("Tick = %d, want %d", res.Tick, tick)
	}
}

// TestMigrateRefusesSimTail is the other half of the amended precondition: a
// SIM-affecting event past the snapshot (not daemon.*) is genuine
// un-snapshotted history and still refuses, untouched.
func TestMigrateRefusesSimTail(t *testing.T) {
	dir := t.TempDir()
	writeV1World(t, dir, 42, 5000, 6, true) // covering snapshot at seq 6
	appendTail(t, dir, store.Event{Tick: 5001, Type: "agent.moved", Payload: json.RawMessage(`{"agent":0,"x":1,"y":1}`)})

	_, err := Migrate(dir)
	if err == nil {
		t.Fatal("Migrate should refuse a sim-affecting event past the snapshot")
	}
	if !strings.Contains(err.Error(), "start and stop") {
		t.Errorf("error should name the start+stop remedy, got: %v", err)
	}
	assertUntouched(t, dir)
}

// TestMigrateRefusesSecondRun is spec 012 US6 AC#5 (FR-025): a migrated world
// refuses a second migration (already v2), and the archive is never overwritten.
func TestMigrateRefusesSecondRun(t *testing.T) {
	dir := t.TempDir()
	writeV1World(t, dir, 42, 5000, 4, true)
	if _, err := Migrate(dir); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if _, err := Migrate(dir); err == nil {
		t.Fatal("second Migrate should refuse (already migrated)")
	}
}

// TestMigrateRefusesExistingArchive is the direct already-migrated guard
// (FR-025): a v1 world that already carries a world.v1.db (e.g. an interrupted
// prior run) is refused without touching the archive.
func TestMigrateRefusesExistingArchive(t *testing.T) {
	dir := t.TempDir()
	writeV1World(t, dir, 42, 5000, 4, true)
	// Plant a pre-existing archive.
	if err := os.WriteFile(filepath.Join(dir, "world.v1.db"), []byte("prior archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Migrate(dir)
	if err == nil {
		t.Fatal("Migrate should refuse when world.v1.db already exists")
	}
	// The pre-existing archive is untouched.
	data, rerr := os.ReadFile(filepath.Join(dir, "world.v1.db"))
	if rerr != nil || string(data) != "prior archive" {
		t.Errorf("existing archive was clobbered: %q err=%v", string(data), rerr)
	}
}

// TestMigrateRefusesRunningDaemon is FR-023's liveness gate: a live daemon
// pidfile refuses migration untouched.
func TestMigrateRefusesRunningDaemon(t *testing.T) {
	dir := t.TempDir()
	writeV1World(t, dir, 42, 5000, 4, true)
	// The test process itself is a live pid — the same trick the daemon tests
	// use to fake liveness.
	if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Migrate(dir)
	if err == nil {
		t.Fatal("Migrate should refuse while a daemon holds the world")
	}
	if !strings.Contains(err.Error(), "daemon is running") {
		t.Errorf("error should name the running daemon, got: %v", err)
	}
	assertUntouched(t, dir)
}

// assertUntouched verifies a refused migration left the world exactly as it was:
// world.db present, no archive, manifest still v1.
func assertUntouched(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "world.db")); err != nil {
		t.Errorf("world.db should be untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "world.v1.db")); err == nil {
		t.Error("no archive should exist after a refused migration")
	}
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.FormatVersion != 1 {
		t.Errorf("manifest should still be v1 after a refused migration, got %d", m.FormatVersion)
	}
}
