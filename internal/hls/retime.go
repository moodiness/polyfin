package hls

import (
	"cmp"
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// The pace of the timings made again (see Retime): Polyfin is quiet once
// it did no work for quietFor, which it looks at every quietPoll while it
// waits or times, and every workPoll otherwise; a busy timing is made
// again retryAfter later, plus up to retryJitter.
const (
	quietFor    = 10 * time.Minute
	quietPoll   = 2 * time.Second
	workPoll    = 15 * time.Second
	retryAfter  = 30 * time.Minute
	retryJitter = 30 * time.Minute
)

// errWorkStarted stops a timing made again once Polyfin works meanwhile.
var errWorkStarted = errors.New("work started during the timing")

// retimer times the GPU and the processor again when Polyfin is quiet
// (see Retime). Its zero value has the defaults.
type retimer struct {
	// run serializes the timings, at startup and later.
	run sync.Mutex
	// otherWork reports heavy work besides Polyfin's conversions (see
	// UseOtherWork); seen is when Polyfin was last seen working, and
	// timed when a timing last ended, in Unix nanoseconds.
	otherWork atomic.Pointer[func() bool]
	seen      atomic.Int64
	timed     atomic.Int64
	// retry stops the busy timing made again later, nil when none
	// waits; under Manager.mu.
	retry func() bool
	// For tests: clock stands for time.Now, the time quiet, retries and
	// timings kept count by; quiet and poll for quietFor and quietPoll;
	// later runs a retry after its delay in place of time.AfterFunc,
	// returning what stops it; polled hears each look at whether Polyfin
	// is quiet, at what time and with what answer; applied hears each
	// timing made again once all it found is in force.
	clock       func() time.Time
	quiet, poll time.Duration
	later       func(delay time.Duration, run func()) (stop func() bool)
	polled      func(now time.Time, quiet bool)
	applied     func()
}

// now is the time quiet, retries and timings kept count by.
func (r *retimer) now() time.Time {
	if r.clock != nil {
		return r.clock()
	}
	return time.Now()
}

// UseOtherWork has busy report the heavy work Polyfin does besides its
// conversions, such as a scheduled task making thumbnails: the timings
// made again wait for it to end too.
func (m *Manager) UseOtherWork(busy func() bool) {
	m.retime.otherWork.Store(&busy)
}

// working reports whether Polyfin works now: converts video, for a
// playback, a channel or a file converted whole, airs a channel, records,
// or does the other heavy work UseOtherWork tells.
func (m *Manager) working() bool {
	m.mu.Lock()
	n := m.converting(nil, nil) + len(m.recordings)
	m.mu.Unlock()
	if n > 0 {
		return true
	}
	busy := m.retime.otherWork.Load()
	return busy != nil && (*busy)()
}

// watchWork notes when Polyfin works, every workPoll, until Close: the
// quiet the timings made again wait for counts from the last time.
func (m *Manager) watchWork() {
	ticker := time.NewTicker(workPoll)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if m.working() {
				m.retime.seen.Store(m.retime.now().UnixNano())
			}
		}
	}
}

// quiet reports whether Polyfin did no work for quietFor, or, by hand,
// does none now.
func (m *Manager) quiet(byHand bool) bool {
	now := m.retime.now()
	quiet := !m.working()
	if !quiet {
		m.retime.seen.Store(now.UnixNano())
	} else if !byHand {
		quiet = now.Sub(time.Unix(0, m.retime.seen.Load())) >= cmp.Or(m.retime.quiet, quietFor)
	}
	if m.retime.polled != nil {
		m.retime.polled(now, quiet)
	}
	return quiet
}

// Retime times the GPU chosen and the processor's HDR tone mapping again,
// as at startup, once Polyfin is quiet: no work (see working) for 10
// minutes, or none now when an administrator asked (byHand). Work
// starting meanwhile stops the timing, which waits for the next quiet
// time. What it finds goes in place as at startup: the best recent
// timings, unless the server was busy (see timingStore), for the
// conversions starting afterwards, those under way going on as they
// started. It returns once a timing ended, or ctx did, or the Manager
// closed.
func (m *Manager) Retime(ctx context.Context, byHand bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-m.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	began := m.retime.now().UnixNano()
	for {
		if err := m.waitQuiet(ctx, byHand); err != nil {
			return err
		}
		err := m.timeAgain(ctx, began)
		if !errors.Is(err, errWorkStarted) {
			return err
		}
		m.logger.Info("Work started during the timing: it is tried again when the server is quiet")
	}
}

// waitQuiet waits until Polyfin is quiet (see quiet), or ctx ends.
func (m *Manager) waitQuiet(ctx context.Context, byHand bool) error {
	for !m.quiet(byHand) {
		timer := time.NewTimer(cmp.Or(m.retime.poll, quietPoll))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// timeAgain times the GPU chosen and the processor's tone mapping, unless
// a timing ended since began, and puts what they find in place; work
// starting meanwhile stops it with errWorkStarted. A busy timing is made
// again later (see retryLater).
func (m *Manager) timeAgain(parent context.Context, began int64) error {
	m.retime.run.Lock()
	defer m.retime.run.Unlock()
	if m.retime.timed.Load() > began {
		return nil
	}
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	go func() {
		ticker := time.NewTicker(cmp.Or(m.retime.poll, quietPoll))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if m.working() {
					cancel(errWorkStarted)
					return
				}
			}
		}
	}()
	m.detecting.Lock()
	want := m.wanted
	m.detecting.Unlock()
	current := m.Hardware()
	var base, measured Hardware
	var found chainTimings
	if current != nil {
		base = current.untimed()
		measured, found = m.measure(ctx, base)
	}
	var speed float64
	var timed bool
	if m.HasFilters("zscale", "tonemap") {
		_, speed, timed = m.timeToneMapping(ctx, current)
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	busy := false
	if current != nil {
		final, gpuBusy := m.settleGPU(base, measured, found)
		m.putTimed(want, current, &final)
		busy = gpuBusy
	}
	if timed {
		speed, processorBusy := m.settleProcessor(current, speed)
		m.setProcessorSpeed(speed)
		busy = busy || processorBusy
	}
	m.retime.timed.Store(m.retime.now().UnixNano())
	m.logHDRPath()
	if busy {
		m.retryLater()
	} else {
		m.stopRetry()
	}
	if m.retime.applied != nil {
		m.retime.applied()
	}
	return nil
}

// retryLater times again, once quiet, 30 to 60 minutes after a timing
// made while the server was busy, unless that is due already.
func (m *Manager) retryLater() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.retime.retry != nil {
		return
	}
	delay := retryAfter + rand.N(retryJitter)
	m.logger.Info("Timing again once the server is quiet", "after", delay.Round(time.Minute))
	run := func() {
		m.mu.Lock()
		m.retime.retry = nil
		if m.closed {
			m.mu.Unlock()
			return
		}
		m.measuring.Add(1)
		m.mu.Unlock()
		defer m.measuring.Done()
		_ = m.Retime(context.Background(), false)
	}
	if m.retime.later != nil {
		m.retime.retry = m.retime.later(delay, run)
		return
	}
	m.retime.retry = time.AfterFunc(delay, run).Stop
}

// stopRetry forgets the retry due, once a timing was not busy.
func (m *Manager) stopRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retime.retry != nil {
		m.retime.retry()
		m.retime.retry = nil
	}
}

// settleGPU is base, the GPU as detected, converting as a timing that
// found found says, measured being what measure made of it: with timings
// kept (see UseTimings), the best of the last 7 days, found among them
// unless a chain timed under 60 % of its best, which tells a busy server
// and leaves found out; else measured itself. It reports a busy server.
func (m *Manager) settleGPU(base, measured Hardware, found chainTimings) (Hardware, bool) {
	if !m.timings.on() {
		return measured, false
	}
	key, now := m.timingKey(&base), m.retime.now()
	best, _ := m.timings.best(m.logger, key, now)
	if below := found.below(best); len(below) > 0 {
		m.logger.Info("Timed while the server was busy: kept the best recent timings", "timed", "gpu", "below", below)
		return best.apply(base), true
	}
	m.timings.add(m.logger, key, found, now)
	best, _ = m.timings.best(m.logger, key, now)
	return best.apply(base), false
}

// settleProcessor is how fast the processor tone maps HDR on hw, nil for
// no GPU, once timed at speed, 0 when it failed, as settleGPU makes it
// for a GPU. It reports a busy server.
func (m *Manager) settleProcessor(hw *Hardware, speed float64) (float64, bool) {
	if !m.timings.on() {
		return speed, false
	}
	key, now := m.timingKey(hw), m.retime.now()
	found := chainTimings{Speeds: map[string]float64{chainProcessor: speed}}
	best, _ := m.timings.best(m.logger, key, now)
	if below := found.below(best); len(below) > 0 {
		m.logger.Info("Timed while the server was busy: kept the best recent timings", "timed", "processor", "below", below)
		return best.Speeds[chainProcessor], true
	}
	m.timings.add(m.logger, key, found, now)
	best, _ = m.timings.best(m.logger, key, now)
	return best.Speeds[chainProcessor], false
}

// applyBest puts the best recent timings kept for hw, nil for no GPU, in
// force at once, the processor's included, and returns hw with them; hw
// itself when none is kept.
func (m *Manager) applyBest(hw *Hardware) *Hardware {
	if !m.timings.on() {
		return hw
	}
	best, n := m.timings.best(m.logger, m.timingKey(hw), m.retime.now())
	if n == 0 {
		return hw
	}
	attrs := []any{"timings", n}
	if speed := best.Speeds[chainProcessor]; speed > 0 {
		m.setProcessorSpeed(speed)
		attrs = append(attrs, "processor_speed", speedText(speed), "tone_mapped_height", m.ToneMappedHeight())
	}
	if hw != nil {
		applied := best.apply(*hw)
		hw = &applied
		attrs = append(attrs, "sdr_chain", chainName(hw.Resident))
		if hw.ToneMapping {
			attrs = append(attrs, "vulkan_decoding", hw.VulkanDecoding, "hdr_speed", speedText(hw.HDRSpeed), "low_speed", speedText(hw.LowHDRSpeed))
		}
	}
	m.logger.Info("Applied the best recent timings", attrs...)
	return hw
}

// putTimed puts timed in place of hw, the GPU chosen for want, as the
// choice of want and as the GPU chosen, unless another was chosen
// meanwhile, and reports whether it did.
func (m *Manager) putTimed(want string, hw, timed *Hardware) bool {
	m.detecting.Lock()
	if m.detected[want] == hw {
		m.detected[want] = timed
	}
	m.detecting.Unlock()
	return m.hardware.CompareAndSwap(hw, timed)
}

// setProcessorSpeed keeps how fast the processor tone maps HDR, and the
// height Automatic caps it at (see ToneMappedHeight).
func (m *Manager) setProcessorSpeed(speed float64) {
	height := LowToneMappedHeight
	if speed >= fastToneMapping {
		height = HighToneMappedHeight
	}
	m.toneMappingSpeed.Store(math.Float64bits(speed))
	m.toneMappedHeight.Store(int32(height))
}

// chainName names the chain kept: gpu, or memory.
func chainName(gpu bool) string {
	if gpu {
		return "gpu"
	}
	return "memory"
}
