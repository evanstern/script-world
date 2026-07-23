package tui

// Header governed-speed surface tests (spec 028 T014, US4-AC1/FR-015). The
// exact-string pins here are the byte-identity regression for ungoverned
// worlds and the plain-language contract for governed ones —
// contracts/status-protocol.md "TUI" §.

import (
	"testing"

	"github.com/evanstern/promptworld/internal/ipc"
)

// TestHeaderViewUngovernedUnchanged is the regression pin: a world with no
// RequestedSpeed set (the pre-028 shape, and any world the governor hasn't
// touched) renders its header byte-identically to before T014.
func TestHeaderViewUngovernedUnchanged(t *testing.T) {
	m := testModel(t)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{
		Tick:          100,
		GameTime:      "Day 1, 06:00",
		Speed:         "16x",
		EffectiveRate: 16.0,
	}}
	got := m.headerView()
	want := "test — tick 100 · Day 1, 06:00 · running · speed 16x (16.0 t/s)"
	if got != want {
		t.Errorf("ungoverned header = %q, want %q", got, want)
	}
}

// TestHeaderViewGoverned pins the exact governed-speed string (FR-015): the
// speed segment gains "asked <requested> — <jobs> minds in flight, debt
// <P>%" once RequestedSpeed differs from the effective Speed.
func TestHeaderViewGoverned(t *testing.T) {
	m := testModel(t)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{
		Tick:           100,
		GameTime:       "Day 1, 06:00",
		Speed:          "16x",
		EffectiveRate:  16.0,
		RequestedSpeed: "32x",
		GovernorDebt:   1.4,
		GovernorJobs:   3,
	}}
	got := m.headerView()
	want := "test — tick 100 · Day 1, 06:00 · running · speed 16x (16.0 t/s) asked 32x — 3 minds in flight, debt 140%"
	if got != want {
		t.Errorf("governed header = %q, want %q", got, want)
	}
}

// TestHeaderViewGovernedSingularMind: exactly one contributing thought reads
// "1 mind in flight", not "1 minds in flight".
func TestHeaderViewGovernedSingularMind(t *testing.T) {
	m := testModel(t)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{
		Tick:           500,
		GameTime:       "Day 2, 14:30",
		Speed:          "8x",
		EffectiveRate:  8.0,
		RequestedSpeed: "16x",
		GovernorDebt:   0.5,
		GovernorJobs:   1,
	}}
	got := m.headerView()
	want := "test — tick 500 · Day 2, 14:30 · running · speed 8x (8.0 t/s) asked 16x — 1 mind in flight, debt 50%"
	if got != want {
		t.Errorf("governed header (singular) = %q, want %q", got, want)
	}
}

// TestHeaderViewRequestedEqualSpeedUngoverned: RequestedSpeed present but
// equal to Speed (a transient the reducer shouldn't produce per data-model.md
// invariants, but the header must still degrade gracefully) renders
// ungoverned — the suffix only appears when the two differ.
func TestHeaderViewRequestedEqualSpeedUngoverned(t *testing.T) {
	m := testModel(t)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{
		Tick:           100,
		GameTime:       "Day 1, 06:00",
		Speed:          "16x",
		EffectiveRate:  16.0,
		RequestedSpeed: "16x",
	}}
	got := m.headerView()
	want := "test — tick 100 · Day 1, 06:00 · running · speed 16x (16.0 t/s)"
	if got != want {
		t.Errorf("header with RequestedSpeed==Speed = %q, want %q (no suffix)", got, want)
	}
}
