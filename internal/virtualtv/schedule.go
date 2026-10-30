package virtualtv

import (
	"slices"
	"strconv"
	"time"
)

// maxAirings bounds what one listing returns: a week of one-minute entries
// is about ten thousand.
const maxAirings = 50_000

// Airing is an entry on the air, from Start to End.
type Airing struct {
	Entry      Entry
	Start, End time.Time
}

// ProgrammeID identifies an airing among its channel's: by its start and
// its item, so that a programme whose channel was rebuilt since is
// another one.
func (a Airing) ProgrammeID() string {
	return strconv.FormatInt(a.Start.Unix(), 10) + ":" + a.Entry.Item.String()
}

// Airings lists what the channel airs over [from, to), by start, each
// airing that overlaps it whole. The schedule is a pure function of the
// channel's epoch, seed, entries and blocks: the same listing gives the
// same airings, whenever and wherever it is computed. It starts at the
// epoch and has no gaps.
//
// Without blocks, the entries air one after the other from the epoch on,
// round after round, each round in their order, or shuffled anew for each
// round by the seed: a round lasts as long as its entries, whatever their
// order, so the airing of any moment is found without going through the
// rounds before it.
//
// With blocks, each day, in loc, airs each block's entries from its start
// to its end, and the channel's own entries in the times between. Each
// block's entries, and the channel's, go on from where they stopped the
// day before. An entry the end of its time cuts is cut there, and airs in
// full at the start of the next time of its entries, unless it started
// that time: then the next entry follows, so that a time shorter than an
// entry does not air the same one every day.
func (c Channel) Airings(from, to time.Time, loc *time.Location) []Airing {
	if from.Before(c.Epoch) {
		from = c.Epoch
	}
	if !to.After(from) || len(c.Entries) == 0 {
		return nil
	}
	if len(c.Blocks) == 0 {
		return newStream(c.Entries, c.Shuffle, c.Seed, 0).continuous(c.Epoch, from, to)
	}
	return c.daily(from, to, loc)
}

// AiringAt is what the channel airs at t, false before its epoch.
func (c Channel) AiringAt(t time.Time, loc *time.Location) (Airing, bool) {
	for _, a := range c.Airings(t, t.Add(time.Nanosecond), loc) {
		if !a.Start.After(t) && a.End.After(t) {
			return a, true
		}
	}
	return Airing{}, false
}

// stream is a list of entries aired round after round: in their order, or
// in each round's shuffle (see order).
type stream struct {
	entries []Entry
	shuffle bool
	seed    uint64
	cycle   time.Duration
	// round, pos and orderOf follow a stream through its times (see
	// daily): the entry next is orderOf[pos] of round.
	round   int64
	pos     int
	orderOf []int
}

func newStream(entries []Entry, shuffle bool, seed int64, salt uint64) *stream {
	s := &stream{entries: entries, shuffle: shuffle, seed: mix(uint64(seed) ^ mix(salt))}
	for _, e := range entries {
		s.cycle += e.Runtime
	}
	s.orderOf = s.order(0)
	return s
}

// order is the order of the entries in a round: theirs, or shuffled by
// the seed and the round with Fisher and Yates's shuffle, whose numbers
// come from SplitMix64, the same on every machine.
func (s *stream) order(round int64) []int {
	order := make([]int, len(s.entries))
	for i := range order {
		order[i] = i
	}
	if !s.shuffle {
		return order
	}
	state := s.seed ^ mix(uint64(round))
	for i := len(order) - 1; i > 0; i-- {
		state = mix(state)
		j := int(state % uint64(i+1))
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// mix is SplitMix64's step.
func mix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// continuous lists the airings over [from, to) of entries aired one after
// the other from epoch on, from not before epoch.
func (s *stream) continuous(epoch, from, to time.Time) []Airing {
	if s.cycle <= 0 {
		return nil
	}
	round := int64(from.Sub(epoch) / s.cycle)
	cursor := epoch.Add(time.Duration(round) * s.cycle)
	var airings []Airing
	for ; cursor.Before(to) && len(airings) < maxAirings; round++ {
		for _, i := range s.order(round) {
			e := s.entries[i]
			end := cursor.Add(e.Runtime)
			if end.After(from) {
				airings = append(airings, Airing{Entry: e, Start: cursor, End: end})
			}
			cursor = end
			if !cursor.Before(to) {
				break
			}
		}
	}
	return airings
}

// next is the entry the stream airs next.
func (s *stream) next() Entry { return s.entries[s.orderOf[s.pos]] }

// advance moves the stream to its next entry, into the next round after
// the last.
func (s *stream) advance() {
	if s.pos++; s.pos == len(s.orderOf) {
		s.round++
		s.pos = 0
		s.orderOf = s.order(s.round)
	}
}

// slot is a daily time of a stream, in minutes past midnight.
type slot struct {
	start, end int
	stream     *stream
}

// daily lists the airings over [from, to) of a channel with blocks (see
// Airings), going through each day from the epoch's on, as each time
// starts where its entries stopped the day before.
func (c Channel) daily(from, to time.Time, loc *time.Location) []Airing {
	base := newStream(c.Entries, c.Shuffle, c.Seed, 0)
	blocks := slices.Clone(c.Blocks)
	slices.SortFunc(blocks, func(a, b Block) int { return a.Start - b.Start })
	var slots []slot
	previous := 0
	for _, b := range blocks {
		if b.Start > previous {
			slots = append(slots, slot{start: previous, end: b.Start, stream: base})
		}
		// A block's shuffle is salted by its start, which does not change
		// when other blocks are added.
		slots = append(slots, slot{start: b.Start, end: b.End, stream: newStream(b.Entries, b.Shuffle, c.Seed, uint64(b.Start)+1)})
		previous = b.End
	}
	if previous < minutesPerDay {
		slots = append(slots, slot{start: previous, end: minutesPerDay, stream: base})
	}
	var airings []Airing
	day := DayStart(c.Epoch, loc)
	ended := day
	for ; day.Before(to) && len(airings) < maxAirings; day = day.AddDate(0, 0, 1) {
		for _, sl := range slots {
			// Minutes past midnight are read as the clock shows them, which
			// a change of time may skip or repeat: a time never starts
			// before the previous one ended.
			start := time.Date(day.Year(), day.Month(), day.Day(), 0, sl.start, 0, 0, loc)
			end := time.Date(day.Year(), day.Month(), day.Day(), 0, sl.end, 0, 0, loc)
			if start.Before(ended) {
				start = ended
			}
			if !end.After(start) {
				continue
			}
			ended = end
			if !start.Before(to) {
				break
			}
			airings = sl.stream.fill(airings, start, end, from, to)
		}
	}
	return airings
}

// fill airs the stream's entries over the time [start, end), adding to
// airings those that overlap [from, to): the entry the end cuts airs in
// full in the stream's next time, unless it started this one.
func (s *stream) fill(airings []Airing, start, end, from, to time.Time) []Airing {
	cursor := start
	for first := true; cursor.Before(end); first = false {
		e := s.next()
		finish := cursor.Add(e.Runtime)
		cut := finish.After(end)
		if cut {
			finish = end
		}
		if finish.After(from) && cursor.Before(to) {
			airings = append(airings, Airing{Entry: e, Start: cursor, End: finish})
		}
		if !cut || first {
			s.advance()
		}
		cursor = finish
	}
	return airings
}
