package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A read of an empty answer ends at once, at its end, rather than waiting
// for bytes that never come, and the empty answer says so.
func TestAnEmptyAnswerEndsReads(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Content-Length", "0")
	}))
	t.Cleanup(server.Close)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/live"}, nil)
	defer s.Release()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	head := make([]byte, ReplyHead)
	n, err := s.ReadAt(ctx, head, 0)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read %d bytes: %v", n, err)
	}
	if reply := s.Reply(head[:n]); reply.Kind() != "an empty body" || reply.Status != http.StatusOK {
		t.Errorf("the empty answer: %+v", reply)
	}
}

// What an answer is shows in its first bytes, whatever type it announces,
// else in its type; its body never shows in its description.
func TestRepliesTellTheirKind(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		reply Reply
		kind  string
	}{
		{Reply{ContentType: "application/octet-stream", Size: 900, Head: []byte("\n\n<!doctype HTML><html>secret")}, "a web page"},
		{Reply{ContentType: "video/mp2t", Size: 40, Head: []byte(" [{\"secret\": 1}]")}, "JSON"},
		{Reply{Size: 40, Head: []byte("<?xml version=\"1.0\"?><error>secret</error>")}, "XML"},
		{Reply{Size: 30, Head: []byte("\xef\xbb\xbf#EXTM3U\n#EXTINF:-1,secret\n")}, "a playlist"},
		{Reply{Size: 30, Head: []byte("\n#EXTM3U\n#EXTINF:-1,secret\n")}, "a playlist that does not start with #EXTM3U"},
		{Reply{Size: 20, Head: []byte("Forbidden: secret é")}, "text"},
		{Reply{Size: 2000, Head: []byte{0x1a, 0x45, 0xdf, 0xa3, 0x00, 0x01}}, "data of an unknown format"},
		{Reply{ContentType: "video/mp4", Size: 0}, "an empty body"},
		{Reply{ContentType: "video/mp4", Size: -1, Head: []byte{}}, "an empty body"},
		{Reply{ContentType: "text/html; charset=utf-8", Size: -1}, "a web page"},
		{Reply{ContentType: "application/problem+json", Size: -1}, "JSON"},
	} {
		if got := tc.reply.Kind(); got != tc.kind {
			t.Errorf("%q: %s, want %s", tc.reply.Head, got, tc.kind)
		}
		if strings.Contains(tc.reply.String(), "secret") {
			t.Errorf("%q tells the body", tc.reply)
		}
	}
}

// A status Polyfin does not read tells what came with it, by its headers.
func TestStatusErrorsTellWhatCameWithThem(t *testing.T) {
	t.Parallel()
	page := statusError(&http.Response{StatusCode: http.StatusForbidden,
		Header: http.Header{"Content-Type": {"text/html"}, "Content-Length": {"2100"}}, Body: http.NoBody}, ErrUnavailable)
	if !strings.Contains(page.Error(), "a web page") || !strings.Contains(Answer(page), "a web page") {
		t.Errorf("a refusal with a page: %v, %s", page, Answer(page))
	}
	bare := statusError(&http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: http.NoBody}, ErrUnavailable)
	if bare.Reply != nil || Answer(bare) != "HTTP 404" {
		t.Errorf("an answer whose headers tell nothing: %v", bare)
	}
}
