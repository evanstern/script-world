package tui

// Chronicle grammar (TASK-34, extended TASK-60): how one event becomes one
// feed entry. See docs/design/tui/patterns/chronicle-grammar.md and
// specs/018-chronicle-digest/contracts/digest-grammar.md. Kept as pure
// functions over store.Event + the replica's agent-name table so they're
// directly table-driven-testable without a Bubble Tea program — no lipgloss,
// no ANSI (R4): the view layer (views.go) styles the `seg` output.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
)

// segRole tags one styled span of a digest summary (R4); the view layer maps
// roles to style tokens, the pure layer never touches lipgloss.
type segRole int

const (
	segText segRole = iota
	segName
	segSpeech
	segEmphasis
	segLabel
)

// seg is one styled span; concatenating every Text in order is the plain
// summary wrap/truncate operates on (data-model.md "seg").
type seg struct {
	Text string
	Role segRole
}

// plainSegs concatenates a digest's segs into the plain (unstyled) summary.
func plainSegs(segs []seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

// eventFamily is the chronicle's namespace-derived grouping (R2): the voice
// (labeled vs. natural phrase) and, later, the family color role both key off
// this rather than the event type string directly.
type eventFamily int

const (
	familyUnknown eventFamily = iota
	familyWorld
	familyClock
	familySim
	familyAgent
	familySocial
	familyGovernance
	familyGru
	familyChronicle
	familyMetatron
	familyDaemon
	familyCog
)

// familyByNamespace maps an event type's namespace (text before the first
// '.') to its family. meeting/norm share one visual role, governance (R2) —
// a meeting is village fabric and a norm is what a meeting produces, and the
// chronicle treats both as one family rather than two.
var familyByNamespace = map[string]eventFamily{
	"world":     familyWorld,
	"clock":     familyClock,
	"sim":       familySim,
	"agent":     familyAgent,
	"social":    familySocial,
	"meeting":   familyGovernance,
	"norm":      familyGovernance,
	"gru":       familyGru,
	"chronicle": familyChronicle,
	"metatron":  familyMetatron,
	"daemon":    familyDaemon,
	"cog":       familyCog,
}

// eventFamilyOf derives a type's family from its namespace prefix (R2). New
// namespaces land in familyUnknown until familyByNamespace grows a row.
func eventFamilyOf(eventType string) eventFamily {
	ns, _, ok := strings.Cut(eventType, ".")
	if !ok {
		return familyUnknown
	}
	if f, ok := familyByNamespace[ns]; ok {
		return f
	}
	return familyUnknown
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

// chronicleLine v2 (data-model.md) is one formatted feed entry — the pure
// content the view layer styles, wraps, and truncates to its panel width.
// Seq leaves the feed line proper (it lives in the detail pane, R7); Tick is
// the new solo-only column (R5); Summary is the digest registry's (or the
// fallback's) styled segments.
type chronicleLine struct {
	Seq     int64
	Tick    int64
	Time    string
	Type    string
	Family  eventFamily
	Summary []seg
}

// formatChronicleLine implements the digest-grammar contract's "Line
// format": a registry hit digests the payload into styled segments (R1);
// a miss or an unmarshal failure (digestFunc's ok=false) falls back to the
// pre-digest compact resolved-name JSON as one segText span — total,
// FR-002/FR-003, never blank, never a panic.
func formatChronicleLine(e store.Event, names []string) chronicleLine {
	l := chronicleLine{
		Seq:    e.Seq,
		Tick:   e.Tick,
		Time:   clock.FormatTOD(int(clock.SecondOfDay(e.Tick))),
		Type:   e.Type,
		Family: eventFamilyOf(e.Type),
	}
	if fn, ok := digestRegistry[e.Type]; ok {
		if segs, ok := fn(e, names); ok {
			l.Summary = segs
			return l
		}
	}
	l.Summary = []seg{{Text: resolvePayloadNames(e.Payload, names), Role: segText}}
	return l
}

// shortType is the dock column's short-name form (R5): the type's last
// namespace segment, e.g. "social.conversation_turn" → "conversation_turn".
func shortType(t string) string {
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		return t[i+1:]
	}
	return t
}

// Type column caps (R5, contract §1): solo shows the full type name up to
// 26 runes; dock shows the short name up to 10.
const (
	typeColumnCapSolo = 26
	typeColumnCapDock = 10
)

// chronicleColumns is the per-visible-window column layout (R5) — computed
// fresh over the lines about to render, never stored, so every row in one
// frame lines up without a global fixed budget.
type chronicleColumns struct {
	Dock      bool // dock width: tick column dropped, short type name
	TickWidth int  // widest visible tick, right-aligned
	TypeWidth int  // widest visible type (solo) / short name (dock), capped
}

// computeChronicleColumns derives one window's column widths (R5). Callers
// pass exactly the lines about to render (R8: window first, then format) so
// this never walks more than one frame's worth of entries.
func computeChronicleColumns(lines []chronicleLine, dock bool) chronicleColumns {
	cap := typeColumnCapSolo
	if dock {
		cap = typeColumnCapDock
	}
	cols := chronicleColumns{Dock: dock}
	for _, l := range lines {
		if w := len(strconv.FormatInt(l.Tick, 10)); w > cols.TickWidth {
			cols.TickWidth = w
		}
		t := l.Type
		if dock {
			t = shortType(t)
		}
		w := len([]rune(t))
		if w > cap {
			w = cap
		}
		if w > cols.TypeWidth {
			cols.TypeWidth = w
		}
	}
	return cols
}

// padType left-justifies a type name to the window's computed column width,
// truncating with "…" past the family cap (R5).
func padType(t string, cols chronicleColumns) string {
	cap := typeColumnCapSolo
	if cols.Dock {
		t = shortType(t)
		cap = typeColumnCapDock
	}
	r := []rune(t)
	if len(r) > cap {
		if cap > 1 {
			r = append(append([]rune{}, r[:cap-1]...), '…')
		} else {
			r = r[:cap]
		}
	}
	if len(r) < cols.TypeWidth {
		r = append(r, []rune(strings.Repeat(" ", cols.TypeWidth-len(r)))...)
	}
	return string(r)
}

// plainChronicleLine assembles a formatChronicleLine result plus its
// window's column layout into one plain (unstyled) text line (contract §1):
// solo shows `<TICK> <HH:MM>  <type>  <summary>`, right-aligned tick; dock
// drops the tick column entirely. The result is the shape wrap/truncate
// operate on, without any lipgloss/ANSI concerns.
func plainChronicleLine(l chronicleLine, cols chronicleColumns) string {
	typ := padType(l.Type, cols)
	summary := plainSegs(l.Summary)
	if cols.Dock {
		return fmt.Sprintf("%s %s  %s", l.Time, typ, summary)
	}
	tick := fmt.Sprintf("%*d", cols.TickWidth, l.Tick)
	return fmt.Sprintf("%s %s  %s  %s", tick, l.Time, typ, summary)
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
