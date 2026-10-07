package jellyfin

import (
	"cmp"
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// A user's quality group is the tallest video they are offered (see
// accounts.User.QualityGroup), a setting Jellyfin does not have. Taller
// versions are left out of what they are offered while one fits; when none
// does, all are, and play converted down to the group. Video taller than
// the group is never sent as it is, nor downloaded.

// videoHeight is the height of the tallest video an analysis found, 0 when
// it found none.
func videoHeight(analysis media.Analysis) int {
	height := 0
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture {
			height = max(height, stream.Height)
		}
	}
	return height
}

// versionHeight is the height of a version's video: its analysis's when
// Polyfin has one, else RemuxDB's description's, else the one the addon's
// labels give, 0 when unknown. Nothing is read from the source to find it.
func (h *Handler) versionHeight(ctx context.Context, version library.Version) int {
	if analysis, ok := h.Playback.Analyzed(ctx, version.ID); ok {
		if height := videoHeight(analysis); height > 0 {
			return height
		}
	}
	if described, ok := h.RemuxDB.Described(version); ok {
		if height := videoHeight(described); height > 0 {
			return height
		}
	}
	return version.Height
}

// inGroup leaves out of versions those taller than the user's quality
// group, keeping the order of the others, while at least one fits or has
// an unknown height. When none does, all are kept rather than offering
// nothing, closest to the group first, as they play converted down to it:
// converting a taller one reads and decodes more for the same picture.
// Versions of the same height keep their order, and unknown heights,
// which never occur here as they count as fitting, would come last.
func (h *Handler) inGroup(ctx context.Context, user accounts.User, versions []library.Version) []library.Version {
	if user.QualityGroup == 0 {
		return versions
	}
	fitting := make([]library.Version, 0, len(versions))
	heights := make(map[accounts.ID]int, len(versions))
	for _, version := range versions {
		height := h.versionHeight(ctx, version)
		heights[version.ID] = height
		if user.FitsGroup(height) {
			fitting = append(fitting, version)
		}
	}
	if len(fitting) > 0 {
		return fitting
	}
	closest := slices.Clone(versions)
	slices.SortStableFunc(closest, func(a, b library.Version) int {
		x, y := heights[a.ID], heights[b.ID]
		if x == 0 || y == 0 {
			return cmp.Compare(y, x)
		}
		return cmp.Compare(x, y)
	})
	return closest
}

// limitHeight keeps in request the user's quality group, the tallest
// video a PlaybackInfo sends as it is, and the height video converted for
// them is scaled down to: the lower of the group and the settings' cap.
func (h *Handler) limitHeight(request *playbackInfoRequest, user accounts.User) {
	request.group = user.QualityGroup
	request.conversionHeight = h.Accounts.ConversionHeight(user)
}

// aboveGroup reports whether a decision would send video taller than the
// user's quality group: as it is, or copied. Above the group, only video
// converted down to it plays.
func aboveGroup(request playbackInfoRequest, analysis media.Analysis, decision playback.Decision) bool {
	return request.group > 0 && videoHeight(analysis) > request.group && (!decision.HLS || decision.Video == nil)
}

// describeConverted describes the video of a source taller than the
// user's quality group as it is sent: converted down to conversion's size.
func describeConverted(request playbackInfoRequest, analysis media.Analysis, streams []playback.MediaStream, conversion *playback.VideoConversion) {
	if request.group == 0 || videoHeight(analysis) <= request.group || conversion == nil {
		return
	}
	for i := range streams {
		if streams[i].Type == "Video" {
			streams[i].Width, streams[i].Height = new(conversion.Width), new(conversion.Height)
		}
	}
}
