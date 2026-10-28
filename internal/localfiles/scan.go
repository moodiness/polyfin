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
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/stremio"
)

// ScanCheck is how often the folders due for a scan are looked for (see
// ScanDue).
const ScanCheck = 10 * time.Minute

// searchRetry is how long after its scan a folder some of whose titles'
// searches failed n times in a row before is scanned again, to search them
// again: firstSearchRetry, doubled after each failure, at most
// maxSearchRetry. The scans due look for it every ScanCheck.
func searchRetry(n int) time.Duration {
	if n >= 7 {
		return maxSearchRetry
	}
	return min(firstSearchRetry<<n, maxSearchRetry)
}

const (
	firstSearchRetry = 5 * time.Minute
	maxSearchRetry   = 6 * time.Hour
)

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
	file = file || kind != KindMusic && (imageFile(name) || dvdFile(path.Base(path.Dir(rel)), name) || playlistFile(rel) ||
		kind != KindHomeVideos && themeAudio(rel))
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

// claim marks a scan of the folder under way, done under ctx; it reports
// false when one already is. The scan's context ends with release, or
// when the folder's scan is stopped (see hold). The scans count in the
// metrics, not the claims of hold.
func (s *Service) claim(ctx context.Context, id accounts.ID) (scanCtx context.Context, release func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.running[id]; busy {
		return nil, nil, false
	}
	scanCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	s.running[id] = scanning{done: done, stop: stop}
	return scanCtx, func() {
		stop()
		s.mu.Lock()
		delete(s.running, id)
		s.scanned[id] = true
		s.mu.Unlock()
		close(done)
	}, true
}

// hold stops the scan of the folder under way, waits for it to end, and
// keeps the folder from being scanned until release: what it holds
// changes meanwhile.
func (s *Service) hold(id accounts.ID) (release func()) {
	for {
		_, release, ok := s.claim(context.Background(), id)
		if ok {
			return release
		}
		s.mu.Lock()
		current, busy := s.running[id]
		s.mu.Unlock()
		if busy {
			current.stop()
			<-current.done
		}
	}
}

// StartScan scans a folder in the background, unless a scan of it is
// under way.
func (s *Service) StartScan(id accounts.ID) {
	scanCtx, release, ok := s.claim(s.ctx, id)
	if !ok {
		return
	}
	finished := metrics.ScanStarted()
	s.wg.Go(func() {
		defer release()
		defer finished()
		// Neither a stopped scan nor one the service's end cut short fails.
		if err := s.scan(scanCtx, id); err != nil && scanCtx.Err() == nil {
			s.logger.Warn("A local folder could not be scanned", "folder", id, "error", err)
		}
	})
}

// Scan scans a folder now, once a scan of it under way ended. A scan
// stopped as what the folder holds changes ends without an error: the
// folder is scanned again.
func (s *Service) Scan(ctx context.Context, id accounts.ID) error {
	for {
		scanCtx, release, ok := s.claim(ctx, id)
		if ok {
			defer release()
			defer metrics.ScanStarted()()
			err := s.scan(scanCtx, id)
			if err != nil && scanCtx.Err() != nil && ctx.Err() == nil {
				return nil
			}
			return err
		}
		s.mu.Lock()
		current, busy := s.running[id]
		s.mu.Unlock()
		if !busy {
			continue
		}
		select {
		case <-current.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ScanDue scans, one after the other, the enabled folders not scanned
// since the server started, those whose last scan is LocalScanHours old,
// and those whose titles' searches failed once their retry is due (see
// searchRetry); all scans every folder. A folder that cannot be read is
// no failure of the run: it is reported on the folder.
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
			var checked, retry *time.Time
			if err := s.db.QueryRow(ctx, "SELECT checked_at, search_retry_at FROM local_folders WHERE addon_id = $1", addon.ID).
				Scan(&checked, &retry); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return err
			}
			now := s.now()
			retrying := retry != nil && !now.Before(*retry)
			stale := hours > 0 && (checked == nil || now.Sub(*checked) >= time.Duration(hours)*time.Hour)
			if !retrying && !stale {
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

// searchesDone tells, once a scan matched a folder's units, how their
// searches went: when files were left unmatched as their searches failed,
// the log tells it once, and the folder is scanned again after
// searchRetry, or after the longest wait an addon asked for when longer;
// else it is not any more.
func (s *Service) searchesDone(ctx context.Context, folder Folder, done *searches, files int) error {
	if done.failed == 0 {
		if folder.SearchRetryAt == nil && folder.SearchRetries == 0 {
			return nil
		}
		_, err := s.db.Exec(ctx, "UPDATE local_folders SET search_retry_at = NULL, search_retries = 0 WHERE addon_id = $1", folder.Addon.ID)
		return err
	}
	retry := s.now().Add(max(searchRetry(folder.SearchRetries), done.wait))
	s.logger.Warn("The metadata addons could not search the titles of a local folder's files", "folder", folder.Addon.Manifest.Name,
		"files", files, "addon", done.first.Addon, "failure", done.first.Failure, "error", done.first.Err, "retry", retry)
	_, err := s.db.Exec(ctx, "UPDATE local_folders SET search_retry_at = $2, search_retries = search_retries + 1 WHERE addon_id = $1",
		folder.Addon.ID, retry)
	return err
}

// retryLater has a folder scanned again after searchRetry, unless it is
// already to be: m is the match of one of its units outside a scan (see
// rematch), whose search failed.
func (s *Service) retryLater(ctx context.Context, id accounts.ID, m match) error {
	if m.reason != reasonSearchFailed {
		return nil
	}
	_, err := s.db.Exec(ctx, `UPDATE local_folders SET search_retry_at = $2, search_retries = 1
		WHERE addon_id = $1 AND search_retry_at IS NULL`, id, s.now().Add(searchRetry(0)))
	return err
}

// stored is a file a scan found before, the identifier its unit's NFO
// file gave (see nfoID), its episodes, as formatEpisode writes them, and
// whether its plain video's identifier is stored (see plainID).
type stored struct {
	size     int64
	modified time.Time
	unit     string
	matched  bool
	reason   string
	nfo      string
	episodes string
	plain    bool
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
	var links map[string]string
	var exact map[string]bool
	if folder.Kind == KindMovies || folder.Kind == KindShows {
		if links, err = s.links(ctx, id); err != nil {
			return err
		}
		if exact, err = s.exactUnits(ctx, id); err != nil {
			return err
		}
	}
	extras := splitExtras(folder.Kind, folder.ReadExtras, links, &list)
	if folder.Kind == KindMusic {
		local := root
		if folder.Share != "" {
			local = ""
		}
		if err := s.scanMusic(ctx, id, list, local); err != nil {
			return err
		}
		if err := s.finish(ctx, id, at, list); err != nil {
			return err
		}
		s.measureMusic(id)
		return nil
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
	// The titles each extra belongs to; a movie's extra named with a
	// suffix that belongs to none is a file of its own, left unmatched.
	owned, loneExtras := extraOwners(folder.Kind, extras, entries)
	lone := make(map[string]bool, len(loneExtras))
	for _, rel := range loneExtras {
		files[rel] = extras[rel].file
		entries[rel] = readEntry(folder.Kind, rootName, rel, files[rel])
		lone[entries[rel].unit] = true
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
	places := unitNFOs(folder.Kind, entries, list.sides)
	for rel := range owned {
		if nfo := extraNFO(rel, extras[rel], list.sides); nfo != "" {
			places[extraPlace(rel)] = nfo
		}
	}
	infos, err := s.readNFOs(ctx, id, list, places)
	if err != nil {
		return err
	}
	// The units to match, with their files to store. A file read as
	// another unit than before, as each stream of a disc was, is read
	// again, as are the files of a unit whose NFO file gives another
	// identifier, or none any more, and the titles of shows' discs whose
	// episodes changed. A lone extra is unmatched, whatever it was before.
	// A file whose name could not be read is stored again once, as a plain
	// video, when scans before plain videos stored it.
	units := map[string][]entry{}
	renewed := map[string]bool{}
	var fresh []string
	for rel, e := range entries {
		e.nfo = infos[places[e.unit]].id()
		old, ok := known[rel]
		changed := !ok || old.size != e.file.size || !old.modified.Equal(e.file.modified) || old.unit != e.unit ||
			old.episodes != formatEpisode(e.episode)
		if changed {
			fresh = append(fresh, rel)
		}
		switch {
		case lone[e.unit]:
			if !changed && old.reason == reasonLoneExtra {
				continue
			}
		case ok && old.nfo != e.nfo:
			renewed[e.unit] = true
		case changed:
		case old.matched || old.reason == reasonUnreadableName && old.plain || old.reason == reasonDiscTitle || old.reason == reasonDiscEpisode:
			continue
		}
		units[e.unit] = append(units[e.unit], e)
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
	var failedFiles atomic.Int64
	for unit, entries := range units {
		group.Go(func() error {
			// A unit matched already, a show gaining an episode or a file
			// that changed, keeps its title: its name is the same. One
			// whose NFO file changed is matched again.
			m, ok := matched[unit]
			switch {
			case lone[unit]:
				m = match{reason: reasonLoneExtra}
			case !ok || renewed[unit]:
				var err error
				if m, err = s.unitMatch(groupCtx, folder.Kind, entries[0], links[unit], searches, exact[unit]); err != nil {
					return err
				}
				if err = s.noteLink(groupCtx, id, unit, links[unit], m); err != nil {
					return err
				}
			}
			if m.reason == reasonSearchFailed {
				failedFiles.Add(int64(len(entries)))
			}
			return s.store(groupCtx, id, folder.Kind, entries, m)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if err := s.searchesDone(ctx, folder, searches, int(failedFiles.Load())); err != nil {
		return err
	}
	videos := entries
	if len(owned) > 0 {
		videos = maps.Clone(entries)
	}
	for rel, row := range owned {
		row.name = extraName(rel, extras[rel], infos[places[extraPlace(rel)]])
		owned[rel] = row
		videos[rel] = entry{path: rel, file: row.file, unit: rel}
	}
	if err := s.storeExtras(ctx, id, owned, list.skipped); err != nil {
		return err
	}
	if err := s.storeSubtitles(ctx, id, list, sidecars(folder.Kind, rootName, videos, list.sides)); err != nil {
		return err
	}
	if err := s.storeArtwork(ctx, id, folder.Kind, root, list); err != nil {
		return err
	}
	if err := s.finish(ctx, id, at, list); err != nil {
		return err
	}
	s.tellScanned(ctx, id, fresh)
	return nil
}

// tellScanned tells the videos of a folder a scan found new or changed, as
// Streams lists them, to what UseScanned set. A failure to list them is
// only logged: the scan itself went through.
func (s *Service) tellScanned(ctx context.Context, id accounts.ID, paths []string) {
	if s.found == nil || len(paths) == 0 || s.files == "" {
		return
	}
	rows, err := s.db.Query(ctx, "SELECT id, path FROM local_files WHERE addon_id = $1 AND path = ANY($2) ORDER BY path", id, paths)
	if err != nil {
		s.logger.Warn("The videos a scan found could not be listed", "error", err)
		return
	}
	var files []accounts.ID
	videos, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Stream, error) {
		var file accounts.ID
		var rel string
		if err := row.Scan(&file, &rel); err != nil {
			return stremio.Stream{}, err
		}
		files = append(files, file)
		return stremio.Stream{URL: s.address(file, rel)}, nil
	})
	if err == nil {
		var subtitles map[accounts.ID][]stremio.Subtitle
		if subtitles, err = s.fileSubtitles(ctx, files); err == nil {
			for i := range videos {
				videos[i].Subtitles = subtitles[files[i]]
			}
		}
	}
	if err != nil {
		s.logger.Warn("The videos a scan found could not be listed", "error", err)
		return
	}
	s.found(videos)
}

// finish marks a folder scanned at at, keeps the prints of a share's
// folders its listing has (see Service.shareSeen), forgets what was read
// in the audio of files gone or changed (see forgetAudio), and gives what
// users did with the titles its files were before what it holds changed to
// those they are now (see follow).
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
	if err := s.forgetAudio(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("What was read in the audio of local files gone or changed could not be deleted", "error", err)
	}
	return s.follow(ctx, id, list.skipped)
}

// audioKept is the condition the rows of the audio tables, of alias r,
// meet while a folder still has their file as it was read: at the same
// path (see EpisodeFile), size and modification time.
const audioKept = `EXISTS (SELECT 1 FROM local_files f JOIN addons a ON a.id = f.addon_id
	WHERE regexp_replace(a.manifest_url, '/$', '') || '/' || f.path = r.path AND f.size = r.size AND f.modified = r.modified)`

// forgetAudio deletes the fingerprints and the segments kept for the audio
// of local episodes (see package audiosegments) whose file no folder has
// as it was read: gone, renamed, changed, or of a folder removed or moved.
func (s *Service) forgetAudio(ctx context.Context) error {
	batch := &pgx.Batch{}
	batch.Queue("DELETE FROM audio_fingerprints r WHERE NOT " + audioKept)
	batch.Queue("DELETE FROM audio_segments r WHERE NOT " + audioKept)
	return s.db.SendBatch(ctx, batch).Close()
}

// storedFiles reads the files a folder's scans found, by path.
func (s *Service) storedFiles(ctx context.Context, id accounts.ID) (map[string]stored, error) {
	rows, err := s.db.Query(ctx, `SELECT path, size, modified, unit, stremio_id IS NOT NULL, reason, nfo_id, season, episode, last_episode,
		plain_id IS NOT NULL FROM local_files WHERE addon_id = $1`, id)
	if err != nil {
		return nil, err
	}
	known := map[string]stored{}
	for rows.Next() {
		var rel string
		var f stored
		var season, episode, last *int
		if err := rows.Scan(&rel, &f.size, &f.modified, &f.unit, &f.matched, &f.reason, &f.nfo, &season, &episode, &last, &f.plain); err != nil {
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
// episodes neither an administrator nor the disc's name gives. A file
// stored for the first time is added now by the server's clock, which
// the scans' times and the windows Added is asked for are taken from
// too: the database's clock can be another machine's. A file listed as a
// plain video keeps the items it is as its former titles, so that what
// users do with them follows it once matched (see plainvideos.go).
func (s *Service) store(ctx context.Context, id accounts.ID, kind string, entries []entry, m match) error {
	batch := &pgx.Batch{}
	added := s.now()
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
		plain := plainID(id, e.unit)
		batch.Queue(`INSERT INTO local_files (addon_id, path, size, modified, unit, title, year, season, episode, last_episode, height,
				stremio_id, imdb_id, tmdb_id, name, poster, title_year, matched_by, reason, nfo_id, length, search_addon, search_failure, added_at,
				plain_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, unit = $5, title = $6, year = $7, season = $8, episode = $9,
				last_episode = $10, height = $11, stremio_id = $12, imdb_id = $13, tmdb_id = $14, name = $15, poster = $16, title_year = $17,
				matched_by = $18, reason = $19, nfo_id = $20, length = $21, search_addon = $22, search_failure = $23, plain_id = $25`,
			id, e.path, e.file.size, e.file.modified, e.unit, e.title.title, year, season, episode, last, heightValue,
			stremioID, f.imdb, f.tmdb, f.name, f.poster, titleYear, f.by, f.reason, e.nfo, length, f.searchAddon, f.searchFailure, added, plain)
		if !plainFile(kind, f.stremioID, f.reason, e.episode.episodic) {
			continue
		}
		for _, item := range titleItems(kind, plain, season, episode, last) {
			batch.Queue("INSERT INTO local_former_titles (addon_id, path, item_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", id, e.path, item.ID)
		}
	}
	return s.db.SendBatch(ctx, batch).Close()
}
