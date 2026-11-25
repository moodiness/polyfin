package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// slowStatement is how long a statement runs before the log tells it:
// past what a page of the admin app or of a player waits for one
// statement, and short of the 5 seconds a whole listing is logged at.
const slowStatement = 500 * time.Millisecond

// The log of slow statements writes at most slowBurst lines at once, then
// one more each slowRefill: a database slow for every statement does not
// flood the log. It counts the lines left out, and the next line says
// how many.
const (
	slowBurst  = 20
	slowRefill = time.Second
)

// slowSQL bounds the characters of a slow statement's SQL in the log.
const slowSQL = 300

// A slow batch's line tells each of its queries, those queued more than
// once told once with how many times, cut at slowBatchSQL characters
// each, the first slowBatchQueries of them: batches that queue a query
// for each of many rows tell it once, and the line stays short.
const (
	slowBatchSQL     = 120
	slowBatchQueries = 8
)

// slowLog is the pool's tracer: it logs the statements and the batches
// that run for slowStatement or longer, at Info, with how long they ran
// and their SQL, its spaces collapsed and cut at slowSQL characters. A
// batch is timed whole, from its sending until its results are read: its
// queries run one after the other and PostgreSQL holds their answers
// until the batch commits, so none of them can be timed alone. It never
// logs the arguments, which can hold secrets. A statement or a batch that
// runs faster costs two readings of the clock and the context value that
// carries its start; nothing is formatted.
type slowLog struct {
	logger    *slog.Logger
	threshold time.Duration
	// now stands for time.Now, for tests.
	now func() time.Time

	mu      sync.Mutex
	tokens  float64
	filled  time.Time
	skipped int
}

// newSlowLog is a slowLog logging into logger.
func newSlowLog(logger *slog.Logger) *slowLog {
	return &slowLog{logger: logger, threshold: slowStatement, now: time.Now, tokens: slowBurst}
}

// started is when a statement started, and its SQL, as the context of
// the statement carries them.
type started struct {
	at  time.Time
	sql string
}

// startedKey is the context key of started.
type startedKey struct{}

func (s *slowLog) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, startedKey{}, &started{at: s.now(), sql: data.SQL})
}

// TraceQueryEnd is called once the statement's rows are read and closed,
// so that its time includes the caller's reading of them.
func (s *slowLog) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(startedKey{}).(*started)
	if !ok {
		return
	}
	if took := s.now().Sub(start.at); took >= s.threshold {
		s.log(ctx, took, start.sql, data.Err)
	}
}

// startedBatch is when a batch was sent, and the batch, as the context of
// the batch carries them.
type startedBatch struct {
	at    time.Time
	batch *pgx.Batch
}

// startedBatchKey is the context key of startedBatch.
type startedBatchKey struct{}

func (s *slowLog) TraceBatchStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	return context.WithValue(ctx, startedBatchKey{}, &startedBatch{at: s.now(), batch: data.Batch})
}

// TraceBatchQuery times nothing: a query's answer waits for the queries
// after it and the commit (see slowLog).
func (s *slowLog) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {}

// TraceBatchEnd is called once the batch's results are read and closed,
// or once it failed to be sent.
func (s *slowLog) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchEndData) {
	start, ok := ctx.Value(startedBatchKey{}).(*startedBatch)
	if !ok {
		return
	}
	if took := s.now().Sub(start.at); took >= s.threshold {
		s.write(ctx, "A database batch was slow", took, batchSQL(start.batch.QueuedQueries), data.Err, "queries", len(start.batch.QueuedQueries))
	}
}

// log writes a slow statement's line (see write).
func (s *slowLog) log(ctx context.Context, took time.Duration, sql string, err error) {
	s.write(ctx, "A database statement was slow", took, shortSQL(sql, slowSQL), err)
}

// write writes msg with how long the statement or the batch took, its
// SQL, the other attributes given, and its error, unless the lines of the
// moment are spent (see slowBurst).
func (s *slowLog) write(ctx context.Context, msg string, took time.Duration, sql string, err error, attrs ...any) {
	s.mu.Lock()
	now := s.now()
	if !s.filled.IsZero() {
		s.tokens = min(slowBurst, s.tokens+float64(now.Sub(s.filled))/float64(slowRefill))
	}
	s.filled = now
	if s.tokens < 1 {
		s.skipped++
		s.mu.Unlock()
		return
	}
	s.tokens--
	skipped := s.skipped
	s.skipped = 0
	s.mu.Unlock()
	attrs = append([]any{"duration", took.Round(time.Millisecond)}, attrs...)
	attrs = append(attrs, "sql", sql)
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	if skipped > 0 {
		attrs = append(attrs, "skipped", skipped)
	}
	s.logger.InfoContext(ctx, msg, attrs...)
}

// batchSQL tells the queries of a batch in their order, each on one line
// and cut at slowBatchSQL characters, separated by "; ". A query queued
// several times is told once, where it first comes, followed by how many
// times, as "(3 times)"; past slowBatchQueries different queries, the
// line says how many more there are.
func batchSQL(queries []*pgx.QueuedQuery) string {
	var order []string
	times := map[string]int{}
	for _, q := range queries {
		if times[q.SQL] == 0 {
			order = append(order, q.SQL)
		}
		times[q.SQL]++
	}
	var b strings.Builder
	for i, sql := range order {
		if i == slowBatchQueries {
			fmt.Fprintf(&b, "; and %d other queries", len(order)-i)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(shortSQL(sql, slowBatchSQL))
		if n := times[sql]; n > 1 {
			fmt.Fprintf(&b, " (%d times)", n)
		}
	}
	return b.String()
}

// shortSQL is sql on one line, its runs of spaces and line breaks made
// one space, cut at most characters with an ellipsis.
func shortSQL(sql string, most int) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if utf8.RuneCountInString(sql) <= most {
		return sql
	}
	cut, n := 0, 0
	for i := range sql {
		if n == most {
			cut = i
			break
		}
		n++
	}
	return sql[:cut] + "…"
}
