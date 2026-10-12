package localfiles

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// A show's disc: a Blu-ray disc's folder, a DVD's or a disc image in a
// shows folder, in a show's folder or one of its season folders (see
// showDisc). Its titles are its episodes (see chooseEpisodes), each an
// entry of the show's unit, matched with the show: a playlist's path, or a
// DVD's or an image's with its title's number (see discTitlePath). The
// episodes a title holds are those an administrator gave it
// (local_disc_episodes), else those the disc's name gives, as in
// "S01E01-E04", when it gives as many as the disc has titles; without
// them, the title is unmatched (reasonDiscEpisode), and Polyfin suggests
// the episodes after those of the season's discs before it. A title plays
// as another movie of a movies folder's disc does (see Input).

// minEpisode is how long a title of a show's disc plays at least to be an
// episode: shorter ones are trailers, menus' clips and most extras, and a
// cartoon's episodes are longer. FFmpeg's bluray protocol also refuses a
// disc without a playlist this long (see minPlayable).
const minEpisode = 3 * time.Minute

// maxDiscEpisodes bounds the episodes of a show's disc: a box set's disc
// holds a few dozen at most, short cartoons included. A disc of more keeps
// the first by number.
const maxDiscEpisodes = 40

// chooseEpisodes chooses the episodes of a show's disc among its
// candidates, and returns their indexes, in the order of their numbers:
// the titles at least minEpisode long, but those playing the same parts
// as a title of a lower number, and the "play all" titles, which play
// mostly parts of two or more other titles, each wholly within it. A title
// replaying an episode with another audio track counts once, and one
// chaining a disc's episodes, whatever its order or its own short parts,
// is left out. At most maxDiscEpisodes are kept.
func chooseEpisodes(candidates []titleCandidate) []int {
	var long []int
	parts := map[int]ranges{}
	for i, c := range candidates {
		if c.length < minEpisode {
			continue
		}
		if r := rangesOf(c.parts); r.size() > 0 {
			long = append(long, i)
			parts[i] = r
		}
	}
	var episodes []int
	for _, i := range long {
		own, size := parts[i], parts[i].size()
		var within []ranges
		repeated := false
		for _, j := range long {
			other := parts[j]
			if j == i || other.overlap(own) != other.size() {
				continue
			}
			if own.overlap(other) == size {
				// The same parts: the title of the lower number stays.
				if c := cmp.Compare(candidates[j].number, candidates[i].number); c < 0 || c == 0 && j < i {
					repeated = true
					break
				}
				continue
			}
			within = append(within, other)
		}
		if repeated || len(within) >= 2 && own.overlap(unionOf(within))*2 > size {
			continue
		}
		episodes = append(episodes, i)
	}
	slices.SortFunc(episodes, func(a, b int) int {
		return cmp.Or(cmp.Compare(candidates[a].number, candidates[b].number), cmp.Compare(a, b))
	})
	return episodes[:min(len(episodes), maxDiscEpisodes)]
}

// ranges are the parts of a disc a title plays, by their source (see
// titlePart), each source's ranges in order, apart from one another.
type ranges map[string][][2]uint64

// rangesOf merges the parts a title plays, those of no length left out.
func rangesOf(parts []titlePart) ranges {
	r := ranges{}
	for _, p := range parts {
		if p.to > p.from {
			r[p.source] = append(r[p.source], [2]uint64{p.from, p.to})
		}
	}
	for source, list := range r {
		slices.SortFunc(list, func(a, b [2]uint64) int { return cmp.Compare(a[0], b[0]) })
		merged := list[:1]
		for _, next := range list[1:] {
			if last := &merged[len(merged)-1]; next[0] <= last[1] {
				last[1] = max(last[1], next[1])
			} else {
				merged = append(merged, next)
			}
		}
		r[source] = merged
	}
	return r
}

// unionOf merges several titles' ranges.
func unionOf(list []ranges) ranges {
	var parts []titlePart
	for _, r := range list {
		for source, ranges := range r {
			for _, x := range ranges {
				parts = append(parts, titlePart{source: source, from: x[0], to: x[1]})
			}
		}
	}
	return rangesOf(parts)
}

// size is how much r holds, in its sources' units.
func (r ranges) size() uint64 {
	var n uint64
	for _, list := range r {
		for _, x := range list {
			n += x[1] - x[0]
		}
	}
	return n
}

// overlap is how much of r o holds too.
func (r ranges) overlap(o ranges) uint64 {
	var n uint64
	for source, list := range r {
		other := o[source]
		i, j := 0, 0
		for i < len(list) && j < len(other) {
			from, to := max(list[i][0], other[j][0]), min(list[i][1], other[j][1])
			if to > from {
				n += to - from
			}
			if list[i][1] < other[j][1] {
				i++
			} else {
				j++
			}
		}
	}
	return n
}

// showDisc reports whether the disc at rel, its folder or, image set, its
// image, is a show's: in the folder of a show, the first of its path,
// which a season folder at the shows folder's top is not. A disc at the
// folder's root, and an image at its top, belong to no show.
func showDisc(rel string, image bool) bool {
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	if image && len(parts) < 2 {
		return false
	}
	return !seasonFolder.MatchString(parts[0]) && !strings.EqualFold(parts[0], "specials")
}

// showTitle tells whether rel may be the entry of a title of a show's disc
// (see collapseDiscs, collapseDVDs and collapseImages): a playlist's path,
// or a DVD's or an image's with its title's number (see discTitlePath);
// with the disc's path, its folder or its image, and the title's number.
// Only the entries of a shows folder are read so.
func showTitle(rel string) (disc string, number int, ok bool) {
	if disc, number, ok := discPlaylist(rel); ok {
		return disc, number, showDisc(disc, false)
	}
	base, number, ok := titleOf(rel)
	if !ok {
		return "", 0, false
	}
	if imageFile(base) {
		return base, number, showDisc(base, true)
	}
	if disc, ok := dvdOf(base); ok {
		return disc, number, showDisc(disc, false)
	}
	return "", 0, false
}

var (
	// discNumberTag numbers a disc in its name: "Disc 2", "DISC02",
	// "Disk 2", "DVD 2", "BD2", "Vol. 2", "Part 2", or the D of "S01D02".
	discNumberTag = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:disc|disk|dvd|bd|cd|vol(?:ume)?|part|d)[ ._-]*(\d{1,3})(?:$|[^0-9])`)
	// discSeasonTag gives a disc's season in its name: "S02", "Season 2",
	// "Saison 2".
	discSeasonTag = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:season|series|saison|staffel|temporada|s)[ ._-]*(\d{1,3})(?:$|[^0-9])`)
)

// discPlace is where a show's disc stands among its show's: its season,
// its number among the season's discs, 0 when its name gives none, and
// the episodes its name gives, when it gives some.
type discPlace struct {
	season, number int
	named          name
}

// placeOf reads where the show's disc at disc, its folder or its image,
// stands: its season is the one its name gives with its episodes, else
// its season folder's, the nearest above it or the disc's folder itself,
// else the one its name gives, else 1; its number is the one its name
// gives (see discNumberTag).
func placeOf(disc string) discPlace {
	parts := strings.Split(disc, "/")
	own := parts[len(parts)-1]
	folders := parts[1:]
	if imageFile(own) {
		own = strings.TrimSuffix(own, path.Ext(own))
		folders = parts[1 : len(parts)-1]
	}
	p := discPlace{season: -1}
	for i := len(folders) - 1; i >= 0 && p.season < 0; i-- {
		if m := seasonFolder.FindStringSubmatch(folders[i]); m != nil {
			p.season, _ = strconv.Atoi(m[1])
		} else if strings.EqualFold(folders[i], "specials") {
			p.season = 0
		}
	}
	if named := parseName(own); named.episodic {
		p.named, p.season = named, named.season
	}
	if m := discSeasonTag.FindStringSubmatch(own); m != nil && p.season < 0 {
		p.season, _ = strconv.Atoi(m[1])
	}
	if p.season < 0 {
		p.season = 1
	}
	if m := discNumberTag.FindStringSubmatch(own); m != nil {
		p.number, _ = strconv.Atoi(m[1])
	}
	return p
}

// sortTitles sorts the paths of a show's disc's titles by their numbers.
func sortTitles(titles []string) {
	slices.SortFunc(titles, func(a, b string) int {
		_, x, _ := showTitle(a)
		_, y, _ := showTitle(b)
		return cmp.Or(cmp.Compare(x, y), strings.Compare(a, b))
	})
}

// numberDiscs gives the titles of shows' discs among a shows folder's
// entries, by path, their episodes: those given, by path (see
// givenEpisodes), else, for a disc whose name gives as many episodes as
// it has titles, those in order, else none.
func numberDiscs(entries map[string]entry, given map[string]name) {
	discs := map[string][]string{}
	for rel := range entries {
		if disc, _, ok := showTitle(rel); ok {
			discs[disc] = append(discs[disc], rel)
		}
	}
	for disc, titles := range discs {
		sortTitles(titles)
		named := placeOf(disc).named
		ranged := named.episodic && named.lastEpisode-named.episode+1 == len(titles)
		for i, rel := range titles {
			e := entries[rel]
			n, ok := given[rel]
			switch {
			case ok:
				e.episode = n
			case ranged:
				e.episode = name{episodic: true, season: named.season, episode: named.episode + i, lastEpisode: named.episode + i}
			default:
				e.episode = name{}
			}
			entries[rel] = e
		}
	}
}

// formatEpisode writes an episode's numbers as a file's name gives them,
// "S01E05", or "S01E05-E06" for several; empty for none.
func formatEpisode(n name) string {
	if !n.episodic {
		return ""
	}
	text := fmt.Sprintf("S%02dE%02d", n.season, n.episode)
	if n.lastEpisode > n.episode {
		text += fmt.Sprintf("-E%02d", n.lastEpisode)
	}
	return text
}

// readEpisode reads episodes written as a file's name gives them, and
// nothing else: "S01E05", "s1e5", "S01E05-E06", "S01E05E06" or
// "S01E05-06".
func readEpisode(text string) (name, bool) {
	text = strings.TrimSpace(text)
	loc := episodeTag.FindStringIndex(text)
	if loc == nil || loc[0] != 0 || loc[1] != len(text) {
		return name{}, false
	}
	n := parseName(text)
	return n, n.episodic
}

// givenEpisodes reads the episodes administrators gave the titles of a
// folder's shows' discs, by path.
func (s *Service) givenEpisodes(ctx context.Context, id accounts.ID) (map[string]name, error) {
	rows, err := s.db.Query(ctx, "SELECT path, season, episode, last_episode FROM local_disc_episodes WHERE addon_id = $1", id)
	if err != nil {
		return nil, err
	}
	given := map[string]name{}
	var rel string
	n := name{episodic: true}
	_, err = pgx.ForEachRow(rows, []any{&rel, &n.season, &n.episode, &n.lastEpisode}, func() error {
		given[rel] = n
		return nil
	})
	return given, err
}

// DiscTitle is a title of a show's disc, as Unmatched lists it: its
// disc's folder or image, its number, how long it plays, 0 when unknown,
// the episodes it has, and those Polyfin suggests it holds when it has
// none, each as a file's name gives them ("S01E05", "S01E05-E06"), empty
// for none.
type DiscTitle struct {
	Disc      string
	Number    int
	Length    time.Duration
	Episode   string
	Suggested string
}

// showDiscTitles reads the titles of a shows folder's discs, matched or
// not, by path, and suggests the episodes of those without: each disc's
// titles in order, the season's discs in the order of their numbers, then
// of their paths, after the episodes the titles before them have; from the
// first episode the disc's name gives, for a disc whose name gives fewer
// or more than its titles; from the first episode for a season's first.
func (s *Service) showDiscTitles(ctx context.Context, id accounts.ID) (map[string]DiscTitle, error) {
	rows, err := s.db.Query(ctx, `SELECT path, season, episode, last_episode, coalesce(length, 0) FROM local_files
		WHERE addon_id = $1 AND (path ~ '#[0-9]+$' OR path ~* '/BDMV/PLAYLIST/[0-9]{5}\.mpls$')`, id)
	if err != nil {
		return nil, err
	}
	titles := map[string]DiscTitle{}
	numbers := map[string]name{}
	discs := map[string][]string{}
	var rel string
	var season, episode, last *int
	var seconds int
	_, err = pgx.ForEachRow(rows, []any{&rel, &season, &episode, &last, &seconds}, func() error {
		disc, number, ok := showTitle(rel)
		if !ok {
			return nil
		}
		t := DiscTitle{Disc: disc, Number: number, Length: time.Duration(seconds) * time.Second}
		if season != nil && episode != nil && last != nil {
			numbers[rel] = name{episodic: true, season: *season, episode: *episode, lastEpisode: *last}
			t.Episode = formatEpisode(numbers[rel])
		}
		titles[rel] = t
		discs[disc] = append(discs[disc], rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The discs of each season of each show.
	type showSeason struct {
		show   string
		number int
	}
	type placed struct {
		disc  string
		place discPlace
	}
	seasons := map[showSeason][]placed{}
	for disc := range discs {
		show, _, _ := strings.Cut(disc, "/")
		p := placeOf(disc)
		key := showSeason{show, p.season}
		seasons[key] = append(seasons[key], placed{disc, p})
	}
	for key, list := range seasons {
		slices.SortFunc(list, func(a, b placed) int {
			return cmp.Or(cmp.Compare(a.place.number, b.place.number), strings.Compare(a.disc, b.disc))
		})
		next := 1
		for _, d := range list {
			if d.place.named.episodic {
				next = d.place.named.episode
			}
			sortTitles(discs[d.disc])
			for _, rel := range discs[d.disc] {
				if n, ok := numbers[rel]; ok {
					next = n.lastEpisode + 1
					continue
				}
				t := titles[rel]
				t.Suggested = formatEpisode(name{episodic: true, season: key.number, episode: next, lastEpisode: next})
				titles[rel] = t
				next++
			}
		}
	}
	return titles, nil
}

// ErrInvalidEpisode reports episodes not written as a file's name gives
// them.
var ErrInvalidEpisode = errors.New("episodes are written as S01E05 or S01E05-E06")

// maxEpisodeChanges bounds the titles SetEpisodes numbers at once.
const maxEpisodeChanges = 1000

// EpisodeChange gives the title of a show's disc at Path, the path its
// entry has (see Unmatched), its episodes, written as a file's name gives
// them, "S01E05" or "S01E05-E06"; an empty Episode removes those given.
type EpisodeChange struct {
	Path    string
	Episode string
}

// SetEpisodes gives titles of shows' discs of a shows folder their
// episodes, for good, or removes those given: each title is that episode
// of its show at once, when its show is matched, and stays so through
// scans, even while its disc is away. A title whose episodes are removed
// has those its disc's name gives, or none.
func (s *Service) SetEpisodes(ctx context.Context, id accounts.ID, changes []EpisodeChange) error {
	if len(changes) == 0 || len(changes) > maxEpisodeChanges {
		return ErrInvalidEpisode
	}
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return err
	}
	if folder.Kind != KindShows {
		return addons.ErrNotFound
	}
	numbers := make([]name, len(changes))
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.Path
		if strings.TrimSpace(c.Episode) == "" {
			continue
		}
		n, ok := readEpisode(c.Episode)
		if !ok {
			return ErrInvalidEpisode
		}
		numbers[i] = n
	}
	rows, err := s.db.Query(ctx, "SELECT path, unit FROM local_files WHERE addon_id = $1 AND path = ANY($2)", id, paths)
	if err != nil {
		return err
	}
	units := map[string]string{}
	var rel, unit string
	if _, err := pgx.ForEachRow(rows, []any{&rel, &unit}, func() error {
		units[rel] = unit
		return nil
	}); err != nil {
		return err
	}
	for _, c := range changes {
		if _, _, ok := showTitle(c.Path); !ok || units[c.Path] == "" {
			return addons.ErrNotFound
		}
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		for i, c := range changes {
			n := numbers[i]
			if !n.episodic {
				if _, err := tx.Exec(ctx, "DELETE FROM local_disc_episodes WHERE addon_id = $1 AND path = $2", id, c.Path); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO local_disc_episodes (addon_id, path, season, episode, last_episode) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (addon_id, path) DO UPDATE SET season = $3, episode = $4, last_episode = $5`,
				id, c.Path, n.season, n.episode, n.lastEpisode); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	matched, err := s.matchedUnits(ctx, id)
	if err != nil {
		return err
	}
	links, err := s.links(ctx, id)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	defer s.artworkChanged()
	for _, c := range changes {
		unit := units[c.Path]
		if done[unit] {
			continue
		}
		done[unit] = true
		entries, err := s.unitFiles(ctx, folder, unit)
		if err != nil || len(entries) == 0 {
			return err
		}
		// The show keeps its title; a show none of whose files had
		// episodes is matched as a scan matches it.
		m, ok := matched[unit]
		if !ok {
			if m, err = s.unitMatch(ctx, folder.Kind, entries[0], links[unit], newSearches()); err != nil {
				return err
			}
		}
		if err := s.store(ctx, id, folder.Kind, entries, m); err != nil {
			return err
		}
	}
	return nil
}

// GivenEpisode is the episodes an administrator gave a title of a show's
// disc (see SetEpisodes): its path, its disc's kind (see Unmatched), its
// disc's folder or image and its number, and its episodes, as a file's
// name gives them.
type GivenEpisode struct {
	Path    string
	Kind    string
	Disc    string
	Number  int
	Episode string
}

// folderEpisodes lists the episodes given to the titles of a folder's
// shows' discs, by path.
func (s *Service) folderEpisodes(ctx context.Context, id accounts.ID) ([]GivenEpisode, error) {
	rows, err := s.db.Query(ctx, `SELECT e.path, e.season, e.episode, e.last_episode, coalesce(i.kind, '') FROM local_disc_episodes e
		LEFT JOIN local_disc_images i ON i.addon_id = e.addon_id AND i.path = regexp_replace(e.path, '#[0-9]+$', '')
		WHERE e.addon_id = $1 ORDER BY e.path`, id)
	if err != nil {
		return nil, err
	}
	list := []GivenEpisode{}
	var rel, image string
	n := name{episodic: true}
	_, err = pgx.ForEachRow(rows, []any{&rel, &n.season, &n.episode, &n.lastEpisode, &image}, func() error {
		disc, number, ok := showTitle(rel)
		if !ok {
			return nil
		}
		given := GivenEpisode{Path: rel, Kind: imageKind(image), Disc: disc, Number: number, Episode: formatEpisode(n)}
		if kind, _, ok := discEntry(rel); ok {
			given.Kind = kind
		}
		list = append(list, given)
		return nil
	})
	return list, err
}
