package localfiles

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// What a folder holds may change (see Update). Between movies, shows and
// home videos, what users did with a video follows it: the items its file
// was the titles of are kept (see rememberTitles) until a scan matches the
// file as the new kind, when their watch state, resume point and favorite
// go to its new titles, as an import adds them (see follow). To or from
// music nothing follows: songs and videos are other files.

// followsKind reports whether what users did with the titles of a
// folder's files follows them when the folder holds to instead of from.
func followsKind(from, to string) bool {
	return from != KindMusic && to != KindMusic
}

// titleItems are the items a matched file of a folder of kind is the
// title of, stremioID its title's identifier: a movie's, a home video's,
// or each episode it holds, from episode to last, with its series and
// season. A show's file whose name tells no episode is none.
func titleItems(kind, stremioID string, season, episode, last *int) []userdata.Item {
	switch kind {
	case KindMovies:
		return []userdata.Item{{ID: library.TitleItemID(library.KindMovie, stremioID)}}
	case KindHomeVideos:
		return []userdata.Item{{ID: library.TitleItemID(library.KindVideo, stremioID)}}
	case KindShows:
		if season == nil || episode == nil {
			return nil
		}
		series, seasonItem := library.TitleItemID(library.KindSeries, stremioID), library.SeasonItemID(stremioID, *season)
		end := *episode
		if last != nil {
			end = max(end, *last)
		}
		var items []userdata.Item
		for number := *episode; number <= end; number++ {
			items = append(items, userdata.Item{ID: library.EpisodeItemID(episodeVideoID(stremioID, *season, number)), Series: series,
				Season: seasonItem})
		}
		return items
	}
	return nil
}

// rememberTitles keeps, in tx, the items the matched files of a folder
// holding from are the titles of, as it comes to hold to: what users did
// with them follows the files (see follow). The items kept by an earlier
// change not followed yet stay, as users' data is theirs. To or from
// music, nothing is kept, and nothing kept before.
func rememberTitles(ctx context.Context, tx pgx.Tx, id accounts.ID, from, to string) error {
	if !followsKind(from, to) {
		_, err := tx.Exec(ctx, "DELETE FROM local_former_titles WHERE addon_id = $1", id)
		return err
	}
	rows, err := tx.Query(ctx, "SELECT path, stremio_id, season, episode, last_episode FROM local_files WHERE addon_id = $1 AND stremio_id IS NOT NULL",
		id)
	if err != nil {
		return err
	}
	var paths []string
	var items []accounts.ID
	var rel, stremioID string
	var season, episode, last *int
	if _, err := pgx.ForEachRow(rows, []any{&rel, &stremioID, &season, &episode, &last}, func() error {
		for _, item := range titleItems(from, stremioID, season, episode, last) {
			paths, items = append(paths, rel), append(items, item.ID)
		}
		return nil
	}); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO local_former_titles (addon_id, path, item_id) SELECT $1, * FROM unnest($2::text[], $3::uuid[])
		ON CONFLICT DO NOTHING`, id, paths, items)
	return err
}

// follow gives what users did with the former titles of a folder's files
// (see rememberTitles) to the titles its scan matched them to, by an
// import's rules (see userdata.Data.Follow). A file not matched keeps its
// former titles for a later scan; a file gone forgets them, but those in
// the folders skipped, which the scan could not read.
func (s *Service) follow(ctx context.Context, id accounts.ID, skipped []string) error {
	rows, err := s.db.Query(ctx, `SELECT o.path, o.item_id, l.kind, f.path IS NOT NULL, f.stremio_id, f.season, f.episode, f.last_episode
		FROM local_former_titles o JOIN local_folders l ON l.addon_id = o.addon_id
		LEFT JOIN local_files f ON f.addon_id = o.addon_id AND f.path = o.path
		WHERE o.addon_id = $1 ORDER BY o.path`, id)
	if err != nil {
		return err
	}
	var moves []userdata.Move
	var done []string
	var rel, kind, moved string
	var item accounts.ID
	var found bool
	var stremioID *string
	var season, episode, last *int
	if _, err := pgx.ForEachRow(rows, []any{&rel, &item, &kind, &found, &stremioID, &season, &episode, &last}, func() error {
		switch {
		case !found && !under(rel, skipped):
			done = append(done, rel)
		case found && stremioID != nil:
			if rel != moved {
				moved, done = rel, append(done, rel)
				moves = append(moves, userdata.Move{To: titleItems(kind, *stremioID, season, episode, last)})
			}
			moves[len(moves)-1].From = append(moves[len(moves)-1].From, item)
		}
		return nil
	}); err != nil {
		return err
	}
	if len(done) == 0 {
		return nil
	}
	settings := s.settings()
	if err := userdata.New(s.db).Follow(ctx, moves, userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "DELETE FROM local_former_titles WHERE addon_id = $1 AND path = ANY($2)", id, done)
	return err
}
