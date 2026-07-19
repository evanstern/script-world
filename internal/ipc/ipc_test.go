package ipc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
)

// harness runs a real loop + server + store against a temp world dir.
type harness struct {
	w      *world.World
	st     *store.Store
	loop   *sim.Loop
	srv    *Server
	cancel context.CancelFunc
	done   chan error
}

func newHarness(t *testing.T, speed clock.Speed) *harness {
	t.Helper()
	dir := t.TempDir() + "/w"
	w, err := world.Create(dir, "test", 42)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	gm := w.Map()
	state := sim.NewState(42, gm)
	state.Speed = speed

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServer(w, st, cancel)
	loop := sim.NewLoop(state, gm, st, srv.Broadcast)
	srv.SetLoop(loop)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	h := &harness{w: w, st: st, loop: loop, srv: srv, cancel: cancel, done: make(chan error, 1)}
	go srv.Serve()
	go func() { h.done <- loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("loop did not stop")
		}
		srv.Close()
		st.Close()
	})
	return h
}

func (h *harness) dial(t *testing.T) *Client {
	t.Helper()
	c, err := Dial(h.w.SockPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestStatusRoundTripUnder2s(t *testing.T) {
	h := newHarness(t, clock.SpeedMax)
	c := h.dial(t)
	start := time.Now()
	sd, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("status took %v, SC-002 requires < 2s", elapsed)
	}
	if sd.World.Name != "test" || sd.World.Seed != 42 {
		t.Errorf("world section wrong: %+v", sd.World)
	}
	if sd.Daemon.Pid == 0 {
		t.Errorf("daemon section missing pid: %+v", sd.Daemon)
	}
}

func TestSubscribeFromZeroIsGapless(t *testing.T) {
	h := newHarness(t, clock.SpeedMax)
	c := h.dial(t)

	// Let the world produce some history first.
	waitForSeq(t, c, 50)

	since := int64(0)
	if err := c.Subscribe(&since); err != nil {
		t.Fatal(err)
	}
	var last int64
	deadline := time.After(10 * time.Second)
	for last < 100 {
		select {
		case p, ok := <-c.Pushes():
			if !ok {
				t.Fatal("push channel closed early")
			}
			if p.Push == "dropped" {
				since := p.LastSeq
				if err := c.Subscribe(&since); err != nil {
					t.Fatal(err)
				}
				last = p.LastSeq
				continue
			}
			if p.Event.Seq != last+1 {
				t.Fatalf("gap in stream: got seq %d after %d", p.Event.Seq, last)
			}
			last = p.Event.Seq
		case <-deadline:
			t.Fatalf("only reached seq %d in 10s", last)
		}
	}
}

func TestAbruptDisconnectLeavesLoopTicking(t *testing.T) {
	h := newHarness(t, clock.SpeedMax)

	// A client that subscribes and then vanishes without goodbye (FR-011).
	raw, err := dialUnix(h.w.SockPath(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	raw.Write([]byte(`{"id":1,"cmd":"subscribe"}` + "\n"))
	time.Sleep(100 * time.Millisecond)
	raw.Close() // abrupt

	// Also: garbage on the wire must only kill that session.
	raw2, err := dialUnix(h.w.SockPath(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	raw2.Write([]byte("this is not json\n"))
	time.Sleep(100 * time.Millisecond)

	c := h.dial(t)
	sd1, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	sd2, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sd2.Clock.Tick <= sd1.Clock.Tick {
		t.Errorf("loop stalled after abrupt disconnects: tick %d -> %d", sd1.Clock.Tick, sd2.Clock.Tick)
	}
}

func TestTimeControlCommands(t *testing.T) {
	h := newHarness(t, clock.SpeedMax)
	c := h.dial(t)

	sd, err := c.Status("pause", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sd.Clock.Paused {
		t.Fatal("pause did not pause")
	}
	pausedTick := sd.Clock.Tick
	time.Sleep(300 * time.Millisecond)
	sd, err = c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Clock.Tick != pausedTick {
		t.Errorf("tick advanced while paused: %d -> %d", pausedTick, sd.Clock.Tick)
	}

	// Idempotent pause: no error, still paused.
	if sd, err = c.Status("pause", nil); err != nil || !sd.Clock.Paused {
		t.Fatalf("second pause: %v %+v", err, sd.Clock)
	}

	if sd, err = c.Status("set_speed", SetSpeedArgs{Speed: "1x"}); err != nil {
		t.Fatal(err)
	} else if sd.Clock.Speed != "1x" {
		t.Errorf("speed = %s, want 1x", sd.Clock.Speed)
	}
	if _, err := c.Status("set_speed", SetSpeedArgs{Speed: "9000x"}); err == nil {
		t.Error("invalid speed should be rejected")
	}

	if sd, err = c.Status("resume", nil); err != nil || sd.Clock.Paused {
		t.Fatalf("resume: %v %+v", err, sd.Clock)
	}

	// The applied commands are themselves events in the log (R3).
	events, err := h.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sawPause, sawSpeed, sawResume bool
	for _, e := range events {
		switch e.Type {
		case "clock.paused":
			sawPause = true
		case "clock.speed_set":
			sawSpeed = true
		case "clock.resumed":
			sawResume = true
		}
	}
	if !sawPause || !sawSpeed || !sawResume {
		t.Errorf("command events missing from log: pause=%v speed=%v resume=%v", sawPause, sawSpeed, sawResume)
	}
}

func TestStateCommand(t *testing.T) {
	h := newHarness(t, clock.SpeedMax)
	c := h.dial(t)
	waitForSeq(t, c, 20)

	sd, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var state sim.State
	if err := json.Unmarshal(sd.State, &state); err != nil {
		t.Fatalf("state is not valid sim.State JSON: %v", err)
	}
	if state.Seed != 42 {
		t.Errorf("state seed = %d, want 42", state.Seed)
	}
	if len(state.Wanderers) == 0 {
		t.Error("state has no wanderers")
	}
	if sd.LastSeq == 0 {
		t.Error("state must report the log position it reflects")
	}

	// Coherence contract: subscribing from LastSeq replays nothing already
	// folded into the state — the first push has seq LastSeq+1 or later,
	// and applying pushes to the state must not error.
	since := sd.LastSeq
	if err := c.Subscribe(&since); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		select {
		case p := <-c.Pushes():
			if p.Push != "event" {
				continue
			}
			if p.Event.Seq <= sd.LastSeq {
				t.Fatalf("push seq %d predates state's last_seq %d", p.Event.Seq, sd.LastSeq)
			}
			if err := state.Apply(*p.Event); err != nil {
				t.Fatalf("state replica cannot apply pushed event: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no pushes after state+subscribe")
		}
	}
}

func TestUnknownCommandKeepsConnection(t *testing.T) {
	h := newHarness(t, clock.Speed4x)
	c := h.dial(t)
	if _, err := c.Call("frobnicate", nil); err == nil {
		t.Error("unknown cmd should error")
	}
	if _, err := c.Status("status", nil); err != nil {
		t.Errorf("connection should survive unknown cmd: %v", err)
	}
}

func waitForSeq(t *testing.T, c *Client, seq int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sd, err := c.Status("status", nil)
		if err != nil {
			t.Fatal(err)
		}
		if sd.Log.LastSeq >= seq {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("log never reached seq %d", seq)
}

func TestStatusDataShape(t *testing.T) {
	// The wire shape is the contract; field names must match exactly.
	sd := StatusData{}
	b, _ := json.Marshal(sd)
	for _, key := range []string{`"world"`, `"clock"`, `"daemon"`, `"log"`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("status shape missing %s in %s", key, b)
		}
	}
}
