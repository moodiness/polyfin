package playback

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Subtitle tracks read whole through a version's Matroska index.
//
// Some apps, jellyfin-web first, take subtitles only as files they download
// whole when they show them. The tracks inside a version reached them only
// once remuxes had read the whole version. The Cues of a Matroska file,
// though, usually list every block of its subtitle tracks: a whole track is
// read from the file's head and Cues, one Cluster to check that the Cues
// list every block, and small spans around the blocks, rather than the
// whole file. A host that serves several ranges at once takes a few
// requests; one that serves a range at a time takes about one per block.

const (
	// locateTime bounds reading a version's head and Cues: a few large
	// reads.
	locateTime = 30 * time.Second
	// trackReadTime bounds reading a track's blocks. A few batched
	// requests take seconds; a host that answers one range at a time takes
	// one request per block or so, paced, which for a film can take minutes.
	// The track is kept once read, for later playbacks.
	trackReadTime = 15 * time.Minute
	// hostTrackReads bounds the tracks read at once from one host, to stay
	// gentle with it, and maxTrackReads those read at once from every host:
	// a slow host delays its own tracks only.
	hostTrackReads = 2
	maxTrackReads  = 4
	// maxTrackBytes bounds a track's file: dialogue takes a few hundred
	// kilobytes, heavy ASS typesetting a few megabytes.
	maxTrackBytes = 32 << 20
)

// ErrNotLocated reports a subtitle stream its version's index does not
// list block by block.
var ErrNotLocated = errors.New("the subtitle track is not located by the index")

// Track is a subtitle track read whole: the file it makes, in its format.
type Track struct {
	// Format is "srt", "vtt" or "ass".
	Format string
	Data   []byte
}

// trackKey names a version's subtitle stream, by its FFmpeg index.
type trackKey struct {
	version accounts.ID
	stream  int
}

// indexedCodecs maps the Matroska codecs of the subtitle tracks read whole
// to the FFmpeg codecs analyses name them by. FFmpeg reads SSA as ASS, and
// plain text tracks as text, which is written out as SubRip. It cannot
// read mkvmerge's WebVTT tracks, S_TEXT/WEBVTT, and names their codec
// unknown: they are not read whole, as no app is told what they hold.
var indexedCodecs = map[string]string{
	"S_TEXT/UTF8":        "subrip",
	"S_TEXT/ASCII":       "text",
	"D_WEBVTT/SUBTITLES": "webvtt",
	"D_WEBVTT/CAPTIONS":  "webvtt",
	"S_TEXT/ASS":         "ass",
	"S_TEXT/SSA":         "ass",
	"S_ASS":              "ass",
	"S_SSA":              "ass",
}

// matroska reports whether an analysis is of a Matroska or WebM file.
func matroska(analysis media.Analysis) bool {
	return slices.Contains(strings.Split(analysis.Format, ","), "matroska")
}

// streamTracks pairs a version's FFmpeg streams with its Matroska tracks.
// FFmpeg makes a stream of each video, audio, subtitle and metadata track
// that names its codec, in track order, then of its attachments: the first
// streams are the tracks. Nothing pairs when the two disagree on a track,
// so that a track is never read for another.
func streamTracks(analysis media.Analysis, tracks []container.Track) map[int]container.Track {
	var kept []container.Track
	for _, track := range tracks {
		switch track.Type {
		case trackVideo, trackAudio, trackSubtitle, trackMetadata:
			if track.CodecID != "" {
				kept = append(kept, track)
			}
		}
	}
	streams := slices.SortedFunc(slices.Values(analysis.Streams), func(a, b media.Stream) int { return cmp.Compare(a.Index, b.Index) })
	if len(streams) < len(kept) {
		return nil
	}
	pairs := make(map[int]container.Track, len(kept))
	for i, track := range kept {
		if stream := streams[i]; stream.Index == i && sameTrack(stream, track) {
			pairs[i] = track
			continue
		}
		return nil
	}
	return pairs
}

// Matroska TrackTypes.
const (
	trackVideo    = 1
	trackAudio    = 2
	trackSubtitle = 0x11
	trackMetadata = 0x21
)

// sameTrack reports whether an FFmpeg stream is what a Matroska track
// makes: the same kind, and for the subtitles read whole, the same codec.
func sameTrack(stream media.Stream, track container.Track) bool {
	switch track.Type {
	case trackVideo:
		return stream.Type == "video"
	case trackAudio:
		return stream.Type == "audio"
	case trackSubtitle:
		codec, indexed := indexedCodecs[track.CodecID]
		return stream.Type == "subtitle" && (!indexed || codec == stream.Codec)
	default:
		return stream.Type == "data"
	}
}

// indexedStreams reports whether an analysis has subtitle streams that can
// be read whole through a Matroska index.
func indexedStreams(analysis media.Analysis) bool {
	return matroska(analysis) && slices.ContainsFunc(analysis.Streams, func(stream media.Stream) bool {
		return stream.Type == "subtitle" && slices.Contains(slices.Collect(maps.Values(indexedCodecs)), stream.Codec)
	})
}

// SubtitlesLocated returns the text subtitle streams of a version, by FFmpeg
// index, whose blocks its Matroska index lists, so that they can be read
// whole: remembered, kept, or read from the version's head and Cues now.
// None when the version is not a Matroska file or its index lists none.
func (s *Service) SubtitlesLocated(ctx context.Context, version library.Version, analysis media.Analysis) map[int]bool {
	if !indexedStreams(analysis) {
		return nil
	}
	streams, ok := s.locations.Get(version.ID)
	if !ok {
		if _, failed := s.unlocated.Get(version.ID); failed {
			return nil
		}
		result, err, _ := s.flight.Do("located "+version.ID.String(), func() (any, error) {
			// Kept like an analysis: read to the end even once the request
			// that started it is canceled.
			return s.locate(context.WithoutCancel(ctx), version, analysis)
		})
		if err != nil {
			return nil
		}
		streams = result.([]int)
	}
	located := make(map[int]bool, len(streams))
	for _, stream := range streams {
		if _, failed := s.untracked.Get(trackKey{version.ID, stream}); !failed {
			located[stream] = true
		}
	}
	return located
}

// locate reads which text subtitle streams of a version its index lists,
// from the database or the version itself.
func (s *Service) locate(ctx context.Context, version library.Version, analysis media.Analysis) ([]int, error) {
	var streams []int32
	err := s.db.QueryRow(ctx, "SELECT located FROM media_subtitle_index WHERE version_id = $1", version.ID).Scan(&streams)
	if err == nil {
		located := make([]int, len(streams))
		for i, stream := range streams {
			located[i] = int(stream)
		}
		s.locations.Put(version.ID, located)
		return located, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Warn("Reading a subtitle index failed", "error", err)
	}
	ctx, cancel := context.WithTimeout(ctx, locateTime)
	defer cancel()
	located, err := s.readLocations(ctx, version, analysis)
	if err != nil {
		// The host or the network failed: asked again a while later.
		s.logger.Info("The subtitles of a version could not be located", "addon", version.Addon, "error", err)
		s.unlocated.Put(version.ID, err)
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO media_subtitle_index (version_id, located) VALUES ($1, $2)
		ON CONFLICT (version_id) DO UPDATE SET located = excluded.located, indexed_at = now()`, version.ID, located); err != nil {
		s.logger.Warn("Saving a subtitle index failed", "error", err)
	}
	s.locations.Put(version.ID, located)
	return located, nil
}

// readLocations reads a version's head and Cues. A file without an index,
// or with one that cannot be read, locates nothing, for good.
func (s *Service) readLocations(ctx context.Context, version library.Version, analysis media.Analysis) ([]int, error) {
	src := s.open(version)
	defer src.Release()
	size, err := s.sizeOf(ctx, src, analysis)
	if err != nil {
		return nil, err
	}
	m, err := container.OpenMatroska(ctx, src, size)
	var counts map[uint64]int
	if err == nil {
		counts, err = m.SubtitleLocations(ctx)
	}
	switch {
	case errors.Is(err, container.ErrNoIndex):
		return []int{}, nil
	case errors.Is(err, container.ErrUnreadable):
		s.logger.Info("The index of a version cannot be read", "addon", version.Addon, "error", err)
		return []int{}, nil
	case err != nil:
		return nil, err
	}
	located := []int{}
	for stream, track := range streamTracks(analysis, m.Tracks()) {
		if _, indexed := indexedCodecs[track.CodecID]; indexed && track.Type == trackSubtitle && track.Decodable && counts[track.Number] > 0 {
			located = append(located, stream)
		}
	}
	slices.Sort(located)
	return located, nil
}

// sizeOf is a version's size: analyzed, or asked of its source.
func (s *Service) sizeOf(ctx context.Context, src interface {
	Size(context.Context) (int64, error)
}, analysis media.Analysis) (int64, error) {
	if analysis.Size > 0 {
		return analysis.Size, nil
	}
	return src.Size(ctx)
}

// SubtitleTrack returns a version's text subtitle stream, by FFmpeg index,
// read whole through its index: remembered, kept, or read now, which may
// take a while. ErrNotLocated when the index does not list its blocks.
func (s *Service) SubtitleTrack(ctx context.Context, version library.Version, analysis media.Analysis, stream int) (Track, error) {
	key := trackKey{version.ID, stream}
	if track, ok := s.tracks.Get(key); ok {
		return track, nil
	}
	if track, ok := s.keptTrack(ctx, key); ok {
		return track, nil
	}
	if err, failed := s.untracked.Get(key); failed {
		return Track{}, err
	}
	if !s.SubtitlesLocated(ctx, version, analysis)[stream] {
		return Track{}, ErrNotLocated
	}
	results := s.flight.DoChan("track "+version.ID.String()+" "+strconv.Itoa(stream), func() (any, error) {
		// Read to the end even when the request that started it gives up:
		// the app asks again, or the next one does.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), trackReadTime)
		defer cancel()
		return s.readTrack(ctx, version, analysis, stream)
	})
	select {
	case result := <-results:
		if result.Err != nil {
			return Track{}, result.Err
		}
		return result.Val.(Track), nil
	case <-ctx.Done():
		return Track{}, ctx.Err()
	}
}

// PrefetchSubtitle reads a track whole in the background, so that it is
// ready when the app asks for it.
func (s *Service) PrefetchSubtitle(version library.Version, analysis media.Analysis, stream int) {
	go func() {
		_, _ = s.SubtitleTrack(context.Background(), version, analysis, stream)
	}()
}

// keptTrack reads a track read whole before from the database.
func (s *Service) keptTrack(ctx context.Context, key trackKey) (Track, bool) {
	var track Track
	err := s.db.QueryRow(ctx, "SELECT format, data FROM media_subtitle_tracks WHERE version_id = $1 AND stream = $2",
		key.version, key.stream).Scan(&track.Format, &track.Data)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("Reading a subtitle track failed", "error", err)
		}
		return Track{}, false
	}
	s.tracks.Put(key, track)
	return track, true
}

// readTrack reads a track's blocks through the version's index and keeps
// the file they make. A track whose file cannot be read so is not offered
// anymore; one its host failed to serve is not offered for a while.
func (s *Service) readTrack(ctx context.Context, version library.Version, analysis media.Analysis, stream int) (Track, error) {
	// The host's slot first: a read waiting for a slot over every host
	// holds up only its own host's reads.
	release, err := s.hostReads.acquire(ctx, hostOf(version.URL))
	if err != nil {
		return Track{}, err
	}
	defer release()
	select {
	case s.trackReads <- struct{}{}:
		defer func() { <-s.trackReads }()
	case <-ctx.Done():
		return Track{}, ctx.Err()
	}
	key := trackKey{version.ID, stream}
	started := time.Now()
	track, err := s.readBlocks(ctx, version, analysis, stream)
	if err != nil {
		s.logger.Info("A subtitle track could not be read through its index", "addon", version.Addon, "error", err)
		s.untracked.Put(key, err)
		if describesFile(err) {
			s.unlocate(ctx, version.ID, stream)
		}
		return Track{}, err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO media_subtitle_tracks (version_id, stream, format, data) VALUES ($1, $2, $3, $4)
		ON CONFLICT (version_id, stream) DO UPDATE SET format = excluded.format, data = excluded.data, read_at = now()`,
		version.ID, stream, track.Format, track.Data); err != nil {
		s.logger.Warn("Saving a subtitle track failed", "error", err)
	}
	s.tracks.Put(key, track)
	s.logger.Debug("Read a subtitle track through its index", "addon", version.Addon, "format", track.Format,
		"bytes", len(track.Data), "duration", time.Since(started))
	return track, nil
}

func (s *Service) readBlocks(ctx context.Context, version library.Version, analysis media.Analysis, stream int) (Track, error) {
	src := s.open(version)
	defer src.Release()
	size, err := s.sizeOf(ctx, src, analysis)
	if err != nil {
		return Track{}, err
	}
	m, err := container.OpenMatroska(ctx, src, size)
	if err != nil {
		return Track{}, err
	}
	track, ok := streamTracks(analysis, m.Tracks())[stream]
	if _, indexed := indexedCodecs[track.CodecID]; !ok || !indexed || !track.Decodable {
		return Track{}, ErrNotLocated
	}
	blocks, err := m.SubtitleBlocks(ctx, src, track.Number)
	if err != nil {
		return Track{}, err
	}
	file, err := trackFile(track, blocks)
	if err != nil {
		return Track{}, fmt.Errorf("%w: %v", errTrackFile, err)
	}
	if len(file.Data) > maxTrackBytes {
		return Track{}, fmt.Errorf("%w: %d bytes", errTrackFile, len(file.Data))
	}
	return file, nil
}

// errTrackFile reports blocks that do not make a subtitle file Polyfin
// serves.
var errTrackFile = errors.New("the blocks of the subtitle track do not make a file")

// describesFile reports whether reading a track failed for what its file
// holds, which reading it again from any host would meet again.
func describesFile(err error) bool {
	return errors.Is(err, container.ErrUnreadable) || errors.Is(err, container.ErrNoIndex) ||
		errors.Is(err, ErrNotLocated) || errors.Is(err, errTrackFile)
}

// unlocate stops offering a stream whose track cannot be read whole, for
// good: the version's index is kept without it.
func (s *Service) unlocate(ctx context.Context, version accounts.ID, stream int) {
	if located, ok := s.locations.Get(version); ok {
		s.locations.Put(version, slices.DeleteFunc(slices.Clone(located), func(i int) bool { return i == stream }))
	}
	if _, err := s.db.Exec(ctx, "UPDATE media_subtitle_index SET located = array_remove(located, $2::integer) WHERE version_id = $1",
		version, stream); err != nil {
		s.logger.Warn("Saving a subtitle index failed", "error", err)
	}
}

// hostSlots bounds the reads under way from each host.
type hostSlots struct {
	mu    sync.Mutex
	hosts map[string]chan struct{}
}

// acquire waits for one of host's slots, which release gives back.
func (h *hostSlots) acquire(ctx context.Context, host string) (release func(), err error) {
	h.mu.Lock()
	slots, ok := h.hosts[host]
	if !ok {
		if h.hosts == nil {
			h.hosts = map[string]chan struct{}{}
		}
		slots = make(chan struct{}, hostTrackReads)
		h.hosts[host] = slots
	}
	h.mu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// hostOf is the host a version's URL names.
func hostOf(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// trackFile writes a track's blocks as the file they make: an ASS track's
// script as its muxer split it, header and events, styles kept.
func trackFile(track container.Track, blocks []container.Block) (Track, error) {
	switch indexedCodecs[track.CodecID] {
	case "subrip", "text":
		return Track{Format: "srt", Data: subtitles.SubRip(blockCues(blocks))}, nil
	case "webvtt":
		// WebM's WebVTT blocks, which FFmpeg writes in Matroska files too,
		// begin with the cue's identifier and settings lines, empty or not.
		cues := make([]container.Block, len(blocks))
		for i, block := range blocks {
			block.Data = cueText(block.Data)
			cues[i] = block
		}
		return Track{Format: "vtt", Data: subtitles.WebVTT(blockCues(cues))}, nil
	case "ass":
		events := make([]subtitles.MatroskaEvent, len(blocks))
		for i, block := range blocks {
			events[i] = subtitles.MatroskaEvent{Start: block.Start, Duration: block.Duration, Data: block.Data}
		}
		script, err := subtitles.MatroskaScript(track.CodecPrivate, events)
		if err != nil {
			return Track{}, err
		}
		return Track{Format: "ass", Data: script.Bytes()}, nil
	}
	return Track{}, ErrNotLocated
}

// cueText is the text of a WebM WebVTT block, after the cue's identifier
// and settings lines: nothing when the block lacks them.
func cueText(data []byte) []byte {
	for range 2 {
		_, rest, found := bytes.Cut(data, []byte("\n"))
		if !found {
			return nil
		}
		data = rest
	}
	return data
}

// blockCues are the cues text blocks hold. A block without a duration
// lasts until the next one starts.
func blockCues(blocks []container.Block) []subtitles.Cue {
	cues := make([]subtitles.Cue, 0, len(blocks))
	for i, block := range blocks {
		text := strings.TrimRight(strings.ReplaceAll(string(block.Data), "\r\n", "\n"), "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		end := block.Start + block.Duration
		if block.Duration <= 0 && i+1 < len(blocks) {
			end = blocks[i+1].Start
		}
		cues = append(cues, subtitles.Cue{Start: block.Start, End: end, Lines: strings.Split(text, "\n")})
	}
	return cues
}

// keptCues returns the cues of a track read whole through its index, if
// one was.
func (s *Service) keptCues(ctx context.Context, key trackKey) ([]subtitles.Cue, bool) {
	if cues, ok := s.trackCues.Get(key); ok {
		return cues, true
	}
	track, ok := s.tracks.Get(key)
	if !ok {
		if track, ok = s.keptTrack(ctx, key); !ok {
			return nil, false
		}
	}
	// The file of a track read whole is SubRip, WebVTT or ASS, which Parse
	// all reads.
	cues, err := subtitles.Parse(track.Data)
	if err != nil {
		return nil, false
	}
	s.trackCues.Put(key, cues)
	return cues, true
}
