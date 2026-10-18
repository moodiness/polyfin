package torrent

import (
	"context"
	"sync"
	"time"
)

// limiter spaces bytes to a rate read at each use, in bytes a second: a
// token bucket holding a second's worth at most. A rate of 0 is unlimited
// unless zeroBlocks, in which case nothing passes.
type limiter struct {
	rate       func() int64
	zeroBlocks bool
	now        func() time.Time

	mu     sync.Mutex
	tokens float64
	last   time.Time
}

func newLimiter(rate func() int64, zeroBlocks bool) *limiter {
	return &limiter{rate: rate, zeroBlocks: zeroBlocks, now: time.Now}
}

// wait waits until n bytes may pass, or ctx is done.
func (l *limiter) wait(ctx context.Context, n int64) error {
	for {
		delay, ok := l.take(n)
		if ok {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// take takes n bytes when they may pass, else tells how long to wait.
func (l *limiter) take(n int64) (time.Duration, bool) {
	rate := l.rate()
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if rate <= 0 {
		l.last = now
		if l.zeroBlocks {
			return time.Second, false
		}
		return 0, true
	}
	burst := float64(max(rate, blockSize))
	if !l.last.IsZero() {
		l.tokens = min(burst, l.tokens+now.Sub(l.last).Seconds()*float64(rate))
	}
	l.last = now
	if l.tokens >= float64(n) || l.tokens >= burst {
		l.tokens -= float64(n)
		return 0, true
	}
	return time.Duration((float64(n) - l.tokens) / float64(rate) * float64(time.Second)), false
}

// allows reports whether the limiter lets anything pass: false for one at
// 0 that blocks.
func (l *limiter) allows() bool {
	return !l.zeroBlocks || l.rate() > 0
}
