# Contract: conversation prompts

All calls `llm.KindConversation` (local tier). One conversation at a time.

## Utterance call (per turn, MaxTokens 128)

System (stable per speaker):
```text
You are <Name>, a villager. <persona.md>
You are talking with <Other>. Your feelings toward them: <warm/cool/wary…> (trust T, affection A).
Reply with ONLY {"say": "<one or two short sentences in your voice>"}
```

User: recent memory window (≤5 lines) + transcript so far + "Your turn."

## Outcome call (once, MaxTokens 192)

```text
Summarize this exchange between <A> and <B>: <transcript>
Reply with ONLY:
{"gist": "<one sentence>", "tone_a": -2..2, "tone_b": -2..2, "retold": "<if A passed on the note below, how A phrased it, else null>"}
Note A may pass on: "<tellable text or (none)>"
```

## Failure semantics

Any parse/transport failure at any step → the whole conversation is abandoned and
NOTHING is injected. The primitive `agent.talked` (already in the log) stands alone.
Caps: 3 utterances per side; gist ≤ 200 chars (truncated); tones clamped −2..2.
