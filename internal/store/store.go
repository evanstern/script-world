// Package store persists a world run: the append-only event log (source of
// truth), snapshots (recovery accelerator), and meta. SQLite in WAL mode via
// the pure-Go modernc.org/sqlite driver.
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
	Seq      int64           `json:"seq"`
	Tick     int64           `json:"tick"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	WallTime string          `json:"wall_time,omitempty"`
}

type Snapshot struct {
	ID    int64
	Tick  int64
	Seq   int64
	State []byte
	Hash  string
}

type Store struct {
	db      *sql.DB
	lastSeq int64
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// The sim loop is the single writer; one connection sidesteps SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	s := &Store{db: db}
	if err := s.db.QueryRow("SELECT COALESCE(MAX(seq), 0) FROM events").Scan(&s.lastSeq); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) LastSeq() int64 { return s.lastSeq }

// AppendEvents assigns contiguous seqs and writes the batch in one
// transaction. Callers treat the batch as one tick's worth of history: none
// of it is visible to subscribers until this returns.
func (s *Store) AppendEvents(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range events {
		events[i].Seq = s.lastSeq + int64(i) + 1
		if events[i].WallTime == "" {
			events[i].WallTime = now
		}
		if _, err := tx.Exec(
			"INSERT INTO events (seq, tick, type, payload, wall_time) VALUES (?, ?, ?, ?, ?)",
			events[i].Seq, events[i].Tick, events[i].Type, string(events[i].Payload), events[i].WallTime,
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.lastSeq += int64(len(events))
	return nil
}

// ReplayEvents streams events with seq > sinceSeq, in order, through fn.
func (s *Store) ReplayEvents(sinceSeq int64, fn func(Event) error) error {
	rows, err := s.db.Query(
		"SELECT seq, tick, type, payload, wall_time FROM events WHERE seq > ? ORDER BY seq", sinceSeq)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e Event
		var payload string
		if err := rows.Scan(&e.Seq, &e.Tick, &e.Type, &payload, &e.WallTime); err != nil {
			return err
		}
		e.Payload = json.RawMessage(payload)
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// EventsSince returns up to limit events with seq > sinceSeq (limit <= 0
// means no limit).
func (s *Store) EventsSince(sinceSeq int64, limit int) ([]Event, error) {
	var out []Event
	err := s.ReplayEvents(sinceSeq, func(e Event) error {
		if limit > 0 && len(out) >= limit {
			return errStopReplay
		}
		out = append(out, e)
		return nil
	})
	if errors.Is(err, errStopReplay) {
		err = nil
	}
	return out, err
}

var errStopReplay = errors.New("stop replay")

func stateHash(state []byte) string {
	sum := sha256.Sum256(state)
	return hex.EncodeToString(sum[:])
}

func (s *Store) SaveSnapshot(tick, seq int64, state []byte) error {
	_, err := s.db.Exec(
		"INSERT INTO snapshots (tick, seq, state, state_hash, wall_time) VALUES (?, ?, ?, ?, ?)",
		tick, seq, string(state), stateHash(state), time.Now().UTC().Format(time.RFC3339))
	return err
}

// LatestValidSnapshot returns the newest snapshot whose state_hash verifies,
// falling back through older ones; nil if none survive (replay from genesis).
func (s *Store) LatestValidSnapshot() (*Snapshot, error) {
	rows, err := s.db.Query(
		"SELECT id, tick, seq, state, state_hash FROM snapshots ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var snap Snapshot
		var state string
		if err := rows.Scan(&snap.ID, &snap.Tick, &snap.Seq, &state, &snap.Hash); err != nil {
			return nil, err
		}
		snap.State = []byte(state)
		if stateHash(snap.State) == snap.Hash {
			return &snap, nil
		}
	}
	return nil, rows.Err()
}

// PruneSnapshots keeps the newest keep snapshots. Events are never touched.
func (s *Store) PruneSnapshots(keep int) error {
	_, err := s.db.Exec(
		"DELETE FROM snapshots WHERE id NOT IN (SELECT id FROM snapshots ORDER BY id DESC LIMIT ?)", keep)
	return err
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value)
	return err
}

func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// CheckContiguity verifies the log has no gaps: seq runs 1..N. A gap is a
// fatal integrity error — the daemon refuses to run on a holed history.
func (s *Store) CheckContiguity() error {
	var count, minSeq, maxSeq int64
	err := s.db.QueryRow(
		"SELECT COUNT(*), COALESCE(MIN(seq), 0), COALESCE(MAX(seq), 0) FROM events").
		Scan(&count, &minSeq, &maxSeq)
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	if minSeq != 1 || maxSeq != count {
		return fmt.Errorf(
			"event log integrity error: %d events but seq range [%d, %d]; the log has gaps — restore this world from a backup copy of its save directory",
			count, minSeq, maxSeq)
	}
	return nil
}

// LastEventTick returns the tick of the newest event (0 if none).
func (s *Store) LastEventTick() (int64, error) {
	var tick int64
	err := s.db.QueryRow("SELECT COALESCE(MAX(tick), 0) FROM events").Scan(&tick)
	return tick, err
}
