package updatetool

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The request is a POST to the address as given, path and query
// included, without a body, with the token as a bearer token; the answer
// is kept whatever its status, its start only.
func TestAsk(t *testing.T) {
	var method, uri, authorization, agent string
	var body []byte
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, uri, authorization, agent = r.Method, r.RequestURI, r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "  busy "+strings.Repeat("x", 2*MaxAnswer))
	}))
	defer tool.Close()
	answer, err := New("Polyfin/0.47.0").Ask(t.Context(), tool.URL+"/v1/update?scope=media", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || uri != "/v1/update?scope=media" || authorization != "Bearer tok" || agent != "Polyfin/0.47.0" || len(body) != 0 {
		t.Errorf("request: %s %s %q %q %q", method, uri, authorization, agent, body)
	}
	if answer.Status != http.StatusServiceUnavailable || !strings.HasPrefix(answer.Body, "busy x") || len(answer.Body) != MaxAnswer-2 {
		t.Errorf("answer: %d %d %.20q", answer.Status, len(answer.Body), answer.Body)
	}
	if _, err := New("Polyfin/0.47.0").Ask(t.Context(), tool.URL, ""); err != nil || authorization != "" {
		t.Errorf("without a token: %q, %v", authorization, err)
	}
}

// A redirect is the tool's answer: the request goes nowhere else.
func TestAskFollowsNoRedirect(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the redirect was followed")
	}))
	defer elsewhere.Close()
	tool := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusFound))
	defer tool.Close()
	answer, err := New("Polyfin").Ask(t.Context(), tool.URL, "tok")
	if err != nil || answer.Status != http.StatusFound {
		t.Errorf("answer: %+v, %v", answer, err)
	}
}

// A tool that does not answer in time is told as such; one that cannot
// be reached tells why, without the address.
func TestAskFailures(t *testing.T) {
	release := make(chan struct{})
	silent := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer silent.Close()
	defer close(release)
	client := New("Polyfin")
	client.Timeout = 100 * time.Millisecond
	if _, err := client.Ask(t.Context(), silent.URL, ""); !errors.Is(err, ErrTimeout) {
		t.Errorf("silent: %v", err)
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	address := gone.URL + "/v1/update?token=secret-in-query"
	gone.Close()
	_, err := New("Polyfin").Ask(t.Context(), address, "")
	if err == nil || errors.Is(err, ErrTimeout) || strings.Contains(err.Error(), "secret-in-query") {
		t.Errorf("gone: %v", err)
	}
}
