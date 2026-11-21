package playback

import (
	"maps"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The sweep deletes the subtitle tracks no playback used for
// TrackKeepDays, but keeps those used or read in that time, those of a
// version analysed in that time, and those a playback used just before,
// however old they were.
func TestTheSweepKeepsRecentSubtitleTracks(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileOpener{}, "ffprobe", nil)
	ctx := t.Context()
	const old, recent = TrackKeepDays + 1, TrackKeepDays - 1
	tracks := []struct {
		version  byte
		days     int
		analysed int
		kept     bool
	}{
		{version: 1, days: old},
		{version: 2, days: old, analysed: recent, kept: true},
		{version: 3, days: recent, kept: true},
		{version: 4, days: old, analysed: old},
		// Used by a playback below.
		{version: 5, days: old, kept: true},
	}
	want := map[accounts.ID]bool{}
	for _, track := range tracks {
		version := accounts.ID{track.version}
		if _, err := s.db.Exec(ctx, `INSERT INTO media_subtitle_tracks (version_id, stream, format, data, read_at)
			VALUES ($1, 2, 'srt', '1', now() - make_interval(days => $2))`, version, track.days); err != nil {
			t.Fatal(err)
		}
		if track.analysed > 0 {
			if _, err := s.db.Exec(ctx, `INSERT INTO media_analyses (version_id, analysis, analyzed_at)
				VALUES ($1, '{}', now() - make_interval(days => $2))`, version, track.analysed); err != nil {
				t.Fatal(err)
			}
		}
		if track.kept {
			want[version] = true
		}
	}
	if track, ok := s.keptTrack(ctx, trackKey{accounts.ID{5}, 2}); !ok || string(track.Data) != "1" {
		t.Fatalf("the track used: %+v %v", track, ok)
	}
	deleted, err := s.SweepSubtitleTracks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("%d tracks deleted, want 2", deleted)
	}
	rows, err := s.db.Query(ctx, "SELECT version_id FROM media_subtitle_tracks")
	if err != nil {
		t.Fatal(err)
	}
	got := map[accounts.ID]bool{}
	for rows.Next() {
		var version accounts.ID
		if err := rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		got[version] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Errorf("tracks kept: %v, want %v", got, want)
	}
}
