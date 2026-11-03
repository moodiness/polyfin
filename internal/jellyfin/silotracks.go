package jellyfin

import (
	"context"
)

// ContainerTracks returns the container's own numbers of the tracks of the
// version a play session PlaybackInfo started plays, by FFmpeg index (see
// playback.Service.ContainerTracks): Silo's apps name the subtitle tracks
// of a stream by them. None for a session not signed by this server, or a
// version not listed since Polyfin started.
func (h *Handler) ContainerTracks(ctx context.Context, playSession string) map[int]uint64 {
	grant, err := h.Playback.Signer().Verify(playSession)
	if err != nil {
		return nil
	}
	version, ok := h.Library.KnownVersion(grant.Version)
	if !ok {
		return nil
	}
	return h.Playback.ContainerTracks(ctx, version)
}
