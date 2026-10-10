package playback

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// Record copies a channel's live stream into the MPEG-TS file at path as it
// comes, for user, until ctx ends or the stream does. The stream is read as
// live playback reads it, through the loopback relay, so that the headers
// it needs are sent and it stays within its confinement; the recording
// counts toward the user's and the server's live encodings (see
// hls.Manager.Record). session names the recording among them.
func (s *Service) Record(ctx context.Context, user accounts.ID, version library.Version, session, path string) error {
	key := hls.Key{Session: "record " + session, User: user.String(), Version: version.ID.String()}
	return s.segments.Record(ctx, key, func(ctx context.Context) (hls.Remux, func(), error) {
		analysis, err := s.AnalyzeLive(ctx, version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		video, ok := LiveVideo(analysis)
		if !ok {
			return hls.Remux{}, nil, ErrNotRemuxable
		}
		target, release := s.loopback.registerLive(version)
		r := hls.Remux{Input: target, Video: video.Index, Audio: liveAudio(analysis)}
		if Manifest(analysis) {
			r.InputOptions = media.LiveOptions
		}
		return r, release, nil
	}, path)
}

// liveAudio is the FFmpeg index of the audio track a recording keeps: the
// stream's default one, else its first; -1 for none.
func liveAudio(analysis media.Analysis) int {
	first := -1
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		if stream.Default {
			return stream.Index
		}
		if first < 0 {
			first = stream.Index
		}
	}
	return first
}

// FinishRecording joins the MPEG-TS parts of a recording into one Matroska
// file at path (see hls.Manager.Finish).
func (s *Service) FinishRecording(ctx context.Context, parts []string, path string) error {
	return s.segments.Finish(ctx, parts, path)
}

// FileVersion describes a file of Polyfin's own, a recording, as a version
// played like an addon's: analyzed, cut into HLS segments and relayed
// through the source cache, read on the loopback interface. id identifies
// the version.
func (s *Service) FileVersion(id accounts.ID, name, path string, size int64) library.Version {
	s.loopback.mu.Lock()
	s.loopback.files[id] = path
	s.loopback.mu.Unlock()
	return library.Version{ID: id, Item: id, Name: name, Filename: filepath.Base(path), Size: size,
		URL: "http://" + s.loopback.listener.Addr().String() + "/file/" + s.loopback.fileKey + "/" + id.String() + filepath.Ext(path)}
}

// ForgetFile stops serving the file of a version FileVersion described, and
// forgets what was learned of it.
func (s *Service) ForgetFile(id accounts.ID) {
	s.loopback.mu.Lock()
	delete(s.loopback.files, id)
	s.loopback.mu.Unlock()
	s.analyses.Delete(id)
	s.failures.Delete(id)
	s.indexes.Delete(id)
	s.unindexed.Delete(id)
}

// serveFile answers ffprobe's, FFmpeg's and the source cache's requests for
// a file FileVersion described: /file/{fileKey}/{id}.{extension}.
func (l *loopback) serveFile(w http.ResponseWriter, r *http.Request, rest string) {
	key, name, _ := strings.Cut(rest, "/")
	raw, _, _ := strings.Cut(name, ".")
	id, err := accounts.ParseID(raw)
	l.mu.Lock()
	path, ok := l.files[id]
	l.mu.Unlock()
	if key != l.fileKey || err != nil || !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

// LocalFiles serves the files of local folders and network shares on the
// loopback interface, as FileVersion serves recordings, played like an
// addon's files: open finds the file a key names, with its modification
// time (see localfiles.Service.Open). It returns the address the keys are
// appended to.
func (s *Service) LocalFiles(open func(ctx context.Context, key string) (io.ReadSeekCloser, time.Time, error)) string {
	s.loopback.mu.Lock()
	s.loopback.local = open
	s.loopback.mu.Unlock()
	return "http://" + s.loopback.listener.Addr().String() + "/local/" + s.loopback.fileKey + "/"
}

// serveLocal answers the requests for a file of a local folder or network
// share, byte ranges included: /local/{fileKey}/{key}.
func (l *loopback) serveLocal(w http.ResponseWriter, r *http.Request, rest string) {
	key, name, _ := strings.Cut(rest, "/")
	l.mu.Lock()
	open := l.local
	l.mu.Unlock()
	if key != l.fileKey || open == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	file, modified, err := open(r.Context(), name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	http.ServeContent(w, r, name, modified, file)
}
