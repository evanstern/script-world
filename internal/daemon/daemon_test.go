package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
)

// openWorldWithMeeting writes a manifest carrying a meeting block and opens it.
func openWorldWithMeeting(t *testing.T, meeting string) *world.World {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"name":"w","seed":42,"format_version":1,"tick_game_seconds":1` + meeting + `}`
	if err := os.WriteFile(filepath.Join(dir, world.ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := world.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return w
}

// TestSeedMeetingConventionConfig (TASK-36 AC#2): a manifest-declared
// convention lands as the establishing event on boot, sets state, and rides
// the log so a replayed daemon re-establishes it without re-injecting.
func TestSeedMeetingConventionConfig(t *testing.T) {
	w := openWorldWithMeeting(t, `,"meeting":{"convene":"11:30","open":"12:00","x":7,"y":9}`)
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	state := sim.NewState(w.Manifest.Seed, w.Map())
	if err := seedMeetingConvention(w, st, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := state.MeetingConvention
	if c == nil {
		t.Fatal("boot seed did not establish the convention")
	}
	if c.ConveneSecond != 11*3600+1800 || c.OpenSecond != 12*3600 || c.Source != "config" {
		t.Errorf("convention = %+v, want 11:30/12:00/config", c)
	}
	if state.MeetingPlace == nil || state.MeetingPlace.X != 7 || state.MeetingPlace.Y != 9 {
		t.Errorf("meeting place = %+v, want the config coords (7,9)", state.MeetingPlace)
	}

	// The event is persisted: a fresh state rebuilt from the log carries it.
	replayed := sim.NewState(w.Manifest.Seed, w.Map())
	var establishes int
	if err := st.ReplayEvents(0, func(e store.Event) error {
		if e.Type == "meeting.convention_established" {
			establishes++
		}
		return replayed.Apply(e)
	}); err != nil {
		t.Fatal(err)
	}
	if establishes != 1 {
		t.Errorf("%d convention_established events in the log, want exactly one", establishes)
	}
	if replayed.MeetingConvention == nil || *replayed.MeetingConvention != *c {
		t.Errorf("replay convention %+v != live %+v", replayed.MeetingConvention, c)
	}

	// Idempotent: a second boot with the convention already in state injects
	// nothing (the guard), so no second event lands.
	if err := seedMeetingConvention(w, st, replayed); err != nil {
		t.Fatal(err)
	}
	var after int
	st.ReplayEvents(0, func(e store.Event) error {
		if e.Type == "meeting.convention_established" {
			after++
		}
		return nil
	})
	if after != 1 {
		t.Errorf("re-seed injected a duplicate: %d establish events", after)
	}
}

// TestSeedMeetingConventionAbsent: no manifest meeting block → no convention,
// no event (emergent default).
func TestSeedMeetingConventionAbsent(t *testing.T) {
	w := openWorldWithMeeting(t, ``)
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	state := sim.NewState(w.Manifest.Seed, w.Map())
	if err := seedMeetingConvention(w, st, state); err != nil {
		t.Fatal(err)
	}
	if state.MeetingConvention != nil {
		t.Errorf("no manifest meeting block should leave the world convention-less, got %+v", state.MeetingConvention)
	}
	var events int
	st.ReplayEvents(0, func(e store.Event) error { events++; return nil })
	if events != 0 {
		t.Errorf("%d events written for a convention-less boot, want none", events)
	}
}

// TestSeedMeetingConventionNoCoords: a manifest meeting without x/y derives the
// place, so the convention still lands with a concrete meeting place.
func TestSeedMeetingConventionNoCoords(t *testing.T) {
	w := openWorldWithMeeting(t, `,"meeting":{"convene":"11:30","open":"12:00"}`)
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	state := sim.NewState(w.Manifest.Seed, w.Map())
	if err := seedMeetingConvention(w, st, state); err != nil {
		t.Fatal(err)
	}
	if state.MeetingConvention == nil || state.MeetingPlace == nil {
		t.Fatalf("convention/place missing: conv=%+v place=%+v", state.MeetingConvention, state.MeetingPlace)
	}
}
