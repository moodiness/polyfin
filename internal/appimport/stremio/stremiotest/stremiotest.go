// Package stremiotest serves a fake Stremio account API and link service
// for tests: the password sign-in, the link codes, the library, the addons
// and the sign-out, as Stremio's own client code calls them.
package stremiotest

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	addon "github.com/moodiness/polyfin/internal/stremio"
)

// Request is a request the fake received: on the API, or the link service
// when Link is set.
type Request struct {
	Link                bool
	Method, Path, Query string
	// AuthKey is the auth key the body holds, Body the whole body.
	AuthKey, Body string
}

// Fake is a fake account API, at API, and link service, at Link, with one
// account.
type Fake struct {
	API, Link string

	mu sync.Mutex
	// Email and Password sign in to the account, which holds Library, the
	// library items, and Addons, the addon descriptors.
	Email, Password string
	Library, Addons any
	// Pasted is an auth key of the account the user's browser holds.
	Pasted   string
	requests []Request
	// keys are the auth keys of the sessions open; codes the link codes
	// asked for, and whether each is approved.
	keys     map[string]bool
	codes    map[string]bool
	lastCode string
	issued   int
}

// New serves a fake account API and link service until the test ends.
func New(t *testing.T) *Fake {
	t.Helper()
	f := &Fake{keys: map[string]bool{}, codes: map[string]bool{}, Pasted: "pasted-key-0123456789abcdef"}
	api := httptest.NewServer(http.HandlerFunc(f.serveAPI))
	t.Cleanup(api.Close)
	link := httptest.NewServer(http.HandlerFunc(f.serveLink))
	t.Cleanup(link.Close)
	f.API, f.Link = api.URL, link.URL
	return f
}

// Set changes the fake under its lock.
func (f *Fake) Set(change func(*Fake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f)
}

// Requests lists the requests the fake received.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// Open counts the sessions opened by a sign-in or a link code, and not
// signed out.
func (f *Fake) Open() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

// Approve approves the last link code asked for, as the user does in a
// browser signed in to Stremio.
func (f *Fake) Approve() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes[f.lastCode] = true
}

// issue opens a session, and returns its auth key.
func (f *Fake) issue() string {
	f.issued++
	key := "issued-key-" + strconv.Itoa(f.issued) + "-0123456789abcdef"
	f.keys[key] = true
	return key
}

func (f *Fake) record(r *http.Request, link bool) []byte {
	body, _ := io.ReadAll(r.Body)
	var auth struct {
		AuthKey string `json:"authKey"`
	}
	_ = json.Unmarshal(body, &auth)
	f.requests = append(f.requests, Request{Link: link, Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, AuthKey: auth.AuthKey,
		Body: string(body)})
	return body
}

func result(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"result": value})
}

// failure answers an error as the API does: with its envelope, status
// 200.
func failure(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message}})
}

func (f *Fake) serveAPI(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := f.record(r, false)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var request struct {
		AuthKey    string   `json:"authKey"`
		Email      string   `json:"email"`
		Password   string   `json:"password"`
		Collection string   `json:"collection"`
		IDs        []string `json:"ids"`
		All        bool     `json:"all"`
	}
	if json.Unmarshal(body, &request) != nil {
		failure(w, 1, "Invalid request")
		return
	}
	if r.URL.Path == "/api/login" {
		if request.Email != f.Email || request.Password != f.Password {
			failure(w, 3, "Wrong passphrase")
			return
		}
		result(w, map[string]any{"authKey": f.issue(), "user": map[string]any{"_id": "user", "email": f.Email}})
		return
	}
	if !f.keys[request.AuthKey] && (f.Pasted == "" || request.AuthKey != f.Pasted) {
		failure(w, 1, "Session not found")
		return
	}
	switch r.URL.Path {
	case "/api/datastoreGet":
		if request.Collection != "libraryItem" || !request.All || request.IDs == nil {
			failure(w, 1, "Invalid request")
			return
		}
		result(w, f.Library)
	case "/api/addonCollectionGet":
		result(w, map[string]any{"addons": f.Addons, "lastModified": "2026-10-01T10:00:00.000Z"})
	case "/api/logout":
		delete(f.keys, request.AuthKey)
		if request.AuthKey == f.Pasted {
			f.Pasted = ""
		}
		result(w, map[string]any{"success": true})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *Fake) serveLink(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(r, true)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v2/create" && r.URL.Query().Get("type") == "Create":
		f.issued++
		code := "C" + strconv.Itoa(f.issued)
		f.codes[code], f.lastCode = false, code
		result(w, map[string]any{"success": true, "code": code, "link": f.Link + "/" + code, "qrcode": f.Link + "/qr?data=" + code})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v2/read" && r.URL.Query().Get("type") == "Read":
		code := r.URL.Query().Get("code")
		approved, known := f.codes[code]
		switch {
		case !known:
			failure(w, 1, "Code not found")
		case !approved:
			failure(w, 1, "No data yet")
		default:
			delete(f.codes, code)
			result(w, map[string]any{"authKey": f.issue()})
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Watched is the watched field of a series, anchored on the video anchor
// at place length-1, telling the videos at the places set in bits watched.
func Watched(anchor string, length int, bits ...int) string {
	var raw []byte
	for _, place := range bits {
		for len(raw) <= place/8 {
			raw = append(raw, 0)
		}
		raw[place/8] |= 1 << (place % 8)
	}
	var compressed bytes.Buffer
	writer, _ := zlib.NewWriterLevel(&compressed, 6)
	_, _ = writer.Write(raw)
	_ = writer.Close()
	return anchor + ":" + strconv.Itoa(length) + ":" + base64.StdEncoding.EncodeToString(compressed.Bytes())
}

// Metas describes series by their identifiers, as the server's addons do
// (see library.Service.TitleMeta).
type Metas map[string]addon.Meta

func (m Metas) TitleMeta(_ context.Context, metaType, id string) (addon.Meta, bool) {
	meta, ok := m[id]
	return meta, ok && meta.Type == metaType
}

// SampleMetas describes the series tt0100 of Sample, with the episodes 1
// to 3 of its first season and the first of its second, in another order.
func SampleMetas() Metas {
	return Metas{"tt0100": {ID: "tt0100", Type: "series", Name: "Series", Videos: []addon.Video{
		{ID: "tt0100:2:1", Season: 2, Episode: 1, Released: "2026-02-01T00:00:00.000Z"},
		{ID: "tt0100:1:1", Season: 1, Episode: 1, Released: "2026-01-01T00:00:00.000Z"},
		{ID: "tt0100:1:2", Season: 1, Episode: 2, Released: "2026-01-08T00:00:00.000Z"},
		{ID: "tt0100:1:3", Season: 1, Episode: 3, Released: "2026-01-15T00:00:00.000Z"},
	}}}
}

// Sample is the library of an account, read against SampleMetas:
//   - the movie tt0000001, in the library, watched twice, last on
//     2026-09-01 at 20:00;
//   - the movie tt0000002, played without being added, stopped at 30 of
//     100 minutes on 2026-10-01 at 20:00;
//   - the movie tt0000003, removed from the library, watched most of the
//     way, with a resume point which Continue Watching no longer shows,
//     changed last on 2026-09-02 at 20:00;
//   - the series tt0100, in the library, its first three episodes watched
//     on 2026-09-04 at 20:00, and S2E1 stopped at 5 of 50 minutes then;
//   - the series tt0200, played without being added, watched against an
//     episode no addon lists;
//   - the series custom:1 ("Home video"), of an addon of its own, in the
//     library;
//   - the movie tmdb:999999 ("Unknown"), which no catalog lists, watched;
//   - an older copy of tt0000001, a channel, a TV channel, another video,
//     and an item Stremio's apps would not read, all left out.
func Sample() []any {
	type state = map[string]any
	return []any{
		map[string]any{"_id": "tt0000001", "type": "movie", "name": "First movie", "removed": false, "temp": false,
			"_ctime": "2026-08-01T20:00:00.000Z", "_mtime": "2026-09-01T22:00:00.000Z",
			"state": state{"lastWatched": "2026-09-01T20:00:00.000Z", "timesWatched": 2, "flaggedWatched": 1, "timeOffset": 0,
				"duration": 6_000_000, "video_id": "tt0000001", "watched": nil}},
		map[string]any{"_id": "tt0000002", "type": "movie", "name": "Second movie", "removed": true, "temp": true,
			"_mtime": "2026-10-01T20:00:00.000Z",
			"state": state{"lastWatched": "2026-10-01T20:00:00.000Z", "timesWatched": 0, "flaggedWatched": 0, "timeOffset": 30 * 60_000,
				"duration": 100 * 60_000, "video_id": "tt0000002"}},
		map[string]any{"_id": "tt0000003", "type": "movie", "name": "Removed movie", "removed": true, "temp": false,
			"_mtime": "2026-09-02T20:00:00.000Z",
			"state": state{"lastWatched": nil, "timesWatched": 0, "flaggedWatched": 1, "timeOffset": 80 * 60_000,
				"duration": 100 * 60_000}},
		map[string]any{"_id": "tt0100", "type": "series", "name": "Series", "removed": false, "temp": false,
			"_mtime": "2026-09-04T20:00:00.000Z",
			"state": state{"lastWatched": "2026-09-04T20:00:00.000Z", "timesWatched": 3, "timeOffset": 5 * 60_000,
				"duration": 50 * 60_000, "video_id": "tt0100:2:1", "watched": Watched("tt0100:1:3", 3, 0, 1, 2), "noNotif": false}},
		map[string]any{"_id": "tt0200", "type": "series", "name": "Unlisted", "removed": true, "temp": true,
			"_mtime": "2026-09-05T20:00:00.000Z",
			"state": state{"lastWatched": "2026-09-05T20:00:00.000Z", "timesWatched": 1, "video_id": "tt0200:1:9",
				"watched": Watched("tt0200:1:9", 9, 8)}},
		map[string]any{"_id": "custom:1", "type": "series", "name": "Home video", "removed": false, "temp": false,
			"_mtime": "2026-09-06T20:00:00.000Z", "state": state{}},
		map[string]any{"_id": "tmdb:999999", "type": "movie", "name": "Unknown", "removed": true, "temp": true,
			"_mtime": "2026-09-07T20:00:00.000Z", "state": state{"lastWatched": "2026-09-07T20:00:00.000Z", "timesWatched": 1}},
		map[string]any{"_id": "tt0000001", "type": "movie", "name": "First movie", "removed": true, "temp": true,
			"_mtime": "2026-01-01T20:00:00.000Z", "state": state{"timesWatched": 0}},
		map[string]any{"_id": "channel:1", "type": "channel", "name": "A channel", "removed": false, "temp": false, "state": state{}},
		map[string]any{"_id": "tv:1", "type": "tv", "name": "A TV channel", "removed": false, "temp": false, "state": state{}},
		map[string]any{"_id": "local:1", "type": "other", "name": "A video", "removed": false, "temp": false,
			"state": state{"timesWatched": 1}},
		map[string]any{"_id": 12, "type": "movie"},
	}
}

// SampleAddons are the addons of an account: Stremio's own, which comes
// with it, and those whose manifests are at first and second, first
// installed twice.
func SampleAddons(first, second string) []any {
	descriptor := func(id, name, transport string, official bool) map[string]any {
		return map[string]any{"manifest": map[string]any{"id": id, "name": name, "version": "1.0.0"}, "transportUrl": transport,
			"flags": map[string]any{"official": official, "protected": official}}
	}
	return []any{
		descriptor("org.example.builtin", "Built in", "https://builtin.example/manifest.json", true),
		descriptor("org.example.first", "First", first, false),
		descriptor("org.example.second", "Second", second, false),
		descriptor("org.example.first", "First", first, false),
	}
}
