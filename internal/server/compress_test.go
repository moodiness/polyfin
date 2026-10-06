package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// jsonOf returns a JSON array of about size bytes, as compressible as the
// API's lists.
func jsonOf(size int) []byte {
	var b bytes.Buffer
	b.WriteString("[")
	for n := 0; b.Len() < size-40; n++ {
		if n > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"Name":"Item ` + strconv.Itoa(n) + `","Type":"Movie"}`)
	}
	b.WriteString("]")
	return b.Bytes()
}

// answering serves every path with the handler, as Jellyfin's routes.
func answering(h http.HandlerFunc) http.Handler {
	return New(Options{
		Database: database{},
		Admin:    fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Polyfin</title>")}},
		AdminAPI: owner("admin api"),
		Jellyfin: h,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// fixed answers with body, of contentType, and the given headers.
func fixed(contentType string, body []byte, headers ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		_, _ = w.Write(body)
	}
}

// fetch requests target from h, with the request headers given in pairs.
func fetch(h http.Handler, method, target string, headers ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	return recorder
}

func gunzip(t *testing.T, body []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("broken gzip: %v", err)
	}
	return data
}

func varies(header http.Header) bool {
	return strings.Contains(strings.Join(header.Values("Vary"), ","), "Accept-Encoding")
}

func TestLargeJSONIsCompressed(t *testing.T) {
	body := jsonOf(8000)
	for name, h := range map[string]http.HandlerFunc{
		"unknown length": fixed("application/json; charset=utf-8", body),
		"known length":   fixed("application/json; charset=utf-8", body, "Content-Length", strconv.Itoa(len(body))),
		"+json type":     fixed("application/problem+json", body),
		// Go finds the type from the body.
		"untyped": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) },
		// Written in small pieces, past the first kilobyte.
		"in pieces": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			for piece := range slicesOf(body, 100) {
				_, _ = w.Write(piece)
			}
		},
	} {
		response := fetch(answering(h), http.MethodGet, "/Items", "Accept-Encoding", "gzip, deflate, br")
		header := response.Header()
		if header.Get("Content-Encoding") != "gzip" {
			t.Errorf("%s: Content-Encoding %q", name, header.Get("Content-Encoding"))
			continue
		}
		if !varies(header) || header.Get("Content-Length") != "" {
			t.Errorf("%s: Vary %q, Content-Length %q", name, header.Values("Vary"), header.Get("Content-Length"))
		}
		if response.Body.Len() >= len(body)/2 {
			t.Errorf("%s: %d bytes for %d", name, response.Body.Len(), len(body))
		}
		if got := gunzip(t, response.Body.Bytes()); !bytes.Equal(got, body) {
			t.Errorf("%s: decoded body differs", name)
		}
	}
}

func slicesOf(data []byte, size int) func(func([]byte) bool) {
	return func(yield func([]byte) bool) {
		for len(data) > 0 {
			n := min(size, len(data))
			if !yield(data[:n]) {
				return
			}
			data = data[n:]
		}
	}
}

func TestSmallAnswersAreNotCompressed(t *testing.T) {
	body := jsonOf(600)
	for name, h := range map[string]http.HandlerFunc{
		"unknown length": fixed("application/json", body),
		"known length":   fixed("application/json", body, "Content-Length", strconv.Itoa(len(body))),
	} {
		response := fetch(answering(h), http.MethodGet, "/Items", "Accept-Encoding", "gzip")
		if response.Header().Get("Content-Encoding") != "" || !bytes.Equal(response.Body.Bytes(), body) {
			t.Errorf("%s: Content-Encoding %q, body changed: %v", name, response.Header().Get("Content-Encoding"), !bytes.Equal(response.Body.Bytes(), body))
		}
		if !varies(response.Header()) {
			t.Errorf("%s: no Vary: Accept-Encoding", name)
		}
	}
}

func TestOnlyClientsAcceptingGzipGetIt(t *testing.T) {
	body := jsonOf(4000)
	h := answering(fixed("application/json", body))
	for acceptEncoding, want := range map[string]bool{
		"":                          false,
		"gzip;q=0":                  false,
		"gzip; q=0.000":             false,
		"br, deflate":               false,
		"identity":                  false,
		"*;q=0":                     false,
		"gzip;q=0, *":               false,
		"gzip;q=nonsense":           false,
		"gzip":                      true,
		"GZIP;q=0.5":                true,
		"x-gzip":                    true,
		"*":                         true,
		"deflate, gzip;q=1.0, br":   true,
		"br;q=1.0, gzip;q=0.8, *;q": true,
	} {
		response := fetch(h, http.MethodGet, "/Items", "Accept-Encoding", acceptEncoding)
		got := response.Header().Get("Content-Encoding") == "gzip"
		if got != want {
			t.Errorf("Accept-Encoding %q: compressed %v, want %v", acceptEncoding, got, want)
		}
		if !varies(response.Header()) {
			t.Errorf("Accept-Encoding %q: no Vary: Accept-Encoding", acceptEncoding)
		}
		if !want && !bytes.Equal(response.Body.Bytes(), body) {
			t.Errorf("Accept-Encoding %q: body changed", acceptEncoding)
		}
	}
}

func TestMediaRangesAndEncodedAnswersPassUntouched(t *testing.T) {
	text := jsonOf(4000)
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		method  string
		headers []string
	}{
		{"range request", fixed("application/json", text), http.MethodGet, []string{"Range", "bytes=0-99"}},
		{"head request", fixed("application/json", text), http.MethodHead, nil},
		{"partial answer", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(text)
		}, http.MethodGet, nil},
		{"video", fixed("video/mp4", text), http.MethodGet, nil},
		{"audio", fixed("audio/mpeg", text), http.MethodGet, nil},
		{"image", fixed("image/png", text), http.MethodGet, nil},
		{"binary", fixed("application/octet-stream", text), http.MethodGet, nil},
		{"event stream", fixed("text/event-stream", text), http.MethodGet, nil},
		{"already encoded", fixed("application/json", text, "Content-Encoding", "br"), http.MethodGet, nil},
	} {
		headers := append([]string{"Accept-Encoding", "gzip"}, tc.headers...)
		response := fetch(answering(tc.handler), tc.method, "/Items", headers...)
		if encoding := response.Header().Get("Content-Encoding"); encoding == "gzip" {
			t.Errorf("%s: compressed", tc.name)
		}
		if tc.method == http.MethodGet && !bytes.Equal(response.Body.Bytes(), text) {
			t.Errorf("%s: body changed", tc.name)
		}
		if tc.name == "video" && varies(response.Header()) {
			t.Errorf("video: Vary: Accept-Encoding")
		}
	}
}

// HLS playlists and segments are fetched as a title plays, whatever their
// type: WebVTT segments are text, and stay as they are.
func TestHLSPlaylistsAndSegmentsAreNotCompressed(t *testing.T) {
	text := []byte("WEBVTT\n\n" + strings.Repeat("00:00:01.000 --> 00:00:02.000\nA line of dialogue\n\n", 100))
	id := "0123456789abcdef0123456789abcdef"
	for _, target := range []string{
		"/Videos/" + id + "/master.m3u8?PlaySessionId=1",
		"/videos/" + id + "/main.m3u8",
		"/Videos/" + id + "/hls1/main/3.mp4",
		"/Videos/" + id + "/hls1/subtitles0/main.m3u8",
		"/Videos/" + id + "/hls1/subtitles0/2.vtt",
		"/Audio/" + id + "/hls1/main/0.ts",
		"/Videos/" + id + "/hls/live/7.ts",
		"/Videos/" + id + "/live/index.vtt",
		"/Videos/" + id + "/Trickplay/320/tiles.m3u8",
	} {
		response := fetch(answering(fixed("text/vtt", text)), http.MethodGet, target, "Accept-Encoding", "gzip")
		if response.Header().Get("Content-Encoding") != "" || !bytes.Equal(response.Body.Bytes(), text) {
			t.Errorf("%s: compressed", target)
		}
	}
	// A whole subtitle file is not HLS.
	response := fetch(answering(fixed("text/vtt", text)), http.MethodGet, "/Videos/"+id+"/"+id+"/Subtitles/0/Stream.vtt", "Accept-Encoding", "gzip")
	if response.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("subtitle file: not compressed")
	}
}

// The WebSocket goes past the compression: the handler takes the
// connection over as with no middleware.
func TestWebSocketIsHijackedThroughCompression(t *testing.T) {
	server := httptest.NewServer(answering(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no Hijacker", http.StatusInternalServerError)
			return
		}
		conn, buffer, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nhello")
		_ = buffer.Flush()
	}))
	defer server.Close()
	for _, target := range []string{"/socket?api_key=k", "/Socket"} {
		conn, err := net.Dial("tcp", server.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = io.WriteString(conn, "GET "+target+" HTTP/1.1\r\nHost: polyfin\r\nAccept-Encoding: gzip\r\n"+
			"Connection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
		reader := bufio.NewReader(conn)
		response, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if response.StatusCode != http.StatusSwitchingProtocols {
			t.Errorf("%s: status %d", target, response.StatusCode)
		}
		if rest, _ := io.ReadAll(reader); string(rest) != "hello" {
			t.Errorf("%s: after the upgrade %q", target, rest)
		}
		_ = conn.Close()
	}
}

// A handler that flushes delivers what it wrote before it returns,
// compressed or not.
func TestFlushDeliversEarly(t *testing.T) {
	first, second := []byte(`{"first":true}`), jsonOf(3000)
	read := make(chan struct{})
	server := httptest.NewServer(answering(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(first)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		select {
		case <-read:
		case <-time.After(5 * time.Second):
			t.Error("the first part never reached the client")
		}
		_, _ = w.Write(second)
	}))
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	for _, acceptEncoding := range []string{"gzip", ""} {
		request, _ := http.NewRequest(http.MethodGet, server.URL+"/Sessions", nil)
		if acceptEncoding != "" {
			request.Header.Set("Accept-Encoding", acceptEncoding)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var body io.Reader = response.Body
		if acceptEncoding != "" {
			if response.Header.Get("Content-Encoding") != "gzip" {
				t.Fatalf("a flushed answer is not compressed: %q", response.Header.Get("Content-Encoding"))
			}
			if body, err = gzip.NewReader(response.Body); err != nil {
				t.Fatal(err)
			}
		}
		got := make([]byte, len(first))
		if _, err := io.ReadFull(body, got); err != nil || !bytes.Equal(got, first) {
			t.Errorf("Accept-Encoding %q: first part %q, %v", acceptEncoding, got, err)
		}
		read <- struct{}{}
		rest, err := io.ReadAll(body)
		if err != nil || !bytes.Equal(rest, second) {
			t.Errorf("Accept-Encoding %q: second part differs, %v", acceptEncoding, err)
		}
		_ = response.Body.Close()
	}
}

// A compressed answer has an ETag of its own, so that a cache never takes
// one variant for the other, and revalidates with it.
func TestCompressedAnswersHaveTheirOwnETag(t *testing.T) {
	h := webHandler(setupState{})
	identity := fetch(h, http.MethodGet, "/web/polyfin.js")
	compressed := fetch(h, http.MethodGet, "/web/polyfin.js", "Accept-Encoding", "gzip")
	tag := identity.Header().Get("ETag")
	if compressed.Header().Get("Content-Encoding") != "gzip" || compressed.Header().Get("ETag") != gzipTag(tag) || gzipTag(tag) == tag {
		t.Fatalf("polyfin.js: Content-Encoding %q, ETag %q for identity %q", compressed.Header().Get("Content-Encoding"), compressed.Header().Get("ETag"), tag)
	}
	if !bytes.Equal(gunzip(t, compressed.Body.Bytes()), identity.Body.Bytes()) || compressed.Header().Get("Cache-Control") != "no-cache" {
		t.Error("polyfin.js: the compressed script differs")
	}
	for _, tc := range []struct {
		held           string
		acceptEncoding string
		status         int
		etag           string
	}{
		{gzipTag(tag), "gzip", http.StatusNotModified, gzipTag(tag)},
		{`"other", ` + gzipTag(tag), "gzip", http.StatusNotModified, gzipTag(tag)},
		{tag, "gzip", http.StatusNotModified, tag},
		{tag, "", http.StatusNotModified, tag},
		// A client that no longer takes gzip needs the identity body.
		{gzipTag(tag), "", http.StatusOK, tag},
		{`"other"`, "gzip", http.StatusOK, gzipTag(tag)},
	} {
		response := fetch(h, http.MethodGet, "/web/polyfin.js", "Accept-Encoding", tc.acceptEncoding, "If-None-Match", tc.held)
		if response.Code != tc.status || response.Header().Get("ETag") != tc.etag {
			t.Errorf("If-None-Match %s, Accept-Encoding %q: %d with ETag %q, want %d with %q",
				tc.held, tc.acceptEncoding, response.Code, response.Header().Get("ETag"), tc.status, tc.etag)
		}
		if tc.status == http.StatusNotModified && response.Body.Len() != 0 {
			t.Errorf("If-None-Match %s: 304 with a body", tc.held)
		}
	}
	weak := answering(fixed("application/json", jsonOf(4000), "ETag", `W/"list"`))
	if response := fetch(weak, http.MethodGet, "/Items", "Accept-Encoding", "gzip"); response.Header().Get("ETag") != `W/"list"` {
		t.Errorf("weak ETag became %q", response.Header().Get("ETag"))
	}
}
