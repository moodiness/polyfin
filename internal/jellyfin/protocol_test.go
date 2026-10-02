package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
