package jellyfin

import (
	"context"
	"errors"
	"maps"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/loudness"
)

// Songs and albums carry their gain as Jellyfin's NormalizationGain, in
// listings as in details, while Even the volume between songs is on: apps
// apply it, the song's or the album's as their playback settings choose,
// and the stream stays as it is. The gains of local music folders come
// with their items (see localfiles); those of music addons' songs are
// measured after their first play (see loudness.Service).

// gains are the gains known of the songs and albums among items, while
// Even the volume between songs is on: those of local music folders, as
// their items tell, and those of music addons (see
// loudness.Service.Gains). Listings never measure.
func (h *Handler) gains(ctx context.Context, items []library.Item) map[accounts.ID]float64 {
	if !h.Accounts.Settings().EvenVolume {
		return nil
	}
	gains := map[accounts.ID]float64{}
	var songs []accounts.ID
	var albums []loudness.Album
	for _, item := range items {
		switch {
		case item.Kind != library.KindTrack && item.Kind != library.KindAlbum:
		case item.NormalizationGain != nil:
			gains[item.ID] = *item.NormalizationGain
		case item.LocalFolder:
			// Read and measured with its folder.
		case item.Kind == library.KindTrack:
			songs = append(songs, item.ID)
		case item.ChildCount != nil:
			albums = append(albums, loudness.Album{ID: item.ID, Tracks: *item.ChildCount})
		}
	}
	maps.Copy(gains, h.Loudness.Gains(ctx, songs, albums))
	return gains
}

// errNoStream reports a song whose addon lists no stream to measure.
var errNoStream = errors.New("the song has no stream")

// measureLoudness has the loudness of a song of a music addon whose play
// started measured, in the background, through the same relay and link
// renewal as its playback (see loudness.Service.Played). Songs of local
// music folders are measured with their folder, and audiobooks never.
func (h *Handler) measureLoudness(user accounts.User, item library.Item) {
	if item.Kind != library.KindTrack || item.LocalFolder || item.NormalizationGain != nil {
		return
	}
	track := loudness.Track{ID: item.ID, Album: item.AlbumID, Length: item.Runtime}
	h.Loudness.Played(track, func(ctx context.Context, read func(context.Context, string, []string) error) error {
		versions, err := h.Library.Versions(ctx, user, item.ID)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			return errNoStream
		}
		return h.Playback.ReadAudio(ctx, versions[0], read)
	})
}
