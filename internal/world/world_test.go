package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateOpenRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "w1")
	w, err := Create(dir, "testworld", 42)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.Manifest.Seed != 42 || w.Manifest.FormatVersion != FormatVersion {
		t.Errorf("manifest = %+v", w.Manifest)
	}
	if _, err := os.Stat(filepath.Join(dir, "agents")); err != nil {
		t.Errorf("agents/ dir missing: %v", err)
	}
	got, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.Manifest != w.Manifest {
		t.Errorf("Open manifest %+v != created %+v", got.Manifest, w.Manifest)
	}
}

func TestCreateRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "junk"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir, "w", 1); err == nil {
		t.Fatal("Create on non-empty dir should fail")
	}
}

func TestOpenRejectsBadFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestName),
		[]byte(`{"name":"x","seed":1,"format_version":99,"tick_game_seconds":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("Open should reject unknown format_version")
	}
}

func TestMeetingConfigSeconds(t *testing.T) {
	ok := []struct {
		convene, open   string
		wantC, wantOpen int
	}{
		{"11:30", "12:00", 11*3600 + 1800, 12 * 3600},
		{"00:00", "23:59", 0, 23*3600 + 59*60},
		{"09:05", "09:06", 9*3600 + 5*60, 9*3600 + 6*60},
	}
	for _, c := range ok {
		mc := &MeetingConfig{Convene: c.convene, Open: c.open}
		gotC, gotO, err := mc.Seconds()
		if err != nil {
			t.Errorf("Seconds(%s,%s) unexpected error: %v", c.convene, c.open, err)
			continue
		}
		if gotC != c.wantC || gotO != c.wantOpen {
			t.Errorf("Seconds(%s,%s) = %d,%d want %d,%d", c.convene, c.open, gotC, gotO, c.wantC, c.wantOpen)
		}
	}
	bad := []MeetingConfig{
		{Convene: "12:00", Open: "11:30"}, // convene after open
		{Convene: "12:00", Open: "12:00"}, // equal
		{Convene: "25:00", Open: "26:00"}, // out of range
		{Convene: "noon", Open: "13:00"},  // unparseable
		{Convene: "12:60", Open: "13:00"}, // bad minutes
	}
	for _, mc := range bad {
		if _, _, err := mc.Seconds(); err == nil {
			t.Errorf("Seconds(%+v) should have errored", mc)
		}
	}
}

func TestOpenRejectsBadMeeting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestName),
		[]byte(`{"name":"x","seed":1,"format_version":1,"tick_game_seconds":1,"meeting":{"convene":"13:00","open":"12:00"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("Open should reject a meeting with convene after open")
	}
}

func TestOpenAcceptsMeeting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestName),
		[]byte(`{"name":"x","seed":1,"format_version":1,"tick_game_seconds":1,"meeting":{"convene":"11:30","open":"12:00","x":7,"y":9}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if w.Manifest.Meeting == nil || w.Manifest.Meeting.X == nil || *w.Manifest.Meeting.X != 7 {
		t.Fatalf("meeting config not parsed: %+v", w.Manifest.Meeting)
	}
}

func TestCalibrationPath(t *testing.T) {
	w := &World{Dir: "/tmp/w"}
	if got := w.CalibrationPath(); got != filepath.Join("/tmp/w", "calibration.json") {
		t.Errorf("CalibrationPath = %q", got)
	}
}
