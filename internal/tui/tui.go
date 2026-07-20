// Package tui is the attachable Bubble Tea client: four panes over a live
// world replica maintained by log shipping — initial state via the protocol
// "state" command, then subscribed events applied through the same
// sim.State reducer the daemon runs.
package tui

import (
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/ipc"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
	"github.com/evanstern/script-world/internal/worldmap"
)

type pane int

const (
	paneMap pane = iota
	paneChronicle
	paneMetatron
	paneSouls
	paneCount
)

var paneNames = [paneCount]string{"map", "chronicle", "metatron", "souls"}

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

	replica *sim.State      // world replica, event-sourced client-side
	gameMap *worldmap.Map   // terrain, regenerated locally from the manifest
	status  *ipc.StatusData // latest clock/daemon status (1s poll)
	events  []store.Event   // chronicle ring, newest last
	lastSeq int64

	active        pane
	width, height int
	panX, panY    int // map-pane camera offset from the wanderer centroid
	quitting      bool
}

func New(w *world.World) Model {
	return Model{w: w, gameMap: w.Map()}
}

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
		m.width, m.height = msg.Width, msg.Height
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
		return m, listen(m.client)

	case disconnectedMsg:
		m.connected = false
		if m.client != nil {
			m.client.Close()
			m.client = nil
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
		m.status = msg.status
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

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		if m.client != nil {
			m.client.Close()
		}
		return m, tea.Quit
	case "1":
		m.active = paneMap
	case "2":
		m.active = paneChronicle
	case "3":
		m.active = paneMetatron
	case "4":
		m.active = paneSouls
	case "tab":
		m.active = (m.active + 1) % paneCount
	case "shift+tab":
		m.active = (m.active + paneCount - 1) % paneCount
	case "up", "down", "left", "right", "c":
		if m.active == paneMap {
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
