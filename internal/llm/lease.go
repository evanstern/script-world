package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Advisory endpoint leases (spec 024 US5, contracts/endpoint-lease.md): the
// TASK-24 closure. Two worlds that point at one shared model endpoint (e.g. two
// promptworld daemons driving one Ollama) would otherwise each run their full
// worker fan-out against it, thrashing the server into timeouts that read as
// breaker-opening failures. A lease pool bounds the COMBINED in-flight calls to
// that endpoint at a declared capacity, coordinating through the OS with no
// daemon-to-daemon protocol (research R1).
//
// The pool is a directory of `capacity` slot files under
// ~/.promptworld/endpoint-leases/<hash>/; acquiring = flock(LOCK_EX|LOCK_NB) on
// the first free slot, releasing = closing the fd. flock is advisory (only
// worlds that DECLARE endpoint_capacity participate — anything else is invisible
// to the pool) and crash-reclaimable (the kernel drops a dead process's locks,
// so survivors acquire freed slots with no operator action, SC of US5). Same-
// process pools contend correctly too: flock is per open-file-description, so
// each acquire opens its own fd and two providers in one world sharing an
// endpoint still share the bound.

// Lease tuning. leaseRetryBase is the sweep interval when no slot is free
// (jittered to de-sync competing sweepers); leaseContendedThreshold is the wait
// above which a provider is flagged contended. Both are vars, not consts, so
// tests can compress the clock (mirroring health.go's breaker knobs).
var (
	leaseRetryBase          = 100 * time.Millisecond
	leaseContendedThreshold = 2 * time.Second
)

// leaseBaseDir resolves the root directory that holds every endpoint-lease pool
// (~/.promptworld/endpoint-leases). ok is false when the user home directory
// cannot be resolved — leases then disable for the run (warn-not-error, matching
// the knob doctrine) rather than failing the boot. Overridable in tests, which
// point it at a temp dir shared across orchestrators to reproduce cross-world
// contention in one process.
var leaseBaseDir = func() (dir string, ok bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".promptworld", "endpoint-leases"), true
}

// leaseWarnf surfaces a lease boot warning (warn-not-error): a provider that
// declared endpoint_capacity but whose pool could not be created runs WITHOUT a
// lease rather than failing to boot. Overridable so tests can silence it.
var leaseWarnf = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "llm: "+format+"\n", args...)
}

// leasePool bounds concurrent calls to one normalized endpoint at `capacity`
// slots. It holds no long-lived fds itself — every acquire opens its own fd on a
// slot file (so concurrent acquirers, in this process or another, contend via
// the kernel's per-fd flock) and the release closes it. contended is the pool's
// live congestion flag, set/cleared by acquisition wait time (hysteresis by
// observation, research R2); a provider reads it for its status row.
type leasePool struct {
	dir       string
	capacity  int
	contended atomic.Bool
}

// normalizeEndpoint canonicalizes an endpoint so cosmetically different spellings
// of one server hash to one pool dir (contract): lowercase scheme+host, strip the
// default :80/:443 ports, strip the trailing slash, keep the path. Two providers
// pointed at the same server with different spellings coordinate; genuinely
// different paths (/v1 vs /v2) stay distinct. An unparseable value degrades to a
// lowercased, trailing-slash-trimmed string (still deterministic).
func normalizeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	if h, port, splitErr := net.SplitHostPort(host); splitErr == nil {
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			host = h
		}
	}
	return scheme + "://" + host + strings.TrimRight(u.Path, "/")
}

// endpointHash is the first 16 hex chars of the normalized endpoint's SHA-256 —
// the pool directory name (contract layout).
func endpointHash(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:16]
}

// newLeasePool creates (or adopts) the on-disk pool for an endpoint at the given
// capacity: it makes the pool dir and writes the plaintext `endpoint` name file
// best-effort for operator inspection. Slot files themselves are created lazily
// on first acquire. A dir-creation failure disables leases for the provider
// (returned as an error the caller turns into a warning) — never a hard boot
// error.
func newLeasePool(baseDir, endpoint string, capacity int) (*leasePool, error) {
	norm := normalizeEndpoint(endpoint)
	dir := filepath.Join(baseDir, endpointHash(norm))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// Best-effort operator breadcrumb; a write failure never disables the pool.
	_ = os.WriteFile(filepath.Join(dir, "endpoint"), []byte(norm+"\n"), 0o644)
	return &leasePool{dir: dir, capacity: capacity}, nil
}

// slotPath is the file backing slot i (zero-padded for a tidy directory listing).
func (lp *leasePool) slotPath(i int) string {
	return filepath.Join(lp.dir, fmt.Sprintf("slot-%02d", i))
}

// trySweep attempts one non-blocking pass over slot-00 … slot-(C-1), returning a
// release func for the first slot it locks (nil, false when every slot is held).
// Each slot is opened fresh (lazy 0o644 creation) and flocked LOCK_EX|LOCK_NB; a
// slot that is locked elsewhere, or that cannot be opened, is skipped this pass.
func (lp *leasePool) trySweep() (release func(), ok bool) {
	for i := 0; i < lp.capacity; i++ {
		f, err := os.OpenFile(lp.slotPath(i), os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			continue // a transient FS error just means "not this slot this pass"
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			f.Close()
			continue
		}
		// Held: closing the fd is the release — the kernel drops the flock, and
		// does so too if this process dies, which is the crash-reclaim guarantee.
		return func() { f.Close() }, true
	}
	return nil, false
}

// acquire holds a slot for the duration of one provider call. It sweeps the
// slots non-blockingly and, finding none free, retries on a jittered ~100 ms
// ticker until ctx is done (the worker's 2-min call cap). It returns the release
// func and the total time waited; on ctx expiry it returns that wait and the
// context error (the worker treats it as a caller-gave-up, never a breaker
// strike). A wait above the contended threshold flags the pool contended; a wait
// below it clears the flag (hysteresis by observation — no timer bookkeeping).
func (lp *leasePool) acquire(ctx context.Context) (release func(), waited time.Duration, err error) {
	start := time.Now()
	for {
		if rel, ok := lp.trySweep(); ok {
			waited = time.Since(start)
			lp.contended.Store(waited > leaseContendedThreshold)
			return rel, waited, nil
		}
		timer := time.NewTimer(leaseRetryDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, time.Since(start), ctx.Err()
		case <-timer.C:
		}
	}
}

// leaseRetryDelay is ~100 ms with ±25% jitter, so two daemons sweeping a
// saturated pool don't lock-step and starve each other.
func leaseRetryDelay() time.Duration {
	spread := int64(leaseRetryBase / 2)
	return leaseRetryBase - leaseRetryBase/4 + time.Duration(rand.Int63n(spread))
}
