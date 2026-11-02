package localfiles

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Show numbering.
//
// A show's files become versions of the catalog's episodes of the season
// and number they tell (see library.withFileEpisodes). An administrator
// may say, per show of a folder, that its files are numbered otherwise:
//
//   - per season (NumberedPerSeason): each file's number counts within its
//     season, the catalog's season listing its episodes in order, as when
//     the catalog numbers on from season 1 (S02E01 is the first episode
//     the catalog lists in season 2, its S02E13 say);
//   - continuing from season 1 (NumberedContinuing): the numbers run on
//     across seasons, each season's starting after the episodes the
//     catalog lists in the seasons before it (S02E13, after 12 episodes in
//     season 1, is the catalog's S02E01);
//   - absolute (NumberedAbsolute): each file's number is its place among
//     all the regular episodes the catalog lists, as for the names telling
//     an absolute number alone (see absolute.go); a season whose numbers
//     start again at or below the previous season's last is counted on
//     from it, so that files split into seasons otherwise than the catalog
//     are read in their order;
//   - in an episode order (NumberedInOrder): the files' seasons and numbers
//     are those of one of the series' TMDB episode groups, mapped to the
//     catalog's episodes through the group (see library.FileOrder).
//
// A file is then a version of the catalog's episode it maps to
// (numberingMapped). One the mapping cannot place is an episode of its
// own (numberingUnmapped), with the reason the folder's page shows, at
// its own season and number, else after the catalog's episodes of its
// season when the catalog lists that number: it is never a version of an
// episode it is not. Specials, and the episodes an administrator gave the
// titles of a show's disc, keep their numbers. Without a choice the files
// keep their own numbers: a mapping is never guessed.
//
// Jellyfin's episodes are its files, at the season and number they tell,
// and their metadata is read at that place in the series' display order
// (MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs, lines
// 276-297 at v12.4). Polyfin's episodes are the catalog's, so the files
// are mapped to them instead, and the series keeps showing in its own
// display order (see library.newLayout).
//
// When the places of a show's files change, as when the choice or the
// catalog changes, what users did with the episodes the files were
// follows them to their new episodes (see userdata.Store.Follow).

// How a show's files are numbered, an administrator's choice.
const (
	NumberedAsCatalog  = ""
	NumberedPerSeason  = "per_season"
	NumberedContinuing = "continuing"
	NumberedAbsolute   = "absolute"
	NumberedInOrder    = "order"
)

// Why a show's numbering could not place a file, its unmapped reason.
const (
	// unmappedNoCatalog: no metadata addon of the server describes the
	// show.
	unmappedNoCatalog = "no_catalog"
	// unmappedOutsideSeason: the catalog's season lists fewer episodes
	// than the file's number reaches, or it has no such season.
	unmappedOutsideSeason = "outside_season"
	// unmappedBeyondCatalog: the catalog lists fewer regular episodes than
	// the file's absolute number.
	unmappedBeyondCatalog = "beyond_catalog"
	// unmappedNotInOrder: the episode order has no episode at the file's
	// season and number.
	unmappedNotInOrder = "not_in_order"
	// unmappedNotListed: the episode order's episode at the file's season
	// and number is not one the catalog lists.
	unmappedNotListed = "not_listed"
	// unmappedNoOrder: TMDB cannot tell the episode order, without a TMDB
	// key or while it does not answer.
	unmappedNoOrder = "no_order"
)

var (
	ErrInvalidNumbering = errors.New("a show's files are numbered as the catalog, per season, continuing, absolute or in an episode order")
	// ErrNoGroups wraps why the episode groups of a show could not be
	// read from TMDB.
	ErrNoGroups = errors.New("TMDB could not tell the show's episode groups")
)

// ShowNumbering is how a show's files are numbered: one of the Numbered
// values, and the TMDB episode group of NumberedInOrder.
type ShowNumbering struct {
	Numbering string
	Group     string
}

// told is what a show's file tells of its episodes: its season and its
// first and last numbers, its absolute number when its name tells one
// alone (its season then its season folder's, else the first), whether
// an administrator gave them (a disc's title), and its path.
type told struct {
	season, first, last int
	absolute            int
	given               bool
	path                string
}

// placed is where a show's file is listed: the season and numbers of the
// episodes it is a version of, how it was numbered (its numbering), and
// why its show's numbering could not place it.
type placed struct {
	season, first, last int
	numbering, unmapped string
}

// numberer places the files of a show of a folder.
type numberer struct {
	choice string
	// known tells whether the catalog is known, and orderKnown whether the
	// episode order is, for NumberedInOrder.
	known, orderKnown bool
	// regular are the catalog's regular episodes in their absolute
	// numbering; seasons its numbers in each season, in order; before the
	// regular episodes it lists in the seasons before each; listed its
	// episodes.
	regular [][2]int
	seasons map[int][]int
	before  map[int]int
	listed  map[[2]int]bool
	// order is the episode order's episodes, by their season and number in
	// it (see library.FileOrder).
	order map[[2]int]library.OrderPlace
}

// newNumberer places the files of a show numbered as choice, catalog being
// the series' episodes as a metadata addon of the server lists them, nil
// when none describes it, and order the episode order's, orderKnown
// telling whether TMDB told it.
func newNumberer(choice string, catalog *stremio.Meta, order map[[2]int]library.OrderPlace, orderKnown bool) numberer {
	n := numberer{choice: choice, known: catalog != nil, order: order, orderKnown: orderKnown}
	if catalog == nil {
		return n
	}
	n.regular = library.AbsoluteEpisodes(catalog.Videos)
	n.seasons, n.before, n.listed = map[int][]int{}, map[int]int{}, map[[2]int]bool{}
	for _, number := range library.ListedNumbers(catalog.Videos) {
		n.listed[number] = true
		n.seasons[number[0]] = append(n.seasons[number[0]], number[1])
	}
	count := 0
	for _, place := range n.regular {
		if _, ok := n.before[place[0]]; !ok {
			n.before[place[0]] = count
		}
		count++
	}
	return n
}

// places gives each of a show's files, in a folder, its place.
func (n numberer) places(files []told) []placed {
	result := make([]placed, len(files))
	if n.choice == NumberedAsCatalog {
		for i, f := range files {
			if f.absolute > 0 {
				season, episode, how := absolutePlace(n.regular, f.absolute, f.season)
				result[i] = placed{season: season, first: episode, last: episode, numbering: how}
				continue
			}
			result[i] = placed{season: f.season, first: f.first, last: f.last}
		}
		return result
	}
	offsets := n.offsets(files)
	for i, f := range files {
		switch {
		case f.given || f.season == 0 && n.choice != NumberedInOrder:
			result[i] = placed{season: f.season, first: f.first, last: f.last}
		case !n.known:
			result[i] = unmappedAt(f, unmappedNoCatalog)
		case n.choice == NumberedInOrder && !n.orderKnown:
			result[i] = unmappedAt(f, unmappedNoOrder)
		default:
			result[i] = n.mapFile(f, offsets[f.season])
		}
	}
	n.settle(files, result)
	return result
}

// unmappedAt is a file left at its own place, for reason.
func unmappedAt(f told, reason string) placed {
	return placed{season: f.season, first: f.first, last: f.last, numbering: numberingUnmapped, unmapped: reason}
}

// mapFile places a file through the show's numbering, offset being the
// absolute number its season's numbers count on from (see offsets). A
// file holding several episodes holds those its numbers map to in one
// run of the catalog's season, from its first.
func (n numberer) mapFile(f told, offset int) placed {
	season, first, reason := n.mapNumber(f, f.first, offset)
	if reason != "" {
		return unmappedAt(f, reason)
	}
	last := first
	for number := f.first + 1; number <= f.last; number++ {
		s, e, why := n.mapNumber(f, number, offset)
		if why != "" || s != season || e != last+1 {
			break
		}
		last = e
	}
	return placed{season: season, first: first, last: last, numbering: numberingMapped}
}

// mapNumber is the catalog's season and number of the file's episode
// number, else why there is none.
func (n numberer) mapNumber(f told, number, offset int) (season, episode int, reason string) {
	inSeason := func(place int) (int, int, string) {
		numbers := n.seasons[f.season]
		if place < 1 || place > len(numbers) {
			return 0, 0, unmappedOutsideSeason
		}
		return f.season, numbers[place-1], ""
	}
	switch n.choice {
	case NumberedPerSeason:
		return inSeason(number)
	case NumberedContinuing:
		return inSeason(number - n.before[f.season])
	case NumberedAbsolute:
		absolute := offset + number
		if f.absolute > 0 {
			absolute = f.absolute
		}
		if absolute < 1 || absolute > len(n.regular) {
			return 0, 0, unmappedBeyondCatalog
		}
		return n.regular[absolute-1][0], n.regular[absolute-1][1], ""
	case NumberedInOrder:
		place, ok := n.order[[2]int{f.season, number}]
		switch {
		case !ok:
			return 0, 0, unmappedNotInOrder
		case !place.Listed:
			return 0, 0, unmappedNotListed
		}
		return place.Season, place.Episode, ""
	}
	return f.season, number, ""
}

// offsets are, for NumberedAbsolute, the absolute number each season's
// numbers count on from: none while a season's first number comes after
// the last the files hold in the seasons before it, the files numbering
// on; else that last, the files starting each season again.
func (n numberer) offsets(files []told) map[int]int {
	offsets := map[int]int{}
	if n.choice != NumberedAbsolute {
		return offsets
	}
	low, high := map[int]int{}, map[int]int{}
	var seasons []int
	for _, f := range files {
		if f.absolute > 0 || f.given || f.season < 1 {
			continue
		}
		if first, ok := low[f.season]; !ok {
			seasons = append(seasons, f.season)
			low[f.season] = f.first
		} else {
			low[f.season] = min(first, f.first)
		}
		high[f.season] = max(high[f.season], f.last)
	}
	slices.Sort(seasons)
	top := 0
	for _, season := range seasons {
		if low[season] <= top {
			offsets[season] = top
		}
		top = offsets[season] + high[season]
	}
	return offsets
}

// settle moves the files left unmapped whose own numbers are episodes the
// catalog lists after the catalog's and the other files' last episode of
// their season, in order: they are episodes of their own, never versions
// of an episode they are not.
func (n numberer) settle(files []told, result []placed) {
	top := map[int]int{}
	for number := range n.listed {
		top[number[0]] = max(top[number[0]], number[1])
	}
	var moving []int
	for i, p := range result {
		if p.numbering == numberingUnmapped && n.listedAny(p) {
			moving = append(moving, i)
			continue
		}
		top[p.season] = max(top[p.season], p.last)
	}
	slices.SortFunc(moving, func(a, b int) int {
		return cmp.Or(cmp.Compare(files[a].season, files[b].season), cmp.Compare(files[a].first, files[b].first),
			strings.Compare(files[a].path, files[b].path))
	})
	for _, i := range moving {
		p := &result[i]
		span := p.last - p.first
		p.first = top[p.season] + 1
		p.last = p.first + span
		top[p.season] = p.last
	}
}

// listedAny reports whether the catalog lists one of the episodes of p.
func (n numberer) listedAny(p placed) bool {
	for number := p.first; number <= p.last; number++ {
		if n.listed[[2]int{p.season, number}] {
			return true
		}
	}
	return false
}

// numberingOf reads how the files of the show of stremioID, in a folder,
// are numbered.
func (s *Service) numberingOf(ctx context.Context, folder accounts.ID, stremioID string) (ShowNumbering, error) {
	var n ShowNumbering
	err := s.db.QueryRow(ctx, "SELECT numbering, episode_group FROM local_show_numbering WHERE addon_id = $1 AND stremio_id = $2",
		folder, stremioID).Scan(&n.Numbering, &n.Group)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShowNumbering{}, nil
	}
	return n, err
}

// numbererFor places the files of a show numbered as choice, catalog
// describing its series, nil when no metadata addon of the server does;
// the episode order of NumberedInOrder is asked of TMDB.
func (s *Service) numbererFor(ctx context.Context, choice ShowNumbering, catalog *stremio.Meta) numberer {
	var order map[[2]int]library.OrderPlace
	known := false
	if choice.Numbering == NumberedInOrder && catalog != nil {
		var err error
		order, err = s.titles.FileOrder(ctx, *catalog, choice.Group)
		if err != nil && !errors.Is(err, tmdb.ErrNoKey) {
			s.logger.Warn("The episode order a show's files are numbered in could not be read from TMDB", "title", catalog.ID, "error", err)
		}
		known = err == nil
	}
	return newNumberer(choice.Numbering, catalog, order, known)
}

// showFile is a show's file, as Episodes and renumber read it: what it
// tells, and where it is listed.
type showFile struct {
	id        accounts.ID
	folder    accounts.ID
	stremioID string
	told      told
	place     placed
	choice    ShowNumbering
}

// showFileColumns are the columns scanShowFile reads, of alias f, n being
// local_show_numbering.
const showFileColumns = `f.id, f.addon_id, f.stremio_id, f.path, f.season, f.episode, coalesce(f.last_episode, f.episode), f.file_season,
	f.file_episode, f.file_last_episode, f.absolute, f.numbering, f.unmapped, coalesce(n.numbering, ''), coalesce(n.episode_group, '')`

// scanShowFile reads a row of showFileColumns.
func scanShowFile(row pgx.CollectableRow) (showFile, error) {
	var f showFile
	var rel string
	var fileSeason, fileEpisode, fileLast, absolute *int
	err := row.Scan(&f.id, &f.folder, &f.stremioID, &rel, &f.place.season, &f.place.first, &f.place.last, &fileSeason, &fileEpisode, &fileLast,
		&absolute, &f.place.numbering, &f.place.unmapped, &f.choice.Numbering, &f.choice.Group)
	if err != nil {
		return f, err
	}
	t := told{path: rel}
	_, _, t.given = showTitle(rel)
	switch {
	case fileSeason != nil && fileEpisode != nil:
		t.season, t.first, t.last = *fileSeason, *fileEpisode, *fileEpisode
		if fileLast != nil {
			t.last = max(*fileLast, t.first)
		}
	case absolute != nil:
		// Stored before the files' own numbers were: its name tells its
		// season.
		_, named, _ := episodeName(rel)
		t.season, t.first, t.last = named.season, *absolute, *absolute
	default:
		t.season, t.first, t.last = f.place.season, f.place.first, f.place.last
	}
	if absolute != nil {
		t.absolute = *absolute
	}
	f.told = t
	return f, nil
}

// placeFiles places the files of a show of a folder with n, writing the
// places that changed. What users did with the episodes a file was
// listed as follows it to those it is now (see userdata.Store.Follow).
// A place is written only if no one changed it meanwhile, so that two
// listings at once move what users did once.
func (s *Service) placeFiles(ctx context.Context, files []showFile, n numberer) error {
	tolds := make([]told, len(files))
	for i, f := range files {
		tolds[i] = f.told
	}
	places := n.places(tolds)
	batch := &pgx.Batch{}
	var changed []int
	for i, f := range files {
		p := places[i]
		if p == f.place {
			continue
		}
		changed = append(changed, i)
		batch.Queue(`UPDATE local_files SET season = $2, episode = $3, last_episode = $4, numbering = $5, unmapped = $6
			WHERE id = $1 AND season = $7 AND episode = $8 AND coalesce(last_episode, episode) = $9 AND numbering = $10 AND unmapped = $11`,
			f.id, p.season, p.first, p.last, p.numbering, p.unmapped, f.place.season, f.place.first, f.place.last, f.place.numbering,
			f.place.unmapped)
	}
	if len(changed) == 0 {
		return nil
	}
	results := s.db.SendBatch(ctx, batch)
	var moves []userdata.Move
	for _, i := range changed {
		tag, err := results.Exec()
		if err != nil {
			_ = results.Close()
			return err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		f := &files[i]
		from := titleItems(KindShows, f.stremioID, &f.place.season, &f.place.first, &f.place.last)
		p := places[i]
		move := userdata.Move{To: titleItems(KindShows, f.stremioID, &p.season, &p.first, &p.last)}
		for _, item := range from {
			move.From = append(move.From, item.ID)
		}
		moves = append(moves, move)
		f.place = p
	}
	if err := results.Close(); err != nil {
		return err
	}
	if len(moves) == 0 {
		return nil
	}
	// The episodes' frames and images follow their files.
	s.artworkChanged()
	settings := s.settings()
	return userdata.New(s.db).Follow(ctx, moves, userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent})
}

// showFiles reads the files of the show of stremioID that a folder holds,
// with their episodes.
func (s *Service) showFiles(ctx context.Context, folder accounts.ID, stremioID string) ([]showFile, error) {
	rows, err := s.db.Query(ctx, `SELECT `+showFileColumns+` FROM local_files f
		LEFT JOIN local_show_numbering n ON n.addon_id = f.addon_id AND n.stremio_id = f.stremio_id
		WHERE f.addon_id = $1 AND f.stremio_id = $2 AND f.season IS NOT NULL AND f.episode IS NOT NULL ORDER BY f.path`, folder, stremioID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanShowFile)
}

// renumber places the files of the show of stremioID that a folder holds
// again, catalog describing its series, nil when no metadata addon of
// the server does. A show numbered in an episode order TMDB cannot tell
// keeps its files' places.
func (s *Service) renumber(ctx context.Context, folder accounts.ID, stremioID string, catalog *stremio.Meta) error {
	files, err := s.showFiles(ctx, folder, stremioID)
	if err != nil || len(files) == 0 {
		return err
	}
	choice := files[0].choice
	if choice.Numbering == NumberedAsCatalog && !slices.ContainsFunc(files, func(f showFile) bool { return f.told.absolute > 0 }) {
		return nil
	}
	n := s.numbererFor(ctx, choice, catalog)
	if choice.Numbering == NumberedInOrder && n.known && !n.orderKnown {
		return nil
	}
	return s.placeFiles(ctx, files, n)
}

// entryPlaces places the files of entries, a unit of a folder being
// stored, matched to the show of stremioID, by path, with n, among the
// show's other files the folder holds.
func (s *Service) entryPlaces(ctx context.Context, folder accounts.ID, stremioID string, entries []entry, n numberer) (map[string]placed, error) {
	others, err := s.showFiles(ctx, folder, stremioID)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]bool, len(entries))
	var files []told
	for _, e := range entries {
		if !e.episode.episodic {
			continue
		}
		paths[e.path] = true
		t := told{season: e.episode.season, first: e.episode.episode, last: max(e.episode.lastEpisode, e.episode.episode),
			absolute: e.episode.absolute, path: e.path}
		_, _, t.given = showTitle(e.path)
		files = append(files, t)
	}
	count := len(files)
	for _, f := range others {
		if !paths[f.told.path] {
			files = append(files, f.told)
		}
	}
	places := make(map[string]placed, count)
	for i, p := range n.places(files)[:count] {
		places[files[i].path] = p
	}
	return places, nil
}

// catalogOf is the description the server's metadata addons give of the
// series of stremioID, nil when none does.
func (s *Service) catalogOf(ctx context.Context, stremioID string) *stremio.Meta {
	meta, ok := s.titles.TitleMeta(ctx, "series", stremioID)
	if !ok {
		return nil
	}
	return &meta
}

// SetNumbering says how the files of the show of stremioID, in a shows
// folder, are numbered, and places them so at once: what users did with
// their episodes follows them, and the episodes only the files hold are
// noted again (see NoteUnlisted). The group of NumberedInOrder must be one
// of the series' TMDB episode groups.
func (s *Service) SetNumbering(ctx context.Context, id accounts.ID, stremioID string, choice ShowNumbering) error {
	switch choice.Numbering {
	case NumberedAsCatalog, NumberedPerSeason, NumberedContinuing, NumberedAbsolute:
		choice.Group = ""
	case NumberedInOrder:
		if choice.Group == "" {
			return ErrInvalidNumbering
		}
	default:
		return ErrInvalidNumbering
	}
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return err
	}
	var held bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM local_files WHERE addon_id = $1 AND stremio_id = $2 AND season IS NOT NULL)",
		id, stremioID).Scan(&held); err != nil {
		return err
	}
	if folder.Kind != KindShows || !held {
		return addons.ErrNotFound
	}
	catalog := s.catalogOf(ctx, stremioID)
	if choice.Numbering == NumberedInOrder {
		if catalog == nil {
			return library.ErrUnknownGroup
		}
		groups, err := s.titles.EpisodeGroupsOf(ctx, *catalog)
		switch {
		case errors.Is(err, tmdb.ErrNoKey):
			return err
		case errors.Is(err, tmdb.ErrNotFound):
			return library.ErrUnknownGroup
		case err != nil:
			return errors.Join(ErrNoGroups, err)
		case !slices.ContainsFunc(groups, func(g tmdb.GroupSummary) bool { return g.ID == choice.Group }):
			return library.ErrUnknownGroup
		}
	}
	if choice.Numbering == NumberedAsCatalog {
		_, err = s.db.Exec(ctx, "DELETE FROM local_show_numbering WHERE addon_id = $1 AND stremio_id = $2", id, stremioID)
	} else {
		_, err = s.db.Exec(ctx, `INSERT INTO local_show_numbering (addon_id, stremio_id, numbering, episode_group) VALUES ($1, $2, $3, $4)
			ON CONFLICT (addon_id, stremio_id) DO UPDATE SET numbering = $3, episode_group = $4`, id, stremioID, choice.Numbering, choice.Group)
	}
	if err != nil {
		return err
	}
	files, err := s.showFiles(ctx, id, stremioID)
	if err != nil {
		return err
	}
	if err := s.placeFiles(ctx, files, s.numbererFor(ctx, choice, catalog)); err != nil {
		return err
	}
	if catalog == nil {
		return nil
	}
	return s.noteSeries(ctx, stremioID, catalog.Videos)
}

// noteSeries notes the episodes the files of the enabled shows folders
// hold for the series of stremioID that videos, its catalog, does not
// list, as describing it does (see NoteUnlisted).
func (s *Service) noteSeries(ctx context.Context, stremioID string, videos []stremio.Video) error {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT f.season, e FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
		JOIN addons a ON a.id = f.addon_id, generate_series(f.episode, coalesce(f.last_episode, f.episode)) e
		WHERE f.stremio_id = $1 AND l.kind = $2 AND a.enabled AND f.season IS NOT NULL AND f.episode IS NOT NULL ORDER BY 1, 2`,
		stremioID, KindShows)
	if err != nil {
		return err
	}
	listed := library.ListedNumbers(videos)
	var unlisted [][2]int
	var season, number int
	if _, err := pgx.ForEachRow(rows, []any{&season, &number}, func() error {
		if _, found := slices.BinarySearchFunc(listed, [2]int{season, number}, func(a, b [2]int) int {
			return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
		}); !found {
			unlisted = append(unlisted, [2]int{season, number})
		}
		return nil
	}); err != nil {
		return err
	}
	if len(unlisted) == 0 {
		listed = nil
	}
	return s.NoteUnlisted(ctx, stremioID, unlisted, listed)
}

// NumberingGroups lists the TMDB episode groups of the series of
// stremioID, which a show of a folder may be numbered in: tmdb.ErrNoKey
// without a TMDB key, library.ErrUnknownGroup for a series no metadata
// addon describes or TMDB does not know.
func (s *Service) NumberingGroups(ctx context.Context, id accounts.ID, stremioID string) ([]tmdb.GroupSummary, error) {
	if _, err := s.Folder(ctx, id); err != nil {
		return nil, err
	}
	catalog := s.catalogOf(ctx, stremioID)
	if catalog == nil {
		return nil, library.ErrUnknownGroup
	}
	groups, err := s.titles.EpisodeGroupsOf(ctx, *catalog)
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		return nil, library.ErrUnknownGroup
	case err != nil && !errors.Is(err, tmdb.ErrNoKey):
		return nil, errors.Join(ErrNoGroups, err)
	}
	return groups, err
}

// FolderShow is a show a shows folder holds: its series' Stremio
// identifier, the name it was matched to, the first of its units by path,
// how many files it has, how they are numbered, how many of them its
// numbering mapped to the catalog's episodes, and those it could not
// place, by path.
type FolderShow struct {
	StremioID  string
	Name, Unit string
	Files      int
	Numbering  ShowNumbering
	Mapped     int
	Unmapped   []UnmappedFile
}

// UnmappedFile is a show's file its numbering could not place: its path,
// the season and episodes it tells, its absolute number when its name
// tells one alone, the episode of its own it is listed as, and why (see
// the unmapped reasons).
type UnmappedFile struct {
	Path                string
	Season, First, Last int
	Absolute            int
	Listed              [3]int
	Reason              string
}

// Shows lists the shows a shows folder holds, by name, with how their
// files are numbered.
func (s *Service) Shows(ctx context.Context, id accounts.ID) ([]FolderShow, error) {
	if _, err := s.Folder(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+showFileColumns+`, f.name, f.unit FROM local_files f
		LEFT JOIN local_show_numbering n ON n.addon_id = f.addon_id AND n.stremio_id = f.stremio_id
		JOIN local_folders l ON l.addon_id = f.addon_id
		WHERE f.addon_id = $1 AND l.kind = $2 AND f.stremio_id IS NOT NULL AND f.season IS NOT NULL AND f.episode IS NOT NULL
		ORDER BY f.path`, id, KindShows)
	if err != nil {
		return nil, err
	}
	var shows []FolderShow
	index := map[string]int{}
	_, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct{}, error) {
		var name, unit string
		f, err := scanShowFile(namedRow{row, []any{&name, &unit}})
		if err != nil {
			return struct{}{}, err
		}
		i, ok := index[f.stremioID]
		if !ok {
			i = len(shows)
			index[f.stremioID] = i
			shows = append(shows, FolderShow{StremioID: f.stremioID, Name: name, Unit: unit, Numbering: f.choice})
		}
		show := &shows[i]
		show.Files++
		switch f.place.numbering {
		case numberingMapped:
			show.Mapped++
		case numberingUnmapped:
			show.Unmapped = append(show.Unmapped, UnmappedFile{Path: f.told.path, Season: f.told.season, First: f.told.first,
				Last: f.told.last, Absolute: f.told.absolute, Listed: [3]int{f.place.season, f.place.first, f.place.last},
				Reason: f.place.unmapped})
		}
		return struct{}{}, nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(shows, func(a, b FolderShow) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.StremioID, b.StremioID))
	})
	return shows, nil
}

// namedRow is a row read by scanShowFile, then into more, the columns
// after showFileColumns.
type namedRow struct {
	pgx.CollectableRow
	more []any
}

func (r namedRow) Scan(dest ...any) error {
	return r.CollectableRow.Scan(append(dest, r.more...)...)
}
