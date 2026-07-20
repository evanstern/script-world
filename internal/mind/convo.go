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

// The conversation driver (TASK-8, scenes + fodder in TASK-22): when two
// villagers talk (the executor's deterministic adjacency beat), the mind may
// escalate the moment into a short model-driven dialogue — and nearby awake
// villagers join the scene (2..sceneCap participants). Everything runs in
// its own goroutine against an immutable snapshot; effects land as ONE
// atomic inject_social batch, or not at all — the primitive talk already
// happened either way.

// SocialInjector is the loop surface for conversation outcomes (test seam).
type SocialInjector interface {
	InjectSocial(events []store.Event) error
}

// sceneCap bounds a scene: each extra participant adds ConvoTurnsPerSide
// utterance calls, and the local tier pays for every one.
const sceneCap = 4

// sceneJoinRadius is how close (Manhattan, to the founding speaker) an
// awake villager must be to join the scene.
const sceneJoinRadius = 2

type convoCtx struct {
	conv     int64 // founding talk's tick = conversation id
	meta     thoughtMeta
	idx      []int
	names    []string
	personas []string
	rels     []string   // per participant: feelings toward each other participant, rendered
	memories [][]string // formatted windows, ≤5 lines each
	callback string     // last-conversation gist between the founding pair, if any
	debts    string     // open debts among participants, rendered
	tell     *sim.Tellable
	teller   int // position in idx (founding pair), valid when tell != nil
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
	cc := md.snapshotConvo(e.Tick, p.A, p.B)
	// The scene is one 13-point decision (contracts/registry.md): its
	// telemetry identity is minted at founding, agent = founding speaker.
	cc.meta = md.newMeta("conversation", p.A, e.Tick, e.Seq, llm.KindConversation)
	cc.meta.job = fmt.Sprintf("conversation-%d", cc.conv)
	log.Printf("mind: conversation %d starting between %s", cc.conv, strings.Join(cc.names, ", "))
	go func() {
		defer md.convoBusy.Store(false)
		md.emitCog(cogThoughtEvent(cc.meta))
		md.runConversation(cc)
	}()
}

// snapshotConvo freezes the scene: the founding pair, any awake villager
// within sceneJoinRadius of the founding speaker (TASK-22), and everything
// the prompts and the outcome batch will need.
func (md *Mind) snapshotConvo(tick int64, a, b int) convoCtx {
	s := md.replica
	cc := convoCtx{conv: tick, idx: []int{a, b}}
	for j := range s.Agents {
		if len(cc.idx) >= sceneCap {
			break
		}
		if j == a || j == b {
			continue
		}
		ag := &s.Agents[j]
		if ag.Dead || ag.Asleep {
			continue
		}
		if absInt(ag.X-s.Agents[a].X)+absInt(ag.Y-s.Agents[a].Y) <= sceneJoinRadius {
			cc.idx = append(cc.idx, j)
		}
	}

	for _, id := range cc.idx {
		ag := s.Agents[id]
		cc.names = append(cc.names, ag.Name)
		cc.personas = append(cc.personas, md.personas[id])
		var feelings []string
		for _, other := range cc.idx {
			if other == id {
				continue
			}
			rel := s.RelationBetween(id, other)
			feelings = append(feelings, fmt.Sprintf("%s: trust %d, affection %d",
				s.Agents[other].Name, rel.Trust/10, rel.Affection/10))
		}
		cc.rels = append(cc.rels, strings.Join(feelings, "; "))
		var mem []string
		for _, m := range sim.SelectMemories(&ag, s.Seed, id, tick, 5) {
			mem = append(mem, sim.FormatMemory(m))
		}
		cc.memories = append(cc.memories, mem)
	}

	// Relationship fodder in (TASK-22): the last conversation between the
	// founding pair, and any open debts inside the scene.
	if r, ok := sim.LastConversationBetween(s, a, b); ok {
		cc.callback = r.Gist
	}
	var debts []string
	for _, d := range s.Debts {
		if d.Status != "open" {
			continue
		}
		var deb, cred bool
		for _, id := range cc.idx {
			deb = deb || d.Debtor == id
			cred = cred || d.Creditor == id
		}
		if deb && cred {
			debts = append(debts, fmt.Sprintf("%s owes %s %s",
				s.Agents[d.Debtor].Name, s.Agents[d.Creditor].Name, d.Kind))
		}
	}
	cc.debts = strings.Join(debts, "; ")

	// One rumor may pass between the founding pair: prefer a shareable
	// secret behind the trust gate, else the best ordinary tellable.
	pair := [2]int{a, b}
	for i, id := range pair {
		other := pair[1-i]
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
		for i, id := range pair {
			if t, ok := sim.TellableFor(s, id, pair[1-i]); ok && t.Confidence > best.Confidence {
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
// must release the slot rather than hold it forever. Sized for a full
// scene at honest local pace: up to sceneCap×ConvoTurnsPerSide utterances
// plus the outcome call at ~40–60s each.
const convoDeadline = 10 * time.Minute

func (md *Mind) runConversation(cc convoCtx) {
	sceneStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), convoDeadline)
	defer cancel()
	transcript := []string{}
	turns := 0
	n := len(cc.idx)
	for t := 0; t < n*sim.ConvoTurnsPerSide; t++ {
		sp := t % n // speaker position, round-robin
		say, err := md.utterance(ctx, cc, sp, transcript)
		if err != nil {
			log.Printf("mind: conversation %d abandoned at turn %d: %v", cc.conv, t, err)
			md.emitCog(md.cogOutcomeEvent(cc.meta, sim.OutcomeUnusable,
				fmt.Sprintf("abandoned at turn %d: %v", t, err), time.Since(sceneStart).Milliseconds()))
			return // all-or-nothing: inject nothing
		}
		transcript = append(transcript, fmt.Sprintf("%s: %s", cc.names[sp], say))
		turns++
	}
	out, err := md.outcome(ctx, cc, transcript)
	if err != nil {
		log.Printf("mind: conversation %d outcome failed: %v", cc.conv, err)
		md.emitCog(md.cogOutcomeEvent(cc.meta, sim.OutcomeUnusable,
			"outcome: "+err.Error(), time.Since(sceneStart).Milliseconds()))
		return
	}
	// Tones arrive per participant; missing tail entries read neutral.
	tones := make([]int, n)
	copy(tones, out.Tones)

	var batch []store.Event
	add := func(typ string, payload any) {
		b, _ := json.Marshal(payload)
		batch = append(batch, store.Event{Type: typ, Payload: b})
	}
	for t, line := range transcript {
		sp := t % n
		listener := -1 // the scene; pairs keep the explicit listener
		if n == 2 {
			listener = cc.idx[1-sp]
		}
		add("social.conversation_turn", sim.ConversationTurnPayload{
			Conv: cc.conv, Speaker: cc.idx[sp], Listener: listener,
			Text: strings.TrimPrefix(line, cc.names[sp]+": "),
		})
	}
	add("social.conversation", sim.ConversationPayload{
		Conv: cc.conv, A: cc.idx[0], B: cc.idx[1], Gist: out.Gist, Turns: turns,
		Participants: cc.idx, Topics: out.Topics, Tones: tones,
	})
	reason := "conversation"
	if len(out.Topics) > 0 {
		reason = "conversation: " + out.Topics[0]
	}
	for i := range cc.idx {
		// Fodder per counterpart (TASK-22): the gist memory is ABOUT the
		// other (subject-tagged, toned) — a gossip seed TellableFor can
		// serve — and each participant's experienced tone colors every
		// edge they hold in the scene.
		others := ""
		if n > 2 {
			others = " and others"
		}
		for j := range cc.idx {
			if j == i {
				continue
			}
			add("agent.memory_added", sim.MemoryAddedPayload{
				Agent: cc.idx[i], Text: fmt.Sprintf("Talked with %s%s — %s", cc.names[j], others, out.Gist),
				Salience: sim.SalConvoGist, Subject: cc.idx[j], Tone: tones[i] * convoMemoryToneScale,
			})
			add("social.relation_changed", sim.RelationChangedPayload{
				A: cc.idx[i], B: cc.idx[j],
				TrustDelta:     tones[i] * sim.ConvoToneTrust,
				AffectionDelta: tones[i] * sim.ConvoToneAffect,
				Reason:         reason,
			})
		}
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
	// The scene and its terminal record land atomically.
	batch = append(batch, md.cogOutcomeEvent(cc.meta, sim.OutcomeLanded, "",
		time.Since(sceneStart).Milliseconds()))
	if err := md.social.InjectSocial(batch); err != nil {
		log.Printf("mind: conversation %d injection rejected: %v", cc.conv, err)
		md.emitCog(md.cogOutcomeEvent(cc.meta, sim.OutcomeUnusable,
			"injection rejected: "+err.Error(), time.Since(sceneStart).Milliseconds()))
	} else {
		log.Printf("mind: conversation %d landed (%d turns, %d participants)", cc.conv, turns, n)
	}
}

// convoMemoryToneScale maps outcome tones (-2..2) onto memory tone valence
// (-60..60), the same scale executor witness memories use.
const convoMemoryToneScale = 30

func (md *Mind) utterance(ctx context.Context, cc convoCtx, sp int, transcript []string) (string, error) {
	var user strings.Builder
	if len(cc.memories[sp]) > 0 {
		user.WriteString("You remember:\n")
		for _, m := range cc.memories[sp] {
			fmt.Fprintf(&user, "- %s\n", m)
		}
	}
	if cc.callback != "" {
		fmt.Fprintf(&user, "\nLast time this pair talked: %s\n", cc.callback)
	}
	if cc.debts != "" {
		fmt.Fprintf(&user, "Standing debts here: %s.\n", cc.debts)
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
	var others []string
	for i, n := range cc.names {
		if i != sp {
			others = append(others, n)
		}
	}
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind: llm.KindConversation,
		System: fmt.Sprintf(`You are %s, a villager. %s
You are talking with %s. Your feelings — %s.
Reply with ONLY {"say": "<one or two short sentences in your voice>"}`,
			cc.names[sp], cc.personas[sp], strings.Join(others, " and "), cc.rels[sp]),
		Prompt:    user.String(),
		MaxTokens: 128,
	})
	if err != nil {
		return "", err
	}
	return parseSay(resp.Text)
}

type convoOutcome struct {
	Gist   string   `json:"gist"`
	Topics []string `json:"topics"`
	Tones  []int    `json:"-"`
	Retold string   `json:"retold"`
	// Models emit tones as ints or floats interchangeably; accept both.
	// tone_a/tone_b are the pre-TASK-22 pair shape, still accepted.
	RawTones []float64 `json:"tones"`
	RawToneA float64   `json:"tone_a"`
	RawToneB float64   `json:"tone_b"`
}

func (md *Mind) outcome(ctx context.Context, cc convoCtx, transcript []string) (convoOutcome, error) {
	note := "(none)"
	if cc.tell != nil {
		note = cc.tell.Text
	}
	teller := cc.names[cc.tellerName()]
	prompt := fmt.Sprintf(`Summarize this exchange between %s:
%s

Reply with ONLY:
{"gist": "<one sentence>", "topics": ["<1-3 short topic tags>"], "tones": [%s — one -2..2 integer per person, in that order], "retold": "<if %s passed on the note below, how they phrased it, else null>"}
Note %s may pass on: %q`,
		strings.Join(cc.names, ", "), strings.Join(transcript, "\n"),
		strings.Join(cc.names, ", "), teller, teller, note)
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind: llm.KindConversation, Prompt: prompt, MaxTokens: 224,
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
