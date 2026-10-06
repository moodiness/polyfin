package source

import (
	"net/url"
	"time"
)

// What the cache learns of hosts and addons, kept for the sources to come.

// hostMemory is how long a host found to want one connection per file is
// given one.
const hostMemory = time.Hour

// gainFactor is how much more a file's connections must read together
// than one alone for its host to be given several.
const gainFactor = 1.2

// hostState is what the cache learned of a host.
type hostState struct {
	// single is how fast one connection alone read from it, in bytes a
	// second; limited, until when it is given one connection per file.
	single  float64
	limited time.Time
}

// hostOf is the host a URL names, port included: what the cache learns
// of is the host the bytes come from.
func hostOf(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// connectionsFor is how many connections a file of host may be read over.
func (c *Cache) connectionsFor(host string) int {
	c.learned.Lock()
	defer c.learned.Unlock()
	if state := c.hosts[host]; state != nil && time.Now().Before(state.limited) {
		return 1
	}
	return max(1, c.connections)
}

// limitHost gives a host one connection per file for a while: it asked to
// slow down, refused a connection more, or served several no faster.
func (c *Cache) limitHost(host, why string) {
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.host(host)
	if !time.Now().Before(state.limited) && c.connections > 1 {
		c.logger.Debug("A host is read over one connection per file", "host", host, "why", why)
	}
	state.limited = time.Now().Add(hostMemory)
}

// noteSingle records how fast one connection alone read from host.
func (c *Cache) noteSingle(host string, rate float64) {
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.host(host)
	if state.single == 0 {
		state.single = rate
	} else {
		state.single = (state.single + rate) / 2
	}
}

// gains reports whether connections reading rate together read faster
// than one alone from host, as far as known.
func (c *Cache) gains(host string, rate float64) bool {
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.hosts[host]
	return state == nil || state.single == 0 || rate >= gainFactor*state.single
}

// host is the state of host, made on first use. c.learned is held.
func (c *Cache) host(host string) *hostState {
	state := c.hosts[host]
	if state == nil {
		state = &hostState{}
		c.hosts[host] = state
	}
	return state
}

// sizeRecord counts the files whose first answer agreed with the size an
// addon announced, and those whose answer did not.
type sizeRecord struct {
	agreed, disagreed int
	// ignored tells that the addon's sizes were found wrong, which was
	// logged.
	ignored bool
}

// sizeTrusted reports whether the sizes an addon announces are trusted to
// catch another file at the first byte: once one of its files had the size
// it announced, and as long as most did. An addon whose sizes are rounded
// or wrong never refuses a file this way.
func (c *Cache) sizeTrusted(announcer string) bool {
	c.learned.Lock()
	defer c.learned.Unlock()
	record := c.sizes[announcer]
	return record != nil && record.agreed > 0 && record.agreed >= record.disagreed
}

// noteSize records whether the first answer of a file had the size its
// addon announced, and logs once an addon whose sizes are no longer used.
func (c *Cache) noteSize(announcer string, agrees bool) {
	c.learned.Lock()
	defer c.learned.Unlock()
	record := c.sizes[announcer]
	if record == nil {
		record = &sizeRecord{}
		c.sizes[announcer] = record
	}
	trusted := record.agreed > 0 && record.agreed >= record.disagreed
	if agrees {
		record.agreed++
		record.ignored = false
		return
	}
	record.disagreed++
	if !record.ignored && (trusted && record.agreed < record.disagreed || record.agreed == 0 && record.disagreed >= 3) {
		record.ignored = true
		c.logger.Info("The file sizes an addon announces do not match its files: they are not used to check them",
			"addon", announcer, "agreed", record.agreed, "disagreed", record.disagreed)
	}
}
