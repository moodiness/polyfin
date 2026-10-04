package eclipse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Track is a song, an audiobook or an episode of a podcast.
type Track struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album,omitempty"`
	// AlbumID is the album a track belongs to, when the addon says, or
	// the album page listed it.
	AlbumID    string  `json:"albumId,omitempty"`
	Duration   Seconds `json:"duration,omitzero"`
	ArtworkURL string  `json:"artworkURL,omitempty"`
	ISRC       string  `json:"isrc,omitempty"`
	Format     string  `json:"format,omitempty"`
	StreamURL  string  `json:"streamURL,omitempty"`
	Explicit   bool    `json:"explicit,omitempty"`
	Year       int     `json:"year,omitempty"`
}

// Album is a release and, on its page, its tracks.
type Album struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	ArtworkURL  string  `json:"artworkURL,omitempty"`
	TrackCount  int     `json:"trackCount,omitempty"`
	Year        int     `json:"year,omitempty"`
	Description string  `json:"description,omitempty"`
	Explicit    bool    `json:"explicit,omitempty"`
	Tracks      []Track `json:"tracks,omitempty"`
}

// Artist is a performer and, on their page, their top tracks and albums.
type Artist struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ArtworkURL string   `json:"artworkURL,omitempty"`
	Bio        string   `json:"bio,omitempty"`
	Genres     []string `json:"genres,omitempty"`
	TopTracks  []Track  `json:"topTracks,omitempty"`
	Albums     []Album  `json:"albums,omitempty"`
}

// Playlist is a list of tracks.
type Playlist struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	ArtworkURL  string  `json:"artworkURL,omitempty"`
	Creator     string  `json:"creator,omitempty"`
	TrackCount  int     `json:"trackCount,omitempty"`
	Tracks      []Track `json:"tracks,omitempty"`
}

// Results are what a search found.
type Results struct {
	Tracks    []Track
	Albums    []Album
	Artists   []Artist
	Playlists []Playlist
}

// Chapter is where a chapter of an audiobook starts.
type Chapter struct {
	Title string
	Start time.Duration
}

// Stream is where a track plays from, and what the bytes are when the
// addon says: Codec, Container, Manifest ("none", "hls" or "dash"),
// SampleRate and BitDepth let playback decide without probing them.
type Stream struct {
	URL        string
	Format     string
	Quality    string
	Expires    time.Time
	Chapters   []Chapter
	Codec      string
	Container  string
	Manifest   string
	SampleRate int
	BitDepth   int
}

// Seconds decodes a duration given as a number of seconds, or a numeric
// string.
type Seconds time.Duration

func (s *Seconds) UnmarshalJSON(data []byte) error {
	var text stremio.Text
	if err := text.UnmarshalJSON(data); err != nil {
		return err
	}
	value, _ := strconv.ParseFloat(strings.TrimSpace(string(text)), 64)
	if value > 0 && value < 1e7 {
		*s = Seconds(time.Duration(value * float64(time.Second)))
	}
	return nil
}

func (s Seconds) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(time.Duration(s).Seconds(), 'f', -1, 64)), nil
}

// Duration is the length as a time.Duration.
func (s Seconds) Duration() time.Duration { return time.Duration(s) }

// rawTrack reads a track as addons send it: durations in seconds or, in
// catalogs, in milliseconds; years as numbers or strings.
type rawTrack struct {
	ID         stremio.Text   `json:"id"`
	Title      string         `json:"title"`
	Name       string         `json:"name"`
	Artist     string         `json:"artist"`
	Album      string         `json:"album"`
	AlbumID    stremio.Text   `json:"albumId"`
	Duration   Seconds        `json:"duration"`
	DurationMs stremio.Number `json:"durationMs"`
	ArtworkURL string         `json:"artworkURL"`
	ISRC       string         `json:"isrc"`
	Format     string         `json:"format"`
	StreamURL  string         `json:"streamURL"`
	Explicit   bool           `json:"explicit"`
	Year       stremio.Text   `json:"year"`
}

func (r rawTrack) track() Track {
	t := Track{ID: strings.TrimSpace(string(r.ID)), Title: strings.TrimSpace(r.Title), Artist: strings.TrimSpace(r.Artist),
		Album: strings.TrimSpace(r.Album), AlbumID: strings.TrimSpace(string(r.AlbumID)), Duration: r.Duration,
		ArtworkURL: artwork(r.ArtworkURL), ISRC: strings.TrimSpace(r.ISRC), Format: strings.ToLower(strings.TrimSpace(r.Format)),
		StreamURL: web(r.StreamURL), Explicit: r.Explicit, Year: year(r.Year)}
	if t.Title == "" {
		t.Title = strings.TrimSpace(r.Name)
	}
	if t.Duration == 0 && r.DurationMs > 0 {
		t.Duration = Seconds(time.Duration(r.DurationMs) * time.Millisecond)
	}
	return t
}

type rawAlbum struct {
	ID          stremio.Text   `json:"id"`
	Title       string         `json:"title"`
	Name        string         `json:"name"`
	Artist      string         `json:"artist"`
	ArtworkURL  string         `json:"artworkURL"`
	TrackCount  stremio.Number `json:"trackCount"`
	Year        stremio.Text   `json:"year"`
	Description string         `json:"description"`
	Explicit    bool           `json:"explicit"`
	Tracks      []rawTrack     `json:"tracks"`
}

func (r rawAlbum) album() Album {
	a := Album{ID: strings.TrimSpace(string(r.ID)), Title: strings.TrimSpace(r.Title), Artist: strings.TrimSpace(r.Artist),
		ArtworkURL: artwork(r.ArtworkURL), TrackCount: max(int(r.TrackCount), 0), Year: year(r.Year), Description: r.Description, Explicit: r.Explicit}
	if a.Title == "" {
		a.Title = strings.TrimSpace(r.Name)
	}
	a.Tracks = tracks(r.Tracks)
	for i := range a.Tracks {
		track := &a.Tracks[i]
		// A track listed on its album's page belongs to it.
		track.AlbumID = a.ID
		if track.Album == "" {
			track.Album = a.Title
		}
		if track.ArtworkURL == "" {
			track.ArtworkURL = a.ArtworkURL
		}
		if track.Year == 0 {
			track.Year = a.Year
		}
	}
	return a
}

type rawArtist struct {
	ID         stremio.Text  `json:"id"`
	Name       string        `json:"name"`
	Title      string        `json:"title"`
	ArtworkURL string        `json:"artworkURL"`
	Bio        string        `json:"bio"`
	Genres     stremio.Names `json:"genres"`
	TopTracks  []rawTrack    `json:"topTracks"`
	Albums     []rawAlbum    `json:"albums"`
}

func (r rawArtist) artist() Artist {
	a := Artist{ID: strings.TrimSpace(string(r.ID)), Name: strings.TrimSpace(r.Name), ArtworkURL: artwork(r.ArtworkURL), Bio: r.Bio,
		Genres: r.Genres, TopTracks: tracks(r.TopTracks)}
	if a.Name == "" {
		a.Name = strings.TrimSpace(r.Title)
	}
	for _, raw := range r.Albums {
		if album := raw.album(); album.ID != "" && album.Title != "" {
			if album.Artist == "" {
				album.Artist = a.Name
			}
			a.Albums = append(a.Albums, album)
		}
	}
	return a
}

type rawPlaylist struct {
	ID          stremio.Text   `json:"id"`
	Title       string         `json:"title"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	ArtworkURL  string         `json:"artworkURL"`
	Creator     string         `json:"creator"`
	Artist      string         `json:"artist"`
	TrackCount  stremio.Number `json:"trackCount"`
	Tracks      []rawTrack     `json:"tracks"`
}

func (r rawPlaylist) playlist() Playlist {
	p := Playlist{ID: strings.TrimSpace(string(r.ID)), Title: strings.TrimSpace(r.Title), Description: r.Description,
		ArtworkURL: artwork(r.ArtworkURL), Creator: strings.TrimSpace(r.Creator), TrackCount: max(int(r.TrackCount), 0), Tracks: tracks(r.Tracks)}
	if p.Title == "" {
		p.Title = strings.TrimSpace(r.Name)
	}
	if p.Creator == "" {
		p.Creator = strings.TrimSpace(r.Artist)
	}
	return p
}

// tracks keeps the tracks that can be played: those with an identifier
// and a title.
func tracks(raw []rawTrack) []Track {
	result := make([]Track, 0, len(raw))
	for _, r := range raw {
		if t := r.track(); t.ID != "" && t.Title != "" {
			result = append(result, t)
		}
	}
	return result
}

// artwork keeps an artwork address apps can be given through Polyfin.
func artwork(raw string) string {
	return web(raw)
}

// web keeps an http or https address.
func web(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return raw
}

// year reads a year from "2024", 2024 or a date starting with it.
func year(text stremio.Text) int {
	value := strings.TrimSpace(string(text))
	if len(value) >= 4 {
		if y, err := strconv.Atoi(value[:4]); err == nil && y > 0 {
			return y
		}
	}
	return 0
}

// Client asks Eclipse addons for their resources, through the Stremio
// client's rules: requests are bounded, confined to public addresses when
// asked, and errors never contain the URL, which may hold credentials.
type Client struct {
	http *stremio.Client
}

// NewClient returns a client fetching through http.
func NewClient(http *stremio.Client) *Client {
	return &Client{http: http}
}

// Addon is what a request needs of an addon: its manifest URL, its
// manifest, the settings values chosen for it, and whether it may only
// reach public addresses.
type Addon struct {
	ManifestURL string
	Manifest    Manifest
	Settings    map[string]string
	Confined    bool
}

// BaseURL is the address an addon's resources are under: its manifest URL
// without /manifest.json.
func BaseURL(manifestURL string) string {
	return strings.TrimSuffix(manifestURL, "/manifest.json")
}

// get asks an addon for path, with the settings and extra in the query,
// and decodes the answer into v.
func (c *Client) get(ctx context.Context, addon Addon, path string, extra url.Values, v any) error {
	query := addon.Manifest.Query(addon.Settings)
	for key, values := range extra {
		query[key] = values
	}
	target := BaseURL(addon.ManifestURL) + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	body, err := c.http.Fetch(ctx, addon.ManifestURL, target, addon.Confined)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		if _, mismatched := errors.AsType[*json.UnmarshalTypeError](err); !mismatched {
			return fmt.Errorf("%w: %v", stremio.ErrInvalidResponse, err)
		}
	}
	return nil
}

func segment(id string) string { return "/" + url.PathEscape(id) }

// Search looks a query up.
func (c *Client) Search(ctx context.Context, addon Addon, query string) (Results, error) {
	var raw struct {
		Tracks    []rawTrack    `json:"tracks"`
		Albums    []rawAlbum    `json:"albums"`
		Artists   []rawArtist   `json:"artists"`
		Playlists []rawPlaylist `json:"playlists"`
	}
	if err := c.get(ctx, addon, "/search", url.Values{"q": {query}}, &raw); err != nil {
		return Results{}, err
	}
	results := Results{Tracks: tracks(raw.Tracks)}
	for _, r := range raw.Albums {
		if a := r.album(); a.ID != "" && a.Title != "" {
			results.Albums = append(results.Albums, a)
		}
	}
	for _, r := range raw.Artists {
		if a := r.artist(); a.ID != "" && a.Name != "" {
			results.Artists = append(results.Artists, a)
		}
	}
	for _, r := range raw.Playlists {
		if p := r.playlist(); p.ID != "" && p.Title != "" {
			results.Playlists = append(results.Playlists, p)
		}
	}
	return results, nil
}

// Album returns an album's page, with its tracks.
func (c *Client) Album(ctx context.Context, addon Addon, id string) (Album, error) {
	var raw rawAlbum
	if err := c.get(ctx, addon, "/album"+segment(id), nil, &raw); err != nil {
		return Album{}, err
	}
	album := raw.album()
	if album.Title == "" {
		return Album{}, stremio.ErrNotFound
	}
	album.ID = id
	for i := range album.Tracks {
		album.Tracks[i].AlbumID = id
	}
	return album, nil
}

// Artist returns an artist's page, with their top tracks and albums.
func (c *Client) Artist(ctx context.Context, addon Addon, id string) (Artist, error) {
	var raw rawArtist
	if err := c.get(ctx, addon, "/artist"+segment(id), nil, &raw); err != nil {
		return Artist{}, err
	}
	artist := raw.artist()
	if artist.Name == "" {
		return Artist{}, stremio.ErrNotFound
	}
	artist.ID = id
	return artist, nil
}

// Playlist returns a playlist's page, with its tracks.
func (c *Client) Playlist(ctx context.Context, addon Addon, id string) (Playlist, error) {
	var raw rawPlaylist
	if err := c.get(ctx, addon, "/playlist"+segment(id), nil, &raw); err != nil {
		return Playlist{}, err
	}
	playlist := raw.playlist()
	if playlist.Title == "" {
		return Playlist{}, stremio.ErrNotFound
	}
	playlist.ID = id
	return playlist, nil
}

// CatalogPage is the length of a catalog page: a shorter page ends the row.
const CatalogPage = 100

// Item is an entry of a catalog row, of the row's type.
type Item struct {
	Track    *Track
	Album    *Album
	Artist   *Artist
	Playlist *Playlist
}

// Catalog lists a page of a catalog row, from skip, a multiple of
// CatalogPage. Entries of another type than the row's are left out, but
// count in the page's length, which the returned count is.
func (c *Client) Catalog(ctx context.Context, addon Addon, catalog Catalog, skip int) ([]Item, int, error) {
	var raw struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := c.get(ctx, addon, "/catalog"+segment(catalog.ID), url.Values{"skip": {strconv.Itoa(skip)}}, &raw); err != nil {
		return nil, 0, err
	}
	items := make([]Item, 0, len(raw.Items))
	for _, entry := range raw.Items {
		var kind struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(entry, &kind)
		if kind.Type != "" && kind.Type != catalog.Type {
			continue
		}
		switch catalog.Type {
		case TypeTrack:
			var r rawTrack
			if json.Unmarshal(entry, &r) == nil {
				if t := r.track(); t.ID != "" && t.Title != "" {
					items = append(items, Item{Track: &t})
				}
			}
		case TypeAlbum:
			var r rawAlbum
			if json.Unmarshal(entry, &r) == nil {
				if a := r.album(); a.ID != "" && a.Title != "" {
					items = append(items, Item{Album: &a})
				}
			}
		case TypeArtist:
			var r rawArtist
			if json.Unmarshal(entry, &r) == nil {
				if a := r.artist(); a.ID != "" && a.Name != "" {
					items = append(items, Item{Artist: &a})
				}
			}
		case TypePlaylist:
			var r rawPlaylist
			if json.Unmarshal(entry, &r) == nil {
				if p := r.playlist(); p.ID != "" && p.Title != "" {
					items = append(items, Item{Playlist: &p})
				}
			}
		}
	}
	return items, len(raw.Items), nil
}

// Stream resolves where a track plays from.
func (c *Client) Stream(ctx context.Context, addon Addon, id string) (Stream, error) {
	var raw struct {
		URL       string         `json:"url"`
		Format    string         `json:"format"`
		Quality   string         `json:"quality"`
		ExpiresAt stremio.Number `json:"expiresAt"`
		Chapters  []struct {
			Title     string  `json:"title"`
			StartTime float64 `json:"startTime"`
		} `json:"chapters"`
		Codec      string         `json:"codec"`
		Container  string         `json:"container"`
		Manifest   string         `json:"manifest"`
		SampleRate stremio.Number `json:"sampleRate"`
		BitDepth   stremio.Number `json:"bitDepth"`
	}
	if err := c.get(ctx, addon, "/stream"+segment(id), nil, &raw); err != nil {
		return Stream{}, err
	}
	stream := Stream{URL: web(raw.URL), Format: strings.ToLower(strings.TrimSpace(raw.Format)), Quality: raw.Quality,
		Codec: strings.ToLower(strings.TrimSpace(raw.Codec)), Container: strings.ToLower(strings.TrimSpace(raw.Container)),
		Manifest: strings.ToLower(strings.TrimSpace(raw.Manifest)), SampleRate: max(int(raw.SampleRate), 0), BitDepth: max(int(raw.BitDepth), 0)}
	if stream.URL == "" {
		return Stream{}, fmt.Errorf("%w: a stream without an http address", stremio.ErrInvalidResponse)
	}
	if raw.ExpiresAt > 0 {
		// Seconds, though some send milliseconds.
		at := int64(raw.ExpiresAt)
		if at > 1e12 {
			stream.Expires = time.UnixMilli(at)
		} else {
			stream.Expires = time.Unix(at, 0)
		}
	}
	for _, chapter := range raw.Chapters {
		if chapter.StartTime >= 0 {
			stream.Chapters = append(stream.Chapters, Chapter{Title: strings.TrimSpace(chapter.Title),
				Start: time.Duration(chapter.StartTime * float64(time.Second))})
		}
	}
	return stream, nil
}
