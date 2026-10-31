package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/torrent"
)

// How a play's torrent starts, for the web client's line under jellyfin-web's
// spinner (see webclient.js, the seventeenth).
//
// A PlaybackInfo of a signed-in user's device records the torrent versions
// its play tries, and why each that failed did not start. Once it answers,
// only the version it chose stays, while the player reads its first
// segments, for startLinger; a play that chose a link, or none, leaves
// nothing. GET /Polyfin/Playback/Starting tells the caller's own device how
// the start goes, from the most advanced of those torrents: its stage, the
// peers found and connected, the bytes received and those the reads still
// wait for. Another user's plays, and another device's, are never told, and
// nothing names a peer, a tracker or an address.

// startLinger is how long a play's chosen torrent is told after PlaybackInfo
// answered: as long as a torrent version's play waits for it at most (see
// playback.Service.TorrentPlayMost).
const startLinger = 3 * time.Minute

// startKey is a device of a user.
type startKey struct{ user, device accounts.ID }

// starts are the plays whose torrents are told, by device.
type starts struct {
	mu      sync.Mutex
	byPlace map[startKey]*playStart
}

// playStart is the start of one play: the torrent versions it tried, in the
// order it tried them, why those that did not start failed, and when its
// PlaybackInfo answered, zero while it waits.
type playStart struct {
	mu       sync.Mutex
	versions []library.Version
	failed   map[accounts.ID]string
	answered time.Time
}

// playStartKey marks the context of a PlaybackInfo whose play's start is
// told (see withPlayStart).
type playStartKey struct{}

// withPlayStart begins telling the start of a play of the caller's device,
// replacing what an earlier play of the device told, and returns the
// context its tries record into. A request without a user's device tells
// nothing.
func (h *Handler) withPlayStart(ctx context.Context) (context.Context, *playStart) {
	c := callerFrom(ctx)
	if c.APIKey != nil || c.User.ID == (accounts.ID{}) || c.Device.ID == (accounts.ID{}) {
		return ctx, nil
	}
	s := &playStart{failed: map[accounts.ID]string{}}
	h.starts.mu.Lock()
	if h.starts.byPlace == nil {
		h.starts.byPlace = map[startKey]*playStart{}
	}
	for key, kept := range h.starts.byPlace {
		if kept.over(time.Now()) {
			delete(h.starts.byPlace, key)
		}
	}
	h.starts.byPlace[startKey{c.User.ID, c.Device.ID}] = s
	h.starts.mu.Unlock()
	return context.WithValue(ctx, playStartKey{}, s), s
}

// playStartOf returns the start a request's play records into, nil when
// none.
func playStartOf(ctx context.Context) *playStart {
	s, _ := ctx.Value(playStartKey{}).(*playStart)
	return s
}

// trying records that the play tries version, a torrent.
func (s *playStart) trying(version library.Version) {
	if s == nil || version.Torrent == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tried := range s.versions {
		if tried.ID == version.ID {
			return
		}
	}
	s.versions = append(s.versions, version)
}

// tried records why version, a torrent, could not start, when err tells it:
// a start that timed out (see torrent.StartError.Why), a route that cannot
// carry it ("route"), or as many torrents running as allowed ("at_cap").
func (s *playStart) tried(version library.Version, err error) {
	if s == nil || version.Torrent == nil || err == nil {
		return
	}
	var why string
	var start *torrent.StartError
	switch {
	case errors.As(err, &start):
		why = start.Why()
	case torrent.RouteFailure(err):
		why = "route"
	case errors.Is(err, torrent.ErrAtCap):
		why = "at_cap"
	default:
		return
	}
	s.mu.Lock()
	s.failed[version.ID] = why
	s.mu.Unlock()
}

// answer records that PlaybackInfo answered with chosen, nil when no
// version plays: only a torrent chosen stays told.
func (s *playStart) answer(chosen *library.Version) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answered = time.Now()
	s.failed = map[accounts.ID]string{}
	s.versions = nil
	if chosen != nil && chosen.Torrent != nil {
		s.versions = []library.Version{*chosen}
	}
}

// over reports whether the start is no longer told at now.
func (s *playStart) over(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.answered.IsZero() && (len(s.versions) == 0 || now.Sub(s.answered) > startLinger)
}

// startProgressJSON is how a play's start goes, as
// /Polyfin/Playback/Starting tells it. Stage is "" when the device's play
// waits for no torrent; else peers, details or pieces (see torrent.Stage),
// or failed when every torrent tried did not start, Failure then telling
// why the first did not. Received and Waiting are bytes.
type startProgressJSON struct {
	Stage          string
	PeersFound     int
	PeersConnected int
	Received       int64
	Waiting        int64
	Failure        string
}

// stageRank orders the stages, the most advanced last.
var stageRank = map[torrent.Stage]int{torrent.StagePeers: 1, torrent.StageDetails: 2, torrent.StagePieces: 3}

// progress tells how the start goes, from the most advanced of the torrents
// it tries, the first tried among equals.
func (h *Handler) progress(s *playStart) startProgressJSON {
	s.mu.Lock()
	versions := append([]library.Version(nil), s.versions...)
	failed := make(map[accounts.ID]string, len(s.failed))
	for id, why := range s.failed {
		failed[id] = why
	}
	s.mu.Unlock()
	var best startProgressJSON
	bestRank, failure := 0, ""
	for _, version := range versions {
		if why, ok := failed[version.ID]; ok {
			failure = cmp.Or(failure, why)
			continue
		}
		p, running := h.Playback.TorrentProgress(version)
		if !running {
			continue
		}
		rank := stageRank[p.Stage]
		if rank > bestRank || rank == bestRank && (p.Connected > best.PeersConnected || p.Connected == best.PeersConnected && p.Received > best.Received) {
			bestRank = rank
			best = startProgressJSON{Stage: string(p.Stage), PeersFound: p.Found, PeersConnected: p.Connected, Received: p.Received, Waiting: p.Waiting}
		}
	}
	if bestRank == 0 && failure != "" {
		return startProgressJSON{Stage: "failed", Failure: failure}
	}
	return best
}

// playbackStarting answers how the start of the caller's device's play goes.
func (h *Handler) playbackStarting(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	h.starts.mu.Lock()
	s := h.starts.byPlace[startKey{c.User.ID, c.Device.ID}]
	h.starts.mu.Unlock()
	if s == nil || s.over(time.Now()) {
		writeJSON(w, http.StatusOK, startProgressJSON{})
		return
	}
	writeJSON(w, http.StatusOK, h.progress(s))
}
