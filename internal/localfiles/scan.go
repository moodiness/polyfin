package localfiles

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// ScanCheck is how often the folders due for a scan are looked for (see
// ScanDue).
const ScanCheck = 10 * time.Minute

// The failures of a folder's scan, its Error.
const (
	errMissing    = "missing"
	errUnreadable = "unreadable"
	errNotFolder  = "not_folder"
)

// readable tells why Polyfin cannot list a folder: it is missing (a share
// not mounted), refused (its permissions), or not a folder; empty when it
// can.
func readable(path string) string {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errMissing
	case err != nil:
		return errUnreadable
	case !info.IsDir():
		return errNotFolder
	}
	dir, err := os.Open(path)
	if err != nil {
		return errUnreadable
	}
	defer dir.Close()
	if _, err := dir.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return errUnreadable
	}
	return ""
}

// found is a video file a scan found: its size and modification time.
type found struct {
	size     int64
	modified time.Time
}

// walk lists the video files under root, by their path in it, with
// slashes. It follows no symbolic link to a folder, and none to a file out
// of root. skipped are the folders it could not read: what was found in
// them before is not gone.
func walk(ctx context.Context, root string) (files map[string]found, skipped []string, err error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, err
	}
	files = map[string]found{}
	err = filepath.WalkDir(real, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, relErr := filepath.Rel(real, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if err != nil {
			if path == real {
				return err
			}
			// A folder it cannot list keeps what was found in it.
			skipped = append(skipped, rel)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case path == real:
			return nil
		case d.IsDir():
			if skippedFolder(d.Name()) {
				return fs.SkipDir
			}
			return nil
		case !videoFile(d.Name()):
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !within(real, target) {
				return nil
			}
			info, err := os.Stat(target)
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			files[rel] = found{size: info.Size(), modified: info.ModTime().UTC().Truncate(time.Microsecond)}
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				skipped = append(skipped, rel)
				return nil
			}
			files[rel] = found{size: info.Size(), modified: info.ModTime().UTC().Truncate(time.Microsecond)}
		}
		return nil
	})
	return files, skipped, err
}

// within reports whether path is root or under it, both clean.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// under reports whether rel is one of the folders skipped or in one.
func under(rel string, skipped []string) bool {
	for _, folder := range skipped {
		if folder == "." || rel == folder || strings.HasPrefix(rel, folder+"/") {
			return true
		}
	}
	return false
}

// claim marks a scan of the folder under way; it reports false when one
// already is.
func (s *Service) claim(id accounts.ID) (release func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.running[id]; busy {
		return nil, false
	}
	done := make(chan struct{})
	s.running[id] = done
	return func() {
		s.mu.Lock()
		delete(s.running, id)
		s.scanned[id] = true
		s.mu.Unlock()
		close(done)
	}, true
}

// StartScan scans a folder in the background, unless a scan of it is
// under way.
func (s *Service) StartScan(id accounts.ID) {
	release, ok := s.claim(id)
	if !ok {
		return
	}
	s.wg.Go(func() {
		defer release()
		if err := s.scan(s.ctx, id); err != nil && s.ctx.Err() == nil {
			s.logger.Warn("A local folder could not be scanned", "folder", id, "error", err)
		}
	})
}

// Scan scans a folder now, once a scan of it under way ended.
func (s *Service) Scan(ctx context.Context, id accounts.ID) error {
	for {
		release, ok := s.claim(id)
		if ok {
			defer release()
			return s.scan(ctx, id)
		}
		s.mu.Lock()
		done := s.running[id]
		s.mu.Unlock()
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ScanDue scans, one after the other, the enabled folders not scanned
// since the server started, and those whose last scan is LocalScanHours
// old; all scans every folder. A folder that cannot be read is no failure
// of the run: it is reported on the folder.
func (s *Service) ScanDue(ctx context.Context, all bool) error {
	installed, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return err
	}
	// The connections of the shares removed since are closed.
	local := map[accounts.ID]bool{}
	for _, addon := range installed {
		local[addon.ID] = addon.Local()
	}
	s.forgetRemotes(func(id accounts.ID) bool { return local[id] })
	hours := s.settings().LocalScanHours
	for _, addon := range installed {
		if !addon.Local() || !addon.Enabled && !all {
			continue
		}
		s.mu.Lock()
		scanned := s.scanned[addon.ID]
		s.mu.Unlock()
		if !all && scanned {
			if hours == 0 {
				continue
			}
			var checked *time.Time
			if err := s.db.QueryRow(ctx, "SELECT checked_at FROM local_folders WHERE addon_id = $1", addon.ID).Scan(&checked); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return err
			}
			if checked != nil && s.now().Sub(*checked) < time.Duration(hours)*time.Hour {
				continue
			}
		}
		if err := s.Scan(ctx, addon.ID); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
	return nil
}

// stored is a file a scan found before.
type stored struct {
	size     int64
	modified time.Time
	unit     string
	matched  bool
	reason   string
}

// scan reads a folder: files gone are forgotten, new and changed ones read
// and matched, and those left unmatched matched again, unless their name
// could not be read; a unit with files already matched keeps their title.
func (s *Service) scan(ctx context.Context, id accounts.ID) error {
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return err
	}
	root := folder.Addon.ManifestURL
	at := s.now()
	var files map[string]found
	var skipped []string
	failure := ""
	if folder.Share != "" {
		var sealed string
		if err := s.db.QueryRow(ctx, "SELECT share_password FROM local_folders WHERE addon_id = $1", id).Scan(&sealed); err != nil {
			return err
		}
		if files, skipped, failure, err = s.shareFiles(ctx, id, root, folder.User, sealed); err != nil {
			return err
		}
	} else if failure = readable(root); failure != "" {
		s.logger.Warn("A local folder cannot be read", "folder", folder.Addon.Manifest.Name, "path", root, "reason", failure)
	} else if files, skipped, err = walk(ctx, root); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.logger.Warn("A local folder cannot be read", "folder", folder.Addon.Manifest.Name, "path", root, "error", err)
		failure = errUnreadable
	}
	if failure != "" {
		// What was found before stays.
		_, err := s.db.Exec(ctx, "UPDATE local_folders SET checked_at = $2, error = $3 WHERE addon_id = $1", id, at, failure)
		return err
	}
	known, err := s.storedFiles(ctx, id)
	if err != nil {
		return err
	}
	var gone []string
	for rel := range known {
		if _, ok := files[rel]; !ok && !under(rel, skipped) {
			gone = append(gone, rel)
		}
	}
	if len(gone) > 0 {
		if _, err := s.db.Exec(ctx, "DELETE FROM local_files WHERE addon_id = $1 AND path = ANY($2)", id, gone); err != nil {
			return err
		}
	}
	// The units to match, with their files to store.
	units := map[string][]entry{}
	for rel, f := range files {
		old, ok := known[rel]
		e := readEntry(folder.Kind, rel, f)
		switch {
		case !ok || old.size != f.size || !old.modified.Equal(f.modified):
		case old.matched || old.reason == reasonUnreadableName:
			continue
		}
		units[e.unit] = append(units[e.unit], e)
	}
	links, err := s.links(ctx, id)
	if err != nil {
		return err
	}
	matched, err := s.matchedUnits(ctx, id)
	if err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	searches := newSearches()
	for unit, entries := range units {
		group.Go(func() error {
			// A unit matched already, a show gaining an episode or a file
			// that changed, keeps its title: its name is the same.
			m, ok := matched[unit]
			if !ok {
				var err error
				if m, err = s.unitMatch(groupCtx, folder.Kind, entries[0], links[unit], searches); err != nil {
					return err
				}
			}
			return s.store(groupCtx, id, folder.Kind, entries, m)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "UPDATE local_folders SET checked_at = $2, scanned_at = $2, error = '' WHERE addon_id = $1", id, at)
	return err
}

// storedFiles reads the files a folder's scans found, by path.
func (s *Service) storedFiles(ctx context.Context, id accounts.ID) (map[string]stored, error) {
	rows, err := s.db.Query(ctx, "SELECT path, size, modified, unit, stremio_id IS NOT NULL, reason FROM local_files WHERE addon_id = $1", id)
	if err != nil {
		return nil, err
	}
	known := map[string]stored{}
	for rows.Next() {
		var rel string
		var f stored
		if err := rows.Scan(&rel, &f.size, &f.modified, &f.unit, &f.matched, &f.reason); err != nil {
			rows.Close()
			return nil, err
		}
		known[rel] = f
	}
	return known, rows.Err()
}

// links reads the links a folder's units have, by unit.
func (s *Service) links(ctx context.Context, id accounts.ID) (map[string]string, error) {
	rows, err := s.db.Query(ctx, "SELECT unit, imdb_id FROM local_links WHERE addon_id = $1", id)
	if err != nil {
		return nil, err
	}
	links := map[string]string{}
	for rows.Next() {
		var unit, imdb string
		if err := rows.Scan(&unit, &imdb); err != nil {
			rows.Close()
			return nil, err
		}
		links[unit] = imdb
	}
	return links, rows.Err()
}

// entry is a file to store, with what its path tells.
type entry struct {
	path  string
	file  found
	unit  string
	title name
	// episode is what an episode's name tells, its season and numbers.
	episode name
}

// readEntry reads what a file's path tells in a folder of kind.
func readEntry(kind, rel string, f found) entry {
	if kind == KindShows {
		show, episode, unit := episodeName(rel)
		return entry{path: rel, file: f, unit: unit, title: show, episode: episode}
	}
	return entry{path: rel, file: f, unit: rel, title: movieName(rel)}
}

// store saves a unit's files with the title they were matched to, or why
// they were not. An episode whose numbers its name does not tell is
// unmatched, whatever its show.
func (s *Service) store(ctx context.Context, id accounts.ID, kind string, entries []entry, m match) error {
	batch := &pgx.Batch{}
	for _, e := range entries {
		var season, episode, last *int
		if e.episode.episodic {
			season, episode, last = &e.episode.season, &e.episode.episode, &e.episode.lastEpisode
		}
		height := e.title.height
		if e.episode.height > 0 {
			height = e.episode.height
		}
		f := m
		if kind == KindShows && !e.episode.episodic && m.stremioID != "" {
			f = match{reason: reasonUnreadableName}
		}
		var stremioID *string
		if f.stremioID != "" {
			stremioID = &f.stremioID
		}
		var year, titleYear *int
		if e.title.year > 0 {
			year = &e.title.year
		}
		if f.year > 0 {
			titleYear = &f.year
		}
		var heightValue *int
		if height > 0 {
			heightValue = &height
		}
		batch.Queue(`INSERT INTO local_files (addon_id, path, size, modified, unit, title, year, season, episode, last_episode, height,
				stremio_id, imdb_id, tmdb_id, name, poster, title_year, matched_by, reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, unit = $5, title = $6, year = $7, season = $8, episode = $9,
				last_episode = $10, height = $11, stremio_id = $12, imdb_id = $13, tmdb_id = $14, name = $15, poster = $16, title_year = $17,
				matched_by = $18, reason = $19`,
			id, e.path, e.file.size, e.file.modified, e.unit, e.title.title, year, season, episode, last, heightValue,
			stremioID, f.imdb, f.tmdb, f.name, f.poster, titleYear, f.by, f.reason)
	}
	return s.db.SendBatch(ctx, batch).Close()
}
