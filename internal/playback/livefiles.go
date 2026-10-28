package playback

import (
	"bufio"
	"bytes"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/source"
)

// segmentExtensions are the names FFmpeg's HLS demuxer takes the media
// of a segment by: since FFmpeg 7.0.3 and 7.1.1 it refuses a segment
// whose name is none of those of the format it finds in it
// (extension_picky), as "Invalid data found when processing input".
// MPEG-TS, fragmented MP4 and AAC are all taken under .ts.
var segmentExtensions = []string{"ts", "m2t", "m2ts", "mts", "mpg", "mpeg", "mpegts", "m4s", "mp4", "m4v", "m4a", "cmfv", "cmfa",
	"aac", "ac3", "eac3", "ec3", "mp3", "vtt", "webvtt"}

// segmentName is the name a relayed playlist gives a segment, name being
// its source's (see FileName): the same when FFmpeg takes media by it,
// else with .ts added, as some providers name their segments .png, .jpg,
// .php or nothing. FFmpeg still checks what it finds in the segment: a
// picture or a page named so is still refused.
func segmentName(name string) string {
	if slices.Contains(segmentExtensions, strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))) {
		return name
	}
	return name + ".ts"
}

// pictureScan bounds how far into a segment its MPEG-TS is looked for
// past a picture in front of it.
const pictureScan = 64 << 10

// pictureBefore returns how many bytes of a picture come before a
// segment's MPEG-TS, 0 for none: some providers put a PNG, JPEG, GIF or
// WebP picture in front of each segment, so that it passes for one. Other
// players skip it; FFmpeg would find the picture, and read it as the
// segment, or refuse it. The MPEG-TS is where five packets in a row start
// with their sync byte, within the first pictureScan bytes; a segment
// that does not start with a picture is never looked into.
func pictureBefore(body *bufio.Reader) int {
	start, _ := body.Peek(12)
	if !picture(start) {
		return 0
	}
	head, _ := body.Peek(pictureScan)
	for at := len(start); at+4*tsPacket < len(head); at++ {
		if head[at] == 0x47 && head[at+tsPacket] == 0x47 && head[at+2*tsPacket] == 0x47 && head[at+3*tsPacket] == 0x47 &&
			head[at+4*tsPacket] == 0x47 {
			return at
		}
	}
	return 0
}

// picture reports whether start, a file's first bytes, is a PNG, JPEG,
// GIF or WebP picture's.
func picture(start []byte) bool {
	return bytes.HasPrefix(start, []byte("\x89PNG\r\n\x1a\n")) || bytes.HasPrefix(start, []byte("\xff\xd8\xff")) ||
		bytes.HasPrefix(start, []byte("GIF87a")) || bytes.HasPrefix(start, []byte("GIF89a")) ||
		len(start) >= 12 && bytes.Equal(start[:4], []byte("RIFF")) && bytes.Equal(start[8:12], []byte("WEBP"))
}

// fileAnswer is what a file of a live stream, a playlist, a key or a
// segment, answered when it was no media: a refusal, a page, nothing. It
// names the file by its name only, never its address, and tells of the
// answer what source.Reply does, never its body.
type fileAnswer struct {
	name  string
	reply source.Reply
}

func (a fileAnswer) Error() string {
	name := a.name
	if path.Ext(name) == "" {
		// A name without an extension may be a token.
		name = "a file named without an extension"
	}
	message := name + " answered " + a.reply.String()
	if a.refused() {
		return ErrSourceUnavailable.Error() + ": " + message
	}
	return message
}

// Unwrap makes a refusal a source unavailable.
func (a fileAnswer) Unwrap() error {
	if a.refused() {
		return ErrSourceUnavailable
	}
	return nil
}

func (a fileAnswer) refused() bool {
	return a.reply.Status != http.StatusOK && a.reply.Status != http.StatusPartialContent && a.reply.Status != http.StatusRequestedRangeNotSatisfiable
}

// replyOf is what response answered, its first bytes peeked from body.
func replyOf(response *http.Response, body *bufio.Reader) source.Reply {
	head, _ := body.Peek(source.ReplyHead)
	return source.Reply{Status: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Size: response.ContentLength,
		Head: bytes.Clone(head)}
}

// noMedia reports whether a file of a live stream answered reply is none:
// text, a page, JSON, or nothing, where media is binary. WebVTT
// subtitles, text, are media.
func noMedia(reply source.Reply) bool {
	return reply.Text() && !bytes.HasPrefix(bytes.TrimPrefix(reply.Head, []byte("\xef\xbb\xbf")), []byte("WEBVTT"))
}
