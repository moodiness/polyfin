package library

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// statements records the SQL statements a pool runs, with their arguments.
type statements struct {
	mu   sync.Mutex
	seen []statement
}

type statement struct {
	sql  string
	args []any
}

func (s *statements) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, statement{sql: data.SQL, args: data.Args})
	return ctx
}

func (s *statements) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// run returns the statements f runs.
func (s *statements) run(f func()) []statement {
	s.mu.Lock()
	s.seen = nil
	s.mu.Unlock()
	f()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

// take returns the statements run since the last take, counted by their
// first word and table, and their total.
func (s *statements) take() (map[string]int, int) {
	s.mu.Lock()
	seen := s.seen
	s.seen = nil
	s.mu.Unlock()
	counts := map[string]int{}
	for _, st := range seen {
		counts[st.kind()]++
	}
	return counts, len(seen)
}

// kind is a statement's first word, with the table it reads or writes.
func (st statement) kind() string {
	words := strings.Fields(st.sql)
	key := ""
	if len(words) > 0 {
		key = strings.ToUpper(words[0])
	}
	for i, word := range words {
		if i > 0 && (strings.EqualFold(words[i-1], "FROM") || strings.EqualFold(words[i-1], "INTO") || strings.EqualFold(words[i-1], "UPDATE")) {
			key += " " + strings.Trim(word, "(),")
			break
		}
	}
	return key
}
