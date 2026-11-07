package source

import (
	"cmp"
	"net/url"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A host whose service is down may answer every link with the same short
// video, an error file, instead of the file asked: told by one small size
// answered for different files, each far over it.
const (
	// errorMost is the largest answer taken for an error file, and
	// errorShare how far under a file's size it must be: under a tenth.
	// The error video of a host seen down was 1,822,520 bytes, for files
	// announced at 300 MB to 8 GB. On a production server, 1,189 analyzed
	// files: none under 105 MB; and the same size is often announced for
	// several versions, a release listed twice or a pack's size on its
	// episodes, which then all answer the same real episode. Both bounds
	// keep such an episode, hundreds of megabytes under a pack's size,
	// from ever being taken for an error file.
	errorMost  = 50_000_000
	errorShare = 10
	// errorRepeat is how many different files answered with the same such
	// size make it the host's error file: refused, and never counted
	// against the sizes an addon announces. Two files of the very same
	// size, under a tenth of their own and too small for a video, are no
	// coincidence.
	errorRepeat = 2
	// errorFiles is how many make it told, on Health and on the versions
	// of the host: one more than errorRepeat, as two versions may be the
	// same file, listed twice with a size announced wrong.
	errorFiles = 3
	// errorWindow is how close together those answers come: the minutes
	// a play takes to try a title's versions, or a few titles'. Answers
	// further apart tell links that changed files, one at a time.
	errorWindow = 10 * time.Minute
	// errorMemory is how long a host told answering an error file stays
	// told without answering it again: once nothing is asked of it,
	// nothing tells whether it recovered.
	errorMemory = 6 * time.Hour
)

// ErrorFile is a host answering an error file instead of the files asked:
// its name, without port or link, since when, and the error file's size.
type ErrorFile struct {
	Host  string
	Since time.Time
	Size  int64
}

// misfits are the answers a host gave of one size far under the files'
// own (see errorShare): the files answered so, since when the host is
// told answering an error file, zero before, and the last such answer.
type misfits struct {
	files       map[accounts.ID]misfit
	since, last time.Time
}

// misfit is a file a host answered with an error file: when first, the
// host its link names, and the addon whose record counted the answer as a
// size announced wrong (see noteSize), empty when none did.
type misfit struct {
	at      time.Time
	link    string
	counted string
}

// noteMisfit records that host answered size for the file of source id,
// whose link names link and whose own size is over errorShare times
// size, and reports whether size is the host's error file: answered for
// errorRepeat different files within errorWindow. An answer that is not,
// to the first answer of a file whose addon announced its size, counts as
// a size announced wrong in that addon's record (see noteSize), unless
// announcer is empty; one that is takes back what the earlier ones of the
// same size counted, so that an outage never makes an addon's sizes
// untrusted. The host is told from errorFiles files on.
func (c *Cache) noteMisfit(host string, id accounts.ID, link string, size int64, announcer string) bool {
	now := c.now()
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.host(host)
	c.forgetMisfits(state, now)
	if state.misfits == nil {
		state.misfits = map[int64]*misfits{}
	}
	m := state.misfits[size]
	if m == nil {
		m = &misfits{files: map[accounts.ID]misfit{}}
		state.misfits[size] = m
	}
	m.last = now
	if _, known := m.files[id]; !known {
		m.files[id] = misfit{at: now, link: link}
	}
	if len(m.files) < errorRepeat {
		if announcer != "" {
			f := m.files[id]
			f.counted = announcer
			m.files[id] = f
			c.noteSizeLocked(announcer, false)
		}
		return false
	}
	for file, f := range m.files {
		if f.counted != "" {
			if record := c.sizes[f.counted]; record != nil && record.disagreed > 0 {
				record.disagreed--
			}
			// The file counts again once its own size is answered.
			c.uncounted[file] = true
			f.counted = ""
			m.files[file] = f
		}
	}
	if m.since.IsZero() && len(m.files) >= errorFiles {
		m.since = now
		for _, f := range m.files {
			if f.at.Before(m.since) {
				m.since = f.at
			}
		}
		c.logger.Warn("A host answers an error file instead of the files asked: they are refused until it answers one of its files",
			"host", hostName(host), "size", size, "files", len(m.files))
	}
	return true
}

// forgetMisfits forgets, of what a host answered, the answers older than
// errorWindow, of a size not told as its error file, and an error file
// not answered for errorMemory. c.learned is held.
func (c *Cache) forgetMisfits(state *hostState, now time.Time) {
	for size, m := range state.misfits {
		if !m.since.IsZero() {
			if now.Sub(m.last) > errorMemory {
				delete(state.misfits, size)
			}
			continue
		}
		for id, f := range m.files {
			if now.Sub(f.at) > errorWindow {
				delete(m.files, id)
			}
		}
		if len(m.files) == 0 {
			delete(state.misfits, size)
		}
	}
}

// recovery is what a host that answered one of its files again had
// answered of other sizes: the sources it answered so, and those sizes.
type recovery struct {
	refused []accounts.ID
	sizes   []int64
}

// noteFit records that host answered a file with its own size: what it
// answered of other sizes is forgotten, and returned, for the sources
// refused for it to be tried again at once (see retry). An answer of a
// size the host answers for other files is no answer of the file's.
func (c *Cache) noteFit(host string, size int64) recovery {
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.hosts[host]
	if state == nil || len(state.misfits) == 0 || state.misfits[size] != nil {
		return recovery{}
	}
	var r recovery
	told := false
	for size, m := range state.misfits {
		told = told || !m.since.IsZero()
		r.sizes = append(r.sizes, size)
		for id := range m.files {
			r.refused = append(r.refused, id)
		}
	}
	state.misfits = nil
	if told {
		c.logger.Info("A host answers its files again: those refused meanwhile are tried again", "host", hostName(host), "files", len(r.refused))
	}
	return r
}

// retry has the sources refused for a host's error file tried again at
// once, now that it answers its files: their failure is forgotten, and
// what one learned of the error file, taken for its file before the host
// was found answering it, is removed, at once or, while a reader holds
// it, once the last one releases it (see forget). Recovered is then told
// them (see OnRecovered). Neither a source's lock nor c.learned is held.
func (c *Cache) retry(r recovery) {
	if len(r.refused) == 0 {
		return
	}
	for _, id := range r.refused {
		c.mu.Lock()
		s := c.sources[id]
		c.mu.Unlock()
		if s == nil {
			continue
		}
		s.Retry()
		if size, known := s.knownSize(); known && slices.Contains(r.sizes, size) {
			c.forget(s)
		}
	}
	c.tellRecovered(r.refused)
}

// forget removes what the cache holds of source s, now, or once its last
// reader releases it (see Source.Release): the readers holding it read on
// what it holds, and the next Open after them starts it anew.
func (c *Cache) forget(s *Source) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.sources[s.id] != s:
	case s.refs > 0:
		s.forgotten = true
	default:
		c.discardLocked(s)
	}
}

// tellRecovered tells recovered the sources refused for a host's error
// file (see OnRecovered).
func (c *Cache) tellRecovered(refused []accounts.ID) {
	c.recoveredMu.Lock()
	recovered := c.recovered
	c.recoveredMu.Unlock()
	if recovered != nil {
		recovered(refused)
	}
}

// OnRecovered has recovered told the sources refused for a host's error
// file, by identifier, as soon as the host answers one of its files: the
// versions refused for it are tried again at once (see ErrorFiles).
// recovered runs on the goroutine that read the answer, so it must return
// quickly.
func (c *Cache) OnRecovered(recovered func(sources []accounts.ID)) {
	c.recoveredMu.Lock()
	defer c.recoveredMu.Unlock()
	c.recovered = recovered
}

// ErrorFiles lists the hosts answering an error file instead of the files
// asked now, by name (see errorFiles).
func (c *Cache) ErrorFiles() []ErrorFile {
	now := c.now()
	c.learned.Lock()
	defer c.learned.Unlock()
	var told []ErrorFile
	for host, state := range c.hosts {
		c.forgetMisfits(state, now)
		if e, ok := errorFileOf(host, state); ok {
			told = append(told, e)
		}
	}
	slices.SortFunc(told, func(a, b ErrorFile) int { return cmp.Compare(a.Host, b.Host) })
	return slices.CompactFunc(told, func(a, b ErrorFile) bool { return a.Host == b.Host })
}

// ErrorFileOf is the error file the host of a source's file answers now,
// if any: the host that refused the source of id for it, else the one its
// link names, else one the links of the files it refused name, as a
// resolver in front of it.
func (c *Cache) ErrorFileOf(id accounts.ID, link string) (ErrorFile, bool) {
	now := c.now()
	host := hostOf(link)
	c.learned.Lock()
	defer c.learned.Unlock()
	var named, behind ErrorFile
	for answering, state := range c.hosts {
		c.forgetMisfits(state, now)
		e, ok := errorFileOf(answering, state)
		if !ok {
			continue
		}
		for _, m := range state.misfits {
			if m.since.IsZero() {
				continue
			}
			if _, refused := m.files[id]; refused {
				return e, true
			}
			for _, f := range m.files {
				if host != "" && f.link == host && behind.Host == "" {
					behind = e
				}
			}
		}
		if host != "" && answering == host {
			named = e
		}
	}
	if named.Host != "" {
		return named, true
	}
	return behind, behind.Host != ""
}

// errorFileOf is the error file a host is told answering, the one told
// first, if any. c.learned is held.
func errorFileOf(host string, state *hostState) (ErrorFile, bool) {
	var e ErrorFile
	for size, m := range state.misfits {
		if !m.since.IsZero() && (e.Since.IsZero() || m.since.Before(e.Since)) {
			e = ErrorFile{Host: hostName(host), Since: m.since, Size: size}
		}
	}
	return e, !e.Since.IsZero()
}

// hostName is a host's name, without its port.
func hostName(host string) string {
	return (&url.URL{Host: host}).Hostname()
}
