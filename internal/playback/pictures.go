package playback

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Picture subtitles, PGS and VobSub, are read into text in the background,
// with the setting on, so that the apps that take only text get them
// without the video being converted to burn them in (see package ocr).
// The tracks of a version are queued the first time an app that cannot
// draw them plays it, the chosen one first, and, with the setting saying
// so, the tracks of the local files a scan finds new or changed. No more
// tracks than the setting allows are read at once, each by one Tesseract
// process. The text is kept in the database by what Polyfin knows the
// track's file by, as seek lookbacks are (see seekFileOf): a track of a
// local file or disc by its folder and path, with its size and
// modification time, which change with what it holds; a track of an
// addon's file by its version; a picture file beside a local file by its
// folder and path, with its size and modification time. A copy is listed
// once its text is kept, after the version's tracks (see TextCopies).

// TextReader reads pictures into text: *ocr.Tesseract.
type TextReader interface {
	// Languages lists the languages it has data for, by Tesseract's codes.
	Languages(ctx context.Context) ([]string, error)
	// Read reads the pictures with the data of language.
	Read(ctx context.Context, pictures []ocr.Picture, language string) ([]subtitles.Cue, error)
}

const (
	// ocrReadTime bounds the reading of a track: copying it out of its
	// file, and Tesseract reading a movie's pictures, a thousand or two,
	// which takes minutes.
	ocrReadTime = time.Hour
	// maxPictureBytes bounds a picture file read whole, or a track copied
	// out of its file.
	maxPictureBytes = 256 << 20
	// readyFor is how long the copies found ready for a version are
	// remembered: a track read since shows from then on.
	readyFor = time.Minute
	// ocrFailureTTL is how long a track that could not be read is not
	// queued again.
	ocrFailureTTL = 6 * time.Hour
)

// The kinds of files the text of a track is kept by (see seekFileOf).
const (
	ocrLocal   = "local"
	ocrVersion = "version"
	ocrFile    = "file"
)

// ocrKey is what the text of a track is kept by: kind and name, as the
// subtitle_ocr table has them, and the track's FFmpeg index, -1 for a
// picture file.
type ocrKey struct {
	kind, name string
	stream     int
}

// pictureTrack is a picture subtitle track, inside a version or a file
// beside it.
type pictureTrack struct {
	key ocrKey
	// state is that of a local file, its size and modification time.
	state    string
	language string
	// slot is its place among the version's picture tracks, files first,
	// which its copy's index follows.
	slot int
	// index is its index among the version's streams.
	index int
	// file is the picture file it is; nil for a track inside the version.
	file *library.ExternalSubtitle
}

// ocrJob is a track to read, or for a local file a scan found, its
// tracks to queue.
type ocrJob struct {
	track pictureTrack
	// version is the version the track belongs to, analyzed, for a track
	// queued as it plays.
	version  library.Version
	analysis media.Analysis
	// input is what FFmpeg reads a local file or disc from.
	input media.Input
	// scanned is a video a scan found, for a job queuing its tracks.
	scanned *stremio.Stream
}

// pictureReader is the state of the reads of picture subtitles.
type pictureReader struct {
	mu     sync.Mutex
	ffmpeg string
	// dir is the folder the tracks FFmpeg copies out are written to, the
	// system's temporary folder when empty.
	dir     string
	reader  TextReader
	queue   []ocrJob
	queued  map[ocrKey]bool
	running int
	// missing are the languages tracks were in without data, by their
	// Tesseract code, with the track's code for those Tesseract has none.
	missing map[string]bool
	failed  *cache.Cache[ocrKey, error]
	// ready remembers the copies found for a version, until generation
	// changes, as a track is read.
	ready      *cache.Cache[readyKey, readyCopies]
	generation int
	texts      *cache.Cache[ocrKey, []subtitles.Cue]
}

// readyKey names a version with its count of subtitle files, which its
// copies' indexes follow.
type readyKey struct {
	version accounts.ID
	files   int
}

// readyCopies are the copies found for a version, in a generation.
type readyCopies struct {
	generation int
	copies     []TextCopy
}

// UseTesseract has picture subtitles read into text by reader, which
// FFmpeg, ffmpeg, copies out of local files into dir. Until it is called,
// none is.
func (s *Service) UseTesseract(ffmpeg, dir string, reader TextReader) {
	s.pictures.mu.Lock()
	defer s.pictures.mu.Unlock()
	s.pictures.ffmpeg, s.pictures.dir, s.pictures.reader = ffmpeg, dir, reader
	if s.pictures.queued == nil {
		s.pictures.queued = map[ocrKey]bool{}
		s.pictures.missing = map[string]bool{}
		s.pictures.failed = cache.New[ocrKey, error](2000, ocrFailureTTL)
		s.pictures.ready = cache.New[readyKey, readyCopies](2000, readyFor)
		s.pictures.texts = cache.New[ocrKey, []subtitles.Cue](200, time.Hour)
	}
}

// ocrMode is the setting, "off" while nothing reads pictures.
func (s *Service) ocrMode() string {
	s.pictures.mu.Lock()
	reader := s.pictures.reader
	s.pictures.mu.Unlock()
	if reader == nil {
		return "off"
	}
	return s.settings().SubtitleOCR
}

// copyBase is the index of a version's first text copy: past its files
// and its last track.
func copyBase(analysis media.Analysis, files []library.ExternalSubtitle) int {
	last := -1
	for _, stream := range analysis.Streams {
		last = max(last, stream.Index)
	}
	return len(files) + last + 1
}

// pictureTracks lists a version's picture subtitles: the picture files
// beside its local file, then the PGS and VobSub tracks inside it, those
// of an addon's file only when it is a Matroska file, which its index
// lets read. A track whose file cannot be told is left out, keeping its
// slot.
func (s *Service) pictureTracks(ctx context.Context, version library.Version, analysis media.Analysis, files []library.ExternalSubtitle) []pictureTrack {
	var tracks []pictureTrack
	slot := 0
	for i := range files {
		file := &files[i]
		if file.Format != "sup" && file.Format != "idx" {
			continue
		}
		if key, ok := strings.CutPrefix(file.URL, s.localBase()); ok {
			if name, state, err := s.localIdentity(ctx, key); err == nil {
				tracks = append(tracks, pictureTrack{key: ocrKey{ocrFile, name, -1}, state: state, language: file.Language, slot: slot, index: i,
					file: file})
			}
		}
		slot++
	}
	var where seekFile
	var known bool
	for _, stream := range analysis.Streams {
		if stream.Type != "subtitle" || !PictureSubtitle(stream.Codec) {
			continue
		}
		if !known {
			var err error
			if where, err = s.seekFileOf(ctx, version); err != nil {
				where.kind = ""
			}
			known = true
		}
		if where.kind == ocrLocal || where.kind == ocrVersion && matroska(analysis) {
			tracks = append(tracks, pictureTrack{key: ocrKey{where.kind, where.name, stream.Index}, state: where.state, language: stream.Language,
				slot: slot, index: len(files) + stream.Index})
		}
		slot++
	}
	return tracks
}

// localIdentity tells the file of a local folder a key names (see
// Inputs.Identity).
func (s *Service) localIdentity(ctx context.Context, key string) (path, state string, err error) {
	s.loopback.mu.Lock()
	inputs := s.loopback.inputs
	s.loopback.mu.Unlock()
	if inputs == nil {
		return "", "", errors.New("no local folders")
	}
	return inputs.Identity(ctx, key)
}

// TextCopies returns the text copies of a version's picture subtitles
// whose text is kept, with the setting on: files, those beside it after
// its title's. A copy is listed only when its pictures held text. What is
// found is remembered for a moment, until a track is read.
func (s *Service) TextCopies(ctx context.Context, version library.Version, analysis media.Analysis, files []library.ExternalSubtitle) []TextCopy {
	if s.ocrMode() == "off" || !hasPictures(analysis, files) {
		return nil
	}
	key := readyKey{version.ID, len(files)}
	s.pictures.mu.Lock()
	generation := s.pictures.generation
	s.pictures.mu.Unlock()
	if kept, ok := s.pictures.ready.Get(key); ok && kept.generation == generation {
		return kept.copies
	}
	ready, err := s.readyTracks(ctx, s.pictureTracks(ctx, version, analysis, files))
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Reading the text copies of picture subtitles failed", "error", err)
		}
		return nil
	}
	base := copyBase(analysis, files)
	copies := make([]TextCopy, len(ready))
	for i, track := range ready {
		copies[i] = TextCopy{Original: track.index, Index: base + track.slot}
	}
	s.pictures.ready.Put(key, readyCopies{generation, copies})
	return copies
}

// hasPictures reports whether a version has picture subtitles, inside it
// or beside it.
func hasPictures(analysis media.Analysis, files []library.ExternalSubtitle) bool {
	return slices.ContainsFunc(analysis.Streams, func(s media.Stream) bool { return s.Type == "subtitle" && PictureSubtitle(s.Codec) }) ||
		slices.ContainsFunc(files, func(f library.ExternalSubtitle) bool { return f.Format == "sup" || f.Format == "idx" })
}

// readyTracks lists the tracks whose text is kept for their file as it
// is, in slot order.
func (s *Service) readyTracks(ctx context.Context, tracks []pictureTrack) ([]pictureTrack, error) {
	if len(tracks) == 0 {
		return nil, nil
	}
	kinds, names, streams := make([]string, len(tracks)), make([]string, len(tracks)), make([]int32, len(tracks))
	for i, t := range tracks {
		kinds[i], names[i], streams[i] = t.key.kind, t.key.name, int32(t.key.stream)
	}
	rows, err := s.db.Query(ctx, `SELECT o.kind, o.name, o.stream, o.file FROM subtitle_ocr o
		JOIN unnest($1::text[], $2::text[], $3::integer[]) AS k (kind, name, stream) ON o.kind = k.kind AND o.name = k.name AND o.stream = k.stream
		WHERE o.text <> ''`, kinds, names, streams)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ready []pictureTrack
	for rows.Next() {
		var key ocrKey
		var state string
		if err := rows.Scan(&key.kind, &key.name, &key.stream, &state); err != nil {
			return nil, err
		}
		for _, t := range tracks {
			if t.key == key && t.state == state {
				ready = append(ready, t)
			}
		}
	}
	slices.SortFunc(ready, func(a, b pictureTrack) int { return a.slot - b.slot })
	return ready, rows.Err()
}

// TextCopyCues returns the text of the copy of index among a version's
// streams (see TextCopies), and whether it is kept.
func (s *Service) TextCopyCues(ctx context.Context, version library.Version, analysis media.Analysis, files []library.ExternalSubtitle, index int) ([]subtitles.Cue, bool) {
	if s.ocrMode() == "off" {
		return nil, false
	}
	slot := index - copyBase(analysis, files)
	tracks := s.pictureTracks(ctx, version, analysis, files)
	i := slices.IndexFunc(tracks, func(t pictureTrack) bool { return t.slot == slot })
	if slot < 0 || i < 0 {
		return nil, false
	}
	return s.keptText(ctx, tracks[i])
}

// keptText reads the text kept of a track, for its file as it is.
func (s *Service) keptText(ctx context.Context, track pictureTrack) ([]subtitles.Cue, bool) {
	if cues, ok := s.pictures.texts.Get(track.key); ok {
		return cues, true
	}
	var state, text string
	err := s.db.QueryRow(ctx, "SELECT file, text FROM subtitle_ocr WHERE kind = $1 AND name = $2 AND stream = $3",
		track.key.kind, track.key.name, track.key.stream).Scan(&state, &text)
	if err != nil || state != track.state || text == "" {
		return nil, false
	}
	cues, err := subtitles.Parse([]byte(text))
	if err != nil {
		return nil, false
	}
	s.pictures.texts.Put(track.key, cues)
	return cues, true
}

// IsTextCopy reports whether index, among a version's streams, is past
// its tracks, where text copies are listed.
func IsTextCopy(analysis media.Analysis, files []library.ExternalSubtitle, index int) bool {
	return index >= copyBase(analysis, files)
}

// QueueTextCopies queues the reading of a version's picture subtitles, as
// an app that cannot draw them plays it: the one of index chosen first,
// among its streams; -1 for none. It returns at once.
func (s *Service) QueueTextCopies(ctx context.Context, version library.Version, analysis media.Analysis, files []library.ExternalSubtitle, chosen int) {
	if s.ocrMode() == "off" || !hasPictures(analysis, files) {
		return
	}
	files = slices.Clone(files)
	go s.queueTracks(context.WithoutCancel(ctx), version, analysis, files, chosen)
}

// queueTracks queues the reading of a version's picture subtitles, the one
// of index chosen first.
func (s *Service) queueTracks(ctx context.Context, version library.Version, analysis media.Analysis, files []library.ExternalSubtitle, chosen int) {
	tracks := s.pictureTracks(ctx, version, analysis, files)
	slices.SortStableFunc(tracks, func(a, b pictureTrack) int {
		switch {
		case a.index == chosen && b.index != chosen:
			return -1
		case b.index == chosen && a.index != chosen:
			return 1
		}
		return 0
	})
	input := media.Input{Path: version.URL}
	if s.ReadsInput(version) {
		var err error
		if input, err = s.input(ctx, version); err != nil {
			s.logger.Info("A disc's picture subtitles cannot be read into text", "error", err)
			return
		}
	}
	jobs := make([]ocrJob, len(tracks))
	for i, track := range tracks {
		jobs[i] = ocrJob{track: track, version: version, analysis: analysis, input: input}
	}
	s.enqueue(jobs...)
}

// QueueScanned queues the reading of the picture subtitles of local
// files a scan found new or changed, with the setting saying so: the
// tracks inside each and the picture files beside it, as Streams lists
// them.
func (s *Service) QueueScanned(videos []stremio.Stream) {
	if s.ocrMode() != "scans" {
		return
	}
	jobs := make([]ocrJob, len(videos))
	for i := range videos {
		jobs[i] = ocrJob{scanned: &videos[i], track: pictureTrack{key: ocrKey{"scan", videos[i].URL, -1}}}
	}
	s.enqueue(jobs...)
}

// enqueue adds jobs to the queue, but for tracks queued or read already,
// or that recently failed, and starts what the setting allows.
func (s *Service) enqueue(jobs ...ocrJob) {
	s.pictures.mu.Lock()
	defer s.pictures.mu.Unlock()
	for _, job := range jobs {
		if s.pictures.queued[job.track.key] {
			continue
		}
		if _, failed := s.pictures.failed.Get(job.track.key); failed {
			continue
		}
		s.pictures.queued[job.track.key] = true
		s.pictures.queue = append(s.pictures.queue, job)
	}
	s.startReads()
}

// startReads starts the jobs queued, as many at once as the setting
// allows. The caller holds s.pictures.mu.
func (s *Service) startReads() {
	for s.pictures.running < s.settings().SubtitleOCRTracks && len(s.pictures.queue) > 0 {
		job := s.pictures.queue[0]
		s.pictures.queue = s.pictures.queue[1:]
		s.pictures.running++
		go func() {
			s.runJob(job)
			s.pictures.mu.Lock()
			defer s.pictures.mu.Unlock()
			s.pictures.running--
			delete(s.pictures.queued, job.track.key)
			s.startReads()
		}()
	}
}

// runJob reads a track, or queues those of a local file a scan found.
func (s *Service) runJob(job ocrJob) {
	ctx, cancel := context.WithTimeout(context.Background(), ocrReadTime)
	defer cancel()
	if s.ocrMode() == "off" {
		return
	}
	if job.scanned != nil {
		s.enqueue(s.scannedJobs(ctx, *job.scanned)...)
		return
	}
	if err := s.readPictures(ctx, job); err != nil {
		s.pictures.failed.Put(job.track.key, err)
		s.logger.Info("A picture subtitle track could not be read into text", "addon", job.version.Addon, "error", err)
	}
}

// scannedJobs are the tracks of a local file a scan found, those inside it
// as ffprobe tells them.
func (s *Service) scannedJobs(ctx context.Context, video stremio.Stream) []ocrJob {
	var jobs []ocrJob
	for i, subtitle := range video.Subtitles {
		if subtitle.Format != "sup" && subtitle.Format != "idx" {
			continue
		}
		key, ok := strings.CutPrefix(subtitle.URL, s.localBase())
		if !ok {
			continue
		}
		if name, state, err := s.localIdentity(ctx, key); err == nil {
			file := library.ExternalSubtitle{URL: subtitle.URL, Format: subtitle.Format, Language: subtitle.Lang}
			jobs = append(jobs, ocrJob{track: pictureTrack{key: ocrKey{ocrFile, name, -1}, state: state, language: subtitle.Lang, index: i, file: &file}})
		}
	}
	version := library.Version{URL: video.URL}
	key, local := strings.CutPrefix(video.URL, s.localBase())
	if !local {
		key, local = strings.CutPrefix(video.URL, s.inputBase())
	}
	name, state, err := s.localIdentity(ctx, key)
	if !local || err != nil {
		return jobs
	}
	in := media.Input{Path: video.URL}
	if s.ReadsInput(version) {
		if in, err = s.input(ctx, version); err != nil {
			return jobs
		}
	}
	analysis, err := s.ffprobe().ProbeInput(ctx, in)
	if err != nil {
		s.logger.Info("A local file's picture subtitles could not be listed", "error", err)
		return jobs
	}
	for _, stream := range analysis.Streams {
		if stream.Type != "subtitle" || !PictureSubtitle(stream.Codec) {
			continue
		}
		language := stream.Language
		if code := in.Languages[stream.ID]; language == "" && code != "" {
			language = code
		}
		jobs = append(jobs, ocrJob{track: pictureTrack{key: ocrKey{ocrLocal, name, stream.Index}, state: state, language: language}, input: in})
	}
	return jobs
}

// readPictures reads a track into text and keeps it, unless it was read
// already for its file as it is. A track whose language has no data is
// not read: the language is told as missing.
func (s *Service) readPictures(ctx context.Context, job ocrJob) error {
	track := job.track
	var state string
	err := s.db.QueryRow(ctx, "SELECT file FROM subtitle_ocr WHERE kind = $1 AND name = $2 AND stream = $3",
		track.key.kind, track.key.name, track.key.stream).Scan(&state)
	if err == nil && state == track.state {
		return nil
	}
	s.pictures.mu.Lock()
	reader, ffmpeg, dir := s.pictures.reader, s.pictures.ffmpeg, s.pictures.dir
	s.pictures.mu.Unlock()
	language := ocr.Language(track.language, s.settings().Language)
	available, err := reader.Languages(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(available, language) {
		missing := cmp.Or(language, localization.LanguageCode(track.language))
		s.pictures.mu.Lock()
		s.pictures.missing[missing] = true
		s.pictures.mu.Unlock()
		s.logger.Info("A picture subtitle track is not read into text: Tesseract has no data for its language", "language", missing)
		return nil
	}
	started := time.Now()
	pictures, err := s.trackPictures(ctx, job, ffmpeg, dir)
	if err != nil {
		return err
	}
	cues, err := reader.Read(ctx, pictures, language)
	if err != nil {
		return err
	}
	text := ""
	if len(cues) > 0 {
		text = string(subtitles.SubRip(cues))
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO subtitle_ocr (kind, name, stream, file, language, text) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (kind, name, stream) DO UPDATE SET file = excluded.file, language = excluded.language, text = excluded.text, read_at = now()`,
		track.key.kind, track.key.name, track.key.stream, track.state, language, text); err != nil {
		return err
	}
	s.pictures.texts.Delete(track.key)
	s.pictures.mu.Lock()
	s.pictures.generation++
	s.pictures.mu.Unlock()
	s.logger.Info("Read a picture subtitle track into text", "addon", job.version.Addon, "language", language, "pictures", len(pictures),
		"cues", len(cues), "duration", time.Since(started).Round(time.Millisecond))
	return nil
}

// trackPictures decodes a track's pictures: a picture file read whole, a
// track of an addon's Matroska file read through its index, or a track of
// a local file or disc that FFmpeg copies out without conversion into dir.
func (s *Service) trackPictures(ctx context.Context, job ocrJob, ffmpeg, dir string) ([]ocr.Picture, error) {
	track := job.track
	switch {
	case track.file != nil:
		key := strings.TrimPrefix(track.file.URL, s.localBase())
		data, err := s.readLocal(ctx, key)
		if err != nil {
			return nil, err
		}
		if track.file.Format == "sup" {
			return ocr.PGS(data)
		}
		// A VobSub pair is named by its .idx file; the same key with the
		// .sub extension opens its .sub file.
		sub, err := s.readLocal(ctx, strings.TrimSuffix(key, path.Ext(key))+".sub")
		if err != nil {
			return nil, err
		}
		return ocr.VobSub(data, sub)
	case track.key.kind == ocrVersion:
		t, blocks, err := s.pictureBlocks(ctx, job.version, job.analysis, track.key.stream)
		if err != nil {
			return nil, err
		}
		return decodePictureBlocks(t, blocks)
	default:
		t, blocks, err := copyPictures(ctx, ffmpeg, dir, job.input, track.key.stream)
		if err != nil {
			return nil, err
		}
		return decodePictureBlocks(t, blocks)
	}
}

// readLocal reads a file of a local folder whole.
func (s *Service) readLocal(ctx context.Context, key string) ([]byte, error) {
	s.loopback.mu.Lock()
	open := s.loopback.local
	s.loopback.mu.Unlock()
	if open == nil {
		return nil, errors.New("no local folders")
	}
	file, _, err := open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPictureBytes+1))
	if err == nil && len(data) > maxPictureBytes {
		err = errors.New("the picture subtitle file is too large")
	}
	return data, err
}

// decodePictureBlocks decodes the blocks of a Matroska PGS or VobSub track.
func decodePictureBlocks(track container.Track, blocks []container.Block) ([]ocr.Picture, error) {
	switch track.CodecID {
	case "S_HDMV/PGS":
		return ocr.PGSBlocks(blocks)
	case "S_VOBSUB":
		return ocr.VobSubBlocks(track.CodecPrivate, blocks)
	}
	return nil, fmt.Errorf("not a picture subtitle track: %s", track.CodecID)
}

// pictureBlocks reads the blocks of a picture subtitle track of an addon's
// Matroska file through its index, as text and PGS tracks are read whole
// (see readBlocks): within the same bounds of requests, and of reads at
// once from its host and from every host.
func (s *Service) pictureBlocks(ctx context.Context, version library.Version, analysis media.Analysis, stream int) (container.Track, []container.Block, error) {
	release, err := s.hostReads.acquire(ctx, hostOf(version.URL))
	if err != nil {
		return container.Track{}, nil, err
	}
	defer release()
	select {
	case s.trackReads <- struct{}{}:
		defer func() { <-s.trackReads }()
	case <-ctx.Done():
		return container.Track{}, nil, ctx.Err()
	}
	src := s.open(version)
	defer src.Release()
	var m *container.Matroska
	err = s.readSized(ctx, version, analysis, src, func(size int64) error {
		var err error
		m, err = container.OpenMatroska(ctx, src, size)
		return err
	})
	if err != nil {
		return container.Track{}, nil, err
	}
	track, ok := streamTracks(analysis, m.Tracks())[stream]
	if !ok || !track.Decodable {
		return container.Track{}, nil, ErrNotLocated
	}
	blocks, err := m.SubtitleBlocks(ctx, &boundedFetcher{src: src, left: int64(trackRequests)}, track.Number)
	s.noteRanges(version, src)
	return track, blocks, err
}

// copyPictures has FFmpeg copy a track out of a local file or disc, without
// conversion, into a Matroska file in dir, and reads its blocks.
func copyPictures(ctx context.Context, ffmpeg, dir string, in media.Input, stream int) (container.Track, []container.Block, error) {
	out, err := os.CreateTemp(dir, "polyfin-pictures-*.mks")
	if err != nil {
		return container.Track{}, nil, err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	// The track keeps the source's timestamps, as remuxes do (-copyts), so
	// that its cues are on the timeline of the track's other deliveries.
	args := append([]string{"-nostdin", "-v", "error", "-copyts"}, in.Args()...)
	args = append(args, "-i", in.Path, "-map", "0:"+strconv.Itoa(stream), "-c", "copy", "-avoid_negative_ts", "disabled",
		"-fs", strconv.Itoa(maxPictureBytes), "-f", "matroska", "-y", out.Name())
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return container.Track{}, nil, fmt.Errorf("copy the track out: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	info, err := out.Stat()
	if err != nil {
		return container.Track{}, nil, err
	}
	r := fileReader{out}
	m, err := container.OpenMatroska(ctx, r, info.Size())
	if err != nil {
		return container.Track{}, nil, err
	}
	for _, track := range m.Tracks() {
		if track.Type == trackSubtitle {
			blocks, err := m.SubtitleBlocks(ctx, r, track.Number)
			return track, blocks, err
		}
	}
	return container.Track{}, nil, errors.New("FFmpeg copied no subtitle track")
}

// fileReader reads a file on disk as container reads files.
type fileReader struct{ file *os.File }

func (f fileReader) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	return f.file.ReadAt(p, off)
}

func (f fileReader) Fetch(_ context.Context, off int64, n int) ([]byte, error) {
	data := make([]byte, n)
	read, err := f.file.ReadAt(data, off)
	if errors.Is(err, io.EOF) && read > 0 {
		err = nil
	}
	return data[:read], err
}

// MissingLanguages lists the languages, by Tesseract's codes, of the
// tracks left unread for want of their data, which Tesseract still lacks.
func (s *Service) MissingLanguages(ctx context.Context) []string {
	s.pictures.mu.Lock()
	reader := s.pictures.reader
	missing := make([]string, 0, len(s.pictures.missing))
	for language := range s.pictures.missing {
		missing = append(missing, language)
	}
	s.pictures.mu.Unlock()
	if reader == nil || len(missing) == 0 {
		return nil
	}
	available, err := reader.Languages(ctx)
	if err != nil {
		return nil
	}
	missing = slices.DeleteFunc(missing, func(language string) bool { return slices.Contains(available, language) })
	slices.Sort(missing)
	return missing
}
