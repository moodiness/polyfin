package source

import (
	"bytes"
	"fmt"
	"mime"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Reply is what a source answered, as far as it tells what it is
// without its bytes beyond the first few: a status (0 when unknown), its
// media type, its size (-1 when unknown) and its first bytes (nil when
// none were read). It holds no address, and its description no byte of
// the body itself.
type Reply struct {
	Status      int
	ContentType string
	Size        int64
	Head        []byte
}

// ReplyHead is how many of an answer's first bytes tell what it is.
const ReplyHead = 512

// String describes the answer, such as "a web page (HTTP 200,
// text/html, 2.1 kB)": its kind (see Kind), then what its headers told.
func (a Reply) String() string {
	var details []string
	if a.Status != 0 {
		details = append(details, "HTTP "+strconv.Itoa(a.Status))
	}
	if mediaType, _, err := mime.ParseMediaType(a.ContentType); err == nil {
		details = append(details, mediaType)
	}
	if a.Size > 0 {
		details = append(details, byteSize(a.Size))
	}
	kind := a.Kind()
	if len(details) == 0 {
		return kind
	}
	return kind + " (" + strings.Join(details, ", ") + ")"
}

// Kind tells what an answer looks like: by its first bytes when read,
// else by its media type. Pages, JSON and XML are told apart, as the
// answers sites give instead of a file are mostly one of them.
func (a Reply) Kind() string {
	if a.Size == 0 || a.Head != nil && len(a.Head) == 0 {
		return "an empty body"
	}
	if a.Head != nil {
		if kind := headKind(a.Head); kind != "" {
			return kind
		}
	}
	mediaType, _, _ := mime.ParseMediaType(a.ContentType)
	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		return "a web page"
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		return "JSON"
	case mediaType == "application/xml" || mediaType == "text/xml" || strings.HasSuffix(mediaType, "+xml"):
		return "XML"
	case strings.Contains(mediaType, "mpegurl"):
		return "a playlist"
	case strings.HasPrefix(mediaType, "text/"):
		return "text"
	case a.Head != nil:
		return "data of an unknown format"
	}
	return "an answer of an unknown kind"
}

// Text reports whether the answer's first bytes, read, are text or
// nothing: a page, JSON, XML, a playlist, other text, or an empty body,
// what sources answer instead of media, which is binary.
func (a Reply) Text() bool {
	return a.Head != nil && (len(a.Head) == 0 || headKind(a.Head) != "")
}

// headKind tells what an answer is by its first bytes, "" when they do
// not tell.
func headKind(head []byte) string {
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	trimmed := bytes.TrimLeft(head, " \t\r\n")
	lower := bytes.ToLower(trimmed[:min(len(trimmed), 16)])
	switch {
	case bytes.HasPrefix(head, []byte("#EXTM3U")):
		return "a playlist"
	case bytes.HasPrefix(trimmed, []byte("#EXTM3U")):
		return "a playlist that does not start with #EXTM3U"
	case bytes.HasPrefix(lower, []byte("<!doctype html")), bytes.HasPrefix(lower, []byte("<html")),
		bytes.HasPrefix(lower, []byte("<head")), bytes.HasPrefix(lower, []byte("<body")):
		return "a web page"
	case bytes.HasPrefix(lower, []byte("<?xml")):
		return "XML"
	case len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '['):
		return "JSON"
	case len(trimmed) == 0:
		return "blank text"
	case printable(head):
		return "text"
	}
	return ""
}

// printable reports whether head reads as text: valid UTF-8, a rune cut
// at its end aside, without control characters but spacing.
func printable(head []byte) bool {
	for len(head) > 0 {
		r, n := utf8.DecodeRune(head)
		if r == utf8.RuneError && n <= 1 {
			return len(head) < utf8.UTFMax && !utf8.FullRune(head)
		}
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0x7f {
			return false
		}
		head = head[n:]
	}
	return true
}

// byteSize writes a size as people read it: "512 bytes", "2.1 kB".
func byteSize(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10) + " bytes"
	case n < 1_000_000:
		return fmt.Sprintf("%.1f kB", float64(n)/1e3)
	case n < 1_000_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}

// Reply is what the source's first answer read as its content told (see
// Reply), with head, its first bytes, as Polyfin's own reads of it, such
// as an analysis's, got them.
func (s *Source) Reply(head []byte) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Reply{Status: s.status, ContentType: s.contentType, Size: s.size, Head: head}
}
