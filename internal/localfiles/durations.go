package localfiles

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The lengths of the videos of movies and shows folders are what the
// library tells of a title or an episode whose description gives none, nor
// TMDB (see library.Local): a scan probes each video it has no length of
// with ffprobe, once, and again only once the file changes (see
// storeDurations). A disc's title has the length its disc's tables tell
// instead, read as the disc is.

// storeDurations probes, after a scan of a movies or shows folder stored
// its files, those of files, the scan's listing, that have neither a
// duration nor a length, discs and disc images left out: their titles have
// the length their tables tell, else none. A video that cannot be probed
// is stored with 0, not to be probed again until it changes. Those of a
// share are read through the loopback server music folders read tags
// through (see probeServer), started only when a video is to be probed.
func (s *Service) storeDurations(ctx context.Context, id accounts.ID, list listing, files map[string]found) error {
	rows, err := s.db.Query(ctx, "SELECT path FROM local_files WHERE addon_id = $1 AND duration IS NULL AND length IS NULL", id)
	if err != nil {
		return err
	}
	stored, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	var wanted []string
	for _, rel := range stored {
		// A file of a folder the walk skipped is probed once it is listed
		// again.
		if _, ok := files[rel]; ok && !discFile(rel) {
			wanted = append(wanted, rel)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	target := probeTarget(func(rel string) string { return filepath.Join(list.root, filepath.FromSlash(rel)) })
	if list.root == "" {
		var stop func()
		if target, stop, err = probeServer(list); err != nil {
			return err
		}
		defer stop()
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for _, rel := range wanted {
		group.Go(func() error {
			seconds, err := s.probeDuration(groupCtx, target(rel))
			if err != nil {
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				s.logger.Debug("A video cannot be probed", "path", rel, "error", err)
			}
			_, err = s.db.Exec(groupCtx, "UPDATE local_files SET duration = $3 WHERE addon_id = $1 AND path = $2", id, rel, seconds)
			return err
		})
	}
	return group.Wait()
}

// probeDuration reads how long a video plays, in seconds, with ffprobe,
// its container's duration alone: 0 when it tells none.
func (s *Service) probeDuration(ctx context.Context, target string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, s.ffprobe, "-v", "error", "-print_format", "json", "-show_entries", "format=duration", target).Output()
	if err != nil {
		return 0, err
	}
	probe, err := readVideoProbe(out)
	return probe.duration, err
}

// lengthOf is how long a file, of alias f, plays, in seconds: its probed
// duration, else its disc title's length; NULL when neither is known.
const lengthOf = "coalesce(nullif(f.duration, 0), f.length)"

// TitleDurations returns how long the movies of the Stremio identifiers ids
// play, as the files of movies folders that hold them tell: a file's
// probed duration, else a disc title's length; the longest of a title's
// files (its extras are none of them). Titles of none are left out. ids
// may be matched titles' stremio_id or plain videos' plain_id (see
// plainvideos.go). Every folder counts, whoever it is shared with: how
// long a movie plays is the same for every user.
func (s *Service) TitleDurations(ctx context.Context, ids []string) (map[string]time.Duration, error) {
	rows, err := s.db.Query(ctx, `SELECT id, max(seconds) FROM (
			SELECT f.stremio_id AS id, `+lengthOf+` AS seconds FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
			WHERE l.kind = $1 AND f.stremio_id = ANY($2)
			UNION ALL SELECT f.plain_id, `+lengthOf+` FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
			WHERE l.kind = $1 AND f.plain_id = ANY($2) AND `+plainListed+`) t
		WHERE seconds > 0 GROUP BY id`, KindMovies, ids)
	if err != nil {
		return nil, err
	}
	durations := map[string]time.Duration{}
	var id string
	var seconds float64
	_, err = pgx.ForEachRow(rows, []any{&id, &seconds}, func() error {
		durations[id] = secondsDuration(seconds)
		return nil
	})
	return durations, err
}

// EpisodeDurations returns how long the episodes of the show of the Stremio
// identifier id play, by season and episode as its files are placed among
// the catalog's episodes (see numbering.go), from the files of shows
// folders holding one episode each: a file's probed duration, else a disc
// title's length, the longest of an episode's files. Episodes of none are
// left out. id may be a matched show's stremio_id or a plain video's
// plain_id. Every folder counts, as for TitleDurations.
func (s *Service) EpisodeDurations(ctx context.Context, id string) (map[[2]int]time.Duration, error) {
	rows, err := s.db.Query(ctx, `SELECT f.season, f.episode, max(`+lengthOf+`) FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
		WHERE l.kind = $1 AND (f.stremio_id = $2 OR f.plain_id = $2 AND `+plainListed+`)
			AND f.season IS NOT NULL AND f.episode = f.last_episode AND `+lengthOf+` > 0
		GROUP BY f.season, f.episode`, KindShows, id)
	if err != nil {
		return nil, err
	}
	durations := map[[2]int]time.Duration{}
	var season, episode int
	var seconds float64
	_, err = pgx.ForEachRow(rows, []any{&season, &episode, &seconds}, func() error {
		durations[[2]int{season, episode}] = secondsDuration(seconds)
		return nil
	})
	return durations, err
}

// secondsDuration is a length of seconds as a duration.
func secondsDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
