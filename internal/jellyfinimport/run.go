package jellyfinimport

import (
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
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
	// client reads with the credentials the import connected with;
	// sessions read as the users signed in as themselves, by their
	// Jellyfin identifier, until they are read.
	client   *client
	sessions map[string]*client
	// series are the identifiers of the series read, by their Jellyfin
	// identifier: every user sees them the same.
	series map[string]map[string]string
	// plex is a user's own import from Plex, nil for the others.
	plex *ownPlex
	// plexSignOut are the tokens of the sign-ins to Plex the import took
	// over, signed out of Plex once it ends.
	plexSignOut []string
	// plexCollections tells the collections of a Plex server's library
	// sections were read: every account sees them, once.
	plexCollections bool
}

// ownPlex is what a user's own import from Plex reads with besides the
// server's token: the token of the Plex account or Home profile signed
// in, which reads its Watchlist when watchlist is set.
type ownPlex struct {
	account   string
	watchlist bool
}

// update changes the status of the i-th user.
func (r *run) update(i int, change func(*UserStatus)) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	change(&r.status.Users[i])
}

// run imports the watch data of each user in turn, until one fails in a
// way that stops the import (see user) or the import is stopped, then ends
// the sessions it still holds.
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
	var signingOut sync.WaitGroup
	signingOut.Go(r.client.signOut)
	for _, session := range r.sessions {
		signingOut.Go(session.signOut)
	}
	for _, token := range r.plexSignOut {
		signingOut.Go(func() { s.plexSignOut(token) })
	}
	signingOut.Wait()
	s.mu.Lock()
	r.status.State, r.status.Problem, r.status.EndedAt = state, problem, new(s.now().UTC())
	// An import shown ended lets the next one start at once.
	s.busy, s.stop = false, nil
	s.mu.Unlock()
	s.logger.Info("An import ended", "kind", r.status.Kind, "server", r.status.ServerName, "state", state, "problem", problem)
}

// history is what was read of a user: the items played, those with a
// resume point, the favorites, the ratings and the Watchlist, the
// collections and the playlists, and what was read of a Plex server to
// find them.
type history struct {
	played, resumes, favorites, ratings, watchlist []itemJSON
	collections, playlists                         []list
	source                                         plexSource
}

// list is a collection or a playlist of the server: its identifier and
// its name there, and its titles, or entries, in order.
type list struct {
	id, name string
	entries  []itemJSON
}

// plexSource counts what was read of a Plex server for a user (see
// UserStatus): the library sections of movies and shows, the items they
// list, the rows of the playback history, and, among those, the rows of
// other accounts, which are left out.
type plexSource struct {
	sections, listed, rows, otherAccounts int
}

// user imports the watch data of the i-th user, and returns the problem
// that stops the import, if any: a user whose data the server does not let
// the key read fails alone. Once read, the data is saved even if the
// import is stopped meanwhile; stopped while reading, nothing is.
func (r *run) user(ctx context.Context, i int) string {
	jellyfinID := r.status.Users[i].JellyfinID
	c := r.client
	if session := r.sessions[jellyfinID]; session != nil {
		c = session
	}
	r.update(i, func(u *UserStatus) { u.State = UserReading })
	h, err := r.read(ctx, i, c, jellyfinID)
	if ctx.Err() != nil {
		r.update(i, func(u *UserStatus) { u.State, u.Read = UserWaiting, 0 })
		return ""
	}
	if c != r.client {
		// The session read all it was opened for.
		c.signOut()
		delete(r.sessions, jellyfinID)
	}
	// What was read before the server failed is imported.
	problem := readProblem(err)
	r.update(i, func(u *UserStatus) {
		u.State = UserSaving
		u.Sections, u.Listed, u.History = h.source.sections, h.source.listed, h.source.rows
	})
	if err := r.merge(context.WithoutCancel(ctx), i, h, problem == ""); err != nil {
		r.s.logger.Warn("A Jellyfin user's watch data could not be saved", "error", err)
		problem = ProblemInternal
	}
	// A Plex account read from the library sections, the owner's or one
	// read with its own token, that has nothing says so.
	var fromLibrary string
	switch {
	case c.kind == Plex && (r.plex != nil || c != r.client):
		fromLibrary = NothingPlexOwn
	case c.kind == Plex && jellyfinID == plexOwner:
		fromLibrary = NothingPlexOwner
	}
	var done UserStatus
	r.update(i, func(u *UserStatus) {
		u.State, u.Problem = UserDone, problem
		if problem != "" {
			u.State = UserFailed
		} else {
			u.NothingAdded = nothingAdded(c.kind, fromLibrary, *u)
		}
		done = *u
	})
	r.logRead(c.kind, fromLibrary, done, h.source)
	if problem == ProblemForbidden {
		// The key may still read the next users.
		return ""
	}
	return problem
}

// nothingAdded tells why the import of u, which ended without a problem,
// added nothing, from a server of kind, fromLibrary being the reason for
// a Plex account read from the library sections: empty when it added
// something, or read only titles it could not match, or lists it could
// not create, which say why.
func nothingAdded(kind Kind, fromLibrary string, u UserStatus) string {
	switch {
	case u.Played+u.Resumed+u.Favorites+u.Ratings+u.Watchlist+u.Collections+u.Playlists+u.ListTitles > 0:
		return ""
	case u.UpToDate > 0:
		return NothingUpToDate
	case u.Read > 0 || len(u.Skipped) > 0:
		return ""
	case kind != Plex:
		return NothingPlayed
	case u.Sections == 0:
		return NothingNoSections
	case fromLibrary != "":
		return fromLibrary
	}
	return NothingPlexNoHistory
}

// logRead logs, at Info, what the import of u read and did, never the
// server's address nor a key; fromLibrary is as nothingAdded takes it.
func (r *run) logRead(kind Kind, fromLibrary string, u UserStatus, source plexSource) {
	attrs := []any{"kind", kind, "user", u.JellyfinID, "state", u.State}
	if kind == Plex {
		attrs = append(attrs, "owner", fromLibrary == NothingPlexOwner, "signedIn", fromLibrary == NothingPlexOwn,
			"sections", u.Sections, "listed", u.Listed)
		if fromLibrary == "" {
			attrs = append(attrs, "history", u.History, "otherAccounts", source.otherAccounts)
		}
	}
	attrs = append(attrs, "read", u.Read, "matched", u.Matched, "played", u.Played, "resumed", u.Resumed, "favorites", u.Favorites,
		"ratings", u.Ratings, "watchlist", u.Watchlist, "collections", u.Collections, "playlists", u.Playlists, "listTitles", u.ListTitles,
		"skippedLists", len(u.Skipped), "upToDate", u.UpToDate, "undated", u.Undated, "unmatched", u.UnmatchedCount,
		"nothingAdded", u.NothingAdded, "problem", u.Problem)
	r.s.logger.Info("An import read a user's watch data", attrs...)
}

// readProblem names what stopped reading a server.
func readProblem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrKeyRefused):
		return ProblemKeyRefused
	case errors.Is(err, ErrForbidden):
		return ProblemForbidden
	case errors.Is(err, ErrNotJellyfin):
		return ProblemNotJellyfin
	}
	return ProblemUnreachable
}

// read reads, with c, the watch data of user, and the identifiers of the
// series of its episodes, and the extras the import brings. A Plex account
// read with its own token, a user's own import's or one Plex gives the
// owner, and the owner read with theirs, are read from the libraries,
// with their ratings and playlists; a user's own Plex import reads the
// account's Watchlist too when asked. A Plex server's collections are read
// once, with the import's own token; a Jellyfin or Emby server's, as each
// user sees them.
func (r *run) read(ctx context.Context, i int, c *client, user string) (history, error) {
	extras := r.status.Extras
	found := func() { r.update(i, func(u *UserStatus) { u.Read++ }) }
	if c.kind == Plex {
		own := r.plex != nil || c != r.client || user == plexOwner
		h, err := c.plexWatched(ctx, user, own, extras.Ratings, r.series, found)
		if err != nil {
			return h, err
		}
		if extras.Collections && !r.plexCollections {
			r.plexCollections = true
			if h.collections, err = r.client.plexCollections(ctx, r.series, found); err != nil {
				return h, err
			}
		}
		if extras.Playlists && own {
			if h.playlists, err = c.plexPlaylists(ctx, r.series, found); err != nil {
				return h, err
			}
		}
		if r.plex == nil || !r.plex.watchlist || r.s.watchlist == nil {
			return h, nil
		}
		h.watchlist, err = r.s.plexWatchlist(ctx, r.plex.account, found)
		return h, err
	}
	var h history
	// liked are the items with a thumb up, rated or not: Jellyfin and Emby
	// cannot list the items rated otherwise.
	var liked []itemJSON
	passes := []struct {
		types, filter string
		into          *[]itemJSON
	}{
		{"Movie,Episode", "IsPlayed", &h.played},
		{"Movie,Episode", "IsResumable", &h.resumes},
		{"Movie,Series,Episode", "IsFavorite", &h.favorites},
	}
	if extras.Ratings {
		passes = append(passes, struct {
			types, filter string
			into          *[]itemJSON
		}{"Movie,Series,Episode", "Likes", &liked})
	}
	for _, pass := range passes {
		err := c.items(ctx, user, pass.types, pass.filter, func(item itemJSON) {
			*pass.into = append(*pass.into, item)
			found()
		})
		if err != nil {
			return h, err
		}
	}
	if extras.Ratings {
		h.ratings, h.favorites = rated(slices.Concat(h.played, h.resumes, h.favorites, liked), h.favorites)
	}
	if extras.Collections {
		lists, err := r.lists(ctx, c, user, "BoxSet", c.collection, found)
		if err != nil {
			return h, err
		}
		h.collections = lists
	}
	if extras.Playlists {
		lists, err := r.lists(ctx, c, user, "Playlist", c.playlist, found)
		if err != nil {
			return h, err
		}
		h.playlists = lists
	}
	var missing []string
	seen := map[string]bool{}
	all := [][]itemJSON{h.played, h.resumes, h.favorites}
	for _, read := range slices.Concat(h.collections, h.playlists) {
		all = append(all, read.entries)
	}
	for _, items := range all {
		for _, item := range items {
			if _, read := r.series[item.SeriesID]; item.Type == "Episode" && item.SeriesID != "" && !read && !seen[item.SeriesID] {
				seen[item.SeriesID] = true
				missing = append(missing, item.SeriesID)
			}
		}
	}
	providers, err := c.providers(ctx, user, missing)
	if err != nil {
		return h, err
	}
	for _, id := range missing {
		// A series the server did not return has no identifier to match.
		r.series[id] = providers[id]
	}
	return h, nil
}

// rated sorts out the items read, each once: those with a rating, which
// Jellyfin and Emby give from 0 to 10, are rated as they are, and those
// with only a thumb up, as Emby may keep, join favorites, as Letterboxd's
// liked movies can. A thumb down alone is left out. It returns the ratings,
// and the favorites with the items liked.
func rated(items, favorites []itemJSON) ([]itemJSON, []itemJSON) {
	var ratings []itemJSON
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		data := item.UserData
		switch {
		case data.Rating != nil:
			item.rating = new(min(max(*data.Rating, 0), 10))
			ratings = append(ratings, item)
		case data.Likes != nil && *data.Likes && !data.IsFavorite:
			favorites = append(favorites, item)
		}
	}
	return ratings, favorites
}

// The types of a playlist that holds no video: Jellyfin and Emby tell a
// playlist's by its items'.
var nonVideo = map[string]bool{"Audio": true, "Photo": true, "Book": true}

// lists reads, with c, the lists of type, BoxSet or Playlist, that user
// sees, and their titles with entries, handing each title read to found.
// A playlist holds movies and episodes only: its other entries, and the
// playlists of music, photos or books, are left out.
func (r *run) lists(ctx context.Context, c *client, user, kind string,
	entries func(ctx context.Context, user, id string, found func(itemJSON)) error, found func()) ([]list, error) {
	var listed []itemJSON
	if err := c.items(ctx, user, kind, "", func(item itemJSON) { listed = append(listed, item) }); err != nil {
		return nil, err
	}
	var lists []list
	for _, item := range listed {
		if item.Type != kind || kind == "Playlist" && nonVideo[item.MediaType] {
			continue
		}
		read := list{id: item.ID, name: item.Name}
		err := entries(ctx, user, item.ID, func(entry itemJSON) {
			if kind == "Playlist" && entry.Type != "Movie" && entry.Type != "Episode" {
				return
			}
			read.entries = append(read.entries, entry)
			found()
		})
		if err != nil {
			return lists, err
		}
		lists = append(lists, read)
	}
	return lists, nil
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
	if item.unmatchable != "" {
		return nil, true
	}
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
		numbers := episodeNumbers(item)
		if !ok || len(numbers) == 0 {
			return nil, true
		}
		ref.Episode, ref.Season, ref.Name = true, *item.ParentIndexNumber, item.SeriesName
		for _, number := range numbers {
			ref.Number = number
			refs = append(refs, ref)
		}
		return refs, true
	}
	return nil, true
}

// episodeNumbers are the numbers of the episodes item, an episode, holds:
// its own, and the next ones up to the last of a file holding several;
// none when it is not numbered.
func episodeNumbers(item itemJSON) []int {
	if item.ParentIndexNumber == nil || item.IndexNumber == nil || *item.ParentIndexNumber < 0 || *item.IndexNumber <= 0 {
		return nil
	}
	first, last := *item.IndexNumber, *item.IndexNumber
	if end := item.IndexNumberEnd; end != nil && *end > last && *end-last < maxEpisodesPerFile {
		last = *end
	}
	numbers := make([]int, 0, last-first+1)
	for number := first; number <= last; number++ {
		numbers = append(numbers, number)
	}
	return numbers
}

// title names item as the titles an import did not find are kept, and
// linked by hand (see package importlinks): its key, and its label. The
// key is the title's identifiers, an episode's series', when it has some;
// else its identifier on the server, an episode's series', with the
// server's, as rating keys of different Plex servers meet; else, for a
// title deleted from Plex's libraries, which has none, its name, an
// episode's series', with the server's: each stays the same from one
// import to the next. It is empty for an item of no title, or of no name.
func (r *run) title(item itemJSON) (string, importlinks.Label) {
	label := importlinks.Label{Name: item.Name, Year: item.ProductionYear}
	var ref library.TitleRef
	id := item.ID
	switch item.Type {
	case "Movie":
		label.Kind = importlinks.KindMovie
		ref, _ = identified(item.ProviderIDs, false)
	case "Series":
		label.Kind = importlinks.KindSeries
		ref, _ = identified(item.ProviderIDs, true)
		ref.Series = true
	case "Episode":
		// An episode is named by its series, whose year the episode's is
		// not.
		label = importlinks.Label{Kind: importlinks.KindEpisode, Name: item.SeriesName}
		if item.ParentIndexNumber != nil && *item.ParentIndexNumber >= 0 {
			label.Season = *item.ParentIndexNumber
		}
		if item.IndexNumber != nil {
			label.Episode = *item.IndexNumber
		}
		ref, _ = identified(r.series[item.SeriesID], true)
		ref.Episode = true
		id = item.SeriesID
	default:
		return "", label
	}
	label.IDs = importlinks.RefIDs(ref)
	if key := importlinks.RefKey(ref); key != "" {
		return key, label
	}
	kind := label.Kind
	if kind == importlinks.KindEpisode {
		kind = importlinks.KindSeries
	}
	switch {
	case id != "" && !strings.HasPrefix(id, plexDeleted):
		return importlinks.Key(kind, "item", r.status.source, id), label
	case label.Name != "":
		return importlinks.Key(kind, "name", r.status.source, label.Name), label
	}
	return "", label
}

// linkedRefs names the titles item is by the link its title was given by
// hand among linked: the movie or the series linked, or, for an episode,
// the series' episodes of item's numbers. It reports false when the title
// has no link, or one of another kind, or the episode no number.
func (r *run) linkedRefs(linked importlinks.Linked, item itemJSON) ([]library.TitleRef, bool) {
	key, label := r.title(item)
	if _, ok := linked[key]; !ok || key == "" {
		return nil, false
	}
	if label.Kind != importlinks.KindEpisode {
		ref, ok := linked.Ref(key, label)
		return []library.TitleRef{ref}, ok
	}
	var refs []library.TitleRef
	for _, number := range episodeNumbers(item) {
		label.Episode = number
		ref, ok := linked.Ref(key, label)
		if !ok {
			return nil, false
		}
		refs = append(refs, ref)
	}
	return refs, len(refs) > 0
}

// unfound are the entries of the items of noMatch, which no title matched
// for their reasons, with what h holds of them that the import would have
// brought: the played mark, its latest date and the most plays, the
// newest dated resume point, the rating, and the Plex Watchlist. A file
// holding several episodes gives one entry for each; an item title names
// no key of, and an episode not numbered, none.
func (r *run) unfound(h history, noMatch []itemJSON, reasons map[string]string) []importlinks.Entry {
	byID := make(map[string]*importlinks.Entry, len(noMatch))
	for _, item := range noMatch {
		if key, label := r.title(item); key != "" {
			byID[item.ID] = &importlinks.Entry{Key: key, Label: label, Reason: reasons[item.ID]}
		}
	}
	for _, item := range h.played {
		if e := byID[item.ID]; e != nil {
			e.Played = true
			if at := date(item.UserData.LastPlayedDate); at != nil && (e.PlayedAt == nil || at.After(*e.PlayedAt)) {
				e.PlayedAt = at
			}
			e.Plays = max(e.Plays, item.UserData.PlayCount)
		}
	}
	for _, item := range h.resumes {
		// As merge does, a resume point without a date is left out.
		if e := byID[item.ID]; e != nil {
			if at := date(item.UserData.LastPlayedDate); at != nil && (e.Resume == nil || at.After(e.Resume.At)) {
				e.Resume = &userdata.ImportedResume{Percent: item.UserData.PlayedPercentage,
					Position: time.Duration(item.UserData.PlaybackPositionTicks) * tick, Runtime: time.Duration(item.RunTimeTicks) * tick,
					At: *at}
			}
		}
	}
	for _, item := range h.ratings {
		if e := byID[item.ID]; e != nil && e.Rating == nil {
			e.Rating = item.rating
		}
	}
	for _, item := range h.watchlist {
		if e := byID[item.ID]; e != nil {
			e.Watchlist = watchlist.Import
		}
	}
	kept := make([]importlinks.Entry, 0, len(noMatch))
	for _, item := range noMatch {
		e := byID[item.ID]
		switch {
		case e == nil:
		case e.Kind != importlinks.KindEpisode:
			kept = append(kept, *e)
		default:
			for _, number := range episodeNumbers(item) {
				each := *e
				each.Episode = number
				kept = append(kept, each)
			}
		}
	}
	return kept
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
// still unplayed, favorites, ratings where Polyfin has none, the
// Watchlist, and the collections and playlists with the titles found. A
// title linked by hand in the user's list of the server's kind is found
// as its link names it. The titles it did not find are counted, and kept
// in that list with what it would have brought of them, whole telling the
// user's data was read whole (see importlinks.Store.Save); it counts those
// it found, and what of theirs Polyfin already had.
func (r *run) merge(ctx context.Context, i int, h history, whole bool) error {
	user := r.status.Users[i].User
	var linked importlinks.Linked
	if r.s.links != nil {
		var err error
		if linked, err = r.s.links.Linked(ctx, user, string(r.status.Kind)); err != nil {
			return err
		}
	}
	var refs []library.TitleRef
	// spans are the refs of each item, in order: played, resumes,
	// favorites, ratings, Watchlist, then the entries of the collections
	// and those of the playlists, one list after the other.
	type span struct{ start, end int }
	lists := [][]itemJSON{h.played, h.resumes, h.favorites, h.ratings, h.watchlist, entries(h.collections), entries(h.playlists)}
	spans := make([][]span, len(lists))
	// noMatch are the items no title matched, each once, in the order
	// read, and reasons why, by their identifier.
	var noMatch []itemJSON
	reasons := map[string]string{}
	report := func(item itemJSON, reason string) {
		if _, seen := reasons[item.ID]; !seen {
			reasons[item.ID] = reason
			noMatch = append(noMatch, item)
		}
	}
	for l, list := range lists {
		spans[l] = make([]span, len(list))
		for j, item := range list {
			found, known := r.refs(item)
			if len(linked) > 0 {
				if byLink, ok := r.linkedRefs(linked, item); ok {
					found, known = byLink, true
				}
			}
			switch {
			case known && len(found) == 0 && item.unmatchable != "":
				report(item, item.unmatchable)
			case known && len(found) == 0:
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
	// reporting it when it has identifiers that none matched, and counting
	// it when some did.
	matched := 0
	targetsOf := func(l, j int) []library.TitleTarget {
		var all []library.TitleTarget
		sp := spans[l][j]
		for _, found := range targets[sp.start:sp.end] {
			all = append(all, found...)
		}
		switch {
		case len(all) > 0:
			matched++
		case sp.end > sp.start:
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
	undated := 0
	for j, item := range h.resumes {
		found := targetsOf(1, j)
		at := date(item.UserData.LastPlayedDate)
		if at == nil {
			// Jellyfin dates every position it keeps, and Plex the ones
			// played: one without a date could not be told older or newer
			// than Polyfin's.
			if len(found) > 0 {
				undated++
			}
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
	// A title rated twice, as the files of one episode, keeps the first
	// rating read.
	ratings := map[accounts.ID]*rating{}
	var ratingOrder []accounts.ID
	for j, item := range h.ratings {
		for _, t := range targetsOf(3, j) {
			if _, listed := ratings[t.ID]; !listed {
				ratings[t.ID] = &rating{item: itemOf(t), value: *item.rating}
				ratingOrder = append(ratingOrder, t.ID)
			}
		}
	}
	// A title goes on the Watchlist once, as its first item.
	var titles []watchlist.Title
	listed := map[accounts.ID]bool{}
	for j := range h.watchlist {
		if found := targetsOf(4, j); len(found) > 0 && !listed[found[0].ID] {
			listed[found[0].ID] = true
			titles = append(titles, watchlist.Title{ID: found[0].ID})
		}
	}
	// Each list holds, in order, the first item each title of its entries
	// designates.
	itemsOf := func(l int, read []list) [][]accounts.ID {
		all := make([][]accounts.ID, len(read))
		j := 0
		for k, each := range read {
			for range each.entries {
				sp := spans[l][j]
				var ids []accounts.ID
				for _, found := range targets[sp.start:sp.end] {
					if len(found) > 0 {
						ids = append(ids, found[0].ID)
					}
				}
				switch {
				case len(ids) > 0:
					matched++
				case sp.end > sp.start:
					report(lists[l][j], ReasonNotFound)
				}
				all[k] = append(all[k], ids...)
				j++
			}
		}
		return all
	}
	collectionItems, playlistItems := itemsOf(5, h.collections), itemsOf(6, h.playlists)
	r.update(i, func(u *UserStatus) {
		u.UnmatchedCount = len(noMatch)
		u.Matched, u.Undated = matched, undated
	})
	if r.s.links != nil {
		if err := r.s.links.Save(ctx, user, string(r.status.Kind), r.unfound(h, noMatch, reasons), whole); err != nil {
			return err
		}
	}

	settings := r.s.settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	if err := r.change(ctx, i, user, markOrder, func(id accounts.ID) userdata.Item { return marks[id].item },
		func(id accounts.ID, d *userdata.Data, u *UserStatus) {
			m := marks[id]
			if d.ImportPlayed(m.at) {
				u.Played++
			} else {
				u.UpToDate++
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
			case userdata.ResumeKept:
				u.UpToDate++
			}
		}); err != nil {
		return err
	}
	if err := r.change(ctx, i, user, favoriteOrder, func(id accounts.ID) userdata.Item { return favorites[id] },
		func(_ accounts.ID, d *userdata.Data, u *UserStatus) {
			if !d.Favorite {
				d.Favorite = true
				u.Favorites++
			} else {
				u.UpToDate++
			}
		}); err != nil {
		return err
	}
	// A rating Polyfin has stays, as an imported Letterboxd rating does.
	if err := r.change(ctx, i, user, ratingOrder, func(id accounts.ID) userdata.Item { return ratings[id].item },
		func(id accounts.ID, d *userdata.Data, u *UserStatus) {
			if d.Rating == nil {
				d.Rating = new(ratings[id].value)
				u.Ratings++
			} else {
				u.UpToDate++
			}
		}); err != nil {
		return err
	}
	if len(titles) > 0 && r.s.watchlist != nil {
		added, err := r.s.watchlist.Save(ctx, user, watchlist.Import, titles, false)
		if err != nil {
			return err
		}
		r.update(i, func(u *UserStatus) { u.Watchlist += added })
	}
	return r.saveLists(ctx, i, user, h, collectionItems, playlistItems)
}

// entries are the entries of lists, one list after the other.
func entries(lists []list) []itemJSON {
	var all []itemJSON
	for _, each := range lists {
		all = append(all, each.entries...)
	}
	return all
}

// saveLists brings the collections of h, which collectionItems hold, and
// its playlists, which playlistItems hold, into Polyfin, the playlists
// into the account of the i-th user, user, counting those it created, the
// titles it added to those an earlier import brought, and those that had
// them all already, and listing those it did not create, as no title of
// theirs was found. A collection is that of the user who starts the
// import; one another user's import brought before gets the titles found
// here added.
func (r *run) saveLists(ctx context.Context, i int, user accounts.ID, h history, collectionItems, playlistItems [][]accounts.ID) error {
	for _, kind := range []struct {
		name  string
		read  []list
		items [][]accounts.ID
		save  func(ctx context.Context, server, source string, owner accounts.ID, name string,
			items []accounts.ID) (accounts.ID, bool, int, error)
		owner accounts.ID
	}{
		{ListCollection, h.collections, collectionItems, r.s.collections.Import, r.status.StartedBy},
		{ListPlaylist, h.playlists, playlistItems, r.s.playlists.Import, user},
	} {
		for k, each := range kind.read {
			id, created, added, err := kind.save(ctx, r.status.source, each.id, kind.owner, each.name, kind.items[k])
			if err != nil {
				return err
			}
			r.update(i, func(u *UserStatus) {
				switch {
				case created && kind.name == ListCollection:
					u.Collections++
				case created:
					u.Playlists++
				case id == accounts.ID{}:
					u.Skipped = append(u.Skipped, SkippedList{Kind: kind.name, Name: each.name})
				case added > 0:
					u.ListTitles += added
				default:
					u.UpToDate++
				}
			})
		}
	}
	return nil
}

// rating is a rating an import adds.
type rating struct {
	item  userdata.Item
	value float64
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
			u.Ratings += counts.Ratings
			u.UpToDate += counts.UpToDate
		})
	}
	return nil
}
