package llm

import (
	"sync"
	"time"
)

// tierHealth is a circuit breaker, one per provider (spec 024): consecutive
// failures open it (fail fast), a backoff window closes into half-open (one probe
// request allowed), and a success resets. This is the "designed degraded state" —
// an unreachable model can never hang or crash the daemon, and recovery is
// automatic. Vars (not consts) so tests can compress the clock.
var (
	failuresToOpen = 3
	backoffInitial = 15 * time.Second
	backoffMax     = 5 * time.Minute
)

type tierHealth struct {
	mu        sync.Mutex
	fails     int
	openUntil time.Time
	backoff   time.Duration
	probing   bool // half-open: one request in flight testing recovery
}

// admit reports whether a request may proceed. While open, everything is
// refused until the backoff elapses; then exactly one probe goes through.
func (h *tierHealth) admit() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fails < failuresToOpen {
		return true
	}
	if time.Now().Before(h.openUntil) {
		return false
	}
	if h.probing {
		return false
	}
	h.probing = true
	return true
}

func (h *tierHealth) succeed() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fails = 0
	h.backoff = 0
	h.probing = false
}

func (h *tierHealth) fail() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fails++
	h.probing = false
	if h.fails >= failuresToOpen {
		if h.backoff == 0 {
			h.backoff = backoffInitial
		} else if h.backoff < backoffMax {
			h.backoff *= 2
			if h.backoff > backoffMax {
				h.backoff = backoffMax
			}
		}
		h.openUntil = time.Now().Add(h.backoff)
	}
}

func (h *tierHealth) down() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.fails >= failuresToOpen
}
