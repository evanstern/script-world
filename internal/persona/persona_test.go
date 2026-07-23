package persona

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/sim"
)

func TestGenesisWritesAllEightReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range sim.AgentNames {
		info, err := os.Stat(PersonaPath(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o444 {
			t.Errorf("%s persona mode = %o, want 0444 (AC#2)", name, perm)
		}
		data, err := os.ReadFile(PersonaPath(dir, name))
		if err != nil || len(data) == 0 {
			t.Errorf("%s persona unreadable/empty: %v", name, err)
		}
		if _, err := os.Stat(SoulPath(dir, name)); err != nil {
			t.Errorf("%s soul.md missing: %v", name, err)
		}
	}
}

func TestGenesisRunsOnce(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	if err := Genesis(dir); err == nil {
		t.Fatal("second genesis must refuse (personas are written exactly once)")
	}
}

func TestOSEnforcesReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	// The filesystem itself refuses writes (belt to the structural braces).
	if err := os.WriteFile(PersonaPath(dir, "Ash"), []byte("tampered"), 0o644); err == nil {
		t.Fatal("writing a persona should fail at the OS level")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	texts := Load(dir)
	for i, name := range sim.AgentNames {
		if texts[i] == "" {
			t.Errorf("persona %s loaded empty", name)
		}
	}
	// Worlds without personas degrade to empty strings, not errors.
	empty := Load(t.TempDir())
	if empty[0] != "" {
		t.Error("missing personas should load as empty strings")
	}
}

func TestEveryAgentHasAnAuthoredPersona(t *testing.T) {
	for _, name := range sim.AgentNames {
		if Texts[name] == "" {
			t.Errorf("no authored persona for %s", name)
		}
	}
}

// TestPersonaMapsSweepAligned (US3, spec AC US3-5, FR-007, SC-004): the
// index-aligned map sweep. For every sim.AgentNames entry, Texts, Anchors,
// DriftMarkers, and Secrets each carry a non-empty entry (DriftMarkers
// additionally a non-empty list of non-empty words), and none of the four
// maps carries a key outside sim.AgentNames. Gaining or losing an entry in
// any one map — checked both by total count and by per-name presence — fails
// this sweep.
func TestPersonaMapsSweepAligned(t *testing.T) {
	if len(Texts) != len(sim.AgentNames) {
		t.Errorf("Texts has %d entries, want %d (one per sim.AgentNames)", len(Texts), len(sim.AgentNames))
	}
	if len(Anchors) != len(sim.AgentNames) {
		t.Errorf("Anchors has %d entries, want %d", len(Anchors), len(sim.AgentNames))
	}
	if len(DriftMarkers) != len(sim.AgentNames) {
		t.Errorf("DriftMarkers has %d entries, want %d", len(DriftMarkers), len(sim.AgentNames))
	}
	if len(Secrets) != len(sim.AgentNames) {
		t.Errorf("Secrets has %d entries, want %d", len(Secrets), len(sim.AgentNames))
	}

	for _, name := range sim.AgentNames {
		if strings.TrimSpace(Texts[name]) == "" {
			t.Errorf("Texts[%s] is empty", name)
		}
		if strings.TrimSpace(Anchors[name]) == "" {
			t.Errorf("Anchors[%s] is empty", name)
		}
		if strings.TrimSpace(Secrets[name]) == "" {
			t.Errorf("Secrets[%s] is empty", name)
		}
		words := DriftMarkers[name]
		if len(words) == 0 {
			t.Errorf("DriftMarkers[%s] is empty", name)
		}
		for _, w := range words {
			if strings.TrimSpace(w) == "" {
				t.Errorf("DriftMarkers[%s] contains an empty word", name)
			}
		}
	}

	known := make(map[string]bool, len(sim.AgentNames))
	for _, n := range sim.AgentNames {
		known[n] = true
	}
	assertNoStrayKeys := func(label string, m map[string]string) {
		for name := range m {
			if !known[name] {
				t.Errorf("%s has a stray key %q outside sim.AgentNames", label, name)
			}
		}
	}
	assertNoStrayKeys("Texts", Texts)
	assertNoStrayKeys("Anchors", Anchors)
	assertNoStrayKeys("Secrets", Secrets)
	for name := range DriftMarkers {
		if !known[name] {
			t.Errorf("DriftMarkers has a stray key %q outside sim.AgentNames", name)
		}
	}
}

// TestAnchorsMatchTemperamentLine (US3, research.md R1, FR-007): each
// Anchors[name] string appears verbatim inside its persona's
// "**Temperament:**" line in Texts[name] — the documented "deliberately
// identical" invariant.
func TestAnchorsMatchTemperamentLine(t *testing.T) {
	for _, name := range sim.AgentNames {
		var tempLine string
		for _, line := range strings.Split(Texts[name], "\n") {
			if strings.HasPrefix(line, "**Temperament:**") {
				tempLine = line
				break
			}
		}
		if tempLine == "" {
			t.Fatalf("%s: no **Temperament:** line found in Texts", name)
		}
		if !strings.Contains(tempLine, Anchors[name]) {
			t.Errorf("%s: anchor %q not found verbatim in Temperament line %q", name, Anchors[name], tempLine)
		}
	}
}

// TestLoadUnreadableDegrades (US3, research.md R2, spec AC US3-3/US3-4): an
// unreadable persona file (mode 0000) degrades Load to an empty string for
// that agent only — every other agent's text stays intact. Mirrors the
// already-tested missing-file contract (Load's documented "any read error →
// empty string").
func TestLoadUnreadableDegrades(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits; the degrade can't be exercised")
	}
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	target := sim.AgentNames[0]
	path := PersonaPath(dir, target)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o444) })

	texts := Load(dir)
	for i, name := range sim.AgentNames {
		if name == target {
			if texts[i] != "" {
				t.Errorf("%s: unreadable persona loaded as %q, want empty", name, texts[i])
			}
			continue
		}
		if texts[i] == "" {
			t.Errorf("%s: unaffected persona loaded empty", name)
		}
	}
}

// TestGenesisSeedsCharterAndJournal (US3, data-model.md §2 genesis seeding):
// a fresh genesis seeds charter.md equal to DefaultCharter and a journal.md
// per agent bearing the rune-budget header; a charter.md already present in
// an empty world dir before genesis is never overwritten (Genesis errors on
// existing personas, so never-overwrite is asserted by seeding charter.md
// alone in an otherwise-empty world dir).
func TestGenesisSeedsCharterAndJournal(t *testing.T) {
	t.Run("fresh genesis seeds charter and journal", func(t *testing.T) {
		dir := t.TempDir()
		if err := Genesis(dir); err != nil {
			t.Fatal(err)
		}
		charter, err := os.ReadFile(filepath.Join(dir, "charter.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(charter) != DefaultCharter {
			t.Error("fresh-genesis charter.md != DefaultCharter")
		}
		for _, name := range sim.AgentNames {
			journal, err := os.ReadFile(JournalPath(dir, name))
			if err != nil {
				t.Fatalf("%s: journal.md missing: %v", name, err)
			}
			want := fmt.Sprintf("0/%d runes", sim.JournalBudgetRunes)
			if !strings.Contains(string(journal), want) {
				t.Errorf("%s: journal.md missing rune-budget header %q: %q", name, want, journal)
			}
		}
	})

	t.Run("genesis never overwrites an existing charter", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		custom := "# My custom charter\n"
		if err := os.WriteFile(filepath.Join(dir, "charter.md"), []byte(custom), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Genesis(dir); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "charter.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != custom {
			t.Errorf("charter.md overwritten by genesis: %q, want unchanged %q", got, custom)
		}
	})
}

// TestSecretEvents (US3, data-model.md §2 secret genesis): SecretEvents
// yields exactly one event per sim.AgentNames entry, each Tick 0 and type
// social.secret_seeded, payload Agent index-aligned with the name order,
// Tone -70, Text equal to Secrets[name].
func TestSecretEvents(t *testing.T) {
	events, err := SecretEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != len(sim.AgentNames) {
		t.Fatalf("SecretEvents = %d events, want %d (one per agent)", len(events), len(sim.AgentNames))
	}
	for i, e := range events {
		name := sim.AgentNames[i]
		if e.Tick != 0 {
			t.Errorf("%s: event tick = %d, want 0", name, e.Tick)
		}
		if e.Type != "social.secret_seeded" {
			t.Errorf("%s: event type = %q, want social.secret_seeded", name, e.Type)
		}
		var p sim.SecretSeededPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("%s: payload unmarshal: %v", name, err)
		}
		if p.Agent != i {
			t.Errorf("%s: payload Agent = %d, want %d (index-aligned)", name, p.Agent, i)
		}
		if p.Tone != -70 {
			t.Errorf("%s: payload Tone = %d, want -70", name, p.Tone)
		}
		if p.Text != Secrets[name] {
			t.Errorf("%s: payload Text = %q, want %q", name, p.Text, Secrets[name])
		}
	}
}
