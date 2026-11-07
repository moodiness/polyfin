package accounts

import (
	"sync"
	"time"
)

// signInLife is how long a resolved access token or API key, or a user
// read by identifier, is trusted without asking the database again. Every
// change this process makes to a user, a device or an API key forgets
// them all at once (see forgetSignIns), so the life only bounds how long a
// change made outside the server, by hand in the database, takes to apply.
const signInLife = 30 * time.Second

// maxSignIns bounds the tokens remembered, and the users. Only tokens that
// resolved are kept, so there are about as many as signed-in devices and
// API keys in use; past the bound, the expired ones go, then all of them.
const maxSignIns = 4096

// signIns remembers the access tokens and API keys resolved lately, by the
// hash of the token, and the users read lately, by identifier: streams,
// HLS segments and admin requests read their user on every request.
// generation counts the changes that forgot them: a lookup that started
// before one keeps nothing, as what it read may be what the change
// replaced.
type signIns struct {
	mu         sync.Mutex
	generation uint64
	devices    map[string]resolvedDevice
	keys       map[string]resolvedKey
	users      map[ID]resolvedUser
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

// resolvedUser is a user as read at a time.
type resolvedUser struct {
	user User
	at   time.Time
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

// deviceByID returns the device of identifier id, if a token resolved to
// it recently.
func (c *signIns) deviceByID(id ID, now time.Time) (Device, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, entry := range c.devices {
		if entry.device.ID == id && now.Sub(entry.at) < signInLife {
			return entry.device, true
		}
	}
	return Device{}, false
}

// touched records the activity written for the device of hash and its
// user, if they are still remembered, without making them more recent:
// the device and user were not read again. Activity times never move
// backwards.
func (c *signIns) touched(hash string, device Device, at time.Time, remoteAddress string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.devices[hash]; ok {
		entry.device.LastActivityAt, entry.device.RemoteAddress = latest(entry.device.LastActivityAt, at), remoteAddress
		entry.user.active(at, entry.device.Client)
		c.devices[hash] = entry
	}
	if entry, ok := c.users[device.UserID]; ok {
		entry.user.active(at, device.Client)
		c.users[device.UserID] = entry
	}
}

// latest returns the later of two times.
func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// active records activity of the user at a time from app, once it is
// written: activity times never move backwards, and the app moves with
// the time only.
func (user *User) active(at time.Time, app string) {
	if user.LastActivityAt != nil && !at.After(*user.LastActivityAt) {
		return
	}
	user.LastActivityAt, user.LastActivityApp = &at, app
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

// user returns the user of identifier id as read lately, if it is recent.
func (c *signIns) user(id ID, now time.Time) (User, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.users[id]
	if !ok || now.Sub(entry.at) >= signInLife {
		return User{}, false
	}
	return entry.user, true
}

// keepUser remembers a user read at a time in a lookup started in
// generation, unless a change came since.
func (c *signIns) keepUser(user User, at time.Time, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation {
		return
	}
	if c.users == nil {
		c.users = map[ID]resolvedUser{}
	}
	if _, ok := c.users[user.ID]; !ok && len(c.users) >= maxSignIns {
		prune(c.users, at, func(e resolvedUser) time.Time { return e.at })
	}
	c.users[user.ID] = resolvedUser{user: user, at: at}
}

// userActive records the activity of the user id written at a time from
// app, if the user is still remembered.
func (c *signIns) userActive(id ID, at time.Time, app string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.users[id]; ok {
		entry.user.active(at, app)
		c.users[id] = entry
	}
}

// forgetUser drops the user id, read again at the next lookup.
func (c *signIns) forgetUser(id ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.users, id)
}

// forget drops every token and user remembered, and makes the lookups
// running keep nothing.
func (c *signIns) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	clear(c.devices)
	clear(c.keys)
	clear(c.users)
}

// prune drops the expired entries of a full map, or every entry when none
// is expired.
func prune[K comparable, E any](entries map[K]E, now time.Time, at func(E) time.Time) {
	for key, entry := range entries {
		if now.Sub(at(entry)) >= signInLife {
			delete(entries, key)
		}
	}
	if len(entries) >= maxSignIns {
		clear(entries)
	}
}

// forgetSignIns makes every access token, API key and user be read from
// the database again. Every change to a user, a device or an API key calls
// it once the change is stored, so that a user disabled, deleted, signed
// out or given other permissions is served as such from their next
// request.
func (s *Store) forgetSignIns() {
	s.signIns.forget()
}
