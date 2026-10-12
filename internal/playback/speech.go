package playback

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/speech"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// The addons' subtitles are written for one release of a title, not
// always the version played. Polyfin times them to the version against
// when the version has speech: a text track inside its file, whose times
// are the file's, else the speech it finds in three stretches of its
// audio. It finds it once per version, in the background, at the
// version's first play, and keeps it.
const (
	// speechStretch is how long each stretch of audio looked at lasts, at
	// most, and minStretch at least.
	speechStretch = 3 * time.Minute
	minStretch    = 30 * time.Second
	// speechBudget bounds what is read of an addon's file to find its
	// speech: its stretches are as long as the budget reads at its
	// bitrate, and past it, or with stretches shorter than minStretch, the
	// version has no speech reference.
	speechBudget = 300_000_000
	// Reading a stretch of an addon's file takes more than its share of
	// the file: readOverhead more, FFmpeg starting up to keyframeLead
	// before it, at the keyframe before it, and readAhead more sent to
	// FFmpeg than it decodes before it stops.
	readOverhead = 1.1
	keyframeLead = 10 * time.Second
	readAhead    = 8 << 20
	// speechQueue bounds the versions waiting for their speech to be
	// found, one at a time; a version dropped is queued again by its next
	// play.
	speechQueue = 8
	// speechTime bounds the finding of a version's speech.
	speechTime = 30 * time.Minute
	// minTrackCues is how many cues a track inside the file needs to be
	// the reference: a track of signs and songs is not the dialogue.
	minTrackCues = 20
	// minSpeech is how much speech the audio needs to be the reference.
	minSpeech = 10 * time.Second
)

// Reference kinds, as version_speech keeps them.
const (
	speechTrack = "track"
	speechAudio = "audio"
	speechNone  = "none"
)

// errNoSpeech reports a version whose speech cannot be found, for good:
// it has no speech reference.
var errNoSpeech = errors.New("the version has no speech reference")

// errOverBudget reports a read past its budget (see readBudget).
var errOverBudget = errors.New("the read went past its budget")

// readBudget counts what FFmpeg reads of a source, and refuses to serve
// more than limit bytes.
type readBudget struct {
	limit int64
	read  atomic.Int64
}

func (b *readBudget) over() bool {
	return b.read.Load() > b.limit
}

// budgetWriter serves a source within its budget.
type budgetWriter struct {
	http.ResponseWriter
	budget *readBudget
}

func (w budgetWriter) Write(p []byte) (int, error) {
	if w.budget.read.Add(int64(len(p))) > w.budget.limit {
		return 0, errOverBudget
	}
	return w.ResponseWriter.Write(p)
}

// speechState is what the service knows of versions' speech and of the
// subtitles synced to them.
type speechState struct {
	mu      sync.Mutex
	jobs    []library.Version
	running bool
	// found are the versions whose speech was looked for, unknown those
	// it was not lately, and failed those it could not be found for, for
	// a while.
	found   *cache.Cache[accounts.ID, storedSpeech]
	unknown *cache.Cache[accounts.ID, struct{}]
	failed  *cache.Cache[accounts.ID, error]
	// timings are the subtitles synced to each version, by subtitle.
	timingsMu sync.Mutex
	timings   *cache.Cache[accounts.ID, map[accounts.ID]speech.Timing]
	// budget is speechBudget, which tests lower.
	budget int64
}

func newSpeechState() speechState {
	return speechState{
		found:   cache.New[accounts.ID, storedSpeech](2000, time.Hour),
		unknown: cache.New[accounts.ID, struct{}](2000, time.Minute),
		failed:  cache.New[accounts.ID, error](2000, failureTTL),
		timings: cache.New[accounts.ID, map[accounts.ID]speech.Timing](2000, time.Hour),
		budget:  speechBudget,
	}
}

// storedSpeech is a version's reference, of a kind.
type storedSpeech struct {
	kind      string
	reference speech.Reference
}

// FindSpeech finds when a version has speech, in the background, when the
// settings sync the addons' subtitles and it was not found before: from a
// text track inside its file, else from its audio (see speechReference).
// A version that is not one file, or that holds one of its source's
// connections, as a replay does, is not looked at. A version recently
// failed or already waiting is not queued; neither is one when the queue
// is full.
func (s *Service) FindSpeech(version library.Version) {
	if !s.settings().SyncAddonSubtitles || s.ReadsInput(version) || version.HoldsConnection {
		return
	}
	if _, failed := s.speech.failed.Get(version.ID); failed {
		return
	}
	if _, found := s.speech.found.Get(version.ID); found {
		return
	}
	s.speech.mu.Lock()
	defer s.speech.mu.Unlock()
	if len(s.speech.jobs) == speechQueue || slices.ContainsFunc(s.speech.jobs, func(v library.Version) bool { return v.ID == version.ID }) {
		return
	}
	s.speech.jobs = append(s.speech.jobs, version)
	if !s.speech.running {
		s.speech.running = true
		go s.findQueuedSpeech()
	}
}

// findQueuedSpeech finds the speech of the versions queued, one at a time,
// until none is left.
func (s *Service) findQueuedSpeech() {
	for {
		s.speech.mu.Lock()
		if len(s.speech.jobs) == 0 {
			s.speech.running = false
			s.speech.mu.Unlock()
			return
		}
		version := s.speech.jobs[0]
		s.speech.jobs = s.speech.jobs[1:]
		s.speech.mu.Unlock()
		ctx, cancel := context.WithTimeout(Background(context.Background()), speechTime)
		s.findSpeech(ctx, version)
		cancel()
	}
}

// findSpeech finds and keeps when a version has speech, unless it was
// before. A version whose speech cannot be found for now is not tried
// again for a while.
func (s *Service) findSpeech(ctx context.Context, version library.Version) {
	if _, found := s.storedSpeech(ctx, version.ID); found {
		return
	}
	started := time.Now()
	kind, reference, err := s.speechReference(ctx, version)
	if err != nil {
		s.speech.failed.Put(version.ID, err)
		s.logger.Info("The speech of a version could not be found", "addon", version.Addon, "error", err)
		return
	}
	if err := s.saveSpeech(ctx, version.ID, kind, reference); err != nil {
		s.logger.Warn("Saving the speech of a version failed", "error", err)
		return
	}
	s.logger.Debug("Found when a version has speech", "addon", version.Addon, "reference", kind,
		"speech", speech.Total(reference.Speech).Round(time.Second), "duration", time.Since(started).Round(time.Millisecond))
}

// speechReference finds when a version has speech: from a text track
// inside its file, in any language, else from its audio. A version whose
// speech cannot be found for good has none.
func (s *Service) speechReference(ctx context.Context, version library.Version) (string, speech.Reference, error) {
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return "", speech.Reference{}, err
	}
	if reference, ok := s.trackReference(ctx, version, analysis); ok {
		return speechTrack, reference, nil
	}
	reference, err := s.audioReference(ctx, version, analysis)
	if errors.Is(err, errNoSpeech) {
		s.logger.Info("A version has no speech reference", "addon", version.Addon, "reason", err)
		return speechNone, speech.Reference{}, nil
	}
	if err != nil {
		return "", speech.Reference{}, err
	}
	return speechAudio, reference, nil
}

// trackReference is the speech of a text track inside the version's file:
// its cues, whose times are the file's, over the whole version. A track
// read whole serves: through the version's index, or as remuxes extracted
// it. Forced tracks, which show only some lines, come after the others.
func (s *Service) trackReference(ctx context.Context, version library.Version, analysis media.Analysis) (speech.Reference, bool) {
	streams := textSubtitles(analysis)
	forced := func(index int) bool {
		return slices.ContainsFunc(analysis.Streams, func(stream media.Stream) bool { return stream.Index == index && stream.Forced })
	}
	slices.SortStableFunc(streams, func(a, b int) int {
		switch {
		case forced(a) == forced(b):
			return 0
		case forced(a):
			return 1
		}
		return -1
	})
	var located map[int]bool
	if len(streams) > 0 {
		located = s.SubtitlesLocated(ctx, version, analysis)
	}
	for _, stream := range streams {
		var cues []subtitles.Cue
		if located[stream] {
			if track, err := s.SubtitleTrack(ctx, version, analysis, stream); err == nil {
				cues, _ = subtitles.Parse(track.Data)
			}
		} else {
			cues, _ = s.ExtractedTrack(ctx, version.ID, analysis.Duration, stream)
		}
		if len(cues) < minTrackCues {
			continue
		}
		reference := speech.Reference{Speech: make([]speech.Span, len(cues))}
		end := analysis.Duration
		for i, cue := range cues {
			reference.Speech[i] = speech.Span{Start: cue.Start, End: cue.End}
			end = max(end, cue.End)
		}
		reference.Windows = []speech.Span{{Start: 0, End: end}}
		return reference, true
	}
	return speech.Reference{}, false
}

// audioReference finds the speech in the audio of a version, in three
// stretches around a quarter, half and three quarters of it: its first
// audio track decoded by FFmpeg, its voice band measured (see
// speech.Detect). The stretches last speechStretch, but for an addon's
// file, which is read within speechBudget: there, they last as long as
// the budget reads at the file's bitrate (see stretchWithin). A file that
// would need stretches shorter than minStretch is not read, and one that
// reads more than the budget is given up.
func (s *Service) audioReference(ctx context.Context, version library.Version, analysis media.Analysis) (speech.Reference, error) {
	if !slices.ContainsFunc(analysis.Streams, func(stream media.Stream) bool { return stream.Type == "audio" }) {
		return speech.Reference{}, fmt.Errorf("%w: no audio track", errNoSpeech)
	}
	if analysis.Duration <= 0 {
		return speech.Reference{}, fmt.Errorf("%w: its length is not known", errNoSpeech)
	}
	stretch := speechStretch
	var budget *readBudget
	if !s.ownFile(version) {
		if size := cmp.Or(analysis.Size, version.Size); size > 0 {
			stretch = stretchWithin(s.speech.budget, float64(size)/analysis.Duration.Seconds())
			if stretch < minStretch {
				return speech.Reference{}, fmt.Errorf("%w: within %d MB, its stretches would last %s, under %s",
					errNoSpeech, s.speech.budget/1_000_000, stretch, minStretch)
			}
		}
		budget = &readBudget{limit: s.speech.budget}
	}
	windows := speechWindows(analysis.Duration, stretch)
	src := s.open(version)
	defer src.Release()
	input, release := s.loopback.registerWithin(src, budget)
	defer release()
	var reference speech.Reference
	for _, window := range windows {
		samples, err := s.decodeSpeech(ctx, input, window)
		if budget != nil && budget.over() {
			return speech.Reference{}, fmt.Errorf("%w: it read more than %d MB", errNoSpeech, s.speech.budget/1_000_000)
		}
		if err != nil {
			return speech.Reference{}, err
		}
		end := window.Start + time.Duration(len(samples))*time.Second/speech.SampleRate
		reference.Windows = append(reference.Windows, speech.Span{Start: window.Start, End: end})
		reference.Speech = append(reference.Speech, speech.Detect(samples, window.Start)...)
	}
	if budget != nil {
		s.logger.Debug("Read the stretches of a version's audio", "addon", version.Addon, "stretch", stretch, "read", budget.read.Load())
	}
	if speech.Total(reference.Speech) < minSpeech {
		return speech.Reference{}, fmt.Errorf("%w: its audio has too little speech", errNoSpeech)
	}
	return reference, nil
}

// stretchWithin is how long each of three stretches of an addon's file
// lasts for reading them to take at most budget bytes, at perSecond
// bytes a second, up to speechStretch, to the second.
func stretchWithin(budget int64, perSecond float64) time.Duration {
	share := float64(budget)/3/readOverhead - readAhead
	seconds := min(share/perSecond-keyframeLead.Seconds(), speechStretch.Seconds())
	return time.Duration(max(seconds, 0) * float64(time.Second)).Truncate(time.Second)
}

// speechWindows are the stretches of a version of duration whose audio is
// looked at: stretch around a quarter, half and three quarters of it,
// within it, those that overlap joined.
func speechWindows(duration, stretch time.Duration) []speech.Span {
	var windows []speech.Span
	for _, at := range []time.Duration{duration / 4, duration / 2, duration * 3 / 4} {
		window := speech.Span{Start: max(at-stretch/2, 0), End: min(at+stretch/2, duration)}
		if n := len(windows); n > 0 && window.Start <= windows[n-1].End {
			windows[n-1].End = window.End
			continue
		}
		windows = append(windows, window)
	}
	return windows
}

// ownFile reports whether a version is a file Polyfin serves itself, of a
// local folder or a recording, read without a budget.
func (s *Service) ownFile(version library.Version) bool {
	return strings.HasPrefix(version.URL, "http://"+s.loopback.listener.Addr().String()+"/")
}

// decodeSpeech decodes a window of the first audio track of input as the
// samples speech.Detect reads: fewer when the version ends sooner.
func (s *Service) decodeSpeech(ctx context.Context, input string, window speech.Span) ([]int16, error) {
	decoding, stop := context.WithCancel(ctx)
	defer stop()
	sink := &sampleSink{want: int((window.End-window.Start).Seconds()*speech.SampleRate) * 2, full: stop}
	audio := hls.Audio{Input: input, Encoder: "pcm_s16le", SampleRate: speech.SampleRate, Channels: 1}
	if err := s.segments.Convert(decoding, audio, "s16le", window.Start, sink); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	samples := make([]int16, len(sink.data)/2)
	for i := range samples {
		samples[i] = int16(uint16(sink.data[2*i]) | uint16(sink.data[2*i+1])<<8)
	}
	return samples, nil
}

// sampleSink keeps the first want bytes written to it, and calls full
// once it has them.
type sampleSink struct {
	data []byte
	want int
	full func()
}

func (k *sampleSink) Write(p []byte) (int, error) {
	if room := k.want - len(k.data); room > 0 {
		k.data = append(k.data, p[:min(room, len(p))]...)
		if len(k.data) == k.want {
			k.full()
		}
	}
	return len(p), nil
}

// storedSpeech returns what was found of a version's speech, whether it
// was looked for.
func (s *Service) storedSpeech(ctx context.Context, version accounts.ID) (storedSpeech, bool) {
	if stored, ok := s.speech.found.Get(version); ok {
		return stored, true
	}
	if _, unknown := s.speech.unknown.Get(version); unknown {
		return storedSpeech{}, false
	}
	var stored storedSpeech
	var windows, spans []byte
	err := s.db.QueryRow(ctx, "SELECT reference, windows, speech FROM version_speech WHERE version_id = $1", version).
		Scan(&stored.kind, &windows, &spans)
	if err == nil {
		stored.reference.Windows, err = unmarshalSpans(windows)
	}
	if err == nil {
		stored.reference.Speech, err = unmarshalSpans(spans)
	}
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("Reading the speech of a version failed", "error", err)
		}
		s.speech.unknown.Put(version, struct{}{})
		return storedSpeech{}, false
	}
	s.speech.found.Put(version, stored)
	return stored, true
}

func (s *Service) saveSpeech(ctx context.Context, version accounts.ID, kind string, reference speech.Reference) error {
	_, err := s.db.Exec(ctx, `INSERT INTO version_speech (version_id, reference, windows, speech) VALUES ($1, $2, $3, $4)
		ON CONFLICT (version_id) DO NOTHING`, version, kind, marshalSpans(reference.Windows), marshalSpans(reference.Speech))
	if err != nil {
		return err
	}
	s.speech.found.Put(version, storedSpeech{kind: kind, reference: reference})
	s.speech.unknown.Delete(version)
	return nil
}

// marshalSpans writes spans as [start, end] pairs of milliseconds.
func marshalSpans(spans []speech.Span) []byte {
	pairs := make([][2]int64, len(spans))
	for i, span := range spans {
		pairs[i] = [2]int64{span.Start.Milliseconds(), span.End.Milliseconds()}
	}
	data, _ := json.Marshal(pairs)
	return data
}

func unmarshalSpans(data []byte) ([]speech.Span, error) {
	var pairs [][2]int64
	if err := json.Unmarshal(data, &pairs); err != nil {
		return nil, err
	}
	spans := make([]speech.Span, len(pairs))
	for i, pair := range pairs {
		spans[i] = speech.Span{Start: time.Duration(pair[0]) * time.Millisecond, End: time.Duration(pair[1]) * time.Millisecond}
	}
	return spans, nil
}

// Speech returns when a version has speech, once found; false while it
// was not, and when the version has no speech reference.
func (s *Service) Speech(ctx context.Context, version accounts.ID) (speech.Reference, bool) {
	stored, ok := s.storedSpeech(ctx, version)
	return stored.reference, ok && stored.kind != speechNone
}

// SubtitleTimings returns how the addon subtitles chosen for a version
// were timed to it, by subtitle; none while the settings do not sync them.
func (s *Service) SubtitleTimings(ctx context.Context, version accounts.ID) map[accounts.ID]speech.Timing {
	if !s.settings().SyncAddonSubtitles {
		return nil
	}
	if timings, ok := s.speech.timings.Get(version); ok {
		return timings
	}
	rows, err := s.db.Query(ctx, "SELECT subtitle_id, ratio, offset_ms FROM subtitle_syncs WHERE version_id = $1", version)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Reading the subtitles synced to a version failed", "error", err)
		}
		return nil
	}
	timings := map[accounts.ID]speech.Timing{}
	var subtitle accounts.ID
	var ratio float64
	var offset int64
	_, err = pgx.ForEachRow(rows, []any{&subtitle, &ratio, &offset}, func() error {
		timings[subtitle] = speech.Timing{Ratio: ratio, Offset: time.Duration(offset) * time.Millisecond}
		return nil
	})
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Reading the subtitles synced to a version failed", "error", err)
		}
		return nil
	}
	s.speech.timings.Put(version, timings)
	return timings
}

// SyncSubtitle returns how an addon subtitle, of cues, is timed to a
// version: as it was before, else against the version's speech, and then
// kept. False while the settings do not sync subtitles, and while the
// version has no speech reference: a subtitle chosen before its version's
// reference is ready is synced the next time it is chosen.
func (s *Service) SyncSubtitle(ctx context.Context, version, subtitle accounts.ID, cues []subtitles.Cue) (speech.Timing, bool) {
	if !s.settings().SyncAddonSubtitles {
		return speech.Unchanged, false
	}
	if timing, ok := s.SubtitleTimings(ctx, version)[subtitle]; ok {
		return timing, true
	}
	reference, ok := s.Speech(ctx, version)
	if !ok {
		return speech.Unchanged, false
	}
	var frameRate float64
	if analysis, ok := s.Analyzed(ctx, version); ok {
		frameRate = videoFrameRate(analysis)
	}
	timing := speech.Align(cues, reference, frameRate)
	_, err := s.db.Exec(ctx, `INSERT INTO subtitle_syncs (version_id, subtitle_id, ratio, offset_ms) VALUES ($1, $2, $3, $4)
		ON CONFLICT (version_id, subtitle_id) DO NOTHING`, version, subtitle, timing.Ratio, timing.Offset.Milliseconds())
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Saving the sync of a subtitle failed", "error", err)
		}
		return timing, true
	}
	s.speech.timingsMu.Lock()
	timings, _ := s.speech.timings.Get(version)
	updated := make(map[accounts.ID]speech.Timing, len(timings)+1)
	for id, t := range timings {
		updated[id] = t
	}
	updated[subtitle] = timing
	s.speech.timings.Put(version, updated)
	s.speech.timingsMu.Unlock()
	if timing.Changes() {
		s.logger.Info("An addon subtitle was synced to its version", "speed", timing.Ratio, "shift", timing.Offset)
	}
	return timing, true
}

// videoFrameRate is the frame rate of a version's video, 0 when not known.
func videoFrameRate(analysis media.Analysis) float64 {
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture {
			return cmp.Or(stream.AverageRate, stream.FrameRate)
		}
	}
	return 0
}

// SyncedTitle is the name of an addon subtitle SyncSubtitle changed: its
// display title followed by "(synced)" in the server language.
func SyncedTitle(title, language string) string {
	if wordsFor(language) == &frenchWords {
		return title + " (recalé)"
	}
	return title + " (synced)"
}
