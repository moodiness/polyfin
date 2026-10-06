package accounts

import (
	"sync"
	"time"
)

// signInLife is how long a resolved access token or API key is trusted
// without asking the database again. Every change this process makes to a
// user, a device or an API key forgets them all at once (see
// forgetSignIns), so the life only bounds how long a change made outside
// the server, by hand in the database, takes to apply.
const signInLife = 30 * time.Second

// maxSignIns bounds the tokens remembered. Only tokens that resolved are
// kept, so there are about as many as signed-in devices and API keys in
// use; past the bound, the expired ones go, then all of them.
const maxSignIns = 4096

// signIns remembers the access tokens and API keys resolved lately, by the
// hash of the token. generation counts the changes that forgot them: a
// lookup that started before one keeps nothing, as what it read may be what
// the change replaced.
type signIns struct {
	mu         sync.Mutex
	generation uint64
	devices    map[string]resolvedDevice
	keys       map[string]resolvedKey
}

// resolvedDevice is a device and its user as resolved at a time.
type resolvedDevice struct {
	device Device
	user   User
	at     time.Time
}

// resolvedKey is an API key as resolved at a time.
type resolvedKey struct {
	key APIKey
	at  time.Time
}

// current returns the generation a lookup starts in.
func (c *signIns) current() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

// device returns what the token of hash resolved to, if it is recent.
func (c *signIns) device(hash string, now time.Time) (resolvedDevice, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.devices[hash]
	if !ok || now.Sub(entry.at) >= signInLife {
		return resolvedDevice{}, false
	}
	return entry, true
}

// keepDevice remembers what the token of hash resolved to in a lookup
// started in generation, unless a change came since.
func (c *signIns) keepDevice(hash string, entry resolvedDevice, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation {
		return
	}
	if c.devices == nil {
		c.devices = map[string]resolvedDevice{}
	}
	if _, ok := c.devices[hash]; !ok && len(c.devices) >= maxSignIns {
		prune(c.devices, entry.at, func(e resolvedDevice) time.Time { return e.at })
	}
	c.devices[hash] = entry
}

// touched records the activity written for the device of hash, if it is
// still remembered, without making it more recent: the device and user
// were not read again.
func (c *signIns) touched(hash string, at time.Time, remoteAddress string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.devices[hash]
	if !ok {
		return
	}
	entry.device.LastActivityAt, entry.device.RemoteAddress = at, remoteAddress
	entry.user.LastActivityAt = &at
	c.devices[hash] = entry
}

// key returns what the API key of hash resolved to, if it is recent.
func (c *signIns) key(hash string, now time.Time) (APIKey, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.keys[hash]
	if !ok || now.Sub(entry.at) >= signInLife {
		return APIKey{}, false
	}
	return entry.key, true
}

// keepKey remembers what the API key of hash resolved to in a lookup
// started in generation, unless a change came since.
func (c *signIns) keepKey(hash string, key APIKey, at time.Time, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation {
		return
	}
	if c.keys == nil {
		c.keys = map[string]resolvedKey{}
	}
	if _, ok := c.keys[hash]; !ok && len(c.keys) >= maxSignIns {
		prune(c.keys, at, func(e resolvedKey) time.Time { return e.at })
	}
	c.keys[hash] = resolvedKey{key: key, at: at}
}

// used records the use written for the API key of hash, if it is still
// remembered.
func (c *signIns) used(hash string, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.keys[hash]; ok {
		entry.key.LastUsedAt = &at
		c.keys[hash] = entry
	}
}

// forget drops every token remembered, and makes the lookups running keep
// nothing.
func (c *signIns) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	clear(c.devices)
	clear(c.keys)
}

// prune drops the expired entries of a full map, or every entry when none
// is expired.
func prune[E any](entries map[string]E, now time.Time, at func(E) time.Time) {
	for hash, entry := range entries {
		if now.Sub(at(entry)) >= signInLife {
			delete(entries, hash)
		}
	}
	if len(entries) >= maxSignIns {
		clear(entries)
	}
}

// forgetSignIns makes every access token and API key be resolved from the
// database again. Every change to a user, a device or an API key calls it
// once the change is stored, so that a user disabled, signed out or given
// other permissions is served as such from their next request.
func (s *Store) forgetSignIns() {
	s.signIns.forget()
}
