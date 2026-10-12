package localfiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
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
// many titles, named after their files. A title's identifier is its
// folder's and its path's (see videoID), the same through every scan and
// restart: a file renamed or moved is another title, and what users did
// with the former one stays with it.
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

// videoID is the Stremio identifier of a home video, from its folder and
// its path in it.
func videoID(folder accounts.ID, rel string) string {
	sum := sha256.Sum256([]byte(folder.String() + "\x00" + rel))
	return library.VideoIDPrefix + hex.EncodeToString(sum[:16])
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

// storedVideo is a home video as a scan stored it: its file then, the
// name, date and plot it shows, and what probing it told.
type storedVideo struct {
	file  found
	title homeTitle
	probe videoProbe
}

// storedVideos reads the home videos of a folder, by path.
func (s *Service) storedVideos(ctx context.Context, id accounts.ID) (map[string]storedVideo, error) {
	rows, err := s.db.Query(ctx, `SELECT f.path, v.size, v.modified, f.name, v.released, v.dated_by, v.plot, v.created, v.duration, v.hdr
		FROM local_files f JOIN local_videos v ON v.addon_id = f.addon_id AND v.path = f.path WHERE f.addon_id = $1`, id)
	if err != nil {
		return nil, err
	}
	known := map[string]storedVideo{}
	var rel string
	var v storedVideo
	_, err = pgx.ForEachRow(rows, []any{&rel, &v.file.size, &v.file.modified, &v.title.name, &v.title.released, &v.title.datedBy, &v.title.plot,
		&v.probe.created, &v.probe.duration, &v.probe.hdr}, func() error {
		known[rel] = v
		return nil
	})
	return known, err
}

// scanHomeVideos reads a home videos folder's listing: videos gone are
// forgotten, new and changed ones probed, and each stored with the name,
// date and plot it shows now, its NFO file read when new or changed; then
// the subtitle files and artwork beside them, and the frames of those
// without a poster. root is the folder's path or address.
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
		if _, err := s.db.Exec(ctx, "DELETE FROM local_files WHERE addon_id = $1 AND path = ANY($2)", id, gone); err != nil {
			return err
		}
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
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for rel, f := range files {
		old, ok := known[rel]
		probed := ok && old.file.size == f.size && old.file.modified.Equal(f.modified)
		nfo := infos[places[rel]]
		if probed {
			if t := homeTitleOf(rel, own, nfo, old.probe, f.modified); t.name == old.title.name && t.plot == old.title.plot &&
				t.datedBy == old.title.datedBy && t.released.Equal(old.title.released) {
				continue
			}
		}
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
			return s.storeVideo(groupCtx, id, rel, f, homeTitleOf(rel, own, nfo, probe, f.modified), probe, !probed)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if err := s.storeSubtitles(ctx, id, list, sidecars(KindHomeVideos, locationName(root), entries, list.sides)); err != nil {
		return err
	}
	if err := s.storeArtwork(ctx, id, KindHomeVideos, root, list); err != nil {
		return err
	}
	return s.storeFrames(ctx, id, target)
}

// storeVideo stores a home video: its file, its title, and what probing
// it told; a file that changed, probed again, has its frame made again.
func (s *Service) storeVideo(ctx context.Context, id accounts.ID, rel string, f found, t homeTitle, probe videoProbe, changed bool) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO local_files (addon_id, path, size, modified, unit, title, stremio_id, name, title_year, matched_by)
				VALUES ($1, $2, $3, $4, $2, $5, $6, $5, $7, $8)
				ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, unit = $2, title = $5, year = NULL, season = NULL,
					episode = NULL, last_episode = NULL, height = NULL, stremio_id = $6, imdb_id = '', tmdb_id = '', name = $5, poster = '',
					title_year = $7, matched_by = $8, reason = '', nfo_id = ''`,
			id, rel, f.size, f.modified, t.name, videoID(id, rel), t.released.Year(), byOwn); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO local_videos (addon_id, path, size, modified, created, duration, hdr, released, dated_by, plot)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, created = $5, duration = $6, hdr = $7, released = $8,
					dated_by = $9, plot = $10,
					frame = CASE WHEN $11 THEN NULL ELSE local_videos.frame END,
					framed = local_videos.framed AND NOT $11,
					frame_id = CASE WHEN $11 THEN gen_random_uuid() ELSE local_videos.frame_id END`,
			id, rel, f.size, f.modified, probe.created, probe.duration, probe.hdr, t.released, t.datedBy, t.plot, changed)
		return err
	})
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
