package localfiles

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A matched show lists every episode its files hold, as Jellyfin makes an
// episode of every video file (see library.withFileEpisodes): those its
// metadata addon's description does not list are named after their files,
// and listed on the folder's page next to the description's numbers. An
// episode with no image, neither the addon's still nor one beside its
// file, shows a frame of its video, as Jellyfin's Screen Grabber takes one
// (MediaBrowser.Providers/MediaInfo/VideoImageProvider.cs, lines 57-117 at
// v12.4): a tenth into it, made when an app first asks for it, then kept
// with the other artwork.

// episodeFrames bounds the episodes' frames made at once: a season's page
// asks for all of its episodes' images together.
const episodeFrames = 2

// frameRetry is how long a frame that could not be made is not tried
// again, unless its file changes: an unreadable file is not read again at
// each request for its image.
const frameRetry = 10 * time.Minute

// failedFrame is a file whose frame could not be made: as the file was
// then, and until when it is not tried again.
type failedFrame struct {
	modified time.Time
	until    time.Time
}

// frameExtension ends the key of an episode's frame (see frameURL), which
// Open makes rather than reads.
const frameExtension = "frame"

// Episodes lists the episodes the files of folders, shows folders enabled,
// hold for the series of stremioID, matched to it: one by season and
// number, named after the first file holding it by path, without its
// extension, as Jellyfin names an episode its provider does not know
// (see plainEpisodeName), its video identifier the one Streams answers
// (see episodeVideoID). catalog, unless nil, is the series' episodes as a
// metadata addon of the server lists them: the files named by an
// absolute number are then the catalog's episodes of their places, as
// they are stored for Streams too (see absolute.go).
func (s *Service) Episodes(ctx context.Context, folders []accounts.ID, stremioID string, catalog []stremio.Video) ([]stremio.Video, error) {
	rows, err := s.db.Query(ctx, `SELECT f.id, f.season, f.episode, coalesce(f.last_episode, f.episode), f.path, f.absolute, f.numbering
		FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id JOIN addons a ON a.id = f.addon_id
		WHERE f.addon_id = ANY($1) AND f.stremio_id = $2 AND l.kind = $3 AND a.enabled AND f.season IS NOT NULL AND f.episode IS NOT NULL
		ORDER BY f.season, f.episode, f.path`, folders, stremioID, KindShows)
	if err != nil {
		return nil, err
	}
	type held struct {
		id                  accounts.ID
		season, first, last int
		rel                 string
		absolute            *int
		numbering           string
	}
	files, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (held, error) {
		var h held
		err := row.Scan(&h.id, &h.season, &h.first, &h.last, &h.rel, &h.absolute, &h.numbering)
		return h, err
	})
	if err != nil {
		return nil, err
	}
	if catalog != nil {
		places := library.AbsoluteEpisodes(catalog)
		batch := &pgx.Batch{}
		for i, h := range files {
			if h.absolute == nil {
				continue
			}
			_, named, _ := episodeName(h.rel)
			season, number, numbering := absolutePlace(places, *h.absolute, named.season)
			if season == h.season && number == h.first && numbering == h.numbering {
				continue
			}
			files[i].season, files[i].first, files[i].last, files[i].numbering = season, number, number, numbering
			batch.Queue("UPDATE local_files SET season = $2, episode = $3, last_episode = $3, numbering = $4 WHERE id = $1", h.id, season, number,
				numbering)
		}
		if batch.Len() > 0 {
			if err := s.db.SendBatch(ctx, batch).Close(); err != nil {
				return nil, err
			}
			// The episodes' frames and images follow their files.
			s.artworkChanged()
			slices.SortStableFunc(files, func(a, b held) int {
				return cmp.Or(cmp.Compare(a.season, b.season), cmp.Compare(a.first, b.first), strings.Compare(a.rel, b.rel))
			})
		}
	}
	var videos []stremio.Video
	seen := map[[2]int]bool{}
	for _, h := range files {
		for number := h.first; number <= h.last; number++ {
			if seen[[2]int{h.season, number}] {
				continue
			}
			seen[[2]int{h.season, number}] = true
			videos = append(videos, stremio.Video{ID: episodeVideoID(stremioID, h.season, number), Title: plainEpisodeName(h.rel, number),
				Season: stremio.Number(h.season), Episode: stremio.Number(number)})
		}
	}
	return videos, nil
}

// NoteUnlisted records the episodes, by season and number, of the series
// of stremioID that the files of local folders hold and that its metadata
// addon's description does not list, and those the description lists, in
// place of those recorded before: the folders' pages count the first and
// list them next to the second (see Unlisted). The description's are kept
// only while the files hold episodes it does not list. It writes only
// when they changed since it last did.
func (s *Service) NoteUnlisted(ctx context.Context, stremioID string, unlisted, listed [][2]int) error {
	if len(unlisted) == 0 {
		listed = nil
	}
	print := numbersPrint(unlisted, listed)
	s.unlistedMu.Lock()
	last, known := s.unlisted[stremioID]
	s.unlistedMu.Unlock()
	if known && last == print {
		return nil
	}
	seasons, numbers := splitNumbers(unlisted)
	listedSeasons, listedNumbers := splitNumbers(listed)
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM local_unlisted_episodes WHERE stremio_id = $1", stremioID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO local_unlisted_episodes (stremio_id, season, episode)
			SELECT $1, s, e FROM unnest($2::int[], $3::int[]) AS u(s, e) ORDER BY s, e ON CONFLICT DO NOTHING`, stremioID, seasons, numbers)
		if err != nil {
			return err
		}
		if len(unlisted) == 0 {
			_, err = tx.Exec(ctx, "DELETE FROM local_unlisted_series WHERE stremio_id = $1", stremioID)
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO local_unlisted_series (stremio_id, seasons, episodes) VALUES ($1, $2, $3)
			ON CONFLICT (stremio_id) DO UPDATE SET seasons = $2, episodes = $3`, stremioID, listedSeasons, listedNumbers)
		return err
	})
	if err != nil {
		return err
	}
	s.unlistedMu.Lock()
	s.unlisted[stremioID] = print
	s.unlistedMu.Unlock()
	return nil
}

// numbersPrint is a hash of lists of seasons and numbers, which tells
// whether they changed.
func numbersPrint(lists ...[][2]int) uint64 {
	h := fnv.New64a()
	var b [8]byte
	put := func(n int) {
		binary.LittleEndian.PutUint64(b[:], uint64(n))
		_, _ = h.Write(b[:])
	}
	for _, list := range lists {
		put(len(list))
		for _, number := range list {
			put(number[0])
			put(number[1])
		}
	}
	return h.Sum64()
}

// splitNumbers is the seasons and the numbers of a list of seasons and
// numbers, apart, as unnest and arrays take them.
func splitNumbers(list [][2]int) (seasons, numbers []int) {
	seasons, numbers = make([]int, len(list)), make([]int, len(list))
	for i, number := range list {
		seasons[i], numbers[i] = number[0], number[1]
	}
	return seasons, numbers
}

// unlistedCount counts the episodes the files of a folder hold that their
// series' metadata addon does not list, as NoteUnlisted recorded them.
const unlistedCount = `(SELECT count(*) FROM (SELECT DISTINCT u.stremio_id, u.season, u.episode FROM local_unlisted_episodes u
	JOIN local_files f ON f.stremio_id = u.stremio_id AND f.season = u.season AND u.episode BETWEEN f.episode AND coalesce(f.last_episode, f.episode)
	WHERE f.addon_id = $1) unlisted)`

// UnlistedShow is a show whose files, in a folder, hold episodes its
// series' metadata addon does not list, as the series was last described
// (see NoteUnlisted): its Stremio identifier, the name it was matched to,
// the first of its files' units by path, and those episodes' seasons, in
// order. Described tells whether the description's own numbers are
// known: they are not for a series recorded by an older Polyfin until it
// is described again. First and Last are the description's first and
// last episodes, by season and number, specials left out unless it lists
// nothing else; nil when it lists none.
type UnlistedShow struct {
	StremioID   string
	Name, Unit  string
	Seasons     []UnlistedSeason
	Described   bool
	First, Last *[2]int
}

// UnlistedSeason is a season of an UnlistedShow: the numbers its files
// hold that the description does not list, and those the description
// lists in the season, none when it has no such season.
type UnlistedSeason struct {
	Season        int
	Files, Listed []Range
}

// Range is the episodes from First to Last of a season.
type Range struct{ First, Last int }

// extend is ranges, in order, with number, which follows them.
func extend(ranges []Range, number int) []Range {
	if n := len(ranges); n > 0 && ranges[n-1].Last+1 >= number {
		ranges[n-1].Last = max(ranges[n-1].Last, number)
		return ranges
	}
	return append(ranges, Range{First: number, Last: number})
}

// Unlisted lists the shows of a folder whose files hold episodes their
// series' metadata addon does not list, by name, as NoteUnlisted recorded
// them: those Folder counts (see Folder.Unlisted).
func (s *Service) Unlisted(ctx context.Context, id accounts.ID) ([]UnlistedShow, error) {
	if _, err := s.Folder(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT u.stremio_id, min(f.name), min(f.unit), u.season, u.episode FROM local_unlisted_episodes u
		JOIN local_files f ON f.stremio_id = u.stremio_id AND f.season = u.season AND u.episode BETWEEN f.episode AND coalesce(f.last_episode, f.episode)
		WHERE f.addon_id = $1 GROUP BY u.stremio_id, u.season, u.episode ORDER BY 1, 4, 5`, id)
	if err != nil {
		return nil, err
	}
	var shows []UnlistedShow
	index := map[string]int{}
	var stremioID, name, unit string
	var season, number int
	_, err = pgx.ForEachRow(rows, []any{&stremioID, &name, &unit, &season, &number}, func() error {
		i, ok := index[stremioID]
		if !ok {
			i = len(shows)
			index[stremioID] = i
			shows = append(shows, UnlistedShow{StremioID: stremioID, Name: name, Unit: unit})
		}
		show := &shows[i]
		show.Unit = min(show.Unit, unit)
		n := len(show.Seasons)
		if n == 0 || show.Seasons[n-1].Season != season {
			show.Seasons = append(show.Seasons, UnlistedSeason{Season: season})
			n++
		}
		show.Seasons[n-1].Files = extend(show.Seasons[n-1].Files, number)
		return nil
	})
	if err != nil || len(shows) == 0 {
		return shows, err
	}
	ids := make([]string, len(shows))
	for i, show := range shows {
		ids[i] = show.StremioID
	}
	rows, err = s.db.Query(ctx, "SELECT stremio_id, seasons, episodes FROM local_unlisted_series WHERE stremio_id = ANY($1)", ids)
	if err != nil {
		return nil, err
	}
	var seasons, numbers []int
	_, err = pgx.ForEachRow(rows, []any{&stremioID, &seasons, &numbers}, func() error {
		show := &shows[index[stremioID]]
		show.Described = true
		listed := map[int][]Range{}
		for i, season := range seasons {
			listed[season] = extend(listed[season], numbers[i])
			number := [2]int{season, numbers[i]}
			if show.First == nil || show.First[0] == 0 && season > 0 {
				show.First = &number
			}
			if season > 0 || show.Last == nil || show.Last[0] == 0 {
				show.Last = &number
			}
		}
		for i := range show.Seasons {
			show.Seasons[i].Listed = listed[show.Seasons[i].Season]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(shows, func(a, b UnlistedShow) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.StremioID, b.StremioID))
	})
	return shows, nil
}

// frameURL stands for the frame of an episode's file in the library's
// Images (see library.Images.Frame): the file's key, which Open makes the
// frame of, and its version, which tells its size and modification time.
func frameURL(file accounts.ID, size int64, modified time.Time) string {
	return artworkURL(file, "."+frameExtension, size, modified)
}

// readFrames sets, in images, the frame of each episode the files of the
// enabled shows folders hold, matched or plain, but discs' and images'
// titles, which Jellyfin makes no frame of either: of the files holding an
// episode, the first of the folder listed first among the sources, then
// by path.
func (s *Service) readFrames(ctx context.Context, images map[accounts.ID]library.Images) error {
	rows, err := s.db.Query(ctx, `SELECT f.id, coalesce(f.stremio_id, f.plain_id), f.season, f.episode, coalesce(f.last_episode, f.episode),
			f.path, f.size, f.modified
		FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id JOIN addons d ON d.id = f.addon_id
		WHERE l.kind = 'shows' AND d.enabled AND f.season IS NOT NULL AND f.episode IS NOT NULL AND (f.stremio_id IS NOT NULL OR `+plainListed+`)
		ORDER BY d.position, d.created_at, d.id, f.path`)
	if err != nil {
		return err
	}
	var file accounts.ID
	var title, rel string
	var season, first, last int
	var size int64
	var modified time.Time
	_, err = pgx.ForEachRow(rows, []any{&file, &title, &season, &first, &last, &rel, &size, &modified}, func() error {
		if !framed(rel) {
			return nil
		}
		url := frameURL(file, size, modified)
		for number := first; number <= last; number++ {
			item := library.EpisodeItemID(episodeVideoID(title, season, number))
			set := images[item]
			if set.Frame == "" {
				set.Frame = url
				images[item] = set
			}
		}
		return nil
	})
	return err
}

// framed reports whether a frame is made of the video at rel: one file, not
// a disc's title nor a disc image.
func framed(rel string) bool {
	if _, _, titled := titleOf(rel); titled {
		return false
	}
	if _, _, ok := discEntry(rel); ok {
		return false
	}
	return !imageFile(rel)
}

// episodeFrame makes the frame of the file of an episode, a tenth into it
// as Jellyfin's Screen Grabber takes one, else its first (see homeFrame),
// with its modification time: from its folder, or a share's through the
// loopback interface (see UseFiles). A file gone, or whose frame cannot be
// made, answers ErrGone; a file whose frame could not be made answers so
// without trying again for frameRetry, unless it changed.
func (s *Service) episodeFrame(ctx context.Context, id accounts.ID) (io.ReadSeekCloser, time.Time, error) {
	var root, rel string
	var modified time.Time
	err := s.db.QueryRow(ctx, `SELECT a.manifest_url, f.path, f.modified FROM local_files f JOIN addons a ON a.id = f.addon_id
		JOIN local_folders l ON l.addon_id = f.addon_id WHERE f.id = $1 AND l.kind = $2`, id, KindShows).Scan(&root, &rel, &modified)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !framed(rel) {
		return nil, time.Time{}, ErrGone
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var input string
	if shareKind(root) == "" {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, time.Time{}, ErrGone
		}
		target, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
		if err != nil || !within(real, target) {
			return nil, time.Time{}, ErrGone
		}
		if info, err := os.Stat(target); err != nil || !info.Mode().IsRegular() {
			return nil, time.Time{}, ErrGone
		}
		input = target
	} else {
		if s.files == "" {
			return nil, time.Time{}, ErrGone
		}
		input = s.address(id, rel)
	}
	s.framesMu.Lock()
	failed, known := s.failedFrames[id]
	s.framesMu.Unlock()
	if known && failed.modified.Equal(modified) && s.now().Before(failed.until) {
		return nil, time.Time{}, ErrGone
	}
	select {
	case s.frames <- struct{}{}:
		defer func() { <-s.frames }()
	case <-ctx.Done():
		return nil, time.Time{}, ctx.Err()
	}
	// Probing and the frame are bounded each (see probeTimeout): callers
	// wait for the frame whole, or its failure.
	probe, err := s.probeVideo(ctx, input)
	if err != nil {
		s.logger.Debug("An episode's file could not be probed for its frame", "path", rel, "error", err)
	}
	frame, err := s.homeFrame(ctx, input, probe.duration, probe.hdr)
	s.framesMu.Lock()
	defer s.framesMu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			// The caller is gone: nothing tells the file cannot be read.
			return nil, time.Time{}, ctx.Err()
		}
		s.failedFrames[id] = failedFrame{modified: modified, until: s.now().Add(frameRetry)}
		s.logger.Warn("No frame of an episode's file could be made", "path", rel, "error", err)
		return nil, time.Time{}, ErrGone
	}
	delete(s.failedFrames, id)
	return readSeekNopCloser{bytes.NewReader(frame)}, modified, nil
}

// frameKey reports whether a key Open is given names an episode's frame
// (see frameURL), and its file.
func frameKey(key string) (accounts.ID, bool) {
	raw, ok := strings.CutSuffix(key, "."+frameExtension)
	if !ok {
		return accounts.ID{}, false
	}
	id, err := accounts.ParseID(raw)
	return id, err == nil
}
