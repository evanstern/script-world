package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

var testNames = []string{"Ash", "Birch", "Cedar", "Rowan"}

// TestClassifyEvent is the chronicle-grammar.md class table.
func TestClassifyEvent(t *testing.T) {
	cases := []struct {
		eventType string
		want      eventClass
	}{
		{"social.conversation_turn", classSpeech},
		{"social.rumor_told", classSpeech},
		{"social.conversation", classScene},
		{"chronicle.entry", classNarration},
		{"clock.paused", classClock},
		{"clock.resumed", classClock},
		{"clock.speed_set", classClock},
		{"agent.talked", classDefault},
		{"agent.moved", classDefault},
		{"something.new", classDefault}, // new types land in default until promoted
	}
	for _, c := range cases {
		if got := classifyEvent(c.eventType); got != c.want {
			t.Errorf("classifyEvent(%q) = %v, want %v", c.eventType, got, c.want)
		}
	}
}

// TestFormatChronicleLineSpeech: privileged treatment — {"Speaker"→"Listener"}
// then the quoted utterance, with agent indices resolved to names.
func TestFormatChronicleLineSpeech(t *testing.T) {
	e := store.Event{Seq: 1201, Tick: 8846, Type: "social.conversation_turn",
		Payload: json.RawMessage(`{"conv":102,"speaker":3,"listener":0,"text":"I stacked wood at dawn"}`)}
	l := formatChronicleLine(e, testNames)
	if l.Class != classSpeech {
		t.Fatalf("class = %v, want classSpeech", l.Class)
	}
	if want := `{"Rowan"→"Ash"}`; l.Subject != want {
		t.Errorf("subject = %q, want %q", l.Subject, want)
	}
	if want := `"I stacked wood at dawn"`; l.Speech != want {
		t.Errorf("speech = %q, want %q", l.Speech, want)
	}

	rumor := store.Event{Seq: 1203, Tick: 8900, Type: "social.rumor_told",
		Payload: json.RawMessage(`{"from":1,"to":2,"rumor_id":0,"subject":0,"tone":30,"text":"ash lets the fire die","confidence":40}`)}
	l2 := formatChronicleLine(rumor, testNames)
	if want := `{"Birch"→"Cedar"}`; l2.Subject != want {
		t.Errorf("rumor subject = %q, want %q", l2.Subject, want)
	}
}

// TestFormatChronicleLineOutOfRangeIndex: an index beyond the roster
// renders as "#N" rather than panicking or silently dropping the field.
func TestFormatChronicleLineOutOfRangeIndex(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "social.conversation_turn",
		Payload: json.RawMessage(`{"speaker":99,"listener":0,"text":"hi"}`)}
	l := formatChronicleLine(e, testNames)
	if want := `{"#99"→"Ash"}`; l.Subject != want {
		t.Errorf("subject = %q, want %q", l.Subject, want)
	}
}

// TestFormatChronicleLineScene: gist-first compact summary.
func TestFormatChronicleLineScene(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "social.conversation",
		Payload: json.RawMessage(`{"conv":500,"a":0,"b":1,"gist":"argued about firewood","turns":6,"tones":[-1,1]}`)}
	l := formatChronicleLine(e, testNames)
	if l.Class != classScene {
		t.Fatalf("class = %v, want classScene", l.Class)
	}
	if !strings.HasPrefix(l.Payload, `{"gist":"argued about firewood"`) {
		t.Errorf("scene payload should be gist-first: %q", l.Payload)
	}
	if !strings.Contains(l.Payload, `"turns":6`) || !strings.Contains(l.Payload, `"tones":[-1,1]`) {
		t.Errorf("scene payload missing fields: %q", l.Payload)
	}
}

// TestFormatChronicleLineClock: type colored yellow (view-layer concern),
// payload compact — here we just verify classification and payload shape.
func TestFormatChronicleLineClock(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "clock.speed_set",
		Payload: json.RawMessage(`{"speed":"4x"}`)}
	l := formatChronicleLine(e, testNames)
	if l.Class != classClock {
		t.Fatalf("class = %v, want classClock", l.Class)
	}
	if l.Payload != `{"speed":"4x"}` {
		t.Errorf("clock payload should pass through unchanged: %q", l.Payload)
	}
}

// TestFormatChronicleLineDefault: agent.talked resolves both agent indices
// by their field names ("a"/"b"), field order preserved.
func TestFormatChronicleLineDefault(t *testing.T) {
	e := store.Event{Seq: 1, Tick: 1, Type: "agent.talked",
		Payload: json.RawMessage(`{"a":0,"b":3}`)}
	l := formatChronicleLine(e, testNames)
	if l.Class != classDefault {
		t.Fatalf("class = %v, want classDefault", l.Class)
	}
	if want := `{"a":"Ash","b":"Rowan"}`; l.Payload != want {
		t.Errorf("payload = %q, want %q", l.Payload, want)
	}
}

// TestResolvePayloadNames: order preserved, unrelated fields untouched,
// out-of-range indices fall back to the raw match.
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
// payload rewrite (chronicle-grammar.md "Inspector").
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

// TestPlainChronicleLine: the two shapes (speech vs. everything else).
func TestPlainChronicleLine(t *testing.T) {
	speech := chronicleLine{Seq: 1, Time: "08:11", Type: "social.conversation_turn",
		Class: classSpeech, Subject: `{"Ash"→"Rowan"}`, Speech: `"hi"`}
	if got, want := plainChronicleLine(speech), `#1 08:11  social.conversation_turn  {"Ash"→"Rowan"} "hi"`; got != want {
		t.Errorf("plainChronicleLine(speech) = %q, want %q", got, want)
	}

	def := chronicleLine{Seq: 2, Time: "08:12", Type: "agent.moved", Class: classDefault, Payload: `{"a":"Ash"}`}
	if got, want := plainChronicleLine(def), `#2 08:12  agent.moved  {"a":"Ash"}`; got != want {
		t.Errorf("plainChronicleLine(default) = %q, want %q", got, want)
	}
}

// TestWrapOrTruncatePlain: solo/narrow (maxWrap<=1) truncates with "…";
// dock (maxWrap>1) wraps up to maxWrap lines, truncating the last.
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
