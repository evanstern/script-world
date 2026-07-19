// Package ipc implements the client attach/detach protocol from
// specs/001-world-daemon/contracts/client-protocol.md: JSON-lines over a
// Unix domain socket in the world's save directory.
package ipc

import (
	"encoding/json"

	"github.com/evanstern/script-world/internal/store"
)

type Request struct {
	ID   int64           `json:"id"`
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

type Response struct {
	ID    int64           `json:"id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Push is a server-initiated message to a subscribed client.
// Push == "event" carries Event; Push == "dropped" carries LastSeq (the
// subscription was canceled on buffer overflow; re-subscribe with since).
type Push struct {
	Push    string       `json:"push"`
	Event   *store.Event `json:"event,omitempty"`
	LastSeq int64        `json:"last_seq,omitempty"`
}

// wireMsg is the union read by clients: a Response has an ID, a Push has Push.
type wireMsg struct {
	ID      *int64          `json:"id,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Push    string          `json:"push,omitempty"`
	Event   *store.Event    `json:"event,omitempty"`
	LastSeq int64           `json:"last_seq,omitempty"`
}

type SubscribeArgs struct {
	Since *int64 `json:"since,omitempty"`
}

type SetSpeedArgs struct {
	Speed string `json:"speed"`
}

// StatusData is the shared response shape for status/pause/resume/set_speed.
type StatusData struct {
	World  WorldStatus  `json:"world"`
	Clock  ClockStatus  `json:"clock"`
	Daemon DaemonStatus `json:"daemon"`
	Log    LogStatus    `json:"log"`
}

type WorldStatus struct {
	Name          string `json:"name"`
	Seed          uint64 `json:"seed"`
	FormatVersion int    `json:"format_version"`
}

type ClockStatus struct {
	Tick          int64   `json:"tick"`
	GameTime      string  `json:"game_time"`
	Paused        bool    `json:"paused"`
	Speed         string  `json:"speed"`
	EffectiveRate float64 `json:"effective_rate"`
	Degraded      bool    `json:"degraded"`
}

type DaemonStatus struct {
	Pid           int   `json:"pid"`
	UptimeSeconds int64 `json:"uptime_seconds"`
	Subscribers   int   `json:"subscribers"`
}

type LogStatus struct {
	LastSeq int64 `json:"last_seq"`
}
