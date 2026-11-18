// Package relayed holds what every answer whose bytes a source gave goes
// through before Polyfin sends it from its own address: artwork, streams,
// the files a playlist names, subtitles. A source chooses those bytes and
// the type it gives them, and a browser that opens such an address as a
// page would treat a page or a script among them as Polyfin's own.
package relayed

import (
	"mime"
	"net/http"
	"strings"
)

// Policy is the Content-Security-Policy of a relayed answer: opened as a
// page, it runs no script and loads nothing, in an origin of its own. A
// player, an <img> or a <video> element that loads the answer ignores it.
const Policy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// Sandbox gives a relayed answer its policy (see Policy).
func Sandbox(header http.Header) {
	header.Set("Content-Security-Policy", Policy)
}

// octetStream is the type of relayed bytes that are no media by their
// source's type: a browser saves them, and shows nothing of them.
const octetStream = "application/octet-stream"

// mediaTypes are the types relayed as their source gave them, beside
// video/* and audio/*: the playlists and manifests players read, the
// subtitles and the pictures a playlist names.
var mediaTypes = map[string]bool{
	"application/vnd.apple.mpegurl": true,
	"application/x-mpegurl":         true,
	"application/dash+xml":          true,
	"application/mp4":               true,
	"application/x-subrip":          true,
	"text/vtt":                      true,
	"image/jpeg":                    true,
	"image/png":                     true,
	"image/gif":                     true,
	"image/webp":                    true,
}

// MediaType is the Content-Type a relayed answer keeps of its source's:
// the source's own when it names media (see mediaTypes), with its
// parameters, else application/octet-stream. A page, a script, text, XML
// or JSON is never relayed as one.
func MediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return octetStream
	}
	if strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "audio/") || mediaTypes[mediaType] {
		return contentType
	}
	return octetStream
}

// Page reports whether an answer is a web page: by the type its source
// gave it, or by its first bytes, read as a browser reads them.
func Page(contentType string, head []byte) bool {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && (mediaType == "text/html" || mediaType == "application/xhtml+xml") {
		return true
	}
	return strings.HasPrefix(http.DetectContentType(head), "text/html")
}

// Picture tells the type of a raster picture by its bytes, never by what
// its source called it: JPEG, PNG, GIF, WebP, BMP, ICO or AVIF. Anything
// else is no picture Polyfin relays: an SVG image is a document, which
// may hold a script.
func Picture(body []byte) (contentType string, ok bool) {
	switch detected := http.DetectContentType(body); detected {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/x-icon":
		return detected, true
	}
	// An AVIF picture is an ISO media file of the brand avif, or avis
	// when animated.
	if len(body) >= 12 && string(body[4:8]) == "ftyp" && (string(body[8:12]) == "avif" || string(body[8:12]) == "avis") {
		return "image/avif", true
	}
	return "", false
}
