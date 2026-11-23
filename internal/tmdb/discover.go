package tmdb

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// DiscoverPage is how many titles a page of TMDB's discover lists.
const DiscoverPage = 20

// Discovered is a movie or series TMDB's discover lists: its TMDB
// identifier, its name, the date it was first released or aired
// ("2006-01-02", empty when TMDB gives none), the path of its poster on
// the image server, its vote count and popularity, and whether TMDB marks
// it as for adults.
type Discovered struct {
	ID         int
	Name       string
	Date       string
	PosterPath string
	Votes      int
	Popularity float64
	Adult      bool
}

// Discover returns the page (from 1) of the movies or series (kind) of
// the networks or companies given by their TMDB identifiers, any of them
// (TMDB's "|" is an OR), as TMDB's discover lists them: the most popular
// first, released or first aired by today ("2006-01-02"), none for
// adults. Movies are those of the companies; series those of the
// networks, else of the companies when no network is given. Without
// either, TMDB is asked nothing.
func (c *Client) Discover(ctx context.Context, kind Kind, networks, companies []int, page int, today string) ([]Discovered, error) {
	query := url.Values{}
	switch {
	case kind == TV && len(networks) > 0:
		query.Set("with_networks", joinIDs(networks))
	case len(companies) > 0:
		query.Set("with_companies", joinIDs(companies))
	default:
		return nil, errors.New("no TMDB network or company to discover titles of")
	}
	return c.discover(ctx, kind, query, page, today)
}

// DiscoverServices returns the page (from 1) of the movies or series
// (kind) the streaming services given by their TMDB identifiers offer in
// country (ISO 3166-1), any of them, by subscription, free or free with
// ads, as Discover lists them. Without a service or a country, TMDB is
// asked nothing.
func (c *Client) DiscoverServices(ctx context.Context, kind Kind, services []int, country string, page int,
	today string) ([]Discovered, error) {
	if len(services) == 0 || country == "" {
		return nil, errors.New("no streaming service or country to discover titles of")
	}
	query := url.Values{"with_watch_providers": {joinIDs(services)}, "watch_region": {country},
		"with_watch_monetization_types": {"flatrate|free|ads"}}
	return c.discover(ctx, kind, query, page, today)
}

// discover asks TMDB's discover for the page of the movies or series
// (kind) query narrows to, the most popular first, released or first
// aired by today, none for adults.
func (c *Client) discover(ctx context.Context, kind Kind, query url.Values, page int, today string) ([]Discovered, error) {
	query.Set("sort_by", "popularity.desc")
	query.Set("include_adult", "false")
	query.Set("page", strconv.Itoa(max(page, 1)))
	if kind == TV {
		query.Set("first_air_date.lte", today)
	} else {
		query.Set("primary_release_date.lte", today)
	}
	var found struct {
		Results []struct {
			ID           int     `json:"id"`
			Title        string  `json:"title"`
			Name         string  `json:"name"`
			ReleaseDate  string  `json:"release_date"`
			FirstAirDate string  `json:"first_air_date"`
			PosterPath   string  `json:"poster_path"`
			Votes        int     `json:"vote_count"`
			Popularity   float64 `json:"popularity"`
			Adult        bool    `json:"adult"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/discover/"+string(kind), query, &found); err != nil {
		return nil, err
	}
	titles := make([]Discovered, 0, len(found.Results))
	for _, r := range found.Results {
		if r.ID <= 0 {
			continue
		}
		titles = append(titles, Discovered{ID: r.ID, Name: strings.TrimSpace(cmp.Or(r.Title, r.Name)),
			Date: cmp.Or(r.ReleaseDate, r.FirstAirDate), PosterPath: r.PosterPath, Votes: r.Votes, Popularity: r.Popularity,
			Adult: r.Adult})
	}
	return titles, nil
}

// joinIDs joins TMDB identifiers with "|", which TMDB's discover reads as
// any of them.
func joinIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, "|")
}
