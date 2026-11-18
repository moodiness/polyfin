package jellyfin

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// servedStale is how long a device keeps its place among its user's
// playbacks at once after the last answer of a playback sent to it:
// players read ahead, pause, and ask again, as DLNA devices do (see
// dlnaStale).
const servedStale = 2 * time.Minute

// servedPlays are the playbacks the media routes send apps, by user and
// device: Playbacks at once counts them whether or not their apps report
// them (see admitPlay). A device holds its place while an answer of a
// playback is being sent to it, and for servedStale after the last one
// that went out. Only the devices of users with a limit have places, and
// none is kept once it is held no more: each look forgets those.
type servedPlays struct {
	mu     sync.Mutex
	places map[servedKey]*servedPlace
}

// servedKey names a place: a user's device, zero for the requests that
// name none, which share one place.
type servedKey struct {
	user, device accounts.ID
}

// servedPlace is a device's place.
type servedPlace struct {
	// answers is how many answers are being sent to the device.
	answers int
	// seen is when the last answer that went out ended; zero when none
	// did, or when the device's app reported the playback's stop since.
	seen time.Time
	// stops counts the stops the device's app reported: an answer that
	// began before one does not keep the place once it ends.
	stops int
}

// held reports whether the device holds its place at now.
func (p *servedPlace) held(now time.Time) bool {
	return p.answers > 0 || !p.seen.IsZero() && now.Sub(p.seen) < servedStale
}

// servedAnswer is an answer being sent to a device that holds a place,
// which it gives back when it ends (see servedPlays.leave); the zero
// value holds none.
type servedAnswer struct {
	key   servedKey
	place *servedPlace
	stops int
}

func newServedPlays() *servedPlays {
	return &servedPlays{places: map[servedKey]*servedPlace{}}
}

// others counts the devices of the user of own that hold a place at now,
// but own's and those among counted, which their apps' reports count
// already; all with any set, where own names a user alone. It forgets the
// places held no more. The caller holds the lock.
func (s *servedPlays) others(own servedKey, any bool, counted []accounts.ID, now time.Time) int {
	n := 0
	for key, place := range s.places {
		switch {
		case !place.held(now):
			delete(s.places, key)
		case key.user == own.user && (any || key != own) && !slices.Contains(counted, key.device):
			n++
		}
	}
	return n
}

// playing counts the devices of user that hold a place at now, but
// except, when set, and those among counted.
func (s *servedPlays) playing(user, except accounts.ID, counted []accounts.ID, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.others(servedKey{user: user, device: except}, except == (accounts.ID{}), counted, now)
}

// enter begins an answer to a device that holds its place at now: its
// own playback never refuses it. It reports false for a device that holds
// none, which admit decides on.
func (s *servedPlays) enter(key servedKey, now time.Time) (servedAnswer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	place := s.places[key]
	if place == nil || !place.held(now) {
		return servedAnswer{}, false
	}
	place.answers++
	return servedAnswer{key: key, place: place, stops: place.stops}, true
}

// admit gives the device of key a place and begins an answer to it,
// unless room or more of its user's other devices hold one, those among
// counted left out. Counting and taking the place are one step: of two
// devices that ask at the same time for the last place, one gets it.
func (s *servedPlays) admit(key servedKey, room int, counted []accounts.ID, now time.Time) (servedAnswer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	taken := s.others(key, false, counted, now)
	place := s.places[key]
	if place == nil {
		if taken >= room {
			return servedAnswer{}, false
		}
		place = &servedPlace{}
		s.places[key] = place
	}
	place.answers++
	return servedAnswer{key: key, place: place, stops: place.stops}, true
}

// leave ends an answer at now. One that went out, a success or a
// redirection to the source, keeps the device's place for servedStale
// from now, unless its app reported a stop since the answer began.
func (s *servedPlays) leave(answer servedAnswer, answered bool, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	place := answer.place
	place.answers--
	if answered && answer.stops == place.stops {
		place.seen = now
	}
	if place.answers == 0 && place.seen.IsZero() {
		delete(s.places, answer.key)
	}
}

// stopped notes that the app of a device reported its playback's stop:
// the device keeps its place only while answers are still being sent to
// it, so that another of the user's devices may go on from it at once.
func (s *servedPlays) stopped(user, device accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := servedKey{user: user, device: device}
	place := s.places[key]
	if place == nil {
		return
	}
	place.stops++
	place.seen = time.Time{}
	if place.answers == 0 {
		delete(s.places, key)
	}
}

// devicesPlaying counts the devices of user playing, sent a playback, or
// making a converted download, but except, DLNA devices included. A
// download whose device is not known counts as one more.
func (h *Handler) devicesPlaying(ctx context.Context, user accounts.User, except accounts.ID) (int, error) {
	counted, more, err := h.reportedPlaying(ctx, user, except)
	if err != nil {
		return 0, err
	}
	return len(counted) + more + h.served.playing(user.ID, except, counted, h.now()), nil
}

// admitPlay gives the device of a media request its place among its
// user's playbacks at once, until the answer ends (see meteredWriter), and
// reports whether the request may be served. A device that holds its place
// goes on, whatever plays elsewhere: its ranges, seeks and segments are
// never refused. One that holds none starts a playback, which is refused
// with 429 when as many of the user's other devices play as they may:
// those PlaybackInfo counts (see playbackLimitReached). granted is the
// device the request's grant was given to, zero for a request that
// carries a token alone.
//
// PlaybackInfo refuses the apps that ask it; this refuses the players
// that do not, or that kept an address, and counts those that report
// nothing.
func (h *Handler) admitPlay(w http.ResponseWriter, r *http.Request, user accounts.User, granted accounts.ID) bool {
	most := user.MaxPlaybacks
	if most <= 0 {
		return true
	}
	if granted == (accounts.ID{}) {
		granted = h.mediaDevice(r)
	}
	key := servedKey{user: user.ID, device: granted}
	answer, ok := h.served.enter(key, h.now())
	if !ok {
		counted, more, err := h.reportedPlaying(r.Context(), user, granted)
		if err != nil {
			h.internalError(w, r, err)
			return false
		}
		h.endStaleGuests()
		room := most - len(counted) - more - h.guests.ofUser(user.ID)
		if answer, ok = h.served.admit(key, room, counted, h.now()); !ok {
			h.refuseAtLimit(w, user)
			return false
		}
	}
	if counted, metered := w.(*meteredWriter); metered && counted.place.place == nil {
		counted.place = answer
	} else {
		// No writer tells when this answer ends: it counts as ended now.
		h.served.leave(answer, true, h.now())
	}
	return true
}

// leavePlace gives back at once the place admitPlay gave the request of
// w, as if its answer had not gone out: what it sends from here on is
// counted elsewhere, as a converted download is while it runs.
func (h *Handler) leavePlace(w http.ResponseWriter) {
	if counted, metered := w.(*meteredWriter); metered && counted.place.place != nil {
		h.served.leave(counted.place, false, h.now())
		counted.place = servedAnswer{}
	}
}

// refuseAtLimit answers a media request that would start one more
// playback than its user may have at once. Media routes answer a bare
// status; PlaybackInfo, which apps ask first, tells them with
// RateLimitExceeded.
func (h *Handler) refuseAtLimit(w http.ResponseWriter, user accounts.User) {
	h.Logger.Info("A playback was refused: the user plays on as many devices as they may", "user", user.Name)
	w.WriteHeader(http.StatusTooManyRequests)
}
