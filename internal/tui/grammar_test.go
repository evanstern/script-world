package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

var testNames = []string{"Ash", "Birch", "Cedar", "Rowan"}

// TestEventFamilyOf is the digest-grammar contract's family derivation (R2):
// namespace prefix, with meeting/norm merged into one governance family.
func TestEventFamilyOf(t *testing.T) {
	cases := []struct {
		eventType string
		want      eventFamily
	}{
		{"world.created", familyWorld},
		{"clock.paused", familyClock},
		{"sim.day_started", familySim},
		{"agent.moved", familyAgent},
		{"social.gave", familySocial},
		{"meeting.opened", familyGovernance},
		{"norm.violated", familyGovernance},
		{"gru.emerged", familyGru},
		{"chronicle.entry", familyChronicle},
		{"metatron.nudged", familyMetatron},
		{"daemon.started", familyDaemon},
		{"cog.thought", familyCog},
		{"future.unknown_type", familyUnknown}, // new namespaces land here until promoted
		{"no-dot-at-all", familyUnknown},
	}
	for _, c := range cases {
		if got := eventFamilyOf(c.eventType); got != c.want {
			t.Errorf("eventFamilyOf(%q) = %v, want %v", c.eventType, got, c.want)
		}
	}
}

// TestFormatChronicleLineFallback: an unregistered type (or one whose
// payload fails to unmarshal against its registered digestFunc) falls back
// to the pre-digest compact resolved-name JSON as one segText span — never
// blank, never a panic (FR-002/FR-003).
func TestFormatChronicleLineFallback(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 60, Type: "future.unknown_type",
		Payload: json.RawMessage(`{"agent":0,"x":1,"y":1}`)}
	l := formatChronicleLine(e, testNames)
	if l.Family != familyUnknown {
		t.Errorf("family = %v, want familyUnknown", l.Family)
	}
	if len(l.Summary) != 1 || l.Summary[0].Role != segText {
		t.Fatalf("fallback summary should be exactly one segText span: %+v", l.Summary)
	}
	if want := `{"agent":"Ash","x":1,"y":1}`; l.Summary[0].Text != want {
		t.Errorf("fallback text = %q, want %q", l.Summary[0].Text, want)
	}
}

// TestFormatChronicleLineFallbackOnUnmarshalFailure: a registered type whose
// payload doesn't unmarshal (digestFunc returns ok=false) falls back exactly
// like a registry miss.
func TestFormatChronicleLineFallbackOnUnmarshalFailure(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "agent.moved", Payload: json.RawMessage(`not json`)}
	l := formatChronicleLine(e, testNames)
	if len(l.Summary) != 1 || l.Summary[0].Role != segText {
		t.Fatalf("unmarshal failure should fall back to one segText span: %+v", l.Summary)
	}
}

// TestFormatChronicleLineSpeechPrivilege: social.conversation_turn and
// social.rumor_told carry the speech privilege (segSpeech role on the
// quoted utterance) per contract §3.
func TestFormatChronicleLineSpeechPrivilege(t *testing.T) {
	e := store.Event{Seq: 1201, Tick: 8846, Type: "social.conversation_turn",
		Payload: json.RawMessage(`{"conv":102,"speaker":3,"listener":0,"text":"I stacked wood at dawn"}`)}
	l := formatChronicleLine(e, testNames)
	if want := `Rowan→Ash "I stacked wood at dawn"`; plainSegs(l.Summary) != want {
		t.Errorf("plain summary = %q, want %q", plainSegs(l.Summary), want)
	}
	if !anyRole(l.Summary, segSpeech) {
		t.Error("conversation_turn summary should carry a segSpeech span")
	}
	if !anyRole(l.Summary, segName) {
		t.Error("conversation_turn summary should carry segName spans for both agents")
	}

	rumor := store.Event{Seq: 1203, Tick: 8900, Type: "social.rumor_told",
		Payload: json.RawMessage(`{"from":1,"to":2,"rumor_id":0,"subject":0,"tone":30,"text":"ash lets the fire die","confidence":40}`)}
	l2 := formatChronicleLine(rumor, testNames)
	if want := `Birch→Cedar rumor: "ash lets the fire die"`; plainSegs(l2.Summary) != want {
		t.Errorf("rumor plain summary = %q, want %q", plainSegs(l2.Summary), want)
	}
	if !anyRole(l2.Summary, segSpeech) {
		t.Error("rumor_told summary should carry a segSpeech span")
	}
}

// TestFormatChronicleLineOutOfRangeIndex: an index beyond the roster
// renders as "#N" rather than panicking or silently dropping the field —
// exercised here through a registered digest (agent.moved).
func TestFormatChronicleLineOutOfRangeIndex(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "agent.moved",
		Payload: json.RawMessage(`{"agent":99,"x":3,"y":4}`)}
	l := formatChronicleLine(e, testNames)
	if want := `#99 → (3,4)`; plainSegs(l.Summary) != want {
		t.Errorf("summary = %q, want %q", plainSegs(l.Summary), want)
	}
}

// TestFormatChronicleLineHail: the hail family (registered digests) resolves
// agent indices to names in the contract's natural-phrase voice.
func TestFormatChronicleLineHail(t *testing.T) {
	cases := []struct {
		eventType string
		payload   string
		want      string
	}{
		{"social.hailed", `{"from":1,"to":3,"until":12345}`, `Birch hailed Rowan (until t12345)`},
		{"social.hail_met", `{"from":1,"to":3}`, `Birch met Rowan`},
		{"social.hail_expired", `{"from":0,"to":2}`, `Ash's hail to Cedar lapsed`},
	}
	for _, c := range cases {
		e := store.Event{Seq: 1, Tick: 1, Type: c.eventType, Payload: json.RawMessage(c.payload)}
		l := formatChronicleLine(e, testNames)
		if got := plainSegs(l.Summary); got != c.want {
			t.Errorf("%s summary = %q, want %q", c.eventType, got, c.want)
		}
	}
}

// TestFormatChronicleLineStorage (T034, migrated to the digest registry):
// the storage-family digests resolve owner/taker to names, matching the
// contract's natural-phrase templates.
func TestFormatChronicleLineStorage(t *testing.T) {
	cases := []struct {
		eventType string
		payload   string
		want      string
	}{
		{"agent.dropped", `{"agent":0,"x":3,"y":4,"kind":"wood","n":2}`, `Ash dropped 2 wood at (3,4)`},
		{"agent.picked_up", `{"agent":1,"x":3,"y":4,"kind":"wood","n":2}`, `Birch picked up 2 wood at (3,4)`},
		{"agent.deposited", `{"agent":2,"x":5,"y":5,"kind":"planks","n":6}`, `Cedar stored 6 planks in the chest at (5,5)`},
		{"agent.withdrew", `{"agent":3,"x":5,"y":5,"kind":"planks","n":1,"owner":0}`, `Rowan took 1 planks from Ash's chest`},
		{"agent.withdrew", `{"agent":0,"x":5,"y":5,"kind":"planks","n":1,"owner":0}`, `Ash took 1 planks from their chest`},
		{"social.chest_taken", `{"owner":0,"taker":3,"x":5,"y":5}`, `Rowan raided Ash's chest at (5,5)`},
		{"sim.food_rotted", `{"x":6,"y":6,"kind":"food_raw","n":4}`, `4 food_raw rotted at (6,6)`},
	}
	for _, c := range cases {
		e := store.Event{Seq: 1, Tick: 1, Type: c.eventType, Payload: json.RawMessage(c.payload)}
		l := formatChronicleLine(e, testNames)
		if got := plainSegs(l.Summary); got != c.want {
			t.Errorf("%s summary = %q, want %q", c.eventType, got, c.want)
		}
	}
}

// TestResolvePayloadNames: order preserved, unrelated fields untouched,
// out-of-range indices fall back to the raw match. (fallback path, unchanged)
func TestResolvePayloadNames(t *testing.T) {
	raw := json.RawMessage(`{"agent":2,"x":7,"y":8}`)
	got := resolvePayloadNames(raw, testNames)
	if want := `{"agent":"Cedar","x":7,"y":8}`; got != want {
		t.Errorf("resolvePayloadNames = %q, want %q", got, want)
	}

	oob := json.RawMessage(`{"agent":99,"x":1,"y":1}`)
	got2 := resolvePayloadNames(oob, testNames)
	if want := `{"agent":99,"x":1,"y":1}`; got2 != want {
		t.Errorf("out-of-range index should pass through unchanged: got %q, want %q", got2, want)
	}
}

// TestFormatInspector: the stored event verbatim, seq/tick/type intact,
// integer indices intact, with a trailing "// name" annotation — never a
// payload rewrite (chronicle-grammar.md "Inspector", contract §5). Unchanged
// by the digest grammar — the inspector's contract is untouched.
func TestFormatInspector(t *testing.T) {
	e := store.Event{Seq: 1202, Tick: 8846, Type: "social.conversation_turn",
		Payload: json.RawMessage(`{"conv":102,"speaker":1,"listener":0,"text":"I stacked wood at dawn, ask Birch"}`)}
	got := formatInspector(e, testNames)

	for _, want := range []string{
		`"seq": 1202`, `"tick": 8846`,
		`"type": "social.conversation_turn"`,
		`"conv": 102`,
		`"speaker": 1`, `// Birch`,
		`"listener": 0`, `// Ash`,
		`"text": "I stacked wood at dawn, ask Birch"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("inspector missing %q in:\n%s", want, got)
		}
	}
	// Field order preserved: speaker before listener before text.
	if strings.Index(got, `"speaker"`) > strings.Index(got, `"listener"`) {
		t.Error("field order not preserved")
	}
}

// TestFormatInspectorNoNamesAvailable: an empty roster (disconnected) must
// not panic and must still show the raw indices.
func TestFormatInspectorNoNamesAvailable(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "social.conversation_turn",
		Payload: json.RawMessage(`{"speaker":1,"listener":0,"text":"hi"}`)}
	got := formatInspector(e, nil)
	if !strings.Contains(got, `"speaker": 1`) {
		t.Errorf("inspector with no names should still show raw indices: %q", got)
	}
	if strings.Contains(got, "// ") {
		t.Errorf("no annotation should be added when names are unavailable: %q", got)
	}
}

// TestPlainChronicleLine: solo shows the right-aligned tick column; dock
// drops it (contract §1).
func TestPlainChronicleLine(t *testing.T) {
	l := chronicleLine{Tick: 42, Time: "08:11", Type: "agent.moved", Summary: []seg{txt("Ash → (1,1)")}}

	solo := computeChronicleColumns([]chronicleLine{l}, false)
	if got, want := plainChronicleLine(l, solo), `42 08:11  agent.moved  Ash → (1,1)`; got != want {
		t.Errorf("plainChronicleLine(solo) = %q, want %q", got, want)
	}

	dock := computeChronicleColumns([]chronicleLine{l}, true)
	if got, want := plainChronicleLine(l, dock), `08:11 moved  Ash → (1,1)`; got != want {
		t.Errorf("plainChronicleLine(dock) = %q, want %q", got, want)
	}
}

// TestComputeChronicleColumnsAlignment: tick right-aligned to the widest
// visible tick; type padded to the widest visible type, capped (R5).
func TestComputeChronicleColumnsAlignment(t *testing.T) {
	lines := []chronicleLine{
		{Tick: 5, Time: "06:00", Type: "clock.paused", Summary: []seg{txt("paused")}},
		{Tick: 12345, Time: "08:11", Type: "social.conversation_turn", Summary: []seg{txt("x")}},
	}
	cols := computeChronicleColumns(lines, false)
	if cols.TickWidth != 5 {
		t.Errorf("TickWidth = %d, want 5 (len of the widest tick, %q)", cols.TickWidth, "12345")
	}
	if cols.TypeWidth != len("social.conversation_turn") {
		t.Errorf("TypeWidth = %d, want %d", cols.TypeWidth, len("social.conversation_turn"))
	}

	// A pathologically long type name truncates at the cap rather than
	// blowing the column out past 26 (solo).
	longType := "future.a_very_long_event_type_name_indeed"
	capped := computeChronicleColumns([]chronicleLine{{Tick: 1, Type: longType}}, false)
	if capped.TypeWidth != typeColumnCapSolo {
		t.Errorf("TypeWidth = %d, want cap %d", capped.TypeWidth, typeColumnCapSolo)
	}
	padded := padType(longType, capped)
	if got := len([]rune(padded)); got != typeColumnCapSolo {
		t.Errorf("padded type width = %d, want %d", got, typeColumnCapSolo)
	}
	if !strings.HasSuffix(padded, "…") {
		t.Errorf("truncated type should end with an ellipsis: %q", padded)
	}
}

// TestWrapOrTruncatePlain: solo/narrow (maxWrap<=1) truncates with "…";
// dock (maxWrap>1) wraps up to maxWrap lines, truncating the last.
// Unchanged by the digest grammar rework (T005).
func TestWrapOrTruncatePlain(t *testing.T) {
	short := "short line"
	if got := wrapOrTruncatePlain(short, 40, 1); len(got) != 1 || got[0] != short {
		t.Errorf("short line under width should pass through: %v", got)
	}

	long := strings.Repeat("x", 50)
	got := wrapOrTruncatePlain(long, 20, 1)
	if len(got) != 1 {
		t.Fatalf("maxWrap=1 must yield exactly one line: %v", got)
	}
	if !strings.HasSuffix(got[0], "…") {
		t.Errorf("truncated line must end with an ellipsis: %q", got[0])
	}
	if len([]rune(got[0])) != 20 {
		t.Errorf("truncated line width = %d, want 20", len([]rune(got[0])))
	}

	wrapped := wrapOrTruncatePlain("one two three four five six seven eight nine ten", 12, 3)
	if len(wrapped) > 3 {
		t.Fatalf("dock wrap must cap at maxWrap=3 lines: %v", wrapped)
	}
	if !strings.HasSuffix(wrapped[len(wrapped)-1], "…") && len(wrapped) == 3 {
		// Only the capped case must show truncation; a run that wrapped
		// itself under 3 lines is not truncated.
		t.Logf("wrapped output: %v", wrapped)
	}
}

// anyRole reports whether any seg in segs carries the given role.
func anyRole(segs []seg, role segRole) bool {
	for _, s := range segs {
		if s.Role == role {
			return true
		}
	}
	return false
}

// --- styleWrapLine (T021/R4): segment-wise styling must never change what
// gets displayed, only how it's colored — these tests strip the role
// attribution back to plain text and compare against wrapOrTruncatePlain's
// own output for the identical source line, across the whole catalog.

// plainOf concatenates a []styledLine's runes, one physical line per output
// entry — the "what would render on screen, ignoring color" projection.
func plainOf(lines []styledLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = string(l.Runes)
	}
	return out
}

// TestStyleWrapLinePlainEquivalence: for every catalog fixture type, at
// both solo (maxWrap=1) and dock (maxWrap=3, narrow width) geometry,
// styleWrapLine's plain-text projection must exactly match
// wrapOrTruncatePlain(plainChronicleLine(...)) — the pre-existing,
// already-tested wrap/truncate behavior (T005) must be bit-for-bit
// unchanged by the styling rework.
func TestStyleWrapLinePlainEquivalence(t *testing.T) {
	names := []string{"Ash", "Birch", "Cedar", "Rowan"}
	widths := []struct {
		width, maxWrap int
		dock           bool
	}{
		{60, 1, false}, // solo
		{18, 3, true},  // dock: narrow enough to force wraps on most digests
	}
	for typ, fx := range catalogFixture {
		e := store.Event{Seq: 1, Tick: 12345, Type: typ, Payload: json.RawMessage(fx.payload)}
		l := formatChronicleLine(e, names)
		for _, w := range widths {
			cols := computeChronicleColumns([]chronicleLine{l}, w.dock)
			plain := wrapOrTruncatePlain(plainChronicleLine(l, cols), w.width, w.maxWrap)
			prefix := chronicleLinePrefix(l, cols)
			styled := plainOf(styleWrapLine(prefix, l.Summary, w.width, w.maxWrap))
			if len(styled) != len(plain) {
				t.Fatalf("%s (width=%d dock=%v): styleWrapLine produced %d lines, wrapOrTruncatePlain produced %d\nstyled: %v\nplain:  %v",
					typ, w.width, w.dock, len(styled), len(plain), styled, plain)
			}
			for i := range plain {
				if styled[i] != plain[i] {
					t.Errorf("%s (width=%d dock=%v) line %d:\nstyled: %q\nplain:  %q", typ, w.width, w.dock, i, styled[i], plain[i])
				}
			}
		}
	}
}

// TestStyleWrapLineMidWordRoleBoundary: a role boundary that falls
// mid-word (agent.spear_broke: name "Ash" immediately followed by
// "'s spear broke", no space) must keep "Ash's" as one unbroken word — not
// split into "Ash" and "'s" by a spuriously inserted wrap-space — while
// still carrying the correct role per half (name, then plain text). This is
// exactly the case a naive per-seg-independent word split (instead of
// flattening to one string before finding word boundaries) would corrupt.
func TestStyleWrapLineMidWordRoleBoundary(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "agent.spear_broke", Payload: json.RawMessage(`{"agent":0}`)}
	l := formatChronicleLine(e, testNames)
	if want := "Ash's spear broke"; plainSegs(l.Summary) != want {
		t.Fatalf("setup: summary = %q, want %q", plainSegs(l.Summary), want)
	}
	cols := computeChronicleColumns([]chronicleLine{l}, true)
	prefix := chronicleLinePrefix(l, cols)
	lines := styleWrapLine(prefix, l.Summary, 12, 3)

	full := strings.Join(plainOf(lines), " ")
	if !strings.Contains(full, "Ash's") {
		t.Fatalf("mid-word boundary corrupted across wrap — \"Ash's\" split apart: %v", plainOf(lines))
	}
	for _, ln := range lines {
		idx := strings.Index(string(ln.Runes), "Ash's")
		if idx < 0 {
			continue
		}
		r := []rune(string(ln.Runes))
		// "Ash" (3 runes at idx) must be styleRoleName; "'s" (2 runes right
		// after) must NOT be — the seg boundary must not leak past "Ash".
		for i := idx; i < idx+3 && i < len(ln.Roles); i++ {
			if ln.Roles[i] != styleRoleName {
				t.Errorf("rune %q at %d (\"Ash\") = role %v, want styleRoleName", r[i], i, ln.Roles[i])
			}
		}
		for i := idx + 3; i < idx+5 && i < len(ln.Roles); i++ {
			if ln.Roles[i] == styleRoleName {
				t.Errorf("rune %q at %d (\"'s\") incorrectly carries styleRoleName", r[i], i)
			}
		}
	}
}
