// Package tmdb asks TMDB for the images of titles, seasons, episodes and
// people, with the server's own API key (Settings.TMDBKey), for
// administrators to choose an item's artwork from (see the jellyfin
// package's remote images), for the details of the people titles credit
// (see the library package's people details), for the studios and
// networks of titles, which smart lists may judge (see the library
// package's studios.go) and movies and series show with their tagline
// (see the library package's titlecredits.go), with the studios' logos,
// for the titles of a studio (see the library package's studiopages.go),
// for the release dates of movies, which the
// wait for their digital release and smart lists judge (see the library
// package's releases.go), for the episode groups series may show in
// (see the library package's episodeorder.go), for the seasons and
// episodes of series, as TMDB numbers them, which the tracking services
// number episodes by (see the library package's episodealign.go), for the
// collections movies belong to, which Silo's apps show as box sets (see
// the boxset package), for the posters and backdrops of titles whose
// addon gives none (see the library package's tmdbartwork.go), and for
// the check of the TMDB identifiers metadata addons give as moviedb_id
// (see the library package's tmdbidcheck.go).
//
// TMDB is asked only while a key is saved. Its answers, and the titles and
// people it did not know, are kept in memory for answerLife; at most
// maxRequests requests go to TMDB at once, its image server's included
// (see Hold). A key is sent as TMDB's API key, or as a bearer token when
// it is a read access token. Errors never hold the key. A request TMDB
// refuses for its rate fails with RateLimited, which tells how long it
// asks to wait.
package tmdb

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
)

// API is the address of TMDB's API, and Images that of its image server.
const (
	API    = "https://api.themoviedb.org/3"
	Images = "https://image.tmdb.org/t/p/"
)

const (
	// answerLife is how long an answer is kept, and maxAnswers how many.
	answerLife = 24 * time.Hour
	maxAnswers = 2000
	// maxRequests bounds the requests to TMDB at once.
	maxRequests = 4
	// requestTimeout bounds a request to the API, and maxAnswer the bytes
	// read of an answer.
	requestTimeout = 20 * time.Second
	maxAnswer      = 4 << 20
)

var (
	// ErrNotFound reports a title, season, episode or person TMDB does not
	// know.
	ErrNotFound = errors.New("not found on TMDB")
	// ErrKeyRefused reports a key TMDB refuses.
	ErrKeyRefused = errors.New("TMDB refused the key")
	// ErrNoKey reports that no key is saved.
	ErrNoKey = errors.New("no TMDB key")
)

// RateLimited reports a request TMDB refused for its rate (HTTP 429):
// RetryAfter is how long its Retry-After asks to wait, 0 when it asks
// nothing.
type RateLimited struct {
	RetryAfter time.Duration
}

func (e *RateLimited) Error() string { return "TMDB refused the request for its rate" }

// Kind is the kind of a title as TMDB names it.
type Kind string

const (
	Movie Kind = "movie"
	TV    Kind = "tv"
)

// Image is one of TMDB's images: its path on the image server, its size,
// its language (ISO 639-1, empty for none), and its votes.
type Image struct {
	Path     string  `json:"file_path"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Language string  `json:"iso_639_1"`
	Rating   float64 `json:"vote_average"`
	Votes    int     `json:"vote_count"`
}

// TitleImages are a movie's or series' images.
type TitleImages struct {
	Posters   []Image `json:"posters"`
	Backdrops []Image `json:"backdrops"`
	Logos     []Image `json:"logos"`
}

// Client asks TMDB. Its zero value is not ready: use New.
type Client struct {
	api, images, userAgent string
	key                    func() string
	http                   *http.Client
	slots                  chan struct{}
	answers                *cache.Cache[string, answer]
	flight                 singleflight.Group
	now                    func() time.Time
	// limitedAt is when TMDB last refused a request for its rate, and
	// limitedFor the wait its Retry-After asked then.
	mu         sync.Mutex
	limitedAt  time.Time
	limitedFor time.Duration
}

// answer is what TMDB answered for a path: its body, or nothing for what
// it does not know.
type answer struct {
	body  []byte
	found bool
}

// New returns a client of the API at api and the image server at images,
// asking with the key settings give, naming Polyfin's version in its
// User-Agent.
func New(api, images, version string, settings func() accounts.Settings) *Client {
	return newClient(api, images, version, settings, time.Now)
}

// newClient is New, keeping answers by the clock now. Its transport is
// its own, a clone of the default one with its timeouts: whoever closes
// the idle connections of http.DefaultTransport, as httptest.Server.Close
// does, cannot break a request to TMDB whose answer without a body has
// just put its connection back among the idle ones (#958).
func newClient(api, images, version string, settings func() accounts.Settings, now func() time.Time) *Client {
	return &Client{
		now:       now,
		api:       strings.TrimRight(api, "/"),
		images:    images,
		userAgent: "Polyfin/" + version,
		key:       func() string { return settings().TMDBKey },
		http:      &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: requestTimeout},
		slots:     make(chan struct{}, maxRequests),
		answers: cache.NewLasting[string, answer](maxAnswers, func() time.Duration { return answerLife }, now).Sized(64<<20, func(a answer) int {
			return len(a.body) + 64
		}),
	}
}

// Enabled reports whether a key is saved: without one, TMDB is never
// asked.
func (c *Client) Enabled() bool { return c != nil && c.key() != "" }

// Hold waits for one of the places among the requests to TMDB at once,
// and returns what gives it back.
func (c *Client) Hold(ctx context.Context) (func(), error) {
	select {
	case c.slots <- struct{}{}:
		return func() { <-c.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// LastRateLimit tells when TMDB last refused a request for its rate
// (HTTP 429) since the start, zero for never, and the wait its
// Retry-After asked then, 0 when it asked none.
func (c *Client) LastRateLimit() (time.Time, time.Duration) {
	if c == nil {
		return time.Time{}, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limitedAt, c.limitedFor
}

// ForgetPerson forgets the answers kept about the person of TMDB
// identifier id, 0 for none, and the searches of name, so that they are
// asked for again, as a refresh of the person asks.
func (c *Client) ForgetPerson(id int, name string) {
	if c == nil {
		return
	}
	person := "/person/" + strconv.Itoa(id)
	search := url.Values{"query": {name}}.Encode()
	c.answers.DeleteFunc(func(key string, _ answer) bool {
		path, query, _ := strings.Cut(key, "?")
		switch {
		case id > 0 && (path == person || strings.HasPrefix(path, person+"/")):
			return true
		case path == "/search/person":
			return slices.Contains(strings.Split(query, "&"), search)
		}
		return false
	})
}

// ImageURL is the address of an image on TMDB's image server, at size
// ("original", or one of TMDB's widths such as "w342").
func (c *Client) ImageURL(path, size string) string {
	return c.images + size + path
}

// FindByIMDb returns the TMDB identifier of the title of kind an IMDb
// identifier names.
func (c *Client) FindByIMDb(ctx context.Context, kind Kind, imdb string) (string, error) {
	return c.find(ctx, kind, imdb, "imdb_id")
}

// FindByTVDB returns the TMDB identifier of the title of kind a TVDB
// identifier names, as Jellyfin 12.4's TMDB providers find it
// (TmdbSeriesProvider.cs lines 92-110, TmdbMovieProvider.cs lines
// 104-113).
func (c *Client) FindByTVDB(ctx context.Context, kind Kind, tvdb string) (string, error) {
	return c.find(ctx, kind, tvdb, "tvdb_id")
}

// Found is what TMDB's find gives for an identifier of another source: the
// TMDB identifier of the first movie and of the first series, each 0 for
// none.
type Found struct {
	Movie, TV int
}

// Of is the identifier found of kind, 0 for none.
func (f Found) Of(kind Kind) int {
	if kind == TV {
		return f.TV
	}
	return f.Movie
}

// FoundByIMDb returns the movie and the series TMDB's find gives for an
// IMDb identifier: one answer holds both kinds, which FindByIMDb shares.
func (c *Client) FoundByIMDb(ctx context.Context, imdb string) (Found, error) {
	return c.found(ctx, imdb, "imdb_id")
}

// find returns the TMDB identifier of the title of kind TMDB's find gives
// for an identifier of another source ("imdb_id", "tvdb_id").
func (c *Client) find(ctx context.Context, kind Kind, id, source string) (string, error) {
	found, err := c.found(ctx, id, source)
	if err != nil {
		return "", err
	}
	if found.Of(kind) == 0 {
		return "", ErrNotFound
	}
	return strconv.Itoa(found.Of(kind)), nil
}

// found returns what TMDB's find gives for an identifier of another source.
func (c *Client) found(ctx context.Context, id, source string) (Found, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Found{}, ErrNotFound
	}
	var answer struct {
		Movies []struct{ ID int } `json:"movie_results"`
		Series []struct{ ID int } `json:"tv_results"`
	}
	if err := c.get(ctx, "/find/"+url.PathEscape(id), url.Values{"external_source": {source}}, &answer); err != nil {
		return Found{}, err
	}
	var found Found
	if len(answer.Movies) > 0 && answer.Movies[0].ID > 0 {
		found.Movie = answer.Movies[0].ID
	}
	if len(answer.Series) > 0 && answer.Series[0].ID > 0 {
		found.TV = answer.Series[0].ID
	}
	return found, nil
}

// TitleImages returns the images of the movie or series of kind TMDB
// identifies as id, in every language.
func (c *Client) TitleImages(ctx context.Context, kind Kind, id string) (TitleImages, error) {
	var images TitleImages
	err := c.get(ctx, "/"+string(kind)+"/"+url.PathEscape(id)+"/images", nil, &images)
	return images, err
}

// Studios returns the names of the networks a series of kind TMDB
// identifies as id aired on, then of the companies that made the movie or
// series, each once.
func (c *Client) Studios(ctx context.Context, kind Kind, id string) ([]string, error) {
	var details struct {
		Networks  []Studio `json:"networks"`
		Companies []Studio `json:"production_companies"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+url.PathEscape(id), nil, &details); err != nil {
		return nil, err
	}
	return JoinStudios(StudioNames(details.Networks), StudioNames(details.Companies)), nil
}

// JoinStudios returns the studios of a title: the names of the networks
// it aired on, then of the companies that made it, each once.
func JoinStudios(networks, companies []string) []string {
	var names []string
	for _, name := range slices.Concat(networks, companies) {
		if name = strings.TrimSpace(name); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// Studio is a network or a company of TMDB's details of a title: its
// TMDB identifier, its name, and the path of its logo on the image server,
// empty when TMDB has none. TMDB serves every logo as a PNG at each of
// its widths, whatever the file it was given.
type Studio struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LogoPath string `json:"logo_path"`
}

// studiosOf returns TMDB's networks or companies, their names and logo
// paths trimmed, each once (by identifier, by name for one of none),
// those of no name left out; never nil.
func studiosOf(list []Studio) []Studio {
	studios := []Studio{}
	for _, s := range list {
		s.Name, s.LogoPath = strings.TrimSpace(s.Name), strings.TrimSpace(s.LogoPath)
		if s.Name != "" && !slices.ContainsFunc(studios, func(kept Studio) bool {
			return kept.ID == s.ID && (s.ID > 0 || kept.Name == s.Name)
		}) {
			studios = append(studios, s)
		}
	}
	return studios
}

// StudioNames returns the names of networks or companies, trimmed, each
// once, empty ones left out; never nil.
func StudioNames(list []Studio) []string {
	names := []string{}
	for _, s := range list {
		if name := strings.TrimSpace(s.Name); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// CollectionRef is the TMDB collection a movie belongs to, as the movie's
// details name it: its identifier, its name, and the paths of its poster
// and backdrop on the image server, empty when TMDB has none.
type CollectionRef struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
}

// MovieCollection returns the collection the movie TMDB identifies as id
// belongs to, nil for none. It reads the movie's details, which Studios
// reads too: one answer serves both.
func (c *Client) MovieCollection(ctx context.Context, id string) (*CollectionRef, error) {
	var details struct {
		Collection *CollectionRef `json:"belongs_to_collection"`
	}
	if err := c.get(ctx, "/movie/"+url.PathEscape(id), nil, &details); err != nil {
		return nil, err
	}
	if details.Collection == nil || details.Collection.ID <= 0 || strings.TrimSpace(details.Collection.Name) == "" {
		return nil, nil
	}
	return details.Collection, nil
}

// Releases are the release dates of a movie, the earliest TMDB knows of
// each kind, in any country, each zero when it knows none: Home, its
// first digital or physical release; Theatrical, its first in cinemas,
// limited or wide; First, its first of any kind, premieres and television
// included.
type Releases struct {
	Home, Theatrical, First time.Time
}

// TMDB's kinds of release dates.
const (
	releaseLimited  = 2
	releaseCinemas  = 3
	releaseDigital  = 4
	releasePhysical = 5
)

// ReleaseDates returns the release dates of the movie TMDB identifies as
// id, each counted from the day it falls on in UTC.
func (c *Client) ReleaseDates(ctx context.Context, id string) (Releases, error) {
	var answer struct {
		Results []struct {
			Dates []struct {
				Date string `json:"release_date"`
				Type int    `json:"type"`
			} `json:"release_dates"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/movie/"+url.PathEscape(id)+"/release_dates", nil, &answer); err != nil {
		return Releases{}, err
	}
	var releases Releases
	earliest := func(kept *time.Time, date time.Time) {
		if kept.IsZero() || date.Before(*kept) {
			*kept = date
		}
	}
	for _, country := range answer.Results {
		for _, d := range country.Dates {
			date, err := time.Parse(time.RFC3339Nano, d.Date)
			if err != nil {
				if date, err = time.Parse(time.DateOnly, d.Date); err != nil {
					continue
				}
			}
			date = date.UTC().Truncate(24 * time.Hour)
			earliest(&releases.First, date)
			switch d.Type {
			case releaseDigital, releasePhysical:
				earliest(&releases.Home, date)
			case releaseLimited, releaseCinemas:
				earliest(&releases.Theatrical, date)
			}
		}
	}
	return releases, nil
}

// SeasonPosters returns the posters of a series' season.
func (c *Client) SeasonPosters(ctx context.Context, id string, season int) ([]Image, error) {
	var images struct {
		Posters []Image `json:"posters"`
	}
	err := c.get(ctx, "/tv/"+url.PathEscape(id)+"/season/"+strconv.Itoa(season)+"/images", nil, &images)
	return images.Posters, err
}

// EpisodeStills returns the stills of a series' episode.
func (c *Client) EpisodeStills(ctx context.Context, id string, season, episode int) ([]Image, error) {
	var images struct {
		Stills []Image `json:"stills"`
	}
	err := c.get(ctx, "/tv/"+url.PathEscape(id)+"/season/"+strconv.Itoa(season)+"/episode/"+strconv.Itoa(episode)+"/images", nil, &images)
	return images.Stills, err
}

// PersonPhotos returns the photos of the person TMDB finds by name: the
// most popular of those of that very name, whatever the case.
func (c *Client) PersonPhotos(ctx context.Context, name string) ([]Image, error) {
	found, err := c.SearchPeople(ctx, name)
	if err != nil {
		return nil, err
	}
	best, popularity := 0, -1.0
	for _, result := range found {
		if result.Popularity > popularity {
			best, popularity = result.ID, result.Popularity
		}
	}
	if best == 0 {
		return nil, ErrNotFound
	}
	var images struct {
		Profiles []Image `json:"profiles"`
	}
	err = c.get(ctx, "/person/"+strconv.Itoa(best)+"/images", nil, &images)
	return images.Profiles, err
}

// FoundPerson is a person TMDB's search finds: their identifier, name,
// popularity, profile photo's path, and the titles they are known for.
type FoundPerson struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Popularity  float64  `json:"popularity"`
	ProfilePath string   `json:"profile_path"`
	KnownFor    []Credit `json:"known_for"`
}

// Credit is a title a person is credited in: a movie or a series ("tv"),
// by its identifier, its names and its release date ("2006-01-02"). A
// person's combined credits also give its poster's path, genres (by
// TMDB's genre identifiers), popularity, vote count and whether it is
// for adults, and the character the person plays, or their job in the
// crew.
type Credit struct {
	ID            int     `json:"id"`
	MediaType     string  `json:"media_type"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Name          string  `json:"name"`
	OriginalName  string  `json:"original_name"`
	ReleaseDate   string  `json:"release_date"`
	FirstAirDate  string  `json:"first_air_date"`
	PosterPath    string  `json:"poster_path"`
	GenreIDs      []int   `json:"genre_ids"`
	Popularity    float64 `json:"popularity"`
	VoteCount     int     `json:"vote_count"`
	Adult         bool    `json:"adult"`
	Character     string  `json:"character"`
	Job           string  `json:"job"`
}

// Names are the credit's title and original title, those of a series
// included.
func (c Credit) Names() []string {
	return slices.DeleteFunc([]string{c.Title, c.OriginalTitle, c.Name, c.OriginalName}, func(s string) bool { return s == "" })
}

// Year is the year of the credit's release, 0 when TMDB gives none.
func (c Credit) Year() int {
	date := cmp.Or(c.ReleaseDate, c.FirstAirDate)
	if len(date) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(date[:4])
	return year
}

// SearchPeople returns the people TMDB finds by name whose name is that
// very name, whatever the case: TMDB's search also finds people whose
// name merely contains the words.
func (c *Client) SearchPeople(ctx context.Context, name string) ([]FoundPerson, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	var found struct {
		Results []FoundPerson `json:"results"`
	}
	if err := c.get(ctx, "/search/person", url.Values{"query": {name}, "include_adult": {"false"}}, &found); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(found.Results, func(p FoundPerson) bool {
		return p.ID <= 0 || !strings.EqualFold(strings.Join(strings.Fields(p.Name), " "), strings.Join(strings.Fields(name), " "))
	}), nil
}

// PersonCredits returns the titles a person is credited in, as an actor
// or in the crew.
func (c *Client) PersonCredits(ctx context.Context, id int) ([]Credit, error) {
	var credits struct {
		Cast []Credit `json:"cast"`
		Crew []Credit `json:"crew"`
	}
	err := c.get(ctx, "/person/"+strconv.Itoa(id)+"/combined_credits", nil, &credits)
	return append(credits.Cast, credits.Crew...), err
}

// Person is a person's details: their biography, birth and death dates
// ("2006-01-02", empty when unknown), place of birth, profile photo's
// path and IMDb identifier.
type Person struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Biography    string `json:"biography"`
	Birthday     string `json:"birthday"`
	Deathday     string `json:"deathday"`
	PlaceOfBirth string `json:"place_of_birth"`
	ProfilePath  string `json:"profile_path"`
	IMDbID       string `json:"imdb_id"`
}

// PersonDetails returns the details of the person TMDB identifies as id,
// their biography in language (an ISO 639-1 code, with a region or not,
// such as "fr" or "pt-BR"), which TMDB leaves empty when it has none in
// it.
func (c *Client) PersonDetails(ctx context.Context, id int, language string) (Person, error) {
	var person Person
	query := url.Values{}
	if language != "" {
		query.Set("language", language)
	}
	err := c.get(ctx, "/person/"+strconv.Itoa(id), query, &person)
	return person, err
}

// FoundTitle is a movie or series TMDB's search finds: its identifier,
// its name and original name, its year (0 when TMDB gives no date) and
// its poster's path.
type FoundTitle struct {
	ID           int
	Name         string
	OriginalName string
	Year         int
	PosterPath   string
}

// SearchTitles returns the movies or series (kind) TMDB's search finds by
// name, the first page of them, of year when it is not zero, as Jellyfin's
// TMDB providers search a title by its name and year.
func (c *Client) SearchTitles(ctx context.Context, kind Kind, name string, year int) ([]FoundTitle, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	query := url.Values{"query": {name}, "include_adult": {"false"}}
	if year > 0 {
		if kind == TV {
			query.Set("first_air_date_year", strconv.Itoa(year))
		} else {
			query.Set("year", strconv.Itoa(year))
		}
	}
	var found struct {
		Results []struct {
			ID            int    `json:"id"`
			Title         string `json:"title"`
			OriginalTitle string `json:"original_title"`
			Name          string `json:"name"`
			OriginalName  string `json:"original_name"`
			ReleaseDate   string `json:"release_date"`
			FirstAirDate  string `json:"first_air_date"`
			PosterPath    string `json:"poster_path"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/search/"+string(kind), query, &found); err != nil {
		return nil, err
	}
	titles := make([]FoundTitle, 0, len(found.Results))
	for _, r := range found.Results {
		if r.ID <= 0 {
			continue
		}
		credit := Credit{ReleaseDate: r.ReleaseDate, FirstAirDate: r.FirstAirDate}
		titles = append(titles, FoundTitle{ID: r.ID, Name: cmp.Or(r.Title, r.Name), OriginalName: cmp.Or(r.OriginalTitle, r.OriginalName),
			Year: credit.Year(), PosterPath: r.PosterPath})
	}
	return titles, nil
}

// IMDbID returns the IMDb identifier of the movie or series (kind) TMDB
// identifies as id, from its external identifiers: empty when TMDB gives
// none, and ErrNotFound when TMDB does not know the title.
func (c *Client) IMDbID(ctx context.Context, kind Kind, id int) (string, error) {
	var ids struct {
		IMDb string `json:"imdb_id"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+strconv.Itoa(id)+"/external_ids", nil, &ids); err != nil {
		return "", err
	}
	return strings.TrimSpace(ids.IMDb), nil
}

// TitleYear returns the year the movie or series (kind) TMDB identifies as
// id was first released or aired, 0 when TMDB gives no date; ErrNotFound
// when TMDB does not know the title. It reads the title's details, which
// Studios reads too: one answer serves both.
func (c *Client) TitleYear(ctx context.Context, kind Kind, id int) (int, error) {
	var details struct {
		Released string `json:"release_date"`
		Aired    string `json:"first_air_date"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+strconv.Itoa(id), nil, &details); err != nil {
		return 0, err
	}
	date := details.Released
	if kind == TV {
		date = details.Aired
	}
	if len(date) < 4 {
		return 0, nil
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0, nil
	}
	return year, nil
}

// GroupSummary is one of a series' episode groups, as TMDB lists them:
// its identifier, name and type (GroupDVD and the others), and how many
// episodes and groups, which the series' seasons are in its order, it
// holds.
type GroupSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Episodes int    `json:"episode_count"`
	Groups   int    `json:"group_count"`
}

// The types of TMDB's episode groups.
const (
	GroupOriginalAirDate = 1
	GroupAbsolute        = 2
	GroupDVD             = 3
	GroupDigital         = 4
	GroupStoryArc        = 5
	GroupProduction      = 6
	GroupTV              = 7
)

// EpisodeGroup is one of TMDB's episode groups in full: a series'
// episodes in another order, in groups that stand for its seasons.
type EpisodeGroup struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Type   int             `json:"type"`
	Groups []EpisodeSeason `json:"groups"`
}

// EpisodeSeason is a group of an EpisodeGroup: a season of its order, at
// Order among them, with its episodes.
type EpisodeSeason struct {
	Name     string         `json:"name"`
	Order    int            `json:"order"`
	Episodes []GroupEpisode `json:"episodes"`
}

// GroupEpisode is an episode of an EpisodeSeason: the series' episode
// Season and Episode, as TMDB numbers them as aired, at Order in the
// group, from 0. Show is the series it is an episode of, which a group
// may mix.
type GroupEpisode struct {
	Show    int `json:"show_id"`
	Season  int `json:"season_number"`
	Episode int `json:"episode_number"`
	Order   int `json:"order"`
}

// EpisodeGroups lists the episode groups of the series TMDB identifies
// as id.
func (c *Client) EpisodeGroups(ctx context.Context, id string) ([]GroupSummary, error) {
	var groups struct {
		Results []GroupSummary `json:"results"`
	}
	err := c.get(ctx, "/tv/"+url.PathEscape(id)+"/episode_groups", nil, &groups)
	return groups.Results, err
}

// EpisodeGroupOf returns the episode group TMDB identifies as id.
func (c *Client) EpisodeGroupOf(ctx context.Context, id string) (EpisodeGroup, error) {
	var group EpisodeGroup
	err := c.get(ctx, "/tv/episode_group/"+url.PathEscape(id), nil, &group)
	return group, err
}

// SeasonSummary is a season of a series as the series' details list it:
// its number (0 for the specials), how many episodes it holds, and the day
// it first aired ("2006-01-02", empty when TMDB gives none).
type SeasonSummary struct {
	Season   int    `json:"season_number"`
	Episodes int    `json:"episode_count"`
	AirDate  string `json:"air_date"`
}

// SeriesSeasons returns the seasons the details of the series TMDB
// identifies as id list, the specials included. It reads the series'
// details, which Studios reads too: one answer serves both.
func (c *Client) SeriesSeasons(ctx context.Context, id int) ([]SeasonSummary, error) {
	var details struct {
		Seasons []SeasonSummary `json:"seasons"`
	}
	err := c.get(ctx, "/tv/"+strconv.Itoa(id), nil, &details)
	return details.Seasons, err
}

// SeasonEpisode is an episode of a series as TMDB numbers it as aired:
// its season (0 for the specials) and number, its name, and the day it
// aired ("2006-01-02", empty when TMDB gives none).
type SeasonEpisode struct {
	Season  int    `json:"season_number"`
	Episode int    `json:"episode_number"`
	Name    string `json:"name"`
	AirDate string `json:"air_date"`
}

const (
	// maxAppended bounds the seasons one request appends to a series'
	// details, as TMDB appends at most 20 answers to one.
	maxAppended = 20
	// maxAppendedEpisodes bounds the episodes of the seasons one request
	// appends, a season holding more asked alone, so that an answer stays
	// within maxAnswer.
	maxAppendedEpisodes = 400
)

// SeasonEpisodes returns the episodes of the seasons of the series TMDB
// identifies as id, in the order of seasons, asked with the series'
// details (append_to_response=season/1,…): up to maxAppended seasons a
// request, and maxAppendedEpisodes episodes as seasons tells them. A
// season TMDB does not give is left out.
func (c *Client) SeasonEpisodes(ctx context.Context, id int, seasons []SeasonSummary) ([]SeasonEpisode, error) {
	var episodes []SeasonEpisode
	for start := 0; start < len(seasons); {
		end, count := start, 0
		for end < len(seasons) && end-start < maxAppended && (end == start || count+seasons[end].Episodes <= maxAppendedEpisodes) {
			count += seasons[end].Episodes
			end++
		}
		appended := make([]string, 0, end-start)
		for _, season := range seasons[start:end] {
			appended = append(appended, "season/"+strconv.Itoa(season.Season))
		}
		var answer map[string]json.RawMessage
		if err := c.get(ctx, "/tv/"+strconv.Itoa(id), url.Values{"append_to_response": {strings.Join(appended, ",")}}, &answer); err != nil {
			return nil, err
		}
		for _, name := range appended {
			var season struct {
				Episodes []SeasonEpisode `json:"episodes"`
			}
			if raw, ok := answer[name]; ok && json.Unmarshal(raw, &season) == nil {
				episodes = append(episodes, season.Episodes...)
			}
		}
		start = end
	}
	return episodes, nil
}

// CheckKey asks TMDB whether it accepts key: ErrKeyRefused when it does
// not, another error when it could not be asked.
func (c *Client) CheckKey(ctx context.Context, key string) error {
	release, err := c.Hold(ctx)
	if err != nil {
		return err
	}
	defer release()
	status, _, _, err := c.ask(ctx, key, "/authentication", nil)
	switch {
	case err != nil:
		return err
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrKeyRefused
	case status != http.StatusOK:
		return fmt.Errorf("TMDB answered %d", status)
	}
	return nil
}

// get reads TMDB's answer for path and query into into, kept for
// answerLife and asked once for all the callers at once, on a context
// the first caller giving up does not cancel.
func (c *Client) get(ctx context.Context, path string, query url.Values, into any) error {
	key := c.key()
	if key == "" {
		return ErrNoKey
	}
	cacheKey := path + "?" + query.Encode()
	kept, ok := c.answers.Get(cacheKey)
	if !ok {
		detached := context.WithoutCancel(ctx)
		result, err, _ := c.flight.Do(cacheKey, func() (any, error) {
			if kept, ok := c.answers.Get(cacheKey); ok {
				return kept, nil
			}
			release, err := c.Hold(detached)
			if err != nil {
				return answer{}, err
			}
			defer release()
			status, header, body, err := c.ask(detached, key, path, query)
			switch {
			case err != nil:
				return answer{}, err
			case status == http.StatusNotFound:
				kept := answer{}
				c.answers.Put(cacheKey, kept)
				return kept, nil
			case status == http.StatusUnauthorized:
				return answer{}, ErrKeyRefused
			case status == http.StatusTooManyRequests:
				limited := &RateLimited{}
				if seconds, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && seconds > 0 {
					limited.RetryAfter = time.Duration(seconds) * time.Second
				}
				c.mu.Lock()
				c.limitedAt, c.limitedFor = c.now(), limited.RetryAfter
				c.mu.Unlock()
				return answer{}, limited
			case status != http.StatusOK:
				return answer{}, fmt.Errorf("TMDB answered %d", status)
			}
			kept := answer{body: body, found: true}
			c.answers.Put(cacheKey, kept)
			return kept, nil
		})
		if err != nil {
			return err
		}
		kept = result.(answer)
	}
	if !kept.found {
		return ErrNotFound
	}
	if err := json.Unmarshal(kept.body, into); err != nil {
		return fmt.Errorf("read TMDB's answer: %w", err)
	}
	return nil
}

// ask sends one request with key and returns TMDB's status, headers and
// answer.
func (c *Client) ask(ctx context.Context, key, path string, query url.Values) (int, http.Header, []byte, error) {
	values := url.Values{}
	for name, value := range query {
		values[name] = slices.Clone(value)
	}
	bearer := strings.Contains(key, ".")
	if !bearer {
		values.Set("api_key", key)
	}
	target := c.api + path
	if len(values) > 0 {
		target += "?" + values.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, nil, errors.New("invalid TMDB request")
	}
	if bearer {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("TMDB could not be asked: %w", withoutURL(err))
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("TMDB's answer could not be read: %w", withoutURL(err))
	}
	return response.StatusCode, response.Header, body, nil
}

// withoutURL drops the URL, which holds the key, from a request's error.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
