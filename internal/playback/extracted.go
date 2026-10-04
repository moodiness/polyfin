package playback

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// maxExtractedCues bounds the cues kept of one track: movies have a few
// thousand, karaoke effects in ASS a few tens of thousands.
const maxExtractedCues = 50_000

// textSubtitleCodecs are the subtitle codecs FFmpeg converts to WebVTT.
var textSubtitleCodecs = []string{"subrip", "ass", "ssa", "webvtt", "mov_text", "text"}

// textSubtitles are the FFmpeg indexes of a version's text subtitle
// streams.
func textSubtitles(analysis media.Analysis) []int {
	var streams []int
	for _, stream := range analysis.Streams {
		if stream.Type == "subtitle" && slices.Contains(textSubtitleCodecs, stream.Codec) {
			streams = append(streams, stream.Index)
		}
	}
	return streams
}

// ExtractableSubtitle reports whether remuxes extract the subtitle stream
// of a version with FFmpeg index index: a text one.
func ExtractableSubtitle(analysis media.Analysis, index int) bool {
	return slices.Contains(textSubtitles(analysis), index)
}

// imageSubtitleCodecs are the subtitle codecs FFmpeg draws onto video.
var imageSubtitleCodecs = []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle"}

// BurnableSubtitle reports whether the subtitle stream of a version with
// FFmpeg index index can be burned into converted video: an image one.
func BurnableSubtitle(analysis media.Analysis, index int) bool {
	for _, stream := range analysis.Streams {
		if stream.Index == index {
			return stream.Type == "subtitle" && slices.Contains(imageSubtitleCodecs, stream.Codec)
		}
	}
	return false
}

// LanguageTag is the RFC 5646 tag of a track's language, as HLS names
// languages: the ISO 639-1 code when there is one ("fra" gives "fr"), else
// the code as given, and nothing for undetermined languages.
func LanguageTag(language string) string {
	code := streamLanguage(strings.TrimSpace(language))
	if code == "" || isSpecialLanguage(code) {
		return ""
	}
	if i, ok := languageLookup().jellyfin[strings.ToLower(code)]; ok && languages[i].short != "" {
		base, region, _ := strings.Cut(languages[i].short, "-")
		if region != "" {
			return base + "-" + strings.ToUpper(region)
		}
		return base
	}
	return code
}

// extracted keeps what remuxes extract of a version's text subtitles, in
// memory while it is played and in PostgreSQL in between, so that later
// playbacks find whole tracks.
type extracted struct {
	mu      sync.Mutex
	covered []span
	tracks  map[int][]subtitles.Cue
	seen    map[int]map[cueKey]struct{}
	dirty   bool
}

// span is a stretch of the version's time.
type span struct{ from, to time.Duration }

// cueKey tells cues apart: remuxes started at different places extract the
// same cues again.
type cueKey struct {
	start, end time.Duration
	text       string
}

func newExtracted() *extracted {
	return &extracted{tracks: map[int][]subtitles.Cue{}, seen: map[int]map[cueKey]struct{}{}}
}

func (x *extracted) Add(stream int, cue subtitles.Cue) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.add(stream, cue)
}

// add keeps a cue in start order, once. The caller holds x.mu.
func (x *extracted) add(stream int, cue subtitles.Cue) {
	seen := x.seen[stream]
	if seen == nil {
		seen = map[cueKey]struct{}{}
		x.seen[stream] = seen
	}
	key := cueKey{cue.Start, cue.End, strings.Join(cue.Lines, "\n")}
	if _, dup := seen[key]; dup || len(seen) >= maxExtractedCues {
		return
	}
	seen[key] = struct{}{}
	track := x.tracks[stream]
	at, _ := slices.BinarySearchFunc(track, cue.Start, func(c subtitles.Cue, start time.Duration) int { return cmp.Compare(c.Start, start+1) })
	x.tracks[stream] = slices.Insert(track, at, cue)
	x.dirty = true
}

func (x *extracted) Cover(from, to time.Duration) {
	if to <= from {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.cover(span{from, to})
}

// cover merges a span into the covered ones. The caller holds x.mu.
func (x *extracted) cover(s span) {
	merged := make([]span, 0, len(x.covered)+1)
	for _, c := range x.covered {
		if c.to < s.from || c.from > s.to {
			merged = append(merged, c)
			continue
		}
		s = span{min(s.from, c.from), max(s.to, c.to)}
	}
	merged = append(merged, s)
	slices.SortFunc(merged, func(a, b span) int { return cmp.Compare(a.from, b.from) })
	if !slices.Equal(merged, x.covered) {
		x.covered, x.dirty = merged, true
	}
}

func (x *extracted) Covers(from, to time.Duration) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, c := range x.covered {
		if c.from <= from && to <= c.to {
			return true
		}
	}
	return false
}

// cues returns the cues of a stream shown during [from, to), or all of
// them when to is zero.
func (x *extracted) cues(stream int, from, to time.Duration) []subtitles.Cue {
	x.mu.Lock()
	defer x.mu.Unlock()
	var result []subtitles.Cue
	for _, cue := range x.tracks[stream] {
		if to > 0 && cue.Start >= to {
			break
		}
		if cue.End > from {
			result = append(result, cue)
		}
	}
	return result
}

// stored is how extracted subtitles are saved: times in milliseconds, cue
// text with its lines joined.
type stored struct {
	Covered [][2]int64             `json:"covered"`
	Tracks  map[string][]storedCue `json:"tracks"`
}

type storedCue struct {
	Start int64  `json:"s"`
	End   int64  `json:"e"`
	Text  string `json:"t"`
}

func (x *extracted) marshal() ([]byte, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.dirty {
		return nil, false
	}
	out := stored{Covered: make([][2]int64, len(x.covered)), Tracks: make(map[string][]storedCue, len(x.tracks))}
	for i, c := range x.covered {
		out.Covered[i] = [2]int64{c.from.Milliseconds(), c.to.Milliseconds()}
	}
	for stream, cues := range x.tracks {
		saved := make([]storedCue, len(cues))
		for i, cue := range cues {
			saved[i] = storedCue{cue.Start.Milliseconds(), cue.End.Milliseconds(), strings.Join(cue.Lines, "\n")}
		}
		out.Tracks[strconv.Itoa(stream)] = saved
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	x.dirty = false
	return data, true
}

func unmarshalExtracted(data []byte) (*extracted, error) {
	var in stored
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, err
	}
	x := newExtracted()
	for _, c := range in.Covered {
		x.cover(span{time.Duration(c[0]) * time.Millisecond, time.Duration(c[1]) * time.Millisecond})
	}
	for key, cues := range in.Tracks {
		stream, err := strconv.Atoi(key)
		if err != nil {
			return nil, errors.New("a stored subtitle track has no stream index")
		}
		for _, cue := range cues {
			x.add(stream, subtitles.Cue{Start: time.Duration(cue.Start) * time.Millisecond, End: time.Duration(cue.End) * time.Millisecond,
				Lines: strings.Split(cue.Text, "\n")})
		}
	}
	x.dirty = false
	return x, nil
}

// extractedOf returns what remuxes extracted of a version's subtitles,
// reading it from the database on first use.
func (s *Service) extractedOf(ctx context.Context, version accounts.ID) *extracted {
	s.extractedMu.Lock()
	defer s.extractedMu.Unlock()
	if x, ok := s.extractions.Get(version); ok {
		return x
	}
	x := newExtracted()
	var data []byte
	// Not canceled with the request: what is read here is kept for later
	// ones, and an empty set in its place would hide what is stored, then
	// overwrite it.
	err := s.db.QueryRow(context.WithoutCancel(ctx), "SELECT extracted FROM media_subtitles WHERE version_id = $1", version).Scan(&data)
	switch {
	case err == nil:
		if loaded, err := unmarshalExtracted(data); err == nil {
			x = loaded
		} else {
			s.logger.Warn("Stored subtitles are unreadable", "error", err)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		s.logger.Warn("Reading extracted subtitles failed", "error", err)
	}
	s.extractions.Put(version, x)
	return x
}

// saveExtracted saves what was extracted of a version's subtitles since it
// was last saved.
func (s *Service) saveExtracted(ctx context.Context, version accounts.ID, x *extracted) {
	data, changed := x.marshal()
	if !changed {
		return
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO media_subtitles (version_id, extracted) VALUES ($1, $2)
		ON CONFLICT (version_id) DO UPDATE SET extracted = excluded.extracted, updated_at = now()`, version, data); err != nil {
		s.logger.Warn("Saving extracted subtitles failed", "error", err)
	}
}

// ExtractedTrack returns an embedded text subtitle track of a version,
// stream being its FFmpeg index, once remuxes have extracted all of it.
func (s *Service) ExtractedTrack(ctx context.Context, versionID accounts.ID, duration time.Duration, stream int) ([]subtitles.Cue, bool) {
	x := s.extractedOf(ctx, versionID)
	if duration <= 0 || !x.Covers(0, duration) {
		return nil, false
	}
	return x.cues(stream, 0, 0), true
}

// SubtitlesExtracted reports whether remuxes have extracted all of a
// version's text subtitles.
func (s *Service) SubtitlesExtracted(ctx context.Context, versionID accounts.ID, duration time.Duration) bool {
	return duration > 0 && s.extractedOf(ctx, versionID).Covers(0, duration)
}
