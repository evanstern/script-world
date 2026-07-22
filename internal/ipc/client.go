package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/evanstern/promptworld/internal/metatron"
)

// ErrReplyTooLarge marks a daemon reply that exceeded the protocol's reply
// cap (maxReplyBytes) — either refused by the server (replyTooLargePrefix
// error) or, on a version-skewed daemon, an over-long line that killed the
// scanner. Reconnecting cannot shrink the payload, so callers must fail fast
// with the message instead of retrying (TASK-19).
var ErrReplyTooLarge = errors.New("daemon reply exceeds the protocol reply cap")

// Client is the attach side of the protocol, used by every CLI subcommand
// (and by the TASK-3 TUI later).
type Client struct {
	conn net.Conn

	writeMu sync.Mutex
	nextID  int64

	mu      sync.Mutex
	pending map[int64]chan Response
	readErr error

	pushes chan Push
	done   chan struct{}
}

// Dial connects to a world's daemon socket. A connection failure means the
// daemon is not running — callers surface that fast, never hang.
func Dial(sockPath string) (*Client, error) {
	conn, err := dialUnix(sockPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("daemon not running (%v)", err)
	}
	c := &Client{
		conn:    conn,
		pending: make(map[int64]chan Response),
		pushes:  make(chan Push, pushBufferSize),
		done:    make(chan struct{}),
	}
	go c.read()
	return c, nil
}

func (c *Client) read() {
	defer func() {
		c.mu.Lock()
		if c.readErr == nil {
			c.readErr = errors.New("connection closed")
		}
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
		close(c.done)
		close(c.pushes)
	}()
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 64*1024), maxReplyBytes)
	for scanner.Scan() {
		var msg wireMsg
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			c.mu.Lock()
			c.readErr = fmt.Errorf("protocol error: %w", err)
			c.mu.Unlock()
			return
		}
		if msg.Push != "" {
			select {
			case c.pushes <- Push{Push: msg.Push, Event: msg.Event, LastSeq: msg.LastSeq}:
			case <-c.done:
				return
			}
			continue
		}
		if msg.ID != nil {
			c.mu.Lock()
			ch := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- Response{ID: *msg.ID, OK: msg.OK, Data: msg.Data, Error: msg.Error}
				close(ch)
			}
		}
	}
	c.mu.Lock()
	if err := scanner.Err(); err != nil && c.readErr == nil {
		if errors.Is(err, bufio.ErrTooLong) {
			// The server caps replies at maxReplyBytes too, so this only
			// happens against a version-skewed daemon — still fatal, still
			// not worth retrying.
			c.readErr = fmt.Errorf("%w: a single reply line exceeded %d bytes (client/daemon version mismatch?)", ErrReplyTooLarge, maxReplyBytes)
		} else {
			c.readErr = err
		}
	}
	c.mu.Unlock()
}

// Call sends a request and waits for its response.
func (c *Client) Call(cmd string, args any) (json.RawMessage, error) {
	var raw json.RawMessage
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		raw = b
	}

	ch := make(chan Response, 1)
	c.writeMu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	b, err := json.Marshal(Request{ID: id, Cmd: cmd, Args: raw})
	if err == nil {
		c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err = c.conn.Write(append(b, '\n'))
	}
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	resp, ok := <-ch
	if !ok {
		c.mu.Lock()
		err := c.readErr
		c.mu.Unlock()
		return nil, err
	}
	if !resp.OK {
		if strings.HasPrefix(resp.Error, replyTooLargePrefix) {
			return nil, fmt.Errorf("%w: %s", ErrReplyTooLarge, resp.Error)
		}
		return nil, errors.New(resp.Error)
	}
	return resp.Data, nil
}

// Status is a convenience for the commands that return StatusData.
func (c *Client) Status(cmd string, args any) (*StatusData, error) {
	data, err := c.Call(cmd, args)
	if err != nil {
		return nil, err
	}
	var sd StatusData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

// FetchState retrieves the full world state and the log position it
// reflects (see StateData).
func (c *Client) FetchState() (*StateData, error) {
	data, err := c.Call("state", nil)
	if err != nil {
		return nil, err
	}
	var sd StateData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

// Subscribe starts the event stream; read from Pushes().
func (c *Client) Subscribe(since *int64) error {
	_, err := c.Call("subscribe", SubscribeArgs{Since: since})
	return err
}

// Pushes delivers server-push messages; closed when the connection dies.
func (c *Client) Pushes() <-chan Push { return c.pushes }

func (c *Client) Close() error { return c.conn.Close() }

// MetatronChat runs one console turn (TASK-12). The call blocks for the
// duration of the angel's cloud round-trip.
func (c *Client) MetatronChat(text string) (*metatron.TurnResult, error) {
	data, err := c.Call("metatron_chat", MetatronChatArgs{Text: text})
	if err != nil {
		return nil, err
	}
	var r metatron.TurnResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// MetatronStatus is the model-free peek: charges, charter provenance, notes.
func (c *Client) MetatronStatus() (*metatron.Status, error) {
	data, err := c.Call("metatron_status", nil)
	if err != nil {
		return nil, err
	}
	var s metatron.Status
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
