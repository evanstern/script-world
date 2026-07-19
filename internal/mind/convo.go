package mind

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// The conversation driver (TASK-8): when two villagers talk (the executor's
// deterministic adjacency beat), the mind may escalate the moment into a
// short model-driven dialogue. Everything runs in its own goroutine against
// an immutable snapshot; effects land as ONE atomic inject_social batch, or
// not at all — the primitive talk already happened either way.

// SocialInjector is the loop surface for conversation outcomes (test seam).
type SocialInjector interface {
	InjectSocial(events []store.Event) error
}

type convoCtx struct {
	conv     int64 // founding talk's tick = conversation id
	idx      [2]int
	names    [2]string
	personas [2]string
	rels     [2]string   // how each feels about the other, rendered
	memories [2][]string // formatted windows, ≤5 lines
	tell     *sim.Tellable
	teller   int // 0 or 1 (position in idx), valid when tell != nil
	secret   bool
}

// maybeStartConversation is called from absorb on agent.talked. Snapshot
// everything needed, then run detached; slot=1 keeps local-tier load sane.
func (md *Mind) maybeStartConversation(e store.Event) {
	if md.social == nil {
		return
	}
	var p sim.TalkedPayload
	if json.Unmarshal(e.Payload, &p) != nil {
		return
	}
	if !md.convoBusy.CompareAndSwap(false, true) {
		return // one at a time; this encounter stays a primitive talk
	}
	ctx := md.snapshotConvo(e.Tick, p.A, p.B)
	log.Printf("mind: conversation %d starting between %s and %s", e.Tick, ctx.names[0], ctx.names[1])
	go func() {
		defer md.convoBusy.Store(false)
		md.runConversation(ctx)
	}()
}

func (md *Mind) snapshotConvo(tick int64, a, b int) convoCtx {
	s := md.replica
	cc := convoCtx{conv: tick, idx: [2]int{a, b}}
	for i, id := range cc.idx {
		ag := s.Agents[id]
		cc.names[i] = ag.Name
		cc.personas[i] = md.personas[id]
		other := cc.idx[1-i]
		rel := s.RelationBetween(id, other)
		cc.rels[i] = fmt.Sprintf("trust %d, affection %d", rel.Trust/10, rel.Affection/10)
		for _, m := range sim.SelectMemories(&ag, s.Seed, id, tick, 5) {
			cc.memories[i] = append(cc.memories[i], sim.FormatMemory(m))
		}
	}
	// One rumor may pass: prefer a shareable secret behind the trust gate,
	// else the best ordinary tellable (either direction, teller = whoever
	// has the better one).
	for i, id := range cc.idx {
		other := cc.idx[1-i]
		if k, r, ok := sim.SecretOf(s, id); ok &&
			s.RelationBetween(id, other).Trust >= sim.SecretTrustGate &&
			sim.SecretShareRoll(s.Seed, tick, id) {
			t := sim.Tellable{RumorID: r.ID, Subject: r.Subject, Tone: r.Tone, Text: k.Text, Confidence: 80}
			cc.tell, cc.teller, cc.secret = &t, i, true
			break
		}
	}
	if cc.tell == nil {
		best := sim.Tellable{Confidence: -1}
		teller := -1
		for i, id := range cc.idx {
			if t, ok := sim.TellableFor(s, id, cc.idx[1-i]); ok && t.Confidence > best.Confidence {
				best, teller = t, i
			}
		}
		if teller >= 0 {
			cc.tell, cc.teller = &best, teller
		}
	}
	return cc
}

// convoDeadline bounds a whole conversation's wall time: on a busy local
// tier the calls queue behind planner traffic, and a starved conversation
// must release the slot rather than hold it for minutes.
const convoDeadline = 6 * time.Minute

func (md *Mind) runConversation(cc convoCtx) {
	ctx, cancel := context.WithTimeout(context.Background(), convoDeadline)
	defer cancel()
	transcript := []string{}
	turns := 0
	for t := 0; t < 2*sim.ConvoTurnsPerSide; t++ {
		sp := t % 2 // speaker position
		say, err := md.utterance(ctx, cc, sp, transcript)
		if err != nil {
			log.Printf("mind: conversation %d abandoned at turn %d: %v", cc.conv, t, err)
			return // all-or-nothing: inject nothing
		}
		transcript = append(transcript, fmt.Sprintf("%s: %s", cc.names[sp], say))
		turns++
	}
	out, err := md.outcome(ctx, cc, transcript)
	if err != nil {
		log.Printf("mind: conversation %d outcome failed: %v", cc.conv, err)
		return
	}

	var batch []store.Event
	add := func(typ string, payload any) {
		b, _ := json.Marshal(payload)
		batch = append(batch, store.Event{Type: typ, Payload: b})
	}
	for t, line := range transcript {
		sp := t % 2
		add("social.conversation_turn", sim.ConversationTurnPayload{
			Conv: cc.conv, Speaker: cc.idx[sp], Listener: cc.idx[1-sp],
			Text: strings.TrimPrefix(line, cc.names[sp]+": "),
		})
	}
	add("social.conversation", sim.ConversationPayload{
		Conv: cc.conv, A: cc.idx[0], B: cc.idx[1], Gist: out.Gist, Turns: turns,
	})
	tones := [2]int{out.ToneA, out.ToneB}
	for i := range cc.idx {
		add("agent.memory_added", sim.MemoryAddedPayload{
			Agent: cc.idx[i], Text: fmt.Sprintf("Talked with %s — %s", cc.names[1-i], out.Gist),
			Salience: sim.SalConvoGist, Subject: -1,
		})
		add("social.relation_changed", sim.RelationChangedPayload{
			A: cc.idx[i], B: cc.idx[1-i],
			TrustDelta:     tones[i] * sim.ConvoToneTrust,
			AffectionDelta: tones[i] * sim.ConvoToneAffect,
			Reason:         "conversation",
		})
	}
	if cc.tell != nil {
		text := cc.tell.Text
		if out.Retold != "" {
			text = out.Retold // the cheap paraphrase — mutation on retell
		}
		add("social.rumor_told", sim.RumorToldPayload{
			From: cc.idx[cc.teller], To: cc.idx[1-cc.teller],
			RumorID: cc.tell.RumorID, Subject: cc.tell.Subject, Tone: cc.tell.Tone,
			Text: text, Confidence: cc.tell.Confidence, Secret: cc.secret,
		})
	}
	if err := md.social.InjectSocial(batch); err != nil {
		log.Printf("mind: conversation %d injection rejected: %v", cc.conv, err)
	} else {
		log.Printf("mind: conversation %d landed (%d turns)", cc.conv, turns)
	}
}

func (md *Mind) utterance(ctx context.Context, cc convoCtx, sp int, transcript []string) (string, error) {
	var user strings.Builder
	if len(cc.memories[sp]) > 0 {
		user.WriteString("You remember:\n")
		for _, m := range cc.memories[sp] {
			fmt.Fprintf(&user, "- %s\n", m)
		}
	}
	if len(transcript) == 0 {
		user.WriteString("\nYou speak first.")
	} else {
		user.WriteString("\nThe conversation so far:\n")
		for _, line := range transcript {
			user.WriteString(line + "\n")
		}
		user.WriteString("\nYour turn.")
	}
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind: llm.KindConversation,
		System: fmt.Sprintf(`You are %s, a villager. %s
You are talking with %s. Your feelings toward them: %s.
Reply with ONLY {"say": "<one or two short sentences in your voice>"}`,
			cc.names[sp], cc.personas[sp], cc.names[1-sp], cc.rels[sp]),
		Prompt:    user.String(),
		MaxTokens: 128,
	})
	if err != nil {
		return "", err
	}
	return parseSay(resp.Text)
}

type convoOutcome struct {
	Gist   string `json:"gist"`
	ToneA  int    `json:"-"`
	ToneB  int    `json:"-"`
	Retold string `json:"retold"`
	// Models emit tones as ints or floats interchangeably; accept both.
	RawToneA float64 `json:"tone_a"`
	RawToneB float64 `json:"tone_b"`
}

func (md *Mind) outcome(ctx context.Context, cc convoCtx, transcript []string) (convoOutcome, error) {
	note := "(none)"
	if cc.tell != nil {
		note = cc.tell.Text
	}
	prompt := fmt.Sprintf(`Summarize this exchange between %s and %s:
%s

Reply with ONLY:
{"gist": "<one sentence>", "tone_a": -2..2, "tone_b": -2..2, "retold": "<if %s passed on the note below, how they phrased it, else null>"}
Note %s may pass on: %q`,
		cc.names[0], cc.names[1], strings.Join(transcript, "\n"),
		cc.names[cc.tellerName()], cc.names[cc.tellerName()], note)
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind: llm.KindConversation, Prompt: prompt, MaxTokens: 192,
	})
	if err != nil {
		return convoOutcome{}, err
	}
	return parseOutcome(resp.Text)
}

func (cc convoCtx) tellerName() int {
	if cc.tell != nil {
		return cc.teller
	}
	return 0
}
