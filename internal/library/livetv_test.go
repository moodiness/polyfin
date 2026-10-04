package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A guide from a user's own addon, which may only reach public addresses,
// can describe a programme of a channel the server's addon lists: its
// artwork stays confined.
func TestProgrammeArtworkIsConfinedByItsGuide(t *testing.T) {
	e := newEnv(t)
	addon := accounts.ID{1}
	channel := record{ID: itemID(channelKey("tv:one")), Key: channelKey("tv:one"), Kind: KindChannel, Addon: &addon,
		Meta: &stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"}}
	video := stremio.Video{ID: "p", Thumbnail: "http://10.0.0.1/thumb.jpg", StartTime: "2026-01-01T10:00:00Z", EndTime: "2026-01-01T11:00:00Z"}
	for _, tc := range []struct {
		name            string
		guide, channels bool
		save            bool
		want            bool
	}{
		{"confined guide, open channel", true, false, true, true},
		{"open guide, confined channel", false, true, true, true},
		{"both open", false, false, true, false},
		{"channel unknown", false, false, false, true},
	} {
		key := programKey("tv:one", tc.name)
		program := record{ID: itemID(key), Key: key, Kind: KindProgram, Channel: "tv:one", Video: &video, Confined: tc.guide}
		if !tc.save {
			program.Channel = "tv:unknown"
		}
		channel.Confined = tc.channels
		if err := e.service.save(t.Context(), []record{channel, program}); err != nil {
			t.Fatal(err)
		}
		_, confined, err := e.service.Artwork(t.Context(), program.ID, "Primary")
		if err != nil || confined != tc.want {
			t.Errorf("%s: confined %v (%v), want %v", tc.name, confined, err, tc.want)
		}
	}
}
