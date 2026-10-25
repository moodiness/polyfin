package jellyfin

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// A play chooses among a title's versions, in their order: the first that
// plays on the app (see decide), or with PreferDirectPlay the first that
// plays without conversion. The first versions, VersionAttempts of them,
// are analyzed if they were not before; the others are tried only when
// they were, which costs nothing. Up to parallelAttempts of them are read
// at once, a version found unreadable making room for the next: a version
// whose host never answers holds up the play only for versionPatience once
// a later one is ready, rather than for the whole analysis timeout. Each
// analysis runs to its end all the same, and is kept for the next play.

const (
	// endedTorrentWait is how long a play at Torrents at once waits for a
	// torrent it ended, the device's own left for this title, to stop: an
	// app's player that stopped ends its last read of the file within a
	// second, its request cut as it leaves; this leaves room for a busy
	// machine. The cap is never passed meanwhile: the new torrent starts
	// once the other stopped.
	endedTorrentWait = 5 * time.Second
	// parallelAttempts is how many versions a play analyzes at once.
	parallelAttempts = 3
	// versionPatience is how long a play waits for the versions before the
	// one it could play once that one is ready.
	versionPatience = 3 * time.Second
)

// candidates are the versions a play may choose, by their index among a
// title's versions, in the order they are tried. analyzed is how many of
// the first may be analyzed now, and named the one the app asked for, -1
// when it asked for none (see playbackInfo).
type candidates struct {
	order    []int
	analyzed int
	named    int
}

// chosen is what a play found among its candidates: the version chosen,
// nil when none plays, and those found unreadable; unstarted tells a
// torrent among them that could not start (see playback.TorrentUnstarted),
// or that was still being read when the play stopped waiting for it, which
// late tells.
type chosen struct {
	decided    *decided
	unreadable map[int]bool
	unstarted  bool
	late       bool
}

// tried is what trying a candidate gave: whether it plays, and plays
// without conversion; unreadable when it could not be analyzed, unstarted
// when it is a torrent that could not start.
type tried struct {
	n           int
	decided     decided
	plays       bool
	unconverted bool
	unreadable  bool
	unstarted   bool
}

// choose tries the candidates of p for a play, and returns the version
// chosen. prefer looks for a version that plays without conversion among
// those analyzed now, else takes the first that plays.
func (h *Handler) choose(r *http.Request, user accounts.User, p playable, opened accounts.ID, request playbackInfoRequest,
	c candidates, prefer bool) chosen {
	result := chosen{unreadable: map[int]bool{}}
	at := openedIndex(opened, p.versions)
	allowed := h.Accounts.Conversions(user)
	// The analyses and keyframe indexes started run to their end whatever
	// the app does meanwhile, for the next play (see playback.Analyze). The
	// tries stop waiting for them once the play is decided, or gave up: no
	// goroutine of the play is left behind. The first candidate is read as
	// the play's; the others, read alongside in case it does not play, in
	// the background, giving way to playbacks (see playback.Background). A
	// torrent, which only a playback reads, is read as the play's wherever
	// it comes: the play joins its swarm.
	tries, stop := context.WithCancel(context.WithoutCancel(r.Context()))
	defer stop()
	detached := r.WithContext(tries)
	aside := r.WithContext(playback.Background(detached.Context()))
	try := func(n int, analyze bool) tried {
		i := c.order[n]
		version := p.versions[i]
		detached := detached
		if n > 0 && version.Torrent == nil {
			detached = aside
		}
		// A torrent that may not start, as many run as Torrents at once
		// allows, is passed over as one that did not start in time. The
		// torrents the device or its user left for another title, which the
		// start of this one would end (see leaveTorrents), make room for it
		// first.
		if err := h.Playback.TorrentAdmitted(version); err != nil {
			ended := h.leaveTorrents(callerFrom(r.Context()).Device.ID, user.ID, h.titleOf(p.item.ID), version.Torrent.InfoHash)
			wait := time.Duration(0)
			if len(ended) > 0 {
				// A torrent ended stops once its last read ends: the app's
				// player, which stopped, may still read it for a moment.
				wait = endedTorrentWait
			}
			if err := h.Playback.TorrentAdmittedWithin(detached.Context(), version, wait); err != nil {
				h.Logger.Info("A torrent version is passed over: as many torrents run as Torrents at once allows", "addon", version.Addon)
				return tried{n: n, unreadable: true, unstarted: true}
			}
		}
		var analysis media.Analysis
		// The subtitle files inside a torrent are listed by its version's
		// first play, before its other streams: the tracks the app names
		// were those of the version without them, as its page showed it,
		// where it could pick no such file. They are not the version's
		// tracks now, and the user's preferences choose instead.
		files := len(h.Playback.TorrentSubtitles(detached.Context(), version))
		if analyze {
			var err error
			if analysis, err = h.Playback.Analyze(detached.Context(), version); err != nil {
				if detached.Context().Err() != nil {
					// The play is decided, or gave up: it waits no more.
					return tried{n: n}
				}
				h.Logger.Info("A version could not be analyzed", "addon", version.Addon, "error", err)
				return tried{n: n, unreadable: true, unstarted: version.Torrent != nil && playback.TorrentUnstarted(err)}
			}
		} else {
			var known bool
			if analysis, known = h.Playback.Analyzed(detached.Context(), version.ID); !known {
				return tried{n: n}
			}
		}
		listed := len(h.Playback.TorrentSubtitles(detached.Context(), version)) != files
		permits := h.convertible(allowed, user, version)
		d, ok := h.decide(detached, p, i, version, sourceID(opened, version, i == at), analysis, request, permits, i == c.named && !listed)
		if !ok {
			if detached.Context().Err() != nil {
				return tried{n: n}
			}
			if d.unstarted {
				return tried{n: n, unreadable: true, unstarted: true}
			}
			if permits != allowed {
				h.Logger.Info("A version would need its video converted while the server converts as many as it may", "addon", version.Addon)
			} else {
				h.Logger.Info("A version would need a conversion the user may not have, or is above their bitrate limit or quality group, or cannot be streamed", "addon", version.Addon)
			}
			return tried{n: n}
		}
		return tried{n: n, decided: d, plays: true, unconverted: prefer && h.unconverted(detached.Context(), d, request)}
	}

	attempted := min(c.analyzed, len(c.order))
	results := make(chan tried, attempted)
	started := 0
	start := func() {
		n := started
		started++
		go func() { results <- try(n, true) }()
	}
	for started < min(attempted, parallelAttempts) {
		start()
	}
	states := make([]*tried, attempted)
	var patience <-chan time.Time
	patient := true
	// A torrent version is waited for as long as its start and a piece
	// take, then as long as its torrent keeps receiving its pieces, within
	// Service.TorrentPlayMost: past that, the torrent versions still being read
	// are passed over, and no candidate more is tried.
	var late <-chan time.Time
	var lateTimer *time.Timer
	waited := time.Now()
	if wait := h.Playback.TorrentPlayWait(); wait > 0 &&
		slices.ContainsFunc(c.order[:attempted], func(i int) bool { return p.versions[i].Torrent != nil }) {
		lateTimer = time.NewTimer(wait)
		defer lateTimer.Stop()
		late = lateTimer.C
	}
	waitingOn := map[int]bool{}
	for started > 0 {
		if n, done := pick(states[:started], prefer, patient); done {
			if n >= 0 {
				result.decided = &states[n].decided
			}
			break
		}
		select {
		case t := <-results:
			states[t.n] = &t
			if t.unreadable {
				result.unreadable[c.order[t.n]] = true
			}
			result.unstarted = result.unstarted || t.unstarted
			if !t.plays && started < attempted && !result.late {
				start()
			}
			if t.plays && patience == nil {
				timer := time.NewTimer(versionPatience)
				defer timer.Stop()
				patience = timer.C
			}
		case <-patience:
			patient = false
		case <-late:
			late, result.late = nil, true
			receiving := false
			for n, t := range states[:started] {
				version := p.versions[c.order[n]]
				if t != nil || version.Torrent == nil {
					continue
				}
				if time.Since(waited) < h.Playback.TorrentPlayMost() && h.Playback.TorrentReceiving(version) {
					receiving = true
					if !waitingOn[n] {
						waitingOn[n] = true
						h.Logger.Info("A torrent version is slow to start but its pieces keep coming: the play waits on for it", "addon", version.Addon)
					}
					continue
				}
				// Its result, should it still come, replaces this.
				states[n] = &tried{n: n, unstarted: true}
				result.unstarted = true
				h.Logger.Info("A torrent version took too long to start for the play; it is read on for the next one", "addon", version.Addon)
			}
			if receiving {
				lateTimer.Reset(min(playback.TorrentPlayCheck, h.Playback.TorrentPlayMost()-time.Since(waited)))
				late = lateTimer.C
			}
		case <-r.Context().Done():
			return result
		}
	}
	if result.decided != nil || result.late {
		return result
	}
	// None of those analyzed now plays: the others are tried only when
	// they were analyzed before.
	for n := attempted; n < len(c.order); n++ {
		if t := try(n, false); t.plays {
			result.decided = &t.decided
			break
		}
	}
	return result
}

// pick picks among the candidates tried, in order, the one a play
// chooses, and reports whether it is done: the first that plays, or with
// prefer the first that plays without conversion, else the first that
// plays. Until patient ends, it waits for each candidate before the one it
// would pick; then those still being tried are passed over.
func pick(states []*tried, prefer, patient bool) (int, bool) {
	first := -1
	for n, t := range states {
		switch {
		case t == nil:
			if patient {
				return -1, false
			}
		case !t.plays:
		case !prefer || t.unconverted:
			return n, true
		case first < 0:
			first = n
		}
	}
	return first, true
}

// playableToPlay gathers a title's versions and subtitles for a play. It
// lists the versions as VersionsToPlay does: stale lists as they are, so
// complete is false when one was, for the caller to wait for the addons'
// new answers when none of their versions plays. The subtitles the addons
// have not given by then are waited for at most subtitleGrace more: those
// known now stand for them, the others reaching the next PlaybackInfo, as a
// track switch asks.
func (h *Handler) playableToPlay(ctx context.Context, user accounts.User, item library.Item) (playable, bool, error) {
	p := playable{item: item, tracks: h.trackPreferences(ctx, user)}
	if item.Kind == library.KindRecording {
		p.versions = h.recordingVersions(ctx, item)
		return p, true, nil
	}
	listed := make(chan []library.ExternalSubtitle, 1)
	go func() {
		subtitles, err := h.Library.Subtitles(ctx, user, item.ID)
		if err != nil {
			subtitles = nil
		}
		listed <- subtitles
	}()
	versions, complete, err := h.Library.VersionsToPlay(ctx, user, item.ID)
	// Before offered, which leaves the failed out of versions in place.
	p.unstarted = h.unstartedTorrent(versions)
	p.versions = h.offered(ctx, user, versions)
	grace := time.NewTimer(subtitleGrace)
	defer grace.Stop()
	select {
	case p.subtitles = <-listed:
		h.subtitleFiles.Put(item.ID, p.subtitles)
	case <-grace.C:
		if subtitles, all, err := h.Library.KnownSubtitles(ctx, user, item.ID); err == nil {
			p.subtitles = subtitles
			if all {
				h.subtitleFiles.Put(item.ID, subtitles)
			}
		}
	}
	return p, complete, err
}

// subtitleGrace is how long a play waits for the addons' subtitles once
// its versions are in.
const subtitleGrace = time.Second

// unstartedTorrent reports whether one of versions is a torrent left out
// for a while, as it could not start: a play then tells that the title
// cannot be played now.
func (h *Handler) unstartedTorrent(versions []library.Version) bool {
	return slices.ContainsFunc(versions, func(v library.Version) bool { return v.Torrent != nil && h.Playback.Unstarted(v.ID) })
}
