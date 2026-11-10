package iptv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/addons"
)

// A synthetic Xtream Codes server: an invented provider with tens of
// thousands of channels in hundreds of groups, several quality variants of
// most channels, and an XMLTV guide covering most of them. Every name is
// made up.

// syntheticCountries prefix the groups and channel names, as providers do;
// the last groups have none.
var syntheticCountries = []string{"FR", "DE", "UK", "ES", "IT", "NL", "BE", "PT", "PL", "SE", "US", "CA", "BR", "TR", "GR", "RO", "AR", "MX"}

var syntheticSyllables = []string{"zeb", "orb", "quil", "lum", "tac", "pif", "nor", "vel", "dax", "mir", "sol", "kep", "rux", "bal", "tor", "fen"}

var syntheticThemes = []string{"News", "Sports", "Kids", "Movies", "Music", "Docs", "Series", "Local", "Culture", "Comedy"}

// syntheticVariants are the quality variants a channel may come in: the
// first channel of every three has three, the second two, the third one.
var syntheticVariants = [][]string{{"FHD", "HD", "SD"}, {"4K", "HD"}, {""}}

type syntheticGroup struct {
	ID      int
	Name    string
	Country string
}

type syntheticStream struct {
	ID      int
	Number  int
	Name    string
	Group   int
	GuideID string
	Logo    string
}

// syntheticChannel is a base channel: its name and guide identifier, ""
// for one the guide does not cover.
type syntheticChannel struct {
	name, guideID string
}

type synthetic struct {
	groups   []syntheticGroup
	streams  []syntheticStream
	channels []syntheticChannel
	// movies and series are the provider's VOD, in vodGroups and
	// seriesGroups (see withVOD).
	movies       []syntheticMovie
	series       []syntheticSeries
	vodGroups    []string
	seriesGroups []string
}

// syntheticMovie is a movie of the provider: its name with the provider's
// dressing, its category (index in vodGroups), container, the day it was
// added and its TMDB id ("" for none).
type syntheticMovie struct {
	ID        int
	Name      string
	Group     int
	Extension string
	Added     int64
	TMDB      string
}

// syntheticSeries is a series of the provider, with seasons of episodes
// each; modified tells its details changed.
type syntheticSeries struct {
	ID       int
	Name     string
	Group    int
	Seasons  int
	Episodes int
	Modified int64
}

var syntheticGenres = []string{"Action", "Drama", "Comedy", "Kids", "Documentary", "Thriller", "Animation", "Science Fiction"}

// withVOD adds movies and series to the provider, in hundreds of
// categories for large numbers: names dressed as providers do (country
// prefix, quality, year), several containers, TMDB ids for most movies.
func (s *synthetic) withVOD(movies, series int) *synthetic {
	groups := max(movies/200, 3)
	for g := range groups {
		s.vodGroups = append(s.vodGroups, fmt.Sprintf("%s| %s %d", syntheticCountries[g%len(syntheticCountries)], syntheticGenres[g%len(syntheticGenres)], g/len(syntheticGenres)+1))
	}
	for g := range max(series/100, 2) {
		s.seriesGroups = append(s.seriesGroups, fmt.Sprintf("%s Series %d", syntheticGenres[g%len(syntheticGenres)], g/len(syntheticGenres)+1))
	}
	extensions := []string{"mkv", "mp4", "ts"}
	qualities := []string{" 4K", " FHD", "", " HD"}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	for i := range movies {
		word := syntheticSyllables[i%len(syntheticSyllables)] + syntheticSyllables[i/len(syntheticSyllables)%len(syntheticSyllables)]
		name := fmt.Sprintf("%s - %s%s %d (%d)%s", syntheticCountries[i%len(syntheticCountries)], strings.ToUpper(word[:1]), word[1:], i,
			1970+i%55, qualities[i%len(qualities)])
		tmdb := ""
		if i%4 != 3 {
			tmdb = fmt.Sprint(1000 + i)
		}
		s.movies = append(s.movies, syntheticMovie{ID: 10_000_000 + i, Name: name, Group: i % groups, Extension: extensions[i%len(extensions)],
			Added: base + int64(i)*3600, TMDB: tmdb})
	}
	for i := range series {
		word := syntheticSyllables[(i+3)%len(syntheticSyllables)] + syntheticSyllables[i/len(syntheticSyllables)%len(syntheticSyllables)]
		s.series = append(s.series, syntheticSeries{ID: 20_000_000 + i, Name: fmt.Sprintf("%s%s Show %d", strings.ToUpper(word[:1]), word[1:], i),
			Group: i % len(s.seriesGroups), Seasons: 1 + i%3, Episodes: 4 + i%5, Modified: base + int64(i)})
	}
	return s
}

// newSynthetic makes about streams entries in groups groups.
func newSynthetic(streams, groups int) *synthetic {
	s := &synthetic{}
	for g := range groups {
		country := ""
		if g < groups*9/10 {
			country = syntheticCountries[g%len(syntheticCountries)]
		}
		name := fmt.Sprintf("%s %d", syntheticThemes[g%len(syntheticThemes)], g/len(syntheticThemes)+1)
		if country != "" {
			name = country + "| " + name
		}
		s.groups = append(s.groups, syntheticGroup{ID: g + 1, Name: name, Country: country})
	}
	perGroup := max(streams/groups, 1)
	for base := 0; len(s.streams) < streams; base++ {
		group := s.groups[min(len(s.streams)/perGroup, groups-1)]
		word := syntheticSyllables[base%len(syntheticSyllables)] + syntheticSyllables[base/len(syntheticSyllables)%len(syntheticSyllables)]
		name := strings.ToUpper(word[:1]) + word[1:] + fmt.Sprintf(" %d", base)
		guideID := ""
		if base%5 != 4 {
			guideID = fmt.Sprintf("%s%d.%s", word, base, strings.ToLower(group.Country))
			if group.Country == "" {
				guideID = fmt.Sprintf("%s%d.zz", word, base)
			}
		}
		s.channels = append(s.channels, syntheticChannel{name: name, guideID: guideID})
		for _, quality := range syntheticVariants[base%len(syntheticVariants)] {
			if len(s.streams) >= streams {
				break
			}
			shown := name
			if group.Country != "" {
				shown = group.Country + ": " + shown
			}
			if quality != "" {
				shown += " " + quality
			}
			id := len(s.streams) + 1
			s.streams = append(s.streams, syntheticStream{ID: id, Number: id, Name: shown, Group: group.ID, GuideID: guideID,
				Logo: fmt.Sprintf("https://logos.example/%d.png", base)})
		}
	}
	return s
}

// syntheticServer serves a synthetic provider for user "user", password
// "secret": the player API, the XMLTV guide, and every stream as the same
// short video when one could be made.
type syntheticServer struct {
	*httptest.Server
	data *synthetic
	// videos are the files streams play, by container (see
	// syntheticVideos).
	videos map[string][]byte
	// requests counts the player API's requests by action.
	mu       sync.Mutex
	requests map[string]int
	// at records when each details request came.
	at []time.Time
}

// count returns how many requests of action came.
func (s *syntheticServer) count(action string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[action]
}

func newSyntheticServer(data *synthetic, videos map[string][]byte) *syntheticServer {
	s := &syntheticServer{data: data, videos: videos, requests: map[string]int{}}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	return s
}

func (s *syntheticServer) serve(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	authorized := query.Get("username") == "user" && query.Get("password") == "secret"
	switch {
	case r.URL.Path == "/player_api.php" && !authorized:
		_ = json.NewEncoder(w).Encode(map[string]any{"user_info": map[string]any{"auth": 0}})
	case r.URL.Path == "/player_api.php":
		s.mu.Lock()
		s.requests[query.Get("action")]++
		if strings.HasSuffix(query.Get("action"), "_info") {
			s.at = append(s.at, time.Now())
		}
		s.mu.Unlock()
		s.playerAPI(w, query.Get("action"), query)
	case r.URL.Path == "/xmltv.php" && authorized:
		w.Header().Set("Content-Type", "application/xml")
		s.guide(w)
	case strings.HasPrefix(r.URL.Path, "/live/user/secret/") && s.videos != nil:
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write(s.videos["ts"])
	case (strings.HasPrefix(r.URL.Path, "/movie/user/secret/") || strings.HasPrefix(r.URL.Path, "/series/user/secret/")) && s.videos != nil:
		// Files are served as they are named, ranges included.
		video, ok := s.videos[strings.TrimPrefix(path.Ext(r.URL.Path), ".")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, path.Base(r.URL.Path), time.Time{}, bytes.NewReader(video))
	default:
		http.NotFound(w, r)
	}
}

func (s *syntheticServer) playerAPI(w http.ResponseWriter, action string, query url.Values) {
	w.Header().Set("Content-Type", "application/json")
	buffered := bufio.NewWriterSize(w, 1<<16)
	defer buffered.Flush()
	encoder := json.NewEncoder(buffered)
	switch action {
	case "":
		_ = encoder.Encode(map[string]any{"user_info": map[string]any{"auth": 1, "status": "Active", "allowed_output_formats": []string{"ts", "m3u8"}}})
	case "get_live_categories":
		categories := make([]map[string]any, 0, len(s.data.groups))
		for _, g := range s.data.groups {
			categories = append(categories, map[string]any{"category_id": fmt.Sprint(g.ID), "category_name": g.Name, "parent_id": 0})
		}
		_ = encoder.Encode(categories)
	case "get_live_streams":
		_, _ = buffered.WriteString("[")
		for i, stream := range s.data.streams {
			if i > 0 {
				_, _ = buffered.WriteString(",")
			}
			_ = encoder.Encode(map[string]any{"num": stream.Number, "name": stream.Name, "stream_type": "live", "stream_id": stream.ID,
				"stream_icon": stream.Logo, "epg_channel_id": stream.GuideID, "category_id": fmt.Sprint(stream.Group)})
		}
		_, _ = buffered.WriteString("]")
	case "get_vod_categories", "get_series_categories":
		names := s.data.vodGroups
		if action == "get_series_categories" {
			names = s.data.seriesGroups
		}
		categories := make([]map[string]any, 0, len(names))
		for i, name := range names {
			categories = append(categories, map[string]any{"category_id": fmt.Sprint(100 + i), "category_name": name, "parent_id": 0})
		}
		_ = encoder.Encode(categories)
	case "get_vod_streams":
		_, _ = buffered.WriteString("[")
		for i, m := range s.data.movies {
			if i > 0 {
				_, _ = buffered.WriteString(",")
			}
			_ = encoder.Encode(map[string]any{"num": i + 1, "name": m.Name, "stream_type": "movie", "stream_id": m.ID,
				"stream_icon": fmt.Sprintf("https://posters.example/m%d.jpg", m.ID), "rating": "7.2", "added": fmt.Sprint(m.Added),
				"category_id": fmt.Sprint(100 + m.Group), "container_extension": m.Extension, "tmdb": m.TMDB})
		}
		_, _ = buffered.WriteString("]")
	case "get_series":
		_, _ = buffered.WriteString("[")
		for i, show := range s.data.series {
			if i > 0 {
				_, _ = buffered.WriteString(",")
			}
			_ = encoder.Encode(map[string]any{"num": i + 1, "name": show.Name, "series_id": show.ID, "cover": fmt.Sprintf("https://posters.example/s%d.jpg", show.ID),
				"plot": "A show of the provider.", "cast": "Ann Zeb, Bob Orb", "director": "Cy Quil", "genre": "Drama", "releaseDate": "2021-03-04",
				"last_modified": fmt.Sprint(show.Modified), "rating": "8", "backdrop_path": []string{"https://posters.example/b.jpg"},
				"category_id": fmt.Sprint(100 + show.Group)})
		}
		_, _ = buffered.WriteString("]")
	case "get_vod_info":
		for _, m := range s.data.movies {
			if fmt.Sprint(m.ID) == query.Get("vod_id") {
				_ = encoder.Encode(map[string]any{"info": map[string]any{"tmdb_id": m.TMDB, "name": m.Name, "description": "The plot of " + m.Name,
					"actors": "Dee Lum, Eve Tac", "director": "Fay Pif", "genre": "Action, Drama", "releasedate": "2020-05-06", "duration_secs": 5400,
					"backdrop_path": []string{"https://posters.example/mb.jpg"}, "youtube_trailer": "abcdefghijk", "rating": "6.5"},
					"movie_data": map[string]any{"stream_id": m.ID, "container_extension": m.Extension}})
				return
			}
		}
		_ = encoder.Encode(map[string]any{"info": []any{}, "movie_data": []any{}})
	case "get_series_info":
		for _, show := range s.data.series {
			if fmt.Sprint(show.ID) != query.Get("series_id") {
				continue
			}
			episodes := map[string][]map[string]any{}
			var seasons []map[string]any
			for season := 1; season <= show.Seasons; season++ {
				seasons = append(seasons, map[string]any{"season_number": season, "cover": fmt.Sprintf("https://posters.example/s%d-%d.jpg", show.ID, season)})
				for episode := 1; episode <= show.Episodes; episode++ {
					episodes[fmt.Sprint(season)] = append(episodes[fmt.Sprint(season)], map[string]any{
						"id": fmt.Sprint(show.ID*100 + season*10 + episode), "episode_num": episode, "season": season, "container_extension": "mkv",
						"title": fmt.Sprintf("%s - S%02dE%02d - Part %d FHD", show.Name, season, episode, episode),
						"info":  map[string]any{"plot": fmt.Sprintf("Episode %d.", episode), "duration_secs": 2700, "releasedate": "2021-03-04"}})
				}
			}
			_ = encoder.Encode(map[string]any{"seasons": seasons, "info": map[string]any{"name": show.Name, "plot": "A show of the provider, in detail.",
				"genre": "Drama"}, "episodes": episodes})
			return
		}
		_ = encoder.Encode(map[string]any{"seasons": []any{}, "info": []any{}, "episodes": []any{}})
	default:
		_ = encoder.Encode([]any{})
	}
}

// guide writes the XMLTV guide: the channels with a guide identifier, each
// with two-hour programmes from six hours ago to a day ahead.
func (s *syntheticServer) guide(w http.ResponseWriter) {
	buffered := bufio.NewWriterSize(w, 1<<16)
	defer buffered.Flush()
	_, _ = buffered.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<tv>\n")
	for _, c := range s.data.channels {
		if c.guideID != "" {
			fmt.Fprintf(buffered, "<channel id=\"%s\"><display-name>%s</display-name></channel>\n", c.guideID, c.name)
		}
	}
	start := time.Now().UTC().Truncate(time.Hour).Add(-6 * time.Hour)
	for _, c := range s.data.channels {
		if c.guideID == "" {
			continue
		}
		for slot := range 15 {
			from := start.Add(time.Duration(slot) * 2 * time.Hour)
			fmt.Fprintf(buffered, "<programme channel=\"%s\" start=\"%s\" stop=\"%s\"><title>%s</title><desc>Part %d of the day on %s.</desc></programme>\n",
				c.guideID, from.Format("20060102150405 -0700"), from.Add(2*time.Hour).Format("20060102150405 -0700"),
				syntheticThemes[slot%len(syntheticThemes)], slot+1, c.name)
		}
	}
	_, _ = buffered.WriteString("</tv>\n")
}

// syntheticVideos makes ten seconds of video with the ffmpeg of
// POLYFIN_TEST_FFMPEG, by container: MPEG-TS for channels and movies,
// Matroska and MP4 for movies and episodes; nil without one.
func syntheticVideos(ctx context.Context) map[string][]byte {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		return nil
	}
	dir, err := os.MkdirTemp("", "polyfin-synthetic-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	videos := map[string][]byte{}
	for _, container := range []string{"ts", "mkv", "mp4"} {
		file := filepath.Join(dir, "video."+container)
		args := []string{"-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=25", "-f", "lavfi", "-i", "sine=frequency=440",
			"-t", "10", "-c:v", "libx264", "-preset", "veryfast", "-g", "50", "-pix_fmt", "yuv420p", "-c:a", "aac"}
		if container == "mp4" {
			args = append(args, "-movflags", "+faststart")
		}
		if err := exec.CommandContext(ctx, ffmpeg, append(args, file)...).Run(); err != nil {
			return nil
		}
		if videos[container], err = os.ReadFile(file); err != nil {
			return nil
		}
	}
	return videos
}

// TestServeSyntheticXtream serves a synthetic Xtream Codes provider until
// interrupted, for trying Polyfin by hand with a large line-up, movies and
// series. It only runs with POLYFIN_SYNTHETIC_XTREAM set: to a number of
// channels (default 54000; a third as many movies and a twentieth as many
// series come with them), or to "1" for the default. POLYFIN_SYNTHETIC_XTREAM_ADDR sets the
// address it listens on (default 127.0.0.1:0).
//
//	POLYFIN_SYNTHETIC_XTREAM=1 go test -run TestServeSyntheticXtream -v -timeout 0 ./internal/iptv
func TestServeSyntheticXtream(t *testing.T) {
	t.Parallel()
	setting := os.Getenv("POLYFIN_SYNTHETIC_XTREAM")
	if setting == "" {
		t.Skip("set POLYFIN_SYNTHETIC_XTREAM to serve a synthetic Xtream provider")
	}
	channels := 54_000
	if n := 0; setting != "1" {
		if _, err := fmt.Sscan(setting, &n); err != nil || n < 1 {
			t.Fatalf("POLYFIN_SYNTHETIC_XTREAM: %q is not a number of channels", setting)
		}
		channels = n
	}
	server := newSyntheticServer(newSynthetic(channels, max(channels/130, 1)).withVOD(channels/3, channels/20), syntheticVideos(t.Context()))
	if address := os.Getenv("POLYFIN_SYNTHETIC_XTREAM_ADDR"); address != "" {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		server.Listener.Close()
		server.Listener = listener
	}
	server.Start()
	defer server.Close()
	fmt.Printf("Synthetic Xtream provider: %d channels in %d groups, %d movies in %d categories, %d series in %d categories, video %v\n",
		len(server.data.streams), len(server.data.groups), len(server.data.movies), len(server.data.vodGroups), len(server.data.series),
		len(server.data.seriesGroups), server.videos != nil)
	fmt.Printf("  server   %s\n  username user\n  password secret\n  guide    %s/xmltv.php?username=user&password=secret\n",
		server.URL, server.URL)
	fmt.Println("Interrupt (Ctrl-C) to stop.")
	<-t.Context().Done()
}

// statementCounter counts the statements a pool sends, COPY and those of
// batches among them, and the rows the database returns to them.
type statementCounter struct{ statements, rows atomic.Int64 }

func (c *statementCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.statements.Add(1)
	return ctx
}

func (c *statementCounter) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.CommandTag.Select() {
		c.rows.Add(data.CommandTag.RowsAffected())
	}
}

func (c *statementCounter) TraceCopyFromStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceCopyFromStartData) context.Context {
	c.statements.Add(1)
	return ctx
}

func (c *statementCounter) TraceCopyFromEnd(context.Context, *pgx.Conn, pgx.TraceCopyFromEndData) {}

func (c *statementCounter) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return ctx
}

func (c *statementCounter) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {
	c.statements.Add(1)
}

func (c *statementCounter) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

// A line-up of about 54,000 entries in hundreds of groups is imported,
// refreshed, merged, listed and searched as one a tenth its size is: with
// no more statements, in at most thirty times the time, processed in at
// most twenty times the processor time, and listed a page at a time with
// at most twice the rows. Reconciling is batched, listing pages and
// searches read the database a fixed number of times and the page alone,
// and no step costs more than in proportion to the line-up. Two small
// line-ups take each step with the large one, one just before it and one
// just after, so that the machine's load weighs on the three alike, and
// the large one is compared with the slower small one. Time alone would
// not do on a loaded machine: each statement waits longer for the
// database, which weighs most on the small line-ups, mostly statements,
// so that work growing faster than the line-up could hide in the gap.
// Processor time does not count waiting, but would count the other tests'
// work, so this test runs alone. The race detector gets smaller line-ups
// (see lineupScale).
//
//nolint:paralleltest // compares its process's processor time, which the other tests' work would add to
func TestLargeLineupsStayFast(t *testing.T) {
	if testing.Short() {
		t.Skip("large line-up")
	}
	// Ten times the entries take at most about ten times as long, a step's
	// fixed costs being the same. On a loaded machine, with or without the
	// race detector, that was measured at most 14 times the time, when a
	// burst of load fell on the large line-up's turn alone, hence thirty,
	// and 10.5 times the processor time, which waiting does not count,
	// hence twenty. A cost growing with the square of the entries is a
	// hundred times larger for the large line-up: past twenty times the
	// processor time, one fails as soon as it takes about one and a half
	// times what processing the line-up does.
	const timeScaling, processorScaling = 30, 20
	statements := &statementCounter{}
	e := tracedEnv(t, statements)
	type lineup struct {
		name            string
		entries, groups int
		account         Account
		addon           addons.Addon
	}
	small := &lineup{name: "Small", entries: 5_400 / lineupScale, groups: 42 / lineupScale}
	large := &lineup{name: "Big", entries: 54_000 / lineupScale, groups: 420 / lineupScale}
	lineups := []*lineup{small, large, {name: "Other small", entries: small.entries, groups: small.groups}}
	for _, l := range lineups {
		server := newSyntheticServer(newSynthetic(l.entries, l.groups), nil)
		server.Start()
		t.Cleanup(server.Close)
		l.account = Account{Kind: addons.KindXtream, Server: server.URL, Username: "user", Password: "secret"}
	}
	// also is what a step compares besides statements and time: the
	// processor time of a step processing the line-ups, and the rows the
	// database returns to a step reading a page or a count, as many
	// whatever the line-up's size.
	type also struct{ processorTime, rows bool }
	// scaled runs a step on each line-up in turn, rounds times, and
	// compares the large one's most statements and rows and least times
	// with the small ones'.
	scaled := func(what string, rounds int, compare also, run func(l *lineup, round int) error) {
		t.Helper()
		var sent, read [3]int64
		var took, used [3]time.Duration
		for round := range rounds {
			for i, l := range lineups {
				sentBefore, readBefore := statements.statements.Load(), statements.rows.Load()
				start, usedBefore := time.Now(), processCPU()
				if err := run(l, round); err != nil {
					t.Fatalf("%s, %d entries: %v", what, l.entries, err)
				}
				tookNow, usedNow := time.Since(start), processCPU()-usedBefore
				sent[i] = max(sent[i], statements.statements.Load()-sentBefore)
				read[i] = max(read[i], statements.rows.Load()-readBefore)
				if round == 0 {
					took[i], used[i] = tookNow, usedNow
				}
				took[i], used[i] = min(took[i], tookNow), min(used[i], usedNow)
			}
		}
		smallSent, smallRead := max(sent[0], sent[2]), max(read[0], read[2])
		smallTook, smallUsed := max(took[0], took[2]), max(used[0], used[2])
		passed := true
		check := func(failed bool, format string, args ...any) {
			t.Helper()
			if failed {
				passed = false
				t.Errorf("%s %s", what, fmt.Sprintf(format, args...))
			}
		}
		check(sent[1] > smallSent, "sent %d statements for %d entries, more than the %d for %d", sent[1], large.entries, smallSent, small.entries)
		check(took[1] > timeScaling*smallTook, "took %v for %d entries, more than %d times the %v for %d", took[1].Round(time.Millisecond),
			large.entries, timeScaling, smallTook.Round(time.Millisecond), small.entries)
		check(compare.processorTime && used[1] > processorScaling*smallUsed, "used %v of processor time for %d entries, more than %d times the %v for %d",
			used[1].Round(time.Millisecond), large.entries, processorScaling, smallUsed.Round(time.Millisecond), small.entries)
		check(compare.rows && read[1] > 2*smallRead, "read %d rows for %d entries, more than twice the %d for %d", read[1], large.entries,
			smallRead, small.entries)
		if passed {
			details := ""
			if compare.processorTime {
				details = fmt.Sprintf(", and %.1f times the processor time", float64(used[1])/float64(max(smallUsed, 1)))
			}
			if compare.rows {
				details = fmt.Sprintf(", reading %d rows against %d", read[1], smallRead)
			}
			t.Logf("%s: %v for %d entries, %.1f times the %v for %d%s", what, took[1].Round(time.Millisecond), large.entries,
				float64(took[1])/float64(smallTook), smallTook.Round(time.Millisecond), small.entries, details)
		}
	}
	scaled("adding", 1, also{processorTime: true}, func(l *lineup, _ int) (err error) {
		l.addon, err = e.service.Add(t.Context(), addons.Shared(), NewSource{Name: l.name, Account: l.account}, false)
		return err
	})
	for _, l := range lineups {
		source, err := e.service.Source(t.Context(), addons.Shared(), l.addon.ID)
		if err != nil || source.Lineup.Channels != l.entries || source.Lineup.Categories != l.groups {
			t.Fatalf("line-up of %d entries: %+v %v", l.entries, source.Lineup, err)
		}
	}
	// An edit survives the refreshes and option changes below.
	_, channels, err := e.service.ListChannels(t.Context(), addons.Shared(), large.addon.ID, ChannelFilter{Q: "Zebzeb 0", Limit: 1})
	if err != nil || len(channels) != 1 {
		t.Fatalf("search: %+v %v", channels, err)
	}
	if _, err := e.service.UpdateChannel(t.Context(), addons.Shared(), large.addon.ID, channels[0].ID, ChannelChanges{Name: &Nullable[string]{Value: new("Kept")}}); err != nil {
		t.Fatal(err)
	}
	scaled("refreshing unchanged", 1, also{processorTime: true}, func(l *lineup, _ int) error {
		return e.service.Refresh(t.Context(), addons.Shared(), l.addon.ID, false)
	})
	merged := ChannelsMerged
	scaled("merging", 1, also{processorTime: true}, func(l *lineup, _ int) error {
		return e.service.Update(t.Context(), addons.Shared(), l.addon.ID, Changes{Options: &OptionsPatch{Channels: &merged}}, false)
	})
	byCountry := CategoriesCountry
	scaled("by country", 1, also{processorTime: true}, func(l *lineup, _ int) error {
		return e.service.Update(t.Context(), addons.Shared(), l.addon.ID, Changes{Options: &OptionsPatch{Categories: &byCountry}}, false)
	})
	if kept, err := e.service.Channel(t.Context(), addons.Shared(), large.addon.ID, channels[0].ID); err != nil || kept.Name != "Kept" || len(kept.Streams) != 3 {
		t.Errorf("the edited channel after merging: %+v %v", kept, err)
	}
	scaled("listing the first page", 5, also{rows: true}, func(l *lineup, _ int) error {
		_, page, err := e.service.ListChannels(t.Context(), addons.Shared(), l.addon.ID, ChannelFilter{Limit: 500 / lineupScale})
		if err == nil && len(page) != 500/lineupScale {
			err = fmt.Errorf("%d channels", len(page))
		}
		return err
	})
	// A page far down costs a few times what the first one does, not tens
	// of times: the listing reads the page alone, and the database skips
	// the channels before it. The two pages are listed in turn, five times,
	// so that the machine's load weighs on both alike, and their fastest
	// times are compared. Alone, the far page takes about four times as
	// long as the first, and two and a half under the race detector.
	listPage := func(offset int) time.Duration {
		t.Helper()
		start := time.Now()
		total, page, err := e.service.ListChannels(t.Context(), addons.Shared(), large.addon.ID, ChannelFilter{Offset: offset, Limit: 500 / lineupScale})
		took := time.Since(start)
		if err == nil && (len(page) != 500/lineupScale || total < 20_500/lineupScale) {
			err = fmt.Errorf("%d of %d", len(page), total)
		}
		if err != nil {
			t.Fatalf("listing the page at %d: %v", offset, err)
		}
		return took
	}
	first, far := listPage(0), listPage(20_000/lineupScale)
	for range 4 {
		first = min(first, listPage(0))
		far = min(far, listPage(20_000/lineupScale))
	}
	if far > 10*first {
		t.Errorf("listing a page far down took %v, more than ten times the first page's %v", far.Round(time.Millisecond), first.Round(time.Millisecond))
	} else {
		t.Logf("listing the first page: %v, a page far down: %v", first.Round(time.Millisecond), far.Round(time.Millisecond))
	}
	scaled("searching", 5, also{rows: true}, func(l *lineup, _ int) error {
		_, _, err := e.service.ListChannels(t.Context(), addons.Shared(), l.addon.ID, ChannelFilter{Q: "orb", Limit: 100 / lineupScale})
		return err
	})
	// Each round disables the channels the keyword finds, or enables them
	// again, so that each writes.
	scaled("bulk by keyword", 5, also{rows: true}, func(l *lineup, round int) error {
		_, _, err := e.service.BulkChannels(t.Context(), addons.Shared(), l.addon.ID, Bulk{Q: "tac", Enabled: round%2 == 1})
		return err
	})
	scaled("listing the catalog", 5, also{}, func(l *lineup, _ int) error {
		e.service.forget(l.addon.ID)
		_, err := e.service.Channels(t.Context(), l.addon.ID)
		return err
	})
	scaled("eight parallel pages", 5, also{rows: true}, func(l *lineup, _ int) error {
		var wg sync.WaitGroup
		errs := make([]error, 8)
		for i := range 8 {
			wg.Go(func() {
				_, _, errs[i] = e.service.ListChannels(t.Context(), addons.Shared(), l.addon.ID, ChannelFilter{Offset: i * l.entries / 54, Limit: 100})
			})
		}
		wg.Wait()
		return errors.Join(errs...)
	})
}
