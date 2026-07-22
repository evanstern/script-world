package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/metatron"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/world"
)

// pushBufferSize bounds a slow subscriber; overflow cancels the subscription
// (never blocks the sim loop) and the client re-syncs with subscribe{since}.
const pushBufferSize = 1024

// maxRequestBytes bounds a single client→server request line. Requests are
// small (a command plus args), so 1 MiB is generous.
const maxRequestBytes = 1 << 20

// maxReplyBytes bounds a single server→client line and is the protocol's
// documented reply ceiling: the client sizes its read buffer to exactly this,
// and the server never emits a longer line (writeResponse substitutes an
// actionable error instead). Split from the old shared 1 MiB maxLineBytes
// (TASK-19): the "state" reply carries the whole sim.State on one line and
// outgrew 1 MiB on long runs, leaving clients stuck in a retry loop.
const maxReplyBytes = 64 << 20

// replyTooLargePrefix marks a server-substituted oversized-reply error so
// clients can classify it as fatal (ErrReplyTooLarge) — reconnecting cannot
// shrink the payload, so retrying is pointless.
const replyTooLargePrefix = "reply too large"

// Server hosts the UDS protocol for one world. The sim loop's lifecycle is
// fully decoupled from every session's (FR-011).
type Server struct {
	w        *world.World
	st       *store.Store
	loop     *sim.Loop
	llm      *llm.Orchestrator // nil when the world has no llm.json
	metatron Angel             // nil when the world has no llm.json
	shutdown func()            // requests daemon shutdown (graceful)
	started  time.Time

	ln net.Listener

	mu       sync.Mutex
	sessions map[*session]struct{}
	closed   bool
}

func NewServer(w *world.World, st *store.Store, shutdown func()) *Server {
	return &Server{
		w:        w,
		st:       st,
		shutdown: shutdown,
		started:  time.Now(),
		sessions: make(map[*session]struct{}),
	}
}

// SetLLM attaches the optional orchestrator (before Serve).
func (s *Server) SetLLM(o *llm.Orchestrator) { s.llm = o }

// Angel is the Metatron surface the server needs (TASK-12; test seam).
type Angel interface {
	Turn(ctx context.Context, text string) (metatron.TurnResult, error)
	Status() metatron.Status
}

// SetMetatron attaches the optional angel (before Serve).
func (s *Server) SetMetatron(a Angel) { s.metatron = a }

// SetLoop wires the sim loop in after construction (loop and server
// reference each other: the loop notifies the server, the server commands
// the loop). Must be called before Serve.
func (s *Server) SetLoop(loop *sim.Loop) { s.loop = loop }

// Listen binds the socket. The daemon sweeps stale sockets before calling.
func (s *Server) Listen() error {
	ln, err := listenUnix(s.w.SockPath())
	if err != nil {
		return fmt.Errorf("bind %s: %w", s.w.SockPath(), err)
	}
	s.ln = ln
	return nil
}

// Serve accepts sessions until Close. Each session failure is that
// session's problem only.
func (s *Server) Serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		sess := newSession(s, conn)
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			return
		}
		s.sessions[sess] = struct{}{}
		s.mu.Unlock()
		go sess.serve()
	}
}

func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()
	if s.ln != nil {
		s.ln.Close()
	}
	for _, sess := range sessions {
		sess.close()
	}
	os.Remove(s.w.SockPath())
}

func (s *Server) dropSession(sess *session) {
	s.mu.Lock()
	delete(s.sessions, sess)
	s.mu.Unlock()
}

// Broadcast is the loop's notify callback. It must never block: sends are
// non-blocking, and a full buffer cancels that subscription with a
// "dropped" push.
func (s *Server) Broadcast(events []store.Event) {
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()
	for _, sess := range sessions {
		sess.offer(events)
	}
}

func (s *Server) subscriberCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for sess := range s.sessions {
		if sess.isSubscribed() {
			n++
		}
	}
	return n
}

// statusData wraps the loop's clock status in the full protocol shape.
func (s *Server) statusData(cs sim.Status) StatusData {
	return StatusData{
		World: WorldStatus{
			Name:          s.w.Manifest.Name,
			Seed:          s.w.Manifest.Seed,
			FormatVersion: s.w.Manifest.FormatVersion,
		},
		Clock: ClockStatus{
			Tick:            cs.Tick,
			GameTime:        cs.GameTime,
			Paused:          cs.Paused,
			Speed:           string(cs.Speed),
			EffectiveRate:   cs.EffectiveRate,
			Degraded:        cs.Degraded,
			MetatronCharges: cs.MetatronCharges,
		},
		Daemon: DaemonStatus{
			Pid:           os.Getpid(),
			UptimeSeconds: int64(time.Since(s.started).Seconds()),
			Subscribers:   s.subscriberCount(),
		},
		Log: LogStatus{LastSeq: cs.LastSeq},
	}
}

func (s *Server) statusDataFull(cs sim.Status) StatusData {
	sd := s.statusData(cs)
	if s.llm != nil {
		st := s.llm.StatusSnapshot()
		sd.LLM = &st
	}
	return sd
}

// session is one attached client.
type session struct {
	srv  *Server
	conn net.Conn

	writeMu sync.Mutex // one JSON line at a time on the wire

	subMu      sync.Mutex
	subscribed bool
	subCh      chan store.Event
	subQuit    chan struct{}
	lastSeq    int64 // newest seq offered to this session (for dropped pushes)
}

func newSession(srv *Server, conn net.Conn) *session {
	return &session{srv: srv, conn: conn}
}

func (c *session) serve() {
	defer func() {
		c.close()
		c.srv.dropSession(c)
	}()
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRequestBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			return // malformed JSON: protocol error, close the connection
		}
		c.handle(req)
	}
	// EOF or read error: abrupt disconnect is fine; cleanup only.
}

func (c *session) handle(req Request) {
	switch req.Cmd {
	case "status":
		c.replyStatus(req.ID, "status", "")
	case "state":
		stateJSON, cs, err := c.srv.loop.DoState()
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		data, err := json.Marshal(StateData{State: stateJSON, LastSeq: cs.LastSeq})
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		c.writeResponse(Response{ID: req.ID, OK: true, Data: data})
	case "pause":
		c.replyStatus(req.ID, "pause", "")
	case "resume":
		c.replyStatus(req.ID, "resume", "")
	case "set_speed":
		var args SetSpeedArgs
		if req.Args != nil {
			if err := json.Unmarshal(req.Args, &args); err != nil {
				c.writeResponse(Response{ID: req.ID, OK: false, Error: "malformed args"})
				return
			}
		}
		if _, err := clock.ParseSpeed(args.Speed); err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		// Uncapped ticking outruns any model by orders of magnitude, so max
		// is reserved for pure-sim worlds (TASK-20).
		if clock.Speed(args.Speed) == clock.SpeedMax && c.srv.llm != nil {
			c.writeResponse(Response{ID: req.ID, OK: false,
				Error: "speed max is reserved for pure-sim worlds; this world has an LLM configured — top speed is 32x (delete llm.json to unlock max)"})
			return
		}
		c.replyStatus(req.ID, "set_speed", clock.Speed(args.Speed))
	case "llm_call":
		if c.srv.llm == nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: "llm orchestrator disabled (no llm.json in the save directory)"})
			return
		}
		var args LLMCallArgs
		if req.Args == nil || json.Unmarshal(req.Args, &args) != nil || args.Prompt == "" {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: "malformed args (need kind, prompt)"})
			return
		}
		// Sessions are per-goroutine; a slow model must not block other
		// commands on this connection — but sessions already handle one
		// request at a time, so a bounded call here is acceptable.
		ctx, cancelCall := context.WithTimeout(context.Background(), 2*time.Minute)
		resp, err := c.srv.llm.Submit(ctx, llm.Request{
			Kind: llm.Kind(args.Kind), System: args.System,
			Prompt: args.Prompt, MaxTokens: args.MaxTokens,
		})
		cancelCall()
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		data, err := json.Marshal(resp)
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		c.writeResponse(Response{ID: req.ID, OK: true, Data: data})
	case "metatron_chat":
		if c.srv.metatron == nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: "metatron is not present in this world (no llm config)"})
			return
		}
		var args MetatronChatArgs
		if req.Args == nil || json.Unmarshal(req.Args, &args) != nil || args.Text == "" {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: "malformed args (need text)"})
			return
		}
		result, err := c.srv.metatron.Turn(context.Background(), args.Text)
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		data, err := json.Marshal(result)
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		c.writeResponse(Response{ID: req.ID, OK: true, Data: data})
	case "metatron_status":
		if c.srv.metatron == nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: "metatron is not present in this world (no llm config)"})
			return
		}
		data, err := json.Marshal(c.srv.metatron.Status())
		if err != nil {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		c.writeResponse(Response{ID: req.ID, OK: true, Data: data})
	case "miracle":
		var args MiracleArgs
		if req.Args == nil || json.Unmarshal(req.Args, &args) != nil || args.Kind == "" {
			c.writeResponse(Response{ID: req.ID, OK: false, Error: "malformed args (need kind)"})
			return
		}
		c.handleMiracle(req.ID, args)
	case "subscribe":
		var args SubscribeArgs
		if req.Args != nil {
			if err := json.Unmarshal(req.Args, &args); err != nil {
				c.writeResponse(Response{ID: req.ID, OK: false, Error: "malformed args"})
				return
			}
		}
		c.subscribe(req.ID, args.Since)
	case "unsubscribe":
		c.unsubscribe()
		c.writeResponse(Response{ID: req.ID, OK: true})
	case "shutdown":
		c.writeResponse(Response{ID: req.ID, OK: true})
		c.srv.shutdown()
	default:
		c.writeResponse(Response{ID: req.ID, OK: false, Error: fmt.Sprintf("unknown cmd %q", req.Cmd)})
	}
}

// handleMiracle lands one operator miracle (spec 016). It needs only the sim
// loop — never srv.llm or srv.metatron — so miracles work on pure-sim worlds:
// the charge bank is sim state, the reducer validates at the InjectSocial
// dry-run, and the shared BuildMiracleBatch composes the batch both doors use.
// The current state is fetched only to resolve the perception-memory recipients
// (which villager stands on a move's source tile); a mismatch by land time is
// rejected whole at the dry-run, never applied in part.
func (c *session) handleMiracle(id int64, args MiracleArgs) {
	stateJSON, _, err := c.srv.loop.DoState()
	if err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	var state sim.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}

	var params metatron.MiracleParams
	var summary string
	switch args.Kind {
	case "move":
		if args.Class == "" {
			c.writeResponse(Response{ID: id, OK: false, Error: "move needs a class (villager|structure|pile)"})
			return
		}
		params = metatron.MiracleParams{Class: args.Class, X: args.X, Y: args.Y, ToX: args.ToX, ToY: args.ToY}
		summary = fmt.Sprintf("moved %s at (%d,%d) to (%d,%d)", args.Class, args.X, args.Y, args.ToX, args.ToY)
	case "remove":
		if args.Class == "" {
			c.writeResponse(Response{ID: id, OK: false, Error: "remove needs a class (structure|pile|terrain)"})
			return
		}
		params = metatron.MiracleParams{Class: args.Class, X: args.X, Y: args.Y}
		summary = fmt.Sprintf("removed %s at (%d,%d)", args.Class, args.X, args.Y)
	case "time_snap":
		// Door-side day/HH:MM → tick (spec 016 US3); the reducer enforces
		// forward-only and the 2-charge price. Works on pure-sim worlds.
		hour, min, perr := clock.ParseTimeOfDay(args.Time)
		if perr != nil {
			c.writeResponse(Response{ID: id, OK: false, Error: perr.Error()})
			return
		}
		params = metatron.MiracleParams{ToTick: clock.TickAt(int64(args.Day), hour, min, 0)}
		summary = fmt.Sprintf("snapped time to day %d %02d:%02d", args.Day, hour, min)
	case "give_item":
		// The shared builder composes it, but the door-side villager name → index
		// wiring lands with US4 — reject cleanly here.
		c.writeResponse(Response{ID: id, OK: false, Error: fmt.Sprintf("miracle kind %q is not yet available", args.Kind)})
		return
	default:
		c.writeResponse(Response{ID: id, OK: false, Error: fmt.Sprintf("unknown miracle kind %q", args.Kind)})
		return
	}

	batch, err := metatron.BuildMiracleBatch(&state, args.Kind, params, args.Gratis)
	if err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	if err := c.srv.loop.InjectSocial(batch); err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	cs, err := c.srv.loop.Do("status", "")
	if err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	data, err := json.Marshal(MiracleData{Kind: args.Kind, Charges: cs.MetatronCharges, Gratis: args.Gratis, Summary: summary})
	if err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	c.writeResponse(Response{ID: id, OK: true, Data: data})
}

func (c *session) replyStatus(id int64, cmd string, speed clock.Speed) {
	cs, err := c.srv.loop.Do(cmd, speed)
	if err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	data, err := json.Marshal(c.srv.statusDataFull(cs))
	if err != nil {
		c.writeResponse(Response{ID: id, OK: false, Error: err.Error()})
		return
	}
	c.writeResponse(Response{ID: id, OK: true, Data: data})
}

// subscribe starts pushes. With since, the log after that seq replays first,
// then the stream goes live with no gap: the live buffer opens before the
// replay reads the store, and the pusher gap-fills from the store whenever
// buffered seqs jump ahead of the cursor.
func (c *session) subscribe(id int64, since *int64) {
	c.subMu.Lock()
	if c.subscribed {
		c.subMu.Unlock()
		c.writeResponse(Response{ID: id, OK: false, Error: "already subscribed"})
		return
	}
	cursor := c.srv.st.LastSeq()
	if since != nil {
		cursor = *since
	}
	c.subscribed = true
	c.subCh = make(chan store.Event, pushBufferSize)
	c.subQuit = make(chan struct{})
	ch, quit := c.subCh, c.subQuit
	c.subMu.Unlock()

	c.writeResponse(Response{ID: id, OK: true})
	go c.push(ch, quit, cursor)
}

func (c *session) unsubscribe() {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.subscribed {
		c.subscribed = false
		close(c.subQuit)
		c.subCh, c.subQuit = nil, nil
	}
}

func (c *session) isSubscribed() bool {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	return c.subscribed
}

// offer hands freshly committed events to this session's subscription
// without ever blocking the caller (the sim loop).
func (c *session) offer(events []store.Event) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if !c.subscribed {
		return
	}
	for _, e := range events {
		c.lastSeq = e.Seq
		select {
		case c.subCh <- e:
		default:
			// Overflow: cancel this subscription; the client re-syncs.
			lastSeq := c.lastSeq
			c.subscribed = false
			close(c.subQuit)
			c.subCh, c.subQuit = nil, nil
			go c.writePush(Push{Push: "dropped", LastSeq: lastSeq})
			return
		}
	}
}

// push delivers events in seq order with no gaps, filling from the store
// whenever the live buffer is ahead of the cursor (e.g. replay-from-since).
func (c *session) push(ch chan store.Event, quit chan struct{}, cursor int64) {
	fill := func(upto int64) bool { // push store events cursor < seq <= upto
		for cursor < upto {
			batch, err := c.srv.st.EventsSince(cursor, 512)
			if err != nil || len(batch) == 0 {
				return false
			}
			for _, e := range batch {
				if e.Seq > upto {
					return true
				}
				if !c.writePush(Push{Push: "event", Event: &e}) {
					return false
				}
				cursor = e.Seq
			}
		}
		return true
	}

	// Initial replay: catch up to the log head as of subscription time.
	if !fill(c.srv.st.LastSeq()) {
		return
	}
	for {
		select {
		case <-quit:
			return
		case e := <-ch:
			if e.Seq <= cursor {
				continue // already delivered during replay
			}
			if e.Seq > cursor+1 && !fill(e.Seq-1) {
				return
			}
			if !c.writePush(Push{Push: "event", Event: &e}) {
				return
			}
			cursor = e.Seq
		}
	}
}

// writeResponse guarantees the wire never carries a line the client cannot
// read: a reply over maxReplyBytes is replaced by an ok:false error (same ID,
// replyTooLargePrefix) that tells the caller what happened and how big the
// payload was — an actionable failure instead of a client-side scanner death.
func (c *session) writeResponse(r Response) bool {
	b, err := json.Marshal(r)
	if err != nil {
		return false
	}
	if len(b)+1 > maxReplyBytes {
		sub := Response{ID: r.ID, OK: false, Error: fmt.Sprintf(
			"%s: reply is %d bytes, over the %d-byte protocol cap — the world state has outgrown what a single attach reply can carry (nightly consolidation shrinks it over time)",
			replyTooLargePrefix, len(b), maxReplyBytes)}
		if b, err = json.Marshal(sub); err != nil {
			return false
		}
	}
	return c.writeLine(b)
}

func (c *session) writePush(p Push) bool {
	b, err := json.Marshal(p)
	if err != nil || len(b)+1 > maxReplyBytes {
		return false // a single event can't realistically hit the cap; drop it
	}
	return c.writeLine(b)
}

func (c *session) writeLine(b []byte) bool {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(append(b, '\n'))
	if err != nil && !errors.Is(err, net.ErrClosed) {
		c.conn.Close() // dead client; reader will unwind
	}
	return err == nil
}

func (c *session) close() {
	c.unsubscribe()
	c.conn.Close()
}
