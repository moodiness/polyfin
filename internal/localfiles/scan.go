package localfiles

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
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

// found is a file a scan found: its size and modification time. extra
// marks the entry of a disc's title other than its main one, a unit of its
// own (see titleUnit); length is how long a disc's title plays, as the
// disc's tables tell it, 0 for another file.
type found struct {
	size     int64
	modified time.Time
	extra    bool
	length   time.Duration
}

// listing is what a walk found under a folder, by path in it, with
// slashes: the files it reads, videos with disc images (see imageFile), the
// files of DVDs (see dvdFile) and Blu-ray discs' playlists (see
// playlistFile), or a music folder's audio files, and the side files beside
// them (see listed). skipped are the folders it could not read: what was
// found in them before is not gone. open opens one of the files found for
// reading, seekable; root is a folder's real path in the container, empty
// for a share. prints are a share's folders listed, by path, "" for its
// root, with their print (see folderPrint).
type listing struct {
	files   map[string]found
	sides   map[string]found
	skipped []string
	open    func(ctx context.Context, rel string) (io.ReadCloser, error)
	root    string
	prints  map[string]uint64
}

// walk lists the files a scan of a folder of kind reads under root, disc
// images, the files of DVDs and Blu-ray discs' playlists among them, and
// their side files. It follows no symbolic link to a folder, and none to a
// file out of root.
func walk(ctx context.Context, root, kind string) (listing, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return listing{}, err
	}
	list := listing{files: map[string]found{}, sides: map[string]found{}, root: real, open: func(_ context.Context, rel string) (io.ReadCloser, error) {
		target, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if !within(real, target) {
			return nil, fs.ErrNotExist
		}
		return os.Open(target)
	}}
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
			list.skipped = append(list.skipped, rel)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		var info fs.FileInfo
		file, side := false, false
		if !d.IsDir() {
			file, side = walked(kind, rel)
		}
		switch {
		case path == real:
			return nil
		case d.IsDir():
			if skippedIn(kind, d.Name()) {
				return fs.SkipDir
			}
			return nil
		case !file && !side:
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !within(real, target) {
				return nil
			}
			if info, err = os.Stat(target); err != nil || !info.Mode().IsRegular() {
				return nil
			}
		case d.Type().IsRegular():
			if info, err = d.Info(); err != nil {
				list.skipped = append(list.skipped, rel)
				return nil
			}
		default:
			return nil
		}
		f := found{size: info.Size(), modified: info.ModTime().UTC().Truncate(time.Microsecond)}
		if !file {
			list.sides[rel] = f
		} else {
			list.files[rel] = f
		}
		return nil
	})
	return list, err
}

// walked tells which files a walk of a folder of kind lists, by their
// path, with slashes: those listed tells, and outside music folders, disc
// images, a DVD's files and a Blu-ray disc's playlists, read as their disc
// (see collapseImages, collapseDVDs, collapseDiscs).
func walked(kind, rel string) (file, side bool) {
	name := path.Base(rel)
	file, side = listed(kind, name)
	file = file || kind != KindMusic && (imageFile(name) || dvdFile(path.Base(path.Dir(rel)), name) || playlistFile(rel))
	return file, side
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
	// The connections of the shares removed since are closed, and what
	// their scans listed forgotten.
	local := map[accounts.ID]bool{}
	for _, addon := range installed {
		local[addon.ID] = addon.Local()
	}
	s.forgetRemotes(func(id accounts.ID) bool { return local[id] })
	s.mu.Lock()
	maps.DeleteFunc(s.prints, func(id accounts.ID, _ map[string]uint64) bool { return !local[id] })
	maps.DeleteFunc(s.kept, func(id accounts.ID, _ int) bool { return !local[id] })
	s.mu.Unlock()
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

// stored is a file a scan found before, the identifier its unit's NFO
// file gave (see nfoID), and its episodes, as formatEpisode writes them.
type stored struct {
	size     int64
	modified time.Time
	unit     string
	matched  bool
	reason   string
	nfo      string
	episodes string
}

// scan reads a folder: files gone are forgotten, new and changed ones read
// and matched, and those left unmatched matched again, unless their name
// could not be read; a unit with files already matched keeps their title,
// unless the identifier its NFO file gives changed. The subtitle files
// beside the videos and the titles' artwork are stored as they are now
// (see storeSubtitles and storeArtwork).
func (s *Service) scan(ctx context.Context, id accounts.ID) error {
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return err
	}
	root := folder.Addon.ManifestURL
	at := s.now()
	var list listing
	failure := ""
	if folder.Share != "" {
		var sealed string
		if err := s.db.QueryRow(ctx, "SELECT share_password FROM local_folders WHERE addon_id = $1", id).Scan(&sealed); err != nil {
			return err
		}
		if list, failure, err = s.shareFiles(ctx, id, root, folder.User, sealed, folder.Kind); err != nil {
			return err
		}
	} else if failure = readable(root); failure != "" {
		s.logger.Warn("A local folder cannot be read", "folder", folder.Addon.Manifest.Name, "path", root, "reason", failure)
	} else if list, err = walk(ctx, root, folder.Kind); err != nil {
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
	if folder.Kind == KindMusic {
		local := root
		if folder.Share != "" {
			local = ""
		}
		if err := s.scanMusic(ctx, id, list, local); err != nil {
			return err
		}
		return s.finish(ctx, id, at, list)
	}
	if folder.Kind == KindHomeVideos {
		if err := s.scanHomeVideos(ctx, id, root, list); err != nil {
			return err
		}
		return s.finish(ctx, id, at, list)
	}
	files := list.files
	collapseDiscs(folder.Kind, files, func(rel string) (io.ReadCloser, error) { return list.open(ctx, rel) })
	choose := titleChooser(folder.Kind)
	if folder.Share == "" {
		collapseDVDs(folder.Kind, files, func(videoTS string) ([]dvdChoice, error) {
			return dvdTitles(filepath.Join(root, filepath.FromSlash(videoTS)), choose)
		})
	} else {
		collapseDVDs(folder.Kind, files, func(videoTS string) ([]dvdChoice, error) {
			return dvdTitlesOf(func(name string) (io.ReaderAt, error) { return readIFO(ctx, list, path.Join(videoTS, name)) }, choose)
		})
	}
	if err := s.collapseImages(ctx, id, folder, root, list); err != nil {
		return err
	}
	known, err := s.storedFiles(ctx, id)
	if err != nil {
		return err
	}
	var gone []string
	for rel := range known {
		if _, ok := files[rel]; !ok && !under(rel, list.skipped) {
			gone = append(gone, rel)
		}
	}
	if len(gone) > 0 {
		if _, err := s.db.Exec(ctx, "DELETE FROM local_files WHERE addon_id = $1 AND path = ANY($2)", id, gone); err != nil {
			return err
		}
	}
	rootName := locationName(root)
	entries := make(map[string]entry, len(files))
	for rel, f := range files {
		entries[rel] = readEntry(folder.Kind, rootName, rel, f)
	}
	if folder.Kind == KindShows {
		given, err := s.givenEpisodes(ctx, id)
		if err != nil {
			return err
		}
		numberDiscs(entries, given)
	}
	places := unitNFOs(folder.Kind, entries, list.sides)
	infos, err := s.readNFOs(ctx, id, list, places)
	if err != nil {
		return err
	}
	// The units to match, with their files to store. A file read as
	// another unit than before, as each stream of a disc was, is read
	// again, as are the files of a unit whose NFO file gives another
	// identifier, or none any more, and the titles of shows' discs whose
	// episodes changed.
	units := map[string][]entry{}
	renewed := map[string]bool{}
	for rel, e := range entries {
		e.nfo = infos[places[e.unit]].id()
		old, ok := known[rel]
		switch {
		case ok && old.nfo != e.nfo:
			renewed[e.unit] = true
		case !ok || old.size != e.file.size || !old.modified.Equal(e.file.modified) || old.unit != e.unit ||
			old.episodes != formatEpisode(e.episode):
		case old.matched || old.reason == reasonUnreadableName || old.reason == reasonDiscTitle || old.reason == reasonDiscEpisode:
			continue
		}
		units[e.unit] = append(units[e.unit], e)
	}
	links, err := s.links(ctx, id)
	if err != nil {
		return err
	}
	if folder.Kind == KindMovies {
		if links, err = s.moveStreamLinks(ctx, id, files, links); err != nil {
			return err
		}
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
			// that changed, keeps its title: its name is the same. One
			// whose NFO file changed is matched again.
			m, ok := matched[unit]
			if !ok || renewed[unit] {
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
	if err := s.storeSubtitles(ctx, id, list, sidecars(folder.Kind, rootName, entries, list.sides)); err != nil {
		return err
	}
	if err := s.storeArtwork(ctx, id, folder.Kind, root, list); err != nil {
		return err
	}
	return s.finish(ctx, id, at, list)
}

// finish marks a folder scanned at at, and keeps the prints of a share's
// folders its listing has (see Service.shareSeen).
func (s *Service) finish(ctx context.Context, id accounts.ID, at time.Time, list listing) error {
	if _, err := s.db.Exec(ctx, "UPDATE local_folders SET checked_at = $2, scanned_at = $2, error = '' WHERE addon_id = $1", id, at); err != nil {
		return err
	}
	if list.prints != nil {
		s.mu.Lock()
		s.prints[id] = list.prints
		s.kept[id]++
		s.mu.Unlock()
	}
	return nil
}

// storedFiles reads the files a folder's scans found, by path.
func (s *Service) storedFiles(ctx context.Context, id accounts.ID) (map[string]stored, error) {
	rows, err := s.db.Query(ctx, `SELECT path, size, modified, unit, stremio_id IS NOT NULL, reason, nfo_id, season, episode, last_episode
		FROM local_files WHERE addon_id = $1`, id)
	if err != nil {
		return nil, err
	}
	known := map[string]stored{}
	for rows.Next() {
		var rel string
		var f stored
		var season, episode, last *int
		if err := rows.Scan(&rel, &f.size, &f.modified, &f.unit, &f.matched, &f.reason, &f.nfo, &season, &episode, &last); err != nil {
			rows.Close()
			return nil, err
		}
		if season != nil && episode != nil && last != nil {
			f.episodes = formatEpisode(name{episodic: true, season: *season, episode: *episode, lastEpisode: *last})
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

// entry is a file to store, with what its path tells, and the identifier
// its unit's NFO file gives (see nfoID).
type entry struct {
	path  string
	file  found
	unit  string
	title name
	// episode is what an episode's name tells, its season and numbers.
	episode name
	nfo     string
}

// readEntry reads what a file's path tells in a folder of kind, root being
// the folder's name (see locationName): a disc's entry, a Blu-ray disc's
// playlist or stream or a DVD's VOB, is read as its disc, matched by its
// folder's name. A disc's title other than its main one, a disc image's
// included, is a unit of its own, which its name does not tell: it is
// matched by hand. A title of a show's disc is one of its show's files,
// its episodes given apart (see numberDiscs).
func readEntry(kind, root, rel string, f found) entry {
	if _, _, ok := showTitle(rel); ok && kind == KindShows {
		top, _, _ := strings.Cut(rel, "/")
		show := parseName(top)
		show.episodic = false
		f.extra = false
		return entry{path: rel, file: f, unit: top, title: show}
	}
	if kind == KindShows {
		show, episode, unit := episodeName(rel)
		return entry{path: rel, file: f, unit: unit, title: show, episode: episode}
	}
	if disc, number, ok := discPlaylist(rel); ok && f.extra {
		return entry{path: rel, file: f, unit: titleUnit(disc, number)}
	}
	if base, number, ok := titleOf(rel); ok {
		f.extra = true
		if _, disc, ok := discEntry(rel); ok {
			return entry{path: rel, file: f, unit: titleUnit(disc, number)}
		}
		return entry{path: rel, file: f, unit: titleUnit(base, number)}
	}
	if _, disc, ok := discEntry(rel); ok {
		return entry{path: rel, file: f, unit: discUnit(disc), title: discName(disc, root)}
	}
	return entry{path: rel, file: f, unit: rel, title: movieName(rel)}
}

// moveStreamLinks moves to their disc the links made on a disc's streams
// when scans read each stream as a movie of its own: a stream is no unit
// any more. A disc without a link takes its stream's, the one of the
// stream it is read as when several are linked, else the first by path;
// the stream's links are removed. The links of a disc not found now stay.
func (s *Service) moveStreamLinks(ctx context.Context, id accounts.ID, files map[string]found, links map[string]string) (map[string]string, error) {
	kept := map[string]string{}
	for rel, f := range files {
		if kind, disc, ok := discEntry(rel); ok && kind == DiscBluRay && !f.extra {
			kept[discUnit(disc)] = rel
		}
	}
	var stale []string
	taken := map[string]string{}
	for unit := range links {
		disc, ok := discStream(unit)
		if !ok {
			continue
		}
		target := discUnit(disc)
		stream, found := kept[target]
		if !found {
			continue
		}
		stale = append(stale, unit)
		if links[target] != "" {
			continue
		}
		if best, ok := taken[target]; !ok || unit == stream || best != stream && unit < best {
			taken[target] = unit
		}
	}
	if len(stale) == 0 {
		return links, nil
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		for target, unit := range taken {
			if _, err := tx.Exec(ctx, "INSERT INTO local_links (addon_id, unit, imdb_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
				id, target, links[unit]); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "DELETE FROM local_links WHERE addon_id = $1 AND unit = ANY($2)", id, stale)
		return err
	})
	if err != nil {
		return nil, err
	}
	for target, unit := range taken {
		links[target] = links[unit]
	}
	for _, unit := range stale {
		delete(links, unit)
	}
	return links, nil
}

// store saves a unit's files with the title they were matched to, or why
// they were not. An episode whose numbers its name does not tell is
// unmatched, whatever its show, and so is a title of a show's disc whose
// episodes neither an administrator nor the disc's name gives.
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
			if _, _, ok := showTitle(e.path); ok {
				f.reason = reasonDiscEpisode
			}
		}
		var length *int
		if e.file.length > 0 {
			length = new(int(e.file.length / time.Second))
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
				stremio_id, imdb_id, tmdb_id, name, poster, title_year, matched_by, reason, nfo_id, length)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, unit = $5, title = $6, year = $7, season = $8, episode = $9,
				last_episode = $10, height = $11, stremio_id = $12, imdb_id = $13, tmdb_id = $14, name = $15, poster = $16, title_year = $17,
				matched_by = $18, reason = $19, nfo_id = $20, length = $21`,
			id, e.path, e.file.size, e.file.modified, e.unit, e.title.title, year, season, episode, last, heightValue,
			stremioID, f.imdb, f.tmdb, f.name, f.poster, titleYear, f.by, f.reason, e.nfo, length)
	}
	return s.db.SendBatch(ctx, batch).Close()
}
