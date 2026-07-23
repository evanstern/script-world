package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/world"
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
	// No LLM on this harness: max stays legal for pure-sim worlds (TASK-20).
	if sd, err = c.Status("set_speed", SetSpeedArgs{Speed: "max"}); err != nil {
		t.Fatalf("pure-sim world must accept max: %v", err)
	} else if sd.Clock.Speed != "max" {
		t.Errorf("speed = %s, want max", sd.Clock.Speed)
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
	if len(state.Agents) == 0 {
		t.Error("state has no agents")
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

// --- TASK-19: large state replies ---

// fakeDaemon speaks the wire protocol from a canned reply function, so tests
// can shape replies the real loop cannot produce (multi-MiB states, raw
// over-long lines). Returns the socket path to Dial.
func fakeDaemon(t *testing.T, reply func(req Request) []byte) string {
	t.Helper()
	sock := t.TempDir() + "/fake.sock"
	ln, err := listenUnix(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 0, 64*1024), maxRequestBytes)
		for scanner.Scan() {
			var req Request
			if json.Unmarshal(scanner.Bytes(), &req) != nil {
				return
			}
			if b := reply(req); b != nil {
				if _, err := conn.Write(append(b, '\n')); err != nil {
					return
				}
			}
		}
	}()
	return sock
}

func dialFake(t *testing.T, sock string) *Client {
	t.Helper()
	c, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// TestFetchStateOver1MiBSucceeds is TASK-19 AC#1's success arm: a state
// payload past the old shared 1 MiB line cap now round-trips (the gru-proof
// failure was exactly this — a healthy daemon whose state line the client's
// scanner refused).
func TestFetchStateOver1MiBSucceeds(t *testing.T) {
	stateJSON, err := json.Marshal(map[string]string{"pad": strings.Repeat("x", 2<<20)})
	if err != nil {
		t.Fatal(err)
	}
	sock := fakeDaemon(t, func(req Request) []byte {
		data, _ := json.Marshal(StateData{State: stateJSON, LastSeq: 7})
		b, _ := json.Marshal(Response{ID: req.ID, OK: true, Data: data})
		return b
	})
	c := dialFake(t, sock)
	sd, err := c.FetchState()
	if err != nil {
		t.Fatalf("state over 1 MiB must succeed: %v", err)
	}
	if len(sd.State) <= 1<<20 {
		t.Fatalf("test payload only %d bytes, not over the old 1 MiB cap", len(sd.State))
	}
	if sd.LastSeq != 7 {
		t.Errorf("last_seq = %d, want 7", sd.LastSeq)
	}
}

// TestServerSubstitutesActionableErrorForOversizedReply: the daemon never
// emits a line past maxReplyBytes — it answers with an ok:false error that
// names the sizes, on the same request ID.
func TestServerSubstitutesActionableErrorForOversizedReply(t *testing.T) {
	clientEnd, serverEnd := net.Pipe()
	defer clientEnd.Close()
	defer serverEnd.Close()
	sess := &session{conn: serverEnd}
	go sess.writeResponse(Response{ID: 9, OK: true,
		Data: json.RawMessage(`"` + strings.Repeat("x", maxReplyBytes) + `"`)})

	scanner := bufio.NewScanner(clientEnd)
	scanner.Buffer(make([]byte, 0, 64*1024), maxReplyBytes)
	if !scanner.Scan() {
		t.Fatalf("no substituted reply: %v", scanner.Err())
	}
	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.ID != 9 {
		t.Fatalf("want ok:false on id 9, got %+v", resp)
	}
	if !strings.HasPrefix(resp.Error, replyTooLargePrefix) {
		t.Errorf("error must carry the classifiable prefix %q: %q", replyTooLargePrefix, resp.Error)
	}
	if !strings.Contains(resp.Error, "protocol cap") {
		t.Errorf("error should be actionable (name the cap): %q", resp.Error)
	}
}

// TestClientClassifiesServerRefusalAsFatal: a server "reply too large" error
// surfaces as ErrReplyTooLarge so callers know retrying is pointless.
func TestClientClassifiesServerRefusalAsFatal(t *testing.T) {
	sock := fakeDaemon(t, func(req Request) []byte {
		b, _ := json.Marshal(Response{ID: req.ID, OK: false,
			Error: replyTooLargePrefix + ": reply is 99999999 bytes, over the protocol cap"})
		return b
	})
	c := dialFake(t, sock)
	if _, err := c.FetchState(); !errors.Is(err, ErrReplyTooLarge) {
		t.Fatalf("want ErrReplyTooLarge, got %v", err)
	}
}

// TestOversizedRawLineFailsFastNotForever is TASK-19 AC#1's failure arm at
// the transport: even a daemon that streams a line past the client's cap
// (version skew) produces a prompt, classifiable error — never a hang and
// never the old silent scanner death that fed the endless retry loop.
func TestOversizedRawLineFailsFastNotForever(t *testing.T) {
	line := append([]byte(`{"id":1,"ok":true,"data":"`),
		bytes.Repeat([]byte("x"), maxReplyBytes+(1<<20))...)
	line = append(line, '"', '}')
	sock := fakeDaemon(t, func(req Request) []byte { return line })
	c := dialFake(t, sock)

	done := make(chan error, 1)
	go func() {
		_, err := c.Call("state", nil)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrReplyTooLarge) {
			t.Fatalf("want ErrReplyTooLarge, got %v", err)
		}
		if !strings.Contains(err.Error(), "exceeded") {
			t.Errorf("error should be actionable: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("client hung on an oversized reply line")
	}
}

// TestMiracleMoveRoundTrip (spec 016 T011): the operator "miracle" command
// lands a villager move over the wire on a pure-sim world (no LLM/angel), spends
// a charge, and the move is visible in the next state fetch. The world is paused
// first so the villagers hold still for a deterministic before/after.
func TestMiracleMoveRoundTrip(t *testing.T) {
	h := newHarness(t, clock.Speed1x)
	c := h.dial(t)

	if _, err := c.Status("pause", nil); err != nil {
		t.Fatal(err)
	}
	sd, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var before sim.State
	if err := json.Unmarshal(sd.State, &before); err != nil {
		t.Fatal(err)
	}
	ax, ay := before.Agents[0].X, before.Agents[0].Y
	// Another villager's tile is a guaranteed-passable destination (villagers
	// may share a tile), so the test needs no map to pick a valid target.
	var bx, by int
	found := false
	for i := 1; i < len(before.Agents); i++ {
		if before.Agents[i].X != ax || before.Agents[i].Y != ay {
			bx, by = before.Agents[i].X, before.Agents[i].Y
			found = true
			break
		}
	}
	if !found {
		t.Skip("all villagers share agent 0's tile")
	}

	data, err := c.Call("miracle", MiracleArgs{Kind: "move", Class: "villager", X: ax, Y: ay, ToX: bx, ToY: by})
	if err != nil {
		t.Fatalf("miracle move rejected over the wire: %v", err)
	}
	var md MiracleData
	if err := json.Unmarshal(data, &md); err != nil {
		t.Fatal(err)
	}
	if md.Kind != "move" || md.Gratis {
		t.Errorf("miracle data wrong: %+v", md)
	}
	if md.Charges != 0 {
		t.Errorf("charges after a charged move = %d, want 0 (genesis 1 spent)", md.Charges)
	}
	if !strings.Contains(md.Summary, "moved") {
		t.Errorf("summary = %q, want a human rendering", md.Summary)
	}

	// The move is visible in the next state fetch.
	sd2, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var after sim.State
	if err := json.Unmarshal(sd2.State, &after); err != nil {
		t.Fatal(err)
	}
	if after.Agents[0].X != bx || after.Agents[0].Y != by {
		t.Errorf("villager at (%d,%d) after move, want (%d,%d)", after.Agents[0].X, after.Agents[0].Y, bx, by)
	}

	// Unknown kinds are refused cleanly; the connection survives.
	if _, err := c.Call("miracle", MiracleArgs{Kind: "smite", X: ax, Y: ay}); err == nil {
		t.Error("unknown miracle kind should be refused")
	}
	if _, err := c.Status("status", nil); err != nil {
		t.Errorf("connection should survive a refused miracle: %v", err)
	}
}

// TestMiracleForcedMoveZeroBank is US2-AS1 / T012: the operator "--force" door
// lands a miracle with an empty charge bank (gratis waives the cost) and leaves
// the bank untouched at zero, while validation and recording are unchanged. The
// world is paused for a deterministic before/after.
func TestMiracleForcedMoveZeroBank(t *testing.T) {
	h := newHarness(t, clock.Speed1x)
	c := h.dial(t)

	if _, err := c.Status("pause", nil); err != nil {
		t.Fatal(err)
	}
	sd, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var before sim.State
	if err := json.Unmarshal(sd.State, &before); err != nil {
		t.Fatal(err)
	}
	// Empty the bank via a charged move so no charges remain (genesis is 1).
	ax, ay := before.Agents[0].X, before.Agents[0].Y
	var bx, by int
	found := false
	for i := 1; i < len(before.Agents); i++ {
		if before.Agents[i].X != ax || before.Agents[i].Y != ay {
			bx, by = before.Agents[i].X, before.Agents[i].Y
			found = true
			break
		}
	}
	if !found {
		t.Skip("all villagers share agent 0's tile")
	}
	if _, err := c.Call("miracle", MiracleArgs{Kind: "move", Class: "villager", X: ax, Y: ay, ToX: bx, ToY: by}); err != nil {
		t.Fatalf("first (charged) move rejected: %v", err)
	}

	// Bank is now 0; a non-forced move must be refused for want of a charge.
	if _, err := c.Call("miracle", MiracleArgs{Kind: "move", Class: "villager", X: bx, Y: by, ToX: ax, ToY: ay}); err == nil {
		t.Fatal("a charged move with an empty bank should be refused")
	}

	// The forced move lands anyway, and the bank stays at zero.
	data, err := c.Call("miracle", MiracleArgs{Kind: "move", Class: "villager", X: bx, Y: by, ToX: ax, ToY: ay, Gratis: true})
	if err != nil {
		t.Fatalf("forced move with an empty bank rejected: %v", err)
	}
	var md MiracleData
	if err := json.Unmarshal(data, &md); err != nil {
		t.Fatal(err)
	}
	if !md.Gratis {
		t.Errorf("forced move data should report gratis=true: %+v", md)
	}
	if md.Charges != 0 {
		t.Errorf("bank after a forced move = %d, want 0 (untouched)", md.Charges)
	}

	sd2, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var after sim.State
	if err := json.Unmarshal(sd2.State, &after); err != nil {
		t.Fatal(err)
	}
	if after.MetatronCharges != 0 {
		t.Errorf("state bank = %d after forced move, want 0", after.MetatronCharges)
	}
	if after.Agents[0].X != ax || after.Agents[0].Y != ay {
		t.Errorf("forced move did not land: agent 0 at (%d,%d), want (%d,%d)", after.Agents[0].X, after.Agents[0].Y, ax, ay)
	}
}

// TestMiracleGiveRoundTrip (spec 016 T022): the operator "miracle" command lands
// a give_item over the wire on a pure-sim world, resolving the villager by NAME
// (contracts §2), spending a charge, with the grant visible in the next state
// fetch. The world is paused first for a deterministic before/after.
func TestMiracleGiveRoundTrip(t *testing.T) {
	h := newHarness(t, clock.Speed1x)
	c := h.dial(t)

	if _, err := c.Status("pause", nil); err != nil {
		t.Fatal(err)
	}
	sd, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var before sim.State
	if err := json.Unmarshal(sd.State, &before); err != nil {
		t.Fatal(err)
	}
	beforeRaw := before.Agents[0].Inv.FoodRaw

	data, err := c.Call("miracle", MiracleArgs{Kind: "give_item", Villager: sim.AgentNames[0], Item: "food_raw", Qty: 2})
	if err != nil {
		t.Fatalf("give_item rejected over the wire: %v", err)
	}
	var md MiracleData
	if err := json.Unmarshal(data, &md); err != nil {
		t.Fatal(err)
	}
	if md.Kind != "give_item" || md.Gratis {
		t.Errorf("miracle data wrong: %+v", md)
	}
	if md.Charges != 0 {
		t.Errorf("charges after a charged grant = %d, want 0 (genesis 1 spent)", md.Charges)
	}
	if !strings.Contains(md.Summary, "granted") {
		t.Errorf("summary = %q, want a human rendering", md.Summary)
	}

	sd2, err := c.FetchState()
	if err != nil {
		t.Fatal(err)
	}
	var after sim.State
	if err := json.Unmarshal(sd2.State, &after); err != nil {
		t.Fatal(err)
	}
	if got := after.Agents[0].Inv.FoodRaw; got != beforeRaw+2 {
		t.Errorf("FoodRaw after grant = %d, want %d", got, beforeRaw+2)
	}

	// An unknown villager name is refused cleanly; the connection survives.
	if _, err := c.Call("miracle", MiracleArgs{Kind: "give_item", Villager: "Nobody", Item: "wood", Qty: 1}); err == nil {
		t.Error("give_item to an unknown villager should be refused")
	}
	if _, err := c.Status("status", nil); err != nil {
		t.Errorf("connection should survive a refused give_item: %v", err)
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

// fakeGovernor is a scripted daemon governor surface for the status-fold tests.
type fakeGovernor struct {
	debt float64
	jobs int
}

func (f fakeGovernor) GovernorStatus() (float64, int) { return f.debt, f.jobs }

// TestStatusGovernorFold (spec 028 US1): when a governor is attached its debt
// snapshot folds into the clock section, exactly the way the LLM snapshot folds.
func TestStatusGovernorFold(t *testing.T) {
	h := newHarness(t, clock.Speed16x)
	h.srv.SetGovernor(fakeGovernor{debt: 1.4, jobs: 3})
	c := h.dial(t)

	sd, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Clock.GovernorDebt != 1.4 || sd.Clock.GovernorJobs != 3 {
		t.Errorf("governor fold = debt %v jobs %d, want 1.4/3", sd.Clock.GovernorDebt, sd.Clock.GovernorJobs)
	}
	// RequestedSpeed is always empty in US1 — the sim-state ceiling arrives in US2.
	if sd.Clock.RequestedSpeed != "" {
		t.Errorf("RequestedSpeed = %q, want empty in US1", sd.Clock.RequestedSpeed)
	}
}

// TestStatusNoGovernorZero (US1-AC4): with no governor attached (a no-LLM
// world), status reports zero governor values — no machinery, no debt.
func TestStatusNoGovernorZero(t *testing.T) {
	h := newHarness(t, clock.Speed4x)
	c := h.dial(t)

	sd, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Clock.GovernorDebt != 0 || sd.Clock.GovernorJobs != 0 || sd.Clock.RequestedSpeed != "" {
		t.Errorf("no-governor status carried governor values: %+v", sd.Clock)
	}
}

// TestStatusGovernorOmitempty: the three spec-028 fields are omitempty, so an
// inert governor keeps status byte-identical to a pre-028 status.
func TestStatusGovernorOmitempty(t *testing.T) {
	b, _ := json.Marshal(StatusData{})
	for _, key := range []string{"requested_speed", "governor_debt", "governor_jobs"} {
		if strings.Contains(string(b), key) {
			t.Errorf("zero status must omit %q (pre-028 byte shape) in %s", key, b)
		}
	}
}

// TestLLMCallAndDegradedWorld covers the llm_call protocol command and AC#3:
// a dead inference endpoint degrades LLM calls while the simulation ticks on
// untouched.
func TestLLMCallAndDegradedWorld(t *testing.T) {
	h := newHarness(t, clock.SpeedMax)

	// A live local mock proves routing over the protocol.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "thought"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2},
		})
	}))
	defer mock.Close()

	orch, err := llm.New(llm.Config{
		MonthlyBudgetUSD: 100,
		Local:            llm.LocalConfig{Endpoint: mock.URL, Model: "test-local"},
		Cloud:            llm.CloudConfig{Model: "claude-opus-4-8", Endpoint: "http://127.0.0.1:1", InputUSDPerMTok: 5, OutputUSDPerMTok: 25},
	}, h.st)
	if err != nil {
		t.Fatal(err)
	}
	defer orch.Close()
	h.srv.SetLLM(orch)

	c := h.dial(t)

	// Routed call succeeds and reports its tier.
	data, err := c.Call("llm_call", LLMCallArgs{Kind: "planner", Prompt: "what next?"})
	if err != nil {
		t.Fatal(err)
	}
	var resp llm.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Tier != llm.TierLocal || resp.Text != "thought" {
		t.Errorf("llm_call response: %+v", resp)
	}

	// Status carries the llm section.
	sd, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The per-provider status table (spec 024) replaces the fixed local/cloud
	// fields; the legacy config derives a provider named "local".
	var localModel string
	if sd.LLM != nil {
		for _, p := range sd.LLM.Providers {
			if p.Name == "local" {
				localModel = p.Model
			}
		}
	}
	if sd.LLM == nil || localModel != "test-local" {
		t.Fatalf("status missing llm section: %+v", sd.LLM)
	}

	// With an LLM configured, max is refused with an actionable error and
	// 32x is the legal ceiling (TASK-20).
	if _, err := c.Status("set_speed", SetSpeedArgs{Speed: "max"}); err == nil {
		t.Error("LLM world must refuse speed max")
	} else if !strings.Contains(err.Error(), "32x") {
		t.Errorf("max refusal should point at 32x: %v", err)
	}
	if sd, err := c.Status("set_speed", SetSpeedArgs{Speed: "32x"}); err != nil {
		t.Fatalf("32x on an LLM world: %v", err)
	} else if sd.Clock.Speed != "32x" {
		t.Errorf("speed = %s, want 32x", sd.Clock.Speed)
	}

	// Kill the local model (AC#3): calls fail, the world does not.
	mock.Close()
	before, _ := c.Status("status", nil)
	for i := 0; i < 4; i++ {
		c.Call("llm_call", LLMCallArgs{Kind: "planner", Prompt: "hello?"})
	}
	time.Sleep(300 * time.Millisecond)
	after, err := c.Status("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Clock.Tick <= before.Clock.Tick {
		t.Fatal("simulation stalled while LLM tier was down (AC#3)")
	}
	localUp := false
	if after.LLM != nil {
		for _, p := range after.LLM.Providers {
			if p.Name == "local" {
				localUp = p.Up
			}
		}
	}
	if localUp {
		t.Error("local tier should report down after repeated failures")
	}

	// Cloud tier at a dead port: same story, plus the error reaches the client.
	if _, err := c.Call("llm_call", LLMCallArgs{Kind: "narrator", Prompt: "x"}); err == nil {
		t.Error("cloud call against dead endpoint should error")
	}
}
