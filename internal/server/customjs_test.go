package server

import (
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

// customTag finds the custom script's tag and its hash.
var customTag = regexp.MustCompile(`<script src="custom\.js\?v=([0-9a-f]{16})" defer></script>`)

// customHandler serves the web client with script as the administrator's,
// which the test can change.
func customHandler(script *atomic.Pointer[string]) http.Handler {
	return New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html><head><title>Polyfin</title></head>")}},
		AdminAPI:      owner("admin api"),
		Jellyfin:      owner("jellyfin"),
		Web:           webFiles,
		SetupRequired: setupState{}.SetupRequired,
		CustomJs:      func() string { return *script.Load() },
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestCustomScriptIsAddedOnlyOnceSet(t *testing.T) {
	script := new(atomic.Pointer[string])
	empty := ""
	script.Store(&empty)
	h := customHandler(script)
	// Empty: the page is as it was, and there is no custom.js.
	if body := get(h, "/web/").Body.String(); strings.Contains(body, "custom.js") {
		t.Errorf("no script set, yet the page loads one: %s", body)
	}
	if body := get(h, "/web/custom.js").Body.String(); body != "jellyfin" {
		t.Errorf("no script set: /web/custom.js answered %q, want Jellyfin's handler", body)
	}
	if body := get(webHandler(setupState{}), "/web/").Body.String(); strings.Contains(body, "custom.js") {
		t.Errorf("without the setting: %s", body)
	}

	first := "console.log('first')"
	script.Store(&first)
	hashes := map[string]bool{}
	for _, target := range []string{"/web/", "/web/index.html"} {
		body := get(h, target).Body.String()
		tags := customTag.FindAllStringSubmatch(body, -1)
		if len(tags) != 1 {
			t.Fatalf("%s: %d custom script tags in %s", target, len(tags), body)
		}
		// After Polyfin's own script, in the head.
		at := strings.Index(body, tags[0][0])
		if at < strings.Index(body, `<script src="polyfin.js"></script>`) || at > strings.Index(body, "</head>") {
			t.Errorf("%s: custom script misplaced in %s", target, body)
		}
		hashes[tags[0][1]] = true
	}
	if len(hashes) != 1 {
		t.Errorf("one script, several hashes: %v", hashes)
	}
	var firstHash string
	for hash := range hashes {
		firstHash = hash
	}
	second := "console.log('second')"
	script.Store(&second)
	tags := customTag.FindStringSubmatch(get(h, "/web/").Body.String())
	if tags == nil || tags[1] == firstHash {
		t.Errorf("the hash did not follow the content: %v, was %s", tags, firstHash)
	}
	// Never in the admin app, nor any other page.
	for _, target := range []string{"/admin/", "/admin/users", "/web/session-login.0123456789abcdef.htm"} {
		if body := get(h, target).Body.String(); strings.Contains(body, "custom.js") {
			t.Errorf("%s loads the custom script: %s", target, body)
		}
	}
}

func TestCustomScriptIsCachedUntilItChanges(t *testing.T) {
	script := new(atomic.Pointer[string])
	code := "document.body.dataset.custom = 'yes'"
	script.Store(&code)
	h := customHandler(script)
	hash := customTag.FindStringSubmatch(get(h, "/web/").Body.String())[1]
	for _, tc := range []struct{ target, cache string }{
		{"/web/custom.js?v=" + hash, immutableCache},
		{"/web/custom.js", "no-cache"},
		// An address of an older script gets today's, revalidated.
		{"/web/custom.js?v=0123456789abcdef", "no-cache"},
	} {
		response := get(h, tc.target)
		header := response.Header()
		if response.Code != http.StatusOK || response.Body.String() != code {
			t.Errorf("%s: %d %q", tc.target, response.Code, response.Body)
		}
		if header.Get("Content-Type") != "application/javascript; charset=utf-8" {
			t.Errorf("%s: Content-Type %q", tc.target, header.Get("Content-Type"))
		}
		if header.Get("Cache-Control") != tc.cache {
			t.Errorf("%s: Cache-Control %q, want %q", tc.target, header.Get("Cache-Control"), tc.cache)
		}
		if header.Get("Content-Security-Policy") != "" {
			t.Errorf("%s: a Content-Security-Policy", tc.target)
		}
	}
}
