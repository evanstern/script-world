package mind

import (
	"strings"
	"testing"
)

// The villager system-prompt frame contract (spec 027,
// contracts/system-prompt.md C1–C5). These tests are meaning-pinned, not
// wording-pinned: doctrine is asserted by what the frame must convey (C3), so a
// craft rewrite of the phrasing keeps them green while a doctrine drift turns
// them red.

// sentinelName is collision-proof: it must not occur anywhere in the frame's
// static text or in the sample personas, so counting it isolates the identity
// statement's single interpolation (C2).
const sentinelName = "Zzyzxonymously"

// framePersona is a representative persona that deliberately does NOT contain
// sentinelName, so it can be stripped to leave the pure frame text (C2/C4).
const framePersona = `# A Villager

**Temperament:** steady, practical, slow to anger.
**Drives:** keep everyone fed; distrusts idleness.`

// frameWithoutPersona returns the rendered frame with the interpolated persona
// text removed, so assertions see only the static frame (contracts C2/C4: the
// persona text is exempt from frame-text rules).
func frameWithoutPersona(t *testing.T, name, persona string) string {
	t.Helper()
	rendered := systemPrompt(name, persona)
	if persona == "" {
		return rendered
	}
	if !strings.Contains(rendered, persona) {
		t.Fatalf("persona text not present verbatim in rendered frame (C4)")
	}
	return strings.Replace(rendered, persona, "", 1)
}

// C1 — purity / cacheability: identical inputs render byte-identical output,
// across repeated calls (SC-005). The signature (name, personaText only)
// structurally forbids dynamic world state; this pins the determinism half.
func TestSystemPromptPurity(t *testing.T) {
	a := systemPrompt(sentinelName, framePersona)
	for i := 0; i < 8; i++ {
		if b := systemPrompt(sentinelName, framePersona); b != a {
			t.Fatalf("render %d differs from first render — prompt is not a pure function (C1)", i)
		}
	}
	// Distinct names differ only where the identity statement (and persona)
	// differ — here personas match, so any difference is the name.
	if systemPrompt("Aria", framePersona) == systemPrompt("Bram", framePersona) {
		t.Fatalf("renders for distinct names are identical — the identity statement is not naming the agent (C1)")
	}
}

// C2 — single naming: the frame text (persona exempt) contains the agent's name
// exactly once. MUST FAIL against the pre-rewrite prompt, which repeats it.
func TestSystemPromptNamesOnce(t *testing.T) {
	frame := frameWithoutPersona(t, sentinelName, framePersona)
	if n := strings.Count(frame, sentinelName); n != 1 {
		t.Fatalf("frame text names the agent %d times, want exactly 1 (C2, SC-002)", n)
	}
	// Also holds with an empty persona (the whole render is frame text).
	if n := strings.Count(systemPrompt(sentinelName, ""), sentinelName); n != 1 {
		t.Fatalf("empty-persona frame names the agent %d times, want exactly 1 (C2)", n)
	}
}

// C3 — doctrine (meaning-pinned, wording-free). The frame must convey the four
// invariants regardless of phrasing.
func TestSystemPromptDoctrine(t *testing.T) {
	frame := frameWithoutPersona(t, sentinelName, framePersona)
	lower := strings.ToLower(frame)

	// 1. Acting-tool-only: the decision is made by calling exactly ONE acting tool.
	if !strings.Contains(lower, "exactly one") {
		t.Errorf("doctrine 1 (acting-tool-only): frame does not state the decision is exactly one call (C3)")
	}
	if !strings.Contains(lower, "tool") {
		t.Errorf("doctrine 1 (acting-tool-only): frame does not reference tools (C3)")
	}

	// 2. Read-then-act: read-only tools may precede the one acting call.
	if !strings.Contains(lower, "read") {
		t.Errorf("doctrine 2 (read-then-act): frame does not mention read-only tools (C3)")
	}
	if !(strings.Contains(lower, "first") || strings.Contains(lower, "before") || strings.Contains(lower, "then")) {
		t.Errorf("doctrine 2 (read-then-act): frame does not convey read-before-act ordering (C3)")
	}

	// 3. Muse-is-an-action with opportunity-cost framing: muse and set_plan are
	// themselves acting choices, and spending a beat thinking costs a beat of
	// doing (this exact idea, not necessarily these words).
	if !strings.Contains(lower, "muse") {
		t.Errorf("doctrine 3 (muse-is-an-action): frame does not mention muse (C3)")
	}
	if !strings.Contains(lower, "set_plan") {
		t.Errorf("doctrine 3 (muse-is-an-action): frame does not mention set_plan as an action (C3)")
	}
	// opportunity cost: contrast a thinking beat against a doing beat.
	thinks := strings.Contains(lower, "think") || strings.Contains(lower, "musing")
	acts := strings.Contains(lower, "doing") || strings.Contains(lower, "acting") || strings.Contains(lower, "act")
	if !(thinks && acts) {
		t.Errorf("doctrine 3 (muse-is-an-action): frame does not convey the thinking-vs-doing opportunity cost (C3)")
	}

	// 4. No free-text path: the frame never invites a prose/JSON text answer.
	for _, banned := range []string{"respond with", "reply with", "json", "output format", "free text", "free-text", "in the form of"} {
		if strings.Contains(lower, banned) {
			t.Errorf("doctrine 4 (no free-text path): frame offers a text/format answer channel via %q (C3)", banned)
		}
	}
}

// C4 — persona block: verbatim, its own block between identity and task framing;
// empty persona renders a clean frame (no doubled blank lines / dangling
// separator).
func TestSystemPromptPersonaBlock(t *testing.T) {
	// Verbatim + ordered: identity (the name) before persona before task framing.
	rendered := systemPrompt(sentinelName, framePersona)
	if !strings.Contains(rendered, framePersona) {
		t.Fatalf("persona text not present verbatim (C4)")
	}
	iName := strings.Index(rendered, sentinelName)
	iPersona := strings.Index(rendered, framePersona)
	iTask := strings.Index(strings.ToLower(rendered), "exactly one")
	if !(iName >= 0 && iPersona > iName && iTask > iPersona) {
		t.Fatalf("frame parts out of order: name@%d persona@%d task@%d, want name<persona<task (C4)", iName, iPersona, iTask)
	}

	// Empty persona: clean render, no tripled newline where the block would be,
	// and identity still precedes task framing.
	empty := systemPrompt(sentinelName, "")
	if strings.Contains(empty, "\n\n\n") {
		t.Errorf("empty-persona render has a doubled blank line / dangling separator (C4): %q", empty)
	}
	if ei, et := strings.Index(empty, sentinelName), strings.Index(strings.ToLower(empty), "exactly one"); !(ei >= 0 && et > ei) {
		t.Errorf("empty-persona frame parts out of order: name@%d task@%d (C4)", ei, et)
	}
}

// TestPromptFrameReport renders the frame for a fixed representative sample
// agent and logs byte / word / approximate-token counts (research D4:
// approx tokens = len(bytes)/4). Run with `-run TestPromptFrameReport -v` at
// each variant's git ref; the numbers land in eval/<variant>.md (SC-004).
func TestPromptFrameReport(t *testing.T) {
	const sampleName = "Ash"
	const samplePersona = `# Ash

**Temperament:** steady, practical, slow to anger.
**Drives:** keep everyone fed; distrusts idleness.
**Quirk:** talks to the fire as if it answers.
**Bonds:** protective of Fern; an old, quiet rivalry with Oak.
`
	frame := systemPrompt(sampleName, samplePersona)
	bytes := len(frame)
	words := len(strings.Fields(frame))
	tokensApprox := bytes / 4
	t.Logf("PROMPT_FRAME_REPORT sample=%q prompt_bytes=%d prompt_words=%d prompt_tokens_approx=%d",
		sampleName, bytes, words, tokensApprox)
}
