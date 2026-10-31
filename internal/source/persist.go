package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// With a policy that keeps them (see Policy.Keep), the cache keeps the
// blocks of the sources it read across restarts. Each source's folder,
// named after the version it reads, holds beside its chunks a state file
// telling which of their blocks are the file's, its size, the digest of
// the link they were read from, when each chunk was last read, and how
// often the source was played. The state claims only blocks whose chunk
// reached the disk before it was written (see save), and never a chunk
// evicted (see unclaim): a block it claims after a crash is the file's.
// The next start reads the states back (see load); a folder without one,
// or with one it cannot read, is removed.

// Policy is how much the cache keeps of the sources, and for how long.
type Policy struct {
	// Limit is the bytes the cache may keep.
	Limit int64
	// Keep is how long a chunk is kept once last read, across restarts.
	// Zero keeps nothing across restarts: each start empties the cache.
	Keep time.Duration
	// Credit is how much longer a chunk of a source played again is kept,
	// and how much later it is evicted, for each play after the first
	// (see replayGap), maxCredits at most.
	Credit time.Duration
}

const (
	// stateFile names the state file in a source's folder.
	stateFile = "state"
	// saveDelay is how long after a change the state of a source is
	// saved: a crash loses at most what was read meanwhile.
	saveDelay = 30 * time.Second
	// sweepEvery spaces the sweeps of the chunks kept past the policy's
	// Keep.
	sweepEvery = time.Hour
	// replayGap is how long after the last playback of a source the next
	// one counts as a play again.
	replayGap = 6 * time.Hour
	// maxCredits bounds the plays a source is credited for.
	maxCredits = 8
)

// keptState is what the cache keeps of a source across restarts.
type keptState struct {
	// Link is the digest of the address the blocks were read from (see
	// linkDigest), never the address itself.
	Link   string `json:"link"`
	Size   int64  `json:"size"`
	Type   string `json:"type,omitempty"`
	Status int    `json:"status,omitempty"`
	// Blocks has a bit set for each block cached, as Source.present.
	Blocks []uint64    `json:"blocks"`
	Chunks []keptChunk `json:"chunks"`
	Plays  int         `json:"plays,omitempty"`
	Played time.Time   `json:"played,omitzero"`
}

// keptChunk tells when a chunk was last read.
type keptChunk struct {
	Index int64     `json:"index"`
	Read  time.Time `json:"read"`
}

// linkDigest is the digest of a source's address, which tells whether a
// location names the link the blocks kept were read from without writing
// the address, which may hold credentials, to disk.
func linkDigest(link string) string {
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:])
}

// setBit and clearBit set and clear a block's bit in a bit set.
func setBit(bits []uint64, block int64) []uint64 {
	word := block / 64
	if grow := word + 1 - int64(len(bits)); grow > 0 {
		bits = append(bits, make([]uint64, grow)...)
	}
	bits[word] |= 1 << (block % 64)
	return bits
}

func clearBit(bits []uint64, block int64) {
	if word := block / 64; word < int64(len(bits)) {
		bits[word] &^= 1 << (block % 64)
	}
}

func hasBit(bits []uint64, block int64) bool {
	word := block / 64
	return word < int64(len(bits)) && bits[word]&(1<<(block%64)) != 0
}

// load reads back the sources a previous run kept in the cache's folder
// when the policy keeps them, and removes them otherwise. A chunk past the
// policy's Keep is removed, and so are chunks past the limit, those whose
// score is the oldest first.
func (c *Cache) load() error {
	folders, err := filepath.Glob(filepath.Join(c.dir, "*.blocks"))
	if err != nil {
		return err
	}
	keep := c.policy().Keep > 0
	for _, folder := range folders {
		if keep {
			id, err := accounts.ParseID(strings.TrimSuffix(filepath.Base(folder), ".blocks"))
			if err == nil {
				if err = c.readBack(folder, id); err == nil {
					continue
				}
			}
			c.logger.Debug("A source kept by the last run could not be read back", "folder", filepath.Base(folder), "error", err)
		}
		if err := os.RemoveAll(folder); err != nil {
			return err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expire()
	c.fit()
	return nil
}

// errNothingKept reports a source folder holding no block the cache can
// trust.
var errNothingKept = errors.New("nothing kept")

// readBack reads back the source of id that folder keeps: the blocks its
// state claims whose chunk is there, long enough to hold them. Chunks it
// does not claim are removed, and a state that changed is written again.
func (c *Cache) readBack(folder string, id accounts.ID) error {
	data, err := os.ReadFile(filepath.Join(folder, stateFile))
	if err != nil {
		return err
	}
	var state keptState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Size <= 0 || state.Link == "" {
		return errNothingKept
	}
	read := map[int64]time.Time{}
	for _, chunk := range state.Chunks {
		read[chunk.Index] = chunk.Read
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}
	blocks := make([]uint64, len(state.Blocks))
	chunks := map[int64]int64{}
	changed := false
	for _, entry := range entries {
		index, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil || index < 0 {
			if entry.Name() != stateFile {
				changed = true
				_ = os.RemoveAll(filepath.Join(folder, entry.Name()))
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var size int64
		for block := index * c.chunkBlocks; block < (index+1)*c.chunkBlocks; block++ {
			if !hasBit(state.Blocks, block) {
				continue
			}
			length := min(blockSize, state.Size-block*blockSize)
			// A chunk cut short holds the blocks before the cut alone.
			if length <= 0 || (block%c.chunkBlocks)*blockSize+length > info.Size() {
				changed = true
				continue
			}
			blocks = setBit(blocks, block)
			size += length
		}
		if size == 0 {
			changed = true
			if err := os.Remove(filepath.Join(folder, entry.Name())); err != nil {
				return err
			}
			continue
		}
		chunks[index] = size
		if _, ok := read[index]; !ok {
			read[index] = info.ModTime()
		}
	}
	if len(chunks) == 0 {
		return errNothingKept
	}
	if !slices.Equal(blocks, state.Blocks) {
		changed = true
	}
	state.Blocks = blocks
	state.Chunks = state.Chunks[:0]
	for index := range chunks {
		state.Chunks = append(state.Chunks, keptChunk{Index: index, Read: read[index]})
	}
	if changed {
		if err := writeState(folder, &state); err != nil {
			return err
		}
	}
	s := c.newSource(id)
	s.pending, s.kept, s.saved = slices.Clone(blocks), state, &state
	s.plays, s.playedAt = state.Plays, state.Played
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources[id] = s
	for index, size := range chunks {
		c.chunks[chunkKey{id, index}] = &chunkUse{size: size, touched: read[index]}
		c.used += size
	}
	return nil
}

// restore makes the blocks a previous run kept of the source its own once
// a location opens it at the link they were read from: they are read
// without a request. Opened at another link, as addons hand out new ones,
// they wait for the first answer to tell the file's size (see learn). The
// source's lock is held.
func (s *Source) restore() {
	if s.pending != nil && s.link == s.kept.Link {
		s.adopt()
	}
}

// adopt makes the blocks a previous run kept of the source its own, with
// what that run learned of it: the readers waiting for one of them get it
// at once. The source's lock is held.
func (s *Source) adopt() {
	s.present, s.pending = s.pending, nil
	s.size = s.kept.Size
	if s.contentType == "" {
		s.contentType = s.kept.Type
	}
	s.sizeChecked, s.sizeNoted = true, true
	if s.status == 0 && s.kept.Status != 0 {
		s.status = s.kept.Status
		close(s.learned)
	}
	for block, w := range s.wanted {
		if s.has(block) {
			delete(s.wanted, block)
			close(w.done)
		}
	}
	s.wakePastEnd()
	s.signal()
}

// dropPending removes the chunks of the blocks a previous run kept of the
// source, of another file than the one its link names now, and tells
// them. No block was stored since they were read back: blocks are stored
// once an answer told the size, and this is the first one (see learn).
// The source's lock is held.
func (s *Source) dropPending() []int64 {
	var chunks []int64
	for word, bits := range s.pending {
		for bit := range int64(64) {
			if bits&(1<<bit) == 0 {
				continue
			}
			index := (int64(word)*64 + bit) / s.cache.chunkBlocks
			if !slices.Contains(chunks, index) {
				chunks = append(chunks, index)
			}
		}
	}
	s.pending = nil
	for _, index := range chunks {
		if err := os.Remove(s.chunkPath(index)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			s.cache.logger.Warn("A cached chunk could not be removed", "error", err)
		}
	}
	s.cache.logger.Info("A source's link now names a file of another size: the blocks kept of it were removed", "source", s.id)
	return chunks
}

// dropKept stops counting the chunks dropPending removed, and the state
// file that claimed them.
func (c *Cache) dropKept(s *Source, chunks []int64) {
	c.mu.Lock()
	for _, index := range chunks {
		key := chunkKey{s.id, index}
		if use := c.chunks[key]; use != nil {
			delete(c.chunks, key)
			c.used -= use.size
		}
	}
	c.mu.Unlock()
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.saved = nil
	if err := os.Remove(filepath.Join(s.dir, stateFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.logger.Warn("A cached source's state could not be removed", "error", err)
	}
}

// played counts a playback of the source starting, as one more play when
// it comes replayGap or more after the last one.
func (c *Cache) played(s *Source) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if s.plays == 0 || now.Sub(s.playedAt) >= replayGap {
		s.plays++
	}
	s.playedAt = now
}

// score is when a chunk counts as last read for eviction and expiry: when
// it was, credited for the plays of its source after the first, so that
// a title played again stays longer than one played once. The cache's
// lock is held.
func (c *Cache) score(key chunkKey, use *chunkUse, credit time.Duration) time.Time {
	s := c.sources[key.source]
	if s == nil || s.plays < 2 || credit <= 0 {
		return use.touched
	}
	return use.touched.Add(time.Duration(min(s.plays-1, maxCredits)) * credit)
}

// expire evicts the chunks kept past the policy's Keep, credit included,
// but those read lately. The cache's lock is held.
func (c *Cache) expire() {
	policy := c.policy()
	if policy.Keep <= 0 {
		return
	}
	now := c.now()
	protected, deadline := now.Add(-protectedFor), now.Add(-policy.Keep)
	for key, use := range c.chunks {
		if use.touched.Before(protected) && c.score(key, use, policy.Credit).Before(deadline) {
			c.evict(key, use)
		}
	}
}

// sweep expires chunks every sweepEvery, until stop is closed.
func (c *Cache) sweep(stop <-chan struct{}) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			c.expire()
			c.mu.Unlock()
		}
	}
}

// saveLater has the source's state saved in saveDelay, unless a save is
// due already, while the policy keeps blocks and the source is not
// transient. The source's lock is held.
func (c *Cache) saveLater(s *Source) {
	if s.saveTimer != nil || s.transient || s.ctx.Err() != nil || c.policy().Keep <= 0 {
		return
	}
	s.saveTimer = time.AfterFunc(saveDelay, func() { c.save(s) })
}

// save saves the source's state, once the chunks written since the last
// save reached the disk. A chunk evicted meanwhile has the save made
// again later, as the state read would claim it (see unclaim).
func (c *Cache) save(s *Source) {
	s.mu.Lock()
	s.saveTimer = nil
	s.mu.Unlock()
	c.mu.Lock()
	if c.sources[s.id] != s {
		c.mu.Unlock()
		return
	}
	state, unsynced, ok := c.snapshot(s)
	evictions := s.evictions.Load()
	c.mu.Unlock()
	if !ok {
		return
	}
	s.writing.RLock()
	defer s.writing.RUnlock()
	if s.ctx.Err() != nil {
		return
	}
	err := syncChunks(s, unsynced)
	again := false
	if err == nil {
		s.saveMu.Lock()
		if again = s.evictions.Load() != evictions; !again {
			if err = writeState(s.dir, state); err == nil {
				s.saved = state
			}
		}
		s.saveMu.Unlock()
	}
	if err != nil || again {
		if err != nil {
			c.logger.Warn("A cached source's state could not be saved", "error", err)
		}
		s.mu.Lock()
		for _, index := range unsynced {
			s.unsynced[index] = true
		}
		c.saveLater(s)
		s.mu.Unlock()
	}
}

// snapshot is the state to save of the source, and the chunks written
// since the last save, which it no longer counts as such; not ok for a
// source not kept: transient, of an unknown size, or whose kept blocks
// still wait for their link (see restore), the state saved staying. The
// cache's lock is held.
func (c *Cache) snapshot(s *Source) (state *keptState, unsynced []int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transient || s.size <= 0 || s.pending != nil || s.link == "" {
		return nil, nil, false
	}
	state = &keptState{Link: s.link, Size: s.size, Type: s.contentType, Status: s.status, Blocks: slices.Clone(s.present),
		Plays: s.plays, Played: s.playedAt}
	for key, use := range c.chunks {
		if key.source == s.id {
			state.Chunks = append(state.Chunks, keptChunk{Index: key.index, Read: use.touched})
		}
	}
	for index := range s.unsynced {
		unsynced = append(unsynced, index)
	}
	clear(s.unsynced)
	return state, unsynced, true
}

// syncChunks has the chunks of the source written lately reach the disk.
// A chunk evicted meanwhile is gone, and claimed no more.
func syncChunks(s *Source, chunks []int64) error {
	for _, index := range chunks {
		file, err := os.OpenFile(s.chunkPath(index), os.O_RDWR, 0)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		err = file.Sync()
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// writeState writes a source's state into its folder, whole or not at all.
func writeState(folder string, state *keptState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temporary := filepath.Join(folder, stateFile+".tmp")
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, filepath.Join(folder, stateFile))
}

// unclaim has the source's state no longer claim a chunk about to be
// evicted, before its file is removed: a new chunk written in its place
// would otherwise be read, after a crash, as the blocks of the old one.
// The cache's lock is held.
func (c *Cache) unclaim(s *Source, index int64) {
	s.evictions.Add(1)
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.saved == nil {
		return
	}
	state := *s.saved
	state.Blocks = slices.Clone(state.Blocks)
	for block := index * c.chunkBlocks; block < (index+1)*c.chunkBlocks; block++ {
		clearBit(state.Blocks, block)
	}
	state.Chunks = slices.DeleteFunc(slices.Clone(state.Chunks), func(chunk keptChunk) bool { return chunk.Index == index })
	if len(state.Chunks) == 0 {
		s.saved = nil
		if err := os.Remove(filepath.Join(s.dir, stateFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			c.logger.Warn("A cached source's state could not be removed", "error", err)
		}
		return
	}
	if err := writeState(s.dir, &state); err != nil {
		// Claiming nothing is safe; claiming the chunk is not.
		s.saved = nil
		_ = os.Remove(filepath.Join(s.dir, stateFile))
		return
	}
	s.saved = &state
}

// keep stops reading the source as the cache closes, and keeps its blocks
// for the next start: its state is saved once its chunks reached the
// disk. A source not kept (see snapshot) has its blocks removed, but one
// whose kept blocks still wait for their link, whose state stays. The
// cache's lock is held.
func (c *Cache) keep(s *Source) error {
	s.halt()
	if s.received != nil {
		c.received.Release(s.received)
	}
	defer s.writing.Unlock()
	state, unsynced, ok := c.snapshot(s)
	if !ok {
		s.mu.Lock()
		waiting := s.pending != nil
		s.mu.Unlock()
		if waiting {
			return nil
		}
		return os.RemoveAll(s.dir)
	}
	if len(state.Chunks) == 0 {
		return os.RemoveAll(s.dir)
	}
	if err := syncChunks(s, unsynced); err != nil {
		return fmt.Errorf("keep a cached source: %w", err)
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if err := writeState(s.dir, state); err != nil {
		return fmt.Errorf("keep a cached source: %w", err)
	}
	s.saved = state
	return nil
}
