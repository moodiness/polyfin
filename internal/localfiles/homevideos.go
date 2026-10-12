package localfiles

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// A home videos folder holds videos no metadata addon knows: recordings of
// a family, a concert or a school play. Each video file is a title of its
// own, of library.VideoType, which the folder lists, describes and plays
// itself: nothing is asked of the metadata addons, and nothing is left
// unmatched. A video alone in a folder of its own, with its side files, is
// one title named after the folder; several videos in one folder are as
// many titles, named after their files. A title's identifier is stored
// with its file and kept through every scan and restart, and it follows
// the video renamed or moved: a video found whose contents are those of
// one gone is that video again (see recognize).
//
// A title's name is its NFO file's <title>, else its file's or its own
// folder's name as written, without extension; its date its NFO file's
// (see homeDate), else the creation time its container tells, as phones
// and cameras write it, else the file's modification time; its plot its
// NFO file's. Its artwork is the images beside it, as for a movie (see
// chooseArtwork), else a frame of the video (see storeFrames). Discs and
// disc images are left out, as in a shows folder. A scan probes the videos
// new or changed with ffprobe, those of a share through the loopback server
// music folders read tags through (see probeServer).

// frameShare is how far into a home video its frame is taken, as Jellyfin
// takes the image of a video it has none of.
const frameShare = 0.1

// earliestCreation is the first creation time a container's tag tells
// that is believed: clocks never set give 1904 or 1970.
var earliestCreation = time.Date(1971, time.January, 1, 0, 0, 0, 0, time.UTC)

// newVideoID is the Stremio identifier of a home video found that is no
// video gone (see recognize): random, as the path it is found at may be
// another video's former one, whose identifier that video keeps.
func newVideoID() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return library.VideoIDPrefix + hex.EncodeToString(id[:])
}

// fingerprintSpan is how much of each end of a home video its fingerprint
// reads.
const fingerprintSpan = 1 << 20

// goneDays is how long the home videos gone are remembered, in days, for a
// video found with their contents to take their identifier (see
// recognize).
const goneDays = 30

// videoFingerprint reads the fingerprint of a home video of size bytes: a
// SHA-256 hash of its first and last fingerprintSpan bytes, of all of it
// when smaller. With the size, it tells the video wherever it is.
func videoFingerprint(file io.ReadSeeker, size int64) ([]byte, error) {
	hash := sha256.New()
	head := min(size, fingerprintSpan)
	if _, err := io.CopyN(hash, file, head); err != nil {
		return nil, err
	}
	rest := size - head
	if rest > fingerprintSpan {
		if _, err := file.Seek(size-fingerprintSpan, io.SeekStart); err != nil {
			return nil, err
		}
		rest = fingerprintSpan
	}
	if _, err := io.CopyN(hash, file, rest); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// readFingerprint reads the fingerprint of a listing's home video at rel,
// of size bytes (see videoFingerprint), in a folder or on a share.
func readFingerprint(ctx context.Context, list listing, rel string, size int64) ([]byte, error) {
	file, err := list.open(ctx, rel)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return nil, errors.New("the file cannot be read from its end")
	}
	return videoFingerprint(seeker, size)
}

// videoFolders are the folders of a home videos folder that are a video's
// own: those holding exactly one video directly, the folder's top left
// out. entries are the folder's videos, by path.
func videoFolders(entries map[string]entry) map[string]bool {
	counts := map[string]int{}
	for rel := range entries {
		counts[path.Dir(rel)]++
	}
	own := map[string]bool{}
	for dir, count := range counts {
		if dir != "." && count == 1 {
			own[dir] = true
		}
	}
	return own
}

// homeName is the name a home video's path gives: its folder's when the
// folder is its own (see videoFolders), else its file's without extension,
// as written.
func homeName(rel string, own map[string]bool) string {
	if dir := path.Dir(rel); own[dir] {
		return path.Base(dir)
	}
	return stem(rel)
}

// Where a home video's date comes from, its dated_by.
const (
	datedByNFO     = "nfo"
	datedByCreated = "created"
	datedByFile    = "file"
)

// homeDate is the date a home video shows, and where it comes from: its
// NFO file's date, else the creation time its container tells, else its
// file's modification time. An NFO file that gives a year alone keeps the
// video's own date when it is of that year, and is the first day of that
// year otherwise.
func homeDate(nfo nfoInfo, created *time.Time, modified time.Time) (time.Time, string) {
	if nfo.released != nil {
		return *nfo.released, datedByNFO
	}
	own, by := modified, datedByFile
	if created != nil {
		own, by = *created, datedByCreated
	}
	if nfo.year > 0 && own.Year() != nfo.year {
		return time.Date(nfo.year, time.January, 1, 0, 0, 0, 0, time.UTC), datedByNFO
	}
	return own, by
}

// homeTitle is what a home video shows: its name, plot and date, and
// where the date comes from (see homeDate).
type homeTitle struct {
	name, plot string
	released   time.Time
	datedBy    string
}

// homeTitleOf is the title of a home video at rel, of nfo, probed as
// probe, its file modified at modified (see homeName and homeDate).
func homeTitleOf(rel string, own map[string]bool, nfo nfoInfo, probe videoProbe, modified time.Time) homeTitle {
	t := homeTitle{name: nfo.title, plot: nfo.plot}
	if t.name == "" {
		t.name = homeName(rel, own)
	}
	t.released, t.datedBy = homeDate(nfo, probe.created, modified)
	return t
}

// videoProbe is what ffprobe tells of a home video: when it was recorded,
// nil when its container does not tell, its length in seconds, 0 when
// unknown, and whether its video is HDR.
type videoProbe struct {
	created  *time.Time
	duration float64
	hdr      bool
}

// creationTags are the tags that tell when a video was recorded, in the
// order they are preferred: the one Apple's devices write, with the
// recording's time zone, then the one every container has.
var creationTags = []string{"com.apple.quicktime.creationdate", "creation_time"}

// creationLayouts are the forms creation times are written in.
var creationLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700", "2006-01-02 15:04:05", "2006-01-02T15:04:05"}

// readVideoProbe reads ffprobe's JSON of a home video: its creation time
// from the container's tags, else from its video stream's, with keys in
// any case; its length; and whether its video's transfer is HDR's.
func readVideoProbe(out []byte) (videoProbe, error) {
	var p probed
	if err := json.Unmarshal(out, &p); err != nil {
		return videoProbe{}, err
	}
	var info videoProbe
	info.duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	tagSets := []map[string]string{p.Format.Tags}
	for _, stream := range p.Streams {
		if stream.CodecType == "video" && stream.Disposition["attached_pic"] == 0 {
			tagSets = append(tagSets, stream.Tags)
			info.hdr = stream.ColorTransfer == "smpte2084" || stream.ColorTransfer == "arib-std-b67"
			break
		}
	}
	for _, tags := range tagSets {
		for _, name := range creationTags {
			for key, value := range tags {
				if !strings.EqualFold(key, name) {
					continue
				}
				if at, ok := parseCreation(value); ok {
					info.created = &at
					return info, nil
				}
			}
		}
	}
	return info, nil
}

// parseCreation reads a creation time tag, false for one not believed (see
// earliestCreation).
func parseCreation(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range creationLayouts {
		if at, err := time.Parse(layout, value); err == nil {
			return at, !at.Before(earliestCreation)
		}
	}
	return time.Time{}, false
}

// probeVideo reads a home video's creation time, length and dynamic range
// with ffprobe.
func (s *Service) probeVideo(ctx context.Context, target string) (videoProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, s.ffprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", target).Output()
	if err != nil {
		return videoProbe{}, err
	}
	return readVideoProbe(out)
}

// storedVideo is a home video as a scan stored it: its identifier, its
// file then, the name, date and plot it shows, what probing it told, and
// its fingerprint, nil until read (see videoFingerprint).
type storedVideo struct {
	id    string
	file  found
	title homeTitle
	probe videoProbe
	print []byte
}

// storedVideos reads the home videos of a folder, by path.
func (s *Service) storedVideos(ctx context.Context, id accounts.ID) (map[string]storedVideo, error) {
	rows, err := s.db.Query(ctx, `SELECT f.path, f.stremio_id, v.size, v.modified, f.name, v.released, v.dated_by, v.plot, v.created, v.duration,
			v.hdr, v.fingerprint
		FROM local_files f JOIN local_videos v ON v.addon_id = f.addon_id AND v.path = f.path WHERE f.addon_id = $1`, id)
	if err != nil {
		return nil, err
	}
	known := map[string]storedVideo{}
	var rel string
	var v storedVideo
	_, err = pgx.ForEachRow(rows, []any{&rel, &v.id, &v.file.size, &v.file.modified, &v.title.name, &v.title.released, &v.title.datedBy,
		&v.title.plot, &v.probe.created, &v.probe.duration, &v.probe.hdr, &v.print}, func() error {
		known[rel] = v
		return nil
	})
	return known, err
}

// scanHomeVideos reads a home videos folder's listing: videos gone are
// forgotten, remembered for goneDays days by their contents, new and
// changed ones fingerprinted and probed, a new one taking the identifier
// of the video gone it is (see recognize), and each stored with the name,
// date and plot it shows now, its NFO file read when new or changed; a
// video gone that another scan found elsewhere meanwhile gives it its
// identifier (see adopt). Then the subtitle files and artwork beside them,
// and the frames of those without a poster. root is the folder's path or
// address.
func (s *Service) scanHomeVideos(ctx context.Context, id accounts.ID, root string, list listing) error {
	files := list.files
	collapseDiscs(KindShows, files, nil)
	collapseDVDs(KindShows, files, nil)
	for rel := range files {
		if imageFile(rel) {
			delete(files, rel)
		}
	}
	known, err := s.storedVideos(ctx, id)
	if err != nil {
		return err
	}
	// When the videos stored were last seen: when the last scan started.
	var seen *time.Time
	if err := s.db.QueryRow(ctx, "SELECT scanned_at FROM local_folders WHERE addon_id = $1", id).Scan(&seen); err != nil {
		return err
	}
	// The files stored no longer listed are gone, but those of folders
	// skipped.
	var gone []string
	rows, err := s.db.Query(ctx, "SELECT path FROM local_files WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	stored, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, rel := range stored {
		if _, ok := files[rel]; !ok && !under(rel, list.skipped) {
			gone = append(gone, rel)
		}
	}
	if len(gone) > 0 {
		// The table's trigger remembers them.
		if _, err := s.db.Exec(ctx, "DELETE FROM local_files WHERE addon_id = $1 AND path = ANY($2)", id, gone); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM local_videos_gone WHERE gone_at <= now() - make_interval(days => $1)", goneDays); err != nil {
		return err
	}
	entries := make(map[string]entry, len(files))
	for rel, f := range files {
		entries[rel] = entry{path: rel, file: f, unit: rel}
	}
	places := unitNFOs(KindHomeVideos, entries, list.sides)
	infos, err := s.readNFOs(ctx, id, list, places)
	if err != nil {
		return err
	}
	own := videoFolders(entries)
	target := probeTarget(func(rel string) string { return filepath.Join(list.root, filepath.FromSlash(rel)) })
	if list.root == "" {
		var stop func()
		if target, stop, err = probeServer(list); err != nil {
			return err
		}
		defer stop()
	}
	// The videos new or changed are fingerprinted, and those found before
	// fingerprints were read.
	var mu sync.Mutex
	prints := map[string][]byte{}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for rel, f := range files {
		if old, ok := known[rel]; ok && old.print != nil && old.file.size == f.size && old.file.modified.Equal(f.modified) {
			continue
		}
		group.Go(func() error {
			print, err := readFingerprint(groupCtx, list, rel, f.size)
			if err != nil {
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				// Read again at the next scan.
				s.logger.Debug("A home video cannot be fingerprinted", "path", rel, "error", err)
				return nil
			}
			mu.Lock()
			prints[rel] = print
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	// How many of the videos new have each contents: those of several are
	// none of the videos gone.
	alike := map[string]int{}
	for rel, print := range prints {
		if _, ok := known[rel]; !ok {
			alike[contents(files[rel].size, print)]++
		}
	}
	group, groupCtx = errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for rel, f := range files {
		old, ok := known[rel]
		probed := ok && old.file.size == f.size && old.file.modified.Equal(f.modified)
		print := prints[rel]
		if probed && old.print != nil {
			print = old.print
		}
		nfo := infos[places[rel]]
		if probed && (old.print != nil || print == nil) {
			if t := homeTitleOf(rel, own, nfo, old.probe, f.modified); t.name == old.title.name && t.plot == old.title.plot &&
				t.datedBy == old.title.datedBy && t.released.Equal(old.title.released) {
				continue
			}
		}
		recognizable := !ok && print != nil && alike[contents(f.size, print)] == 1
		group.Go(func() error {
			probe := old.probe
			if !probed {
				var err error
				if probe, err = s.probeVideo(groupCtx, target(rel)); err != nil {
					if groupCtx.Err() != nil {
						return groupCtx.Err()
					}
					// Dated by its file alone.
					s.logger.Debug("A home video cannot be probed", "path", rel, "error", err)
					probe = videoProbe{}
				}
			}
			return s.storeVideo(groupCtx, id, rel, f, homeTitleOf(rel, own, nfo, probe, f.modified), probe, !probed, print, recognizable)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if seen != nil {
		adopted := false
		for _, rel := range gone {
			if old, ok := known[rel]; ok && old.print != nil {
				ok, err := s.adopt(ctx, old.id, old.file.size, old.print, *seen)
				if err != nil {
					return err
				}
				adopted = adopted || ok
			}
		}
		if adopted {
			s.artworkChanged()
		}
	}
	if err := s.storeSubtitles(ctx, id, list, sidecars(KindHomeVideos, locationName(root), entries, list.sides)); err != nil {
		return err
	}
	if err := s.storeArtwork(ctx, id, KindHomeVideos, root, list); err != nil {
		return err
	}
	return s.storeFrames(ctx, id, target)
}

// contents is the key of a home video's contents: its size and
// fingerprint.
func contents(size int64, print []byte) string {
	return strconv.FormatInt(size, 10) + ":" + string(print)
}

// storeVideo stores a home video: its file, its title, what probing it
// told, and its fingerprint, nil when it could not be read; a file that
// changed, probed again, has its frame made again. A video stored before
// keeps its identifier, and a new one takes a new one, or that of the
// video gone it is when it is recognizable, no other video of its scan
// having its contents (see recognize).
func (s *Service) storeVideo(ctx context.Context, id accounts.ID, rel string, f found, t homeTitle, probe videoProbe, changed bool,
	print []byte, recognizable bool) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		videoID, added, recognized := newVideoID(), (*time.Time)(nil), false
		if recognizable {
			gone, at, ok, err := recognize(ctx, tx, f.size, print)
			if err != nil {
				return err
			}
			if ok {
				videoID, added, recognized = gone, at, true
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO local_files (addon_id, path, size, modified, unit, title, stremio_id, name, title_year, matched_by,
					added_at)
				VALUES ($1, $2, $3, $4, $2, $5, $6, $5, $7, $8, CASE WHEN $9 THEN $10::timestamptz ELSE now() END)
				ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, unit = $2, title = $5, year = NULL, season = NULL,
					episode = NULL, last_episode = NULL, height = NULL,
					stremio_id = CASE WHEN local_files.matched_by = $8 THEN local_files.stremio_id ELSE $6 END,
					imdb_id = '', tmdb_id = '', name = $5, poster = '', title_year = $7, matched_by = $8, reason = '', nfo_id = ''`,
			id, rel, f.size, f.modified, t.name, videoID, t.released.Year(), byOwn, recognized, added); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO local_videos (addon_id, path, size, modified, created, duration, hdr, released, dated_by, plot,
					fingerprint)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $12)
				ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, created = $5, duration = $6, hdr = $7, released = $8,
					dated_by = $9, plot = $10, fingerprint = $12,
					frame = CASE WHEN $11 THEN NULL ELSE local_videos.frame END,
					framed = local_videos.framed AND NOT $11,
					frame_id = CASE WHEN $11 THEN gen_random_uuid() ELSE local_videos.frame_id END`,
			id, rel, f.size, f.modified, probe.created, probe.duration, probe.hdr, t.released, t.datedBy, t.plot, changed, print)
		return err
	})
}

// videosLock serializes, across scans, the transactions that give home
// videos the identifiers of videos gone (see recognize and adopt): a video
// found by one scan while the scan of the folder it left forgets it is
// seen by one of them.
const videosLock = "polyfin.localfiles.videos"

// recognize takes, in tx, the identifier of the video gone a home video
// found is, of size bytes and fingerprint print: the one video gone from
// any home videos folder in the last goneDays days with these contents,
// unless a video still there has them, its original. It returns when that
// video was first found, and false when there is no such video, or several.
func recognize(ctx context.Context, tx pgx.Tx, size int64, print []byte) (id string, added *time.Time, ok bool, err error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", videosLock); err != nil {
		return "", nil, false, err
	}
	rows, err := tx.Query(ctx, `SELECT video_id, added_at FROM local_videos_gone
		WHERE size = $1 AND fingerprint = $2 AND gone_at > now() - make_interval(days => $3)
			AND NOT EXISTS (SELECT 1 FROM local_videos v WHERE v.size = $1 AND v.fingerprint = $2)`, size, print, goneDays)
	if err != nil {
		return "", nil, false, err
	}
	type gone struct {
		id    string
		added *time.Time
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (gone, error) {
		var g gone
		err := row.Scan(&g.id, &g.added)
		return g, err
	})
	if err != nil || len(found) != 1 {
		return "", nil, false, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM local_videos_gone WHERE video_id = $1", found[0].id); err != nil {
		return "", nil, false, err
	}
	return found[0].id, found[0].added, true, nil
}

// adopt gives the identifier of a home video a scan just forgot, of size
// bytes and fingerprint print, last seen by the scan that started at seen,
// to the video another scan found since with the same contents: the video
// moved, found where it went before the scan of the folder it left. That
// video took a new identifier, its original still there; it takes this
// one and the time it was first found, as if recognized when found (see
// recognize), when it is the only video with these contents and this the
// only video gone with them. A video users already did something with
// under its new identifier is left alone (see used). adopt reports whether
// a video took the identifier.
func (s *Service) adopt(ctx context.Context, id string, size int64, print []byte, seen time.Time) (bool, error) {
	adopted := false
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", videosLock); err != nil {
			return err
		}
		var added *time.Time
		err := tx.QueryRow(ctx, "SELECT added_at FROM local_videos_gone WHERE video_id = $1", id).Scan(&added)
		if errors.Is(err, pgx.ErrNoRows) {
			// Recognized meanwhile.
			return nil
		}
		if err != nil {
			return err
		}
		var gone int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM local_videos_gone
				WHERE size = $1 AND fingerprint = $2 AND gone_at > now() - make_interval(days => $3)`, size, print, goneDays).Scan(&gone); err != nil {
			return err
		}
		if gone != 1 {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT f.id, f.stremio_id, f.added_at FROM local_videos v
			JOIN local_files f ON f.addon_id = v.addon_id AND f.path = v.path WHERE v.size = $1 AND v.fingerprint = $2`, size, print)
		if err != nil {
			return err
		}
		type found struct {
			file  accounts.ID
			id    string
			added *time.Time
		}
		live, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (found, error) {
			var f found
			err := row.Scan(&f.file, &f.id, &f.added)
			return f, err
		})
		if err != nil || len(live) != 1 || live[0].added == nil || !live[0].added.After(seen) {
			return err
		}
		if busy, err := used(ctx, tx, library.TitleItemID(library.KindVideo, live[0].id)); err != nil || busy {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE local_files SET stremio_id = $2, added_at = $3 WHERE id = $1", live[0].file, id, added); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM local_videos_gone WHERE video_id = $1", id); err != nil {
			return err
		}
		adopted = true
		return nil
	})
	return adopted, err
}

// used reports whether users did something with a title, by its item: a
// watch state, resume point, favorite or rating, a playback, a collection
// or playlist holding it, a subtitle file uploaded, or an administrator's
// edit of its details or images.
func used(ctx context.Context, tx pgx.Tx, item accounts.ID) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_data WHERE item_id = $1)
		OR EXISTS (SELECT 1 FROM playback_history WHERE item_id = $1)
		OR EXISTS (SELECT 1 FROM collection_items WHERE item_id = $1)
		OR EXISTS (SELECT 1 FROM playlist_entries WHERE item_id = $1)
		OR EXISTS (SELECT 1 FROM uploaded_subtitles WHERE item_id = $1)
		OR EXISTS (SELECT 1 FROM item_overrides WHERE item_id = $1)
		OR EXISTS (SELECT 1 FROM item_images WHERE item_id = $1)`, item).Scan(&found)
	return found, err
}

// storeFrames makes the frames of a folder's home videos that have no
// poster beside them and no frame made for their file as it is: an
// unreadable video is not tried again until it changes.
func (s *Service) storeFrames(ctx context.Context, id accounts.ID, target probeTarget) error {
	rows, err := s.db.Query(ctx, `SELECT path, duration, hdr FROM local_videos v WHERE addon_id = $1 AND NOT framed
		AND NOT EXISTS (SELECT 1 FROM local_artwork a WHERE a.addon_id = v.addon_id AND a.unit = v.path AND a.season IS NULL
			AND a.image_type = $2)`, id, imagePrimary)
	if err != nil {
		return err
	}
	type pending struct {
		rel      string
		duration float64
		hdr      bool
	}
	var wanted []pending
	var p pending
	if _, err := pgx.ForEachRow(rows, []any{&p.rel, &p.duration, &p.hdr}, func() error {
		wanted = append(wanted, p)
		return nil
	}); err != nil {
		return err
	}
	if len(wanted) == 0 {
		return nil
	}
	defer s.artworkChanged()
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for _, p := range wanted {
		group.Go(func() error {
			frame, err := s.homeFrame(groupCtx, target(p.rel), p.duration, p.hdr)
			if err != nil {
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				s.logger.Debug("No frame of a home video could be made", "path", p.rel, "error", err)
			}
			_, err = s.db.Exec(groupCtx, "UPDATE local_videos SET frame = $3, framed = true, frame_id = gen_random_uuid() WHERE addon_id = $1 AND path = $2",
				id, p.rel, frame)
			return err
		})
	}
	return group.Wait()
}

// homeFrame makes the frame a home video shows without a poster: the
// keyframe frameShare into it, else its first, as chapter images are made
// (see thumbnails.Frame), tone mapped when it is HDR and FFmpeg can.
func (s *Service) homeFrame(ctx context.Context, input string, duration float64, hdr bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	at := time.Duration(duration * frameShare * float64(time.Second))
	frame, err := thumbnails.Frame(ctx, s.ffmpeg, input, at, hdr && s.toneMapping)
	if err != nil && at > 0 && ctx.Err() == nil {
		frame, err = thumbnails.Frame(ctx, s.ffmpeg, input, 0, hdr && s.toneMapping)
	}
	if err == nil && len(frame) == 0 {
		err = errors.New("the frame is empty")
	}
	return frame, err
}

// videoMeta describes a home video from what its folder's scan stored: its
// name, plot, date and year, and its length, in minutes, when it is one
// at least.
func videoMeta(id, name, plot string, released time.Time, duration float64) stremio.Meta {
	meta := stremio.Meta{ID: id, Type: library.VideoType, Name: name, Description: plot, Released: released.UTC().Format(time.RFC3339),
		Year: stremio.Text(strconv.Itoa(released.Year()))}
	if minutes := int(math.Round(duration / 60)); minutes > 0 {
		meta.Runtime = stremio.Text(strconv.Itoa(minutes) + " min")
	}
	return meta
}

// homeCatalog answers a home videos folder's catalog: its videos, the
// latest recorded first, those whose name holds search when given, from
// skip.
func (s *Service) homeCatalog(ctx context.Context, source accounts.ID, skip int, search string) ([]stremio.Meta, error) {
	rows, err := s.db.Query(ctx, `SELECT f.stremio_id, f.name, v.plot, v.released, v.duration FROM local_files f
		JOIN local_videos v ON v.addon_id = f.addon_id AND v.path = f.path
		WHERE f.addon_id = $1 AND f.stremio_id IS NOT NULL AND ($2 = '' OR strpos(lower(f.name), lower($2)) > 0)
		ORDER BY v.released DESC, f.stremio_id OFFSET $3 LIMIT $4`, source, strings.TrimSpace(search), max(skip, 0), catalogPage)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Meta, error) {
		var id, name, plot string
		var released time.Time
		var duration float64
		if err := row.Scan(&id, &name, &plot, &released, &duration); err != nil {
			return stremio.Meta{}, err
		}
		return videoMeta(id, name, plot, released, duration), nil
	})
}

// HeldVideos reports which of the home videos of the Stremio identifiers
// ids a file of a home videos folder holds now: the others are gone, from
// disk or as their folder holds something else, and may come back within
// goneDays days (see recognize).
func (s *Service) HeldVideos(ctx context.Context, ids []string) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT f.stremio_id FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
		WHERE l.kind = $1 AND f.stremio_id = ANY($2)`, KindHomeVideos, ids)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	var id string
	_, err = pgx.ForEachRow(rows, []any{&id}, func() error {
		held[id] = true
		return nil
	})
	return held, err
}

// homeMeta describes one of a home videos folder's videos by its
// identifier.
func (s *Service) homeMeta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error) {
	var name, plot string
	var released time.Time
	var duration float64
	err := s.db.QueryRow(ctx, `SELECT f.name, v.plot, v.released, v.duration FROM local_files f
		JOIN local_videos v ON v.addon_id = f.addon_id AND v.path = f.path WHERE f.addon_id = $1 AND f.stremio_id = $2`, source, id).
		Scan(&name, &plot, &released, &duration)
	if errors.Is(err, pgx.ErrNoRows) {
		return stremio.Meta{}, stremio.ErrNotFound
	}
	if err != nil {
		return stremio.Meta{}, err
	}
	return videoMeta(id, name, plot, released, duration), nil
}
