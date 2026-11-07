package importlinks

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Target is the title a user links an entry to, as Parse reads it: an
// IMDb identifier or a TMDB one, and the Kind the input tells, KindMovie
// or KindSeries, empty when it does not tell (an IMDb identifier, or a
// TMDB one alone, which is of the entry's kind).
type Target struct {
	IMDb string
	TMDB int
	Kind string
}

var (
	imdbID = regexp.MustCompile(`^tt\d{1,12}$`)
	// imdbPath is the path of an IMDb title page, maybe in a language.
	imdbPath = regexp.MustCompile(`^(?:/[a-z]{2}(?:-[a-z]{2})?)?/title/(tt\d{1,12})(?:/.*)?$`)
	// tmdbPath is the path of a TMDB movie or series page: its kind, its
	// identifier, maybe followed by a slug of its name, and maybe one of
	// its pages, such as a season.
	tmdbPath = regexp.MustCompile(`^/(movie|tv)/(\d{1,10})(?:-[^/]*)?(?:/.*)?$`)
)

// Parse reads what a user links an entry to: an IMDb identifier ("tt…"), a
// TMDB identifier ("tmdb:…" or the number alone), the address of an IMDb
// title page, or the address of a TMDB movie or series page, which tells
// the kind. It fails with ErrInvalidTarget for anything else.
func Parse(input string) (Target, error) {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	if imdbID.MatchString(lower) {
		return Target{IMDb: lower}, nil
	}
	if number, ok := tmdbNumber(strings.TrimPrefix(lower, "tmdb:")); ok {
		return Target{TMDB: number}, nil
	}
	address := input
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" {
		return Target{}, ErrInvalidTarget
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.ToLower(u.EscapedPath())
	switch host {
	case "imdb.com", "m.imdb.com":
		if m := imdbPath.FindStringSubmatch(path); m != nil {
			return Target{IMDb: m[1]}, nil
		}
	case "themoviedb.org":
		if m := tmdbPath.FindStringSubmatch(path); m != nil {
			number, ok := tmdbNumber(m[2])
			if !ok {
				break
			}
			kind := KindMovie
			if m[1] == "tv" {
				kind = KindSeries
			}
			return Target{TMDB: number, Kind: kind}, nil
		}
	}
	return Target{}, ErrInvalidTarget
}

// tmdbNumber reads a TMDB identifier: a positive number.
func tmdbNumber(s string) (int, bool) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, false
	}
	number, err := strconv.Atoi(s)
	return number, err == nil && number > 0
}
