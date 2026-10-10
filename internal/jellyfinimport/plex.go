package jellyfinimport

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// A Plex Media Server is read with its owner's token, in JSON, with GET
// requests only: Plex is never written to.

// plexOwner is the account Plex numbers its owner with, whatever the
// owner's plex.tv account is.
const plexOwner = "1"

// The types of the library sections and items an import reads.
const (
	plexMovie   = "movie"
	plexShow    = "show"
	plexEpisode = "episode"
)

// plexItemJSON is a movie, show or episode as Plex lists it in a library
// section or in the playback history.
type plexItemJSON struct {
	RatingKey string `json:"ratingKey"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	// Guid lists the item's identifiers elsewhere, such as imdb://tt…,
	// tmdb://… and tvdb://…, with includeGuids=1.
	Guid []struct {
		ID string `json:"id"`
	} `json:"Guid"`
	// GrandparentRatingKey, or else GrandparentKey, names an episode's show,
	// ParentIndex its season and Index its number.
	GrandparentRatingKey string `json:"grandparentRatingKey"`
	GrandparentKey       string `json:"grandparentKey"`
	GrandparentTitle     string `json:"grandparentTitle"`
	ParentIndex          *int   `json:"parentIndex"`
	Index                *int   `json:"index"`
	// ViewCount, LastViewedAt, a time in seconds, and ViewOffset, a position
	// in milliseconds, are the token's owner's; Duration is in
	// milliseconds. ViewedAt dates a play of the history.
	ViewCount    int   `json:"viewCount"`
	LastViewedAt int64 `json:"lastViewedAt"`
	ViewOffset   int64 `json:"viewOffset"`
	Duration     int64 `json:"duration"`
	ViewedAt     int64 `json:"viewedAt"`
}

// show is the rating key of an episode's show.
func (p plexItemJSON) show() string {
	if p.GrandparentRatingKey != "" {
		return p.GrandparentRatingKey
	}
	if p.GrandparentKey == "" {
		return ""
	}
	return path.Base(p.GrandparentKey)
}

// providers are the identifiers of p's Guid list, as Jellyfin names them.
func (p plexItemJSON) providers() map[string]string {
	providers := map[string]string{}
	for _, guid := range p.Guid {
		scheme, id, _ := strings.Cut(guid.ID, "://")
		switch scheme {
		case "imdb":
			providers["Imdb"] = id
		case "tmdb":
			providers["Tmdb"] = id
		case "tvdb":
			providers["Tvdb"] = id
		}
	}
	return providers
}

// plexContainerJSON is a page of a Plex list.
type plexContainerJSON struct {
	MediaContainer struct {
		TotalSize *int           `json:"totalSize"`
		Metadata  []plexItemJSON `json:"Metadata"`
		Directory []struct {
			Key, Type string
		} `json:"Directory"`
		Account []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"Account"`
	}
}

// plexLibrary is what an import reads of a Plex server's library sections
// once: the movies and shows by their rating key, with their identifiers
// and the owner's data, and the sections of shows.
type plexLibrary struct {
	movies, shows map[string]plexItemJSON
	// movieOrder lists the movies in the order read.
	movieOrder   []string
	showSections []string
}

// plexConnect reads which Plex Media Server the address leads to, then,
// with token, its name and the accounts it knows, its owner first.
func (c *client) plexConnect(ctx context.Context, token string) (Server, []User, error) {
	var identity struct {
		MediaContainer struct {
			MachineIdentifier string `json:"machineIdentifier"`
			Version           string `json:"version"`
		}
	}
	err := c.get(ctx, "/identity", nil, &identity)
	if errors.Is(err, ErrKeyRefused) || errors.Is(err, ErrForbidden) || err == nil && identity.MediaContainer.MachineIdentifier == "" {
		// Plex answers it to anyone.
		err = ErrNotJellyfin
	}
	if err != nil {
		return Server{}, nil, err
	}
	c.key = token
	var root struct {
		MediaContainer struct {
			FriendlyName string `json:"friendlyName"`
		}
	}
	if err := c.get(ctx, "/", nil, &root); err != nil {
		return Server{}, nil, err
	}
	// Only the owner's token lists the accounts.
	var accounts plexContainerJSON
	if err := c.get(ctx, "/accounts", nil, &accounts); err != nil {
		return Server{}, nil, err
	}
	var users []User
	for _, account := range accounts.MediaContainer.Account {
		// Plex lists an account 0 without a name, which is nobody.
		if account.ID <= 0 || account.Name == "" {
			continue
		}
		user := User{ID: strconv.Itoa(account.ID), Name: account.Name, Administrator: strconv.Itoa(account.ID) == plexOwner}
		if user.Administrator {
			users = append([]User{user}, users...)
		} else {
			users = append(users, user)
		}
	}
	return Server{Kind: Plex, Name: root.MediaContainer.FriendlyName, Version: identity.MediaContainer.Version, Address: c.address}, users, nil
}

// plexList reads, page by page, the items of the list at path with query,
// handing each to found.
func (c *client) plexList(ctx context.Context, path string, query url.Values, found func(plexItemJSON)) error {
	for start := 0; ; {
		page := maps.Clone(query)
		page.Set("X-Plex-Container-Start", strconv.Itoa(start))
		page.Set("X-Plex-Container-Size", strconv.Itoa(c.s.timing.pageSize))
		var list plexContainerJSON
		if err := c.get(ctx, path, page, &list); err != nil {
			return err
		}
		items := list.MediaContainer.Metadata
		for _, item := range items {
			found(item)
		}
		start += len(items)
		// A list without totalSize was not paged.
		if total := list.MediaContainer.TotalSize; len(items) == 0 || total == nil || start >= *total {
			return nil
		}
	}
}

// library reads the movies and shows of the server's library sections,
// with their identifiers and the owner's data of the movies, once.
func (c *client) library(ctx context.Context) (*plexLibrary, error) {
	if c.plex != nil {
		return c.plex, nil
	}
	var sections plexContainerJSON
	if err := c.get(ctx, "/library/sections", nil, &sections); err != nil {
		return nil, err
	}
	lib := &plexLibrary{movies: map[string]plexItemJSON{}, shows: map[string]plexItemJSON{}}
	for _, section := range sections.MediaContainer.Directory {
		var into map[string]plexItemJSON
		var kind string
		switch section.Type {
		case plexMovie:
			into, kind = lib.movies, "1"
		case plexShow:
			into, kind = lib.shows, "2"
			lib.showSections = append(lib.showSections, section.Key)
		default:
			continue
		}
		err := c.plexList(ctx, "/library/sections/"+url.PathEscape(section.Key)+"/all", url.Values{"type": {kind}, "includeGuids": {"1"}},
			func(item plexItemJSON) {
				if _, seen := into[item.RatingKey]; !seen && section.Type == plexMovie {
					lib.movieOrder = append(lib.movieOrder, item.RatingKey)
				}
				into[item.RatingKey] = item
			})
		if err != nil {
			return nil, err
		}
	}
	c.plex = lib
	return lib, nil
}

// plexWatched reads the watch data of the Plex account user, handing each
// item read to found, and records the identifiers of the shows of its
// episodes in series, by the show's rating key. The owner's comes from the
// library sections: the movies and episodes played, with their play counts
// and the date last played, and those with a resume point. Another
// account's is its playback history: the movies and episodes it played,
// each play dated. Plex does not let the owner's token read their resume
// points, and has no favorites.
func (c *client) plexWatched(ctx context.Context, user string, series map[string]map[string]string, found func()) (history, error) {
	var h history
	lib, err := c.library(ctx)
	if err != nil {
		return h, err
	}
	add := func(p plexItemJSON) {
		item := lib.item(p, series)
		item.UserData.PlayCount = p.ViewCount
		item.UserData.LastPlayedDate = plexDate(p.LastViewedAt)
		if p.ViewCount > 0 {
			item.UserData.Played = true
			h.played = append(h.played, item)
			found()
		}
		if p.ViewOffset > 0 {
			item.UserData.PlaybackPositionTicks = int64(time.Duration(p.ViewOffset) * time.Millisecond / tick)
			if p.Duration > 0 {
				item.UserData.PlayedPercentage = float64(p.ViewOffset) * 100 / float64(p.Duration)
			}
			h.resumes = append(h.resumes, item)
			found()
		}
	}
	if user == plexOwner {
		for _, key := range lib.movieOrder {
			add(lib.movies[key])
		}
		for _, section := range lib.showSections {
			if err := c.plexList(ctx, "/library/sections/"+url.PathEscape(section)+"/all", url.Values{"type": {"4"}}, add); err != nil {
				return h, err
			}
		}
		return h, nil
	}
	// Each play of the history is a line: an item's plays are counted, and
	// its latest dates it.
	plays := map[string]*plexItemJSON{}
	var order []string
	err = c.plexList(ctx, "/status/sessions/history/all", url.Values{"accountID": {user}, "sort": {"viewedAt:desc"}}, func(p plexItemJSON) {
		if p.Type != plexMovie && p.Type != plexEpisode || p.RatingKey == "" {
			return
		}
		seen := plays[p.RatingKey]
		if seen == nil {
			seen = &p
			seen.ViewCount, seen.LastViewedAt = 0, 0
			plays[p.RatingKey] = seen
			order = append(order, p.RatingKey)
		}
		seen.ViewCount++
		seen.LastViewedAt = max(seen.LastViewedAt, p.ViewedAt)
	})
	if err != nil {
		return h, err
	}
	for _, key := range order {
		p := plays[key]
		item := lib.item(*p, series)
		item.UserData.Played, item.UserData.PlayCount, item.UserData.LastPlayedDate = true, p.ViewCount, plexDate(p.LastViewedAt)
		h.played = append(h.played, item)
		found()
	}
	return h, nil
}

// item is p as Jellyfin describes it, without the user's data: a movie
// with the identifiers its library section gives, or an episode of a show
// whose identifiers go into series.
func (lib *plexLibrary) item(p plexItemJSON, series map[string]map[string]string) itemJSON {
	item := itemJSON{ID: p.RatingKey, Name: p.Title, ProductionYear: p.Year, RunTimeTicks: int64(time.Duration(p.Duration) * time.Millisecond / tick)}
	switch p.Type {
	case plexMovie:
		item.Type = "Movie"
		// The history leaves the identifiers out: the library section
		// gives them.
		if listed, ok := lib.movies[p.RatingKey]; ok {
			item.ProviderIDs, item.ProductionYear = listed.providers(), listed.Year
		}
	case plexEpisode:
		item.Type = "Episode"
		item.SeriesID, item.SeriesName, item.ParentIndexNumber, item.IndexNumber = p.show(), p.GrandparentTitle, p.ParentIndex, p.Index
		// A show no longer in a library section has no identifier.
		show, listed := lib.shows[item.SeriesID]
		series[item.SeriesID] = nil
		if listed {
			series[item.SeriesID] = show.providers()
		}
	}
	return item
}

// plexDate writes a Plex time in seconds as Jellyfin writes dates, empty
// for none.
func plexDate(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}
