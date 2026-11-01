package library

import (
	"cmp"
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A series a local folder holds lists every episode its files hold, as
// Jellyfin makes an episode of every video file of a show's folder,
// whatever its metadata provider knows
// (Emby.Server.Implementations/Library/Resolvers/TV/EpisodeResolver.cs,
// lines 50-88 at v12.4). Its description, a metadata addon's, gains the
// seasons and episodes the files hold that it does not list, named after
// their files: an addon numbering the series otherwise than the files, or
// giving no episode list at all, leaves no file without its episode.

// withFileEpisodes is meta, the description of the series r that
// describer gave, with the episodes the local folders of v hold for it
// that it does not list, by season and number. A metadata addon's
// description also tells the folders which of their episodes it does not
// list, and which it lists, which their pages show; a local folder's own
// lists them all already, of its files. A description from one of the
// server's metadata addons is the catalog the files named by an absolute
// number are the episodes of (see AbsoluteEpisodes): a user's own addon
// numbering the series otherwise moves no file, which every user plays.
func (s *Service) withFileEpisodes(ctx context.Context, v view, r record, describer installed, meta stremio.Meta) stremio.Meta {
	if s.local == nil || r.Meta == nil || r.Meta.Type != string(KindSeries) || r.Kind != KindSeries {
		return meta
	}
	var folders []accounts.ID
	for _, entry := range v.addons {
		if entry.addon.Local() {
			folders = append(folders, entry.addon.ID)
		}
	}
	if len(folders) == 0 {
		return meta
	}
	var catalog []stremio.Video
	if describer.shared && !own(describer.addon) {
		catalog = meta.Videos
	}
	held, err := s.local.Episodes(ctx, folders, r.Meta.ID, catalog)
	if err != nil {
		s.logger.Debug("The episodes local folders hold could not be read", "title", r.Meta.ID, "error", err)
		return meta
	}
	if len(held) == 0 {
		return meta
	}
	listed := make(map[[2]int]bool, len(meta.Videos))
	for _, video := range meta.Videos {
		listed[[2]int{int(video.Season), video.EpisodeNumber()}] = true
	}
	var added []stremio.Video
	var unlisted [][2]int
	for _, video := range held {
		number := [2]int{int(video.Season), video.EpisodeNumber()}
		if listed[number] {
			continue
		}
		listed[number] = true
		added = append(added, video)
		unlisted = append(unlisted, number)
	}
	if !describer.addon.Local() {
		var numbers [][2]int
		if len(unlisted) > 0 {
			numbers = listedNumbers(meta.Videos)
		}
		if err := s.local.NoteUnlisted(ctx, r.Meta.ID, unlisted, numbers); err != nil {
			s.logger.Debug("The episodes a description does not list could not be recorded", "title", r.Meta.ID, "error", err)
		}
	}
	if len(added) == 0 {
		return meta
	}
	// The description may be kept and shared: it is copied, never changed
	// in place.
	meta.Videos = append(slices.Clone(meta.Videos), added...)
	return meta
}

// listedNumbers are the seasons and numbers of the episodes videos list,
// in order, each once.
func listedNumbers(videos []stremio.Video) [][2]int {
	numbers := make([][2]int, 0, len(videos))
	for _, video := range videos {
		numbers = append(numbers, [2]int{int(video.Season), video.EpisodeNumber()})
	}
	slices.SortFunc(numbers, func(a, b [2]int) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) })
	return slices.Compact(numbers)
}
