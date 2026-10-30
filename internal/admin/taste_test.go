package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/taste"
	"github.com/moodiness/polyfin/internal/userdata"
)

// genreTitles describes every title as a movie of the genre its first byte
// names.
type genreTitles map[byte]string

func (g genreTitles) ItemsOf(_ context.Context, _ accounts.User, ids []accounts.ID, _ []library.Kind) ([]library.Item, error) {
	var items []library.Item
	for _, id := range ids {
		items = append(items, library.Item{ID: id, Kind: library.KindMovie, Genres: []string{g[id[0]]}})
	}
	return items, nil
}

// facetOf returns the liked and disliked names of a facet of an answer.
func facetOf(answer map[string]any, facet string) (liked, disliked []string) {
	facets, _ := answer["facets"].([]any)
	for _, f := range facets {
		entry := f.(map[string]any)
		if entry["facet"] != facet {
			continue
		}
		for _, l := range entry["liked"].([]any) {
			liked = append(liked, l.(map[string]any)["name"].(string))
		}
		for _, d := range entry["disliked"].([]any) {
			name := d.(map[string]any)["name"].(string)
			if d.(map[string]any)["vetoed"] == true {
				name += " (vetoed)"
			}
			disliked = append(disliked, name)
		}
	}
	return liked, disliked
}

// My account shows a user their own taste profile, nobody else's, and
// turns their taglines off and on.
func TestOwnTasteProfile(t *testing.T) {
	t.Parallel()
	var tastes *taste.Service
	var data *userdata.Store
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		data = userdata.New(d.pool)
		tastes = taste.New(d.pool, o.Accounts, data, genreTitles{1: "Thriller", 2: "Horror"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		o.Taste = tastes
	})
	viewer := api.signedIn("viewer", false)
	other := api.signedIn("other", false)
	status, answer, _ := viewer.call(http.MethodGet, "/account/taste", nil)
	if status != http.StatusOK || answer["built"] != nil || answer["confident"] != false || answer["taglines"] != true {
		t.Fatalf("before a build: %d %v", status, answer)
	}
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	user := users[slices.IndexFunc(users, func(u accounts.User) bool { return u.Name == "viewer" })]
	now := time.Now()
	for i := range 3 {
		if _, err := data.Change(t.Context(), user.ID, []userdata.Item{{ID: accounts.ID{1, byte(i)}}}, func(d *userdata.Data) {
			d.Played, d.PlayCount, d.LastPlayed = true, 1, &now
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := data.Change(t.Context(), user.ID, []userdata.Item{{ID: accounts.ID{2}}, {ID: accounts.ID{2, 1}}}, func(d *userdata.Data) { d.Like(false) }); err != nil {
		t.Fatal(err)
	}
	if err := tastes.BuildAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, answer, _ = viewer.call(http.MethodGet, "/account/taste", nil)
	liked, disliked := facetOf(answer, "genre")
	if status != http.StatusOK || answer["built"] == nil || answer["confident"] != true || answer["titles"] != 5.0 ||
		!slices.Equal(liked, []string{"Thriller"}) || !slices.Equal(disliked, []string{"Horror (vetoed)"}) {
		t.Errorf("after a build: %d %v", status, answer)
	}
	if status, answer, _ := other.call(http.MethodGet, "/account/taste", nil); status != http.StatusOK || answer["titles"] != 0.0 || answer["confident"] != false {
		t.Errorf("another user's: %d %v", status, answer)
	}
	if status, answer, _ := viewer.call(http.MethodPut, "/account/taste", map[string]any{"taglines": false}); status != http.StatusOK ||
		answer["taglines"] != false || answer["confident"] != true {
		t.Errorf("taglines off: %d %v", status, answer)
	}
	if status, _, _ := viewer.call(http.MethodPut, "/account/taste", map[string]any{}); status != http.StatusBadRequest {
		t.Errorf("nothing to change: %d", status)
	}
	if on, err := tastes.Taglines(t.Context(), user.ID); err != nil || on {
		t.Errorf("taglines kept: %v %v", on, err)
	}
}
