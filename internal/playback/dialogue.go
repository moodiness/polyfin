package playback

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// KeptDialogue returns the cues of the first text track inside a version's
// file that is already at hand, read whole before through the version's
// index or extracted while repackaging it, with at least minTrackCues
// cues: the dialogue, as for a speech reference. Forced tracks, which show
// only some lines, are left out. Nothing is read from the version for this.
func (s *Service) KeptDialogue(ctx context.Context, version accounts.ID, analysis media.Analysis) ([]subtitles.Cue, bool) {
	for _, stream := range textSubtitles(analysis) {
		if slices.ContainsFunc(analysis.Streams, func(st media.Stream) bool { return st.Index == stream && st.Forced }) {
			continue
		}
		var cues []subtitles.Cue
		key := trackKey{version, stream}
		track, kept := s.tracks.Get(key)
		if !kept {
			track, kept = s.keptTrack(ctx, key)
		}
		if kept {
			cues, _ = subtitles.Parse(track.Data)
		} else {
			cues, _ = s.ExtractedTrack(ctx, version, analysis.Duration, stream)
		}
		if len(cues) >= minTrackCues {
			return cues, true
		}
	}
	return nil, false
}
