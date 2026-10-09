// Package anime maps anime between the identifiers of AniDB, Kitsu,
// MyAnimeList, AniList, TVDB, TMDB and IMDb, and the episodes of AniDB's
// entries to TVDB's seasons and episodes. AniDB, and the services that
// follow it, make each season of a series an entry of its own, its
// episodes numbered from 1, where Polyfin's titles follow the seasons of
// IMDb and TVDB.
//
// It reads two community lists: Fribb/anime-lists' anime-list-full.json
// for the identifiers, and Anime-Lists/anime-lists' anime-list-master.xml
// for the episodes. A copy of each is kept in the service's folder and
// read at start; each is downloaded again once a day, with a conditional
// request, and a download that fails leaves the last copy in use. Lookups
// read what is in memory and never wait for a download.
package anime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The lists' addresses.
const (
	IDsURL      = "https://raw.githubusercontent.com/Fribb/anime-lists/master/anime-list-full.json"
	EpisodesURL = "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-list-master.xml"
)

const (
	// maxListBytes bounds a list downloaded: the identifier list takes
	// about 20 MB, the episode list less.
	maxListBytes = 128 << 20
	// stateFile keeps, beside the copies, what the conditional requests
	// need and when each list was last checked.
	stateFile = "lists.json"
)

// timing is how often the lists are refreshed. Tests shorten it.
type timing struct {
	// refreshEvery is how long a list is used before it is checked again.
	refreshEvery time.Duration
	// retryFirst is the wait after a failed download, doubling up to
	// retryMax.
	retryFirst, retryMax time.Duration
	// download bounds each download.
	download time.Duration
}

var defaultTiming = timing{refreshEvery: 24 * time.Hour, retryFirst: 10 * time.Minute, retryMax: 6 * time.Hour, download: 5 * time.Minute}

// Options are what the service needs.
type Options struct {
	// Dir is the folder the copies of the lists are kept in, which the
	// service creates.
	Dir string
	// Version is Polyfin's, which the downloads tell.
	Version string
	Logger  *slog.Logger
	// IDsURL and EpisodesURL replace the lists' addresses. Tests point
	// them at fakes; the public lists are read otherwise.
	IDsURL, EpisodesURL string
}

// Service maps anime between identifiers and numberings. A nil Service
// maps nothing.
type Service struct {
	dir       string
	userAgent string
	logger    *slog.Logger
	client    *http.Client
	now       func() time.Time
	timing    timing
	ids       atomic.Pointer[idList]
	episodes  atomic.Pointer[episodeList]
	lists     []*list
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// list is one of the lists: where it comes from, where its copy is kept,
// and how it is read.
type list struct {
	name, file, url string
	read            func(io.Reader) (func(), error)
	state           listState
	loaded          bool
	// due is when the list is checked next, failures how many downloads
	// in a row failed.
	due      time.Time
	failures int
}

// listState is what is kept of a list beside its copy.
type listState struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	CheckedAt    time.Time `json:"checkedAt"`
}

// New returns a service that reads the copies of the lists kept in
// options.Dir, then keeps them fresh, in the background until Close.
func New(options Options) *Service {
	s := newService(options, defaultTiming)
	s.start()
	return s
}

func newService(options Options, t timing) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{dir: options.Dir, userAgent: "Polyfin/" + options.Version, logger: options.Logger, client: &http.Client{},
		now: time.Now, timing: t, ctx: ctx, cancel: cancel}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	s.lists = []*list{
		{name: "identifiers", file: "anime-list-full.json", url: cmpOr(options.IDsURL, IDsURL), read: func(r io.Reader) (func(), error) {
			l, err := readIDs(r)
			return func() { s.ids.Store(l) }, err
		}},
		{name: "episodes", file: "anime-list-master.xml", url: cmpOr(options.EpisodesURL, EpisodesURL), read: func(r io.Reader) (func(), error) {
			l, err := readEpisodes(r)
			return func() { s.episodes.Store(l) }, err
		}},
	}
	return s
}

func (s *Service) start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run()
	}()
}

// Close stops refreshing the lists and waits for a download under way to
// stop.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

// Ready reports whether both lists are read, which they are once a copy
// was kept or downloaded.
func (s *Service) Ready() bool {
	return s != nil && s.ids.Load() != nil && s.episodes.Load() != nil
}

// IDs lists the entries source identifies by id: at most one for AniDB,
// Kitsu, MyAnimeList and AniList, all those of a series or movie for TVDB,
// TMDB's shows or movies, and IMDb.
func (s *Service) IDs(source Source, id string) []IDs {
	if s == nil {
		return nil
	}
	return s.ids.Load().lookup(source, id)
}

// Episode finds where the episode number of the AniDB entry anidb is on
// TVDB: a regular episode, or a special when special. The mapping-list's
// episodes named one by one come first, then its ranges, then the entry's
// default season and offset, which may be TVDB's absolute numbering.
// Specials are only where the mapping-list puts them.
func (s *Service) Episode(anidb int, special bool, number int) (Place, bool) {
	if s == nil {
		return Place{}, false
	}
	return s.episodes.Load().place(anidb, special, number)
}

// Title is an anime entry, or one of its episodes, as Polyfin's titles
// and the tracking services know it: a movie by its IMDb and TMDB
// identifiers, or an episode by its series' TVDB identifier, and IMDb's
// when the identifier list gives the same series, in TVDB's numbering.
// When Absolute, Number is TVDB's absolute number of the episode, and
// Season unknown.
type Title struct {
	Movie          bool
	IMDb           string
	TMDB, TVDB     int
	Season, Number int
	Absolute       bool
}

// Title finds what the entry source names by id is, or its episode
// number (one of its specials when special): a movie when movie is set
// or AniDB counts the entry as one, and the identifier list gives a movie
// identifier; else the TVDB episode the episode list maps it to. An entry
// TVDB does not list as part of a series, with a movie identifier, is
// that movie. It reports false when the lists do not know.
func (s *Service) Title(source Source, id int, special bool, number int, movie bool) (Title, bool) {
	if s == nil || id <= 0 {
		return Title{}, false
	}
	var entry IDs
	found := s.IDs(source, strconv.Itoa(id))
	if len(found) > 0 {
		entry = found[0]
	}
	anidb := entry.AniDB
	if source == AniDB {
		anidb = id
	}
	// The identifier list names movies' identifiers for movies only: a
	// series' entry may give the series' IMDb identifier.
	hasMovie := entry.Movie || len(entry.TMDBMovies) > 0
	asMovie := func() (Title, bool) {
		t := Title{Movie: true}
		if len(entry.IMDb) == 1 {
			t.IMDb = entry.IMDb[0]
		}
		if len(entry.TMDBMovies) == 1 {
			t.TMDB = entry.TMDBMovies[0]
		}
		return t, t.IMDb != "" || t.TMDB > 0
	}
	if (movie || entry.Movie) && hasMovie && !special {
		if t, ok := asMovie(); ok {
			return t, true
		}
	}
	place, ok := s.Episode(anidb, special, max(number, 1))
	if !ok {
		if hasMovie && !special && number <= 1 {
			return asMovie()
		}
		return Title{}, false
	}
	t := Title{TVDB: place.TVDB, Season: place.Season, Number: place.Number, Absolute: place.Absolute}
	if !entry.Movie && entry.TVDB == place.TVDB && len(entry.IMDb) == 1 {
		t.IMDb = entry.IMDb[0]
	}
	return t, true
}

// StremioID reads a Stremio identifier that names an anime entry by its
// Kitsu, MyAnimeList or AniDB identifier ("kitsu:<id>", "mal:<id>",
// "anidb:<id>"), or one of its episodes ("kitsu:<id>:<episode>"); episode
// is 0 for the entry.
func StremioID(id string) (source Source, entry, episode int, ok bool) {
	parts := strings.Split(id, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", 0, 0, false
	}
	switch Source(parts[0]) {
	case Kitsu, MyAnimeList, AniDB:
	default:
		return "", 0, 0, false
	}
	entry, err := strconv.Atoi(parts[1])
	if err != nil || entry <= 0 {
		return "", 0, 0, false
	}
	if len(parts) == 3 {
		if episode, err = strconv.Atoi(parts[2]); err != nil || episode <= 0 {
			return "", 0, 0, false
		}
	}
	return Source(parts[0]), entry, episode, true
}

// run reads the copies kept, then refreshes each list when due until the
// service closes.
func (s *Service) run() {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		s.logger.Warn("The anime mapping lists' folder cannot be created: anime are not mapped", "folder", s.dir, "error", err)
	}
	states := map[string]listState{}
	if raw, err := os.ReadFile(filepath.Join(s.dir, stateFile)); err == nil {
		_ = json.Unmarshal(raw, &states)
	}
	now := s.now()
	for _, l := range s.lists {
		l.due = now
		if err := s.load(l); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				s.logger.Warn("A kept anime mapping list cannot be read: it is downloaded again", "list", l.name, "error", err)
			}
			continue
		}
		l.loaded, l.state = true, states[l.file]
		if !l.state.CheckedAt.IsZero() {
			l.due = l.state.CheckedAt.Add(s.timing.refreshEvery)
		}
	}
	for {
		next := time.Time{}
		for _, l := range s.lists {
			if !s.now().Before(l.due) {
				s.refreshList(l)
			}
			if s.ctx.Err() != nil {
				return
			}
			if next.IsZero() || l.due.Before(next) {
				next = l.due
			}
		}
		timer := time.NewTimer(next.Sub(s.now()))
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// load reads the copy of l kept.
func (s *Service) load(l *list) error {
	file, err := os.Open(filepath.Join(s.dir, l.file))
	if err != nil {
		return err
	}
	defer file.Close()
	use, err := l.read(io.LimitReader(file, maxListBytes))
	if err != nil {
		return err
	}
	use()
	return nil
}

// refreshList downloads l unless it did not change, and schedules its next
// check: a day later, or sooner after a failure.
func (s *Service) refreshList(l *list) {
	changed, err := s.download(l)
	if s.ctx.Err() != nil {
		return
	}
	if err != nil {
		l.failures++
		wait := min(s.timing.retryFirst<<min(l.failures-1, 16), s.timing.retryMax)
		l.due = s.now().Add(wait)
		if l.loaded {
			s.logger.Warn("An anime mapping list could not be refreshed: the last copy stays in use", "list", l.name, "retry_in", wait.String(), "error", err)
		} else {
			s.logger.Warn("An anime mapping list could not be downloaded: anime are not mapped until it is", "list", l.name, "retry_in", wait.String(), "error", err)
		}
		return
	}
	l.failures, l.loaded = 0, true
	l.due = l.state.CheckedAt.Add(s.timing.refreshEvery)
	s.saveState()
	if changed {
		s.logger.Info("Refreshed an anime mapping list", "list", l.name)
	}
}

// download asks for l, conditionally when a copy is kept, and keeps and
// uses the new copy once read whole. changed is false when the list did not
// change.
func (s *Service) download(l *list) (changed bool, err error) {
	ctx, cancel := context.WithTimeout(s.ctx, s.timing.download)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("User-Agent", s.userAgent)
	if l.loaded {
		if l.state.ETag != "" {
			request.Header.Set("If-None-Match", l.state.ETag)
		}
		if l.state.LastModified != "" {
			request.Header.Set("If-Modified-Since", l.state.LastModified)
		}
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotModified && l.loaded:
		l.state.CheckedAt = s.now()
		return false, nil
	case response.StatusCode != http.StatusOK:
		return false, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	temp, err := os.CreateTemp(s.dir, "."+l.file+"-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(temp.Name())
	n, err := io.Copy(temp, io.LimitReader(response.Body, maxListBytes+1))
	if err == nil && n > maxListBytes {
		err = errors.New("the list is too large")
	}
	if _, seekErr := temp.Seek(0, io.SeekStart); err == nil {
		err = seekErr
	}
	var use func()
	if err == nil {
		use, err = l.read(temp)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Rename(temp.Name(), filepath.Join(s.dir, l.file)); err != nil {
		return false, err
	}
	use()
	l.state = listState{ETag: response.Header.Get("ETag"), LastModified: response.Header.Get("Last-Modified"), CheckedAt: s.now()}
	return true, nil
}

// saveState keeps what the conditional requests need, and when each list
// was checked, so that a restart downloads nothing that is fresh.
func (s *Service) saveState() {
	states := map[string]listState{}
	for _, l := range s.lists {
		if l.loaded {
			states[l.file] = l.state
		}
	}
	raw, _ := json.Marshal(states)
	path := filepath.Join(s.dir, stateFile)
	err := os.WriteFile(path+".tmp", raw, 0o644)
	if err == nil {
		err = os.Rename(path+".tmp", path)
	}
	if err != nil {
		s.logger.Warn("The anime mapping lists' state could not be kept", "error", err)
	}
}
