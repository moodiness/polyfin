package playback

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
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

const (
	// liveProbeCap bounds the analysis of a live stream, within the
	// settings' AnalysisTimeout: a stream that answered its first bytes
	// (see sniff) shows what it holds within a second or two.
	liveProbeCap = 8 * time.Second
	// shapeLife is how long what a channel's stream holds is trusted, and
	// shapeRefresh how old it is when a start analyzes it again on the
	// way, from the stream already read.
	shapeLife    = 7 * 24 * time.Hour
	shapeRefresh = time.Hour
)

// shortProbe are FFmpeg's options for reading an MPEG-TS stream whose
// tracks are known: half a second of probing instead of five.
var shortProbe = []string{"-probesize", "500000", "-analyzeduration", "500000", "-fflags", "+nobuffer"}

// AnalyzeLive returns what a live stream holds: as found before, kept in
// the database for shapeLife; else as ffprobe finds it, reading the
// stream's shared feed (see openFeed) for a second, or an HLS playlist
// through the loopback relay, within liveProbeCap. A stream that does not
// answer with a live stream fails at once with ErrLiveDead,
// ErrLiveRefused or ErrLiveTimeout; dead and silent streams are not tried
// again for a while, refused ones are at the next start. How it answered
// is told to LiveSources' health.
func (s *Service) AnalyzeLive(ctx context.Context, version library.Version) (media.Analysis, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return analysis, nil
	}
	if analysis, at, ok := s.shape(ctx, version.ID); ok && time.Since(at) < shapeLife {
		s.analyses.Put(version.ID, analysis)
		return analysis, nil
	}
	if err, failed := s.failures.Get(version.ID); failed {
		return media.Analysis{}, err
	}
	result, err, _ := s.flight.Do("live "+version.ID.String(), func() (any, error) {
		return s.probeLive(context.WithoutCancel(ctx), version)
	})
	if err != nil {
		return media.Analysis{}, err
	}
	return result.(media.Analysis), nil
}

// probeLive analyzes a live stream (see AnalyzeLive) and keeps what it
// found.
func (s *Service) probeLive(ctx context.Context, version library.Version) (media.Analysis, error) {
	prober := s.ffprobe()
	prober.Timeout = min(prober.Timeout, liveProbeCap)
	ctx, cancel := context.WithTimeout(ctx, prober.Timeout)
	defer cancel()
	reader, err := s.openFeed(ctx, version)
	if err != nil && !errors.Is(err, errManifest) {
		s.liveFailed(ctx, version, err)
		return media.Analysis{}, err
	}
	// The analysis joins the feed opened for it: it holds it open meanwhile.
	defer reader.Close()
	target, release := s.loopback.registerLiveFor(version, userOf(ctx))
	defer release()
	analysis, err := prober.ProbeLive(ctx, target)
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", ErrLiveTimeout, err)
		} else if LiveFailure(err) == "" {
			err = fmt.Errorf("%w: %v", ErrLiveDead, err)
		}
		s.liveFailed(ctx, version, err)
		return media.Analysis{}, err
	}
	s.analyses.Put(version.ID, analysis)
	s.saveShape(ctx, version.ID, analysis)
	s.report(ctx, version, nil)
	return analysis, nil
}

// liveFailed records a live stream's failure: told to LiveSources'
// health, and, unless it was refused, kept among the failures.
func (s *Service) liveFailed(ctx context.Context, version library.Version, err error) {
	s.logger.Info("A channel's stream failed", "addon", version.Addon, "failure", LiveFailure(err), "error", err)
	s.report(ctx, version, err)
	if LiveFailure(err) != LiveRefused {
		s.failures.Put(version.ID, err)
	}
}

// shape returns what a channel's stream was found to hold, and when.
func (s *Service) shape(ctx context.Context, version accounts.ID) (media.Analysis, time.Time, bool) {
	var data []byte
	var at time.Time
	err := s.db.QueryRow(ctx, "SELECT analysis, analyzed_at FROM live_stream_shapes WHERE version_id = $1", version).Scan(&data, &at)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("Reading a channel's stream shape failed", "error", err)
		}
		return media.Analysis{}, time.Time{}, false
	}
	var analysis media.Analysis
	if err := json.Unmarshal(data, &analysis); err != nil {
		return media.Analysis{}, time.Time{}, false
	}
	return analysis, at, true
}

func (s *Service) saveShape(ctx context.Context, version accounts.ID, analysis media.Analysis) {
	data, err := json.Marshal(analysis)
	if err == nil {
		_, err = s.db.Exec(ctx, `INSERT INTO live_stream_shapes (version_id, analysis) VALUES ($1, $2)
			ON CONFLICT (version_id) DO UPDATE SET analysis = excluded.analysis, analyzed_at = now()`, version, data)
	}
	if err != nil {
		s.logger.Warn("Saving a channel's stream shape failed", "error", err)
	}
}

// forgetShape drops what a channel's stream was found to hold, so that it
// is analyzed again.
func (s *Service) forgetShape(ctx context.Context, version accounts.ID) {
	s.analyses.Delete(version)
	if _, err := s.db.Exec(ctx, "DELETE FROM live_stream_shapes WHERE version_id = $1", version); err != nil {
		s.logger.Warn("Forgetting a channel's stream shape failed", "error", err)
	}
}

// refreshShape analyzes a channel's stream again, reading its feed that
// a start opened, when what it was found to hold is older than
// shapeRefresh.
func (s *Service) refreshShape(ctx context.Context, version library.Version) {
	if _, at, ok := s.shape(ctx, version.ID); !ok || time.Since(at) < shapeRefresh {
		return
	}
	go func() {
		if _, err, _ := s.flight.Do("live "+version.ID.String(), func() (any, error) {
			return s.probeLive(context.WithoutCancel(ctx), version)
		}); err != nil {
			s.logger.Debug("A channel's stream could not be analyzed again", "error", err)
		}
	}()
}

// ServeChannel serves a channel's MPEG-TS stream to a player that plays it
// as it is: from its feed (see ServeFeed), unless the player may be sent
// to the source itself (relay unset, Redirectable) and that costs no
// connection Polyfin needs: the source has no connection limit and no one
// reads the feed, which is closed first.
func (s *Service) ServeChannel(w http.ResponseWriter, r *http.Request, version library.Version, relay bool) error {
	if !relay && s.liveReaders(version.ID) == 0 && !s.limited(r.Context(), version) && s.Redirectable(r.Context(), version) {
		s.closeIdleFeed(version.ID)
		return s.Serve(w, r, version, Delivery{})
	}
	return s.ServeFeed(w, r, version)
}

// limited reports whether a version's source plays a limited number of
// streams at once.
func (s *Service) limited(ctx context.Context, version library.Version) bool {
	s.feeds.mu.Lock()
	connections := s.feeds.connections
	s.feeds.mu.Unlock()
	if connections == nil || version.Origin.Addon == (accounts.ID{}) {
		return false
	}
	limit, err := connections(ctx, version.Origin.Addon)
	return err != nil || limit > 0
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
// the address asked, the version's own when empty, an MPEG-TS stream read
// from its feed (see ServeFeed). The addresses an HLS
// playlist names are rewritten by link, so that the player fetches them
// through Polyfin too, with the headers the source needs and within its
// confinement; a playlist naming an address that cannot be rewritten is
// refused. Other files are relayed as they come, byte ranges included, as
// EXT-X-BYTERANGE asks them. When the source does not answer, the player
// receives a 502 and the error is returned for logging.
func (s *Service) ServeLive(w http.ResponseWriter, r *http.Request, version library.Version, target string, link func(string) string) error {
	// An MPEG-TS stream is read from its feed, shared with every other
	// reader of the channel.
	if target == "" && !s.manifest(version.ID) {
		err := s.ServeFeed(w, r, version)
		if !errors.Is(err, errManifest) {
			return err
		}
	}
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
// FFmpeg sends the headers it needs, stays within its confinement and, for
// MPEG-TS, shares the stream's feed. An MPEG-TS stream whose tracks are
// known is read with a short probe; opened again, after a run that failed
// quickly, it is analyzed again and read with FFmpeg's full probe.
func (s *Service) liveOpener(remux Remux) hls.Opener {
	opens := 0
	return func(ctx context.Context) (hls.Remux, func(), error) {
		ctx = ForUser(ctx, remux.User)
		opens++
		if opens > 1 {
			s.forgetShape(ctx, remux.Version.ID)
		}
		analysis, err := s.AnalyzeLive(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		video, ok := LiveVideo(analysis)
		if !ok {
			return hls.Remux{}, nil, ErrNotRemuxable
		}
		target, release := s.loopback.registerLiveFor(remux.Version, remux.User)
		r := hls.Remux{Input: target, Video: video.Index, Audio: audioOf(analysis, remux.Audio), Format: remux.Format}
		switch {
		case Manifest(analysis):
			r.InputOptions = media.LiveOptions
		case opens == 1:
			r.InputOptions = shortProbe
			s.refreshShape(ctx, remux.Version)
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
	return l.registerLiveFor(version, accounts.ID{})
}

// registerLiveFor registers a version's live stream read for user, whose
// oldest stream of the source may make room for it (see makeRoom).
func (l *loopback) registerLiveFor(version library.Version, user accounts.ID) (string, func()) {
	key := randomKey()
	if user != (accounts.ID{}) {
		key += "." + user.String()
	}
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
	if _, user, ok := strings.Cut(parts[0], "."); ok {
		if id, err := accounts.ParseID(user); err == nil {
			r = r.WithContext(ForUser(r.Context(), id))
		}
	}
	_ = l.live(w, r, version, target, link)
}
