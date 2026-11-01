package library

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// A library of other videos (see othervideos.go) may show its titles as
// movies and shows (addons.Library.ShowAsTitles), for the Jellyfin apps
// that keep only libraries of movies and shows, and list only Movie and
// Series items. The library then has no content type, as Jellyfin's mixed
// libraries. A title whose description has two videos or more, or one its
// description gives an episode number, is a series: each video is an
// episode, of the season its description numbers, season 1 when it gives
// none, numbered as its description numbers it, or after the season's
// numbered episodes in the description's order when it gives no number
// (see numberVideos). A title without videos, or with one video without
// an episode number, the file of a movie, is a movie (see shownKind). The
// episodes play as a folder's videos do: the streams the description
// embeds for them, then those the stream resource lists for their
// identifiers.
//
// A title keeps the identifier of a series whichever it is (see
// catalogTitleKind), so that it stays the same item when its description
// gains or loses videos; its videos are episodes, of their own
// identifiers. Those are not the identifiers the same titles and videos
// have as home videos: switching the option gives users' played marks,
// resume points and favorites to the items the titles and videos become
// (see FollowTitles), and the items they were are no longer found (see
// misplaced).

// catalogTitleKind is the kind whose key identifies a title of metaType
// that a catalog of entry lists (see titleKind): a series for one shown as
// movies and shows, whether it is a series or a movie.
func catalogTitleKind(entry installed, catalog stremio.Catalog, metaType string) (Kind, bool) {
	kind, ok := titleKind(metaType)
	if ok && otherType(metaType) && entry.addon.ShowsAsTitles(catalog.Type, catalog.ID) {
		return KindSeries, true
	}
	return kind, ok
}

// catalogOffers reports whether a catalog of entry lists titles of one of
// kinds, and returns the kind of their keys (see catalogTitleKind): one
// shown as movies and shows lists movies and series.
func catalogOffers(entry installed, catalog stremio.Catalog, kinds []Kind) (Kind, bool) {
	kind, ok := catalogTitleKind(entry, catalog, catalog.Type)
	switch {
	case !ok:
		return "", false
	case slices.Contains(kinds, kind):
		return kind, true
	}
	return kind, kind == KindSeries && otherType(catalog.Type) && slices.Contains(kinds, KindMovie)
}

// libraryCollectionType is the content of a library (see collectionType):
// none, as Jellyfin's mixed libraries, for one that shows its titles as
// movies and shows.
func libraryCollectionType(l addons.Library) string {
	if l.ShowAsTitles {
		return ""
	}
	return collectionType(l.Catalog.Type)
}

// shownTitle reports whether r is a title of the other or channel type
// shown as a series or a movie.
func (r record) shownTitle() bool {
	return (r.Kind == KindSeries || r.Kind == KindMovie) && r.Meta != nil && otherType(r.Meta.Type)
}

// misplaced reports whether r is a title of the other or channel type
// recorded as its library showed it before the library switched between
// home videos and movies and shows: a folder or a video of a catalog whose
// library now shows its titles as movies and shows, or a series or a
// movie of one that no longer does. Such an item is no longer found.
func (v view) misplaced(r record) bool {
	if !r.shapedTitle() {
		return false
	}
	shows := false
	if r.Addon != nil {
		if entry, ok := v.addon(*r.Addon); ok {
			shows = entry.addon.ShowsAsTitles(r.CatalogType, r.CatalogID)
		}
	}
	return r.shownTitle() != shows
}

// shownKind is the kind a description gives a title shown as movies and
// shows: a movie when it has no video, or one its description gives no
// episode number, as a movie's file is; else a series, a single episode
// numbered so (S35E01) being a show of one episode.
func shownKind(meta stremio.Meta) Kind {
	if len(meta.Videos) == 0 || len(meta.Videos) == 1 && meta.Videos[0].EpisodeNumber() <= 0 {
		return KindMovie
	}
	return KindSeries
}

// shownMeta is the description of r with its videos numbered when r is a
// title shown as a series (see numberVideos), else meta itself: a movie's
// one video keeps no number, so that it stays a movie.
func shownMeta(r record, meta stremio.Meta) stremio.Meta {
	if !r.shownTitle() || shownKind(meta) == KindMovie {
		return meta
	}
	return numberVideos(meta)
}

// videoSeason is the season of a video of a title shown as movies and
// shows: the one its description gives; season 1 when it gives neither a
// season nor an episode number. Stremio descriptions cannot tell season 0
// from none: a video of season 0 with an episode number is a special, as
// they number them.
func videoSeason(video stremio.Video) int {
	if video.Season > 0 || video.EpisodeNumber() > 0 {
		return int(video.Season)
	}
	return 1
}

// numberVideos gives each video of the description of a title shown as
// movies and shows its season (see videoSeason) and an episode number:
// its own, or for a video without one, the next after the highest of its
// season, in the description's order. The description kept is left as it
// is.
func numberVideos(meta stremio.Meta) stremio.Meta {
	unnumbered := false
	for _, video := range meta.Videos {
		if video.EpisodeNumber() <= 0 {
			unnumbered = true
			break
		}
	}
	if !unnumbered {
		return meta
	}
	videos := make([]stremio.Video, len(meta.Videos))
	copy(videos, meta.Videos)
	highest := map[int]int{}
	for _, video := range videos {
		if n := video.EpisodeNumber(); n > 0 {
			season := videoSeason(video)
			highest[season] = max(highest[season], n)
		}
	}
	for i := range videos {
		if videos[i].EpisodeNumber() > 0 {
			continue
		}
		season := videoSeason(videos[i])
		highest[season]++
		videos[i].Season, videos[i].Episode = stremio.Number(season), stremio.Number(highest[season])
	}
	meta.Videos = videos
	return meta
}

// FollowTitles gives, for each library that switched between showing its
// titles as home videos and as movies and shows from before to after, the
// libraries of a scope before and after a change, what users did with its
// titles and their videos to the items they become: the series or movie a
// folder or a video becomes, the episode a folder's video becomes, and
// back. Played marks and favorites are given, resume points move, by an
// import's rules (see userdata.Store.Follow). A library turned off or on
// switches as well: its titles are found again as home videos while it is
// off.
func (s *Service) FollowTitles(ctx context.Context, before, after []addons.Library) error {
	shows := func(libraries []addons.Library) map[addons.LibraryKey]bool {
		result := map[addons.LibraryKey]bool{}
		for _, l := range libraries {
			if l.Enabled && l.ShowAsTitles {
				result[addons.LibraryKey{AddonID: l.AddonID, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID}] = true
			}
		}
		return result
	}
	was, is := shows(before), shows(after)
	var errs []error
	for key := range was {
		if !is[key] {
			errs = append(errs, s.followTitles(ctx, key, false))
		}
	}
	for key := range is {
		if !was[key] {
			errs = append(errs, s.followTitles(ctx, key, true))
		}
	}
	return errors.Join(errs...)
}

// followTitles gives what users did with the titles of a catalog, and
// their videos, to the items they become as the catalog's library shows
// them as movies and shows, when shown is set, or as home videos (see
// FollowTitles). The records of the items given anything are stored, so
// that the lists of users' data find them before any listing does.
func (s *Service) followTitles(ctx context.Context, key addons.LibraryKey, shown bool) error {
	addon, err := json.Marshal(key.AddonID)
	if err != nil {
		return err
	}
	fromTitle, fromVideo, kinds := titleKey(KindVideo, ""), folderVideoKey(""), []string{string(KindFolder), string(KindVideo)}
	if !shown {
		fromTitle, fromVideo, kinds = titleKey(KindSeries, ""), episodeKey(""), []string{string(KindSeries), string(KindMovie)}
	}
	titles, err := s.loadWhere(ctx, `kind = ANY($1) AND starts_with(key, $2) AND data->'addon' = $3::jsonb AND data->>'catalogType' = $4
		AND data->>'catalogId' = $5`, kinds, fromTitle, string(addon), key.CatalogType, key.CatalogID)
	if err != nil {
		return err
	}
	// A title's kind on either side is its kept description's, when one
	// is kept (see shownKind and otherKind).
	entry, _ := s.addons.Find(ctx, key.AddonID)
	described := func(r record) (stremio.Meta, bool) {
		return s.keptDescription(ctx, installed{addon: entry}, metaKey{key.AddonID, r.Meta.Type, r.Meta.ID}, inDatabase)
	}
	var moves []userdata.Move
	var records []record
	movedTitles := map[string]record{}
	// oneFile maps the identifier of a title shown as a movie of one video
	// to that video: what users did with the video, a folder's, is the
	// movie's, and back.
	oneFile := map[string]stremio.Video{}
	var ids []string
	for _, r := range titles {
		if r.Meta == nil || !otherType(r.Meta.Type) {
			continue
		}
		meta, known := described(r)
		moved := r
		switch {
		case shown && known:
			moved.Kind = shownKind(meta)
		case shown && r.Kind == KindFolder:
			moved.Kind = KindSeries
		case shown:
			moved.Kind = KindMovie
		case known:
			moved.Kind = otherKind(meta)
		case r.Kind == KindSeries:
			moved.Kind = KindFolder
		default:
			moved.Kind = KindVideo
		}
		moved.Key = titleKey(KindVideo, r.Meta.ID)
		if shown {
			moved.Key = titleKey(KindSeries, r.Meta.ID)
		}
		moved.ID = itemID(moved.Key)
		movedTitles[r.Meta.ID] = moved
		ids = append(ids, r.Meta.ID)
		to := userdata.Item{ID: moved.ID}
		if known && len(meta.Videos) == 1 && shownKind(meta) == KindMovie {
			oneFile[r.Meta.ID] = meta.Videos[0]
			if !shown {
				// The movie's data goes to its file, the folder's video.
				video := meta.Videos[0]
				to.ID = itemID(folderVideoKey(video.ID))
				records = append(records, record{ID: to.ID, Key: folderVideoKey(video.ID), Kind: KindVideo, Parent: &moved.ID,
					SeriesID: r.Meta.ID, Video: &video})
			}
		}
		moves = append(moves, userdata.Move{From: []accounts.ID{r.ID}, To: []userdata.Item{to}})
	}
	if len(ids) == 0 {
		return nil
	}
	videos, err := s.loadWhere(ctx, `starts_with(key, $1) AND data->>'seriesId' = ANY($2)
		AND EXISTS (SELECT 1 FROM user_data u WHERE u.item_id = items.id)`, fromVideo, ids)
	if err != nil {
		return err
	}
	holders := map[string]bool{}
	for _, r := range videos {
		if r.Video == nil {
			continue
		}
		video := *r.Video
		var moved record
		var to userdata.Item
		if _, movie := oneFile[r.SeriesID]; shown && movie {
			// The one video of a title shown as a movie is the movie.
			movedTitle := movedTitles[r.SeriesID]
			holders[r.SeriesID] = true
			moves = append(moves, userdata.Move{From: []accounts.ID{r.ID}, To: []userdata.Item{{ID: movedTitle.ID}}})
			continue
		}
		if shown {
			season := videoSeason(video)
			video.Season = stremio.Number(season)
			seasonID := itemID(seasonKey(r.SeriesID, season))
			moved = record{Key: episodeKey(video.ID), Kind: KindEpisode, Parent: &seasonID, SeriesID: r.SeriesID, Season: season, Video: &video}
			to = userdata.Item{Series: itemID(titleKey(KindSeries, r.SeriesID)), Season: seasonID}
		} else {
			folder := itemID(titleKey(KindVideo, r.SeriesID))
			moved = record{Key: folderVideoKey(video.ID), Kind: KindVideo, Parent: &folder, SeriesID: r.SeriesID, Video: &video}
		}
		moved.ID = itemID(moved.Key)
		to.ID = moved.ID
		records = append(records, moved)
		holders[r.SeriesID] = true
		moves = append(moves, userdata.Move{From: []accounts.ID{r.ID}, To: []userdata.Item{to}})
	}
	// The titles whose users' data or videos move are recorded.
	given, err := s.withUserData(ctx, titles)
	if err != nil {
		return err
	}
	for id, moved := range movedTitles {
		if holders[id] || given[id] {
			records = append(records, moved)
		}
	}
	if err := s.save(ctx, records); err != nil {
		return err
	}
	settings := s.settings()
	return userdata.New(s.db).Follow(ctx, moves, userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent})
}

// withUserData tells, by Stremio identifier, which titles among records
// some user has data of.
func (s *Service) withUserData(ctx context.Context, records []record) (map[string]bool, error) {
	ids := make([]accounts.ID, 0, len(records))
	byID := make(map[accounts.ID]string, len(records))
	for _, r := range records {
		if r.Meta != nil {
			ids = append(ids, r.ID)
			byID[r.ID] = r.Meta.ID
		}
	}
	rows, err := s.db.Query(ctx, "SELECT DISTINCT item_id FROM user_data WHERE item_id = ANY($1)", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var id accounts.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[byID[id]] = true
	}
	return result, rows.Err()
}
