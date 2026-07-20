// Package ipc implements the client attach/detach protocol from
// specs/001-world-daemon/contracts/client-protocol.md: JSON-lines over a
// Unix domain socket in the world's save directory.
package ipc

import (
	"encoding/json"

	"github.com/evanstern/script-world/internal/llm"
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

// LLMCallArgs / LLMCallData carry the "llm_call" command (routing proof and
// the transport TASK-7 minds will use).
type LLMCallArgs struct {
	Kind      string `json:"kind"`
	System    string `json:"system,omitempty"`
	Prompt    string `json:"prompt"`
	MaxTokens int64  `json:"max_tokens,omitempty"`
}

// MetatronChatArgs carries one console turn (TASK-12); the response Data is
// a metatron.TurnResult. "metatron_status" takes no args and answers a
// metatron.Status (the model-free peek).
type MetatronChatArgs struct {
	Text string `json:"text"`
}

// StateData answers the "state" command: the full canonical sim.State JSON
// plus the log position it reflects — subscribe with since=last_seq for a
// gapless live replica.
type StateData struct {
	State   json.RawMessage `json:"state"`
	LastSeq int64           `json:"last_seq"`
}

// StatusData is the shared response shape for status/pause/resume/set_speed.
type StatusData struct {
	World  WorldStatus  `json:"world"`
	Clock  ClockStatus  `json:"clock"`
	Daemon DaemonStatus `json:"daemon"`
	Log    LogStatus    `json:"log"`
	// LLM is present only when the world has an orchestrator (llm.json).
	LLM *llm.Status `json:"llm,omitempty"`
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
	// MetatronCharges is the nudge bank (TASK-12) — surfaced here so
	// clients render ⚡ without a state fetch.
	MetatronCharges int `json:"metatron_charges"`
}

type DaemonStatus struct {
	Pid           int   `json:"pid"`
	UptimeSeconds int64 `json:"uptime_seconds"`
	Subscribers   int   `json:"subscribers"`
}

type LogStatus struct {
	LastSeq int64 `json:"last_seq"`
}
