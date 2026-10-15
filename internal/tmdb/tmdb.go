// Package tmdb asks TMDB for the images of titles, seasons, episodes and
// people, with the server's own API key (Settings.TMDBKey), for
// administrators to choose an item's artwork from (see the jellyfin
// package's remote images).
//
// TMDB is asked only while a key is saved. Its answers, and the titles and
// people it did not know, are kept in memory for answerLife; at most
// maxRequests requests go to TMDB at once, its image server's included
// (see Hold). A key is sent as TMDB's API key, or as a bearer token when
// it is a read access token. Errors never hold the key.
package tmdb

import (
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
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	var found struct {
		Results []struct {
			ID         int
			Name       string
			Popularity float64
		}
	}
	if err := c.get(ctx, "/search/person", url.Values{"query": {name}, "include_adult": {"false"}}, &found); err != nil {
		return nil, err
	}
	best, popularity := 0, -1.0
	for _, result := range found.Results {
		if strings.EqualFold(strings.TrimSpace(result.Name), name) && result.ID > 0 && result.Popularity > popularity {
			best, popularity = result.ID, result.Popularity
		}
	}
	if best == 0 {
		return nil, ErrNotFound
	}
	var images struct {
		Profiles []Image `json:"profiles"`
	}
	err := c.get(ctx, "/person/"+strconv.Itoa(best)+"/images", nil, &images)
	return images.Profiles, err
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
