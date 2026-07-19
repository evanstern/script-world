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
type Meter struct {
	mu     sync.Mutex
	st     MeterStore
	budget float64
	month  string // "2026-07"
	spent  float64
}

func metaKey(month string) string { return "llm_spend_" + month }

func currentMonth() string { return time.Now().UTC().Format("2006-01") }

func NewMeter(st MeterStore, budgetUSD float64) (*Meter, error) {
	m := &Meter{st: st, budget: budgetUSD, month: currentMonth()}
	raw, err := st.GetMeta(metaKey(m.month))
	if err != nil {
		return nil, err
	}
	if raw != "" {
		if m.spent, err = strconv.ParseFloat(raw, 64); err != nil {
			return nil, fmt.Errorf("corrupt spend meter %q: %w", raw, err)
		}
	}
	return m, nil
}

// rollover resets the counter when the UTC month changes. Callers hold mu.
func (m *Meter) rollover() {
	if now := currentMonth(); now != m.month {
		m.month = now
		m.spent = 0
	}
}

// Allow reports whether another cloud call may start: the ceiling throttles
// BEFORE the call is made, never after the money is spent.
func (m *Meter) Allow() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollover()
	return m.spent < m.budget
}

// Add records the actual cost of a completed call and persists it.
func (m *Meter) Add(costUSD float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollover()
	m.spent += costUSD
	return m.st.SetMeta(metaKey(m.month), strconv.FormatFloat(m.spent, 'f', -1, 64))
}

// Snapshot returns (month, spent, budget) for status reporting.
func (m *Meter) Snapshot() (string, float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollover()
	return m.month, m.spent, m.budget
}
