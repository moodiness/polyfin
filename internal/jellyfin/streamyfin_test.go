package jellyfin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/streamyfin"
)

// streamyfinAnswer is what Streamyfin reads of /Streamyfin/config.
type streamyfinAnswer struct {
	Settings struct {
		Home struct {
			Locked bool `json:"locked"`
			Value  struct {
				Sections []struct {
					Title       string `json:"title"`
					Orientation string `json:"orientation"`
					Items       *struct {
						ParentID         string   `json:"parentId"`
						IncludeItemTypes []string `json:"includeItemTypes"`
					} `json:"items"`
					NextUp *struct{} `json:"nextUp"`
					Custom *struct {
						Endpoint string `json:"endpoint"`
					} `json:"custom"`
				} `json:"sections"`
			} `json:"value"`
		} `json:"home"`
	} `json:"settings"`
}

// Streamyfin asks for the settings of its server plugin: Polyfin answers
// the rows an administrator chose as its locked home screen, only those of
// the libraries and collections each user sees, and 404 without any, as a
// server without the plugin, so that Streamyfin shows its own home screen.
// Each row's listing answers what it shows.
func TestStreamyfinGetsTheHomeRows(t *testing.T) {
	var rows *streamyfin.Store
	s, token, ids := browsingOn(t, newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		rows = streamyfin.New(pool)
		o.Streamyfin = rows
	}))
	config := func(token string) (int, streamyfinAnswer) {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Streamyfin/config", app("streamyfin", token), nil)
		var answer streamyfinAnswer
		if status == http.StatusOK {
			if err := json.Unmarshal(body, &answer); err != nil {
				t.Fatalf("answer: %v %s", err, body)
			}
		}
		return status, answer
	}
	if status, _ := config(token); status != http.StatusNotFound {
		t.Errorf("without rows: %d", status)
	}

	var groups QueryResult
	s.get(t, "/Items?ParentId="+ids["Groups"], token, &groups)
	if len(groups.Items) != 1 {
		t.Fatalf("the collections: %+v", groups.Items)
	}
	group := groups.Items[0]
	id := func(raw string) accounts.ID {
		parsed, err := accounts.ParseID(raw)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	if err := rows.SetRows(t.Context(), []streamyfin.Row{
		{Kind: streamyfin.KindResume},
		{Kind: streamyfin.KindNextUp},
		{Kind: streamyfin.KindLibrary, Item: id(ids["Groups"])},
		{Kind: streamyfin.KindLibrary, Item: id(ids["Top"]), Title: "Popular now"},
		{Kind: streamyfin.KindCollection, Item: id(group.Id)},
	}); err != nil {
		t.Fatal(err)
	}
	status, answer := config(token)
	home := answer.Settings.Home
	var got []string
	for _, section := range home.Value.Sections {
		var kind string
		switch {
		case section.Custom != nil:
			kind = "custom " + section.Custom.Endpoint
		case section.NextUp != nil:
			kind = "next up"
		case section.Items != nil:
			kind = "items of " + section.Items.ParentID + " " + strings.Join(section.Items.IncludeItemTypes, ",")
		}
		got = append(got, section.Title+" | "+section.Orientation+" | "+kind)
	}
	want := []string{
		"Continue Watching | horizontal | custom /UserItems/Resume",
		"Next Up | horizontal | next up",
		"Groups | horizontal | items of " + ids["Groups"] + " ",
		"Popular now | vertical | items of " + ids["Top"] + " ",
		"Group | vertical | items of " + group.Id + " Movie,Series",
	}
	if status != http.StatusOK || !home.Locked || strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("config: %d, locked %v\n%s\nwant\n%s", status, home.Locked, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Each row's listing, as Streamyfin asks for it: recursive, in pages.
	listed := func(query string) []string {
		t.Helper()
		var result QueryResult
		if status := s.get(t, "/Items?recursive=true&startIndex=0&limit=10&"+query, token, &result); status != http.StatusOK {
			t.Fatalf("%s: %d", query, status)
		}
		var types []string
		for _, item := range result.Items {
			types = append(types, item.Type)
		}
		return types
	}
	if types := listed("parentId=" + ids["Groups"]); strings.Join(types, ",") != "BoxSet" {
		t.Errorf("the Groups row: %v", types)
	}
	if types := listed("parentId=" + group.Id + "&includeItemTypes=Movie&includeItemTypes=Series"); len(types) == 0 ||
		strings.Trim(strings.ReplaceAll(strings.Join(types, ","), "Movie", ""), ",") != "" {
		t.Errorf("the Group row: %v", types)
	}

	// A user who hides Top gets every row but its own.
	s.user("kid", func(c *accounts.UserChanges) { c.HiddenLibraries = &[]accounts.ID{id(ids["Top"])} })
	kidToken := s.signIn("kid", "streamyfin")
	if _, answer := config(kidToken); len(answer.Settings.Home.Value.Sections) != 4 {
		t.Errorf("a user who hides Top: %+v", answer.Settings.Home.Value.Sections)
	}
	// With only rows they cannot see, Streamyfin shows its own home screen.
	if err := rows.SetRows(t.Context(), []streamyfin.Row{{Kind: streamyfin.KindLibrary, Item: id(ids["Top"])}}); err != nil {
		t.Fatal(err)
	}
	if status, _ := config(kidToken); status != http.StatusNotFound {
		t.Errorf("a user who sees none of the rows: %d", status)
	}
	if status, _ := config(token); status != http.StatusOK {
		t.Errorf("a user who sees the row: %d", status)
	}
}
