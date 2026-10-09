package jellyfinimport

import (
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// batch is how many items an import resolves and writes at a time.
const batch = 500

// maxEpisodesPerFile bounds the episodes one file holding several counts
// for, against a numbering gone wrong.
const maxEpisodesPerFile = 10

// tick is Jellyfin's unit of time.
const tick = 100 * time.Nanosecond

var imdbPattern = regexp.MustCompile(`^tt\d+$`)

// newID returns a random identifier for an import.
func newID() string {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id.String()
}

// run is an import running.
type run struct {
	s *Service
	// status is the import's, which s.mu guards.
	status *Status
	client *client
	// series are the identifiers of the series read, by their Jellyfin
	// identifier: every user sees them the same.
	series map[string]map[string]string
}

// update changes the status of the i-th user.
func (r *run) update(i int, change func(*UserStatus)) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	change(&r.status.Users[i])
}

// run imports the watch data of each user in turn, until one fails or the
// import is stopped.
func (r *run) run(ctx context.Context) {
	s := r.s
	state, problem := StateDone, ""
	for i := range r.status.Users {
		if ctx.Err() != nil {
			break
		}
		if problem = r.user(ctx, i); problem != "" {
			state = StateFailed
			break
		}
	}
	if state == StateDone && ctx.Err() != nil {
		state = StateStopped
	}
	s.mu.Lock()
	r.status.State, r.status.Problem, r.status.EndedAt = state, problem, new(s.now().UTC())
	s.mu.Unlock()
	s.logger.Info("A Jellyfin import ended", "server", r.status.ServerName, "state", state, "problem", problem)
}

// history is what was read of a user: the items played, those with a
// resume point, and the favorites.
type history struct {
	played, resumes, favorites []itemJSON
}

// user imports the watch data of the i-th user, and returns the problem
// that stops the import, if any. Once read, the data is saved even if the
// import is stopped meanwhile; stopped while reading, nothing is.
func (r *run) user(ctx context.Context, i int) string {
	jellyfinID := r.status.Users[i].JellyfinID
	r.update(i, func(u *UserStatus) { u.State = UserReading })
	h, err := r.read(ctx, i, jellyfinID)
	if ctx.Err() != nil {
		r.update(i, func(u *UserStatus) { u.State, u.Read = UserWaiting, 0 })
		return ""
	}
	// What was read before the server failed is imported.
	problem := readProblem(err)
	r.update(i, func(u *UserStatus) { u.State = UserSaving })
	if err := r.merge(context.WithoutCancel(ctx), i, h); err != nil {
		r.s.logger.Warn("A Jellyfin user's watch data could not be saved", "error", err)
		problem = ProblemInternal
	}
	r.update(i, func(u *UserStatus) {
		u.State, u.Problem = UserDone, problem
		if problem != "" {
			u.State = UserFailed
		}
	})
	return problem
}

// readProblem names what stopped reading a server.
func readProblem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrKeyRefused):
		return ProblemKeyRefused
	case errors.Is(err, ErrNotJellyfin):
		return ProblemNotJellyfin
	}
	return ProblemUnreachable
}

// read reads the watch data of user, and the identifiers of the series of
// its episodes.
func (r *run) read(ctx context.Context, i int, user string) (history, error) {
	var h history
	for _, pass := range []struct {
		types, filter string
		into          *[]itemJSON
	}{
		{"Movie,Episode", "IsPlayed", &h.played},
		{"Movie,Episode", "IsResumable", &h.resumes},
		{"Movie,Series,Episode", "IsFavorite", &h.favorites},
	} {
		err := r.client.items(ctx, user, pass.types, pass.filter, func(item itemJSON) {
			*pass.into = append(*pass.into, item)
			r.update(i, func(u *UserStatus) { u.Read++ })
		})
		if err != nil {
			return h, err
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, list := range [][]itemJSON{h.played, h.resumes, h.favorites} {
		for _, item := range list {
			if _, read := r.series[item.SeriesID]; item.Type == "Episode" && item.SeriesID != "" && !read && !seen[item.SeriesID] {
				seen[item.SeriesID] = true
				missing = append(missing, item.SeriesID)
			}
		}
	}
	found, err := r.client.providers(ctx, user, missing)
	if err != nil {
		return h, err
	}
	for _, id := range missing {
		// A series the server did not return has no identifier to match.
		r.series[id] = found[id]
	}
	return h, nil
}

// provider is the identifier of providers named name, whatever its case.
func provider(providers map[string]string, name string) string {
	for key, value := range providers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// identified names the title providers identify, as Jellyfin names them:
// Imdb, Tmdb, and for a series Tvdb, as Polyfin looks movies up by IMDb and
// TMDB only. It reports false when none is usable.
func identified(providers map[string]string, series bool) (library.TitleRef, bool) {
	var ref library.TitleRef
	if imdb := provider(providers, "Imdb"); imdbPattern.MatchString(imdb) {
		ref.IMDb = imdb
	}
	if id, err := strconv.Atoi(provider(providers, "Tmdb")); err == nil && id > 0 {
		ref.TMDB = id
	}
	if id, err := strconv.Atoi(provider(providers, "Tvdb")); err == nil && id > 0 && series {
		ref.TVDB = id
	}
	return ref, ref.IMDb != "" || ref.TMDB > 0 || ref.TVDB > 0
}

// refs names the titles item is: a movie or a series by its identifiers,
// an episode by its series' and its numbers, every episode of a file
// holding several. None means it cannot be matched; known is false for an
// episode whose series was not read.
func (r *run) refs(item itemJSON) (refs []library.TitleRef, known bool) {
	switch item.Type {
	case "Movie":
		if ref, ok := identified(item.ProviderIDs, false); ok {
			ref.Name = item.Name
			return []library.TitleRef{ref}, true
		}
	case "Series":
		if ref, ok := identified(item.ProviderIDs, true); ok {
			ref.Series, ref.Name = true, item.Name
			return []library.TitleRef{ref}, true
		}
	case "Episode":
		providers, read := r.series[item.SeriesID]
		if !read && item.SeriesID != "" {
			return nil, false
		}
		ref, ok := identified(providers, true)
		ref.Name = item.SeriesName
		if !ok || item.ParentIndexNumber == nil || item.IndexNumber == nil || *item.ParentIndexNumber < 0 || *item.IndexNumber <= 0 {
			return nil, true
		}
		ref.Episode, ref.Season = true, *item.ParentIndexNumber
		last := *item.IndexNumber
		if end := item.IndexNumberEnd; end != nil && *end > last && *end-last < maxEpisodesPerFile {
			last = *end
		}
		for number := *item.IndexNumber; number <= last; number++ {
			ref.Number = number
			refs = append(refs, ref)
		}
		return refs, true
	}
	return nil, true
}

// unmatched describes item, which no title matched for reason.
func unmatched(item itemJSON, reason string) Unmatched {
	u := Unmatched{Name: item.Name, Kind: strings.ToLower(item.Type), Year: item.ProductionYear, Reason: reason}
	if item.Type == "Episode" {
		u.Series, u.Season, u.Episode = item.SeriesName, item.ParentIndexNumber, item.IndexNumber
	}
	return u
}

// mark is a played mark an import adds: at the latest date Jellyfin
// gives, nil when it gives none, and the most plays it counts.
type mark struct {
	item  userdata.Item
	at    *time.Time
	plays int
}

// point is a resume point an import adds.
type point struct {
	item userdata.Item
	userdata.ImportedResume
}

// merge finds the titles of h and adds them to the data of the i-th
// user's Polyfin account: played marks, then resume points of the titles
// still unplayed, then favorites. It reports the titles it did not find.
func (r *run) merge(ctx context.Context, i int, h history) error {
	user := r.status.Users[i].User
	var refs []library.TitleRef
	// spans are the refs of each item, in order: played, resumes,
	// favorites.
	type span struct{ start, end int }
	lists := [][]itemJSON{h.played, h.resumes, h.favorites}
	spans := make([][]span, len(lists))
	var noMatch []Unmatched
	reported := map[string]bool{}
	report := func(item itemJSON, reason string) {
		if !reported[item.ID] {
			reported[item.ID] = true
			noMatch = append(noMatch, unmatched(item, reason))
		}
	}
	for l, list := range lists {
		spans[l] = make([]span, len(list))
		for j, item := range list {
			found, known := r.refs(item)
			if known && len(found) == 0 {
				report(item, ReasonNoIdentifier)
			}
			spans[l][j] = span{len(refs), len(refs) + len(found)}
			refs = append(refs, found...)
		}
	}
	targets := make([][]library.TitleTarget, 0, len(refs))
	for start := 0; start < len(refs); start += batch {
		found, err := r.s.titles.Resolve(ctx, refs[start:min(start+batch, len(refs))])
		if err != nil {
			return err
		}
		targets = append(targets, found...)
	}
	// targetsOf lists the items the j-th item of list l designates,
	// reporting it when it has identifiers that none matched.
	targetsOf := func(l, j int) []library.TitleTarget {
		var all []library.TitleTarget
		sp := spans[l][j]
		for _, found := range targets[sp.start:sp.end] {
			all = append(all, found...)
		}
		if sp.end > sp.start && len(all) == 0 {
			report(lists[l][j], ReasonNotFound)
		}
		return all
	}
	itemOf := func(t library.TitleTarget) userdata.Item {
		return userdata.Item{ID: t.ID, Series: t.Series, Season: t.Season}
	}

	marks := map[accounts.ID]*mark{}
	var markOrder []accounts.ID
	for j, item := range h.played {
		at := date(item.UserData.LastPlayedDate)
		for _, t := range targetsOf(0, j) {
			m := marks[t.ID]
			if m == nil {
				m = &mark{item: itemOf(t)}
				marks[t.ID] = m
				markOrder = append(markOrder, t.ID)
			}
			if at != nil && (m.at == nil || at.After(*m.at)) {
				m.at = at
			}
			m.plays = max(m.plays, item.UserData.PlayCount)
		}
	}
	points := map[accounts.ID]*point{}
	var pointOrder []accounts.ID
	for j, item := range h.resumes {
		found := targetsOf(1, j)
		at := date(item.UserData.LastPlayedDate)
		if at == nil {
			// Jellyfin dates every position it keeps; one without a date
			// could not be told older or newer than Polyfin's.
			continue
		}
		for _, t := range found {
			if p := points[t.ID]; p != nil && !at.After(p.At) {
				continue
			}
			if _, listed := points[t.ID]; !listed {
				pointOrder = append(pointOrder, t.ID)
			}
			runtime := time.Duration(item.RunTimeTicks) * tick
			if runtime <= 0 {
				runtime = t.Runtime
			}
			points[t.ID] = &point{item: itemOf(t), ImportedResume: userdata.ImportedResume{Percent: item.UserData.PlayedPercentage,
				Position: time.Duration(item.UserData.PlaybackPositionTicks) * tick, Runtime: runtime, At: *at}}
		}
	}
	favorites := map[accounts.ID]userdata.Item{}
	var favoriteOrder []accounts.ID
	for j := range h.favorites {
		for _, t := range targetsOf(2, j) {
			if _, listed := favorites[t.ID]; !listed {
				favorites[t.ID] = itemOf(t)
				favoriteOrder = append(favoriteOrder, t.ID)
			}
		}
	}
	r.update(i, func(u *UserStatus) {
		u.UnmatchedCount = len(noMatch)
		u.Unmatched = noMatch[:min(len(noMatch), maxUnmatched)]
	})

	settings := r.s.settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	if err := r.change(ctx, i, user, markOrder, func(id accounts.ID) userdata.Item { return marks[id].item },
		func(id accounts.ID, d *userdata.Data, u *UserStatus) {
			m := marks[id]
			if d.ImportPlayed(m.at) {
				u.Played++
			}
			// Polyfin counted its own plays: the larger count stands.
			d.PlayCount = max(d.PlayCount, m.plays)
		}); err != nil {
		return err
	}
	if err := r.change(ctx, i, user, pointOrder, func(id accounts.ID) userdata.Item { return points[id].item },
		func(id accounts.ID, d *userdata.Data, u *UserStatus) {
			switch d.ImportResume(points[id].ImportedResume, thresholds) {
			case userdata.ResumePlayed:
				u.Played++
			case userdata.ResumeSet:
				u.Resumed++
			}
		}); err != nil {
		return err
	}
	return r.change(ctx, i, user, favoriteOrder, func(id accounts.ID) userdata.Item { return favorites[id] },
		func(_ accounts.ID, d *userdata.Data, u *UserStatus) {
			if !d.Favorite {
				d.Favorite = true
				u.Favorites++
			}
		})
}

// change applies change to the data user keeps of the items ids name,
// batch by batch, counting what it did in the i-th user's status once each
// batch is saved.
func (r *run) change(ctx context.Context, i int, user accounts.ID, ids []accounts.ID, itemOf func(accounts.ID) userdata.Item,
	change func(accounts.ID, *userdata.Data, *UserStatus)) error {
	for start := 0; start < len(ids); start += batch {
		chunk := ids[start:min(start+batch, len(ids))]
		items := make([]userdata.Item, 0, len(chunk))
		for _, id := range chunk {
			items = append(items, itemOf(id))
		}
		var counts UserStatus
		if _, err := r.s.userData.ChangeEach(ctx, user, items, func(item userdata.Item, d *userdata.Data) {
			change(item.ID, d, &counts)
		}); err != nil {
			return err
		}
		r.update(i, func(u *UserStatus) {
			u.Played += counts.Played
			u.Resumed += counts.Resumed
			u.Favorites += counts.Favorites
		})
	}
	return nil
}
