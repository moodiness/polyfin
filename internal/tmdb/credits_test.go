package tmdb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// creditsServer answers the paths of answers as TMDB does, recording each
// request's path and query.
func creditsServer(t *testing.T, answers map[string]any) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		query.Del("api_key")
		mu.Lock()
		asked = append(asked, r.URL.Path+"?"+query.Encode())
		mu.Unlock()
		answer, ok := answers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return New(server.URL+"/3", "https://images.example/t/p/", "test", keyed("k3y")), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
}

func names(credits []TitleCredit, kind string) []string {
	var kept []string
	for _, c := range credits {
		if c.Kind == kind {
			kept = append(kept, c.Name+"/"+c.Role)
		}
	}
	return kept
}

// A movie's people are its first 15 actors in billing order, with their
// character in the language asked, then its first 15 directors, writers
// (of a script, screenplay or novel) and producers in TMDB's order, the
// rest of the crew left out, each with their TMDB identifier and photo,
// as Jellyfin 12.4 keeps them. They come with the movie's details, its
// companies and tagline, in one request.
func TestAMoviesCreditsAreItsBilledCastAndCrew(t *testing.T) {
	t.Parallel()
	var cast []map[string]any
	for order := 19; order >= 0; order-- {
		cast = append(cast, map[string]any{"id": 100 + order, "name": fmt.Sprintf("Actor %02d", order), "character": fmt.Sprintf("Rôle %d", order),
			"order": order, "profile_path": fmt.Sprintf("/a%d.jpg", order)})
	}
	cast = append(cast, map[string]any{"id": 999, "name": " ", "order": -1})
	crew := []map[string]any{
		{"id": 1, "name": "Eve Director", "job": "Director", "department": "Directing", "profile_path": "/eve.jpg"},
		{"id": 2, "name": "Ed Editor", "job": "Editor", "department": "Editing"},
		{"id": 3, "name": "Will Writer", "job": "Screenplay", "department": "Writing"},
		{"id": 4, "name": "Nora Novelist", "job": "Novel", "department": "Writing"},
		{"id": 5, "name": "Pat Producer", "job": "Producer", "department": "Production"},
		{"id": 6, "name": "Exec Producer", "job": "Executive Producer", "department": "Production"},
	}
	for i := range 20 {
		crew = append(crew, map[string]any{"id": 200 + i, "name": fmt.Sprintf("Producer %02d", i), "job": "Producer", "department": "Production"})
	}
	client, asked := creditsServer(t, map[string]any{"/3/movie/1001": map[string]any{"tagline": " Une phrase. ",
		"production_companies": []any{map[string]any{"name": "North Pictures"}, map[string]any{"name": " "}, map[string]any{"name": "North Pictures"},
			map[string]any{"name": "South Films"}},
		"credits": map[string]any{"cast": cast, "crew": crew}}})
	details, err := client.TitleDetails(t.Context(), Movie, "1001", "fr-FR")
	if err != nil {
		t.Fatal(err)
	}
	if details.Tagline != "Une phrase." || !slices.Equal(details.Companies, []string{"North Pictures", "South Films"}) ||
		details.Networks == nil || len(details.Networks) != 0 {
		t.Errorf("details %q %q %q", details.Tagline, details.Companies, details.Networks)
	}
	credits := details.Credits
	actors := names(credits, Actor)
	if len(actors) != MaxCast || actors[0] != "Actor 00/Rôle 0" || actors[14] != "Actor 14/Rôle 14" {
		t.Errorf("actors %v", actors)
	}
	if credits[0].ID != 100 || credits[0].ProfilePath != "/a0.jpg" {
		t.Errorf("the first actor %+v", credits[0])
	}
	var crewKept []string
	for _, c := range credits[len(actors):] {
		crewKept = append(crewKept, c.Kind+":"+c.Name)
	}
	want := []string{"Director:Eve Director", "Writer:Will Writer", "Writer:Nora Novelist", "Producer:Pat Producer"}
	for i := range MaxCrew - len(want) {
		want = append(want, fmt.Sprintf("Producer:Producer %02d", i))
	}
	if !slices.Equal(crewKept, want) {
		t.Errorf("crew %v, want %v", crewKept, want)
	}
	if got := asked(); len(got) != 1 || got[0] != "/3/movie/1001?append_to_response=credits&language=fr-FR" {
		t.Errorf("TMDB was asked %v", got)
	}
}

// A series' actors are those of its aggregate credits, over its whole
// run, in billing order, an actor of several characters credited once a
// character, the one they played the longest first; its crew is that of
// its credits. Both come from one request. Without aggregate credits, its
// actors are those of its credits.
func TestASeriesCreditsAreItsAggregateCast(t *testing.T) {
	t.Parallel()
	aggregate := []map[string]any{
		{"id": 11, "name": "Second Billed", "order": 1, "roles": []any{map[string]any{"character": "Sam", "episode_count": 10}}},
		{"id": 10, "name": "Lead", "order": 0, "profile_path": "/lead.jpg", "roles": []any{
			map[string]any{"character": "Young Lee", "episode_count": 2}, map[string]any{"character": "Lee", "episode_count": 30}}},
	}
	for i := range 20 {
		aggregate = append(aggregate, map[string]any{"id": 100 + i, "name": fmt.Sprintf("Guest %02d", i), "order": 2 + i,
			"roles": []any{map[string]any{"character": "Guest", "episode_count": 1}}})
	}
	client, asked := creditsServer(t, map[string]any{
		"/3/tv/2031": map[string]any{"tagline": "Every tide turns.",
			"networks":             []any{map[string]any{"name": "Coast TV"}, map[string]any{"name": "Coast Streams"}},
			"production_companies": []any{map[string]any{"name": "Coast TV"}, map[string]any{"name": "Harbor Works"}},
			"aggregate_credits": map[string]any{"cast": aggregate, "crew": []any{
				map[string]any{"id": 77, "name": "Episode Director", "jobs": []any{map[string]any{"job": "Director"}}, "department": "Directing"}}},
			"credits": map[string]any{"cast": []any{map[string]any{"id": 10, "name": "Lead", "character": "Lee", "order": 0}},
				"crew": []any{map[string]any{"id": 50, "name": "Shaun Runner", "job": "Producer", "department": "Production"}}}},
		"/3/tv/2032": map[string]any{"aggregate_credits": map[string]any{"cast": []any{}},
			"credits": map[string]any{"cast": []any{map[string]any{"id": 10, "name": "Lead", "character": "Lee", "order": 0}}}},
	})
	details, err := client.TitleDetails(t.Context(), TV, "2031", "en")
	if err != nil {
		t.Fatal(err)
	}
	if details.Tagline != "Every tide turns." || !slices.Equal(details.Networks, []string{"Coast TV", "Coast Streams"}) ||
		!slices.Equal(details.Companies, []string{"Coast TV", "Harbor Works"}) {
		t.Errorf("details %q %q %q", details.Tagline, details.Networks, details.Companies)
	}
	if got := JoinStudios(details.Networks, details.Companies); !slices.Equal(got, []string{"Coast TV", "Coast Streams", "Harbor Works"}) {
		t.Errorf("studios %q", got)
	}
	credits := details.Credits
	actors := names(credits, Actor)
	if len(actors) != MaxCast+1 || !slices.Equal(actors[:3], []string{"Lead/Lee", "Lead/Young Lee", "Second Billed/Sam"}) ||
		actors[len(actors)-1] != "Guest 12/Guest" {
		t.Errorf("actors %v", actors)
	}
	if got := names(credits, Producer); !slices.Equal(got, []string{"Shaun Runner/Producer"}) || len(names(credits, Director)) != 0 {
		t.Errorf("crew %+v", credits[len(actors):])
	}
	if got := asked(); len(got) != 1 || !strings.HasPrefix(got[0], "/3/tv/2031?") || !strings.Contains(got[0], "append_to_response=aggregate_credits%2Ccredits") {
		t.Errorf("TMDB was asked %v", got)
	}
	details, err = client.TitleDetails(t.Context(), TV, "2032", "en")
	if credits = details.Credits; err != nil || len(credits) != 1 || credits[0].Role != "Lee" {
		t.Errorf("without aggregate credits: %+v %v", credits, err)
	}
}
