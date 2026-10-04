package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadCredentials(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name   string
		header map[string]string
		target string
		legacy bool
		want   credentials
	}{
		{
			name:   "quoted values in any key case",
			header: map[string]string{"Authorization": `MediaBrowser client="Infuse", DEVICE="Apple TV", DeviceId="abc", Version="8.1", Token="` + token + `"`},
			want:   credentials{Client: "Infuse", Device: "Apple TV", DeviceID: "abc", Version: "8.1", Token: token},
		},
		{
			name:   "unquoted values and a lower-case scheme",
			header: map[string]string{"Authorization": `mediabrowser Client=Swiftfin, Device=iPhone, DeviceId=x1, Version=1.2, Token=` + token},
			want:   credentials{Client: "Swiftfin", Device: "iPhone", DeviceID: "x1", Version: "1.2", Token: token},
		},
		{
			name:   "form-encoded values",
			header: map[string]string{"Authorization": `MediaBrowser Client="Jellyfin%20Web", Device="Living+room", DeviceId="a%2Cb", Version="10"`},
			want:   credentials{Client: "Jellyfin Web", Device: "Living room", DeviceID: "a,b", Version: "10"},
		},
		{
			name:   "ApiKey query parameter in any case",
			target: "/Users/Me?apikey=" + token,
			want:   credentials{Token: token},
		},
		{
			name:   "legacy forms refused by default",
			header: map[string]string{"X-Emby-Token": token, "X-Emby-Authorization": `MediaBrowser Token="` + token + `"`},
			target: "/Users/Me?api_key=" + token,
		},
		{
			name:   "Emby scheme refused by default",
			header: map[string]string{"Authorization": `Emby Token="` + token + `"`},
		},
		{
			name:   "Emby scheme accepted when legacy is enabled",
			header: map[string]string{"Authorization": `Emby Client="Kodi", Token="` + token + `"`},
			legacy: true,
			want:   credentials{Client: "Kodi", Token: token},
		},
		{
			name:   "api_key accepted when legacy is enabled",
			target: "/Users/Me?api_key=" + token,
			legacy: true,
			want:   credentials{Token: token},
		},
		{
			name:   "X-Emby-Token accepted when legacy is enabled",
			header: map[string]string{"X-Emby-Token": token},
			legacy: true,
			want:   credentials{Token: token},
		},
	} {
		target := tc.target
		if target == "" {
			target = "/Users/Me"
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		for key, value := range tc.header {
			r.Header.Set(key, value)
		}
		if got := readCredentials(r, tc.legacy); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestRouterMatchesLikeJellyfin(t *testing.T) {
	rt := &router{}
	respond := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(name + ":" + r.PathValue("userId")))
		})
	}
	rt.handle(http.MethodGet, "/Users/{userId}", respond("user"))
	rt.handle(http.MethodGet, "/Users/Me", respond("me"))
	rt.handle(http.MethodPost, "/Users/AuthenticateByName", respond("authenticate"))

	for _, tc := range []struct {
		method, target string
		status         int
		body           string
	}{
		{http.MethodGet, "/users/me", http.StatusOK, "me:"},
		{http.MethodGet, "/Users/Me/", http.StatusOK, "me:"},
		{http.MethodHead, "/Users/Me", http.StatusOK, "me:"},
		{http.MethodGet, "/USERS/AbCd", http.StatusOK, "user:AbCd"},
		// A literal segment beats a parameter whatever the method.
		{http.MethodGet, "/Users/AuthenticateByName", http.StatusMethodNotAllowed, ""},
		{http.MethodPost, "/users/authenticatebyname", http.StatusOK, "authenticate:"},
		{http.MethodDelete, "/Users/Me", http.StatusMethodNotAllowed, ""},
		{http.MethodGet, "/Nothing/Here", http.StatusNotFound, ""},
	} {
		recorder := httptest.NewRecorder()
		rt.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.target, nil))
		if recorder.Code != tc.status || recorder.Body.String() != tc.body {
			t.Errorf("%s %s: got %d %q, want %d %q", tc.method, tc.target, recorder.Code, recorder.Body, tc.status, tc.body)
		}
	}
}

func TestRouterLogsWhatItDoesNotServe(t *testing.T) {
	var output bytes.Buffer
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	unmatched := newUnmatchedRequests(slog.New(slog.NewJSONHandler(&output, nil)))
	unmatched.now = func() time.Time { return now }
	rt := &router{unmatched: unmatched}
	rt.handle(http.MethodGet, "/Users/Me", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	const secret = "0123456789abcdef0123456789abcdef"
	request := func(method, target, authorization string) {
		t.Helper()
		r := httptest.NewRequest(method, target, nil)
		if authorization != "" {
			r.Header.Set("Authorization", authorization)
		}
		rt.ServeHTTP(httptest.NewRecorder(), r)
	}
	type line struct {
		Level, Msg, Method, Path, Client, Version string
		Status                                    int
	}
	logged := func() []line {
		t.Helper()
		if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "Shrek") {
			t.Fatalf("the log shows a token or a query: %s", output.String())
		}
		var lines []line
		for _, raw := range strings.Split(strings.TrimSpace(output.String()), "\n") {
			if raw == "" {
				continue
			}
			var l line
			if err := json.Unmarshal([]byte(raw), &l); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, l)
		}
		output.Reset()
		return lines
	}

	swiftfin := `MediaBrowser Client="Swiftfin iOS", Device="iPhone", DeviceId="abc", Version="1.3.0", Token="` + secret + `"`
	request(http.MethodGet, "/Items/c0ffee00c0ffee00c0ffee00c0ffee00/Download?api_key="+secret+"&searchTerm=Shrek", swiftfin)
	request(http.MethodGet, "/Users/Me", swiftfin)
	request(http.MethodGet, "/Videos/c0ffee00-c0ff-ee00-c0ff-ee00c0ffee00/hls1/main/12.ts?ApiKey="+secret, "")
	request(http.MethodDelete, "/Users/Me", swiftfin)
	want := []line{
		{Level: "INFO", Msg: "An app asked for something Polyfin does not serve", Method: "GET", Path: "/Items/{id}/Download", Status: 404, Client: "Swiftfin iOS", Version: "1.3.0"},
		{Level: "INFO", Msg: "An app asked for something Polyfin does not serve", Method: "GET", Path: "/Videos/{id}/hls1/main/{id}.ts", Status: 404},
		{Level: "INFO", Msg: "An app asked for something Polyfin does not serve", Method: "DELETE", Path: "/Users/Me", Status: 405, Client: "Swiftfin iOS", Version: "1.3.0"},
	}
	if got := logged(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("logged %+v, want %+v", got, want)
	}

	// The same endpoint for another item, spelled in another case, is not
	// logged again within the hour; another method is.
	request(http.MethodGet, "/items/0000000000000000000000000000abcd/download", swiftfin)
	now = now.Add(59 * time.Minute)
	request(http.MethodGet, "/Items/c0ffee00c0ffee00c0ffee00c0ffee00/Download", swiftfin)
	request(http.MethodPost, "/Items/c0ffee00c0ffee00c0ffee00c0ffee00/Download", swiftfin)
	if got := logged(); len(got) != 1 || got[0].Method != "POST" {
		t.Fatalf("within the hour, logged %+v", got)
	}
	now = now.Add(2 * time.Minute)
	request(http.MethodGet, "/Items/c0ffee00c0ffee00c0ffee00c0ffee00/Download", swiftfin)
	if got := logged(); len(got) != 1 || got[0].Path != "/Items/{id}/Download" {
		t.Fatalf("an hour later, logged %+v", got)
	}

	// A scanner's distinct paths fill the bounded set, then go unlogged
	// until the hour passes.
	for i := range maxUnmatched {
		request(http.MethodGet, fmt.Sprintf("/probe/x%d", i), "")
	}
	if got := logged(); len(got) != maxUnmatched-2 {
		t.Fatalf("logged %d distinct paths, want %d", len(got), maxUnmatched-2)
	}
	request(http.MethodGet, "/Missing", "")
	if got := logged(); len(got) != 0 {
		t.Fatalf("beyond the bound, logged %+v", got)
	}
	now = now.Add(time.Hour)
	request(http.MethodGet, "/Missing", "")
	if got := logged(); len(got) != 1 {
		t.Fatalf("once the set expired, logged %+v", got)
	}
	if len(unmatched.logged) > maxUnmatched {
		t.Fatalf("remembers %d shapes", len(unmatched.logged))
	}
}

// Go accepts request lines and headers of about a megabyte: what an
// unmatched request keeps and logs stays short whatever it sends.
func TestRouterLogsLongRequestsShort(t *testing.T) {
	var output bytes.Buffer
	unmatched := newUnmatchedRequests(slog.New(slog.NewJSONHandler(&output, nil)))
	rt := &router{unmatched: unmatched}
	long := strings.Repeat("a", maxLogged-1) + "é" + strings.Repeat("b", 1<<20)
	for i := range 3 {
		r := httptest.NewRequest(http.MethodGet, "/"+long+fmt.Sprint(i), nil)
		r.Method = "M" + long
		r.Header.Set("Authorization", `MediaBrowser Client="`+long+`", Version="`+long+`"`)
		rt.ServeHTTP(httptest.NewRecorder(), r)
	}
	if output.Len() > 4*maxLogged+1000 {
		t.Fatalf("logged %d bytes", output.Len())
	}
	var line struct{ Method, Path, Client, Version string }
	if err := json.Unmarshal(bytes.SplitN(output.Bytes(), []byte("\n"), 2)[0], &line); err != nil {
		t.Fatal(err)
	}
	if want := "/" + strings.Repeat("a", maxLogged-1) + "…"; line.Path != want {
		t.Errorf("path %q, want %q", line.Path, want)
	}
	// The client's and version's cut falls inside "é", which is dropped
	// whole.
	if line.Client != strings.Repeat("a", maxLogged-1)+"…" || line.Version != line.Client || len(line.Method) > maxLogged+len("…") {
		t.Errorf("logged %+v", line)
	}
	// Paths that differ only past the cut are one shape.
	if len(unmatched.logged) != 1 {
		t.Errorf("remembers %d shapes", len(unmatched.logged))
	}
	for key := range unmatched.logged {
		if len(key) > 2*(maxLogged+len("…"))+1 {
			t.Errorf("remembers a key of %d bytes", len(key))
		}
	}
}

func TestPathShape(t *testing.T) {
	for path, want := range map[string]string{
		"/Items/0123456789ABCDEF0123456789abcdef/Images/Primary/0": "/Items/{id}/Images/Primary/{id}",
		"/Users/01234567-89ab-cdef-0123-456789abcdef/Items":        "/Users/{id}/Items",
		"/Videos/0123456789abcdef0123456789abcdef/stream.mkv":      "/Videos/{id}/stream.mkv",
		"/Audio/0123456789abcdef0123456789abcdef.mp3":              "/Audio/{id}.mp3",
		"/Videos/abc/hls1/main/0.ts":                               "/Videos/abc/hls1/main/{id}.ts",
		"/web/index.html":                                          "/web/index.html",
		// Almost identifiers stay: they are endpoint names.
		"/Items/0123456789abcdef0123456789abcdeg":       "/Items/0123456789abcdef0123456789abcdeg",
		"/Items/0123-4567-89ab-cdef-0123456789abcdef-0": "/Items/0123-4567-89ab-cdef-0123456789abcdef-0",
	} {
		if got := pathShape(path); got != want {
			t.Errorf("pathShape(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCrossOriginRequests(t *testing.T) {
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))

	preflight := httptest.NewRequest(http.MethodOptions, "/Users/AuthenticateByName", nil)
	preflight.Header.Set("Origin", "https://app.example")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, preflight)
	if recorder.Code != http.StatusNoContent ||
		recorder.Header().Get("Access-Control-Allow-Origin") != "*" ||
		recorder.Header().Get("Access-Control-Allow-Methods") != "POST" ||
		recorder.Header().Get("Access-Control-Allow-Headers") != "authorization,content-type" {
		t.Errorf("preflight: %d %v", recorder.Code, recorder.Header())
	}

	request := httptest.NewRequest(http.MethodGet, "/System/Info/Public", nil)
	request.Header.Set("Origin", "https://app.example")
	recorder = httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTeapot || recorder.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("cross-origin request: %d %v", recorder.Code, recorder.Header())
	}
}
