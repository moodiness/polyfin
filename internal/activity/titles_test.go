package activity

import (
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// An entry about a title tells where its name holds the title, in
// characters, in each language and wherever its template puts the title;
// the other entries, and a name clipped, tell none.
func TestEntriesTellWhereTheirTitleIs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var language atomic.Value
	language.Store("en")
	store := New(migrated.New(t), func() accounts.Settings { return accounts.Settings{Language: language.Load().(string)} },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	user := accounts.User{ID: accounts.ID{1}, Name: "Zoé 🎬"}
	item := accounts.ID{2}
	link := accounts.ShareLink{User: accounts.ShareLinkUser{ID: user.ID, Name: user.Name}, Item: item, Title: "Amélie (2001)"}

	store.PlaybackStarted(ctx, user, "Glass Garden, season 1, episode 4, In Bloom", item.String(), "TV")
	language.Store("fr")
	store.PlaybackStopped(ctx, user, "Amélie", item.String(), "Télé du salon")
	language.Store("de")
	store.SharedPlayed(ctx, link, "192.0.2.1")
	language.Store("en")
	store.ShareLinkRevokedBy(ctx, link, accounts.User{ID: accounts.ID{3}, Name: "root"})
	store.AudioStarted(ctx, user, "", item.String(), "TV")
	store.SignedIn(ctx, user, "192.0.2.1")
	store.PlaybackStarted(ctx, user, strings.Repeat("Long ", 300), item.String(), "TV")

	page, err := store.Entries(ctx, Query{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", "", "", "Amélie (2001)", "Amélie (2001)", "Amélie", "Glass Garden, season 1, episode 4, In Bloom"}
	if len(page.Entries) != len(want) {
		t.Fatalf("%d entries", len(page.Entries))
	}
	for i, e := range page.Entries {
		if (e.TitleStart == nil) != (e.TitleEnd == nil) {
			t.Fatalf("%q: half a title", e.Name)
		}
		got := ""
		if e.TitleStart != nil {
			got = string([]rune(e.Name)[*e.TitleStart:*e.TitleEnd])
		}
		if got != want[i] {
			t.Errorf("%q: title %q, want %q", e.Name, got, want[i])
		}
	}
	if name := page.Entries[4].Name; !strings.HasPrefix(name, "Ein Gast hat Amélie (2001) über") {
		t.Errorf("shared playback: %q", name)
	}
}
