package scribe

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

func mustPayloadJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSoulRendersFromEvents(t *testing.T) {
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)

	scr, err := New(dir, 42, m, state.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	defer scr.Close()

	scr.Observe([]store.Event{{
		Tick: 3600, Type: "agent.memory_added",
		Payload: mustPayloadJSON(t, map[string]any{"agent": 0, "text": "Built a fire.", "salience": 5}),
	}})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
		s := string(data)
		if strings.Contains(s, "Built a fire.") && strings.Contains(s, "(5★)") &&
			strings.Contains(s, "day 1 07:00") && strings.Contains(s, "1 memories") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
	t.Fatalf("soul.md never rendered the memory; content:\n%s", data)
}

func TestDeathFreezesSoulHeader(t *testing.T) {
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	scr, err := New(dir, 42, m, state.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	defer scr.Close()

	scr.Observe([]store.Event{{
		Tick: 7200, Type: "agent.died",
		Payload: mustPayloadJSON(t, map[string]any{"agent": 1, "cause": "exposure"}),
	}})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Birch"))
		if strings.Contains(string(data), "Dead") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("soul header never marked death")
}
