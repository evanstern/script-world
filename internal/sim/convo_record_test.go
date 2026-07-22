package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// TestConversationRecordRing (TASK-22): social.conversation now leaves a
// durable, bounded record; pre-TASK-22 pair payloads still apply.
func TestConversationRecordRing(t *testing.T) {
	m := testMap(11)
	s := NewState(11, m)

	// New scene shape.
	s.Apply(store.Event{Tick: 100, Type: "social.conversation", Payload: mustPayload(ConversationPayload{
		Conv: 100, A: 0, B: 1, Gist: "argued about firewood", Turns: 6,
		Participants: []int{0, 1, 2}, Topics: []string{"firewood"}, Tones: []int{1, -1, 0},
	})})
	// Legacy pair shape (no participants).
	s.Apply(store.Event{Tick: 200, Type: "social.conversation", Payload: mustPayload(ConversationPayload{
		Conv: 200, A: 3, B: 4, Gist: "shared a joke", Turns: 4,
	})})

	if len(s.Conversations) != 2 {
		t.Fatalf("records = %d, want 2", len(s.Conversations))
	}
	r := s.Conversations[0]
	if len(r.Participants) != 3 || r.Topics[0] != "firewood" || r.Tones[1] != -1 {
		t.Errorf("scene record wrong: %+v", r)
	}
	if got := s.Conversations[1].Participants; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("legacy payload must fall back to [A,B]: %v", got)
	}

	if rec, ok := LastConversationBetween(s, 0, 2); !ok || rec.Conv != 100 {
		t.Errorf("LastConversationBetween(0,2): %v %v", rec, ok)
	}
	if _, ok := LastConversationBetween(s, 0, 4); ok {
		t.Error("0 and 4 never talked")
	}
	if rec, ok := LastConversationInvolving(s, 4); !ok || rec.Conv != 200 {
		t.Errorf("LastConversationInvolving(4): %v %v", rec, ok)
	}

	// The ring is bounded: oldest records fall off at the cap.
	for i := 0; i < convoRecordCap+10; i++ {
		s.Apply(store.Event{Tick: int64(300 + i), Type: "social.conversation", Payload: mustPayload(ConversationPayload{
			Conv: int64(300 + i), A: 0, B: 1, Gist: "chatter", Turns: 4,
		})})
	}
	if len(s.Conversations) != convoRecordCap {
		t.Errorf("ring size = %d, want cap %d", len(s.Conversations), convoRecordCap)
	}
	if s.Conversations[0].Conv <= 200 {
		t.Error("oldest records should have fallen off the ring")
	}
}
