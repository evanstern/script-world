package sim

// Spec 019 (US3) reducer tests: the journal.* Apply arms enforce the budget and
// entry existence deterministically — the door dry-run turns their errors into
// rejections, so an over-budget write or an unknown-id delete never lands.

import (
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

func writeJournal(t *testing.T, s *State, tick int64, agent int, text string) error {
	t.Helper()
	return s.Apply(store.Event{Tick: tick, Type: "journal.entry_written",
		Payload: mustPayload(JournalWrittenPayload{Agent: agent, Text: text})})
}

func deleteJournal(t *testing.T, s *State, tick int64, agent, entry int) error {
	t.Helper()
	return s.Apply(store.Event{Tick: tick, Type: "journal.entry_deleted",
		Payload: mustPayload(JournalDeletedPayload{Agent: agent, Entry: entry})})
}

// TestJournalIDStabilityAndOrder: ids are reducer-assigned, monotonic, never
// reused; entries stay in append order; deleting the middle preserves the rest
// and does NOT renumber survivors.
func TestJournalIDStabilityAndOrder(t *testing.T) {
	s := NewState(42, testMap(42))
	for i, txt := range []string{"first", "second", "third"} {
		if err := writeJournal(t, s, int64(100+i), 0, txt); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	j := s.Agents[0].Journal
	if j == nil || len(j.Entries) != 3 || j.NextID != 3 {
		t.Fatalf("after 3 writes: %+v", j)
	}
	if j.Entries[0].ID != 0 || j.Entries[1].ID != 1 || j.Entries[2].ID != 2 {
		t.Errorf("ids not monotonic from 0: %+v", j.Entries)
	}

	// Delete the middle: survivors keep their ids (no renumber), order preserved.
	if err := deleteJournal(t, s, 200, 0, 1); err != nil {
		t.Fatal(err)
	}
	j = s.Agents[0].Journal
	if len(j.Entries) != 2 || j.Entries[0].ID != 0 || j.Entries[1].ID != 2 {
		t.Errorf("after deleting #1: %+v", j.Entries)
	}
	if j.NextID != 3 {
		t.Errorf("NextID = %d, want 3 (ids never reused)", j.NextID)
	}
	// A new write takes the next fresh id, not a reused one.
	if err := writeJournal(t, s, 300, 0, "fourth"); err != nil {
		t.Fatal(err)
	}
	if last := s.Agents[0].Journal.Entries[2]; last.ID != 3 {
		t.Errorf("new entry id = %d, want 3", last.ID)
	}
}

// TestJournalBudgetRejectionLeavesStateUntouched: a write that would exceed the
// budget errors (the door would reject it) and mutates NOTHING; a smaller write
// that fits still lands. The reason names the budget and current usage.
func TestJournalBudgetRejectionLeavesStateUntouched(t *testing.T) {
	s := NewState(42, testMap(42))
	// Fill most of the budget with legal ≤cap writes.
	big := strings.Repeat("x", journalWriteCapRunes) // 1000 runes each
	for i := 0; i < 3; i++ {
		if err := writeJournal(t, s, int64(10+i), 0, big); err != nil {
			t.Fatalf("legal write %d rejected: %v", i, err)
		}
	}
	used := s.Agents[0].Journal.JournalUsedRunes()
	if used != 3*journalWriteCapRunes {
		t.Fatalf("used = %d, want %d", used, 3*journalWriteCapRunes)
	}

	// The whole budget is 4000; 3000 used. A 1001-rune... but the wire cap is
	// enforced upstream, so simulate a within-cap write that still overflows:
	// 3000 used + 1000 = 4000 (exactly fits), so a further 1-rune write overflows.
	if err := writeJournal(t, s, 20, 0, big); err != nil {
		t.Fatalf("the 4000th rune write should fit exactly: %v", err)
	}
	before := s.Agents[0].Journal.Clone()
	err := writeJournal(t, s, 21, 0, "!") // 4000 used, +1 overflows
	if err == nil {
		t.Fatal("over-budget write must be rejected")
	}
	if !strings.Contains(err.Error(), "journal budget") || !strings.Contains(err.Error(), "4000") {
		t.Errorf("rejection reason should name the budget: %q", err.Error())
	}
	// State untouched: same entries, same NextID.
	after := s.Agents[0].Journal
	if after.NextID != before.NextID || len(after.Entries) != len(before.Entries) {
		t.Errorf("rejected write mutated the journal: before %+v after %+v", before, after)
	}
}

// TestJournalDeleteUnknownIDErrors: deleting an absent id errors (the door tells
// the agent nothing matched) and mutates nothing; deleting from a never-used
// journal likewise errors.
func TestJournalDeleteUnknownIDErrors(t *testing.T) {
	s := NewState(42, testMap(42))
	if err := deleteJournal(t, s, 10, 0, 5); err == nil {
		t.Error("delete from an empty journal must error")
	}
	if err := writeJournal(t, s, 11, 0, "one"); err != nil {
		t.Fatal(err)
	}
	err := deleteJournal(t, s, 12, 0, 99)
	if err == nil || !strings.Contains(err.Error(), "no journal entry #99") {
		t.Errorf("delete unknown id: err = %v, want \"no journal entry #99\"", err)
	}
	if len(s.Agents[0].Journal.Entries) != 1 {
		t.Error("failed delete mutated the journal")
	}
}

// TestJournalFreedBudgetReusable: deleting an entry frees its runes for future
// writes — after a delete, a write that would not have fit now lands.
func TestJournalFreedBudgetReusable(t *testing.T) {
	s := NewState(42, testMap(42))
	big := strings.Repeat("x", journalWriteCapRunes)
	for i := 0; i < 4; i++ { // 4000 used — full
		if err := writeJournal(t, s, int64(10+i), 0, big); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := writeJournal(t, s, 20, 0, "y"); err == nil {
		t.Fatal("journal is full — write must be rejected")
	}
	// Free 1000 runes by deleting entry #0.
	if err := deleteJournal(t, s, 21, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(t, s, 22, 0, big); err != nil {
		t.Errorf("write after freeing budget should fit: %v", err)
	}
}

// TestJournalSearchDeterministicNewestFirst: search is a case-insensitive
// substring match, newest-first, capped, and model-free.
func TestJournalSearchDeterministicNewestFirst(t *testing.T) {
	s := NewState(42, testMap(42))
	for i, txt := range []string{"the Fire held", "cold morning", "banked the FIRE", "foraged"} {
		if err := writeJournal(t, s, int64(10+i), 0, txt); err != nil {
			t.Fatal(err)
		}
	}
	got := s.Agents[0].Journal.SearchJournal("fire")
	if len(got) != 2 {
		t.Fatalf("search \"fire\" matched %d entries, want 2 (case-insensitive)", len(got))
	}
	// Newest-first: "banked the FIRE" (id 2) before "the Fire held" (id 0).
	if got[0].ID != 2 || got[1].ID != 0 {
		t.Errorf("search order = [%d %d], want newest-first [2 0]", got[0].ID, got[1].ID)
	}
	if len(s.Agents[0].Journal.SearchJournal("nonexistent")) != 0 {
		t.Error("no-match search must return empty, not an error")
	}
	// Nil journal is safe.
	if (&Agent{}).Journal.SearchJournal("x") != nil {
		t.Error("search on a nil journal must return nil")
	}
}
