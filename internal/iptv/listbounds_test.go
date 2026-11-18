package iptv

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// answeringXtream serves an Xtream account whose player API answers each
// action with its body, the login being the action "".
func answeringXtream(t *testing.T, actions map[string]string) (requester, Account) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := actions[r.URL.Query().Get("action")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return requester{client: stremio.NewClient("test"), pacer: NewPacer(0)},
		Account{Kind: addons.KindXtream, Server: server.URL + "/", Username: "user", Password: "pass"}
}

// categories is a category list of count categories, numbered from 1.
func categories(count int) string {
	var list strings.Builder
	list.WriteByte('[')
	for i := range count {
		if i > 0 {
			list.WriteByte(',')
		}
		fmt.Fprintf(&list, `{"category_id":"%d","category_name":" Group %d "}`, i+1, i+1)
	}
	list.WriteByte(']')
	return list.String()
}

// An Xtream category list is read one category at a time, up to
// maxCategories: a longer one is too large, whatever its elements hold,
// and one of that many is read whole. A list that is null is one of no
// category, and anything else is no list, as before.
func TestAnXtreamCategoryListIsBoundedInCategories(t *testing.T) {
	t.Parallel()
	client, account := answeringXtream(t, map[string]string{
		"at":      categories(maxCategories),
		"past":    categories(maxCategories + 1),
		"empties": "[" + strings.Repeat("{},", 4*maxCategories) + "{}]",
		"null":    "null",
		"object":  `{"category_id":"1"}`,
		"number":  `[5]`,
	})
	names, err := xtreamCategories(t.Context(), client, account, false, "at")
	if err != nil || len(names) != maxCategories || names["1"] != "Group 1" || names[text(fmt.Sprint(maxCategories))] != fmt.Sprintf("Group %d", maxCategories) {
		t.Errorf("a list of %d categories: %d read, %v", maxCategories, len(names), err)
	}
	for _, action := range []string{"past", "empties"} {
		if names, err := xtreamCategories(t.Context(), client, account, false, action); !errors.Is(err, ErrTooLarge) || names != nil {
			t.Errorf("%s: %d categories, %v; want ErrTooLarge", action, len(names), err)
		}
	}
	if names, err := xtreamCategories(t.Context(), client, account, false, "null"); err != nil || names == nil || len(names) != 0 {
		t.Errorf("null: %v, %v; want no category", names, err)
	}
	for _, action := range []string{"object", "number"} {
		if _, err := xtreamCategories(t.Context(), client, account, false, action); !errors.Is(err, ErrInvalidList) {
			t.Errorf("%s: %v, want ErrInvalidList", action, err)
		}
	}
}

// An Xtream login answer holds a few fields: one past maxLoginSize is too
// large, rather than decoded whole within what a list may weigh.
func TestAnXtreamLoginAnswerIsBoundedInBytes(t *testing.T) {
	t.Parallel()
	formats := `"ts"` + strings.Repeat(`,""`, maxLoginSize/3)
	client, account := answeringXtream(t, map[string]string{
		"":                    `{"user_info":{"auth":1,"allowed_output_formats":[` + formats + `]}}`,
		"get_live_categories": `[]`,
		"get_live_streams":    `[]`,
	})
	if _, err := fetchXtream(t.Context(), client, account, false, livePart); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a login answer of %d bytes: %v, want ErrTooLarge", len(formats), err)
	}
	client, account = answeringXtream(t, map[string]string{
		"":                    `{"user_info":{"auth":1,"allowed_output_formats":["ts"` + strings.Repeat(`,"m3u8"`, 1000) + `]}}`,
		"get_live_categories": `[]`,
		"get_live_streams":    `[{"num":1,"name":"Zeb One","stream_id":101}]`,
	})
	if d, err := fetchXtream(t.Context(), client, account, false, livePart); err != nil || len(d.entries) != 1 {
		t.Errorf("an ordinary login answer: %d channels, %v", len(d.entries), err)
	}
}

// An entry keeps the first maxProperties #KODIPROP lines since the
// previous address: those past them are skipped, however many come, with
// or without an entry to take them, and the next entry reads its own.
func TestKodiPropertiesPastTheirBoundAreSkipped(t *testing.T) {
	t.Parallel()
	const typed = "#KODIPROP:inputstream.adaptive.manifest_type=mpd\n"
	entries, err := parse(t, "#EXTM3U\n"+
		"#EXTINF:-1,Zeb Past\n"+strings.Repeat("#KODIPROP:other=1\n", maxProperties)+typed+"https://live.example/zeb.mpd\n"+
		"#EXTINF:-1,Orbe Within\n"+strings.Repeat("#KODIPROP:other=1\n", maxProperties-1)+typed+"https://live.example/orbe.mpd\n"+
		strings.Repeat("#KODIPROP:\n", 100*maxProperties)+"https://live.example/none.mpd\n"+
		"#EXTINF:-1,Plume After\n"+typed+"https://live.example/plume.mpd\n")
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	if entries[0].Protection != nil {
		t.Errorf("a property past the %d read was read: %+v", maxProperties, entries[0].Protection)
	}
	for _, entry := range entries[1:] {
		if entry.Protection == nil || entry.Protection.Manifest != "mpd" {
			t.Errorf("%s: its property was not read: %+v", entry.Name, entry.Protection)
		}
	}
}
