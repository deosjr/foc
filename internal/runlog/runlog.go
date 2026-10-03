// Package runlog is the turn log: an append-only list of structured records
// (letters, interpretations, orders, resolution, observations, facts, RNG
// draws, model calls) that is written to turns.jsonl.
package runlog

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"sync"
)

// Record is one line of turns.jsonl.
type Record struct {
	Turn  int    `json:"turn"`
	Phase string `json:"phase"`
	Kind  string `json:"kind"`
	Data  any    `json:"data"`
}

// Log collects records. It is safe for concurrent use. Model calls made
// concurrently are held as pending and flushed in a stable order, so the log
// is identical from run to run.
type Log struct {
	mu      sync.Mutex
	turn    int
	phase   string
	lines   [][]byte
	pending []pending
}

type pending struct {
	key  string
	line []byte
}

func New() *Log { return &Log{} }

// SetPhase sets the turn and phase stamped on subsequent records.
func (l *Log) SetPhase(turn int, phase string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushLocked()
	l.turn, l.phase = turn, phase
}

// Add appends a record.
func (l *Log) Add(kind string, data any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, l.encode(kind, data))
}

// AddUnordered queues a record made from a concurrent goroutine. Pending
// records are written sorted by key at the next Flush or SetPhase.
func (l *Log) AddUnordered(key, kind string, data any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, pending{key: key, line: l.encode(kind, data)})
}

// Flush writes pending records in key order.
func (l *Log) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushLocked()
}

func (l *Log) flushLocked() {
	sort.SliceStable(l.pending, func(i, j int) bool { return l.pending[i].key < l.pending[j].key })
	for _, p := range l.pending {
		l.lines = append(l.lines, p.line)
	}
	l.pending = nil
}

func (l *Log) encode(kind string, data any) []byte {
	b, err := json.Marshal(Record{Turn: l.turn, Phase: l.phase, Kind: kind, Data: data})
	if err != nil {
		b, _ = json.Marshal(Record{Turn: l.turn, Phase: l.phase, Kind: "log-error", Data: err.Error()})
	}
	return b
}

// WriteTo writes every record as JSON lines.
func (l *Log) WriteTo(w io.Writer) (int64, error) {
	l.Flush()
	l.mu.Lock()
	defer l.mu.Unlock()
	var n int64
	for _, line := range l.lines {
		m, err := w.Write(append(line, '\n'))
		n += int64(m)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// Bytes returns the log as JSON lines.
func (l *Log) Bytes() []byte {
	var buf bytes.Buffer
	l.WriteTo(&buf)
	return buf.Bytes()
}

// Records returns the decoded records, for tests and the review.
func (l *Log) Records() []map[string]any {
	l.Flush()
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]map[string]any, 0, len(l.lines))
	for _, line := range l.lines {
		var m map[string]any
		if json.Unmarshal(line, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}
