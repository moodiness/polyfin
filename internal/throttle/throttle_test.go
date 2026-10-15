package throttle

import (
	"sync"
	"testing"
	"time"
)

// clocked returns failures on a clock the test moves.
func clocked(limit int) (*Failures, *time.Time) {
	f := New(limit, time.Minute)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f.now = func() time.Time { return now }
	return f, &now
}

// Attempts made at once, none of them settled yet, are refused past the
// limit.
func TestReserveCountsAttemptsAtOnce(t *testing.T) {
	t.Parallel()
	f := New(3, time.Minute)
	start := make(chan struct{})
	var mu sync.Mutex
	reserved := 0
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			<-start
			if attempt, _ := f.Reserve("client"); attempt != nil {
				mu.Lock()
				reserved++
				mu.Unlock()
			}
		})
	}
	close(start)
	wg.Wait()
	if reserved != 3 {
		t.Errorf("%d attempts reserved at once with a limit of 3", reserved)
	}
}

func TestSettlingAnAttempt(t *testing.T) {
	t.Parallel()
	f, now := clocked(2)
	failed, _ := f.Reserve("client")
	failed.Fail()
	// Cancel after Fail changes nothing: a deferred Cancel only takes back
	// an attempt left unsettled.
	failed.Cancel()
	cancelled, _ := f.Reserve("client")
	cancelled.Cancel()
	last, _ := f.Reserve("client")
	if last == nil {
		t.Fatal("refused with one failure and an attempt taken back")
	}
	if attempt, wait := f.Reserve("client"); attempt != nil || wait != time.Minute {
		t.Fatalf("with one failure and one attempt under way: %v, wait %v", attempt, wait)
	}
	// A success forgets the failures of the key.
	last.Succeed()
	if attempt, _ := f.Reserve("client"); attempt == nil {
		t.Fatal("refused after a success")
	}

	// A failure expires after the window.
	other, _ := f.Reserve("other")
	other.Fail()
	*now = now.Add(30 * time.Second)
	again, _ := f.Reserve("other")
	again.Fail()
	if attempt, wait := f.Reserve("other"); attempt != nil || wait != 30*time.Second {
		t.Fatalf("at the limit: %v, wait %v", attempt, wait)
	}
	*now = now.Add(30 * time.Second)
	if attempt, _ := f.Reserve("other"); attempt == nil {
		t.Error("refused once the first failure expired")
	}
}

// An attempt of several keys is counted for all of them, or for none when
// one of them is at its limit.
func TestReserveSeveralKeys(t *testing.T) {
	t.Parallel()
	f, _ := clocked(1)
	user, _ := f.Reserve("user")
	user.Fail()
	if attempt, _ := f.Reserve("client", "user"); attempt != nil {
		t.Fatal("reserved past the user's limit")
	}
	if attempt, _ := f.Reserve("client"); attempt == nil {
		t.Fatal("a refused attempt counted toward the client")
	}
}
