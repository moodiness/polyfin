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
// plays on the app (see decide), or with PreferDirectPlay the one that
// plays at the least cost (see playCost) among those tried. The first
// versions, VersionAttempts of them, are analyzed if they were not before;
// the others are tried only when they were, which costs nothing. They are
// read one after another: the next starts when the one before is
// unreadable or does not settle the play, or stays silent for
// versionPatience, the silent one read on alongside it. A version whose
// host never answers thus holds up the play for versionPatience, and the
// version after it then plays as soon as it is ready, rather than after
// the whole analysis timeout. Each analysis runs to its end all the same,
// and is kept for the next play.

const (
	// endedTorrentWait is how long a play at Torrents at once waits for a
	// torrent it ended, the device's own left for this title, to stop: an
	// app's player that stopped ends its last read of the file within a
	// second, its request cut as it leaves; this leaves room for a busy
	// machine. The cap is never passed meanwhile: the new torrent starts
	// once the other stopped.
	endedTorrentWait = 5 * time.Second
	// versionPatience is how long a play waits for the versions before the
	// one it could play once that one is ready, and how long a version in
	// turn may stay silent before the next starts alongside it.
	versionPatience = 3 * time.Second
)

// candidates are the versions a play may choose, by their index among a
// title's versions, in the order they are tried. analyzed is how many of
// the first may be analyzed now, and named the one the app asked for, -1
// when it asked for none (see playbackInfo); picked tells that the app
// asked for that one alone, by its own identifier, as a user picks it in
// the version menu.
type candidates struct {
	order    []int
	analyzed int
	named    int
	picked   bool
}

// tryable leaves out of c the torrents that did not start in time lately
// (see didNotStart), which item details still list: a play that picks no
// version tries the next ones. A version picked is tried again at once,
// whatever happened to it before.
func (h *Handler) tryable(p playable, c candidates) candidates {
	if c.picked {
		for _, i := range c.order {
			if h.didNotStart(p.versions[i]) {
				h.Playback.RetryUnstarted(p.versions[i])
			}
		}
		return c
	}
	c.order = slices.DeleteFunc(slices.Clone(c.order), func(i int) bool { return h.didNotStart(p.versions[i]) })
	return c
}

// didNotStart reports whether version is a torrent that did not start in
// time lately (see playback.Service.Unstarted).
func (h *Handler) didNotStart(version library.Version) bool {
	return version.Torrent != nil && h.Playback.Unstarted(version.ID)
}

// analyzedBefore reports whether a version's analysis is kept, from an
// earlier play or preparation: trying it reads nothing.
func (h *Handler) analyzedBefore(ctx context.Context, version library.Version) bool {
	_, ok := h.Playback.Analyzed(ctx, version.ID)
	return ok
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

// tried is what trying a candidate gave: whether it plays, and at which
// cost, with the languages of the tracks it plays; unreadable when it
// could not be analyzed, unstarted when it is a torrent that could not
// start.
type tried struct {
	n          int
	decided    decided
	plays      bool
	cost       playCost
	languages  playedLanguages
	unreadable bool
	unstarted  bool
}

// choose tries the candidates of p for a play, and returns the version
// chosen. prefer looks for the version that plays at the least cost among
// those analyzed now, else takes the first that plays; for a user given
// Play local files first, among the local files while one plays (see
// pick).
func (h *Handler) choose(r *http.Request, user accounts.User, p playable, opened accounts.ID, request playbackInfoRequest,
	c candidates, prefer bool) chosen {
	result := chosen{unreadable: map[int]bool{}}
	c = h.tryable(p, c)
	at := openedIndex(opened, p.versions)
	allowed := h.Accounts.Conversions(user)
	sides := make([]int, len(c.order))
	if user.LocalFirst {
		for n, i := range c.order {
			sides[n] = remoteRank(p.versions[i])
		}
	}
	// What each candidate gave is kept with the title's last plays (see
	// playdecisions.go), once the play is decided; a play the app gave up
	// tells nothing.
	outcomes := newPlayTries(p.versions, c.order)
	started := 0
	defer func() {
		if r.Context().Err() != nil {
			return
		}
		device := callerFrom(r.Context()).Device
		decision := PlayDecision{At: h.now(), User: user.Name, Device: device.DeviceName, App: device.Client, Picked: c.picked,
			Tries: outcomes.reached(started)}
		if result.decided != nil {
			decision.Chosen, decision.Delivery = result.decided.version.ID, delivery(result.decided.decision)
		}
		h.recordDecision(p.item.ID, decision)
	}()
	// The analyses and keyframe indexes started run to their end whatever
	// the app does meanwhile, for the next play (see playback.Analyze). The
	// tries stop waiting for them once the play is decided, or gave up: no
	// goroutine of the play is left behind. Each candidate is read as the
	// play's: one starts only once the play needs it (see choose's loop).
	tries, stop := context.WithCancel(context.WithoutCancel(r.Context()))
	defer stop()
	detached := r.WithContext(tries)
	try := func(n int, analyze bool) tried {
		i := c.order[n]
		version := p.versions[i]
		// The web client tells how the torrents analyzed for the play start
		// (see startprogress.go).
		start := playStartOf(r.Context())
		if analyze {
			start.trying(version)
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
				h.Logger.Info("A torrent version is passed over: as many torrents run as Torrents at once allows", "addon", version.AddonLog)
				start.tried(version, err)
				outcomes.note(n, TryTorrentCap, nil)
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
				h.Logger.Info("A version could not be analyzed", "addon", version.AddonLog, "error", err)
				start.tried(version, err)
				unstarted := version.Torrent != nil && playback.TorrentUnstarted(err)
				if unstarted {
					outcomes.note(n, TryUnstarted, err)
				} else {
					outcomes.note(n, TryUnreadable, err)
				}
				return tried{n: n, unreadable: true, unstarted: unstarted}
			}
		} else {
			var known bool
			if analysis, known = h.Playback.Analyzed(detached.Context(), version.ID); !known {
				outcomes.note(n, TryNotAnalyzed, nil)
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
				outcomes.note(n, TryUnstarted, nil)
				return tried{n: n, unreadable: true, unstarted: true}
			}
			if permits != allowed {
				h.Logger.Info("A version would need its video converted while the server converts as many as it may", "addon", version.AddonLog)
				outcomes.note(n, TryConversionCap, nil)
			} else {
				h.Logger.Info("A version would need a conversion the user may not have, or is above their bitrate limit or quality group, or cannot be streamed", "addon", version.AddonLog)
				outcomes.note(n, TryUnplayable, nil)
			}
			return tried{n: n}
		}
		outcomes.note(n, TryPlays, nil)
		return tried{n: n, decided: d, plays: true, cost: playCostOf(request, d.analysis.Bitrate, d.decision),
			languages: languagesPlayed(p.tracks, d.source.MediaStreams, d.decision)}
	}

	attempted := min(c.analyzed, len(c.order))
	results := make(chan tried, attempted)
	// A candidate starts once the play needs it: the first at once, the
	// next when those in turn are tried without settling the play, or when
	// the one in turn stays silent for versionPatience. The candidates
	// before silentBefore were silent so long: the play passes them over as
	// soon as a later one plays, having waited for them already.
	var silence <-chan time.Time
	silentBefore := 0
	start := func() {
		n := started
		started++
		silence = nil
		if started < attempted {
			silence = h.patienceEnd(versionPatience)
		}
		go func() {
			// A try that panicked, in what reads the version, is one whose
			// version cannot be read: the play goes on with the next.
			result := tried{n: n, unreadable: true}
			defer func() {
				if cause := recover(); cause != nil {
					logRecovered(h.Logger, "Trying a version ended on a panic", cause)
				}
				results <- result
			}()
			result = try(n, true)
		}()
	}
	if attempted > 0 {
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
	unsettled := func(t *tried) bool { return t == nil }
	plays := func(t *tried) bool { return t != nil && t.plays }
	for started > 0 {
		n, done := pick(states[:started], sides, prefer, patient)
		// The play is settled once it has a version that plays, without
		// conversion when it prefers one. Else, with no candidate in turn
		// being read, it tries the next: once one plays converted, only a
		// version analyzed before, which costs nothing to try.
		settled := done && n >= 0 && (!prefer || states[n].cost <= costCopied)
		if !settled && patient && started < attempted && !result.late && !slices.ContainsFunc(states[silentBefore:started], unsettled) &&
			(!slices.ContainsFunc(states[:started], plays) || h.analyzedBefore(detached.Context(), p.versions[c.order[started]])) {
			start()
			continue
		}
		if done {
			if n >= 0 {
				result.decided = &states[n].decided
				if n > 0 && states[0] != nil && states[0].plays {
					h.Logger.Info("Prefer versions the app plays without conversion passed over the first version for one that costs the app less to play",
						"item", p.item.ID, "passed", p.versions[c.order[0]].AddonLog, "passedPlays", delivery(states[0].decided.decision),
						"addon", states[n].decided.version.AddonLog, "plays", delivery(states[n].decided.decision))
				}
			}
			break
		}
		select {
		case t := <-results:
			states[t.n] = &t
			if t.n == started-1 {
				silence = nil
			}
			if t.unreadable {
				result.unreadable[c.order[t.n]] = true
			}
			result.unstarted = result.unstarted || t.unstarted
			if t.plays {
				// A version that plays after others stayed silent is not
				// held up for them again: they had their patience.
				if !slices.ContainsFunc(states[silentBefore:started], unsettled) && (!prefer || t.cost <= costCopied) {
					patient = false
				} else if patience == nil {
					patience = h.patienceEnd(versionPatience)
				}
			}
		case <-silence:
			silence = nil
			if patient && started < attempted && !result.late {
				h.Logger.Info("A version is slow to analyze: the next one is tried alongside it", "addon", p.versions[c.order[started-1]].AddonLog)
				silentBefore = started
				start()
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
						h.Logger.Info("A torrent version is slow to start but its pieces keep coming: the play waits on for it", "addon", version.AddonLog)
					}
					continue
				}
				// Its result, should it still come, replaces this.
				states[n] = &tried{n: n, unstarted: true}
				result.unstarted = true
				h.Logger.Info("A torrent version took too long to start for the play; it is read on for the next one", "addon", version.AddonLog)
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
// prefer the one that plays at the least cost (see playCost) among those
// playing the user's languages as well as the first that plays (see
// playedLanguages), the first of them on a tie. sides parts the
// candidates, by their index: prefer looks no further than the side of
// the first that plays, so that, for a user given Play local files first,
// a local file that plays at all comes before a remote stream. Until
// patient ends, it waits for each candidate before the first that plays,
// and, while the cheapest found needs a conversion, for those after it;
// then those still being tried are passed over. A version that needs no
// conversion, repackaged, is not held up for one that might play as it
// is: only those already tried may replace it.
func pick(states []*tried, sides []int, prefer, patient bool) (int, bool) {
	first, best := -1, -1
	for n, t := range states {
		switch {
		case t == nil:
			if patient && (first < 0 || states[best].cost > costCopied) {
				return -1, false
			}
		case !t.plays:
		case first >= 0 && sides[n] != sides[first]:
			return best, true
		case !prefer:
			return n, true
		case first < 0:
			first, best = n, n
		case t.cost < states[best].cost && t.languages.asGoodAs(states[first].languages):
			best = n
		}
		if best >= 0 && states[best].cost == costDirect {
			return best, true
		}
	}
	return best, true
}

// playableToPlay gathers a title's versions and subtitles for a play. It
// lists the versions as VersionsToPlay does: stale lists as they are, so
// complete is false when one was, for the caller to wait for the addons'
// new answers when none of their versions plays. The subtitles are asked
// for the title and, once the versions are in, for the version the play
// starts with as the request names it (see playedIndex), with what
// describes it to subtitle addons (see playback.Service.SubtitleHints):
// the files given for the version come first, then the title's others, as
// an addon may give only those matching the video. The subtitles the
// addons have not given by then are waited for at most subtitleGrace
// more: the title's known now stand for its list when it did not come,
// the others reaching the next PlaybackInfo, as a track switch asks. The
// files listed are kept for user, after those described before (see
// keepSubtitles).
func (h *Handler) playableToPlay(ctx context.Context, user accounts.User, item library.Item, opened accounts.ID,
	mediaSource string) (playable, bool, error) {
	p := playable{item: item, tracks: h.titleTracks(ctx, user, item)}
	if item.Kind == library.KindRecording {
		p.versions = h.recordingVersions(ctx, item)
		return p, true, nil
	}
	title := h.listSubtitles(ctx, user, item, library.SubtitleHints{})
	versions, complete, err := h.Library.VersionsToPlay(ctx, user, item.ID)
	// Before offered, which leaves the failed out of versions in place.
	p.unstarted = h.unstartedTorrent(versions)
	h.offer(ctx, user, &p, versions)
	var hinted <-chan []library.ExternalSubtitle
	if len(p.versions) > 0 {
		played := p.versions[playedIndex(p.versions, opened, mediaSource)]
		if hints := h.Playback.SubtitleHints(ctx, played); hints != (library.SubtitleHints{}) {
			hinted = h.listSubtitles(ctx, user, item, hints)
		}
	}
	grace := time.NewTimer(subtitleGrace)
	defer grace.Stop()
	fromHints, fromTitle, titleCame := awaitSubtitles(grace.C, hinted, title)
	if !titleCame {
		fromTitle, _, _ = h.Library.KnownSubtitles(ctx, user, item.ID)
	}
	p.subtitles = h.keepSubtitles(user, item.ID, appendNewFiles(fromHints, fromTitle))
	return p, complete, err
}

// awaitSubtitles waits until the lists hinted and title send, or until
// grace, and returns them, and whether title came. A nil hinted is not
// waited for.
func awaitSubtitles(grace <-chan time.Time, hinted, title <-chan []library.ExternalSubtitle) (fromHints, fromTitle []library.ExternalSubtitle,
	titleCame bool) {
	for hinted != nil || title != nil {
		select {
		case fromHints = <-hinted:
			hinted = nil
		case fromTitle = <-title:
			title, titleCame = nil, true
		case <-grace:
			hinted, title = nil, nil
		}
	}
	return fromHints, fromTitle, titleCame
}

// listSubtitles lists in the background the subtitle files of item for
// user, asked with hints (see library.Service.SubtitlesFor), and sends
// them, none when they could not be listed.
func (h *Handler) listSubtitles(ctx context.Context, user accounts.User, item library.Item,
	hints library.SubtitleHints) <-chan []library.ExternalSubtitle {
	listed := make(chan []library.ExternalSubtitle, 1)
	go func() {
		var subtitles []library.ExternalSubtitle
		defer func() {
			if cause := recover(); cause != nil {
				logRecovered(h.Logger, "Listing a title's subtitles ended on a panic", cause)
			}
			listed <- subtitles
		}()
		subtitles, err := h.Library.SubtitlesFor(ctx, user, item.ID, hints)
		if err != nil {
			subtitles = nil
		}
	}()
	return listed
}

// playedIndex is the index, among versions, of the version a play starts
// with, as PlaybackInfo tries them: the version mediaSource names, else
// the one the item was opened as (see openedIndex).
func playedIndex(versions []library.Version, opened accounts.ID, mediaSource string) int {
	if id, ok := parseGUID(mediaSource); ok {
		if i := slices.IndexFunc(versions, func(v library.Version) bool { return v.ID == id }); i >= 0 {
			return i
		}
	}
	return openedIndex(opened, versions)
}

// subtitleGrace is how long a play waits for the addons' subtitles once
// its versions are in.
const subtitleGrace = time.Second

// unstartedTorrent reports whether one of versions is a torrent that did
// not start lately, which a play picking no version skips: a play then
// tells that the title cannot be played now.
func (h *Handler) unstartedTorrent(versions []library.Version) bool {
	return slices.ContainsFunc(versions, h.didNotStart)
}
