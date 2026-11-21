package database

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moodiness/polyfin/internal/testdb"
)

// lockedBuffer is a bytes.Buffer safe for the pool's goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testSlowLog is a slowLog logging into out, whose clock reads *clock.
func testSlowLog(out *lockedBuffer, clock *time.Time) *slowLog {
	s := newSlowLog(slog.New(slog.NewTextHandler(out, nil)))
	s.now = func() time.Time { return *clock }
	return s
}

// A statement under the threshold logs nothing; one at it or over logs
// its duration and its SQL on one line, cut short, never its arguments,
// and its error. A batch's query counts from when its result was asked
// for.
func TestOnlySlowStatementsAreLogged(t *testing.T) {
	t.Parallel()
	out := &lockedBuffer{}
	clock := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	s := testSlowLog(out, &clock)
	run := func(sql string, took time.Duration, err error, args ...any) {
		ctx := s.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: sql, Args: args})
		clock = clock.Add(took)
		s.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: err})
	}
	run("SELECT 1", 499*time.Millisecond, nil)
	if out.String() != "" {
		t.Fatalf("a fast statement logged %s", out)
	}
	run("SELECT id\n\t  FROM accounts\n\t WHERE password_hash = $1", 500*time.Millisecond, nil, "hunter2")
	if got := out.String(); !strings.Contains(got, `level=INFO msg="A database statement was slow" duration=500ms sql="SELECT id FROM accounts WHERE password_hash = $1"`) ||
		strings.Contains(got, "hunter2") {
		t.Errorf("logged %s", got)
	}
	long := "SELECT " + strings.Repeat("é, ", 200) + "1"
	run(long, 2*time.Second, errors.New("canceling statement due to user request"))
	// 300 characters: "SELECT ", 97 times "é, ", then "é,".
	if got := out.String(); !strings.Contains(got, `duration=2s sql="SELECT `+strings.Repeat("é, ", 97)+`é,…" error="canceling statement due to user request"`) {
		t.Errorf("logged %s", got)
	}
	s.TraceBatchQuery(t.Context(), nil, pgx.TraceBatchQueryData{SQL: "UPDATE items SET seen = $1", Args: []any{"hunter2"}, ReadStartTime: clock.Add(-time.Second)})
	s.TraceBatchQuery(t.Context(), nil, pgx.TraceBatchQueryData{SQL: "UPDATE fast SET seen = $1", ReadStartTime: clock.Add(-time.Millisecond)})
	if got := out.String(); strings.Count(got, "\n") != 3 || !strings.Contains(got, `duration=1s sql="UPDATE items SET seen = $1"`) || strings.Contains(got, "hunter2") {
		t.Errorf("logged %s", got)
	}
}

// A database slow for every statement logs 20 lines at once, then one a
// second, and the next line says how many were left out.
func TestTheLogOfSlowStatementsIsBounded(t *testing.T) {
	t.Parallel()
	out := &lockedBuffer{}
	clock := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	s := testSlowLog(out, &clock)
	for range 25 {
		s.log(t.Context(), time.Second, "SELECT 1", nil)
	}
	if got := strings.Count(out.String(), "\n"); got != slowBurst {
		t.Fatalf("%d lines at once, want %d", got, slowBurst)
	}
	clock = clock.Add(5 * slowRefill)
	s.log(t.Context(), time.Second, "SELECT 2", nil)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != slowBurst+1 || !strings.HasSuffix(lines[len(lines)-1], `sql="SELECT 2" skipped=5`) {
		t.Errorf("logged %s", out)
	}
}

// The pool Polyfin opens logs its slow statements and batch queries,
// without their arguments. The test's clock is the real one plus an hour
// more at each reading, so that every statement is slow: pgx times a
// batch's queries from its own reading of the real clock, which this one
// is always hours past, whatever the date.
func TestThePoolLogsSlowStatements(t *testing.T) {
	t.Parallel()
	url := testdb.CopyURL(t, testdb.Empty(t))
	out := &lockedBuffer{}
	config, err := poolConfig(url, slog.New(slog.NewTextHandler(out, nil)))
	if err != nil {
		t.Fatal(err)
	}
	tracer, ok := config.ConnConfig.Tracer.(*slowLog)
	if !ok {
		t.Fatalf("the pool's tracer is %T", config.ConnConfig.Tracer)
	}
	var hours atomic.Int64
	tracer.now = func() time.Time { return time.Now().Add(time.Duration(hours.Add(1)) * time.Hour) }
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var got string
	if err := pool.QueryRow(t.Context(), "SELECT $1::text", "hunter2").Scan(&got); err != nil || got != "hunter2" {
		t.Fatalf("%q, %v", got, err)
	}
	batch := &pgx.Batch{}
	batch.Queue("SELECT length($1::text)", "hunter3")
	if err := pool.SendBatch(t.Context(), batch).Close(); err != nil {
		t.Fatal(err)
	}
	logged := out.String()
	for _, want := range []string{`msg="A database statement was slow" duration=1h`, `sql="SELECT $1::text"`, `sql="SELECT length($1::text)"`} {
		if !strings.Contains(logged, want) {
			t.Errorf("did not log %s: %s", want, logged)
		}
	}
	if strings.Contains(logged, "hunter") {
		t.Errorf("logged an argument: %s", logged)
	}
}

// What a fast statement costs the tracer.
func BenchmarkAFastStatement(b *testing.B) {
	s := newSlowLog(slog.New(slog.DiscardHandler))
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		s.TraceQueryEnd(s.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"}), nil, pgx.TraceQueryEndData{})
	}
}
