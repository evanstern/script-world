// Package tui is the attachable Bubble Tea client: a live view over a world
// replica maintained by log shipping — initial state via the protocol
// "state" command, then subscribed events applied through the same
// sim.State reducer the daemon runs.
//
// TASK-34 widescreen redesign: at width >= widescreenBreakpoint the app
// renders the composite home page (map ‖ dock, docs/design/tui/pages/home.md)
// instead of the single-pane-at-a-time UI; below it, today's single-pane UI
// renders unchanged (docs/design/tui/pages/solo-views.md "Narrow fallback").
// The focus contract (docs/design/tui/patterns/focus-contract.md) replaces
// the old "the metatron console owns the keyboard while active" rule, which
// silently swallowed 1-4/q/space once pane 3 was entered.
package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/metatron"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/world"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// pane names both the narrow-fallback's single active pane and the
// widescreen dock's selected tab — paneMap is narrow-only (the widescreen
// map is always visible, never a dock tab); the dock only ever selects
// paneChronicle/paneMetatron/paneVillagers.
type pane int

const (
	paneMap pane = iota
	paneChronicle
	paneMetatron
	paneVillagers
	paneCount
)

var paneNames = [paneCount]string{"map", "chronicle", "metatron", "villagers"}

// dockTabKey is the keymap.md key that selects/solos each dock tab.
var dockTabKey = map[pane]string{paneChronicle: "2", paneMetatron: "3", paneVillagers: "4"}

// speedSteps is the [ / ] cycling order.
// max is deliberately absent: the watchable ladder tops out at 32x (TASK-20);
// uncapped ticking is for headless pure-sim runs only.
var speedSteps = []clock.Speed{clock.Speed1x, clock.Speed4x, clock.Speed8x, clock.Speed16x, clock.Speed32x}

const chronicleCap = 500

// Model is the Bubble Tea model. All protocol calls run inside tea.Cmds so
// the UI never blocks on the socket.
type Model struct {
	w *world.World

	client    *ipc.Client
	connected bool
	lastErr   string
	fatalErr  string // unrecoverable (e.g. reply over protocol cap): quit, don't retry

	replica *sim.State      // world replica, event-sourced client-side
	gameMap *worldmap.Map   // terrain, regenerated locally from the manifest
	status  *ipc.StatusData // latest clock/daemon status (1s poll)
	events  []store.Event   // chronicle ring, newest last
	lastSeq int64

	// active is the narrow fallback's single visible pane (today's model,
	// unchanged). dockTab/solo are the widescreen composite's dock
	// selection and zoom state (pages/solo-views.md). Both are kept in
	// sync on tab-select so a resize across the breakpoint always shows
	// whatever was last looked at, without either being reset by resize.
	active        pane
	dockTab       pane // paneChronicle by default (dock.md: "Default tab on launch")
	solo          bool // dockTab is zoomed to full width (pages/solo-views.md)
	width, height int
	panX, panY    int // map-pane camera offset from the wanderer centroid
	quitting      bool

	// Chronicle pane filters (TASK-11): narrated entries filtered by agent
	// and thread; chronRaw falls back to the raw event feed.
	chronAgent  int // -1 = all
	chronThread string
	chronRaw    bool

	// Chronicle inspect mode (TASK-34, panels/chronicle.md; detail pane
	// TASK-60 spec 018 US2): entered automatically whenever the clock is
	// paused and the chronicle is visible. Selection indexes the raw feed
	// (events); remembered across tab switches, cleared on resume.
	// chronDetailScroll is the always-on detail pane's own scroll offset
	// (contract §5/R6/R7) — reset to 0 on selection move, pause exit, and
	// reconnect (data-model.md "Interaction state"); the render-time clamp
	// to the pane's actual content length lives in chronicleInspectBody,
	// the same defensive-tolerance pattern chronSelectionBase uses.
	chronSelected     int // -1 = none
	chronDetailScroll int

	// Metatron (TASK-12, re-surfaced as the minibuffer by TASK-34): the
	// transcript is dock/pane content; mbInput/mbFocused/mbBusy are the
	// minibuffer's own state, governed by the focus contract
	// (patterns/focus-contract.md) everywhere it appears.
	transcript     []string // rendered transcript rows, newest last
	consoleCharter string   // "default charter" | "custom charter" | ""

	mbFocused bool
	mbInput   string
	mbBusy    bool
	mbErr     string
	mbHistory []string
	mbHistPos int    // index into mbHistory while cycling; == len(mbHistory) means "the live draft"
	mbDraft   string // input stashed when history-cycling away from an in-progress draft
	mbFlash   string // one-shot dormant-state message (minibuffer.md "answer arrived — 3 to read")

	metatronUnseen bool // dock tab badge: a reply landed while the tab wasn't visible

	// Villagers tab (TASK-56, data-model.md "New TUI model state"):
	// villSelected is the roster cursor, clamped to [0, len(replica.Agents))
	// wherever read (the replica can arrive late or be swapped wholesale on
	// reconnect); villDetail opens the selected villager's detail view.
	// Neither field is persisted — client-only, event-sourced from nothing.
	villSelected int
	villDetail   bool
}

func New(w *world.World) Model {
	return Model{w: w, gameMap: w.Map(), chronAgent: -1, dockTab: paneChronicle, chronSelected: -1}
}

// FatalErr reports the unrecoverable error that made the TUI quit, if any —
// the ui command surfaces it as a real exit error after Run returns.
func (m Model) FatalErr() string { return m.fatalErr }

func (m Model) Init() tea.Cmd {
	return tea.Batch(connect(m.w), pollTick())
}

// --- messages ---

type connectedMsg struct {
	client  *ipc.Client
	status  *ipc.StatusData
	replica *sim.State
	lastSeq int64
}

type disconnectedMsg struct{ err error }

type pushMsg struct{ push ipc.Push }

type statusMsg struct{ status *ipc.StatusData }

type consoleReplyMsg struct {
	result *metatron.TurnResult
	err    error
}

type consoleStatusMsg struct{ status *metatron.Status }

// fetchConsoleStatus grabs the model-free peek when the metatron tab/pane is
// selected.
func fetchConsoleStatus(c *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		st, err := c.MetatronStatus()
		if err != nil {
			return consoleStatusMsg{}
		}
		return consoleStatusMsg{status: st}
	}
}

type pollMsg struct{}

type retryMsg struct{}

// --- commands ---

// connect dials, fetches the state snapshot, and subscribes from exactly the
// seq that snapshot reflects — the replica starts gapless.
func connect(w *world.World) tea.Cmd {
	return func() tea.Msg {
		c, err := ipc.Dial(w.SockPath())
		if err != nil {
			return disconnectedMsg{err}
		}
		sd, err := c.FetchState()
		if err != nil {
			c.Close()
			return disconnectedMsg{err}
		}
		replica := sim.NewState(w.Manifest.Seed, w.Map())
		if err := json.Unmarshal(sd.State, replica); err != nil {
			c.Close()
			return disconnectedMsg{fmt.Errorf("state decode: %w", err)}
		}
		st, err := c.Status("status", nil)
		if err != nil {
			c.Close()
			return disconnectedMsg{err}
		}
		since := sd.LastSeq
		if err := c.Subscribe(&since); err != nil {
			c.Close()
			return disconnectedMsg{err}
		}
		return connectedMsg{client: c, status: st, replica: replica, lastSeq: sd.LastSeq}
	}
}

// listen delivers one push per invocation; Update re-arms it.
func listen(c *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-c.Pushes()
		if !ok {
			return disconnectedMsg{fmt.Errorf("connection lost")}
		}
		return pushMsg{p}
	}
}

func pollTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return pollMsg{} })
}

func retryLater() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return retryMsg{} })
}

func fetchStatus(c *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		st, err := c.Status("status", nil)
		if err != nil {
			return disconnectedMsg{err}
		}
		return statusMsg{st}
	}
}

// sendConsole runs one Metatron turn off the UI goroutine.
func sendConsole(c *ipc.Client, text string) tea.Cmd {
	return func() tea.Msg {
		r, err := c.MetatronChat(text)
		return consoleReplyMsg{result: r, err: err}
	}
}

func timeControl(c *ipc.Client, cmd string, args any) tea.Cmd {
	return func() tea.Msg {
		st, err := c.Status(cmd, args)
		if err != nil {
			return disconnectedMsg{err}
		}
		return statusMsg{st}
	}
}

// --- update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		// Resizing across the widescreen breakpoint swaps layouts live;
		// no field here is reset, so no state is lost (pages/solo-views.md
		// "Narrow fallback"). clampGeometry re-bounds the handful of
		// fields that persist *values* across frames (pan offset,
		// chronicle selection) so nothing computed at the old size can
		// push a panel off-frame at the new one (B5) — everything else
		// (dock tab, solo, filters, transcript) is size-independent by
		// construction and needs no clamping.
		m.width, m.height = msg.Width, msg.Height
		m.clampGeometry()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case connectedMsg:
		if m.client != nil {
			m.client.Close()
		}
		m.client = msg.client
		m.connected = true
		m.lastErr = ""
		m.status = msg.status
		m.replica = msg.replica
		m.lastSeq = msg.lastSeq
		m.clampVillSelected()   // R5: connectedMsg swaps the replica wholesale
		m.chronDetailScroll = 0 // data-model.md: detail pane scroll resets on reconnect
		return m, listen(m.client)

	case disconnectedMsg:
		m.connected = false
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}
		if errors.Is(msg.err, ipc.ErrReplyTooLarge) {
			// Reconnecting cannot shrink the state — retrying forever would
			// be the TASK-19 bug. Fail fast with the actionable message.
			m.fatalErr = msg.err.Error()
			m.quitting = true
			return m, tea.Quit
		}
		m.lastErr = msg.err.Error()
		return m, retryLater()

	case retryMsg:
		if m.connected || m.quitting {
			return m, nil
		}
		return m, connect(m.w)

	case pushMsg:
		return m.handlePush(msg.push)

	case statusMsg:
		wasPaused := m.status != nil && m.status.Clock.Paused
		m.status = msg.status
		nowPaused := m.status != nil && m.status.Clock.Paused
		if wasPaused && !nowPaused {
			// Resume: collapse everything, snap back to tail-follow
			// (panels/chronicle.md Mode 2 "On resume"; contract §5/R7).
			m.chronSelected = -1
			m.chronDetailScroll = 0
		}
		return m, nil

	case consoleStatusMsg:
		if msg.status == nil {
			m.consoleCharter = ""
		} else if msg.status.CharterDefault {
			m.consoleCharter = "default charter"
		} else {
			m.consoleCharter = "custom charter"
		}
		return m, nil

	case consoleReplyMsg:
		m.mbBusy = false
		if msg.err != nil {
			m.mbErr = msg.err.Error()
			return m, nil
		}
		r := msg.result
		for _, mo := range r.Moments {
			m.transcript = append(m.transcript, "! "+mo)
		}
		m.transcript = append(m.transcript, "angel: "+r.Reply)
		if r.Nudge != nil {
			m.transcript = append(m.transcript, fmt.Sprintf("⚡ %s → %s: %q",
				r.Nudge.Form, strings.Join(r.Nudge.Targets, ", "), r.Nudge.Text))
		}
		if len(m.transcript) > 200 {
			m.transcript = m.transcript[len(m.transcript)-200:]
		}
		// Reply arrival (minibuffer.md): stream in place if the metatron
		// tab/pane is visible; otherwise badge the tab and flash the
		// minibuffer once.
		if !m.metatronVisible() {
			m.metatronUnseen = true
			m.mbFlash = "answer arrived — 3 to read"
		}
		return m, nil

	case pollMsg:
		cmds := []tea.Cmd{pollTick()}
		if m.connected && m.client != nil {
			cmds = append(cmds, fetchStatus(m.client))
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// --- focus contract (patterns/focus-contract.md) ---

// quit is ctrl+c/q from any unfocused state: rule 3, "ctrl+c quits the app
// from any state whatsoever".
func (m Model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	if m.client != nil {
		m.client.Close()
	}
	return m, tea.Quit
}

// chronicleVisible reports whether the chronicle is the thing currently on
// screen, in whichever layout is active — the gate for both the a/t/r
// filter keys and automatic inspect-mode entry.
func (m Model) chronicleVisible() bool {
	if isWidescreen(m.width) {
		return m.dockTab == paneChronicle
	}
	return m.active == paneChronicle
}

// metatronVisible reports whether the metatron transcript is the thing
// currently on screen — governs whether a reply streams in place or badges
// the tab (minibuffer.md).
func (m Model) metatronVisible() bool {
	if isWidescreen(m.width) {
		return m.dockTab == paneMetatron
	}
	return m.active == paneMetatron
}

// villagersVisible reports whether the villagers tab is the thing currently
// on screen — the gate for the roster/detail selection keys (contracts/
// state-and-keys.md "Keys bind only while the villagers tab is the visible
// dock tab or solo'd").
func (m Model) villagersVisible() bool {
	if isWidescreen(m.width) {
		return m.dockTab == paneVillagers
	}
	return m.active == paneVillagers
}

// mapControllable reports whether arrow keys should pan the map: always in
// widescreen (pages/home.md: "regardless of which dock tab is selected"),
// only while the map pane is active in the narrow fallback (unchanged).
func (m Model) mapControllable() bool {
	if isWidescreen(m.width) {
		return true
	}
	return m.active == paneMap
}

// inspecting reports whether inspect mode (panels/chronicle.md Mode 2) is
// live: paused, and the chronicle is the thing on screen.
func (m Model) inspecting() bool {
	return m.status != nil && m.status.Clock.Paused && m.chronicleVisible()
}

// handleKey is the top-level key dispatcher implementing the three modes of
// patterns/keymap.md, in priority order: ctrl+c always quits (rule 3);
// minibuffer-focused keys own the keyboard only when focus was explicitly
// acquired (rule 1); inspect-mode keys layer on top of, never replace, the
// global mode (rule 5 / keymap.md "Mode: inspect").
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit()
	}
	if m.mbFocused {
		return m.handleMinibufferKey(msg)
	}
	if m.inspecting() {
		if mdl, cmd, handled := m.handleInspectKey(msg); handled {
			return mdl, cmd
		}
	}
	if m.villagersVisible() {
		if mdl, cmd, handled := m.handleVillagersKey(msg); handled {
			return mdl, cmd
		}
	}
	return m.handleGlobalKey(msg)
}

// handleGlobalKey is patterns/keymap.md "Mode: global".
func (m Model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.quit()
	case "1":
		m.solo = false
		m.active = paneMap
		return m, nil
	case "2":
		return m.selectTab(paneChronicle)
	case "3":
		return m.selectTab(paneMetatron)
	case "4":
		return m.selectTab(paneVillagers)
	case "tab":
		return m.selectTab(nextDockTab(m.dockTab))
	case "shift+tab":
		return m.selectTab(prevDockTab(m.dockTab))
	case "m":
		return m.focusMinibuffer()
	case "enter":
		// Narrow-fallback-only affordance (focus-contract.md scope): the
		// metatron pane's dormant input line focuses on 'm' *or* Enter,
		// mirroring minibuffer.md's placeholder hint since there is no
		// separate always-visible minibuffer bar to press 'm' toward.
		if !isWidescreen(m.width) && m.active == paneMetatron {
			return m.focusMinibuffer()
		}
	case "esc":
		// Rule 3, "esc always releases" — here nothing is focused, so the
		// next thing esc releases is a solo zoom (solo-views.md state
		// machine: "solo(k) --esc--> home, tab=k").
		if m.solo {
			m.solo = false
		}
		return m, nil
	case "a", "t", "r":
		if m.chronicleVisible() {
			switch msg.String() {
			case "a": // all → each villager → all
				m.chronAgent++
				if m.replica == nil || m.chronAgent >= len(m.replica.Agents) {
					m.chronAgent = -1
				}
			case "t": // all → each thread seen in the ring → all
				m.chronThread = nextThread(m.replica, m.chronThread)
			case "r":
				m.chronRaw = !m.chronRaw
			}
		}
	case "up", "down", "left", "right", "c":
		if m.mapControllable() {
			switch msg.String() {
			case "up":
				m.panY -= 4
			case "down":
				m.panY += 4
			case "left":
				m.panX -= 4
			case "right":
				m.panX += 4
			case "c":
				m.panX, m.panY = 0, 0
			}
		}
	case " ":
		if m.connected && m.status != nil {
			cmd := "pause"
			if m.status.Clock.Paused {
				cmd = "resume"
			}
			return m, timeControl(m.client, cmd, nil)
		}
	case "[", "]":
		if m.connected && m.status != nil {
			cur := clock.Speed(m.status.Clock.Speed)
			idx := 1 // default 4x position
			for i, s := range speedSteps {
				if s == cur {
					idx = i
				}
			}
			if msg.String() == "[" && idx > 0 {
				idx--
			} else if msg.String() == "]" && idx < len(speedSteps)-1 {
				idx++
			}
			if speedSteps[idx] != cur {
				return m, timeControl(m.client, "set_speed", ipc.SetSpeedArgs{Speed: string(speedSteps[idx])})
			}
		}
	}
	return m, nil
}

// nextDockTab/prevDockTab cycle the three dock tabs (tab/shift+tab aliases,
// keymap.md "Migration notes" — not load-bearing).
func nextDockTab(cur pane) pane {
	switch cur {
	case paneChronicle:
		return paneMetatron
	case paneMetatron:
		return paneVillagers
	default:
		return paneChronicle
	}
}

func prevDockTab(cur pane) pane {
	switch cur {
	case paneChronicle:
		return paneVillagers
	case paneMetatron:
		return paneChronicle
	default:
		return paneMetatron
	}
}

// selectTab implements the solo-views.md state machine for k ∈
// {chronicle, metatron, villagers}: same key on the already-selected tab zooms
// solo; same key again returns home. A different key while solo switches
// which tab is solo'd rather than dropping back to home — the state
// machine only specifies the same-key case, so this keeps solo a pure
// "the dock at full width" (dock.md: "same component, two widths") rather
// than adding an implicit extra "back home" side effect to tab-switching.
// active mirrors the selection so a resize down to the narrow fallback
// shows the same content that was last looked at.
func (m Model) selectTab(k pane) (tea.Model, tea.Cmd) {
	if isWidescreen(m.width) {
		if m.solo {
			if m.dockTab == k {
				m.solo = false
			} else {
				m.dockTab = k
			}
		} else if m.dockTab == k {
			m.solo = true
		} else {
			m.dockTab = k
		}
	} else {
		m.dockTab = k
	}
	m.active = k
	var cmd tea.Cmd
	if k == paneMetatron {
		m.metatronUnseen = false
		m.mbFlash = ""
		if m.connected && m.client != nil {
			cmd = fetchConsoleStatus(m.client)
		}
	}
	return m, cmd
}

// focusMinibuffer is the 'm' key (focus-contract.md rule 1: "text capture
// begins solely on an explicit focus action"). In the narrow fallback the
// input line only exists inside the metatron pane, so focusing also
// switches to it — the focused chrome must be visible the instant it is
// focused (rule 2).
func (m Model) focusMinibuffer() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if !isWidescreen(m.width) {
		mdl, c := m.selectTab(paneMetatron)
		m = mdl.(Model)
		cmd = c
	}
	m.mbFocused = true
	m.mbErr = ""
	m.mbHistPos = len(m.mbHistory)
	m.mbDraft = ""
	m.metatronUnseen = false
	m.mbFlash = ""
	return m, cmd
}

// handleMinibufferKey is patterns/keymap.md "Mode: minibuffer focused" —
// every key has a visible effect (focus-contract.md rule 4); there is no
// key whose press produces no observable change.
func (m Model) handleMinibufferKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Rule 3: "esc always releases. One keypress returns full
		// keyboard control, instantly."
		m.mbFocused = false
		m.mbHistPos = len(m.mbHistory)
	case tea.KeyEnter:
		text := strings.TrimSpace(m.mbInput)
		if text == "" {
			// "⏎ on an empty buffer releases focus (no-op send)."
			m.mbFocused = false
			return m, nil
		}
		if !m.connected || m.client == nil {
			m.mbFocused = false
			m.mbErr = "not connected"
			return m, nil
		}
		m.mbHistory = append(m.mbHistory, text)
		m.transcript = append(m.transcript, "you: "+text)
		m.mbInput = ""
		m.mbHistPos = len(m.mbHistory)
		m.mbBusy = true
		m.mbErr = ""
		// "Focus is released automatically on send; esc (or any
		// navigation) just proceeds — busy never blocks the UI."
		m.mbFocused = false
		return m, sendConsole(m.client, text)
	case tea.KeyBackspace:
		if r := []rune(m.mbInput); len(r) > 0 {
			m.mbInput = string(r[:len(r)-1])
		}
	case tea.KeyUp:
		m.historyUp()
	case tea.KeyDown:
		m.historyDown()
	case tea.KeySpace:
		m.mbInput += " "
	case tea.KeyRunes:
		m.mbInput += string(msg.Runes)
	}
	return m, nil
}

func (m *Model) historyUp() {
	if len(m.mbHistory) == 0 {
		return
	}
	if m.mbHistPos == len(m.mbHistory) {
		m.mbDraft = m.mbInput
	}
	if m.mbHistPos > 0 {
		m.mbHistPos--
	}
	m.mbInput = m.mbHistory[m.mbHistPos]
}

func (m *Model) historyDown() {
	if m.mbHistPos < len(m.mbHistory) {
		m.mbHistPos++
	}
	if m.mbHistPos >= len(m.mbHistory) {
		m.mbHistPos = len(m.mbHistory)
		m.mbInput = m.mbDraft
		return
	}
	m.mbInput = m.mbHistory[m.mbHistPos]
}

// handleInspectKey is patterns/keymap.md "Mode: inspect" — layered on top
// of the global mode, never replacing it (handled is false for any key it
// does not own, so handleKey falls through to handleGlobalKey). J/K scroll
// the always-on detail pane (contract §5/§6, R6); ⏎ is reserved (R7) — the
// pane's [future: actions] slot and detailActions are the documented
// attachment point (FR-009, contract §5 "Extension point"), so it is
// swallowed (handled=true) rather than left to fall through by accident.
func (m Model) handleInspectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "j":
		m.chronMoveSelection(1)
		return m, nil, true
	case "k":
		m.chronMoveSelection(-1)
		return m, nil, true
	case "g":
		m.chronJumpFirst()
		return m, nil, true
	case "G":
		m.chronJumpLast()
		return m, nil, true
	case "J":
		m.chronDetailScroll++ // clamped to content length at render time
		return m, nil, true
	case "K":
		if m.chronDetailScroll > 0 {
			m.chronDetailScroll--
		}
		return m, nil, true
	case "enter":
		return m, nil, true // reserved no-op (contract §5 "Extension point")
	}
	return m, nil, false
}

// handleVillagersKey is contracts/state-and-keys.md's key grammar table,
// layered on top of the global mode exactly like handleInspectKey — j/k/g/G
// select in the roster, ⏎ opens the detail view, esc closes it. Unlike
// inspect mode this does not require the clock to be paused: it is gated
// purely on villagersVisible() (dock.md "Each tab keeps its own state").
// esc on the roster returns handled=false so it falls through to the
// global esc (focus-contract.md rule 3: minibuffer → detail → solo → home).
func (m Model) handleVillagersKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	m.clampVillSelected()
	switch msg.String() {
	case "esc":
		if m.villDetail {
			m.villDetail = false
			return m, nil, true
		}
		return m, nil, false
	case "j":
		if !m.villDetail {
			m.villMoveSelection(1)
		}
		return m, nil, true
	case "k":
		if !m.villDetail {
			m.villMoveSelection(-1)
		}
		return m, nil, true
	case "g":
		if !m.villDetail {
			m.villJumpFirst()
		}
		return m, nil, true
	case "G":
		if !m.villDetail {
			m.villJumpLast()
		}
		return m, nil, true
	case "enter":
		if !m.villDetail && m.villCount() > 0 {
			m.villDetail = true
		}
		return m, nil, true
	}
	return m, nil, false
}

// villCount is len(replica.Agents), 0 with a nil/empty replica — the bound
// every villSelected read clamps against (R5).
func (m Model) villCount() int {
	if m.replica == nil {
		return 0
	}
	return len(m.replica.Agents)
}

// clampVillSelected bounds villSelected to [0, villCount()) — called on
// reconnect (connectedMsg swaps the replica wholesale) and defensively on
// every villagers keypress; renderers clamp again at read time the same way
// chronSelectionBase does for the chronicle.
func (m *Model) clampVillSelected() {
	n := m.villCount()
	if n == 0 {
		m.villSelected = 0
		return
	}
	m.villSelected = clampInt(m.villSelected, 0, n-1)
}

func (m *Model) villMoveSelection(delta int) {
	n := m.villCount()
	if n == 0 {
		return
	}
	m.villSelected = clampInt(m.villSelected+delta, 0, n-1)
}

func (m *Model) villJumpFirst() {
	if m.villCount() == 0 {
		return
	}
	m.villSelected = 0
}

func (m *Model) villJumpLast() {
	n := m.villCount()
	if n == 0 {
		return
	}
	m.villSelected = n - 1
}

// chronSelectionBase resolves the "current" selection: if nothing is
// selected yet (or the ring rotated past the old index), it starts from
// the tail — the most recently paused-over event.
func (m Model) chronSelectionBase() int {
	n := len(m.events)
	if n == 0 {
		return -1
	}
	if m.chronSelected < 0 || m.chronSelected >= n {
		return n - 1
	}
	return m.chronSelected
}

func (m *Model) chronMoveSelection(delta int) {
	n := len(m.events)
	if n == 0 {
		return
	}
	sel := m.chronSelectionBase() + delta
	if sel < 0 {
		sel = 0
	}
	if sel >= n {
		sel = n - 1
	}
	m.chronSelected = sel
	m.chronDetailScroll = 0 // data-model.md: reset on selection move
}

func (m *Model) chronJumpFirst() {
	if len(m.events) == 0 {
		return
	}
	m.chronSelected = 0
	m.chronDetailScroll = 0
}

func (m *Model) chronJumpLast() {
	if len(m.events) == 0 {
		return
	}
	m.chronSelected = len(m.events) - 1
	m.chronDetailScroll = 0
}

// detailAction is one future jump-off action attachable to the detail
// pane's bottom-right "[future: actions]" slot (contract §5 "Extension
// point", FR-009). Not populated by this feature — the hook exists so a
// later feature can wire actions (e.g. "jump to related event") without
// re-deriving where they attach; ⏎ (handleInspectKey) is the reserved key.
type detailAction struct {
	Label string
}

// detailActions returns the jump-off actions available for one event.
// Always nil today; deliberately takes the event so a future
// implementation doesn't need to change the call site.
func detailActions(e store.Event) []detailAction {
	return nil
}

func (m Model) handlePush(p ipc.Push) (tea.Model, tea.Cmd) {
	if !m.connected || m.client == nil {
		return m, nil
	}
	switch p.Push {
	case "event":
		m.applyEvent(*p.Event)
		return m, listen(m.client)
	case "dropped":
		// Overflow: the replica may have missed events — resync from scratch.
		m.client.Close()
		m.client = nil
		m.connected = false
		m.lastErr = "stream overflow; resyncing"
		return m, connect(m.w)
	}
	return m, listen(m.client)
}

// applyEvent folds one pushed event into the replica and the chronicle ring.
// Events at or before the state snapshot's seq are already reflected and skipped.
func (m *Model) applyEvent(e store.Event) {
	if e.Seq <= m.lastSeq {
		return
	}
	if m.replica != nil {
		m.replica.Apply(e) // same reducer as the daemon; errors are cosmetic here
		if e.Tick > m.replica.Tick {
			m.replica.Tick = e.Tick
		}
	}
	m.lastSeq = e.Seq
	m.events = append(m.events, e)
	if len(m.events) > chronicleCap {
		m.events = m.events[len(m.events)-chronicleCap:]
	}
}

// clampGeometry re-bounds the size-independent-in-value-but-not-in-validity
// fields on resize (B5). Almost all geometry in this package is derived
// fresh every View() call from (width, height) directly — there is no
// cache to go stale. The two fields that *do* persist a value across
// frames are covered here:
func (m *Model) clampGeometry() {
	// Pan offset: render-time clamping in renderMapGrid already keeps the
	// *visible* camera window inside the map regardless of panX/panY's
	// magnitude, but an offset accumulated at a wide viewport is still
	// stale once the viewport shrinks — cap it to a map-sized window so
	// it can never represent "off the map" at any size.
	if m.gameMap != nil {
		m.panX = clampInt(m.panX, -m.gameMap.W, m.gameMap.W)
		m.panY = clampInt(m.panY, -m.gameMap.H, m.gameMap.H)
	}
	// Chronicle selection: bounded to the current ring length. Read-time
	// callers (chronSelectionBase) already tolerate an out-of-range
	// value defensively, but the cached field itself should stay honest
	// rather than merely tolerated.
	if m.chronSelected >= len(m.events) {
		m.chronSelected = len(m.events) - 1
	}
	// dockTab/solo are a small fixed enum + bool, never derived from
	// width/height — nothing to clamp; the narrow fallback ignores
	// `solo` entirely and `dockTab` is always one of the three valid
	// tabs regardless of layout.
}

// agentNames resolves the replica's roster for the chronicle grammar's
// name-resolution (patterns/chronicle-grammar.md, "the existing chronNames
// mechanism", generalized to raw event payloads).
func (m Model) agentNames() []string {
	if m.replica == nil {
		return nil
	}
	names := make([]string, len(m.replica.Agents))
	for i, a := range m.replica.Agents {
		names[i] = a.Name
	}
	return names
}
