package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

// TestDeterminism_FullBinary is quickstart Scenario D at the binary level:
// two worlds with the same seed and an identical (empty) command timeline
// must produce byte-identical sim histories. The unit harness
// (internal/sim) covers non-empty command timelines tick-exactly; here the
// two daemons stop at different wall-clock ticks, so histories compare over
// the common tick prefix, excluding wall-dependent bookkeeping types
// (daemon.*, clock.*) whose ticks depend on when commands/stops landed.
func TestDeterminism_FullBinary(t *testing.T) {
	histories := make([][]string, 2)
	lastTicks := make([]int64, 2)

	for i := range histories {
		dir := filepath.Join(t.TempDir(), "w")
		run(t, "new", dir, "--seed", "777")
		os.Remove(filepath.Join(dir, "llm.json")) // pure-sim: max needs no LLM (TASK-20)
		run(t, "start", dir)
		run(t, "speed", dir, "max")
		// Past tick 24500: the day-1 governance cycle (convene 19800, open
		// 21600, turns, close) is inside the compared window (TASK-13).
		waitTick(t, dir, 25000)
		run(t, "stop", dir)

		st, err := store.Open(filepath.Join(dir, "world.db"))
		if err != nil {
			t.Fatal(err)
		}
		events, err := st.EventsSince(0, 0)
		st.Close()
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		var lastTick int64
		for _, e := range events {
			if strings.HasPrefix(e.Type, "daemon.") || strings.HasPrefix(e.Type, "clock.") {
				continue
			}
			lines = append(lines, fmt.Sprintf("%d %s %s", e.Tick, e.Type, string(e.Payload)))
			lastTick = e.Tick
		}
		histories[i] = lines
		lastTicks[i] = lastTick
	}

	minTick := min(lastTicks[0], lastTicks[1])
	trim := func(lines []string) []string {
		var out []string
		for _, l := range lines {
			var tick int64
			fmt.Sscanf(l, "%d", &tick)
			if tick <= minTick {
				out = append(out, l)
			}
		}
		return out
	}
	a, b := trim(histories[0]), trim(histories[1])
	if len(a) == 0 {
		t.Fatal("no sim events to compare")
	}
	if len(a) != len(b) {
		t.Fatalf("histories diverge in length: %d vs %d events up to tick %d", len(a), len(b), minTick)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("histories diverge at index %d:\nA: %s\nB: %s", i, a[i], b[i])
		}
	}
}
