package jellyfin

import (
	"cmp"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Moonfin's extras: the MDBList ratings and TMDB extras Moonfin's apps
// show on detail pages, from the ratings Polyfin keeps and its TMDB
// client, with the server's keys, which are never given to users.
//
// Each JSON answer carries success: true when it worked, as the apps
// require; otherwise success: false and an error, with 200, as without
// the server's key or for a title TMDB does not know. Parameters out of
// shape answer 400.

// moonfinExtraRoutes registers Moonfin's extras through route, which
// registers a route for a signed-in user the companion is on for.
func (h *Handler) moonfinExtraRoutes(route func(method, pattern string, handler http.HandlerFunc)) {
	route(http.MethodGet, "/Moonfin/MdbList/Ratings", h.moonfinRatings)
	route(http.MethodGet, "/Moonfin/Tmdb/Reviews", h.moonfinReviews)
	route(http.MethodGet, "/Moonfin/Tmdb/PersonLinks", h.moonfinPersonLinks)
	route(http.MethodGet, "/Moonfin/Tmdb/Details", h.moonfinDetails)
	route(http.MethodGet, "/Moonfin/Tmdb/Thumb", h.moonfinThumb)
	route(http.MethodGet, "/Moonfin/Tmdb/EpisodeRating", h.moonfinEpisodeRating)
	route(http.MethodGet, "/Moonfin/Tmdb/SeasonRatings", h.moonfinSeasonRatings)
	route(http.MethodGet, "/Moonfin/Tmdb/NextEpisode", h.moonfinNextEpisode)
}

// extraFailure answers success: false with message.
func extraFailure(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}

// extraNumber reads a query parameter as a whole number from least to
// 1<<30.
func extraNumber(r *http.Request, name string, least int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(query(r, name)))
	return n, err == nil && n >= least && n <= 1<<30
}

// extraKind reads the type parameter of a TMDB extra: movie or tv.
func extraKind(r *http.Request) (tmdb.Kind, bool) {
	switch strings.ToLower(strings.TrimSpace(query(r, "type"))) {
	case "movie":
		return tmdb.Movie, true
	case "tv":
		return tmdb.TV, true
	}
	return "", false
}

// imdbPattern is the shape of an IMDb identifier.
var imdbPattern = regexp.MustCompile(`^tt[0-9]{1,12}$`)

// extraTitle is the title a TMDB extra names: its kind, and its TMDB
// identifier, else its IMDb one.
type extraTitle struct {
	kind   tmdb.Kind
	tmdbID int
	imdb   string
}

// readExtraTitle reads the title of a TMDB extra: its type, and its
// tmdbId, else its imdbId. ok is false when they are out of shape.
func readExtraTitle(r *http.Request) (title extraTitle, ok bool) {
	title.kind, ok = extraKind(r)
	if !ok {
		return extraTitle{}, false
	}
	if query(r, "tmdbId") != "" {
		title.tmdbID, ok = extraNumber(r, "tmdbId", 1)
		return title, ok
	}
	title.imdb = strings.ToLower(strings.TrimSpace(query(r, "imdbId")))
	return title, imdbPattern.MatchString(title.imdb)
}

// extraOutline returns TMDB's outline of a title in the metadata
// language, its TMDB identifier found by its IMDb one when it is named
// by that.
func (h *Handler) extraOutline(r *http.Request, title extraTitle) (tmdb.Outline, error) {
	if title.tmdbID == 0 {
		found, err := h.TMDB.FoundByIMDb(r.Context(), title.imdb)
		if err != nil {
			return tmdb.Outline{}, err
		}
		if title.tmdbID = found.Of(title.kind); title.tmdbID == 0 {
			return tmdb.Outline{}, tmdb.ErrNotFound
		}
	}
	language := cmp.Or(callerFrom(r.Context()).User.MetadataLanguage, h.Accounts.Settings().Language)
	return h.TMDB.TitleOutline(r.Context(), title.kind, title.tmdbID, language)
}

// extraTMDBFailure answers a TMDB extra TMDB could not give: unknown to
// TMDB, or not asked.
func (h *Handler) extraTMDBFailure(w http.ResponseWriter, r *http.Request, route string, err error) {
	switch {
	case errors.Is(err, tmdb.ErrNoKey):
		extraFailure(w, http.StatusOK, noTMDBKey)
	case errors.Is(err, tmdb.ErrNotFound):
		extraFailure(w, http.StatusOK, "TMDB does not know this title")
	default:
		if r.Context().Err() == nil {
			h.Logger.Warn("TMDB could not be asked for Moonfin", "route", route, "error", err)
		}
		extraFailure(w, http.StatusOK, "TMDB could not be asked")
	}
}

// noTMDBKey is the error of a TMDB extra without the server's TMDB key.
const noTMDBKey = "The server has no TMDB key"

// moonfinRating is one of the ratings /Moonfin/MdbList/Ratings answers:
// its source, as MDBList names it, its value as the apps show it (the
// IMDb rating out of 10, the others out of 100) and its score out of 100.
type moonfinRating struct {
	Source string  `json:"source"`
	Value  float64 `json:"value"`
	Score  float64 `json:"score"`
}

// moonfinRatings answers /Moonfin/MdbList/Ratings?type=movie|show&tmdbId=…
// with the MDBList ratings Polyfin keeps of the title: its IMDb rating,
// critics' score (tomatoes), audience score (tomatoes_audience) and
// Metacritic score, those it has none of left out; the apps keep those
// their user chose. The title is asked about by its IMDb identifier when
// TMDB gives it, as the library's titles are, so that both share what is
// kept, else by its TMDB one. A title not kept yet is asked about in the
// background, and answers success: false until then.
func (h *Handler) moonfinRatings(w http.ResponseWriter, r *http.Request) {
	var series bool
	switch strings.ToLower(strings.TrimSpace(query(r, "type"))) {
	case "movie":
	case "show":
		series = true
	default:
		extraFailure(w, http.StatusBadRequest, "type is movie or show")
		return
	}
	id, ok := extraNumber(r, "tmdbId", 1)
	if !ok {
		extraFailure(w, http.StatusBadRequest, "tmdbId is a TMDB identifier")
		return
	}
	if h.Accounts.Settings().MDBListRatingsKey == "" {
		extraFailure(w, http.StatusOK, "The server has no MDBList key")
		return
	}
	title := ratings.Title{Series: series, TMDB: strconv.Itoa(id)}
	if h.TMDB.Enabled() {
		kind := tmdb.Movie
		if series {
			kind = tmdb.TV
		}
		// Without TMDB's answer, the title is asked about by its TMDB
		// identifier.
		if imdb, err := h.TMDB.IMDbID(r.Context(), kind, id); err == nil {
			title.IMDb = imdb
		}
	}
	scores, kept := h.Library.TitleScores(r.Context(), title)
	if !kept {
		extraFailure(w, http.StatusOK, "No ratings are kept of this title yet")
		return
	}
	list := []moonfinRating{}
	add := func(source string, value *float64, score func(float64) float64) {
		if value != nil && *value > 0 {
			list = append(list, moonfinRating{Source: source, Value: *value, Score: score(*value)})
		}
	}
	outOf100 := func(value float64) float64 { return value }
	add("imdb", scores.IMDb, func(value float64) float64 { return math.Round(value * 10) })
	add("tomatoes", scores.Critics, outOf100)
	add("tomatoes_audience", scores.Audience, outOf100)
	add("metacritic", scores.Metacritic, outOf100)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "ratings": list})
}

// moonfinReview is a review /Moonfin/Tmdb/Reviews answers.
type moonfinReview struct {
	Author    string   `json:"author"`
	Rating    *float64 `json:"rating,omitempty"`
	Content   string   `json:"content"`
	CreatedAt string   `json:"createdAt,omitempty"`
	AvatarURL string   `json:"avatarUrl,omitempty"`
	URL       string   `json:"url,omitempty"`
}

// moonfinReviews answers /Moonfin/Tmdb/Reviews?tmdbId=…&type=movie|tv&language=…
// with up to 20 of TMDB's user reviews of the title, in language, else in
// English, their HTML turned into light Markdown.
func (h *Handler) moonfinReviews(w http.ResponseWriter, r *http.Request) {
	kind, kindOK := extraKind(r)
	id, idOK := extraNumber(r, "tmdbId", 1)
	if !kindOK || !idOK {
		extraFailure(w, http.StatusBadRequest, "type is movie or tv, tmdbId a TMDB identifier")
		return
	}
	if !h.TMDB.Enabled() {
		extraFailure(w, http.StatusOK, noTMDBKey)
		return
	}
	found, err := h.TMDB.Reviews(r.Context(), kind, id, strings.TrimSpace(query(r, "language")))
	if err != nil {
		h.extraTMDBFailure(w, r, "Reviews", err)
		return
	}
	reviews := make([]moonfinReview, 0, len(found))
	for _, review := range found {
		content := lightMarkdown(review.Content)
		if content == "" {
			continue
		}
		avatar := ""
		if path, ok := strings.CutPrefix(review.AvatarPath, "/http"); ok {
			avatar = "http" + path
		} else if review.AvatarPath != "" {
			avatar = h.TMDB.ImageURL(review.AvatarPath, "w185")
		}
		reviews = append(reviews, moonfinReview{Author: review.Author, Rating: review.Rating, Content: content,
			CreatedAt: review.Created, AvatarURL: avatar, URL: review.URL})
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "reviews": reviews})
}

// moonfinPersonLinks answers /Moonfin/Tmdb/PersonLinks?tmdbId=… with a
// person's IMDb identifier, website and accounts, those TMDB has none of
// left out.
func (h *Handler) moonfinPersonLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := extraNumber(r, "tmdbId", 1)
	if !ok {
		extraFailure(w, http.StatusBadRequest, "tmdbId is a TMDB identifier")
		return
	}
	if !h.TMDB.Enabled() {
		extraFailure(w, http.StatusOK, noTMDBKey)
		return
	}
	links, err := h.TMDB.PersonLinks(r.Context(), id)
	if err != nil {
		h.extraTMDBFailure(w, r, "PersonLinks", err)
		return
	}
	answer := map[string]any{"success": true}
	for name, value := range map[string]string{"imdbId": links.IMDb, "homepage": links.Homepage, "instagram": links.Instagram,
		"twitter": links.Twitter, "facebook": links.Facebook, "tiktok": links.TikTok, "youtube": links.YouTube} {
		if value != "" {
			answer[name] = value
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// moonfinDetails answers /Moonfin/Tmdb/Details?type=movie|tv&tmdbId=… (or
// imdbId=…) with what an app shows of a title the server does not hold,
// in the metadata language: its overview, TMDB's rating, its genres'
// names, runTimeTicks, a backdrop without text and a logo (paths on
// TMDB's image server) and a YouTube trailer, those TMDB has none of
// left out.
func (h *Handler) moonfinDetails(w http.ResponseWriter, r *http.Request) {
	title, ok := readExtraTitle(r)
	if !ok {
		extraFailure(w, http.StatusBadRequest, "type is movie or tv, with a tmdbId or an imdbId")
		return
	}
	if !h.TMDB.Enabled() {
		extraFailure(w, http.StatusOK, noTMDBKey)
		return
	}
	outline, err := h.extraOutline(r, title)
	if err != nil {
		h.extraTMDBFailure(w, r, "Details", err)
		return
	}
	answer := map[string]any{"success": true, "genres": outline.Genres}
	if outline.Overview != "" {
		answer["overview"] = outline.Overview
	}
	if outline.Rating > 0 {
		answer["rating"] = outline.Rating
	}
	if outline.Runtime > 0 {
		answer["runTimeTicks"] = int64(outline.Runtime) * 60 * 10_000_000
	}
	if outline.Backdrop != "" {
		answer["backdropPath"] = outline.Backdrop
	}
	if outline.Logo != "" {
		answer["logoPath"] = outline.Logo
	}
	if outline.Trailer != "" {
		answer["trailerUrl"] = "https://www.youtube.com/watch?v=" + outline.Trailer
	}
	writeJSON(w, http.StatusOK, answer)
}

// moonfinThumb answers /Moonfin/Tmdb/Thumb?type=movie|tv&tmdbId=… (or
// imdbId=…) with a redirect to a backdrop of the title on TMDB's image
// server with its title on it, in the metadata language, else in
// English, else TMDB's main one; with scheme=http, an http address, for
// apps that cannot load https. Without the server's TMDB key, or for a
// title TMDB does not know or has no backdrop of, it answers 404.
func (h *Handler) moonfinThumb(w http.ResponseWriter, r *http.Request) {
	title, ok := readExtraTitle(r)
	if !ok {
		http.Error(w, "type is movie or tv, with a tmdbId or an imdbId", http.StatusBadRequest)
		return
	}
	if !h.TMDB.Enabled() {
		http.NotFound(w, r)
		return
	}
	outline, err := h.extraOutline(r, title)
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) && r.Context().Err() == nil {
		h.Logger.Warn("TMDB could not be asked for Moonfin", "route", "Thumb", "error", err)
	}
	if err != nil || outline.TitledBackdrop == "" {
		http.NotFound(w, r)
		return
	}
	target := h.TMDB.ImageURL(outline.TitledBackdrop, "w780")
	if strings.EqualFold(query(r, "scheme"), "http") {
		if rest, ok := strings.CutPrefix(target, "https://"); ok {
			target = "http://" + rest
		}
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// extraSeason reads the tmdbId and season of a series' season, and
// answers 400, or success: false without the server's TMDB key; ok is
// false once answered.
func (h *Handler) extraSeason(w http.ResponseWriter, r *http.Request) (id, season int, ok bool) {
	id, idOK := extraNumber(r, "tmdbId", 1)
	season, seasonOK := extraNumber(r, "season", 0)
	if !idOK || !seasonOK {
		extraFailure(w, http.StatusBadRequest, "tmdbId is a TMDB identifier, season a number")
		return 0, 0, false
	}
	if !h.TMDB.Enabled() {
		extraFailure(w, http.StatusOK, noTMDBKey)
		return 0, 0, false
	}
	return id, season, true
}

// moonfinEpisodeRating answers /Moonfin/Tmdb/EpisodeRating?tmdbId=…&season=…&episode=…
// with TMDB's rating of an episode of a series, out of 10, from its
// season's answer, which /Moonfin/Tmdb/SeasonRatings shares.
func (h *Handler) moonfinEpisodeRating(w http.ResponseWriter, r *http.Request) {
	episode, ok := extraNumber(r, "episode", 0)
	if !ok {
		extraFailure(w, http.StatusBadRequest, "episode is a number")
		return
	}
	id, season, ok := h.extraSeason(w, r)
	if !ok {
		return
	}
	votes, err := h.TMDB.SeasonVotes(r.Context(), id, season)
	if err != nil {
		h.extraTMDBFailure(w, r, "EpisodeRating", err)
		return
	}
	for _, vote := range votes {
		if vote.Episode == episode {
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "voteAverage": vote.Average})
			return
		}
	}
	h.extraTMDBFailure(w, r, "EpisodeRating", tmdb.ErrNotFound)
}

// moonfinSeasonRatings answers /Moonfin/Tmdb/SeasonRatings?tmdbId=…&season=…
// with TMDB's ratings of the episodes of a season of a series.
func (h *Handler) moonfinSeasonRatings(w http.ResponseWriter, r *http.Request) {
	id, season, ok := h.extraSeason(w, r)
	if !ok {
		return
	}
	votes, err := h.TMDB.SeasonVotes(r.Context(), id, season)
	if err != nil {
		h.extraTMDBFailure(w, r, "SeasonRatings", err)
		return
	}
	type episodeRating struct {
		EpisodeNumber int     `json:"episodeNumber"`
		VoteAverage   float64 `json:"voteAverage"`
	}
	episodes := make([]episodeRating, 0, len(votes))
	for _, vote := range votes {
		episodes = append(episodes, episodeRating{EpisodeNumber: vote.Episode, VoteAverage: vote.Average})
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "episodes": episodes})
}

// moonfinNextEpisode answers /Moonfin/Tmdb/NextEpisode?tmdbId=… with the
// next episode of a series to air: the day it airs, its season and
// number and its name; success: false when none is scheduled.
func (h *Handler) moonfinNextEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := extraNumber(r, "tmdbId", 1)
	if !ok {
		extraFailure(w, http.StatusBadRequest, "tmdbId is a TMDB identifier")
		return
	}
	if !h.TMDB.Enabled() {
		extraFailure(w, http.StatusOK, noTMDBKey)
		return
	}
	next, err := h.TMDB.NextEpisode(r.Context(), id)
	if err != nil {
		h.extraTMDBFailure(w, r, "NextEpisode", err)
		return
	}
	if next == nil {
		extraFailure(w, http.StatusOK, "No episode is scheduled")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "airDate": next.AirDate, "seasonNumber": next.Season,
		"episodeNumber": next.Episode, "name": next.Name})
}

// blankLines matches three line ends or more, with the spaces between
// them.
var blankLines = regexp.MustCompile(`\n[ \t]*\n(?:[ \t]*\n)+`)

// lightMarkdown turns the HTML of a review into light Markdown: bold and
// italics, links, headings, quotes, list items, rules, paragraphs and
// line breaks; other tags are dropped, their text kept, but for scripts'
// and styles'. Text without tags is kept as it is, its line ends made
// \n.
func lightMarkdown(text string) string {
	var out strings.Builder
	var links []string
	skip := 0
	tokens := html.NewTokenizer(strings.NewReader(strings.ReplaceAll(text, "\r\n", "\n")))
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			break
		}
		token := tokens.Token()
		start := kind == html.StartTagToken || kind == html.SelfClosingTagToken
		end := kind == html.EndTagToken
		switch {
		case kind == html.TextToken:
			if skip == 0 {
				out.WriteString(token.Data)
			}
		case token.Data == "script" || token.Data == "style":
			if start {
				skip++
			} else if end && skip > 0 {
				skip--
			}
		case token.Data == "b" || token.Data == "strong":
			if start || end {
				out.WriteString("**")
			}
		case token.Data == "i" || token.Data == "em":
			if start || end {
				out.WriteString("*")
			}
		case token.Data == "a":
			if start {
				href := ""
				for _, attribute := range token.Attr {
					if attribute.Key == "href" && (strings.HasPrefix(attribute.Val, "https://") || strings.HasPrefix(attribute.Val, "http://")) {
						href = attribute.Val
					}
				}
				links = append(links, href)
				if href != "" {
					out.WriteString("[")
				}
			} else if end && len(links) > 0 {
				if href := links[len(links)-1]; href != "" {
					out.WriteString("](" + href + ")")
				}
				links = links[:len(links)-1]
			}
		case token.Data == "br":
			out.WriteString("\n")
		case token.Data == "hr":
			out.WriteString("\n\n---\n\n")
		case token.Data == "p" || token.Data == "div" || token.Data == "ul" || token.Data == "ol":
			out.WriteString("\n\n")
		case token.Data == "li":
			if start {
				out.WriteString("\n- ")
			}
		case token.Data == "blockquote":
			if start {
				out.WriteString("\n\n> ")
			} else {
				out.WriteString("\n\n")
			}
		case len(token.Data) == 2 && token.Data[0] == 'h' && token.Data[1] >= '1' && token.Data[1] <= '6':
			if start {
				out.WriteString("\n\n" + strings.Repeat("#", int(token.Data[1]-'0')) + " ")
			} else {
				out.WriteString("\n\n")
			}
		}
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(out.String(), "\n\n"))
}
