package bandwidth

import (
	"math"
	"testing"
	"time"
)

// clock is a time the test moves on.
type clock struct{ now time.Time }

func (c *clock) read() time.Time { return c.now }

func near(got, want float64) bool {
	return math.Abs(got-want) <= want*0.02
}

// A synthetic stream of 2 MB/s, sent in 64 KiB writes, then throttled to
// 250 kB/s as a slow app reads: the meter tells the first rate once a
// window has passed, and the second one window after the drop. Another
// key's bytes count in the total only.
func TestMeterFollowsAStreamAndItsDrop(t *testing.T) {
	c := &clock{now: time.Unix(1_700_000_000, 0)}
	ms := New()
	ms.now = c.read
	m := ms.Hold("play")
	other := ms.Hold("other")
	ms.Sample()
	send := func(perSecond int, seconds int) {
		for range seconds {
			for sent := 0; sent < perSecond; {
				n := min(64<<10, perSecond-sent)
				m.Add(n)
				sent += n
			}
			other.Add(1000)
			c.now = c.now.Add(Every)
			ms.Sample()
		}
	}
	send(2_000_000, 3)
	if got := ms.Rate("play"); !near(got, 2_000_000) {
		t.Errorf("after 3 s: %.0f B/s, not 2 MB/s", got)
	}
	send(2_000_000, 20)
	if got := ms.Rate("play"); !near(got, 2_000_000) {
		t.Errorf("after 23 s: %.0f B/s, not 2 MB/s", got)
	}
	// Half a window after the drop, the average is between the two.
	send(250_000, 5)
	if got := ms.Rate("play"); got <= 300_000 || got >= 1_900_000 {
		t.Errorf("5 s after the drop: %.0f B/s", got)
	}
	send(250_000, 5)
	if got := ms.Rate("play"); !near(got, 250_000) {
		t.Errorf("a window after the drop: %.0f B/s, not 250 kB/s", got)
	}
	if got, want := ms.Total("play"), int64(23*2_000_000+10*250_000); got != want {
		t.Errorf("total %d, not %d", got, want)
	}
	if got := ms.TotalRate(); !near(got, 251_000) {
		t.Errorf("all meters: %.0f B/s, not 251 kB/s", got)
	}
	if got := ms.Rate("play", "other", "play", ""); !near(got, 251_000) {
		t.Errorf("both keys, once each: %.0f B/s", got)
	}
	// Stopped, the rate falls to nothing within a window.
	send(0, 11)
	if got := ms.Rate("play"); got != 0 {
		t.Errorf("stopped: %.0f B/s", got)
	}
}

// A meter nothing holds is forgotten once it counted nothing for an hour;
// one held is kept, and so are its bytes.
func TestMetersForgetIdleMeters(t *testing.T) {
	c := &clock{now: time.Unix(1_700_000_000, 0)}
	ms := New()
	ms.now = c.read
	held, released := ms.Hold("held"), ms.Hold("released")
	held.Add(10)
	released.Add(20)
	ms.Release(released)
	ms.Sample()
	c.now = c.now.Add(idleFor - time.Second)
	ms.Sample()
	if ms.Total("released") != 20 {
		t.Fatal("forgotten before an hour")
	}
	c.now = c.now.Add(time.Second)
	ms.Sample()
	if ms.Total("released") != 0 || ms.Total("held") != 10 {
		t.Errorf("after an hour: released %d, held %d", ms.Total("released"), ms.Total("held"))
	}
	// Held again, the key starts anew.
	again := ms.Hold("released")
	again.Add(5)
	if ms.Total("released") != 5 {
		t.Errorf("held again: %d", ms.Total("released"))
	}
}

// Adding never allocates: it runs on every write of a playback.
func TestAddDoesNotAllocate(t *testing.T) {
	m := New().Hold("play")
	if allocs := testing.AllocsPerRun(1000, func() { m.Add(32 << 10) }); allocs != 0 {
		t.Errorf("%v allocations per Add", allocs)
	}
}
