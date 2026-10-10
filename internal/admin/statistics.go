package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/plays"
)

// The periods statistics and history cover, by the name the API takes:
// the last 7 or 30 days, the last year, or all the history.
var statisticsPeriods = map[string]int{"7d": 7, "30d": 30, "year": 365, "all": 0}

const (
	defaultStatisticsPeriod = "30d"
	defaultHistoryLimit     = 20
	maxHistoryLimit         = 100
)

// statisticsJSON sums the playbacks of a period: since when (null for
// all), and the unit of its buckets, "day", or "month" for all the
// history. Times are played times in seconds, pauses left out. Hours are
// the 168 hours of the week, from Monday 0:00, by the time zone asked.
// HistoryEnabled and HistoryDays tell whether the history is kept, and
// for how many days.
type statisticsJSON struct {
	Period         string           `json:"period"`
	Since          *time.Time       `json:"since"`
	Unit           string           `json:"unit"`
	Plays          int              `json:"plays"`
	Played         int64            `json:"played"`
	Users          []statUserJSON   `json:"users"`
	Buckets        []statBucketJSON `json:"buckets"`
	Movies         []statRankedJSON `json:"movies"`
	Series         []statRankedJSON `json:"series"`
	Channels       []statRankedJSON `json:"channels"`
	Apps           []statGroupJSON  `json:"apps"`
	Devices        []statDeviceJSON `json:"devices"`
	Methods        []statMethodJSON `json:"methods"`
	Hours          []int64          `json:"hours"`
	HistoryEnabled bool             `json:"historyEnabled"`
	HistoryDays    int              `json:"historyDays"`
}

type statUserJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Plays  int    `json:"plays"`
	Played int64  `json:"played"`
}

// statBucketJSON is a day, or a month from its first day, as YYYY-MM-DD,
// with what each user played in it.
type statBucketJSON struct {
	Start  string             `json:"start"`
	Played int64              `json:"played"`
	Users  []statUserTimeJSON `json:"users"`
}

type statUserTimeJSON struct {
	ID     string `json:"id"`
	Played int64  `json:"played"`
}

// statRankedJSON is a movie, a series or a channel, by its latest name,
// with how many times and by how many users it played.
type statRankedJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Plays  int    `json:"plays"`
	Users  int    `json:"users"`
	Played int64  `json:"played"`
}

type statGroupJSON struct {
	Name   string `json:"name"`
	Plays  int    `json:"plays"`
	Played int64  `json:"played"`
}

type statDeviceJSON struct {
	Name   string `json:"name"`
	App    string `json:"app"`
	Plays  int    `json:"plays"`
	Played int64  `json:"played"`
}

// statMethodJSON is a play method: "direct_play", "direct_stream",
// "conversion", or "" when no report told.
type statMethodJSON struct {
	Method string `json:"method"`
	Plays  int    `json:"plays"`
	Played int64  `json:"played"`
}

// historyEntryJSON is a playback of the history.
type historyEntryJSON struct {
	ID        int64           `json:"id"`
	User      historyUserJSON `json:"user"`
	Item      historyItemJSON `json:"item"`
	App       string          `json:"app"`
	Device    string          `json:"device"`
	StartedAt time.Time       `json:"startedAt"`
	EndedAt   time.Time       `json:"endedAt"`
	Played    int64           `json:"played"`
	Position  int64           `json:"position"`
	Method    string          `json:"method"`
}

type historyUserJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// historyItemJSON is what played, as it was called then: series, season
// and episode are set for an episode, channelName for a channel, a Replay
// programme or a recording.
type historyItemJSON struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	SeriesName  *string `json:"seriesName"`
	Season      *int    `json:"season"`
	Episode     *int    `json:"episode"`
	ChannelName *string `json:"channelName"`
}

// statisticsScope is whose playbacks a route covers: nil for everyone,
// which only administrators reach, else the signed-in user.
type statisticsScope func(r *http.Request) *accounts.ID

func ownPlaybacks(r *http.Request) *accounts.ID {
	return new(sessionFrom(r.Context()).User.ID)
}

// statisticsRoutes answers the statistics and history of one scope.
type statisticsRoutes struct {
	h     *handler
	scope statisticsScope
}

// filter reads the period and, for administrators, the user a request
// asks for, answering 400 and false when either is invalid.
func (s statisticsRoutes) filter(w http.ResponseWriter, r *http.Request) (plays.Filter, string, bool) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = defaultStatisticsPeriod
	}
	days, ok := statisticsPeriods[period]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_period")
		return plays.Filter{}, "", false
	}
	f := plays.Filter{User: s.scope(r)}
	if days > 0 {
		f.Since = s.h.now().AddDate(0, 0, -days)
	}
	if raw := r.URL.Query().Get("user"); raw != "" && f.User == nil {
		id, err := accounts.ParseID(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_user")
			return plays.Filter{}, "", false
		}
		f.User = &id
	}
	return f, period, true
}

// statistics sums the playbacks of a period, by the time zone a request
// names (timeZone, an IANA name, UTC by default).
func (s statisticsRoutes) statistics(w http.ResponseWriter, r *http.Request) {
	if s.h.Plays == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	f, period, ok := s.filter(w, r)
	if !ok {
		return
	}
	zone := time.UTC
	if name := r.URL.Query().Get("timeZone"); name != "" {
		var err error
		if zone, err = time.LoadLocation(name); err != nil || name == "Local" {
			writeError(w, http.StatusBadRequest, "invalid_time_zone")
			return
		}
	}
	unit := plays.Day
	if period == "all" {
		unit = plays.Month
	}
	st, err := s.h.Plays.Statistics(r.Context(), f, unit, zone)
	if err != nil {
		s.h.internalError(w, r, err)
		return
	}
	settings := s.h.Accounts.Settings()
	result := statisticsJSON{Period: period, Unit: unit, Plays: st.Plays, Played: secondsOf(st.Played), Users: []statUserJSON{},
		Buckets: []statBucketJSON{}, Movies: ranked(st.Movies), Series: ranked(st.Series), Channels: ranked(st.Channels),
		Apps: []statGroupJSON{}, Devices: []statDeviceJSON{}, Methods: []statMethodJSON{}, Hours: make([]int64, len(st.Hours)),
		HistoryEnabled: settings.PlaybackHistory, HistoryDays: settings.PlaybackHistoryDays}
	if !f.Since.IsZero() {
		result.Since = new(f.Since.UTC().Truncate(time.Second))
	}
	for _, u := range st.Users {
		result.Users = append(result.Users, statUserJSON{ID: u.User.String(), Name: u.Name, Plays: u.Plays, Played: secondsOf(u.Played)})
	}
	for _, b := range st.Buckets {
		bucket := statBucketJSON{Start: b.Start, Played: secondsOf(b.Played), Users: []statUserTimeJSON{}}
		// In the order of the users, the one who played longest first.
		for _, u := range st.Users {
			if played, ok := b.Users[u.User]; ok {
				bucket.Users = append(bucket.Users, statUserTimeJSON{ID: u.User.String(), Played: secondsOf(played)})
			}
		}
		result.Buckets = append(result.Buckets, bucket)
	}
	for _, g := range st.Apps {
		result.Apps = append(result.Apps, statGroupJSON{Name: g.Name, Plays: g.Plays, Played: secondsOf(g.Played)})
	}
	for _, g := range st.Devices {
		result.Devices = append(result.Devices, statDeviceJSON{Name: g.Name, App: g.App, Plays: g.Plays, Played: secondsOf(g.Played)})
	}
	for _, g := range st.Methods {
		result.Methods = append(result.Methods, statMethodJSON{Method: g.Name, Plays: g.Plays, Played: secondsOf(g.Played)})
	}
	for i, played := range st.Hours {
		result.Hours[i] = secondsOf(played)
	}
	writeJSON(w, http.StatusOK, result)
}

func ranked(list []plays.Ranked) []statRankedJSON {
	result := make([]statRankedJSON, 0, len(list))
	for _, r := range list {
		result = append(result, statRankedJSON{ID: r.ID.String(), Name: r.Name, Plays: r.Plays, Users: r.Users, Played: secondsOf(r.Played)})
	}
	return result
}

func secondsOf(d time.Duration) int64 {
	return int64(d.Round(time.Second) / time.Second)
}

// history lists the playbacks of a period, the latest first: limit of
// them (1 to 100, 20 by default) from start.
func (s statisticsRoutes) history(w http.ResponseWriter, r *http.Request) {
	if s.h.Plays == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	f, _, ok := s.filter(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	limit, start := defaultHistoryLimit, 0
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxHistoryLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = value
	}
	if raw := query.Get("start"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		start = value
	}
	entries, total, err := s.h.Plays.Entries(r.Context(), f, start, limit)
	if err != nil {
		s.h.internalError(w, r, err)
		return
	}
	items := make([]historyEntryJSON, 0, len(entries))
	for _, e := range entries {
		item := historyItemJSON{ID: e.Title.Item.String(), Kind: e.Title.Kind, Name: e.Title.Name}
		if e.Title.Kind == plays.Episode {
			item.SeriesName, item.Season, item.Episode = &e.Title.SeriesName, &e.Title.Season, &e.Title.Episode
		}
		if e.Title.ChannelName != "" {
			item.ChannelName = &e.Title.ChannelName
		}
		items = append(items, historyEntryJSON{ID: e.ID, User: historyUserJSON{ID: e.User.String(), Name: e.UserName}, Item: item, App: e.App,
			Device: e.Device, StartedAt: e.Started.UTC(), EndedAt: e.Ended.UTC(), Played: secondsOf(e.Played),
			Position: secondsOf(e.Position), Method: e.Method})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// export answers the playbacks of a period as CSV, the latest first.
func (s statisticsRoutes) export(w http.ResponseWriter, r *http.Request) {
	if s.h.Plays == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	f, _, ok := s.filter(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="playback-history.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	out := &trackedWriter{ResponseWriter: w}
	if err := s.h.Plays.WriteCSV(r.Context(), out, f); err != nil {
		if !out.written {
			w.Header().Del("Content-Disposition")
			s.h.internalError(w, r, err)
			return
		}
		s.h.Logger.Warn("The playback history could not be exported", "error", err)
	}
}

// trackedWriter tells whether anything was written.
type trackedWriter struct {
	http.ResponseWriter
	written bool
}

func (t *trackedWriter) Write(p []byte) (int, error) {
	t.written = true
	return t.ResponseWriter.Write(p)
}
