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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
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
// tracks are known: a second and a half of probing instead of five, which
// finds the audio of a stream whose first audio packets come up to a
// second after its video, as FFmpeg's own MPEG-TS muxer delays them by
// 0.7 seconds; FFmpeg stops sooner once every track is found. What it
// probes is kept, not dropped, as the start of the stream it writes: the
// seconds of a feed's start (see feedReader.place) make the first live
// playlist's lead at once.
var shortProbe = []string{"-probesize", "4000000", "-analyzeduration", "1500000"}

// Endless reports whether version streams with no end, as a live stream
// does, which then plays as a channel's stream plays (see AnalyzeLive): an
// HLS playlist that does not end, or a source that announces no size,
// either without a duration once analyzed. A version analyzed before is
// told by its analysis, without reaching its source. A file, of a known
// size, is told at its first bytes, which its analysis reads anyway; it is
// not analyzed here. Torrents and local files have an end.
func (s *Service) Endless(ctx context.Context, version library.Version) (bool, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return Endless(analysis), nil
	}
	if unbounded, err := s.Unbounded(ctx, version); !unbounded || err != nil {
		return false, err
	}
	analysis, err := s.AnalyzeLive(ctx, version)
	if err != nil {
		return false, err
	}
	return Endless(analysis), nil
}

// Unbounded reports whether version's first bytes may be those of a stream
// with no end: an HLS playlist, or a source that announces no size, which
// a file's analysis would wait on until its deadline. It analyzes nothing.
// A source that announces no size is told as soon as it answers, without
// waiting for the first block of its bytes, which a live stream sends in
// real time; one that announces a size is told by its first bytes, as a
// playlist is. Torrents and local files have an end.
func (s *Service) Unbounded(ctx context.Context, version library.Version) (bool, error) {
	if version.Torrent != nil || s.ReadsInput(version) {
		return false, nil
	}
	src := s.open(version)
	defer src.Release()
	reading, stop := context.WithCancel(ctx)
	defer stop()
	type result struct {
		n   int
		err error
	}
	head := make([]byte, 16)
	read := make(chan result, 1)
	go func() {
		n, err := src.ReadAt(reading, head, 0)
		read <- result{n, err}
	}()
	var r result
	select {
	case r = <-read:
	case <-src.Learned():
		if _, sized := src.KnownSize(); !sized {
			// The read stops before the source is released.
			stop()
			<-read
			return true, nil
		}
		r = <-read
	}
	if r.n == 0 && r.err != nil {
		return false, r.err
	}
	size, sized := src.KnownSize()
	playlist := bytes.HasPrefix(bytes.TrimPrefix(head[:r.n], []byte("\xef\xbb\xbf")), []byte("#EXTM3U"))
	return playlist || !sized || size <= 0, nil
}

// Endless reports whether an analysis is that of a stream with no end:
// one whose duration is not known.
func Endless(analysis media.Analysis) bool {
	return analysis.Duration <= 0
}

// AnalyzeLive returns what a live stream holds: as found before, kept in
// the database for shapeLife; else as ffprobe finds it, reading the
// stream's shared feed (see openFeed) for a second, or an HLS playlist
// through the loopback relay, within liveProbeCap once the source
// answered. A stream that does not answer with a live stream fails at once
// with ErrLiveDead, ErrLiveRefused or ErrLiveTimeout. A source that sent
// nothing in time, or refused, is asked again at the next start; one that
// answered no live stream, or whose analysis outlasted its time, is not for
// a while. How it answered is told to LiveSources' health.
func (s *Service) AnalyzeLive(ctx context.Context, version library.Version) (media.Analysis, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return analysis, nil
	}
	if analysis, at, ok := s.shape(ctx, version.ID); ok && time.Since(at) < shapeLife {
		s.analyses.Put(version.ID, analysis)
		return analysis, nil
	}
	if failure, failed := s.failures.Get(version.ID); failed {
		return media.Analysis{}, failure.err
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
// found. The source has its feed's times to answer (see sniff), and the
// analysis liveProbeCap from then on. A stream whose list names a licence
// Polyfin cannot have fails before the source is asked, with ErrLiveDRM; a
// DASH stream is read with its ClearKey keys, then checked (see
// checkDASH).
func (s *Service) probeLive(ctx context.Context, version library.Version) (media.Analysis, error) {
	if version.Protection.Unsupported() {
		err := drmFailure(version.Protection)
		s.liveFailed(ctx, version, err, true)
		return media.Analysis{}, err
	}
	reader, err := s.openFeed(ctx, version)
	if err != nil && !errors.Is(err, errManifest) {
		// A source may be slow once: one that sent nothing in time is asked
		// again at the next start, unless its health leaves it out.
		s.liveFailed(ctx, version, err, LiveFailure(err) != LiveTimeout)
		return media.Analysis{}, err
	}
	// The analysis joins the feed opened for it: it holds it open meanwhile.
	defer reader.Close()
	dash := errors.Is(err, errDASH)
	var options []string
	keyed := false
	if dash {
		if options, keyed, err = s.dashOptions(ctx, version); err != nil {
			s.liveFailed(ctx, version, err, LiveFailure(err) != LiveTimeout)
			return media.Analysis{}, err
		}
	}
	prober := s.ffprobe()
	prober.Timeout = min(prober.Timeout, liveProbeCap)
	probeCtx, cancel := context.WithTimeout(ctx, prober.Timeout)
	defer cancel()
	live := s.loopback.registerLiveFor(version, userOf(ctx))
	defer live.release()
	analysis, err := prober.ProbeLive(probeCtx, live.target, options...)
	timedOut := probeCtx.Err() != nil
	if err == nil && dash {
		// The check reads the stream again: it has its own time.
		checkCtx, cancel := context.WithTimeout(ctx, prober.Timeout)
		err = s.checkDASH(checkCtx, version, analysis, live.target, options, keyed)
		timedOut = checkCtx.Err() != nil
		cancel()
	}
	if err != nil {
		if timedOut {
			err = fmt.Errorf("%w: %w", ErrLiveTimeout, err)
		} else if dash && !s.readsDASH(ctx) {
			err = fmt.Errorf("%w: this server's FFmpeg has no DASH demuxer: %v", ErrLiveDead, err)
		} else if LiveFailure(err) == "" {
			err = fmt.Errorf("%w: %v", ErrLiveDead, err)
		}
		// A file of an HLS stream that answered no media tells why
		// ffprobe found none.
		if answered := live.answered(time.Time{}); answered != nil {
			err = fmt.Errorf("%w; %v", err, answered)
		}
		// The source answered: an analysis that outlasted its time would
		// be waited for again.
		s.liveFailed(ctx, version, err, true)
		return media.Analysis{}, err
	}
	s.analyses.Put(version.ID, analysis)
	s.saveShape(ctx, version.ID, analysis)
	s.report(ctx, version, nil)
	return analysis, nil
}

// liveFailed records a live stream's failure: told to LiveSources' health
// and, when kept and not a refusal, kept among the failures, which the
// next starts skip.
func (s *Service) liveFailed(ctx context.Context, version library.Version, err error, kept bool) {
	s.logger.Info("A channel's stream failed", "addon", version.Addon, "stream", version.Name, "failure", LiveFailure(err), "error", err)
	s.report(ctx, version, err)
	if kept && LiveFailure(err) != LiveRefused {
		s.fail(version, err)
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
	return err != nil || limit.Connections > 0
}

// Manifest reports whether a live analysis is of an HLS playlist or a
// DASH manifest, whose files are fetched one by one, rather than of one
// endless stream.
func Manifest(analysis media.Analysis) bool {
	return strings.Contains(","+analysis.Format+",", ",hls,") || DASH(analysis)
}

// Redirectable reports whether a player may be sent to a version's source
// itself: the source needs no headers and is on a public address.
func (s *Service) Redirectable(ctx context.Context, version library.Version) bool {
	return len(version.Headers) == 0 && s.public(ctx, version.URL)
}

// ServeLive relays a live stream, or a file of it, to a player: target is
// the address asked, the version's own when empty, an MPEG-TS stream read
// from its feed (see ServeFeed). The addresses an HLS
// playlist names are rewritten by link, given each address and the name
// to give its file, so that the player fetches them through Polyfin too,
// with the headers the source needs and within its confinement; a
// playlist naming an address that cannot be rewritten is refused. Other
// files are relayed as they come, byte ranges included, as EXT-X-BYTERANGE
// asks them, but for a picture in front of a segment's MPEG-TS, which is
// skipped (see pictureBefore). When the source does not answer, the player
// receives a 502 and the error is returned for logging. A file that
// answered no media, a refusal or a page, returns a fileAnswer telling
// it, once relayed. A DASH manifest is refused: it plays converted.
func (s *Service) ServeLive(w http.ResponseWriter, r *http.Request, version library.Version, target string, link func(target, name string) string) error {
	return s.serveLive(w, r, version, target, liveRelay{link: link})
}

// liveRelay names the files a relayed live stream refers to: link a file,
// given its address and the name to give it; folder an origin's root on
// the loopback interface (see rewriteManifest), nil for a player, to which
// no DASH manifest is relayed.
type liveRelay struct {
	link   func(target, name string) string
	folder func(origin string) string
}

// serveLive is ServeLive, relaying FFmpeg a DASH manifest when
// relay.folder is set.
func (s *Service) serveLive(w http.ResponseWriter, r *http.Request, version library.Version, target string, relay liveRelay) error {
	link := relay.link
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
	for name, value := range version.RequestHeaders() {
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
	body := bufio.NewReaderSize(response.Body, 64<<10)
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
	default:
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return fileAnswer{name: FileName(target), reply: replyOf(response, body)}
	}
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
	// A DASH manifest is rewritten for FFmpeg however the source answered,
	// a part of it from its first byte included, as FFmpeg asks for it.
	if response.StatusCode != http.StatusRequestedRangeNotSatisfiable && dashAnswer(response, body) {
		if relay.folder == nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return errPlayerManifest
		}
		data, err := io.ReadAll(io.LimitReader(body, maxPlaylist))
		if response.StatusCode == http.StatusPartialContent && !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes 0-") {
			err = fmt.Errorf("%w: a part of a manifest", ErrUnsafePlaylist)
		}
		base, _ := url.Parse(target)
		if response.Request != nil {
			base = response.Request.URL
		}
		var rewritten []byte
		if err == nil {
			rewritten, err = rewriteManifest(data, base, relay.folder)
		}
		if err != nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return err
		}
		w.Header().Set("Content-Type", "application/dash+xml")
		w.Header().Set("Cache-Control", "no-cache")
		// FFmpeg's dash demuxer reads a manifest to its end, which an
		// answer of chunks does not reach for it.
		w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
		_, err = w.Write(rewritten)
		return err
	}
	// What is no media, a page or an empty answer, is relayed as it is,
	// and told.
	var odd error
	if head, _ := body.Peek(source.ReplyHead); noMedia(source.Reply{Head: head}) {
		odd = fileAnswer{name: FileName(target), reply: replyOf(response, body)}
	}
	skip := 0
	if response.StatusCode == http.StatusOK {
		skip = pictureBefore(body)
	}
	for _, name := range forwardedResponse {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	if skip > 0 {
		w.Header().Del("Content-Length")
		if response.ContentLength > int64(skip) {
			w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength-int64(skip), 10))
		}
		_, _ = body.Discard(skip)
	}
	w.WriteHeader(response.StatusCode)
	buffer := copyBuffers.Get().(*[256 << 10]byte)
	defer copyBuffers.Put(buffer)
	if _, err = io.CopyBuffer(w, body, buffer[:]); err != nil {
		return err
	}
	return odd
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
// against base, the playlist's own, with the name of its file: a
// segment's as segmentName gives it, any other's its own (see FileName).
// It reads the playlist as FFmpeg's HLS demuxer does, which reads it
// through Polyfin too: a line ends at a carriage return, a line feed or a
// NUL, a tag's URI attribute may be quoted or not, and an address line is
// a segment but after #EXT-X-STREAM-INF, which names a variant's
// playlist. Inline data: addresses stay as they are; any other address
// that is not HTTP, or does not parse, refuses the playlist, as FFmpeg
// would fetch it itself.
func rewritePlaylist(data []byte, base *url.URL, link func(target, name string) string) ([]byte, error) {
	rewrite := func(ref string, segment bool) (string, error) {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(strings.ToLower(ref), "data:") {
			return ref, nil
		}
		target, err := base.Parse(ref)
		if err != nil || target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
			return "", fmt.Errorf("%w: %q", ErrUnsafePlaylist, ref)
		}
		address := target.String()
		name := FileName(address)
		if segment {
			name = segmentName(name)
		}
		return link(address, name), nil
	}
	var out bytes.Buffer
	variant := false
	lines := strings.FieldsFunc(string(data), func(r rune) bool { return r == '\r' || r == '\n' || r == 0 })
	for _, line := range lines {
		var err error
		switch {
		case strings.HasPrefix(line, "#"):
			variant = variant || strings.HasPrefix(line, "#EXT-X-STREAM-INF:")
			line, err = rewriteAttributes(line, func(ref string) (string, error) { return rewrite(ref, false) })
		case strings.TrimSpace(line) != "":
			line, err = rewrite(line, !variant)
			variant = false
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
// when it does not run; uri names the files it lists. The first waits for
// the segments the app asks for, and a lead (see hls.Manager.LivePlaylist).
func (s *Service) LivePlaylist(ctx context.Context, remux Remux, uri func(name string) string) ([]byte, error) {
	return s.segments.LivePlaylist(ctx, remux.key(), s.liveOpener(remux), uri, remux.MinSegments)
}

// LiveSegment opens a file the playlist of a live remux lists.
func (s *Service) LiveSegment(remux Remux, name string) (*os.File, error) {
	return s.segments.LiveSegment(remux.key(), name)
}

// liveOpener reads the live stream through the loopback relay, so that
// FFmpeg sends the headers it needs, stays within its confinement and, for
// MPEG-TS, shares the stream's feed. An MPEG-TS stream whose tracks are
// known is read with a short probe; opened again, after a run that failed
// quickly, it is analyzed again and read with FFmpeg's full probe. A DASH
// stream is read with its keys (see dashInput).
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
		live := s.loopback.registerLiveFor(remux.Version, remux.User)
		r := hls.Remux{Input: live.target, Refused: live.refused, Answered: live.answered, Channel: remux.Channel, Video: video.Index,
			Audio: audioOf(analysis, remux.Audio), Format: remux.Format}
		switch {
		case DASH(analysis):
			if r.InputOptions, err = s.dashInput(ctx, remux.Version); err != nil {
				live.release()
				return hls.Remux{}, nil, err
			}
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
		return r, live.release, nil
	}
}

// registerLive makes a version's live stream readable on the loopback
// interface until release is called: its playlists rewritten to name
// their files on the loopback interface too.
func (l *loopback) registerLive(version library.Version) (string, func()) {
	live := l.registerLiveFor(version, accounts.ID{})
	return live.target, live.release
}

// recordingSuffix ends the loopback key of a recording's live stream.
const recordingSuffix = "rec"

// registerRecording registers a version's live stream read for a
// recording, which keeps its tuner (see makeRoom).
func (l *loopback) registerRecording(version library.Version) (string, func()) {
	live := l.registerKey(version, randomKey()+"."+recordingSuffix)
	return live.target, live.release
}

// registerLiveFor registers a version's live stream read for user, whose
// oldest stream of the source may make room for it (see makeRoom).
func (l *loopback) registerLiveFor(version library.Version, user accounts.ID) liveRegistration {
	key := randomKey()
	if user != (accounts.ID{}) {
		key += "." + user.String()
	}
	return l.registerKey(version, key)
}

// liveRegistration is a live stream registered on the loopback interface,
// readable at target until release is called.
type liveRegistration struct {
	target string
	// refused tells the refusal of the source (see ErrLiveRefused) the
	// last request of the stream was answered with, nil for none.
	refused func() error
	// answered tells the last file of the stream that answered no media
	// since a time (see fileAnswer), nil for none: an encoding asks for
	// those since it last made a segment, an analysis for any.
	answered func(since time.Time) error
	release  func()
}

// liveTarget is a live stream registered on the loopback interface.
type liveTarget struct {
	version library.Version
	// refused is the refusal of the source the last request was answered
	// with, nil for none; answered the last file that answered no media,
	// at answeredAt. They are guarded by the loopback's mu.
	refused    error
	answered   error
	answeredAt time.Time
}

func (l *loopback) registerKey(version library.Version, key string) liveRegistration {
	t := &liveTarget{version: version}
	l.mu.Lock()
	l.feeds[key] = t
	l.mu.Unlock()
	guarded := func(field *error) func() error {
		return func() error {
			l.mu.Lock()
			defer l.mu.Unlock()
			return *field
		}
	}
	return liveRegistration{
		target:  l.liveURL(key, "", FileName(version.URL)),
		refused: guarded(&t.refused),
		answered: func(since time.Time) error {
			l.mu.Lock()
			defer l.mu.Unlock()
			if t.answeredAt.Before(since) {
				return nil
			}
			return t.answered
		},
		release: func() {
			l.mu.Lock()
			delete(l.feeds, key)
			l.mu.Unlock()
		},
	}
}

// liveURL is the loopback address of a file of a live stream, target
// being its source's address, empty for the stream itself, and name the
// name its file is given.
func (l *loopback) liveURL(key, target, name string) string {
	encoded := "-"
	if target != "" {
		encoded = base64.RawURLEncoding.EncodeToString([]byte(target))
	}
	return "http://" + l.listener.Addr().String() + "/live/" + key + "/" + encoded + "/" + url.PathEscape(name)
}

// serveLive answers FFmpeg's and ffprobe's requests for a live stream
// registered on the loopback interface: /live/{key}/{target}/{name}, or
// /live/{key}/~{origin}/{path} for a file a DASH manifest names (see
// folderURL).
func (l *loopback) serveLive(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	l.mu.Lock()
	live, ok := l.feeds[parts[0]]
	l.mu.Unlock()
	target := ""
	switch {
	case strings.HasPrefix(parts[1], "~"):
		var found bool
		target, found = folderTarget(r, parts[0], parts[1])
		ok = ok && found
	case parts[1] != "-":
		decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
		ok = ok && err == nil
		target = string(decoded)
	}
	if !ok || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	relay := liveRelay{
		link:   func(file, name string) string { return l.liveURL(parts[0], file, name) },
		folder: func(origin string) string { return l.folderURL(parts[0], origin) },
	}
	if _, rest, ok := strings.Cut(parts[0], "."); ok {
		if rest == recordingSuffix {
			r = r.WithContext(forRecording(r.Context()))
		} else if id, err := accounts.ParseID(rest); err == nil {
			r = r.WithContext(ForUser(r.Context(), id))
		}
	}
	// A new request clears the refusal of the one before it.
	l.mu.Lock()
	live.refused = nil
	l.mu.Unlock()
	err := l.live(w, r, live.version, target, relay)
	var answered fileAnswer
	odd := errors.As(err, &answered)
	if !errors.Is(err, ErrLiveRefused) {
		err = nil
	}
	l.mu.Lock()
	live.refused = err
	if odd {
		live.answered, live.answeredAt = answered, time.Now()
	}
	l.mu.Unlock()
}
