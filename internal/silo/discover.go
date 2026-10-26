package silo

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// Similar titles, For You and the calendar.
//
// Similar titles are those the Jellyfin API gives a title's page. For You
// is Jellyfin's movie recommendations, which Polyfin makes from the movies
// the user played last: a row of the titles similar to each, without those
// played. The calendar lists the coming episodes of the series the user
// watches or marked favorite, as the Jellyfin API's Upcoming lists them to
// Jellyfin apps. Polyfin knows no popular, trending or other recommended
// titles: it offers none.

func (s *Server) discoverRoutes() {
	s.mux.HandleFunc("GET /api/v2/recommendations/similar/{id}", s.profiled(s.similar))
	s.mux.HandleFunc("GET /api/v2/recommendations/discover", s.profiled(s.discover))
	s.mux.HandleFunc("GET /api/v2/calendar", s.profiled(s.calendar))
}

// similar lists the titles similar to a title, as its page in Jellyfin
// apps does: without those the user played, nor those they may not see.
func (s *Server) similar(w http.ResponseWriter, r *http.Request, v viewer) {
	limit := 12
	if text := r.URL.Query().Get("limit"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > 50 {
			validationProblem(w, "query.limit", "out_of_range", "limit is between 1 and 50.")
			return
		}
		limit = value
	}
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items/"+i.Id+"/Similar?"+url.Values{"Limit": {strconv.Itoa(limit)},
		"Fields": {listFields}}.Encode(), nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []catalogItem `json:"items"`
	}{uniqueCards(cards(r, result.Items), limit)})
}

// uniqueCards are cards, each once, at most limit of them.
func uniqueCards(all []catalogItem, limit int) []catalogItem {
	seen := map[string]bool{}
	result := []catalogItem{}
	for _, c := range all {
		if !seen[c.ContentID] && len(result) < limit {
			seen[c.ContentID] = true
			result = append(result, c)
		}
	}
	return result
}

type discoverRow struct {
	Type  string        `json:"type"`
	Title string        `json:"title"`
	Items []catalogItem `json:"items"`
}

// becauseTitles title For You's rows, by the server's language.
var becauseTitles = map[string]string{
	"en":    "Because you watched %s",
	"fr":    "Parce que vous avez regardé %s",
	"de":    "Weil du %s angesehen hast",
	"es":    "Porque viste %s",
	"it":    "Perché hai guardato %s",
	"pt-BR": "Porque você assistiu %s",
}

// discover answers For You: a row of the titles similar to each of the
// movies the user played last, those they played left out, as Jellyfin's
// movie recommendations give them.
func (s *Server) discover(w http.ResponseWriter, r *http.Request, v viewer) {
	var categories []struct {
		Items              []item
		RecommendationType string
		BaselineItemName   string
	}
	a, err := s.call(r, v, http.MethodGet, "/Movies/Recommendations?"+url.Values{"categoryLimit": {"6"}, "itemLimit": {strconv.Itoa(sectionLimit)},
		"Fields": {listFields}}.Encode(), nil, &categories)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	title := cmp.Or(becauseTitles[s.options.Accounts.Settings().Language], becauseTitles["en"])
	rows := []discoverRow{}
	for _, category := range categories {
		items := uniqueCards(sectionCards(r, category.Items), sectionLimit)
		if len(items) > 0 {
			// A row of titles gathered around one the user watched.
			rows = append(rows, discoverRow{Type: "cluster", Title: fmt.Sprintf(title, category.BaselineItemName), Items: items})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Items []discoverRow `json:"items"`
	}{rows})
}

// calendarDays bounds a calendar's window, as the contract does.
const calendarDays = 31

type calendarEvent struct {
	ContentID     string   `json:"content_id"`
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	EpisodeTitle  string   `json:"episode_title,omitempty"`
	SeriesID      string   `json:"series_id,omitempty"`
	SeasonNumber  *int     `json:"season_number,omitempty"`
	EpisodeNumber *int     `json:"episode_number,omitempty"`
	AirDate       string   `json:"air_date"`
	LocalAirDate  string   `json:"local_air_date"`
	PosterURL     string   `json:"poster_url,omitempty"`
	Watched       bool     `json:"watched"`
	Badges        []string `json:"badges"`
}

type calendarDay struct {
	Date  string          `json:"date"`
	Items []calendarEvent `json:"items"`
}

// calendarFilters are the filters of the calendar Polyfin answers: the
// series the user follows, all of them or those of their favorites or
// watchlist. It knows of no others, popular or trending: those list
// nothing.
var calendarFilters = map[string]bool{"": true, "all": true, "everything": true, "following": true, "favorites": true,
	"watchlist": true, "popular": false, "trending": false}

// calendar lists the coming episodes of the series the user follows, by
// day of the viewer's zone, in a window of days: those the Jellyfin API's
// Upcoming lists, aired from yesterday on.
func (s *Server) calendar(w http.ResponseWriter, r *http.Request, v viewer) {
	q := r.URL.Query()
	zone := time.UTC
	if name := q.Get("timezone"); name != "" {
		location, err := time.LoadLocation(name)
		if err != nil || name == "Local" {
			validationProblem(w, "query.timezone", "invalid", "timezone is an IANA zone.")
			return
		}
		zone = location
	}
	start, err := time.ParseInLocation(time.DateOnly, q.Get("start"), zone)
	if err != nil {
		validationProblem(w, "query.start", "invalid", "start is a date, YYYY-MM-DD.")
		return
	}
	end, err := time.ParseInLocation(time.DateOnly, q.Get("end"), zone)
	if err != nil || end.Before(start) || end.After(start.AddDate(0, 0, calendarDays-1)) {
		validationProblem(w, "query.end", "invalid", "end is a date, YYYY-MM-DD, from start to 30 days after it.")
		return
	}
	filter := q.Get("filter")
	listed, known := calendarFilters[filter]
	if !known {
		validationProblem(w, "query.filter", "unsupported", "This filter is not one of the contract's.")
		return
	}
	days := []calendarDay{}
	if !listed {
		writeJSON(w, http.StatusOK, struct {
			Events []calendarDay `json:"events"`
		}{days})
		return
	}
	query := url.Values{"Fields": {listFields}}
	if number := q.Get("library_id"); number != "" {
		view, ok := s.library(w, r, v, number)
		if !ok {
			return
		}
		query.Set("ParentId", view.Id)
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Shows/Upcoming?"+query.Encode(), nil, &result)
	if err == nil && a.ok() && filter == "favorites" {
		result.Items, a, err = s.ofFavoriteSeries(r, v, result.Items)
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	held := watchlisted(r)
	byDay := map[string]int{}
	for _, i := range result.Items {
		if i.Type != "Episode" || i.PremiereDate == nil {
			continue
		}
		if _, ok := held[i.SeriesId]; filter == "watchlist" && !ok {
			continue
		}
		// Episodes have an air date, not a time: each is listed on its
		// date, in any zone.
		aired := date(i.PremiereDate)
		if aired < start.Format(time.DateOnly) || aired > end.Format(time.DateOnly) {
			continue
		}
		event := calendarEvent{ContentID: i.Id, Type: "episode", Title: cmp.Or(i.SeriesName, i.Name), EpisodeTitle: i.Name,
			SeriesID: i.SeriesId, SeasonNumber: i.ParentIndexNumber, EpisodeNumber: i.IndexNumber, AirDate: aired,
			LocalAirDate: aired, PosterURL: artworkURL(i.SeriesId, "Primary", i.SeriesPrimaryImageTag, artworkWidth(r, "poster")),
			Watched: i.UserData != nil && i.UserData.Played, Badges: premiereBadges(i)}
		day, ok := byDay[event.LocalAirDate]
		if !ok {
			day = len(days)
			byDay[event.LocalAirDate] = day
			days = append(days, calendarDay{Date: event.LocalAirDate, Items: []calendarEvent{}})
		}
		days[day].Items = append(days[day].Items, event)
	}
	slices.SortStableFunc(days, func(a, b calendarDay) int { return cmp.Compare(a.Date, b.Date) })
	writeJSON(w, http.StatusOK, struct {
		Events []calendarDay `json:"events"`
	}{days})
}

// premiereBadges are the badges of an episode that opens a series or a
// season.
func premiereBadges(i item) []string {
	switch {
	case i.IndexNumber == nil || *i.IndexNumber != 1 || i.ParentIndexNumber == nil || *i.ParentIndexNumber < 1:
		return []string{}
	case *i.ParentIndexNumber == 1:
		return []string{"series_premiere"}
	}
	return []string{"season_premiere"}
}

// ofFavoriteSeries keeps the episodes of the user's favorite series.
func (s *Server) ofFavoriteSeries(r *http.Request, v viewer, episodes []item) ([]item, answer, error) {
	var favorites queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"Filters": {"IsFavorite"}, "Recursive": {"true"},
		"IncludeItemTypes": {"Series"}}.Encode(), nil, &favorites)
	if err != nil || !a.ok() {
		return nil, a, err
	}
	series := map[string]bool{}
	for _, i := range favorites.Items {
		series[i.Id] = true
	}
	return slices.DeleteFunc(episodes, func(i item) bool { return !series[i.SeriesId] }), a, nil
}
