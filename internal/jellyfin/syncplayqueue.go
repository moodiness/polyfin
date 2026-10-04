package jellyfin

import (
	"math/rand/v2"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The repeat modes of a SyncPlay queue, in the order of Jellyfin's
// GroupRepeatMode values.
var repeatModes = []string{"RepeatOne", "RepeatAll", "RepeatNone"}

// The shuffle modes of a SyncPlay queue, in the order of Jellyfin's
// GroupShuffleMode values.
var shuffleModes = []string{"Sorted", "Shuffle"}

// queueEntry is an item of a SyncPlay queue. The same item may be queued
// twice: each entry has an identifier of its own, which apps name it by.
type queueEntry struct {
	item, entry accounts.ID
}

// playQueue is what a SyncPlay group plays, as Jellyfin's play queue
// manager keeps it: the items in the order they were queued, and their
// shuffled order while shuffling, which the playing index then refers to.
type playQueue struct {
	sorted, shuffled []queueEntry
	// playing indexes the entry played in the current order, -1 for none.
	playing int
	shuffle bool
	repeat  string
	changed time.Time
	// additions counts the times items were queued, which tells whether
	// the items checked while the group was not held are all there are.
	additions int
}

func newPlayQueue() playQueue {
	q := playQueue{}
	q.reset()
	return q
}

func (q *playQueue) reset() {
	q.sorted, q.shuffled = nil, nil
	q.playing = -1
	q.shuffle = false
	q.repeat = "RepeatNone"
	q.changed = time.Now()
}

// entries are the entries in the order they play.
func (q *playQueue) entries() []queueEntry {
	if q.shuffle {
		return q.shuffled
	}
	return q.sorted
}

func (q *playQueue) setEntries(entries []queueEntry) {
	if q.shuffle {
		q.shuffled = entries
	} else {
		q.sorted = entries
	}
}

func (q *playQueue) current() (queueEntry, bool) {
	if q.playing < 0 {
		return queueEntry{}, false
	}
	return q.entries()[q.playing], true
}

// playingEntry is the identifier of the entry played, zero for none.
func (q *playQueue) playingEntry() accounts.ID {
	current, _ := q.current()
	return current.entry
}

// items lists the items queued, in the order they play.
func (q *playQueue) items() []accounts.ID {
	items := make([]accounts.ID, 0, len(q.sorted))
	for _, e := range q.entries() {
		items = append(items, e.item)
	}
	return items
}

// newEntries makes entries of items about to be queued.
func (q *playQueue) newEntries(items []accounts.ID) []queueEntry {
	q.additions++
	entries := make([]queueEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, queueEntry{item: item, entry: randomGUID()})
	}
	return entries
}

func shuffled(entries []queueEntry) []queueEntry {
	entries = slices.Clone(entries)
	rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
	return entries
}

// set replaces the items, none of them playing.
func (q *playQueue) set(items []accounts.ID) {
	q.sorted = q.newEntries(items)
	q.shuffled = nil
	if q.shuffle {
		q.shuffled = shuffled(q.sorted)
	}
	q.playing = -1
	q.changed = time.Now()
}

// add queues items at the end.
func (q *playQueue) add(items []accounts.ID) {
	entries := q.newEntries(items)
	q.sorted = append(q.sorted, entries...)
	if q.shuffle {
		q.shuffled = append(q.shuffled, entries...)
	}
	q.changed = time.Now()
}

// addNext queues items right after the one playing, or first when none is.
func (q *playQueue) addNext(items []accounts.ID) {
	entries := q.newEntries(items)
	if q.shuffle {
		current, _ := q.current()
		at := slices.Index(q.sorted, current)
		if q.playing < 0 {
			at = -1
		}
		q.sorted = slices.Insert(q.sorted, at+1, entries...)
		q.shuffled = slices.Insert(q.shuffled, q.playing+1, entries...)
	} else {
		q.sorted = slices.Insert(q.sorted, q.playing+1, entries...)
	}
	q.changed = time.Now()
}

// setShuffle shuffles the entries, the one playing first, or restores
// their order, keeping the one playing.
func (q *playQueue) setShuffle(shuffle bool) {
	if shuffle {
		if current, ok := q.current(); ok {
			rest := slices.Delete(slices.Clone(q.entries()), q.playing, q.playing+1)
			q.shuffled = append([]queueEntry{current}, shuffled(rest)...)
			q.playing = 0
		} else {
			q.shuffled = shuffled(q.sorted)
		}
		q.shuffle = true
	} else {
		// A playlist already sorted has no shuffled order to map back.
		if current, ok := q.current(); ok && len(q.shuffled) > 0 {
			q.playing = slices.Index(q.sorted, current)
		}
		q.shuffled = nil
		q.shuffle = false
	}
	q.changed = time.Now()
}

func (q *playQueue) setRepeat(mode string) {
	q.repeat = mode
	q.changed = time.Now()
}

// clear empties the queue, but for the entry playing if it is kept.
func (q *playQueue) clear(keepPlaying bool) {
	current, playing := q.current()
	q.sorted, q.shuffled = nil, nil
	q.changed = time.Now()
	if keepPlaying && playing {
		q.sorted = []queueEntry{current}
		if q.shuffle {
			q.shuffled = []queueEntry{current}
		}
		q.playing = 0
		return
	}
	q.playing = -1
}

// playItem plays the first entry of an item, if any.
func (q *playQueue) playItem(item accounts.ID) {
	q.playing = slices.IndexFunc(q.entries(), func(e queueEntry) bool { return e.item == item })
	q.changed = time.Now()
}

// playEntry plays an entry, and reports whether it was found.
func (q *playQueue) playEntry(entry accounts.ID) bool {
	q.playing = slices.IndexFunc(q.entries(), func(e queueEntry) bool { return e.entry == entry })
	q.changed = time.Now()
	return q.playing >= 0
}

// playIndex plays the entry at index, none when out of range.
func (q *playQueue) playIndex(index int) {
	q.playing = -1
	if index >= 0 && index < len(q.entries()) {
		q.playing = index
	}
	q.changed = time.Now()
}

// remove takes entries out, and reports whether the one playing was. The
// entry before it plays then, or the first one.
func (q *playQueue) remove(entries []accounts.ID) bool {
	listed := make(map[accounts.ID]bool, len(entries))
	for _, entry := range entries {
		listed[entry] = true
	}
	removed := func(e queueEntry) bool { return listed[e.entry] }
	current, playing := q.current()
	before := 0
	if playing {
		for _, e := range q.entries()[:q.playing] {
			if removed(e) {
				before++
			}
		}
	}
	q.sorted = slices.DeleteFunc(q.sorted, removed)
	q.shuffled = slices.DeleteFunc(q.shuffled, removed)
	q.changed = time.Now()
	if !playing {
		return false
	}
	if removed(current) {
		q.playing -= before + 1
		if q.playing < 0 {
			q.playing = -1
			if len(q.entries()) > 0 {
				q.playing = 0
			}
		}
		return true
	}
	q.playEntry(current.entry)
	return false
}

// move moves an entry to index, within bounds, and reports whether it was
// found.
func (q *playQueue) move(entry accounts.ID, index int) bool {
	entries := q.entries()
	from := slices.IndexFunc(entries, func(e queueEntry) bool { return e.entry == entry })
	if from < 0 {
		return false
	}
	current, playing := q.current()
	moved := entries[from]
	entries = slices.Delete(entries, from, from+1)
	entries = slices.Insert(entries, min(max(index, 0), len(entries)), moved)
	q.setEntries(entries)
	q.changed = time.Now()
	q.playing = -1
	if playing {
		q.playing = slices.Index(entries, current)
	}
	return true
}

// next plays the next entry, the same one again when repeating one, or the
// first after the last when repeating all. It reports whether an entry
// starts.
func (q *playQueue) next() bool {
	return q.skip(1)
}

// previous plays the previous entry, as next does the next one.
func (q *playQueue) previous() bool {
	return q.skip(-1)
}

func (q *playQueue) skip(step int) bool {
	count := len(q.entries())
	if count == 0 {
		return false
	}
	if q.repeat == "RepeatOne" {
		q.changed = time.Now()
		return true
	}
	q.playing += step
	if q.playing < 0 || q.playing >= count {
		repeatAll := q.repeat == "RepeatAll"
		switch {
		case step > 0 && repeatAll:
			q.playing = 0
		case step < 0 && repeatAll:
			q.playing = count - 1
		default:
			q.playing = min(max(q.playing, 0), count-1)
			return false
		}
	}
	q.changed = time.Now()
	return true
}

// update describes the queue to apps, as Jellyfin's PlayQueueUpdate.
func (q *playQueue) update(reason string, start int64, playing bool) PlayQueueUpdate {
	playlist := make([]SyncPlayQueueItem, 0, len(q.entries()))
	for _, e := range q.entries() {
		playlist = append(playlist, SyncPlayQueueItem{ItemId: e.item.String(), PlaylistItemId: e.entry.String()})
	}
	shuffle := shuffleModes[0]
	if q.shuffle {
		shuffle = shuffleModes[1]
	}
	return PlayQueueUpdate{Reason: reason, LastUpdate: Time(q.changed), Playlist: playlist, PlayingItemIndex: q.playing,
		StartPositionTicks: start, IsPlaying: playing, ShuffleMode: shuffle, RepeatMode: q.repeat}
}
