package silo

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func (s *Server) browseRoutes() {
	s.mux.HandleFunc("GET /api/v2/user/libraries", s.signedIn(s.libraries))
	s.mux.HandleFunc("GET /api/v2/home/sections", s.profiled(s.homeSections))
	s.mux.HandleFunc("GET /api/v2/home/sections/{id}/items", s.profiled(s.homeSection))
	s.mux.HandleFunc("GET /api/v2/catalog", s.profiled(s.catalog))
	s.mux.HandleFunc("GET /api/v2/catalog/search/capabilities", s.profiled(s.searchCapabilities))
	s.mux.HandleFunc("GET /api/v2/catalog/items/{id}", s.profiled(s.catalogItem))
	s.mux.HandleFunc("GET /api/v2/catalog/items/{id}/versions", s.profiled(s.catalogVersions))
	s.mux.HandleFunc("GET /api/v2/catalog/items/{id}/episodes", s.profiled(s.itemEpisodes))
	s.mux.HandleFunc("GET /api/v2/catalog/series/{id}/seasons", s.profiled(s.seasons))
	s.mux.HandleFunc("GET /api/v2/catalog/series/{id}/seasons/{num}/episodes", s.profiled(s.seasonEpisodes))
	s.mux.HandleFunc("GET /api/v2/watch/{id}", s.profiled(s.watch))
	s.mux.HandleFunc("POST /api/v2/watched/{id}", s.profiled(s.watched(true)))
	s.mux.HandleFunc("DELETE /api/v2/watched/{id}", s.profiled(s.watched(false)))
	// Jellyfin apps load artwork without credentials, and so do Silo's.
	s.mux.HandleFunc("GET /api/v2/artwork/{id}/{kind}", s.artwork)
}

// The fields the descriptions of the Jellyfin API are asked with.
const (
	listFields   = "Overview,Genres,DateCreated,ProviderIds,Studios,ChildCount,RecursiveItemCount"
	detailFields = listFields + ",People,Taglines,OriginalTitle,MediaSources,MediaStreams"
)

// pageInfo tells whether more follows, and where from.
type pageInfo struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type catalogItem struct {
	ContentID         string     `json:"content_id"`
	Title             string     `json:"title"`
	Type              string     `json:"type"`
	Status            string     `json:"status"`
	Genres            []string   `json:"genres"`
	Keywords          []string   `json:"keywords"`
	Year              int        `json:"year,omitempty"`
	ReleaseDate       string     `json:"release_date,omitempty"`
	AddedAt           string     `json:"added_at,omitempty"`
	Overview          string     `json:"overview,omitempty"`
	ContentRating     string     `json:"content_rating,omitempty"`
	DurationSeconds   float64    `json:"duration_seconds,omitempty"`
	Runtime           int        `json:"runtime,omitempty"`
	Studios           []string   `json:"studios,omitempty"`
	PosterURL         string     `json:"poster_url,omitempty"`
	BackdropURL       string     `json:"backdrop_url,omitempty"`
	LogoURL           string     `json:"logo_url,omitempty"`
	SeriesID          string     `json:"series_id,omitempty"`
	SeriesTitle       string     `json:"series_title,omitempty"`
	SeasonNumber      *int       `json:"season_number,omitempty"`
	EpisodeNumber     *int       `json:"episode_number,omitempty"`
	ShowStatus        string     `json:"show_status,omitempty"`
	ItemSource        string     `json:"item_source,omitempty"`
	PositionSeconds   float64    `json:"position_seconds,omitempty"`
	ProgressUpdatedAt string     `json:"progress_updated_at,omitempty"`
	UserState         *userState `json:"user_state,omitempty"`
	PosterIsStill     *bool      `json:"poster_is_episode_still,omitempty"`
	BackdropIsStill   *bool      `json:"backdrop_is_episode_still,omitempty"`
}

type userState struct {
	InWatchlist   bool `json:"in_watchlist"`
	IsFavorite    bool `json:"is_favorite"`
	NotInterested bool `json:"not_interested"`
	Played        bool `json:"played"`
}

// kinds are the types of the API, by the Jellyfin API's types.
var kinds = map[string]string{"Movie": "movie", "Series": "series", "Season": "season", "Episode": "episode", "Video": "movie"}

// artworkWidths are the widths of the image sizes the apps may ask.
var artworkWidths = map[string]int{"small": 240, "medium": 480, "large": 960}

// artworkWidth is the width the request asks for a slot (poster,
// backdrop, logo, still), 0 for the original.
func artworkWidth(r *http.Request, slot string) int {
	if width, err := strconv.Atoi(r.URL.Query().Get(slot + "_width")); err == nil && width > 0 && width <= 7680 {
		return width
	}
	if width, ok := artworkWidths[r.URL.Query().Get("image_size")]; ok {
		return width
	}
	return 0
}

// artworkURL is the address of an item's image of a kind, as Jellyfin's
// tag identifies it; empty without one.
func artworkURL(id, kind, tag string, width int) string {
	if id == "" || tag == "" {
		return ""
	}
	query := url.Values{"tag": {tag}}
	if width > 0 {
		query.Set("width", strconv.Itoa(width))
	}
	return "/api/v2/artwork/" + url.PathEscape(id) + "/" + kind + "?" + query.Encode()
}

// artworkKinds are the Jellyfin image types the artwork route serves.
var artworkKinds = []string{"Primary", "Backdrop", "Logo", "Thumb"}

// artwork serves an item's image, as the Jellyfin API serves it to
// Jellyfin apps.
func (s *Server) artwork(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	id, err := accounts.ParseID(r.PathValue("id"))
	if err != nil || !slices.Contains(artworkKinds, kind) {
		problem(w, http.StatusNotFound, "not_found", "No such artwork.")
		return
	}
	query := url.Values{}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		query.Set("tag", tag)
	}
	if width, err := strconv.Atoi(r.URL.Query().Get("width")); err == nil && width > 0 && width <= 7680 {
		query.Set("maxWidth", strconv.Itoa(width))
	}
	s.relay(w, r, "/Items/"+id.String()+"/Images/"+kind+"?"+query.Encode(), "")
}

func date(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

func seconds(ticks *int64) float64 {
	if ticks == nil {
		return 0
	}
	return float64(*ticks) / 1e7
}

// catalogItemOf describes an item of the Jellyfin API as a card.
func catalogItemOf(r *http.Request, i item) catalogItem {
	c := catalogItem{ContentID: i.Id, Title: i.Name, Type: cmp.Or(kinds[i.Type], strings.ToLower(i.Type)), Status: "matched",
		Genres: []string{}, Keywords: []string{}, ContentRating: i.OfficialRating, ReleaseDate: date(i.PremiereDate),
		SeriesID: i.SeriesId, SeriesTitle: i.SeriesName, ShowStatus: i.Status}
	c.Genres = append(c.Genres, i.Genres...)
	if i.ProductionYear != nil {
		c.Year = *i.ProductionYear
	}
	if i.DateCreated != nil {
		c.AddedAt = instant(*i.DateCreated)
	}
	if i.Overview != nil {
		c.Overview = *i.Overview
	}
	if duration := seconds(i.RunTimeTicks); duration > 0 {
		c.DurationSeconds, c.Runtime = duration, int(duration/60+0.5)
	}
	for _, studio := range i.Studios {
		c.Studios = append(c.Studios, studio.Name)
	}
	switch i.Type {
	case "Episode":
		c.SeasonNumber, c.EpisodeNumber = i.ParentIndexNumber, i.IndexNumber
	case "Season":
		c.SeasonNumber = i.IndexNumber
	}
	c.PosterURL, c.BackdropURL, c.LogoURL = artworkOf(r, i)
	if i.Type == "Episode" && c.PosterURL != "" {
		c.PosterIsStill = new(true)
	}
	if i.UserData != nil {
		c.UserState = &userState{IsFavorite: i.UserData.IsFavorite, Played: i.UserData.Played}
		if i.UserData.PlaybackPositionTicks > 0 {
			c.PositionSeconds = float64(i.UserData.PlaybackPositionTicks) / 1e7
		}
		if i.UserData.LastPlayedDate != nil {
			c.ProgressUpdatedAt = instant(*i.UserData.LastPlayedDate)
		}
	}
	return c
}

// artworkOf are the poster, backdrop and logo of an item: an episode's
// poster is its still, a season's or episode's backdrop and logo its
// series'.
func artworkOf(r *http.Request, i item) (poster, backdrop, logo string) {
	poster = artworkURL(i.Id, "Primary", i.ImageTags["Primary"], artworkWidth(r, "poster"))
	if poster == "" && i.Type == "Season" {
		poster = artworkURL(i.SeriesId, "Primary", i.SeriesPrimaryImageTag, artworkWidth(r, "poster"))
	}
	if len(i.BackdropImageTags) > 0 {
		backdrop = artworkURL(i.Id, "Backdrop", i.BackdropImageTags[0], artworkWidth(r, "backdrop"))
	} else if len(i.ParentBackdropImageTags) > 0 {
		backdrop = artworkURL(i.ParentBackdropItemId, "Backdrop", i.ParentBackdropImageTags[0], artworkWidth(r, "backdrop"))
	}
	logo = artworkURL(i.Id, "Logo", i.ImageTags["Logo"], artworkWidth(r, "logo"))
	if logo == "" {
		logo = artworkURL(i.ParentLogoItemId, "Logo", i.ParentLogoImageTag, artworkWidth(r, "logo"))
	}
	return poster, backdrop, logo
}

// browsable are the Jellyfin API's types Silo's apps browse and play.
var browsable = []string{"Movie", "Series", "Season", "Episode", "Video"}

func cards(r *http.Request, items []item) []catalogItem {
	result := []catalogItem{}
	for _, i := range items {
		if slices.Contains(browsable, i.Type) {
			result = append(result, catalogItemOf(r, i))
		}
	}
	return result
}

type userLibrary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	SortOrder int    `json:"sort_order"`
	PosterURL string `json:"poster_url,omitempty"`
}

// libraryTypes are the types of the libraries Silo's apps list, by the
// Jellyfin API's collection types: those of movies and series.
var libraryTypes = map[string]string{"movies": "movies", "tvshows": "shows", "": "mixed", "homevideos": "mixed"}

// views are the user's libraries Silo's apps browse, as the Jellyfin API
// lists them to the user, with their numbers.
func (s *Server) views(r *http.Request, v viewer) ([]item, map[accounts.ID]int32, answer, error) {
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/UserViews", nil, &result)
	if err != nil || !a.ok() {
		return nil, nil, a, err
	}
	var views []item
	var ids []accounts.ID
	for _, view := range result.Items {
		id, err := accounts.ParseID(view.Id)
		if _, ok := libraryTypes[view.CollectionType]; ok && err == nil {
			views, ids = append(views, view), append(ids, id)
		}
	}
	numbers, err := s.numbers.of(r.Context(), ids...)
	return views, numbers, a, err
}

// libraries lists the libraries the user sees, as the Jellyfin API lists
// them to Jellyfin apps: those the user may see and has not hidden.
func (s *Server) libraries(w http.ResponseWriter, r *http.Request, v viewer) {
	if s.profileRefused(w, r, v, false) {
		return
	}
	views, numbers, a, err := s.views(r, v)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	libraries := []userLibrary{}
	for i, view := range views {
		id, _ := accounts.ParseID(view.Id)
		libraries = append(libraries, userLibrary{ID: strconv.Itoa(int(numbers[id])), Name: view.Name,
			Type: libraryTypes[view.CollectionType], SortOrder: i,
			PosterURL: artworkURL(view.Id, "Primary", view.ImageTags["Primary"], artworkWidth(r, "poster"))})
	}
	writeJSON(w, http.StatusOK, struct {
		Items []userLibrary `json:"items"`
	}{libraries})
}

// library is the Jellyfin view a library number stands for; ok is false,
// the request answered, when it is none of the user's.
func (s *Server) library(w http.ResponseWriter, r *http.Request, v viewer, number string) (item, bool) {
	id, err := s.numbers.id(r.Context(), number)
	if err != nil && err != errUnknownNumber {
		s.internalError(w, r, err)
		return item{}, false
	}
	views, _, a, err := s.views(r, v)
	if err != nil {
		s.internalError(w, r, err)
		return item{}, false
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return item{}, false
	}
	for _, view := range views {
		if view.Id == id.String() {
			return view, true
		}
	}
	problem(w, http.StatusNotFound, "not_found", "Library not found.")
	return item{}, false
}

type section struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	Type       string        `json:"section_type"`
	Items      []catalogItem `json:"items"`
	ItemLimit  int           `json:"item_limit"`
	TotalCount int           `json:"total_count"`
	Customized bool          `json:"customized"`
	Featured   bool          `json:"featured"`
	IsCustom   bool          `json:"is_custom"`
}

// sectionLimit is how many titles a Home section lists.
const sectionLimit = 24

// sectionTitles are the titles of the Home sections, by the server's
// language: Continue Watching, Next Up, and the latest of a library.
var sectionTitles = map[string][3]string{
	"en":    {"Continue Watching", "Next Up", "Latest in %s"},
	"fr":    {"Reprendre", "À suivre", "Derniers ajouts dans %s"},
	"de":    {"Weiterschauen", "Als Nächstes", "Neu in %s"},
	"es":    {"Seguir viendo", "A continuación", "Lo último en %s"},
	"it":    {"Continua a guardare", "Prossimo", "Ultimi in %s"},
	"pt-BR": {"Continuar assistindo", "A seguir", "Novidades em %s"},
}

func (s *Server) titles() [3]string {
	if titles, ok := sectionTitles[s.options.Accounts.Settings().Language]; ok {
		return titles
	}
	return sectionTitles["en"]
}

// The Home sections: what the user is watching, the next episodes, then
// the latest titles of each library, as the Jellyfin API's lists of those
// names give them to Jellyfin apps' home pages.
const (
	resumeSection = "continue_watching"
	nextUpSection = "next_up"
	latestPrefix  = "latest-"
)

// sectionOf reads one Home section.
func (s *Server) sectionOf(r *http.Request, v viewer, id, title string, library string) (section, answer, error) {
	limit := strconv.Itoa(sectionLimit)
	sec := section{ID: id, Title: title, ItemLimit: sectionLimit}
	var items []item
	var a answer
	var err error
	switch {
	case id == resumeSection:
		sec.Type = resumeSection
		var result queryResult
		a, err = s.call(r, v, http.MethodGet, "/UserItems/Resume?MediaTypes=Video&Limit="+limit+"&Fields="+listFields, nil, &result)
		items = result.Items
	case id == nextUpSection:
		sec.Type = nextUpSection
		var result queryResult
		a, err = s.call(r, v, http.MethodGet, "/Shows/NextUp?Limit="+limit+"&Fields="+listFields, nil, &result)
		items = result.Items
	default:
		sec.Type = "recently_added"
		a, err = s.call(r, v, http.MethodGet, "/Items/Latest?ParentId="+library+"&Limit="+limit+"&Fields="+listFields, nil, &items)
	}
	if err != nil || !a.ok() {
		return sec, a, err
	}
	sec.Items = cards(r, items)
	for i := range sec.Items {
		switch sec.Type {
		case resumeSection:
			sec.Items[i].ItemSource = "in_progress"
		case nextUpSection:
			sec.Items[i].ItemSource = "next_up"
		}
	}
	sec.TotalCount = len(sec.Items)
	return sec, a, nil
}

// homeSections are the sections of the Home page: those with titles.
func (s *Server) homeSections(w http.ResponseWriter, r *http.Request, v viewer) {
	views, numbers, a, err := s.views(r, v)
	if err == nil && !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	titles := s.titles()
	type wanted struct{ id, title, library string }
	all := []wanted{{resumeSection, titles[0], ""}, {nextUpSection, titles[1], ""}}
	for _, view := range views {
		id, _ := accounts.ParseID(view.Id)
		all = append(all, wanted{latestPrefix + strconv.Itoa(int(numbers[id])), fmt.Sprintf(titles[2], view.Name), view.Id})
	}
	sections := []section{}
	for _, want := range all {
		sec, a, err := s.sectionOf(r, v, want.id, want.title, want.library)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !a.ok() {
			s.jellyfinRefusal(w, r, a)
			return
		}
		if len(sec.Items) > 0 {
			sections = append(sections, sec)
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Sections []section `json:"sections"`
	}{sections})
}

// homeSection is one section of the Home page, by its identifier.
func (s *Server) homeSection(w http.ResponseWriter, r *http.Request, v viewer) {
	sec, ok := s.namedSection(w, r, v, r.PathValue("id"))
	if ok {
		writeJSON(w, http.StatusOK, sec)
	}
}

func (s *Server) namedSection(w http.ResponseWriter, r *http.Request, v viewer, id string) (section, bool) {
	titles := s.titles()
	var sec section
	var a answer
	var err error
	switch {
	case id == resumeSection:
		sec, a, err = s.sectionOf(r, v, id, titles[0], "")
	case id == nextUpSection:
		sec, a, err = s.sectionOf(r, v, id, titles[1], "")
	case strings.HasPrefix(id, latestPrefix):
		view, ok := s.library(w, r, v, strings.TrimPrefix(id, latestPrefix))
		if !ok {
			return section{}, false
		}
		sec, a, err = s.sectionOf(r, v, id, fmt.Sprintf(titles[2], view.Name), view.Id)
	default:
		problem(w, http.StatusNotFound, "not_found", "Section not found.")
		return section{}, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return section{}, false
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return section{}, false
	}
	return sec, true
}

// catalogTypes are the Jellyfin API's types a catalog's type stands for.
var catalogTypes = map[string]string{"movie": "Movie", "series": "Series", "episode": "Episode", "video": "Movie,Series",
	"video_with_episodes": "Movie,Series,Episode"}

// catalogSorts are the Jellyfin API's orders a catalog's sort stands for.
var catalogSorts = map[string]string{"title": "SortName", "year": "ProductionYear,SortName", "release_date": "PremiereDate,SortName",
	"added_at": "DateCreated,SortName", "rating": "CommunityRating,SortName", "runtime": "Runtime,SortName", "random": "Random"}

// cursor is where a catalog page starts, written for the apps.
func cursor(start int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("start:" + strconv.Itoa(start)))
}

func startOf(text string) (int, bool) {
	data, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return 0, false
	}
	value, ok := strings.CutPrefix(string(data), "start:")
	start, err := strconv.Atoi(value)
	return start, ok && err == nil && start >= 0
}

type catalogPage struct {
	Items        []catalogItem `json:"items"`
	Page         pageInfo      `json:"page"`
	Total        int           `json:"total"`
	TotalExact   bool          `json:"total_exact"`
	WindowCursor string        `json:"window_cursor"`
}

// catalog pages through a library, a Home section or a search, as the
// Jellyfin API lists them to Jellyfin apps.
func (s *Server) catalog(w http.ResponseWriter, r *http.Request, v viewer) {
	q := r.URL.Query()
	limit := 50
	if text := q.Get("limit"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > 200 {
			validationProblem(w, "query.limit", "out_of_range", "limit is between 1 and 200.")
			return
		}
		limit = value
	}
	start := 0
	if text := q.Get("cursor"); text != "" {
		var ok bool
		if start, ok = startOf(text); !ok {
			problem(w, http.StatusBadRequest, "invalid_cursor", "This cursor is not one of this server's.")
			return
		}
	}
	types, ok := catalogTypes[cmp.Or(q.Get("type"), "video")]
	if !ok {
		validationProblem(w, "query.type", "unsupported", "This server lists movies, series and episodes.")
		return
	}
	sort := strings.TrimPrefix(q.Get("sort"), "-")
	order, ok := catalogSorts[sort]
	if sort != "" && !ok {
		validationProblem(w, "query.sort", "unknown_sort_field", "This server sorts by title, year, release_date, added_at, rating, runtime or random.")
		return
	}
	switch cmp.Or(q.Get("source"), "query") {
	case "section":
		sec, ok := s.namedSection(w, r, v, q.Get("section_id"))
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, pageOf(sec.Items, start, limit, len(sec.Items)))
		return
	case "favorites":
	case "query":
	default:
		validationProblem(w, "query.source", "unsupported", "This server lists libraries, sections, favorites and searches.")
		return
	}
	query := url.Values{"Fields": {listFields}, "IncludeItemTypes": {types}, "StartIndex": {strconv.Itoa(start)},
		"Limit": {strconv.Itoa(limit)}, "EnableTotalRecordCount": {"true"}}
	if order != "" {
		query.Set("SortBy", order)
		query.Set("SortOrder", "Ascending")
		if strings.HasPrefix(q.Get("sort"), "-") {
			query.Set("SortOrder", "Descending")
		}
	}
	if library := q.Get("library_id"); library != "" {
		view, ok := s.library(w, r, v, library)
		if !ok {
			return
		}
		query.Set("ParentId", view.Id)
	}
	if q.Get("source") == "favorites" {
		query.Set("Filters", "IsFavorite")
		query.Set("Recursive", "true")
	}
	if term := strings.TrimSpace(cmp.Or(q.Get("q"), q.Get("name_prefix"))); term != "" {
		query.Set("SearchTerm", term)
		query.Set("Recursive", "true")
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+query.Encode(), nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	items := cards(r, result.Items)
	total := max(result.TotalRecordCount, start+len(result.Items))
	page := catalogPage{Items: items, Total: total, TotalExact: true, WindowCursor: cursor(0)}
	if next := start + len(result.Items); len(result.Items) > 0 && next < total {
		page.Page = pageInfo{HasMore: true, NextCursor: cursor(next)}
	}
	writeJSON(w, http.StatusOK, page)
}

// pageOf is a page of a list read whole.
func pageOf(all []catalogItem, start, limit, total int) catalogPage {
	page := catalogPage{Items: []catalogItem{}, Total: total, TotalExact: true, WindowCursor: cursor(0)}
	if start < len(all) {
		page.Items = all[start:min(start+limit, len(all))]
	}
	if next := start + len(page.Items); next < len(all) {
		page.Page = pageInfo{HasMore: true, NextCursor: cursor(next)}
	}
	return page
}

// searchCapabilities tells the apps searches are available, through the
// catalog's q.
func (s *Server) searchCapabilities(w http.ResponseWriter, _ *http.Request, _ viewer) {
	writeJSON(w, http.StatusOK, struct {
		Allowed  bool   `json:"allowed"`
		State    string `json:"state"`
		Revision string `json:"revision"`
	}{true, "available", "polyfin-1"})
}

// itemOf reads an item's description, answering the request when the
// user may not see it.
func (s *Server) itemOf(w http.ResponseWriter, r *http.Request, v viewer, id string) (item, bool) {
	if _, err := accounts.ParseID(id); err != nil {
		problem(w, http.StatusNotFound, "not_found", "Item not found.")
		return item{}, false
	}
	var i item
	a, err := s.call(r, v, http.MethodGet, "/Items/"+id+"?Fields="+detailFields, nil, &i)
	if err != nil {
		s.internalError(w, r, err)
		return item{}, false
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return item{}, false
	}
	if !slices.Contains(browsable, i.Type) {
		problem(w, http.StatusNotFound, "not_found", "Item not found.")
		return item{}, false
	}
	return i, true
}

type castCredit struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	Order     int    `json:"order"`
	PersonID  string `json:"person_id,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
}

type crewCredit struct {
	Name     string `json:"name"`
	Job      string `json:"job"`
	PersonID string `json:"person_id,omitempty"`
	PhotoURL string `json:"photo_url,omitempty"`
}

type subtitleInfo struct {
	Language        string `json:"language"`
	Codec           string `json:"codec,omitempty"`
	Title           string `json:"title,omitempty"`
	Source          string `json:"source"`
	Forced          bool   `json:"forced"`
	HearingImpaired bool   `json:"hearing_impaired"`
}

type rollup struct {
	Played          bool    `json:"played"`
	PositionSeconds float64 `json:"position_seconds,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	IsInProgress    bool    `json:"is_in_progress,omitempty"`
	InProgressCount int     `json:"in_progress_count"`
	UnplayedCount   int     `json:"unplayed_count"`
	WatchedCount    int     `json:"watched_count"`
}

// rollupOf is the user's progress on an item: a title's position, or a
// series' or season's episodes left.
func rollupOf(i item) *rollup {
	if i.UserData == nil {
		return nil
	}
	data := i.UserData
	result := &rollup{Played: data.Played, DurationSeconds: seconds(i.RunTimeTicks)}
	if data.PlaybackPositionTicks > 0 {
		result.PositionSeconds, result.IsInProgress, result.InProgressCount = float64(data.PlaybackPositionTicks)/1e7, true, 1
	}
	if data.UnplayedItemCount != nil {
		result.UnplayedCount = *data.UnplayedItemCount
		if total := i.RecursiveItemCount; total != nil {
			result.WatchedCount = max(*total-*data.UnplayedItemCount, 0)
		}
	} else if data.Played {
		result.WatchedCount = 1
	} else {
		result.UnplayedCount = 1
	}
	return result
}

type itemDetail struct {
	catalogItem
	OriginalTitle string         `json:"original_title,omitempty"`
	Tagline       string         `json:"tagline,omitempty"`
	Cast          []castCredit   `json:"cast"`
	Crew          []crewCredit   `json:"crew"`
	Ratings       []rating       `json:"ratings"`
	Subtitles     []subtitleInfo `json:"subtitles"`
	Versions      []fileVersion  `json:"versions"`
	SeasonCount   *int           `json:"season_count,omitempty"`
	EpisodeCount  *int           `json:"episode_count,omitempty"`
	ImdbID        string         `json:"imdb_id,omitempty"`
	TmdbID        string         `json:"tmdb_id,omitempty"`
	TvdbID        string         `json:"tvdb_id,omitempty"`
	UserData      *rollup        `json:"user_data,omitempty"`
}

type rating struct {
	Source  string  `json:"source"`
	Name    string  `json:"name"`
	Score   float64 `json:"score"`
	Display string  `json:"display"`
}

// ratingsOf are the ratings an item's page shows: the community's and the
// critics', on a scale of 100.
func ratingsOf(i item) []rating {
	ratings := []rating{}
	if i.CommunityRating != nil && *i.CommunityRating > 0 {
		score := min(*i.CommunityRating*10, 100)
		ratings = append(ratings, rating{Source: "community", Name: "Community", Score: score,
			Display: strconv.FormatFloat(*i.CommunityRating, 'f', 1, 64)})
	}
	if i.CriticRating != nil && *i.CriticRating > 0 {
		score := min(*i.CriticRating, 100)
		ratings = append(ratings, rating{Source: "critics", Name: "Critics", Score: score,
			Display: strconv.FormatFloat(*i.CriticRating, 'f', 0, 64) + "%"})
	}
	return ratings
}

// catalogItem describes a title for its page.
func (s *Server) catalogItem(w http.ResponseWriter, r *http.Request, v viewer) {
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	versions, err := s.versionsOf(r.Context(), i)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	detail := itemDetail{catalogItem: catalogItemOf(r, i), OriginalTitle: i.OriginalTitle, Cast: []castCredit{}, Crew: []crewCredit{},
		Ratings: ratingsOf(i), Subtitles: subtitlesOf(i.MediaSources), Versions: versions, UserData: rollupOf(i),
		ImdbID: i.ProviderIds["Imdb"], TmdbID: i.ProviderIds["Tmdb"], TvdbID: i.ProviderIds["Tvdb"]}
	if len(i.Taglines) > 0 {
		detail.Tagline = i.Taglines[0]
	}
	for _, p := range i.People {
		photo := artworkURL(p.Id, "Primary", p.PrimaryImageTag, artworkWidth(r, "photo"))
		if p.Type == "Actor" || p.Type == "GuestStar" {
			detail.Cast = append(detail.Cast, castCredit{Name: p.Name, Character: p.Role, Order: len(detail.Cast), PersonID: p.Id, PhotoURL: photo})
		} else {
			detail.Crew = append(detail.Crew, crewCredit{Name: p.Name, Job: p.Type, PersonID: p.Id, PhotoURL: photo})
		}
	}
	if i.Type == "Series" {
		detail.SeasonCount, detail.EpisodeCount = i.ChildCount, i.RecursiveItemCount
	}
	writeJSON(w, http.StatusOK, detail)
}

// catalogVersions lists a title's versions.
func (s *Server) catalogVersions(w http.ResponseWriter, r *http.Request, v viewer) {
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	versions, err := s.versionsOf(r.Context(), i)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []fileVersion `json:"items"`
	}{versions})
}

type season struct {
	ContentID     string  `json:"content_id"`
	Title         string  `json:"title"`
	SeasonNumber  int     `json:"season_number"`
	EpisodeCount  int     `json:"episode_count"`
	Overview      string  `json:"overview,omitempty"`
	PosterURL     string  `json:"poster_url,omitempty"`
	AirDate       string  `json:"air_date,omitempty"`
	IsSpecials    bool    `json:"is_specials,omitempty"`
	UserData      *rollup `json:"user_data,omitempty"`
	PlayContentID string  `json:"play_content_id,omitempty"`
}

// seriesSeasons reads a series' seasons, answering the request when the
// user may not see the series.
func (s *Server) seriesSeasons(w http.ResponseWriter, r *http.Request, v viewer, series string) ([]item, bool) {
	if _, err := accounts.ParseID(series); err != nil {
		problem(w, http.StatusNotFound, "not_found", "Series not found.")
		return nil, false
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Shows/"+series+"/Seasons?Fields="+listFields, nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return nil, false
	}
	return result.Items, true
}

func (s *Server) seasons(w http.ResponseWriter, r *http.Request, v viewer) {
	items, ok := s.seriesSeasons(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	seasons := []season{}
	for _, i := range items {
		row := season{ContentID: i.Id, Title: i.Name, AirDate: date(i.PremiereDate), UserData: rollupOf(i)}
		if i.IndexNumber != nil {
			row.SeasonNumber = *i.IndexNumber
		}
		row.IsSpecials = row.SeasonNumber == 0
		if i.ChildCount != nil {
			row.EpisodeCount = *i.ChildCount
		}
		if i.Overview != nil {
			row.Overview = *i.Overview
		}
		if r.URL.Query().Get("include_artwork") != "false" {
			row.PosterURL, _, _ = artworkOf(r, i)
		}
		seasons = append(seasons, row)
	}
	writeJSON(w, http.StatusOK, struct {
		Items []season `json:"items"`
	}{seasons})
}

type episodeFile struct {
	FileID       string `json:"file_id"`
	FileSize     int64  `json:"file_size"`
	Container    string `json:"container,omitempty"`
	CodecVideo   string `json:"codec_video,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	AudioChannel int    `json:"audio_channels,omitempty"`
	HDR          bool   `json:"hdr"`
}

type episode struct {
	ContentID     string        `json:"content_id"`
	Title         string        `json:"title"`
	SeasonNumber  int           `json:"season_number"`
	EpisodeNumber int           `json:"episode_number"`
	Runtime       int           `json:"runtime"`
	Overview      string        `json:"overview,omitempty"`
	AirDate       string        `json:"air_date,omitempty"`
	StillURL      string        `json:"still_url,omitempty"`
	Files         []episodeFile `json:"files,omitempty"`
	UserData      *rollup       `json:"user_data,omitempty"`
	ImdbID        string        `json:"imdb_id,omitempty"`
	TmdbID        string        `json:"tmdb_id,omitempty"`
	TvdbID        string        `json:"tvdb_id,omitempty"`
}

// episodesOf lists episodes, from the Jellyfin API's episodes of a series
// asked with query.
func (s *Server) episodesOf(w http.ResponseWriter, r *http.Request, v viewer, series, query string) {
	if _, err := accounts.ParseID(series); err != nil {
		problem(w, http.StatusNotFound, "not_found", "Series not found.")
		return
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Shows/"+series+"/Episodes?Fields="+listFields+",MediaSources&"+query, nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	// The versions of every episode are numbered at once.
	numbers, err := s.versionNumbers(r.Context(), result.Items)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	episodes := []episode{}
	for _, i := range result.Items {
		row := episode{ContentID: i.Id, Title: i.Name, AirDate: date(i.PremiereDate), UserData: rollupOf(i),
			Runtime: int(seconds(i.RunTimeTicks)/60 + 0.5), ImdbID: i.ProviderIds["Imdb"], TmdbID: i.ProviderIds["Tmdb"], TvdbID: i.ProviderIds["Tvdb"]}
		if i.ParentIndexNumber != nil {
			row.SeasonNumber = *i.ParentIndexNumber
		}
		if i.IndexNumber != nil {
			row.EpisodeNumber = *i.IndexNumber
		}
		if i.Overview != nil {
			row.Overview = *i.Overview
		}
		row.StillURL = artworkURL(i.Id, "Primary", i.ImageTags["Primary"], artworkWidth(r, "still"))
		for _, version := range versionsWith(i, numbers) {
			row.Files = append(row.Files, episodeFile{FileID: version.FileID, FileSize: version.FileSize, Container: version.Container,
				CodecVideo: version.CodecVideo, Resolution: version.Resolution, HDR: version.HDR})
		}
		episodes = append(episodes, row)
	}
	writeJSON(w, http.StatusOK, struct {
		Items []episode `json:"items"`
	}{episodes})
}

func (s *Server) seasonEpisodes(w http.ResponseWriter, r *http.Request, v viewer) {
	number, err := strconv.Atoi(r.PathValue("num"))
	if err != nil || number < 0 {
		validationProblem(w, "path.num", "out_of_range", "A season number is 0 or more.")
		return
	}
	s.episodesOf(w, r, v, r.PathValue("id"), "Season="+strconv.Itoa(number))
}

// itemEpisodes lists the episodes of a season, or of a series.
func (s *Server) itemEpisodes(w http.ResponseWriter, r *http.Request, v viewer) {
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	switch i.Type {
	case "Season":
		s.episodesOf(w, r, v, i.SeriesId, "SeasonId="+i.Id)
	case "Series":
		s.episodesOf(w, r, v, i.Id, "")
	default:
		problem(w, http.StatusNotFound, "not_found", "This item has no episodes.")
	}
}

// watched marks an item played or unplayed, as Jellyfin apps do.
func (s *Server) watched(played bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		id, err := accounts.ParseID(r.PathValue("id"))
		if err != nil {
			problem(w, http.StatusNotFound, "not_found", "Item not found.")
			return
		}
		method := http.MethodPost
		if !played {
			method = http.MethodDelete
		}
		a, err := s.call(r, v, method, "/UserPlayedItems/"+id.String(), nil, nil)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !a.ok() {
			s.jellyfinRefusal(w, r, a)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
