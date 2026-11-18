package relayed

import (
	"net/http"
	"strings"
	"testing"
)

// A relayed answer keeps the type its source gave only when it names
// media: a page, a script, an SVG image, text, XML or JSON becomes bytes.
func TestOnlyMediaTypesAreRelayedAsTheirSourceGaveThem(t *testing.T) {
	t.Parallel()
	for given, want := range map[string]string{
		"video/mp2t":                          "video/mp2t",
		"video/mp4; codecs=\"avc1.64001f\"":   "video/mp4; codecs=\"avc1.64001f\"",
		"Video/MP4":                           "Video/MP4",
		"audio/aac":                           "audio/aac",
		"audio/mpegurl":                       "audio/mpegurl",
		"application/vnd.apple.mpegurl":       "application/vnd.apple.mpegurl",
		"application/x-mpegURL":               "application/x-mpegURL",
		"application/dash+xml":                "application/dash+xml",
		"application/mp4":                     "application/mp4",
		"application/x-subrip":                "application/x-subrip",
		"text/vtt; charset=utf-8":             "text/vtt; charset=utf-8",
		"image/jpeg":                          "image/jpeg",
		"image/png":                           "image/png",
		"application/octet-stream":            "application/octet-stream",
		"binary/octet-stream":                 "application/octet-stream",
		"":                                    "application/octet-stream",
		"text/html":                           "application/octet-stream",
		"text/html; charset=utf-8":            "application/octet-stream",
		"TEXT/HTML":                           "application/octet-stream",
		"application/xhtml+xml":               "application/octet-stream",
		"image/svg+xml":                       "application/octet-stream",
		"application/xml":                     "application/octet-stream",
		"text/xml":                            "application/octet-stream",
		"text/plain":                          "application/octet-stream",
		"text/javascript":                     "application/octet-stream",
		"application/javascript":              "application/octet-stream",
		"application/json":                    "application/octet-stream",
		"application/pdf":                     "application/octet-stream",
		"video/mp4, text/html":                "application/octet-stream",
		"text/html; video/mp4":                "application/octet-stream",
		"not a type":                          "application/octet-stream",
		"multipart/x-mixed-replace; boundary": "application/octet-stream",
	} {
		if got := MediaType(given); got != want {
			t.Errorf("%q is relayed as %q, want %q", given, got, want)
		}
	}
}

// A picture is told by its bytes: the raster formats are pictures, an SVG
// image, a page or text is none, whatever comes before its first tag.
func TestPicturesAreToldByTheirBytes(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"JPEG":                    {"\xff\xd8\xff\xe0\x00\x10JFIF\x00", "image/jpeg"},
		"PNG":                     {"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "image/png"},
		"GIF":                     {"GIF89a\x01\x00\x01\x00", "image/gif"},
		"WebP":                    {"RIFF\x1a\x00\x00\x00WEBPVP8L", "image/webp"},
		"BMP":                     {"BM\x46\x00\x00\x00\x00\x00", "image/bmp"},
		"ICO":                     {"\x00\x00\x01\x00\x01\x00\x10\x10", "image/x-icon"},
		"AVIF":                    {"\x00\x00\x00\x1cftypavif\x00\x00\x00\x00", "image/avif"},
		"animated AVIF":           {"\x00\x00\x00\x1cftypavis\x00\x00\x00\x00", "image/avif"},
		"SVG":                     {`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`, ""},
		"SVG after a declaration": {`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`, ""},
		"SVG after blank lines":   {"\n\n  <svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", ""},
		"SVG after a mark":        {"\xef\xbb\xbf<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", ""},
		"page":                    {"<!doctype html><html></html>", ""},
		"MP4 video":               {"\x00\x00\x00\x1cftypisom\x00\x00\x00\x00", ""},
		"text":                    {"a picture", ""},
		"nothing":                 {"", ""},
	} {
		got, ok := Picture([]byte(tc.body))
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%s: %q %v, want %q", name, got, ok, tc.want)
		}
	}
}

// A page is told by the type its source gave it, or by its first bytes.
func TestPagesAreToldByTheirTypeOrTheirBytes(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		contentType, head string
		want              bool
	}{
		"typed a page":            {"text/html; charset=utf-8", "anything", true},
		"typed XHTML":             {"application/xhtml+xml", "<x/>", true},
		"a page typed video":      {"video/mp2t", "<!DOCTYPE html><html>", true},
		"a page without a type":   {"", "  <html><body>", true},
		"a script without a type": {"", "<script>1</script>", true},
		"a comment before a page": {"application/octet-stream", "<!-- x --><html>", true},
		"subtitles":               {"text/vtt", "WEBVTT\n\n00:00.000 --> 00:01.000\nHello", false},
		"a key of letters":        {"application/octet-stream", "0123456789abcdef", false},
		"JSON":                    {"application/json", `{"error":"gone"}`, false},
		"a segment":               {"video/mp2t", "G\x40\x11\x10\x00", false},
	} {
		if got := Page(tc.contentType, []byte(tc.head)); got != tc.want {
			t.Errorf("%s: a page %v, want %v", name, got, tc.want)
		}
	}
}

// The policy sandboxes the answer and lets it load nothing.
func TestARelayedAnswerIsSandboxed(t *testing.T) {
	t.Parallel()
	header := http.Header{}
	header.Set("Content-Security-Policy", "default-src *")
	Sandbox(header)
	policy := header.Get("Content-Security-Policy")
	directives := map[string]bool{}
	for directive := range strings.SplitSeq(policy, ";") {
		directives[strings.TrimSpace(directive)] = true
	}
	if !directives["sandbox"] || !directives["default-src 'none'"] || len(header.Values("Content-Security-Policy")) != 1 {
		t.Errorf("the policy: %q", header.Values("Content-Security-Policy"))
	}
}
