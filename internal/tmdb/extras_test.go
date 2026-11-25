package tmdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

// A title's reviews are the first 20 TMDB gives in the language asked,
// those of no text left out, or, when it has none in it, those in
// English; each review's author is their name, else their username.
func TestReviewsFallBackToEnglishWhenNoneAreInTheLanguage(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var asked []string
	requests := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		language := r.URL.Query().Get("language")
		mu.Lock()
		asked = append(asked, r.URL.Path+" "+language)
		mu.Unlock()
		results := []any{}
		if language == "en-US" {
			results = append(results, map[string]any{"author": "ghost", "author_details": map[string]any{"name": "", "username": "ghost",
				"avatar_path": "/a.jpg", "rating": 8.0}, "content": "Great.", "created_at": "2024-01-02T03:04:05.000Z", "url": "https://tmdb.example/r/1"},
				map[string]any{"author": "empty", "content": "  "})
			for i := range 25 {
				results = append(results, map[string]any{"author": fmt.Sprint("writer ", i), "author_details": map[string]any{"name": fmt.Sprint("Writer ", i), "rating": nil},
					"content": "Fine."})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"page": 1, "results": results})
	}))
	t.Cleanup(server.Close)
	client := New(server.URL+"/3", "https://images.example/t/p/", "test", keyed("k3y"))
	reviews, err := client.Reviews(t.Context(), Movie, 550, "fr-FR")
	if err != nil {
		t.Fatal(err)
	}
	if got := requests(); !slices.Equal(got, []string{"/3/movie/550/reviews fr-FR", "/3/movie/550/reviews en-US"}) {
		t.Errorf("asked %q", got)
	}
	if len(reviews) != maxReviews {
		t.Fatalf("%d reviews, want %d", len(reviews), maxReviews)
	}
	first := reviews[0]
	if first.Author != "ghost" || first.Rating == nil || *first.Rating != 8 || first.Content != "Great." || first.Created != "2024-01-02T03:04:05.000Z" ||
		first.AvatarPath != "/a.jpg" || first.URL != "https://tmdb.example/r/1" {
		t.Errorf("first review %+v", first)
	}
	if second := reviews[1]; second.Author != "Writer 0" || second.Rating != nil {
		t.Errorf("second review %+v", second)
	}
	// In English, TMDB is asked once.
	if _, err := client.Reviews(t.Context(), TV, 1399, "en"); err != nil || !slices.Equal(requests()[2:], []string{"/3/tv/1399/reviews en"}) {
		t.Errorf("in English: asked %q, %v", requests(), err)
	}
}

// A person's links come from their details and external identifiers,
// asked in one request; a person TMDB does not know is ErrNotFound.
func TestPersonLinksAreReadWithTheExternalIdentifiers(t *testing.T) {
	t.Parallel()
	client, asked := creditsServer(t, map[string]any{"/3/person/287": map[string]any{"homepage": " https://person.example ",
		"external_ids": map[string]any{"imdb_id": "nm0000093", "instagram_id": "bradpitt", "twitter_id": "", "facebook_id": nil,
			"tiktok_id": "brad", "youtube_id": "UC1"}}})
	links, err := client.PersonLinks(t.Context(), 287)
	if err != nil {
		t.Fatal(err)
	}
	want := PersonLinks{IMDb: "nm0000093", Homepage: "https://person.example", Instagram: "bradpitt", TikTok: "brad", YouTube: "UC1"}
	if links != want {
		t.Errorf("links %+v", links)
	}
	if got := asked(); !slices.Equal(got, []string{"/3/person/287?append_to_response=external_ids"}) {
		t.Errorf("asked %q", got)
	}
	if _, err := client.PersonLinks(t.Context(), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown person: %v", err)
	}
}

// A title's outline comes with its images and videos: the best voted
// backdrop without text, the titled backdrop and logo in the language,
// else in English, and an official YouTube trailer before a teaser or
// another site's; a series' runtime is that of its episodes, and an SVG
// logo is served as a PNG.
func TestTitleOutlineChoosesItsImagesAndTrailer(t *testing.T) {
	t.Parallel()
	image := func(path string, language any, rating float64) map[string]any {
		return map[string]any{"file_path": path, "iso_639_1": language, "vote_average": rating}
	}
	video := func(key, site, kind string, official bool, language string) map[string]any {
		return map[string]any{"key": key, "site": site, "type": kind, "official": official, "iso_639_1": language}
	}
	client, asked := creditsServer(t, map[string]any{
		"/3/movie/550": map[string]any{"overview": " A story. ", "vote_average": 8.4, "genres": []any{map[string]any{"name": "Drama"}, map[string]any{"name": " "}},
			"runtime": 139, "backdrop_path": "/main.jpg",
			"images": map[string]any{
				"backdrops": []any{image("/en.jpg", "en", 5), image("/plain-low.jpg", nil, 2), image("/plain.jpg", nil, 6), image("/fr.jpg", "fr", 3)},
				"logos":     []any{image("/logo-en.png", "en", 9), image("/logo-fr.svg", "fr", 1)},
			},
			"videos": map[string]any{"results": []any{video("vimeo", "Vimeo", "Trailer", true, "fr"), video("teaser", "YouTube", "Teaser", true, "fr"),
				video("fan", "YouTube", "Trailer", false, "fr"), video("official", "YouTube", "Trailer", true, "en")}}},
		"/3/tv/1399": map[string]any{"episode_run_time": []int{55}, "images": map[string]any{"backdrops": []any{}, "logos": []any{}}},
	})
	outline, err := client.TitleOutline(t.Context(), Movie, 550, "fr-FR")
	if err != nil {
		t.Fatal(err)
	}
	if outline.Overview != "A story." || outline.Rating != 8.4 || !slices.Equal(outline.Genres, []string{"Drama"}) || outline.Runtime != 139 ||
		outline.Backdrop != "/plain.jpg" || outline.TitledBackdrop != "/fr.jpg" || outline.Logo != "/logo-fr.png" || outline.Trailer != "official" {
		t.Errorf("outline %+v", outline)
	}
	if got := asked(); !slices.Equal(got, []string{"/3/movie/550?append_to_response=images%2Cvideos&include_image_language=fr%2Cen%2Cnull&include_video_language=fr%2Cen%2Cnull&language=fr-FR"}) {
		t.Errorf("asked %q", got)
	}
	english, err := client.TitleOutline(t.Context(), Movie, 550, "en")
	if err != nil || english.TitledBackdrop != "/en.jpg" || english.Logo != "/logo-en.png" {
		t.Errorf("in English: %+v %v", english, err)
	}
	series, err := client.TitleOutline(t.Context(), TV, 1399, "")
	if err != nil || series.Runtime != 55 || series.Backdrop != "" || series.TitledBackdrop != "" || series.Trailer != "" || series.Genres == nil {
		t.Errorf("series %+v %v", series, err)
	}
	if _, err := client.TitleOutline(t.Context(), Movie, 1, "fr"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown title: %v", err)
	}
}

// A season's votes are those of its episodes; a series' next episode is
// read from its details, nil when none is scheduled.
func TestSeasonVotesAndTheNextEpisode(t *testing.T) {
	t.Parallel()
	client, _ := creditsServer(t, map[string]any{
		"/3/tv/1399/season/2": map[string]any{"episodes": []any{map[string]any{"episode_number": 1, "vote_average": 8.2},
			map[string]any{"episode_number": 2, "vote_average": 0}}},
		"/3/tv/1399": map[string]any{"next_episode_to_air": map[string]any{"air_date": "2026-12-01", "season_number": 3, "episode_number": 4,
			"name": " The Return "}},
		"/3/tv/1400": map[string]any{"next_episode_to_air": nil},
	})
	votes, err := client.SeasonVotes(t.Context(), 1399, 2)
	if err != nil || !slices.Equal(votes, []EpisodeVote{{1, 8.2}, {2, 0}}) {
		t.Errorf("votes %v %v", votes, err)
	}
	if _, err := client.SeasonVotes(t.Context(), 1399, 9); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown season: %v", err)
	}
	next, err := client.NextEpisode(t.Context(), 1399)
	if err != nil || next == nil || *next != (NextEpisode{AirDate: "2026-12-01", Season: 3, Episode: 4, Name: "The Return"}) {
		t.Errorf("next episode %+v %v", next, err)
	}
	if next, err := client.NextEpisode(t.Context(), 1400); next != nil || err != nil {
		t.Errorf("nothing scheduled: %+v %v", next, err)
	}
}
