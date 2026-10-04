package iptv

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

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
	data  *synthetic
	video []byte
}

func newSyntheticServer(data *synthetic, video []byte) *syntheticServer {
	s := &syntheticServer{data: data, video: video}
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
		s.playerAPI(w, query.Get("action"))
	case r.URL.Path == "/xmltv.php" && authorized:
		w.Header().Set("Content-Type", "application/xml")
		s.guide(w)
	case strings.HasPrefix(r.URL.Path, "/live/user/secret/") && s.video != nil:
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write(s.video)
	default:
		http.NotFound(w, r)
	}
}

func (s *syntheticServer) playerAPI(w http.ResponseWriter, action string) {
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

// syntheticVideo makes ten seconds of MPEG-TS video with the ffmpeg of
// POLYFIN_TEST_FFMPEG, nil without one.
func syntheticVideo(ctx context.Context) []byte {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		return nil
	}
	video, err := exec.CommandContext(ctx, ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440", "-t", "10", "-c:v", "libx264", "-preset", "veryfast", "-c:a", "aac",
		"-f", "mpegts", "pipe:1").Output()
	if err != nil {
		return nil
	}
	return video
}

// TestServeSyntheticXtream serves a synthetic Xtream Codes provider until
// interrupted, for trying Polyfin by hand with a large line-up. It only runs
// with POLYFIN_SYNTHETIC_XTREAM set: to a number of channels (default
// 54000), or to "1" for the default. POLYFIN_SYNTHETIC_XTREAM_ADDR sets the
// address it listens on (default 127.0.0.1:0).
//
//	POLYFIN_SYNTHETIC_XTREAM=1 go test -run TestServeSyntheticXtream -v -timeout 0 ./internal/iptv
func TestServeSyntheticXtream(t *testing.T) {
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
	server := newSyntheticServer(newSynthetic(channels, max(channels/130, 1)), syntheticVideo(t.Context()))
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
	fmt.Printf("Synthetic Xtream provider: %d channels in %d groups, video %v\n", len(server.data.streams), len(server.data.groups), server.video != nil)
	fmt.Printf("  server   %s\n  username user\n  password secret\n  guide    %s/xmltv.php?username=user&password=secret\n",
		server.URL, server.URL)
	fmt.Println("Interrupt (Ctrl-C) to stop.")
	<-t.Context().Done()
}

// A line-up of about 54,000 entries in hundreds of groups is imported,
// merged, refreshed, listed and searched within bounds: reconciling is
// batched and listing pages and searches read the database once.
func TestLargeLineupsStayFast(t *testing.T) {
	if testing.Short() {
		t.Skip("large line-up")
	}
	e := newEnv(t)
	server := newSyntheticServer(newSynthetic(54_000, 420), nil)
	server.Start()
	t.Cleanup(server.Close)
	account := Account{Kind: addons.KindXtream, Server: server.URL, Username: "user", Password: "secret"}
	bound := func(what string, limit time.Duration, run func() error) {
		t.Helper()
		start := time.Now()
		if err := run(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		limit *= raceSlowdown
		if took := time.Since(start); took > limit {
			t.Errorf("%s took %v, more than %v", what, took.Round(time.Millisecond), limit)
		} else {
			t.Logf("%s: %v", what, took.Round(time.Millisecond))
		}
	}
	var addon addons.Addon
	bound("adding", 12*time.Second, func() (err error) {
		addon, err = e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Big", Account: account}, false)
		return err
	})
	source, err := e.service.Source(t.Context(), addons.Shared(), addon.ID)
	if err != nil || source.Lineup.Channels != 54_000 || source.Lineup.Categories != 420 {
		t.Fatalf("line-up: %+v %v", source.Lineup, err)
	}
	// An edit survives the refreshes and option changes below.
	_, channels, err := e.service.ListChannels(t.Context(), addons.Shared(), addon.ID, ChannelFilter{Q: "Zebzeb 0", Limit: 1})
	if err != nil || len(channels) != 1 {
		t.Fatalf("search: %+v %v", channels, err)
	}
	if _, err := e.service.UpdateChannel(t.Context(), addons.Shared(), addon.ID, channels[0].ID, ChannelChanges{Name: &Nullable[string]{Value: new("Kept")}}); err != nil {
		t.Fatal(err)
	}
	bound("refreshing unchanged", 12*time.Second, func() error { return e.service.Refresh(t.Context(), addons.Shared(), addon.ID, false) })
	merged := ChannelsMerged
	bound("merging", 15*time.Second, func() error {
		return e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Options: &OptionsPatch{Channels: &merged}}, false)
	})
	byCountry := CategoriesCountry
	bound("by country", 15*time.Second, func() error {
		return e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Options: &OptionsPatch{Categories: &byCountry}}, false)
	})
	if kept, err := e.service.Channel(t.Context(), addons.Shared(), addon.ID, channels[0].ID); err != nil || kept.Name != "Kept" || len(kept.Streams) != 3 {
		t.Errorf("the edited channel after merging: %+v %v", kept, err)
	}
	bound("listing a page far down", 1*time.Second, func() error {
		total, page, err := e.service.ListChannels(t.Context(), addons.Shared(), addon.ID, ChannelFilter{Offset: 20_000, Limit: 500})
		if err == nil && (len(page) != 500 || total < 20_500) {
			err = fmt.Errorf("%d of %d", len(page), total)
		}
		return err
	})
	bound("searching", 1*time.Second, func() error {
		_, _, err := e.service.ListChannels(t.Context(), addons.Shared(), addon.ID, ChannelFilter{Q: "orb", Limit: 100})
		return err
	})
	bound("bulk by keyword", 2*time.Second, func() error {
		_, _, err := e.service.BulkChannels(t.Context(), addons.Shared(), addon.ID, Bulk{Q: "tac", Enabled: false})
		return err
	})
	bound("listing the catalog", 2*time.Second, func() error {
		e.service.forget(addon.ID)
		_, err := e.service.Channels(t.Context(), addon.ID)
		return err
	})
	var wg sync.WaitGroup
	bound("eight parallel pages", 2*time.Second, func() error {
		errs := make([]error, 8)
		for i := range 8 {
			wg.Go(func() {
				_, _, errs[i] = e.service.ListChannels(t.Context(), addons.Shared(), addon.ID, ChannelFilter{Offset: i * 1000, Limit: 100})
			})
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return err
			}
		}
		return nil
	})
}
