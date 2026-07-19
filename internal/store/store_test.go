package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ev(tick int64, typ string) Event {
	return Event{Tick: tick, Type: typ, Payload: json.RawMessage(`{}`)}
}

func TestAppendAssignsContiguousSeq(t *testing.T) {
	s := openTestStore(t)
	if err := s.AppendEvents([]Event{ev(1, "a"), ev(1, "b")}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvents([]Event{ev(2, "c")}); err != nil {
		t.Fatal(err)
	}
	if s.LastSeq() != 3 {
		t.Errorf("LastSeq = %d, want 3", s.LastSeq())
	}
	got, err := s.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range got {
		if e.Seq != int64(i)+1 {
			t.Errorf("event %d has seq %d, want %d", i, e.Seq, i+1)
		}
	}
	if err := s.CheckContiguity(); err != nil {
		t.Errorf("CheckContiguity: %v", err)
	}
}

func TestAppendOnlyTriggers(t *testing.T) {
	s := openTestStore(t)
	if err := s.AppendEvents([]Event{ev(1, "a")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE events SET type = 'tampered' WHERE seq = 1"); err == nil {
		t.Fatal("UPDATE on events should be rejected by trigger")
	}
	if _, err := s.db.Exec("DELETE FROM events WHERE seq = 1"); err == nil {
		t.Fatal("DELETE on events should be rejected by trigger")
	}
}

func TestSnapshotSaveVerifyFallback(t *testing.T) {
	s := openTestStore(t)
	if err := s.SaveSnapshot(100, 5, []byte(`{"tick":100}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSnapshot(200, 9, []byte(`{"tick":200}`)); err != nil {
		t.Fatal(err)
	}
	snap, err := s.LatestValidSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("LatestValidSnapshot: %v %v", snap, err)
	}
	if snap.Tick != 200 {
		t.Errorf("latest snapshot tick = %d, want 200", snap.Tick)
	}

	// Corrupt the newest snapshot: recovery must fall back to the older one.
	if _, err := s.db.Exec("UPDATE snapshots SET state = '{\"corrupt\":1}' WHERE tick = 200"); err != nil {
		t.Fatal(err)
	}
	snap, err = s.LatestValidSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("after corruption: %v %v", snap, err)
	}
	if snap.Tick != 100 {
		t.Errorf("fallback snapshot tick = %d, want 100", snap.Tick)
	}
}

func TestPruneKeepsEventsUntouched(t *testing.T) {
	s := openTestStore(t)
	if err := s.AppendEvents([]Event{ev(1, "a"), ev(2, "b")}); err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 30; i++ {
		if err := s.SaveSnapshot(i*100, 2, []byte(`{"n":1}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PruneSnapshots(24); err != nil {
		t.Fatal(err)
	}
	var snapCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM snapshots").Scan(&snapCount); err != nil {
		t.Fatal(err)
	}
	if snapCount != 24 {
		t.Errorf("snapshots after prune = %d, want 24", snapCount)
	}
	snap, err := s.LatestValidSnapshot()
	if err != nil || snap == nil || snap.Tick != 3000 {
		t.Errorf("newest snapshot should survive prune, got %+v err %v", snap, err)
	}
	events, err := s.EventsSince(0, 0)
	if err != nil || len(events) != 2 {
		t.Errorf("events must never be pruned: %d events, err %v", len(events), err)
	}
}

func TestContiguityDetectsGap(t *testing.T) {
	s := openTestStore(t)
	// Bypass AppendEvents to fabricate a holed log (triggers allow INSERT).
	if _, err := s.db.Exec(
		"INSERT INTO events (seq, tick, type, payload, wall_time) VALUES (1, 1, 'a', '{}', ''), (3, 2, 'b', '{}', '')"); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckContiguity(); err == nil {
		t.Fatal("CheckContiguity should detect the seq gap")
	}
}

func TestMetaRoundTrip(t *testing.T) {
	s := openTestStore(t)
	if v, err := s.GetMeta("missing"); err != nil || v != "" {
		t.Errorf("GetMeta(missing) = %q, %v", v, err)
	}
	if err := s.SetMeta("seed", "42"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("seed", "43"); err != nil {
		t.Fatal(err) // upsert
	}
	if v, _ := s.GetMeta("seed"); v != "43" {
		t.Errorf("GetMeta(seed) = %q, want 43", v)
	}
}
