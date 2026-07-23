package llm

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

// MeterStore is the persistence the meter needs — satisfied by *store.Store.
type MeterStore interface {
	GetMeta(key string) (string, error)
	SetMeta(key, value string) error
}

// Meter tracks cloud spend against the monthly ceiling, persisted in the
// world's meta table so restarts never forget money already spent. Months
// are wall-clock UTC — spend is an operator concern, not sim state.
//
// One wallet, per-provider attribution (spec 024 US4, research R4): the total
// key llm_spend_YYYY-MM stays authoritative — it is what Allow() reads and what
// legacy worlds already persisted, so nothing migrates — and each Add also
// writes an additive per-provider key llm_spend_YYYY-MM:<provider>. Snapshot
// sums the per-provider keys; the total minus that sum is the (unattributed)
// remainder a legacy month (total with no breakdown) surfaces.
type Meter struct {
	mu     sync.Mutex
	st     MeterStore
	budget float64
	month  string // "2026-07"
	spent  float64
	// providers is the declared provider roster, fixed at construction: the
	// meter loads each one's persisted per-provider key at open and enumerates
	// them for the snapshot. A key with no declared provider (config drift) is
	// simply not reloaded — it folds into the unattributed remainder.
	providers []string
	// perProvider is the in-memory attribution for the current month, seeded from
	// the store at open and updated on every Add. Cleared on month rollover.
	perProvider map[string]float64
}

func metaKey(month string) string { return "llm_spend_" + month }

// providerMetaKey is the additive per-provider attribution key: the total key
// with a ":<provider>" suffix, so it sorts beside the total and a legacy world
// (no such keys) reads back exactly its total under (unattributed).
func providerMetaKey(month, provider string) string {
	return "llm_spend_" + month + ":" + provider
}

func currentMonth() string { return time.Now().UTC().Format("2006-01") }

// NewMeter opens the meter for the current month: it loads the authoritative
// total and each declared provider's persisted attribution (so per-provider
// spend survives a restart with zero migration, FR-010). providers is the
// declared roster — the names whose per-provider keys are reloaded and
// enumerated in the snapshot.
func NewMeter(st MeterStore, budgetUSD float64, providers []string) (*Meter, error) {
	m := &Meter{
		st: st, budget: budgetUSD, month: currentMonth(),
		providers: providers, perProvider: make(map[string]float64, len(providers)),
	}
	raw, err := st.GetMeta(metaKey(m.month))
	if err != nil {
		return nil, err
	}
	if raw != "" {
		if m.spent, err = strconv.ParseFloat(raw, 64); err != nil {
			return nil, fmt.Errorf("corrupt spend meter %q: %w", raw, err)
		}
	}
	for _, name := range providers {
		praw, err := st.GetMeta(providerMetaKey(m.month, name))
		if err != nil {
			return nil, err
		}
		if praw == "" {
			continue
		}
		v, err := strconv.ParseFloat(praw, 64)
		if err != nil {
			return nil, fmt.Errorf("corrupt spend meter %q: %w", praw, err)
		}
		m.perProvider[name] = v
	}
	return m, nil
}

// rollover resets the counter when the UTC month changes. Both the total and
// the per-provider attribution roll together (a fresh month starts from zero on
// every key; the new month's keys are written on the next Add). Callers hold mu.
func (m *Meter) rollover() {
	if now := currentMonth(); now != m.month {
		m.month = now
		m.spent = 0
		m.perProvider = make(map[string]float64, len(m.providers))
	}
}

// Allow reports whether another cloud call may start: the ceiling throttles
// BEFORE the call is made, never after the money is spent. It reads ONLY the
// authoritative total (one wallet), never a per-provider slice.
func (m *Meter) Allow() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollover()
	return m.spent < m.budget
}

// Add records the actual cost of a completed call and persists it. Under one
// lock it writes BOTH the authoritative total key (llm_spend_YYYY-MM — the
// meaning Allow() reads, so legacy persisted spend carries forward untouched)
// AND the additive per-provider attribution key (llm_spend_YYYY-MM:<provider>).
// The total is written first: if the second write fails the money still counts
// against the ceiling, folding into the unattributed remainder rather than
// vanishing (research R4).
func (m *Meter) Add(provider string, costUSD float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollover()
	m.spent += costUSD
	if err := m.st.SetMeta(metaKey(m.month), strconv.FormatFloat(m.spent, 'f', -1, 64)); err != nil {
		return err
	}
	m.perProvider[provider] += costUSD
	return m.st.SetMeta(providerMetaKey(m.month, provider), strconv.FormatFloat(m.perProvider[provider], 'f', -1, 64))
}

// Snapshot returns (month, spent, budget, perProvider) for status reporting. The
// returned map is a copy — safe to read without the lock. On every path
// Σ(perProvider) + unattributed == spent, where unattributed = spent −
// Σ(perProvider) is what a legacy month (a total with no per-provider keys)
// surfaces.
func (m *Meter) Snapshot() (month string, spent, budget float64, perProvider map[string]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollover()
	pp := make(map[string]float64, len(m.perProvider))
	for k, v := range m.perProvider {
		pp[k] = v
	}
	return m.month, m.spent, m.budget, pp
}
