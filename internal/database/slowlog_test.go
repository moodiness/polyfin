package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
// and its error.
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
}

// A batch is logged whole, once, when it took the threshold or longer
// from its sending until its results were read: with how many queries it
// held and each of them, one queued several times told once, never their
// arguments, and its error. None of its queries is logged on its own,
// whose answer waited for the others. Its lines count with the others
// against slowBurst.
func TestASlowBatchIsLoggedWhole(t *testing.T) {
	t.Parallel()
	out := &lockedBuffer{}
	clock := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	s := testSlowLog(out, &clock)
	long := "UPDATE items SET data = " + strings.Repeat("x || ", 40) + "y"
	batch := &pgx.Batch{}
	batch.Queue("SELECT id, xmin\n\t FROM items WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE", "hunter2")
	for range 3 {
		batch.Queue("INSERT INTO seen (id) VALUES ($1)", "hunter3")
	}
	batch.Queue(long)
	// The first answer comes once the batch committed, as PostgreSQL holds
	// the answers until then; the others follow at once. Each run starts
	// at the same time, so that the lines spent are not refilled.
	start := clock
	run := func(batch *pgx.Batch, took time.Duration, err error) {
		clock = start
		ctx := s.TraceBatchStart(t.Context(), nil, pgx.TraceBatchStartData{Batch: batch})
		read := clock
		clock = clock.Add(took)
		for _, q := range batch.QueuedQueries {
			s.TraceBatchQuery(ctx, nil, pgx.TraceBatchQueryData{SQL: q.SQL, Args: q.Arguments, ReadStartTime: read})
			read = clock
		}
		s.TraceBatchEnd(ctx, nil, pgx.TraceBatchEndData{Err: err})
	}
	run(batch, 495*time.Millisecond, nil)
	if out.String() != "" {
		t.Fatalf("a fast batch logged %s", out)
	}
	run(batch, 5*time.Second, errors.New("canceling statement due to user request"))
	// 120 characters of the long query: "UPDATE items SET data = ", then
	// 19 times "x || ", then "x".
	want := `level=INFO msg="A database batch was slow" duration=5s queries=5 sql="SELECT id, xmin FROM items WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE; ` +
		`INSERT INTO seen (id) VALUES ($1) (3 times); UPDATE items SET data = ` + strings.Repeat("x || ", 19) + `x…" error="canceling statement due to user request"`
	if got := out.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, want) || strings.Contains(got, "statement was slow") ||
		strings.Contains(got, "hunter") {
		t.Errorf("logged %s\nwant %s", got, want)
	}

	// Past 8 different queries, the line counts the others.
	many := &pgx.Batch{}
	for i := range 11 {
		many.Queue(fmt.Sprintf("SELECT %d", i))
	}
	run(many, 5*time.Second, nil)
	if got := out.String(); !strings.Contains(got, `queries=11 sql="SELECT 0; SELECT 1; SELECT 2; SELECT 3; SELECT 4; SELECT 5; SELECT 6; SELECT 7; and 3 other queries"`) {
		t.Errorf("logged %s", got)
	}

	for range 25 {
		run(batch, 5*time.Second, nil)
	}
	if got := strings.Count(out.String(), "\n"); got != slowBurst {
		t.Errorf("%d lines at once, want %d", got, slowBurst)
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

// A batch whose second query is slow logs one line, the batch's, with
// its two queries: its first query, whose answer PostgreSQL held until the
// second was done, is not told as slow.
func TestThePoolLogsASlowBatchWhole(t *testing.T) {
	t.Parallel()
	url := testdb.CopyURL(t, testdb.Empty(t))
	out := &lockedBuffer{}
	config, err := poolConfig(url, slog.New(slog.NewTextHandler(out, nil)))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	batch := &pgx.Batch{}
	batch.Queue("SELECT 1")
	batch.Queue("SELECT pg_sleep(0.6)")
	if err := pool.SendBatch(t.Context(), batch).Close(); err != nil {
		t.Fatal(err)
	}
	logged := out.String()
	if strings.Contains(logged, "statement was slow") || !strings.Contains(logged, `msg="A database batch was slow"`) ||
		!strings.Contains(logged, `queries=2 sql="SELECT 1; SELECT pg_sleep(0.6)"`) {
		t.Errorf("logged %s", logged)
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
