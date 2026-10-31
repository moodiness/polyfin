// Package tmdb asks TMDB for the images of titles, seasons, episodes and
// people, with the server's own API key (Settings.TMDBKey), for
// administrators to choose an item's artwork from (see the jellyfin
// package's remote images), for the details of the people titles credit
// (see the library package's people details), for the studios and
// networks of titles, which smart lists may judge (see the library
// package's studios.go), and for the release dates of movies, which the
// wait for their digital release and smart lists judge (see the library
// package's releases.go).
//
// TMDB is asked only while a key is saved. Its answers, and the titles and
// people it did not know, are kept in memory for answerLife; at most
// maxRequests requests go to TMDB at once, its image server's included
// (see Hold). A key is sent as TMDB's API key, or as a bearer token when
// it is a read access token. Errors never hold the key.
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

// newClient is New, keeping answers by the clock now.
func newClient(api, images, version string, settings func() accounts.Settings, now func() time.Time) *Client {
	return &Client{
		api:       strings.TrimRight(api, "/"),
		images:    images,
		userAgent: "Polyfin/" + version,
		key:       func() string { return settings().TMDBKey },
		http:      &http.Client{Timeout: requestTimeout},
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

// ImageURL is the address of an image on TMDB's image server, at size
// ("original", or one of TMDB's widths such as "w342").
func (c *Client) ImageURL(path, size string) string {
	return c.images + size + path
}

// FindByIMDb returns the TMDB identifier of the title of kind an IMDb
// identifier names.
func (c *Client) FindByIMDb(ctx context.Context, kind Kind, imdb string) (string, error) {
	var found struct {
		Movies []struct{ ID int } `json:"movie_results"`
		Series []struct{ ID int } `json:"tv_results"`
	}
	if err := c.get(ctx, "/find/"+url.PathEscape(imdb), url.Values{"external_source": {"imdb_id"}}, &found); err != nil {
		return "", err
	}
	results := found.Movies
	if kind == TV {
		results = found.Series
	}
	if len(results) == 0 || results[0].ID <= 0 {
		return "", ErrNotFound
	}
	return strconv.Itoa(results[0].ID), nil
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
	type named struct {
		Name string `json:"name"`
	}
	var details struct {
		Networks  []named `json:"networks"`
		Companies []named `json:"production_companies"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+url.PathEscape(id), nil, &details); err != nil {
		return nil, err
	}
	var names []string
	for _, n := range slices.Concat(details.Networks, details.Companies) {
		if name := strings.TrimSpace(n.Name); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names, nil
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
// by its identifier, its names and its release date ("2006-01-02").
type Credit struct {
	ID            int    `json:"id"`
	MediaType     string `json:"media_type"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
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
// identifies as id, from its external identifiers; ErrNotFound when TMDB
// gives none.
func (c *Client) IMDbID(ctx context.Context, kind Kind, id int) (string, error) {
	var ids struct {
		IMDb string `json:"imdb_id"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+strconv.Itoa(id)+"/external_ids", nil, &ids); err != nil {
		return "", err
	}
	if strings.TrimSpace(ids.IMDb) == "" {
		return "", ErrNotFound
	}
	return strings.TrimSpace(ids.IMDb), nil
}

// CheckKey asks TMDB whether it accepts key: ErrKeyRefused when it does
// not, another error when it could not be asked.
func (c *Client) CheckKey(ctx context.Context, key string) error {
	release, err := c.Hold(ctx)
	if err != nil {
		return err
	}
	defer release()
	status, _, err := c.ask(ctx, key, "/authentication", nil)
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
			status, body, err := c.ask(detached, key, path, query)
			switch {
			case err != nil:
				return answer{}, err
			case status == http.StatusNotFound:
				kept := answer{}
				c.answers.Put(cacheKey, kept)
				return kept, nil
			case status == http.StatusUnauthorized:
				return answer{}, ErrKeyRefused
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

// ask sends one request with key and returns TMDB's status and answer.
func (c *Client) ask(ctx context.Context, key, path string, query url.Values) (int, []byte, error) {
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
		return 0, nil, errors.New("invalid TMDB request")
	}
	if bearer {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("TMDB could not be asked: %w", withoutURL(err))
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	if err != nil {
		return 0, nil, fmt.Errorf("TMDB's answer could not be read: %w", withoutURL(err))
	}
	return response.StatusCode, body, nil
}

// withoutURL drops the URL, which holds the key, from a request's error.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
