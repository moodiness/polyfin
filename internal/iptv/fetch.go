package iptv

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	// maxListSize bounds a download of a list, and maxChannels the
	// channels kept from it; maxVODListSize bounds a list read for its
	// movies or series, and maxTitles the movies, series or episodes kept
	// from it.
	maxListSize    = 100 << 20
	maxChannels    = 100_000
	maxVODListSize = 400 << 20
	maxTitles      = 400_000
	// A list download must answer within listAnswer, send something at
	// least every listStall and end within listTimeout.
	listAnswer  = 30 * time.Second
	listStall   = time.Minute
	listTimeout = 5 * time.Minute
)

// parts are what a download reads of an account: its live channels, its
// movies, its series. An M3U playlist holds them all, read at once.
type parts struct{ live, movies, series bool }

// livePart reads only the live channels, as before movies and series.
var livePart = parts{live: true}

// partsOf are the parts options import.
func partsOf(o Options) parts { return parts{live: o.LiveTv, movies: o.Movies, series: o.Series} }

// missing are the parts of want that got lacks.
func (want parts) missing(got parts) parts {
	return parts{live: want.live && !got.live, movies: want.movies && !got.movies, series: want.series && !got.series}
}

func (p parts) any() bool { return p.live || p.movies || p.series }

// downloaded is what downloads of an account brought: the entries of an
// M3U playlist, of every kind, or of an Xtream account's live list; an
// Xtream account's movies and series. got tells the parts read.
type downloaded struct {
	got            parts
	entries        []Entry
	movies, series []Title
	// xtream tells movies and series are an Xtream account's lists; an
	// M3U playlist's are among its entries.
	xtream bool
	// connections is how many streams an Xtream account may play at once,
	// as its login says; nil when it does not say, 0 for no limit.
	connections *int
}

// merge adds to d the parts of more that d lacks.
func (d *downloaded) merge(more downloaded) {
	d.xtream = d.xtream || more.xtream
	if d.connections == nil {
		d.connections = more.connections
	}
	if more.got.live && !d.got.live {
		d.entries, d.got.live = more.entries, true
	}
	if more.got.movies && !d.got.movies {
		d.movies, d.got.movies = more.movies, true
	}
	if more.got.series && !d.got.series {
		d.series, d.got.series = more.series, true
	}
}

// fetch downloads the parts of an account that want names: an M3U
// playlist whole, or the Xtream player API lists of each part. confined
// keeps the requests on public addresses. Errors never contain an address:
// they embed credentials. Entries of a kind not wanted as titles count as
// channels, as they become.
func fetch(ctx context.Context, client requester, account Account, confined bool, want parts) (downloaded, error) {
	var d downloaded
	channels, titles := 0, 0
	add := func(e Entry) error {
		vod := e.Kind == KindMovie && want.movies || e.Kind == KindEpisode && want.series
		switch {
		case vod && titles >= maxTitles, !vod && channels >= maxChannels:
			return ErrTooLarge
		case vod:
			titles++
		default:
			channels++
		}
		d.entries = append(d.entries, e)
		return nil
	}
	var err error
	switch account.Kind {
	case addons.KindM3U:
		limit := int64(maxListSize)
		if want.movies || want.series {
			limit = maxVODListSize
		}
		err = download(ctx, client, account.URL, confined, limit, func(body io.Reader) error { return ParseM3U(body, add) })
		d.got = parts{live: true, movies: true, series: true}
	case addons.KindXtream:
		d, err = fetchXtream(ctx, client, account, confined, want)
	default:
		err = ErrInvalidAddress
	}
	return d, err
}

// download requests address in its host's turn (see Pacer) and hands its
// body to read, bounded in size (limit) and time (see listTimeout). A
// provider that asks to slow down moves its host's next turn back by what
// it asks; the request is tried again then, twice at most, when that is
// within maxRetryAfter, else it fails with a RateLimitError.
func download(ctx context.Context, client requester, address string, confined bool, limit int64, read func(io.Reader) error) error {
	for attempt := 0; ; attempt++ {
		err := downloadOnce(ctx, client, address, confined, limit, read)
		var limited *RateLimitError
		if !errors.As(err, &limited) {
			return err
		}
		wait := cmp.Or(limited.RetryAfter, RequestGap*2)
		client.pacer.Delay(address, wait)
		if attempt >= 2 || wait > maxRetryAfter {
			return err
		}
	}
}

func downloadOnce(ctx context.Context, client requester, address string, confined bool, limit int64, read func(io.Reader) error) error {
	if err := client.pacer.Wait(ctx, address); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	answered := time.AfterFunc(listAnswer, cancel)
	response, err := client.client.Open(ctx, http.MethodGet, address, nil, confined)
	answered.Stop()
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if limited := RateLimited(response); limited != nil {
		return limited
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", stremio.ErrUnreachable, response.StatusCode)
	}
	stall := time.AfterFunc(listStall, cancel)
	defer stall.Stop()
	err = read(&bounded{r: response.Body, left: limit, stall: stall})
	if ctx.Err() != nil && !errors.Is(err, ErrTooLarge) && !errors.Is(err, ErrInvalidList) {
		return fmt.Errorf("%w: %v", stremio.ErrUnreachable, ctx.Err())
	}
	return err
}

// bounded reads at most left bytes, then fails with ErrTooLarge; each read
// that brings something pushes the stall timer back.
type bounded struct {
	r     io.Reader
	left  int64
	stall *time.Timer
}

func (b *bounded) Read(p []byte) (int, error) {
	if b.left <= 0 {
		var probe [1]byte
		if n, _ := b.r.Read(probe[:]); n > 0 {
			return 0, ErrTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.r.Read(p)
	b.left -= int64(n)
	if n > 0 {
		b.stall.Reset(listStall)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		err = fmt.Errorf("%w: %v", stremio.ErrUnreachable, err)
	}
	return n, err
}

// text reads a JSON value Xtream servers send as a string or a number.
type text string

func (t *text) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*t = text(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		*t = ""
		return nil
	}
	*t = text(n.String())
	return nil
}

// fetchXtream reads the parts of an Xtream Codes account that want names
// through its player API: the account's allowed formats, then the live
// categories and streams, the VOD categories and streams, and the series
// categories and series. Only list data is read: details of a title are
// asked when it is opened (see Service.details).
func fetchXtream(ctx context.Context, client requester, account Account, confined bool, want parts) (downloaded, error) {
	d := downloaded{got: want, xtream: true}
	var login struct {
		UserInfo struct {
			Auth           text     `json:"auth"`
			Formats        []string `json:"allowed_output_formats"`
			MaxConnections text     `json:"max_connections"`
		} `json:"user_info"`
	}
	if err := download(ctx, client, account.api(""), confined, maxListSize, decodeJSON(&login)); err != nil {
		return d, err
	}
	if login.UserInfo.Auth != "1" {
		return d, ErrLoginRefused
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(login.UserInfo.MaxConnections))); err == nil && n >= 0 {
		d.connections = &n
	}
	if want.live {
		// MPEG-TS, unless the account only allows HLS.
		hls := len(login.UserInfo.Formats) > 0 && !slices.Contains(login.UserInfo.Formats, "ts") && slices.Contains(login.UserInfo.Formats, "m3u8")
		if err := fetchXtreamLive(ctx, client, account, confined, hls, func(e Entry) error {
			if len(d.entries) >= maxChannels {
				return ErrTooLarge
			}
			d.entries = append(d.entries, e)
			return nil
		}); err != nil {
			return d, err
		}
	}
	for _, part := range []struct {
		wanted bool
		typ    string
		into   *[]Title
	}{{want.movies, typeMovie, &d.movies}, {want.series, typeSeries, &d.series}} {
		if !part.wanted {
			continue
		}
		titles, err := fetchXtreamTitles(ctx, client, account, confined, part.typ)
		if err != nil {
			return d, err
		}
		*part.into = titles
	}
	return d, nil
}

// api is the address of an action of an Xtream account's player API.
func (a Account) api(action string) string {
	address, _ := a.address()
	if action != "" {
		address += "&action=" + action
	}
	return address
}

// xtreamCategories reads the names of a category list of the player API.
func xtreamCategories(ctx context.Context, client requester, account Account, confined bool, action string) (map[text]string, error) {
	var categories []struct {
		ID   text   `json:"category_id"`
		Name string `json:"category_name"`
	}
	if err := download(ctx, client, account.api(action), confined, maxListSize, decodeJSON(&categories)); err != nil {
		return nil, err
	}
	names := make(map[text]string, len(categories))
	for _, category := range categories {
		names[category.ID] = strings.TrimSpace(category.Name)
	}
	return names, nil
}

// eachItem decodes the items of a JSON array one at a time.
func eachItem[T any](body io.Reader, item func(T) error) error {
	decoder := json.NewDecoder(body)
	if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
		return ErrInvalidList
	}
	for decoder.More() {
		var value T
		if err := decoder.Decode(&value); err != nil {
			return jsonError(err)
		}
		if err := item(value); err != nil {
			return err
		}
	}
	return nil
}

// fetchXtreamLive lists an Xtream account's live channels.
func fetchXtreamLive(ctx context.Context, client requester, account Account, confined, hls bool, add func(Entry) error) error {
	names, err := xtreamCategories(ctx, client, account, confined, "get_live_categories")
	if err != nil {
		return err
	}
	type stream struct {
		Number   text   `json:"num"`
		Name     string `json:"name"`
		ID       text   `json:"stream_id"`
		Icon     string `json:"stream_icon"`
		Guide    string `json:"epg_channel_id"`
		Category text   `json:"category_id"`
	}
	return download(ctx, client, account.api("get_live_streams"), confined, maxListSize, func(body io.Reader) error {
		return eachItem(body, func(stream stream) error {
			name := strings.TrimSpace(stream.Name)
			if stream.ID == "" || heading(name) {
				return nil
			}
			number, _ := strconv.Atoi(string(stream.Number))
			return add(Entry{ID: string(stream.ID), Name: name, Number: max(number, 0), Logo: strings.TrimSpace(stream.Icon),
				Group: names[stream.Category], GuideID: strings.TrimSpace(stream.Guide), URL: account.streamURL(string(stream.ID), hls)})
		})
	})
}

// xtreamTitle is a movie of get_vod_streams or a series of get_series, as
// the player API lists them.
type xtreamTitle struct {
	Name      string          `json:"name"`
	StreamID  text            `json:"stream_id"`
	SeriesID  text            `json:"series_id"`
	Icon      string          `json:"stream_icon"`
	Cover     string          `json:"cover"`
	Rating    text            `json:"rating"`
	Added     text            `json:"added"`
	Modified  text            `json:"last_modified"`
	Category  text            `json:"category_id"`
	Extension string          `json:"container_extension"`
	TMDB      text            `json:"tmdb"`
	TMDBID    text            `json:"tmdb_id"`
	IMDB      text            `json:"imdb"`
	Year      text            `json:"year"`
	Plot      string          `json:"plot"`
	Cast      string          `json:"cast"`
	Director  string          `json:"director"`
	Genre     string          `json:"genre"`
	Released  string          `json:"releaseDate"`
	Released2 string          `json:"release_date"`
	Backdrop  json.RawMessage `json:"backdrop_path"`
	Trailer   string          `json:"youtube_trailer"`
}

// fetchXtreamTitles lists an Xtream account's movies or series, in its
// order.
func fetchXtreamTitles(ctx context.Context, client requester, account Account, confined bool, typ string) ([]Title, error) {
	categoriesAction, listAction := "get_vod_categories", "get_vod_streams"
	if typ == typeSeries {
		categoriesAction, listAction = "get_series_categories", "get_series"
	}
	names, err := xtreamCategories(ctx, client, account, confined, categoriesAction)
	if err != nil {
		return nil, err
	}
	var titles []Title
	err = download(ctx, client, account.api(listAction), confined, maxVODListSize, func(body io.Reader) error {
		return eachItem(body, func(item xtreamTitle) error {
			key := string(item.StreamID)
			if typ == typeSeries {
				key = string(item.SeriesID)
			}
			raw := strings.TrimSpace(item.Name)
			if key == "" || raw == "" || heading(raw) {
				return nil
			}
			if len(titles) >= maxTitles {
				return ErrTooLarge
			}
			name, year := titleName(raw)
			if y, err := strconv.Atoi(string(item.Year)); err == nil && y >= 1800 && y <= 3000 {
				year = y
			}
			t := Title{Type: typ, Key: key, Name: name, Year: year, Category: names[item.Category], Poster: strings.TrimSpace(cmp.Or(item.Icon, item.Cover)),
				Rating: ratingText(string(item.Rating)), Extension: strings.Trim(strings.TrimSpace(item.Extension), "."),
				TMDB: numeric(cmp.Or(string(item.TMDB), string(item.TMDBID))), IMDb: imdbID(string(item.IMDB))}
			t.Quality, _ = streamQuality(raw)
			if added, err := strconv.ParseInt(string(item.Added), 10, 64); err == nil && added > 0 {
				at := time.Unix(added, 0).UTC()
				t.Added = &at
			}
			if typ == typeSeries {
				t.Version = string(item.Modified)
				released := cmp.Or(item.Released, item.Released2)
				if t.Year == 0 {
					t.Year = yearOf(released)
				}
				t.Listing = listing{Plot: strings.TrimSpace(item.Plot), Cast: splitNames(item.Cast), Director: splitNames(item.Director),
					Genres: splitNames(item.Genre), Released: released, Background: firstImage(item.Backdrop), Trailer: strings.TrimSpace(item.Trailer)}
			}
			titles = append(titles, t)
			return nil
		})
	})
	return titles, err
}

func decodeJSON(into any) func(io.Reader) error {
	return func(body io.Reader) error { return jsonError(json.NewDecoder(body).Decode(into)) }
}

// jsonError reports a response that is not the JSON expected as an invalid
// list.
func jsonError(err error) error {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typeErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return ErrInvalidList
	}
	return err
}
