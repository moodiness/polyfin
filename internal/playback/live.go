package playback

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// maxPlaylist bounds the HLS playlists a live relay reads and rewrites.
const maxPlaylist = 4 << 20

// liveBandwidth stands for the bitrate of a live stream that announces
// none, in a master playlist that must give one: players only compare it
// with other variants, and there is one.
const liveBandwidth = 10_000_000

// AnalyzeLive returns what ffprobe finds in a live stream, read through
// Polyfin for a few seconds. The analysis is kept in memory for a while,
// not saved: a channel's stream may change. A stream that cannot be
// analyzed is not tried again for a while.
func (s *Service) AnalyzeLive(ctx context.Context, version library.Version) (media.Analysis, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return analysis, nil
	}
	if err, failed := s.failures.Get(version.ID); failed {
		return media.Analysis{}, err
	}
	result, err, _ := s.flight.Do("live "+version.ID.String(), func() (any, error) {
		target, release := s.loopback.registerLive(version)
		defer release()
		analysis, err := s.ffprobe().ProbeLive(context.WithoutCancel(ctx), target)
		if err != nil {
			s.failures.Put(version.ID, err)
			return nil, err
		}
		s.analyses.Put(version.ID, analysis)
		return analysis, nil
	})
	if err != nil {
		return media.Analysis{}, err
	}
	return result.(media.Analysis), nil
}

// Manifest reports whether a live analysis is of an HLS playlist, which
// players fetch file by file, rather than of one endless stream.
func Manifest(analysis media.Analysis) bool {
	return strings.Contains(","+analysis.Format+",", ",hls,")
}

// Redirectable reports whether a player may be sent to a version's source
// itself: the source needs no headers and is on a public address.
func (s *Service) Redirectable(ctx context.Context, version library.Version) bool {
	return len(version.Headers) == 0 && s.public(ctx, version.URL)
}

// ServeLive relays a live stream, or a file of it, to a player: target is
// the address asked, the version's own when empty. The addresses an HLS
// playlist names are rewritten by link, so that the player fetches them
// through Polyfin too, with the headers the source needs and within its
// confinement; a playlist naming an address that cannot be rewritten is
// refused. Other files are relayed as they come, byte ranges included, as
// EXT-X-BYTERANGE asks them. When the source does not answer, the player
// receives a 502 and the error is returned for logging.
func (s *Service) ServeLive(w http.ResponseWriter, r *http.Request, version library.Version, target string, link func(string) string) error {
	if target == "" {
		target = version.URL
	}
	header := http.Header{}
	for name, value := range version.Headers {
		header.Set(name, value)
	}
	for _, name := range forwardedRequest {
		if value := r.Header.Get(name); value != "" {
			header.Set(name, value)
		}
	}
	response, err := s.opener.Open(r.Context(), http.MethodGet, target, header, version.Confined)
	if err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
	default:
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return fmt.Errorf("%w: HTTP %d", ErrSourceUnavailable, response.StatusCode)
	}
	body := bufio.NewReaderSize(response.Body, 64<<10)
	// A playlist is rewritten however the source answered, a part of it
	// included: FFmpeg asks for its playlists from their first byte.
	if response.StatusCode != http.StatusRequestedRangeNotSatisfiable && playlist(response, body) {
		data, err := io.ReadAll(io.LimitReader(body, maxPlaylist))
		if response.StatusCode == http.StatusPartialContent && !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes 0-") {
			err = fmt.Errorf("%w: a part of a playlist", ErrUnsafePlaylist)
		}
		base, _ := url.Parse(target)
		if response.Request != nil {
			base = response.Request.URL
		}
		var rewritten []byte
		if err == nil {
			rewritten, err = rewritePlaylist(data, base, link)
		}
		if err != nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return err
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		_, err = w.Write(rewritten)
		return err
	}
	for _, name := range forwardedResponse {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	buffer := copyBuffers.Get().(*[256 << 10]byte)
	defer copyBuffers.Put(buffer)
	_, err = io.CopyBuffer(w, body, buffer[:])
	return err
}

// playlist reports whether a response is an HLS playlist: by its type, or
// by its first line, as sources often answer playlists as plain text or
// bytes.
func playlist(response *http.Response, body *bufio.Reader) bool {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "mpegurl") {
		return true
	}
	start, _ := body.Peek(10)
	return bytes.HasPrefix(bytes.TrimPrefix(start, []byte("\xef\xbb\xbf")), []byte("#EXTM3U"))
}

// ErrUnsafePlaylist reports a playlist naming an address Polyfin cannot
// relay, which FFmpeg would otherwise fetch itself, past the confinement.
var ErrUnsafePlaylist = errors.New("the playlist names an address that cannot be relayed")

// rewritePlaylist names every address of a playlist by link, resolved
// against base, the playlist's own. It reads the playlist as FFmpeg's HLS
// demuxer does, which reads it through Polyfin too: a line ends at a
// carriage return, a line feed or a NUL, and a tag's URI attribute may be
// quoted or not. Inline data: addresses stay as they are; any other address
// that is not HTTP, or does not parse, refuses the playlist, as FFmpeg
// would fetch it itself.
func rewritePlaylist(data []byte, base *url.URL, link func(string) string) ([]byte, error) {
	rewrite := func(ref string) (string, error) {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(strings.ToLower(ref), "data:") {
			return ref, nil
		}
		target, err := base.Parse(ref)
		if err != nil || target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
			return "", fmt.Errorf("%w: %q", ErrUnsafePlaylist, ref)
		}
		return link(target.String()), nil
	}
	var out bytes.Buffer
	lines := strings.FieldsFunc(string(data), func(r rune) bool { return r == '\r' || r == '\n' || r == 0 })
	for _, line := range lines {
		var err error
		switch {
		case strings.HasPrefix(line, "#"):
			line, err = rewriteAttributes(line, rewrite)
		case strings.TrimSpace(line) != "":
			line, err = rewrite(line)
		}
		if err != nil {
			return nil, err
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// rewriteAttributes rewrites the URI attributes of a tag's attribute list,
// read as FFmpeg reads one: KEY=VALUE pairs apart by commas, each value
// within double quotes or up to the next comma. The URI is written quoted.
func rewriteAttributes(line string, rewrite func(string) (string, error)) (string, error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return line, nil
	}
	var out strings.Builder
	out.WriteString(line[:colon+1])
	rest := line[colon+1:]
	for rest != "" {
		skipped := len(rest) - len(strings.TrimLeft(rest, " \t,"))
		out.WriteString(rest[:skipped])
		rest = rest[skipped:]
		equal := strings.IndexByte(rest, '=')
		if equal < 0 {
			out.WriteString(rest)
			break
		}
		key := rest[:equal]
		rest = rest[equal+1:]
		var value, raw string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				end = len(rest) - 1
				value, raw, rest = rest[1:], rest, ""
			} else {
				value, raw, rest = rest[1:end+1], rest[:end+2], rest[end+2:]
			}
		} else {
			end := strings.IndexByte(rest, ',')
			if end < 0 {
				end = len(rest)
			}
			value, raw, rest = rest[:end], rest[:end], rest[end:]
		}
		out.WriteString(key + "=")
		if strings.EqualFold(strings.TrimSpace(key), "URI") {
			rewritten, err := rewrite(value)
			if err != nil {
				return "", err
			}
			raw = `"` + rewritten + `"`
		}
		out.WriteString(raw)
	}
	return out.String(), nil
}

// FileName is the last path segment of an address, which players and
// FFmpeg recognize files by: a playlist by .m3u8, a segment by .ts.
func FileName(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return "stream"
	}
	if name := path.Base(parsed.Path); name != "." && name != "/" {
		return name
	}
	return "stream"
}

// LiveVariant describes a live remux in a master playlist.
func (s *Service) LiveVariant(ctx context.Context, remux Remux) (hls.Variant, error) {
	analysis, err := s.AnalyzeLive(ctx, remux.Version)
	if err != nil {
		return hls.Variant{}, err
	}
	video, ok := LiveVideo(analysis)
	if !ok {
		return hls.Variant{}, ErrNotRemuxable
	}
	bandwidth := analysis.Bitrate
	if bandwidth <= 0 {
		for _, stream := range analysis.Streams {
			if stream.Index == video.Index || stream.Index == remux.Audio {
				bandwidth += stream.Bitrate
			}
		}
	}
	if bandwidth <= 0 {
		bandwidth = liveBandwidth
	}
	return variant(analysis, video, bandwidth, remux), nil
}

// LiveVideo is the video a live stream plays: of the variants an HLS
// playlist offers, which ffprobe lists together, the largest.
func LiveVideo(analysis media.Analysis) (media.Stream, bool) {
	var best media.Stream
	found := false
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture && (!found || stream.Width*stream.Height > best.Width*best.Height) {
			best, found = stream, true
		}
	}
	return best, found
}

// LivePlaylist returns the media playlist of a live remux, starting FFmpeg
// when it does not run; uri names the files it lists.
func (s *Service) LivePlaylist(ctx context.Context, remux Remux, uri func(name string) string) ([]byte, error) {
	return s.segments.LivePlaylist(ctx, remux.key(), s.liveOpener(remux), uri)
}

// LiveSegment opens a file the playlist of a live remux lists.
func (s *Service) LiveSegment(remux Remux, name string) (*os.File, error) {
	return s.segments.LiveSegment(remux.key(), name)
}

// liveOpener reads the live stream through the loopback relay, so that
// FFmpeg sends the headers it needs and stays within its confinement.
func (s *Service) liveOpener(remux Remux) hls.Opener {
	return func(ctx context.Context) (hls.Remux, func(), error) {
		analysis, err := s.AnalyzeLive(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		video, ok := LiveVideo(analysis)
		if !ok {
			return hls.Remux{}, nil, ErrNotRemuxable
		}
		target, release := s.loopback.registerLive(remux.Version)
		r := hls.Remux{Input: target, Video: video.Index, Audio: audioOf(analysis, remux.Audio), Format: remux.Format}
		if Manifest(analysis) {
			r.InputOptions = media.LiveOptions
		}
		if audio, ok := streamOf(analysis, r.Audio); ok && audio.Codec == "aac" {
			r.ADTS = true
		}
		remux.convert(&r, video, TuningOf(s.settings()))
		return r, release, nil
	}
}

// registerLive makes a version's live stream readable on the loopback
// interface until release is called: its playlists rewritten to name
// their files on the loopback interface too.
func (l *loopback) registerLive(version library.Version) (string, func()) {
	key := randomKey()
	l.mu.Lock()
	l.feeds[key] = version
	l.mu.Unlock()
	release := func() {
		l.mu.Lock()
		delete(l.feeds, key)
		l.mu.Unlock()
	}
	return l.liveURL(key, "", version.URL), release
}

// liveURL is the loopback address of a file of a live stream, target
// being its source's address, empty for the stream itself.
func (l *loopback) liveURL(key, target, source string) string {
	encoded := "-"
	if target != "" {
		encoded = base64.RawURLEncoding.EncodeToString([]byte(target))
	}
	return "http://" + l.listener.Addr().String() + "/live/" + key + "/" + encoded + "/" + url.PathEscape(FileName(source))
}

// serveLive answers FFmpeg's and ffprobe's requests for a live stream
// registered on the loopback interface: /live/{key}/{target}/{name}.
func (l *loopback) serveLive(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	l.mu.Lock()
	version, ok := l.feeds[parts[0]]
	l.mu.Unlock()
	target := ""
	if parts[1] != "-" {
		decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
		ok = ok && err == nil
		target = string(decoded)
	}
	if !ok || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	link := func(file string) string { return l.liveURL(parts[0], file, file) }
	_ = l.live(w, r, version, target, link)
}
