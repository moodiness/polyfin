package iptv

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// xtreamServer is an Xtream Codes server with one account, user / p@ss
// word, whose formats are given; requests counts the player API calls.
type xtreamServer struct {
	url      string
	formats  []string
	requests atomic.Int32
}

func newXtreamServer(t *testing.T, formats ...string) *xtreamServer {
	t.Helper()
	x := &xtreamServer{formats: formats}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player_api.php" {
			http.NotFound(w, r)
			return
		}
		x.requests.Add(1)
		query := r.URL.Query()
		if query.Get("username") != "user" || query.Get("password") != "p@ss word" {
			_ = json.NewEncoder(w).Encode(map[string]any{"user_info": map[string]any{"auth": 0}})
			return
		}
		switch query.Get("action") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_info": map[string]any{"auth": 1, "status": "Active",
				"allowed_output_formats": x.formats}, "server_info": map[string]any{"url": "elsewhere.example"}})
		case "get_live_categories":
			_, _ = w.Write([]byte(`[{"category_id": "1", "category_name": "News"}, {"category_id": 2, "category_name": "Kids"}]`))
		case "get_live_streams":
			_, _ = w.Write([]byte(`[
				{"num": 1, "name": "ZZ| Zeb One", "stream_id": 101, "stream_icon": "https://img.example/zeb.png", "epg_channel_id": "zeb.zz", "category_id": "1"},
				{"num": "2", "name": "##### KIDS #####", "stream_id": 102, "category_id": "2"},
				{"num": 3, "name": "Orbe Junior", "stream_id": "103", "stream_icon": "", "epg_channel_id": null, "category_id": 2},
				{"num": 4, "name": "No Id", "category_id": "2"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	x.url = server.URL
	return x
}

func (x *xtreamServer) account(password string) Account {
	return Account{Kind: addons.KindXtream, Server: x.url + "/", Username: "user", Password: password}
}

func TestXtreamAccountsAreRead(t *testing.T) {
	client := requester{client: stremio.NewClient("test"), pacer: NewPacer(0)}
	for _, tc := range []struct {
		formats   []string
		extension string
	}{
		{[]string{"m3u8", "ts", "rtmp"}, ".ts"},
		{[]string{"m3u8"}, ".m3u8"},
		{nil, ".ts"},
	} {
		x := newXtreamServer(t, tc.formats...)
		account := x.account("p@ss word")
		address, err := account.address()
		if err != nil {
			t.Fatal(err)
		}
		list, err := fetch(t.Context(), client, accountOf(addons.KindXtream, address), false, livePart)
		entries := list.entries
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("channels: %+v", entries)
		}
		zeb, orbe := entries[0], entries[1]
		if zeb.ID != "101" || zeb.Name != "ZZ| Zeb One" || zeb.Number != 1 || zeb.Group != "News" || zeb.GuideID != "zeb.zz" ||
			zeb.Logo != "https://img.example/zeb.png" || zeb.URL != x.url+"/live/user/p@ss%20word/101"+tc.extension {
			t.Errorf("formats %v: %+v", tc.formats, zeb)
		}
		if orbe.ID != "103" || orbe.Group != "Kids" || orbe.Number != 3 || orbe.GuideID != "" {
			t.Errorf("formats %v: %+v", tc.formats, orbe)
		}
	}
	x := newXtreamServer(t, "ts")
	address, _ := x.account("wrong").address()
	if _, err := fetch(t.Context(), client, accountOf(addons.KindXtream, address), false, livePart); !errors.Is(err, ErrLoginRefused) {
		t.Errorf("a refused account: %v", err)
	}
	if _, err := fetch(t.Context(), client, accountOf(addons.KindXtream, address), true, livePart); !errors.Is(err, stremio.ErrPrivateNetwork) {
		t.Errorf("a confined source on this machine: %v", err)
	}
}

func TestAccountsAndAddresses(t *testing.T) {
	account := Account{Kind: addons.KindXtream, Server: " https://tv.example:8080/ ", Username: "user", Password: "p@ss&word"}
	address, err := account.address()
	if err != nil {
		t.Fatal(err)
	}
	back := accountOf(addons.KindXtream, address)
	if back.Server != "https://tv.example:8080" || back.Username != "user" || back.Password != "p@ss&word" {
		t.Errorf("account read back: %+v", back)
	}
	if guide := ProviderGuide(back); guide != "https://tv.example:8080/xmltv.php?password=p%40ss%26word&username=user" {
		t.Errorf("provider guide: %s", guide)
	}
	if redacted := Redact(address); redacted != "https://tv.example:8080/…" || strings.Contains(redacted, "user") {
		t.Errorf("redacted: %s", redacted)
	}
	for _, bad := range []Account{
		{Kind: addons.KindM3U, URL: "ftp://tv.example/list.m3u"},
		{Kind: addons.KindM3U, URL: "list.m3u"},
		{Kind: addons.KindXtream, Server: "https://tv.example", Username: "user"},
		{Kind: addons.KindXtream, Server: "https://tv.example?x=1", Username: "user", Password: "pw"},
		{Kind: "other", URL: "https://tv.example/list.m3u"},
	} {
		if _, err := bad.address(); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}
