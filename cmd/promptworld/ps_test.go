package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/worlds"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestLiveOnlyFiltersNonLiveStates(t *testing.T) {
	in := []worlds.Instance{
		{Name: "a", State: worlds.Running},
		{Name: "b", State: worlds.Paused},
		{Name: "c", State: worlds.Unresponsive},
		{Name: "d", State: worlds.Stopped},
		{Name: "e", State: worlds.Missing},
		{Name: "f", State: worlds.Unreadable},
	}
	out := liveOnly(in)
	if len(out) != 3 {
		t.Fatalf("liveOnly kept %d, want 3: %+v", len(out), out)
	}
	for _, inst := range out {
		if inst.State == worlds.Stopped || inst.State == worlds.Missing || inst.State == worlds.Unreadable {
			t.Errorf("liveOnly must not keep %s", inst.State)
		}
	}
}

func TestPsRowForRunningWithLLM(t *testing.T) {
	inst := worlds.Instance{
		Name: "aria", Path: "/w/aria", State: worlds.Running,
		WorldName: "aria", Seed: 7, FormatVersion: 1,
		Status: &ipc.StatusData{
			World:  ipc.WorldStatus{Name: "aria", Seed: 7, FormatVersion: 1},
			Clock:  ipc.ClockStatus{Tick: 100, GameTime: "d1 00:01:40", Speed: "4x"},
			Daemon: ipc.DaemonStatus{Pid: 4242, UptimeSeconds: 60, Subscribers: 1},
			LLM:    &llm.Status{Month: "2026-07"},
		},
	}
	row := psRowFor(inst)
	if row.State != "running" || row.Name != "aria" || row.Path != "/w/aria" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.Clock == nil || row.Clock.Tick != 100 {
		t.Fatalf("expected live clock, got %+v", row.Clock)
	}
	if row.Daemon == nil || row.Daemon.Pid != 4242 {
		t.Fatalf("expected live daemon pid, got %+v", row.Daemon)
	}
	if row.LLM == nil {
		t.Error("expected LLM status to be present for an inference-enabled world")
	}
	if row.LLMConfigured != nil {
		t.Error("running rows must not carry llm_configured (that's the stopped-row field)")
	}
}

func TestPsRowForRunningWithoutLLM(t *testing.T) {
	inst := worlds.Instance{
		Name: "aria", State: worlds.Running,
		Status: &ipc.StatusData{Clock: ipc.ClockStatus{Tick: 1}},
	}
	row := psRowFor(inst)
	if row.LLM != nil {
		t.Error("expected no LLM status when the daemon reports none")
	}
}

func TestPsRowForStopped(t *testing.T) {
	inst := worlds.Instance{
		Name: "old-run", Path: "/w/old-run", State: worlds.Stopped,
		WorldName: "old-run", Seed: 9, FormatVersion: 1,
		Tick: 52100, GameTime: "d1 14:28:20", OfflineSpeed: "1x", LLMConfigured: false,
	}
	row := psRowFor(inst)
	if row.State != "stopped" {
		t.Fatalf("state = %s", row.State)
	}
	if row.Daemon == nil || row.Daemon.Running == nil || *row.Daemon.Running {
		t.Fatalf("expected daemon.running = false, got %+v", row.Daemon)
	}
	if row.Clock == nil || row.Clock.Tick != 52100 {
		t.Fatalf("expected last-known clock, got %+v", row.Clock)
	}
	if row.LLMConfigured == nil || *row.LLMConfigured {
		t.Fatalf("expected llm_configured = false, got %v", row.LLMConfigured)
	}
	if row.LLM != nil {
		t.Error("stopped rows must not carry a live llm status")
	}
}

func TestPsRowForUnresponsive(t *testing.T) {
	inst := worlds.Instance{Name: "wedged", Path: "/w/wedged", State: worlds.Unresponsive, Pid: 555}
	row := psRowFor(inst)
	if row.State != "unresponsive" {
		t.Fatalf("state = %s", row.State)
	}
	if row.Daemon == nil || row.Daemon.Pid != 555 {
		t.Fatalf("expected pid carried through, got %+v", row.Daemon)
	}
	if row.Clock != nil {
		t.Error("unresponsive rows carry no clock data (never rendered as running, FR-002)")
	}
}

func TestPsRowForMissing(t *testing.T) {
	inst := worlds.Instance{Name: "ghost", Path: "/gone", State: worlds.Missing}
	row := psRowFor(inst)
	if row.State != "missing" || row.World != nil || row.Clock != nil || row.Daemon != nil {
		t.Fatalf("missing row must carry only name/path/state, got %+v", row)
	}
}

func TestPsRowForUnreadable(t *testing.T) {
	inst := worlds.Instance{Name: "corrupt", Path: "/bad", State: worlds.Unreadable, Error: "corrupt world.json: boom"}
	row := psRowFor(inst)
	if row.State != "unreadable" || row.Error == "" {
		t.Fatalf("unreadable row must carry an error, got %+v", row)
	}
	if row.World != nil || row.Clock != nil {
		t.Fatalf("unreadable row must not carry world/clock, got %+v", row)
	}
}

func TestPsTableFieldsDash(t *testing.T) {
	tick, gameTime, speed, llmOn := psTableFields(worlds.Instance{State: worlds.Unresponsive})
	if tick != "-" || gameTime != "-" || speed != "-" || llmOn {
		t.Errorf("expected all-dash fields for unresponsive, got (%q %q %q %v)", tick, gameTime, speed, llmOn)
	}
}

func TestPrintPsTableEmptyMeansNoWorldsRunning(t *testing.T) {
	out := captureStdout(t, func() { printPsTable(nil) })
	if !strings.Contains(out, "no worlds running") {
		t.Errorf("expected 'no worlds running', got %q", out)
	}
}

func TestPrintPsTableIncludesColumnsAndRow(t *testing.T) {
	instances := []worlds.Instance{{
		Name: "aria", Path: "/w/aria", State: worlds.Running, Pid: 4242,
		Status: &ipc.StatusData{Clock: ipc.ClockStatus{Tick: 180321, GameTime: "d3 02:05:21", Speed: "8x"}, LLM: &llm.Status{}},
	}}
	out := captureStdout(t, func() { printPsTable(instances) })
	for _, want := range []string{"NAME", "STATE", "PID", "TICK", "GAME TIME", "SPEED", "LLM", "PATH", "aria", "running", "4242", "180321", "8x", "on", "/w/aria"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}
