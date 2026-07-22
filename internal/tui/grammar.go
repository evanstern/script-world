package tui

// Chronicle grammar (TASK-34): how one event becomes one feed entry. See
// docs/design/tui/patterns/chronicle-grammar.md. Kept as pure functions over
// store.Event + the replica's agent-name table so they're directly
// table-driven-testable without a Bubble Tea program.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
)

// eventClass is the chronicle-grammar.md class table.
type eventClass int

const (
	classDefault eventClass = iota
	classSpeech
	classScene
	classNarration
	classClock
)

// classifyEvent implements the chronicle-grammar.md class table. New event
// types land in classDefault until a row here promotes them.
func classifyEvent(eventType string) eventClass {
	switch eventType {
	case "social.conversation_turn", "social.rumor_told":
		return classSpeech
	case "social.conversation":
		return classScene
	case "chronicle.entry":
		return classNarration
	case "clock.paused", "clock.resumed", "clock.speed_set":
		return classClock
	default:
		return classDefault
	}
}

// agentIndexFields are the payload keys whose integer value is a resolvable
// agent index — the generalized form of the existing chronNames mechanism,
// applied to raw event payloads instead of narrated chronicle entries.
var agentIndexFields = map[string]bool{
	"agent": true, "a": true, "b": true,
	"from": true, "to": true,
	"speaker": true, "listener": true, "subject": true,
	// spec 013 (storage): agent.withdrew's Owner and social.chest_taken's
	// Owner/Taker are agent-index fields too (T034 gap — chest_taken was
	// added as chronicle/TUI material per its doc comment, but owner/taker
	// weren't in this table, so the raw feed and inspector rendered them as
	// bare integers instead of names).
	"owner": true, "taker": true,
}

// agentIndexFieldRe matches `"<field>":<int>` for the known agent-index
// fields, in the original payload's field order.
var agentIndexFieldRe = regexp.MustCompile(`"(agent|a|b|from|to|speaker|listener|subject|owner|taker)":(-?\d+)`)

// agentName resolves an index against the roster; out-of-range indices
// render as "#N" rather than panicking or silently dropping the field.
func agentName(names []string, idx int) string {
	if idx >= 0 && idx < len(names) {
		return names[idx]
	}
	return fmt.Sprintf("#%d", idx)
}

// resolvePayloadNames rewrites known agent-index fields in a compact JSON
// payload to quoted names, in place, preserving field order — the feed
// line's payload is a *view* (see chronicle-grammar.md).
func resolvePayloadNames(raw json.RawMessage, names []string) string {
	return agentIndexFieldRe.ReplaceAllStringFunc(string(raw), func(match string) string {
		sub := agentIndexFieldRe.FindStringSubmatch(match)
		field, numStr := sub[1], sub[2]
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 0 || n >= len(names) {
			return match
		}
		nameJSON, _ := json.Marshal(names[n])
		return fmt.Sprintf("%q:%s", field, string(nameJSON))
	})
}

// speechFields extracts the (speaker, listener, text) triple from the two
// speech-class payload shapes.
func speechFields(e store.Event) (from, to int, text string, ok bool) {
	switch e.Type {
	case "social.conversation_turn":
		var p sim.ConversationTurnPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return 0, 0, "", false
		}
		return p.Speaker, p.Listener, p.Text, true
	case "social.rumor_told":
		var p sim.RumorToldPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return 0, 0, "", false
		}
		return p.From, p.To, p.Text, true
	}
	return 0, 0, "", false
}

// sceneSummary builds the gist-first compact form for social.conversation.
func sceneSummary(e store.Event) string {
	var p sim.ConversationPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return string(e.Payload)
	}
	var b strings.Builder
	b.WriteByte('{')
	gistJSON, _ := json.Marshal(p.Gist)
	fmt.Fprintf(&b, "\"gist\":%s,\"turns\":%d", gistJSON, p.Turns)
	if len(p.Tones) > 0 {
		tonesJSON, _ := json.Marshal(p.Tones)
		fmt.Fprintf(&b, ",\"tones\":%s", tonesJSON)
	}
	b.WriteByte('}')
	return b.String()
}

// chronicleLine is one formatted feed entry — the pure content the view
// layer styles, wraps, and truncates to its panel width.
type chronicleLine struct {
	Seq     int64
	Time    string
	Type    string
	Class   eventClass
	Subject string // speech class only: {"Speaker"→"Listener"}
	Speech  string // speech class only: quoted utterance
	Payload string // everything else: compact JSON, names resolved
}

// formatChronicleLine implements the "Line format" section of
// chronicle-grammar.md: #<seq> <HH:MM> <type> <subject> <payload>.
func formatChronicleLine(e store.Event, names []string) chronicleLine {
	l := chronicleLine{Seq: e.Seq, Time: clock.Format(e.Tick), Type: e.Type, Class: classifyEvent(e.Type)}
	switch l.Class {
	case classSpeech:
		if from, to, text, ok := speechFields(e); ok {
			l.Subject = fmt.Sprintf("{%q→%q}", agentName(names, from), agentName(names, to))
			l.Speech = fmt.Sprintf("%q", text)
			return l
		}
	case classScene:
		l.Payload = sceneSummary(e)
		return l
	}
	l.Payload = resolvePayloadNames(e.Payload, names)
	return l
}

// plainChronicleLine assembles a formatChronicleLine result into one plain
// (unstyled) text line — the shape wrap/truncate operate on, and the piece
// AC9's "chronicle line formatting per event class" tests exercise
// directly, without any lipgloss/ANSI concerns.
func plainChronicleLine(l chronicleLine) string {
	prefix := fmt.Sprintf("#%d %s  %s", l.Seq, l.Time, l.Type)
	if l.Class == classSpeech && l.Subject != "" {
		return prefix + "  " + l.Subject + " " + l.Speech
	}
	return prefix + "  " + l.Payload
}

// wrapOrTruncatePlain implements chronicle-grammar.md's "Width overflow"
// rule: solo/narrow keep one line per event and truncate with "…"; dock
// wraps to at most maxWrap lines before truncating the last one. Pure and
// ANSI-free so wrap/truncate boundaries are directly testable.
func wrapOrTruncatePlain(s string, width, maxWrap int) []string {
	if width < 10 {
		width = 10
	}
	if maxWrap <= 1 {
		r := []rune(s)
		if len(r) <= width {
			return []string{s}
		}
		if width > 1 {
			r = r[:width-1]
		}
		return []string{string(r) + "…"}
	}
	lines := wrapText(s, width)
	if len(lines) <= maxWrap {
		return lines
	}
	lines = lines[:maxWrap]
	last := []rune(lines[maxWrap-1])
	if len(last) > width-1 {
		last = last[:width-1]
	}
	lines[maxWrap-1] = string(last) + "…"
	return lines
}

// --- inspector (paused expand): verbatim stored event, annotated ---

// kv is one ordered top-level key/raw-value pair of a flat JSON object.
type kv struct {
	Key string
	Val json.RawMessage
}

// parseObjectOrdered walks a JSON object's top level preserving field
// order (map[string]any does not) — needed so the inspector reproduces the
// event exactly as persisted, not alphabetized.
func parseObjectOrdered(raw json.RawMessage) ([]kv, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("not an object")
	}
	var pairs []kv
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		pairs = append(pairs, kv{Key: key, Val: val})
	}
	return pairs, nil
}

// formatAnnotatedPayload pretty-prints a flat JSON object at the given
// indent, appending "// name" beside any agent-index field's integer value.
// The payload bytes themselves are never rewritten — only a trailing
// comment is added, per chronicle-grammar.md's inspector rules.
func formatAnnotatedPayload(raw json.RawMessage, names []string, indent string) string {
	pairs, err := parseObjectOrdered(raw)
	if err != nil || len(pairs) == 0 {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, indent, "  "); err != nil {
			return string(raw)
		}
		return buf.String()
	}
	inner := indent + "  "
	rendered := make([]string, len(pairs))
	for i, p := range pairs {
		rendered[i] = fmt.Sprintf("%s%q: %s", inner, p.Key, string(p.Val))
	}
	for i := range rendered {
		if i < len(rendered)-1 {
			rendered[i] += ","
		}
	}
	for i, p := range pairs {
		if !agentIndexFields[p.Key] {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(string(p.Val))); err == nil && n >= 0 && n < len(names) {
			rendered[i] += fmt.Sprintf("   // %s", names[n])
		}
	}
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString(strings.Join(rendered, "\n"))
	b.WriteString("\n" + indent + "}")
	return b.String()
}

// formatInspector is the expanded, paused view of one stored event:
// seq/tick/type verbatim, payload annotated but never rewritten.
func formatInspector(e store.Event, names []string) string {
	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"seq\": %d, \"tick\": %d,\n", e.Seq, e.Tick)
	fmt.Fprintf(&b, "  \"type\": %q,\n", e.Type)
	b.WriteString("  \"payload\": ")
	b.WriteString(formatAnnotatedPayload(e.Payload, names, "  "))
	b.WriteString("\n}")
	return b.String()
}
