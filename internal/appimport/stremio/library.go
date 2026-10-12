package stremio

import (
	"cmp"
	"context"
	"encoding/json"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/appimport"
	addon "github.com/moodiness/polyfin/internal/stremio"
)

// metaFetches bounds the series described at a time.
const metaFetches = 4

// number is a number of the account API, which reads null, and anything
// that is not a number, as zero.
type number int64

func (n *number) UnmarshalJSON(data []byte) error {
	*n = 0
	var f float64
	if json.Unmarshal(data, &f) == nil {
		*n = number(f)
	}
	return nil
}

// date is a date of the account API, in RFC 3339, which reads null, and
// anything else, as zero.
type date struct{ time.Time }

func (d *date) UnmarshalJSON(data []byte) error {
	*d = date{}
	var text string
	if json.Unmarshal(data, &text) == nil {
		if at, err := time.Parse(time.RFC3339Nano, text); err == nil {
			d.Time = at.UTC()
		}
	}
	return nil
}

// item is a title of the account's library, as Stremio's apps keep it.
// Removed is false for a title in the user's library; Temp is set for one
// played without being added to it: its history, and Continue Watching.
// Created is when Stremio first kept the title.
type item struct {
	ID      string `json:"_id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Removed bool   `json:"removed"`
	Temp    bool   `json:"temp"`
	Created date   `json:"_ctime"`
	Changed date   `json:"_mtime"`
	State   struct {
		LastWatched date `json:"lastWatched"`
		// TimeOffset is the resume point in the video played last,
		// VideoID, of Duration; both in milliseconds.
		TimeOffset number `json:"timeOffset"`
		Duration   number `json:"duration"`
		// TimesWatched counts the plays of a movie; FlaggedWatched is set
		// once most of it was watched.
		TimesWatched   number `json:"timesWatched"`
		FlaggedWatched number `json:"flaggedWatched"`
		VideoID        string `json:"video_id"`
		// Watched tells the episodes of a series watched (see watched).
		Watched string `json:"watched"`
	} `json:"state"`
}

// items reads the library items Polyfin imports, movies and series, in
// Stremio's order, the latest changed of each title only. Stremio's apps
// leave out the items they do not read; so does Polyfin.
func items(raw []json.RawMessage) []item {
	var result []item
	index := map[string]int{}
	for _, r := range raw {
		var it item
		if json.Unmarshal(r, &it) != nil {
			continue
		}
		it.ID = strings.TrimSpace(it.ID)
		// Channels, TV and other videos are not titles Polyfin imports.
		if it.ID == "" || it.Type != "movie" && it.Type != "series" {
			continue
		}
		if i, seen := index[it.ID]; seen {
			if it.Changed.After(result[i].Changed.Time) {
				result[i] = it
			}
			continue
		}
		index[it.ID] = len(result)
		result = append(result, it)
	}
	return result
}

// key names an entry: a title, whose season and episode are -1, or an
// episode of it.
type key struct {
	id              string
	season, episode int
}

// builder gathers the entries of a profile, one per title, in order.
type builder struct {
	r       *Reader
	entries map[key]*appimport.Entry
	order   []key
}

// entry returns the entry of the title id, a "movie" or "series" by kind,
// or of its episode for kind "episode", made when missing.
func (b *builder) entry(id, name, kind string, season, episode int) *appimport.Entry {
	k := key{id: id, season: -1, episode: -1}
	if kind == "episode" {
		k.season, k.episode = season, episode
	}
	if e := b.entries[k]; e != nil {
		return e
	}
	e := &appimport.Entry{Label: appimport.Label{ID: id, Name: name, Kind: kind}}
	if kind == "episode" {
		e.Label.Season, e.Label.Episode = season, episode
	}
	e.Title, _ = appimport.TitleRef(b.r.anime, id, kind == "movie", max(k.season, 0), max(k.episode, 0))
	b.entries[k] = e
	b.order = append(b.order, k)
	return e
}

// account reads the library items and the addon descriptors of an account
// into its one profile:
//   - a movie watched is played, at the date it was last watched, as many
//     times as it was;
//   - a series' episodes watched are played, at the date the series was
//     last watched, when Polyfin's addons list its episodes as Stremio's
//     apps did (see watched); else the series is reported;
//   - the video played last of a title in Continue Watching, the movie or
//     an episode, has its resume point;
//   - the titles of the user's library go on the watchlist, from the date
//     Stremio first kept them;
//   - the addons the user installed, Stremio's own left out, are offered.
func (r *Reader) account(ctx context.Context, raw, descriptors []json.RawMessage) appimport.Account {
	list := items(raw)
	lists := r.episodeLists(ctx, list)
	b := &builder{r: r, entries: map[key]*appimport.Entry{}}
	for _, it := range list {
		name := strings.TrimSpace(it.Name)
		listed := !it.Removed && !it.Temp
		var addedAt *time.Time
		if listed && !it.Created.IsZero() {
			addedAt = &it.Created.Time
		}
		at := it.State.LastWatched.Time
		if at.IsZero() {
			at = it.Changed.Time
		}
		var playedAt *time.Time
		if !at.IsZero() {
			playedAt = &at
		}
		var resume *appimport.Resume
		if it.State.TimeOffset > 0 && (!it.Removed || it.Temp) && !at.IsZero() {
			resume = &appimport.Resume{Position: time.Duration(it.State.TimeOffset) * time.Millisecond,
				Runtime: time.Duration(max(it.State.Duration, 0)) * time.Millisecond, At: at}
		}

		if it.Type == "movie" {
			played := it.State.TimesWatched > 0 || it.State.FlaggedWatched > 0
			if !played && resume == nil && !listed {
				continue
			}
			e := b.entry(it.ID, name, "movie", 0, 0)
			e.Watchlist, e.AddedAt, e.Resume = listed, addedAt, resume
			if played {
				e.Played, e.PlayedAt, e.Plays = true, playedAt, int(max(it.State.TimesWatched, 0))
			}
			continue
		}

		field, watchedSome := parseWatched(it.State.Watched)
		watchedSome = watchedSome && field.any()
		if _, identified := appimport.TitleRef(r.anime, it.ID, false, 0, 0); !identified {
			// Polyfin finds no title by this identifier: one entry tells it,
			// whatever the profile holds of the series.
			if listed || watchedSome || resume != nil {
				e := b.entry(it.ID, name, "series", 0, 0)
				e.Watchlist, e.AddedAt = listed, addedAt
			}
			continue
		}
		if listed {
			e := b.entry(it.ID, name, "series", 0, 0)
			e.Watchlist, e.AddedAt = true, addedAt
		}
		videos := lists[it.ID]
		if watchedSome {
			places, ok := field.places(videoIDs(videos))
			if !ok {
				b.entry(it.ID, name, "series", 0, 0).Problem = appimport.ReasonEpisodeList
			}
			for _, i := range places {
				if season, episode, ok := place(it.ID, videos[i]); ok {
					e := b.entry(it.ID, name, "episode", season, episode)
					e.Played, e.PlayedAt = true, playedAt
				}
			}
		}
		if resume != nil {
			if season, episode, ok := resumePlace(it.ID, it.State.VideoID, videos); ok {
				b.entry(it.ID, name, "episode", season, episode).Resume = resume
			}
		}
	}

	profile := appimport.Profile{ID: "1", Entries: make([]appimport.Entry, 0, len(b.order)), Addons: addons(descriptors)}
	for _, k := range b.order {
		profile.Entries = append(profile.Entries, *b.entries[k])
	}
	return appimport.Account{App: App, Profiles: []appimport.Profile{profile}}
}

// episodeLists returns the episodes of the series of list Polyfin needs,
// as the server's addons list them, in the order Stremio's apps place them
// in (see ordered): those of the series with episodes watched, and of
// those whose resume point's video its identifier does not place. A series
// no addon describes is left out.
func (r *Reader) episodeLists(ctx context.Context, list []item) map[string][]addon.Video {
	var wanted []string
	for _, it := range list {
		if it.Type != "series" || r.metas == nil {
			continue
		}
		if _, identified := appimport.TitleRef(r.anime, it.ID, false, 0, 0); !identified {
			continue
		}
		field, ok := parseWatched(it.State.Watched)
		_, _, placed := placeOf(it.ID, it.State.VideoID)
		if ok && field.any() || it.State.TimeOffset > 0 && it.State.VideoID != "" && !placed {
			wanted = append(wanted, it.ID)
		}
	}
	found := make([][]addon.Video, len(wanted))
	described := make([]bool, len(wanted))
	var group errgroup.Group
	group.SetLimit(metaFetches)
	for i, id := range wanted {
		group.Go(func() error {
			if meta, ok := r.metas.TitleMeta(ctx, "series", id); ok {
				found[i], described[i] = ordered(meta.Videos), true
			}
			return nil
		})
	}
	_ = group.Wait()
	lists := make(map[string][]addon.Video, len(wanted))
	for i, id := range wanted {
		if described[i] {
			lists[id] = found[i]
		}
	}
	return lists
}

// ordered lists a series' videos in the order Stremio's apps tell their
// places in: each identifier once, the first kept, stably sorted by
// season, episode and release date, where a missing value comes first.
// Polyfin reads a missing season and episode as zero: a video without
// either, or numbered season 0 episode 0, has none.
func ordered(videos []addon.Video) []addon.Video {
	seen := make(map[string]bool, len(videos))
	list := make([]addon.Video, 0, len(videos))
	for _, v := range videos {
		if !seen[v.ID] {
			seen[v.ID] = true
			list = append(list, v)
		}
	}
	sortKey := func(v addon.Video) [3]int64 {
		k := [3]int64{math.MinInt64, math.MinInt64, math.MinInt64}
		if v.Season != 0 || v.Episode != 0 {
			k[0], k[1] = int64(v.Season), int64(v.Episode)
		}
		if released, err := time.Parse(time.RFC3339Nano, v.Released); err == nil {
			k[2] = released.UnixMilli()
		}
		return k
	}
	slices.SortStableFunc(list, func(a, b addon.Video) int {
		ka, kb := sortKey(a), sortKey(b)
		return cmp.Or(cmp.Compare(ka[0], kb[0]), cmp.Compare(ka[1], kb[1]), cmp.Compare(ka[2], kb[2]))
	})
	return list
}

func videoIDs(videos []addon.Video) []string {
	ids := make([]string, len(videos))
	for i, v := range videos {
		ids[i] = v.ID
	}
	return ids
}

// place returns the season and episode numbers of a video of the series
// id: as the addon numbers it, else as its identifier does, else by the
// number some addons give.
func place(id string, v addon.Video) (season, episode int, ok bool) {
	if v.Episode > 0 && v.Season >= 0 {
		return int(v.Season), int(v.Episode), true
	}
	if season, episode, ok := placeOf(id, v.ID); ok {
		return season, episode, true
	}
	if v.Number > 0 && v.Season >= 0 {
		return int(v.Season), int(v.Number), true
	}
	return 0, 0, false
}

// placeOf reads the season and episode numbers the identifier of a video
// of the series id gives: "<id>:<season>:<episode>", or "<id>:<episode>"
// as anime addons number episodes in their entry, season 1 then.
func placeOf(id, video string) (season, episode int, ok bool) {
	rest, found := strings.CutPrefix(video, id+":")
	if !found || id == "" {
		return 0, 0, false
	}
	parts := strings.Split(rest, ":")
	switch len(parts) {
	case 1:
		parts = []string{"1", parts[0]}
	case 2:
	default:
		return 0, 0, false
	}
	season, errS := strconv.Atoi(parts[0])
	episode, errE := strconv.Atoi(parts[1])
	if errS != nil || errE != nil || season < 0 || episode <= 0 {
		return 0, 0, false
	}
	return season, episode, true
}

// resumePlace returns the season and episode numbers of video, the video
// of the series id played last: as the series' videos number it, else as
// its identifier does.
func resumePlace(id, video string, videos []addon.Video) (season, episode int, ok bool) {
	if i := slices.IndexFunc(videos, func(v addon.Video) bool { return v.ID == video }); i >= 0 && video != "" {
		if season, episode, ok := place(id, videos[i]); ok {
			return season, episode, true
		}
	}
	return placeOf(id, video)
}

// descriptor is an addon the account installed: its manifest, the address
// of the manifest, and whether it is one of Stremio's own.
type descriptor struct {
	Manifest struct {
		Name string `json:"name"`
	} `json:"manifest"`
	TransportURL string `json:"transportUrl"`
	Flags        struct {
		Official bool `json:"official"`
	} `json:"flags"`
}

// addons reads the addons the user installed, each once, Stremio's own
// left out: they come with Stremio.
func addons(raw []json.RawMessage) []appimport.Addon {
	var result []appimport.Addon
	for _, r := range raw {
		var d descriptor
		if json.Unmarshal(r, &d) != nil || d.Flags.Official {
			continue
		}
		manifest := strings.TrimSpace(d.TransportURL)
		parsed, err := url.Parse(manifest)
		if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" {
			continue
		}
		if slices.ContainsFunc(result, func(a appimport.Addon) bool { return a.ManifestURL == manifest }) {
			continue
		}
		name := strings.TrimSpace(d.Manifest.Name)
		if name == "" {
			name = parsed.Host
		}
		result = append(result, appimport.Addon{Name: name, ManifestURL: manifest})
	}
	return result
}
