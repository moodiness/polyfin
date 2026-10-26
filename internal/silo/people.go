package silo

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// People.
//
// The people titles credit are the Jellyfin API's Person items, which
// Polyfin identifies by their name. Silo's apps identify people by
// numbers, as they do libraries and files: the Android app opens a
// person's page only by a positive number. A person's page is the Jellyfin
// API's description of them, and their titles those it lists for them to
// the user, under the same limits.

func (s *Server) peopleRoutes() {
	s.mux.HandleFunc("GET /api/v2/catalog/people", s.profiled(s.searchPeople))
	s.mux.HandleFunc("GET /api/v2/catalog/people/{id}", s.profiled(s.person))
	// Polyfin knows people by the titles that credit them, and asks no
	// source for their biographies: the apps stop asking on this answer.
	s.mux.HandleFunc("POST /api/v2/catalog/people/{id}/refresh", s.profiled(func(w http.ResponseWriter, _ *http.Request, _ viewer) {
		problem(w, http.StatusServiceUnavailable, "unavailable", "This server does not refresh people's details.")
	}))
}

// personJSON describes a person.
type personJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PhotoURL string `json:"photo_url,omitempty"`
	ImdbID   string `json:"imdb_id,omitempty"`
	TmdbID   string `json:"tmdb_id,omitempty"`
	TvdbID   string `json:"tvdb_id,omitempty"`
}

// personNumbers numbers people, by their Polyfin identifiers.
func (s *Server) personNumbers(r *http.Request, people []person) (map[accounts.ID]int32, error) {
	ids := make([]accounts.ID, 0, len(people))
	for _, p := range people {
		if id, err := accounts.ParseID(p.Id); err == nil {
			ids = append(ids, id)
		}
	}
	return s.numbers.of(r.Context(), ids...)
}

// personNumber is the number of a person, "" for one without an
// identifier.
func personNumber(numbers map[accounts.ID]int32, p person) string {
	id, err := accounts.ParseID(p.Id)
	if err != nil {
		return ""
	}
	return strconv.Itoa(int(numbers[id]))
}

// personID is the Polyfin identifier of the person a number stands for;
// ok is false, the request answered, for a number of no person.
func (s *Server) personID(w http.ResponseWriter, r *http.Request, number string) (string, bool) {
	id, err := s.numbers.id(r.Context(), number)
	if err == errUnknownNumber {
		problem(w, http.StatusNotFound, "not_found", "Person not found.")
		return "", false
	}
	if err != nil {
		s.internalError(w, r, err)
		return "", false
	}
	return id.String(), true
}

// person describes a person, as the Jellyfin API describes them to the
// user: someone credited only in titles the user may not see is unknown.
func (s *Server) person(w http.ResponseWriter, r *http.Request, v viewer) {
	number := r.PathValue("id")
	id, ok := s.personID(w, r, number)
	if !ok {
		return
	}
	var p item
	a, err := s.call(r, v, http.MethodGet, "/Items/"+id+"?Fields=ProviderIds", nil, &p)
	switch {
	case err != nil:
		s.internalError(w, r, err)
	case !a.ok():
		s.jellyfinRefusal(w, r, a)
	case p.Type != "Person":
		problem(w, http.StatusNotFound, "not_found", "Person not found.")
	default:
		writeJSON(w, http.StatusOK, personJSON{ID: number, Name: p.Name,
			PhotoURL: artworkURL(p.Id, "Primary", p.ImageTags["Primary"], artworkWidth(r, "photo")),
			ImdbID:   p.ProviderIds["Imdb"], TmdbID: p.ProviderIds["Tmdb"], TvdbID: p.ProviderIds["Tvdb"]})
	}
}

// peopleScopes are the media scopes a search of people may be narrowed
// to: Polyfin credits people in movies and series, those of other media
// are none.
var peopleScopes = map[string]bool{"": true, "video": true, "video_with_episodes": true, "movie": true, "series": true,
	"episode": true, "audiobook": false, "ebook": false, "manga": false}

// searchPeople lists the people whose name holds the search, credited in
// titles the user may see, as the Jellyfin API lists them.
func (s *Server) searchPeople(w http.ResponseWriter, r *http.Request, v viewer) {
	q := r.URL.Query()
	limit := 20
	if text := q.Get("limit"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > 100 {
			validationProblem(w, "query.limit", "out_of_range", "limit is between 1 and 100.")
			return
		}
		limit = value
	}
	scope := q.Get("media_scope")
	videos, known := peopleScopes[scope]
	if !known {
		validationProblem(w, "query.media_scope", "unsupported", "This media scope is not one of the contract's.")
		return
	}
	people := []personJSON{}
	if videos {
		query := url.Values{"Limit": {strconv.Itoa(limit)}, "Fields": {"ProviderIds"}}
		if term := strings.TrimSpace(q.Get("q")); term != "" {
			query.Set("SearchTerm", term)
		}
		var result queryResult
		a, err := s.call(r, v, http.MethodGet, "/Persons?"+query.Encode(), nil, &result)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !a.ok() {
			s.jellyfinRefusal(w, r, a)
			return
		}
		credited := make([]person, 0, len(result.Items))
		for _, i := range result.Items {
			credited = append(credited, person{Id: i.Id, Name: i.Name})
		}
		numbers, err := s.personNumbers(r, credited)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		for n, i := range result.Items {
			number := personNumber(numbers, credited[n])
			if number == "" {
				continue
			}
			people = append(people, personJSON{ID: number, Name: i.Name,
				PhotoURL: artworkURL(i.Id, "Primary", i.ImageTags["Primary"], artworkWidth(r, "photo")),
				ImdbID:   i.ProviderIds["Imdb"], TmdbID: i.ProviderIds["Tmdb"], TvdbID: i.ProviderIds["Tvdb"]})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Items []personJSON `json:"items"`
	}{people})
}
