package mind

import "testing"

// TestParseOutcomeLenient (TASK-42 T014): the observed unquoted-value shapes
// (gist starting with a participant initial F/H/S; a bare retold value)
// recover without a model call, while genuinely broken replies still fail.
func TestParseOutcomeLenient(t *testing.T) {
	recoverable := []struct {
		name string
		in   string
		gist string
	}{
		{"unquoted gist F", `{"gist": Fenwick and Rowan talked about the fire, "topics": ["fire"], "tones": [1, 1], "retold": null}`, "Fenwick and Rowan talked about the fire"},
		{"unquoted gist H", `{"gist": Hazel and Rowan talked about the fire, "topics": ["fire"], "tones": [1, 1], "retold": null}`, "Hazel and Rowan talked about the fire"},
		{"unquoted gist S", `{"gist": Sorrel and Rowan talked about the fire, "topics": ["fire"], "tones": [1, 1], "retold": null}`, "Sorrel and Rowan talked about the fire"},
		{"unquoted retold", `{"gist": "planned the firewood run", "topics": ["fire"], "tones": [1, 1], "retold": she said the fire is cursed}`, "planned the firewood run"},
	}
	for _, tc := range recoverable {
		o, err := parseOutcome(tc.in)
		if err != nil {
			t.Errorf("%s: expected lenient recovery, got %v", tc.name, err)
			continue
		}
		if o.Gist != tc.gist {
			t.Errorf("%s: gist = %q, want %q", tc.name, o.Gist, tc.gist)
		}
		if len(o.Tones) != 2 {
			t.Errorf("%s: tones = %v, want 2", tc.name, o.Tones)
		}
	}

	// The last case's retold must be recovered as its prose, not dropped.
	if o, _ := parseOutcome(recoverable[3].in); o.Retold != "she said the fire is cursed" {
		t.Errorf("unquoted retold = %q, want the recovered prose", o.Retold)
	}

	unrecoverable := []struct {
		name string
		in   string
	}{
		{"prose only", `the model just rambled in prose about the weather`},
		{"unterminated object", `{"gist": "planned the firewood`},
		{"valid json empty gist", `{"gist": "", "topics": ["fire"], "tones": [1, 1]}`},
		{"no gist key", `{"topics": ["fire"], "tones": [1, 1], "retold": null}`},
	}
	for _, tc := range unrecoverable {
		if _, err := parseOutcome(tc.in); err == nil {
			t.Errorf("%s: expected failure, got nil", tc.name)
		}
	}
}

// TestParseOutcomeHappyPathUnchanged: a well-formed reply parses untouched —
// the lenient path is never entered when json.Unmarshal already succeeds.
func TestParseOutcomeHappyPathUnchanged(t *testing.T) {
	o, err := parseOutcome(`{"gist": "planned firewood", "topics": ["fire", "chores"], "tones": [2, -1], "retold": null}`)
	if err != nil {
		t.Fatal(err)
	}
	if o.Gist != "planned firewood" || len(o.Topics) != 2 || len(o.Tones) != 2 || o.Tones[1] != -1 {
		t.Errorf("parsed: %+v", o)
	}
}
