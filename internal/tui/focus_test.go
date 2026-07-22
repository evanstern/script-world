package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/store"
)

// --- focus-contract.md "Acceptance checks", run in both layouts ---

// TestFocusContractCheck1_NoFocusOnNavigation: "From any screen, type 3
// then 1 — you are looking at the map. No focus was acquired in between."
func TestFocusContractCheck1_NoFocusOnNavigation(t *testing.T) {
	for _, layout := range []string{"widescreen", "narrow"} {
		t.Run(layout, func(t *testing.T) {
			m := testModel(t)
			if layout == "widescreen" {
				m = widescreenModel(t)
			}
			var mdl tea.Model = m
			mdl = update(mdl, "3")
			if mdl.(Model).mbFocused {
				t.Fatal("selecting the metatron tab/pane must not focus the minibuffer")
			}
			mdl = update(mdl, "1")
			mm := mdl.(Model)
			if mm.mbFocused {
				t.Fatal("no focus should ever have been acquired")
			}
			if mm.active != paneMap {
				t.Errorf("active = %s, want map", paneNames[mm.active])
			}
			if layout == "widescreen" && mm.solo {
				t.Error("'1' must return home, not stay solo")
			}
		})
	}
}

// TestFocusContractCheck2_EscThenSpacePauses: "Focus the minibuffer, type
// 'hello', press esc, press space — the clock pauses."
func TestFocusContractCheck2_EscThenSpacePauses(t *testing.T) {
	for _, layout := range []string{"widescreen", "narrow"} {
		t.Run(layout, func(t *testing.T) {
			m := testModel(t)
			if layout == "widescreen" {
				m = widescreenModel(t)
			}
			m.connected = true
			m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: false}}
			var mdl tea.Model = m
			mdl = update(mdl, "m")
			if !mdl.(Model).mbFocused {
				t.Fatal("'m' must focus the minibuffer")
			}
			for _, r := range "hello" {
				mdl = update(mdl, string(r))
			}
			if got := mdl.(Model).mbInput; got != "hello" {
				t.Fatalf("typed input = %q, want %q", got, "hello")
			}
			mdl = update(mdl, "esc")
			if mdl.(Model).mbFocused {
				t.Fatal("esc must release focus")
			}
			mdl, cmd := mdl.(Model).Update(key(" "))
			if mdl.(Model).mbFocused {
				t.Fatal("still focused after esc+space")
			}
			if cmd == nil {
				t.Fatal("space while unfocused must dispatch pause/resume — the clock must pause")
			}
		})
	}
}

// TestFocusContractCheck3_EveryKeyVisiblyActsWhileFocused: "Focus the
// minibuffer and press every key on the keyboard — each press changed
// something visible." Exercises the full "Mode: minibuffer focused" table
// from patterns/keymap.md.
func TestFocusContractCheck3_EveryKeyVisiblyActsWhileFocused(t *testing.T) {
	m := testModel(t)
	var mdl tea.Model = m
	mdl = update(mdl, "m")

	// Printable keys and space append to the buffer, visibly.
	for _, k := range []string{"q", "u", "i", "c", "k", " ", "1"} {
		before := mdl.(Model).mbInput
		mdl = update(mdl, k)
		after := mdl.(Model).mbInput
		if after == before {
			t.Fatalf("key %q produced no visible change to the buffer", k)
		}
	}
	if got := mdl.(Model).mbInput; got != "quick 1" {
		t.Fatalf("input = %q, want %q", got, "quick 1")
	}
	if !mdl.(Model).mbFocused {
		t.Fatal("typed keys must not have been hijacked into some global action")
	}

	// Backspace deletes, visibly.
	before := mdl.(Model).mbInput
	mdl = update(mdl, "backspace")
	if mdl.(Model).mbInput == before {
		t.Fatal("backspace produced no visible change")
	}

	// Enter with no connection: releases focus and surfaces an error —
	// still a visible effect, never a silent no-op.
	mdl, cmd := mdl.(Model).Update(key("enter"))
	if mdl.(Model).mbFocused {
		t.Error("enter must always change focus state (release), never no-op silently")
	}
	if cmd != nil {
		t.Error("disconnected send should not dispatch a command")
	}
	if mdl.(Model).mbErr == "" {
		t.Error("disconnected enter should surface an error, not silently do nothing")
	}

	// Re-focus, verify esc's visible effect.
	mdl = update(mdl, "m")
	if !mdl.(Model).mbFocused {
		t.Fatal("m must re-focus")
	}
	mdl = update(mdl, "esc")
	if mdl.(Model).mbFocused {
		t.Fatal("esc must release focus, visibly")
	}
}

// TestGlobalKeysNeverLeakWhileFocused: global keys (pane select, quit,
// pause, speed) type into the buffer instead of firing while focused — the
// mirror image of the old bug (only ctrl+c is exempt, rule 3).
func TestGlobalKeysNeverLeakWhileFocused(t *testing.T) {
	for _, layout := range []string{"widescreen", "narrow"} {
		t.Run(layout, func(t *testing.T) {
			m := testModel(t)
			if layout == "widescreen" {
				m = widescreenModel(t)
			}
			var mdl tea.Model = m
			mdl = update(mdl, "m")
			for _, k := range []string{"1", "2", "3", "4", "q", "r", "a", "t"} {
				mdl = update(mdl, k)
			}
			mm := mdl.(Model)
			if !mm.mbFocused {
				t.Fatal("focus must survive typing characters that double as global keys")
			}
			if mm.quitting {
				t.Fatal("'q' while focused must type, not quit")
			}
			if got := mm.mbInput; got != "1234qrat" {
				t.Fatalf("input = %q, want the literal keys typed", got)
			}
		})
	}
}

// TestUnfocusedGlobalKeysAllWork: space/[/]/q/pane-select all fire while
// unfocused — focus-contract.md rule 5.
func TestUnfocusedGlobalKeysAllWork(t *testing.T) {
	m := testModel(t)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: false, Speed: "4x"}}
	var mdl tea.Model = m

	mdl, cmd := mdl.(Model).Update(key(" "))
	if cmd == nil {
		t.Error("space unfocused must pause")
	}
	mdl, cmd = mdl.(Model).Update(key("]"))
	if cmd == nil {
		t.Error("] unfocused must change speed")
	}
	mdl = update(mdl, "2")
	if mdl.(Model).active != paneChronicle {
		t.Error("2 unfocused must select the chronicle pane")
	}
	mdl, cmd = mdl.(Model).Update(key("q"))
	if cmd == nil || !mdl.(Model).quitting {
		t.Error("q unfocused must quit")
	}
}

// --- solo zoom state machine (pages/solo-views.md), AC3 ---

func TestSoloZoomStateMachine(t *testing.T) {
	m := widescreenModel(t)
	var mdl tea.Model = m
	if mdl.(Model).solo {
		t.Fatal("must not start solo")
	}
	if mdl.(Model).dockTab != paneChronicle {
		t.Fatal("chronicle must be the default dock tab (dock.md)")
	}

	// home, tab=chronicle --2--> solo(chronicle)
	mdl = update(mdl, "2")
	mm := mdl.(Model)
	if !mm.solo || mm.dockTab != paneChronicle {
		t.Fatalf("second '2' press should zoom solo: solo=%v tab=%s", mm.solo, paneNames[mm.dockTab])
	}

	// solo(chronicle) --2--> home, tab=chronicle
	mdl = update(mdl, "2")
	mm = mdl.(Model)
	if mm.solo {
		t.Fatal("same key again must return home")
	}

	// home, tab=chronicle --3--> home, tab=metatron (select, not solo)
	mdl = update(mdl, "3")
	mm = mdl.(Model)
	if mm.solo || mm.dockTab != paneMetatron {
		t.Fatalf("different key selects, does not solo: solo=%v tab=%s", mm.solo, paneNames[mm.dockTab])
	}

	// home, tab=metatron --3--> solo(metatron) --1--> home, tab=metatron
	mdl = update(mdl, "3")
	if !mdl.(Model).solo {
		t.Fatal("second '3' press should zoom solo")
	}
	mdl = update(mdl, "1")
	mm = mdl.(Model)
	if mm.solo {
		t.Fatal("'1' must exit solo (solo-views.md state machine)")
	}
	if mm.dockTab != paneMetatron {
		t.Fatal("'1' returns home with the tab selection intact")
	}

	// esc also exits solo.
	mdl = update(mdl, "3") // solo(metatron) again
	if !mdl.(Model).solo {
		t.Fatal("setup: expected solo")
	}
	mdl = update(mdl, "esc")
	if mdl.(Model).solo {
		t.Fatal("esc must exit solo, same as '1'")
	}
}

// TestSoloPreservesTabState: chronicle filters set before zooming solo
// survive the round trip home -> solo -> home (solo-views.md "Solo rules").
func TestSoloPreservesTabState(t *testing.T) {
	m := widescreenModel(t)
	m.chronAgent = 2
	m.chronThread = "cold-start"
	var mdl tea.Model = m
	// Chronicle is already the default selected tab, so one '2' press zooms.
	mdl = update(mdl, "2")
	if !mdl.(Model).solo {
		t.Fatal("expected solo")
	}
	mdl = update(mdl, "1") // back home
	mm := mdl.(Model)
	if mm.chronAgent != 2 || mm.chronThread != "cold-start" {
		t.Errorf("chronicle filter state lost across solo round trip: agent=%d thread=%q", mm.chronAgent, mm.chronThread)
	}
}

// --- resize crosses the breakpoint without losing state (AC2) ---

func TestResizeAcrossBreakpointPreservesState(t *testing.T) {
	m := testModel(t) // narrow
	m.panX, m.panY = 12, -8
	m.chronAgent = 3
	m.dockTab = paneVillagers
	var mdl tea.Model = m

	mdl, _ = mdl.(Model).Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	mm := mdl.(Model)
	if !isWidescreen(mm.width) {
		t.Fatal("width should now be widescreen")
	}
	if mm.panX != 12 || mm.panY != -8 || mm.chronAgent != 3 || mm.dockTab != paneVillagers {
		t.Errorf("state lost crossing to widescreen: %+v", mm)
	}
	if v := mm.View(); v == "" {
		t.Error("widescreen view rendered empty")
	}

	mdl, _ = mdl.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	mm = mdl.(Model)
	if isWidescreen(mm.width) {
		t.Fatal("width should now be narrow")
	}
	if mm.panX != 12 || mm.panY != -8 || mm.chronAgent != 3 {
		t.Errorf("state lost crossing back to narrow: %+v", mm)
	}
	if v := mm.View(); v == "" {
		t.Error("narrow view rendered empty")
	}
}

// --- inspect mode (panels/chronicle.md Mode 2), AC7 ---

func seedEvents(m *Model, n int) {
	for i := int64(1); i <= int64(n); i++ {
		m.applyEvent(store.Event{Seq: i, Tick: i * 60, Type: "agent.moved",
			Payload: json.RawMessage(`{"agent":0,"x":1,"y":1}`)})
	}
}

func pausedModel(t *testing.T) Model {
	t.Helper()
	m := widescreenModel(t)
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
	m.connected = true
	seedEvents(&m, 5)
	return m
}

func TestInspectModeEnteredWhenPausedAndChronicleVisible(t *testing.T) {
	m := pausedModel(t)
	if !m.inspecting() {
		t.Fatal("paused + chronicle tab selected must enter inspect mode")
	}
	m.dockTab = paneVillagers
	if m.inspecting() {
		t.Fatal("inspect mode should not apply when chronicle is not visible")
	}
}

func TestInspectSelectionMoveAndJump(t *testing.T) {
	m := pausedModel(t)
	var mdl tea.Model = m
	mdl = update(mdl, "j") // from unselected, base is the tail (last event)
	mm := mdl.(Model)
	if mm.chronSelected != 4 { // 5 events, base=4 (tail), +1 clamps to 4
		t.Fatalf("chronSelected = %d, want 4 (clamped at tail)", mm.chronSelected)
	}
	mdl = update(mm, "k")
	mdl = update(mdl, "k")
	if got := mdl.(Model).chronSelected; got != 2 {
		t.Fatalf("after two k's, chronSelected = %d, want 2", got)
	}
	mdl = update(mdl, "g")
	if got := mdl.(Model).chronSelected; got != 0 {
		t.Fatalf("g should jump to first: got %d", got)
	}
	mdl = update(mdl, "G")
	if got := mdl.(Model).chronSelected; got != 4 {
		t.Fatalf("G should jump to last: got %d", got)
	}
}

// TestInspectDetailPaneAlwaysVisible: the detail pane shows the selected
// event verbatim with zero extra keypresses (FR-008, contract §5) — no ⏎
// required, unlike the inline inspector this replaced.
func TestInspectDetailPaneAlwaysVisible(t *testing.T) {
	m := pausedModel(t)
	m.chronSelected = 2
	body := m.chronicleInspectBody(60, 20)
	if !strings.Contains(body, `"seq": 3`) {
		t.Errorf("detail pane should show the selected event verbatim: %q", body)
	}
	if !strings.Contains(body, "DETAIL · seq 3") {
		t.Errorf("detail pane should carry the rule line contract §5 specifies: %q", body)
	}
}

// TestInspectEnterIsNoOp: ⏎ is reserved for the future jump-off actions bar
// (contract §5 "Extension point", R7) — it must not move the selection or
// change what's rendered.
func TestInspectEnterIsNoOp(t *testing.T) {
	m := pausedModel(t)
	m.chronSelected = 2
	before := m.chronicleInspectBody(60, 20)
	var mdl tea.Model = m
	mdl = update(mdl, "enter")
	mm := mdl.(Model)
	if mm.chronSelected != 2 {
		t.Fatalf("enter must not move the selection: chronSelected = %d", mm.chronSelected)
	}
	if after := mm.chronicleInspectBody(60, 20); before != after {
		t.Errorf("enter is a reserved no-op and must not change the rendered body:\nbefore: %q\nafter:  %q", before, after)
	}
}

// TestInspectResetsOnResume: resume clears the selection and the detail
// pane's scroll offset (data-model.md "Interaction state").
func TestInspectResetsOnResume(t *testing.T) {
	m := pausedModel(t)
	m.chronSelected = 2
	m.chronDetailScroll = 3
	mdl, _ := m.Update(statusMsg{status: &ipc.StatusData{Clock: ipc.ClockStatus{Paused: false}}})
	mm := mdl.(Model)
	if mm.chronSelected != -1 || mm.chronDetailScroll != 0 {
		t.Errorf("resume must clear selection and detail scroll: selected=%d scroll=%d", mm.chronSelected, mm.chronDetailScroll)
	}
}

// TestInspectStateSurvivesTabSwitch: "Selection is remembered while paused
// even if the user switches tabs and returns" (panels/chronicle.md).
func TestInspectStateSurvivesTabSwitch(t *testing.T) {
	m := pausedModel(t)
	m.chronSelected = 1
	var mdl tea.Model = m
	mdl = update(mdl, "4") // switch to villagers
	mdl = update(mdl, "2") // back to chronicle
	if got := mdl.(Model).chronSelected; got != 1 {
		t.Errorf("selection not remembered across tab switch: got %d", got)
	}
}

// TestInspectSelectionMoveResetsDetailScroll: moving the selection follows
// with the detail pane (it now shows a different event) and resets any
// scroll offset back to the top (data-model.md) — carried over from the B2
// regression this replaces: j/k must never be a no-op, and the composite
// must stay within its row budget regardless of pane content.
func TestInspectSelectionMoveResetsDetailScroll(t *testing.T) {
	m := pausedModel(t)
	m.chronSelected = 2
	m.chronDetailScroll = 5
	var mdl tea.Model = m
	mdl = update(mdl, "k")
	mm := mdl.(Model)
	if mm.chronSelected != 1 {
		t.Fatalf("k must move the selection: chronSelected = %d, want 1", mm.chronSelected)
	}
	if mm.chronDetailScroll != 0 {
		t.Errorf("selection move must reset detail scroll: got %d, want 0", mm.chronDetailScroll)
	}

	mdl = update(mm, "j")
	mdl = update(mdl, "j")
	if got := mdl.(Model).chronSelected; got != 3 {
		t.Fatalf("j must move the selection: chronSelected = %d, want 3", got)
	}

	// B2's regression: the composite must stay within its row budget no
	// matter the pane content (5 events exactly fill an 8-row split's list).
	body := mm.chronicleInspectBody(60, 8)
	if lines := strings.Split(body, "\n"); len(lines) != 8 {
		t.Fatalf("inspect body must render to exactly its row budget (8): got %d lines:\n%s", len(lines), body)
	}
}

// TestInspectDetailScrollKeysAndClamp: J/K scroll the detail pane; K at 0
// stays at 0; a scroll far past the content is clamped at render time
// rather than growing the pane or panicking (contract §5, R6).
func TestInspectDetailScrollKeysAndClamp(t *testing.T) {
	m := pausedModel(t)
	m.chronSelected = 2
	var mdl tea.Model = m
	mdl = update(mdl, "K")
	if got := mdl.(Model).chronDetailScroll; got != 0 {
		t.Errorf("K at scroll 0 must clamp at 0: got %d", got)
	}
	mdl = update(mdl, "J")
	mdl = update(mdl, "J")
	if got := mdl.(Model).chronDetailScroll; got != 2 {
		t.Errorf("J should increment scroll: got %d, want 2", got)
	}

	mm := mdl.(Model)
	mm.chronDetailScroll = 99999 // far past any real content
	body := mm.chronicleInspectBody(60, 10)
	if got := len(strings.Split(body, "\n")); got > 10 {
		t.Fatalf("render-time scroll clamp must not overflow the row budget: got %d lines", got)
	}
}

// TestInspectDetailPaneBoundsOversizedPayload: a world.migrated-sized
// payload (spec 018 FR-011) must never grow the composite past its row
// budget — the pane windows the annotated payload rather than emitting it
// in full, and shows the scroll footer once content overflows.
func TestInspectDetailPaneBoundsOversizedPayload(t *testing.T) {
	m := pausedModel(t)
	seedEvents(&m, 30) // enough events that the list fills its share too
	// formatAnnotatedPayload renders one line per *top-level* field (nested
	// values pass through compact) — so a world.migrated-sized payload needs
	// many top-level siblings, not one deeply-populated nested map, to
	// actually overflow the pane the way the real embedded sim.State does.
	fields := make(map[string]json.RawMessage, 40)
	for i := 0; i < 40; i++ {
		fields[fmt.Sprintf("field_%02d", i)] = json.RawMessage(fmt.Sprintf(`"value_%d"`, i))
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal fixture payload: %v", err)
	}
	m.applyEvent(store.Event{Seq: 1000, Tick: 3000, Type: "world.migrated", Payload: payload})
	m.chronSelected = len(m.events) - 1

	body := m.chronicleInspectBody(60, 20)
	if lines := strings.Split(body, "\n"); len(lines) > 20 {
		t.Fatalf("an oversized payload must not grow the body past its row budget: got %d lines, want <= 20:\n%s", len(lines), body)
	}
	if !strings.Contains(body, "more — J to scroll") {
		t.Errorf("an oversized payload should show the scroll footer: %q", body)
	}
	if !strings.Contains(body, "[future: actions]") {
		t.Errorf("the footer should carry the reserved actions slot (FR-009): %q", body)
	}
}

// TestResizeRoundTripWhilePausedWithSelection is B5: shrinking widescreen
// -> narrow -> back to widescreen while paused with an active selection
// must not panic, must keep the selection valid, must clamp a pan offset
// that would otherwise be stale, and must still render to exactly the new
// height on the way back.
func TestResizeRoundTripWhilePausedWithSelection(t *testing.T) {
	m := widescreenModel(t) // 140x40
	seedEvents(&m, 50)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
	m.chronSelected = 40
	m.panX, m.panY = 500, 500 // pathologically large, pre-clamp
	var mdl tea.Model = m

	mdl, _ = mdl.(Model).Update(tea.WindowSizeMsg{Width: 100, Height: 30}) // -> narrow
	mm := mdl.(Model)
	if isWidescreen(mm.width) {
		t.Fatal("100 cols should be narrow")
	}
	if v := mm.View(); v == "" {
		t.Fatal("narrow view rendered empty after shrink")
	}
	if mm.chronSelected < 0 || mm.chronSelected >= len(mm.events) {
		t.Errorf("selection out of range after shrink: %d (events=%d)", mm.chronSelected, len(mm.events))
	}

	mdl, _ = mdl.(Model).Update(tea.WindowSizeMsg{Width: 140, Height: 40}) // -> back to widescreen
	mm = mdl.(Model)
	if !isWidescreen(mm.width) {
		t.Fatal("140 cols should be widescreen")
	}
	v := mm.View()
	lines := strings.Split(v, "\n")
	if len(lines) != mm.height {
		t.Errorf("View() after resize round trip = %d lines, want %d", len(lines), mm.height)
	}
	if mm.chronSelected != 40 {
		t.Errorf("selection should have survived the round trip unchanged (event count never changed): got %d, want 40", mm.chronSelected)
	}
	if mm.gameMap != nil && (mm.panX < -mm.gameMap.W || mm.panX > mm.gameMap.W || mm.panY < -mm.gameMap.H || mm.panY > mm.gameMap.H) {
		t.Errorf("pan offset not clamped to the map after resize: panX=%d panY=%d (map %dx%d)",
			mm.panX, mm.panY, mm.gameMap.W, mm.gameMap.H)
	}
}
