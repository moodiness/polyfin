package jellyfin

import (
	"context"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
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
// nil when none plays, and those found unreadable.
type chosen struct {
	decided    *decided
	unreadable map[int]bool
}

// tried is what trying a candidate gave: whether it plays, and plays
// without conversion; unreadable when it could not be analyzed.
type tried struct {
	n           int
	decided     decided
	plays       bool
	unconverted bool
	unreadable  bool
}

// choose tries the candidates of p for a play, and returns the version
// chosen. prefer looks for a version that plays without conversion among
// those analyzed now, else takes the first that plays.
func (h *Handler) choose(r *http.Request, user accounts.User, p playable, opened accounts.ID, request playbackInfoRequest,
	c candidates, prefer bool) chosen {
	result := chosen{unreadable: map[int]bool{}}
	at := openedIndex(opened, p.versions)
	allowed := h.Accounts.Conversions(user)
	// The analyses and keyframe indexes started are kept whatever the app
	// does meanwhile, for the next play.
	detached := r.WithContext(context.WithoutCancel(r.Context()))
	try := func(n int, analyze bool) tried {
		i := c.order[n]
		version := p.versions[i]
		var analysis media.Analysis
		if analyze {
			var err error
			if analysis, err = h.Playback.Analyze(detached.Context(), version); err != nil {
				h.Logger.Info("A version could not be analyzed", "addon", version.Addon, "error", err)
				return tried{n: n, unreadable: true}
			}
		} else {
			var known bool
			if analysis, known = h.Playback.Analyzed(detached.Context(), version.ID); !known {
				return tried{n: n}
			}
		}
		permits := h.convertible(allowed, user, version)
		d, ok := h.decide(detached, p, i, version, sourceID(opened, version, i == at), analysis, request, permits, i == c.named)
		if !ok {
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
			if !t.plays && started < attempted {
				start()
			}
			if t.plays && patience == nil {
				timer := time.NewTimer(versionPatience)
				defer timer.Stop()
				patience = timer.C
			}
		case <-patience:
			patient = false
		case <-r.Context().Done():
			return result
		}
	}
	if result.decided != nil {
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

// warmer is what warms the start of a version's first HLS segment ahead
// of the player's request.
type warmer interface {
	Warm(version library.Version, start time.Duration)
}

// warm has the bytes of the version chosen for an HLS play read from
// start, where the play begins, while the app reads the answer.
func (h *Handler) warm(version library.Version, start time.Duration) {
	if w, ok := any(h.Playback).(warmer); ok {
		w.Warm(version, start)
	}
}
