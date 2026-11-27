package iptv

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Versions of the same title (Options.TitleVersions). A provider may list
// the same movie or series more than once, as entries of other qualities
// or categories. An Xtream account whose option is on makes the titles of
// a type that its lists give the same TMDB id one title, whose versions
// are each entry's file. The groups are stored (iptv_title_groups), each
// with its leader: the entry first in the provider's order when the group
// formed, whose identifiers the group takes. It keeps them while the
// group lasts, even once the provider no longer lists the leader, as a
// line-up's channel keeps its identity through its former streams; the
// group ends once one of its titles alone may still be grouped (see
// regroup), and waits under its leader once the provider lists none. Its
// face, the leader while listed, else the member that joined first, gives
// its list data and the category catalogs list it in; catalogs, counts and
// searches list each group once (see notGroupedAway). The other members'
// identifiers are no longer described nor played: the library opens them
// as the group's title (see Grouped).
//
// A series' episodes are matched by season and episode number: the
// group's episode of numbers s and n (see groupEpisodeID) plays the file
// of each listed member that has one, a member's first of those numbers.
// An episode without a number, or another of the same numbers in the same
// entry, keeps its entry's identifier and plays its file alone. Matching
// needs each member's details: opening a group asks the provider for each
// listed member's, one request a second per host (see Pacer).
//
// A title joining a group gives what users did with it, its seasons and
// its episodes matched to the group's items (see userdata.Store.Follow);
// so does a group ending as its leader is kept apart or the option is
// turned off, to the leader's own items, else to its members' (see
// splitMoves). An administrator keeps apart a title whose TMDB id is
// wrong (see KeepApart); it then forms no group.

// episodeNumbers are an episode's season and episode numbers.
type episodeNumbers struct{ season, episode int }

// numbered are the episodes of d a group matches by number: the first of
// each season and episode number, an episode number given.
func numbered(d *details) map[episodeNumbers]episodeDetails {
	if d == nil {
		return nil
	}
	result := make(map[episodeNumbers]episodeDetails, len(d.Episodes))
	for _, e := range d.Episodes {
		n := episodeNumbers{e.Season, e.Episode}
		if _, seen := result[n]; !seen && e.Episode > 0 {
			result[n] = e
		}
	}
	return result
}

// groupEpisodeID is the Stremio identifier of the episode of numbers n of
// the group of series leader.
func groupEpisodeID(source accounts.ID, leader string, n episodeNumbers) string {
	return prefix(source) + "gep:" + leader + ":" + strconv.Itoa(n.season) + ":" + strconv.Itoa(n.episode)
}

// groupEpisodeParts reads what follows "gep:" in a group's episode
// identifier: its leader and numbers.
func groupEpisodeParts(rest string) (string, int, int, bool) {
	rest, number, ok := cutLast(rest)
	if !ok {
		return "", 0, 0, false
	}
	leader, season, ok := cutLast(rest)
	if !ok || leader == "" {
		return "", 0, 0, false
	}
	return leader, season, number, true
}

// cutLast cuts the number after the last colon of text.
func cutLast(text string) (string, int, bool) {
	at := strings.LastIndexByte(text, ':')
	if at < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(text[at+1:])
	return text[:at], n, err == nil && n >= 0
}

// playsAsVOD reports whether the key of a source's identifier names a
// movie or an episode, which vodStreams answers.
func playsAsVOD(key string) bool {
	return strings.HasPrefix(key, "vod:") || strings.HasPrefix(key, "ep:") || strings.HasPrefix(key, "gep:")
}

// notGroupedAway selects, for t, the titles a catalog lists: all but the
// members of a group other than its face.
const notGroupedAway = `NOT EXISTS (SELECT 1 FROM iptv_title_members m JOIN iptv_title_groups g
	ON g.addon_id = m.addon_id AND g.type = m.type AND g.tmdb = m.tmdb
	WHERE m.addon_id = t.addon_id AND m.type = t.type AND m.key = t.key AND g.face <> t.key)`

// groupLeader is, for t, the leader of the group it is the face of, null
// for none.
const groupLeader = `(SELECT g.leader FROM iptv_title_groups g WHERE g.addon_id = t.addon_id AND g.type = t.type AND g.tmdb = t.tmdb
	AND g.face = t.key)`

// titleGroup is a group of a source's titles of a type: its TMDB id, its
// leader, its members in the order they joined it (the leader first) and
// those the provider lists now, in that order.
type titleGroup struct {
	typ, tmdb, leader string
	members, listed   []string
}

type typedKey struct{ typ, key string }

// sourceGroups are a source's groups at a version of the source (see
// CatalogVersion), by the key of each of their members.
type sourceGroups struct {
	version string
	byKey   map[typedKey]*titleGroup
}

// groupOf returns the group of a source's title of a type, nil when the
// source, of state, groups none or the title is in none. The groups are
// read once for each version of the source: every change of them updates
// the source's row (see reconcile and KeepApart).
func (s *Service) groupOf(ctx context.Context, state sourceState, source accounts.ID, typ, key string) (*titleGroup, error) {
	if !state.titleVersions {
		return nil, nil
	}
	s.mu.Lock()
	kept := s.groups[source]
	s.mu.Unlock()
	if kept == nil || kept.version != state.version {
		read, err := s.readGroups(ctx, source)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.groups == nil {
			s.groups = map[accounts.ID]*sourceGroups{}
		}
		s.groups[source] = read
		s.mu.Unlock()
		kept = read
	}
	return kept.byKey[typedKey{typ, key}], nil
}

// titleGroup returns the group of a source's title of a type, as groupOf
// does.
func (s *Service) titleGroup(ctx context.Context, source accounts.ID, typ, key string) (*titleGroup, error) {
	state, err := s.sourceState(ctx, source)
	if errors.Is(err, stremio.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.groupOf(ctx, state, source, typ, key)
}

// readGroups reads a source's groups, with its version in the same
// statement.
func (s *Service) readGroups(ctx context.Context, source accounts.ID) (*sourceGroups, error) {
	rows, err := s.db.Query(ctx, `SELECT i.xmin::text || ' ' || coalesce(i.fetched_at::text, ''), g.type, g.tmdb, g.leader, m.key, m.listed
		FROM iptv_sources i LEFT JOIN iptv_title_groups g ON g.addon_id = i.addon_id
		LEFT JOIN iptv_title_members m ON m.addon_id = g.addon_id AND m.type = g.type AND m.tmdb = g.tmdb
		WHERE i.addon_id = $1 ORDER BY g.type, g.tmdb, m.position`, source)
	if err != nil {
		return nil, err
	}
	read := &sourceGroups{byKey: map[typedKey]*titleGroup{}}
	var current *titleGroup
	var typ, tmdb, leader, key *string
	var listed *bool
	if _, err := pgx.ForEachRow(rows, []any{&read.version, &typ, &tmdb, &leader, &key, &listed}, func() error {
		if typ == nil || key == nil {
			return nil
		}
		if current == nil || current.typ != *typ || current.tmdb != *tmdb {
			current = &titleGroup{typ: *typ, tmdb: *tmdb, leader: *leader}
		}
		current.members = append(current.members, *key)
		if *listed {
			current.listed = append(current.listed, *key)
		}
		read.byKey[typedKey{*typ, *key}] = current
		return nil
	}); err != nil {
		return nil, err
	}
	return read, nil
}

// groupMember is a listed member of a group, as groupMeta describes it.
type groupMember struct {
	title   Title
	kind    string
	details *details
}

// groupMeta describes a group: its face's description under its leader's
// identifier and, for a series, the episodes of all its listed members
// (see groupVideos). Opening it asks the provider for the details of each
// listed member that are due, as titleMeta does for one title.
func (s *Service) groupMeta(ctx context.Context, state sourceState, source accounts.ID, g *titleGroup) (stremio.Meta, error) {
	members := make([]groupMember, 0, len(g.listed))
	for _, key := range g.listed {
		kept, err := s.description(ctx, state, source, g.typ, key)
		if errors.Is(err, stremio.ErrNotFound) {
			continue
		}
		if err != nil {
			return stremio.Meta{}, err
		}
		d := kept.details
		if IsOpening(ctx) && kept.fetchDue(s.now()) {
			t, err := s.title(ctx, source, g.typ, key)
			if errors.Is(err, stremio.ErrNotFound) {
				continue
			}
			if err != nil {
				return stremio.Meta{}, err
			}
			if d, err = s.details(ctx, source, t, true); err != nil {
				return stremio.Meta{}, err
			}
		}
		members = append(members, groupMember{title: kept.title, kind: kept.kind, details: d})
	}
	if len(members) == 0 {
		return stremio.Meta{}, stremio.ErrNotFound
	}
	face := members[0]
	meta := describedTitle(source, face.title, face.kind, face.details, nil)
	meta.ID = titleID(source, g.typ, g.leader)
	if g.typ != typeSeries {
		return meta, nil
	}
	meta.Videos = groupVideos(source, g.leader, members)
	// Each season's poster, from the first member that has one.
	posters := map[string]string{}
	for _, m := range members {
		if m.details != nil {
			for season, poster := range m.details.Seasons {
				if _, ok := posters[season]; !ok {
					posters[season] = poster
				}
			}
		}
	}
	if len(posters) > 0 {
		meta.Extras = &stremio.Extras{SeasonPosterByNumber: posters}
	}
	return meta, nil
}

// groupVideos lists a group's episodes: each season and episode number
// once, described by the first member that has it, under the group's
// identifier, and the others under their entry's own (see numbered), by
// season and episode.
func groupVideos(source accounts.ID, leader string, members []groupMember) []stremio.Video {
	var videos []stremio.Video
	seen := map[episodeNumbers]bool{}
	for _, m := range members {
		if m.details == nil {
			continue
		}
		matched := numbered(m.details)
		for _, e := range m.details.Episodes {
			n := episodeNumbers{e.Season, e.Episode}
			if matched[n].Key != e.Key {
				videos = append(videos, e.video(episodeID(source, m.title.Key, e.Key)))
				continue
			}
			if !seen[n] {
				seen[n] = true
				videos = append(videos, e.video(groupEpisodeID(source, leader, n)))
			}
		}
	}
	slices.SortStableFunc(videos, func(a, b stremio.Video) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Episode, b.Episode))
	})
	return videos
}

// groupVersion is a member's file of a group's movie or episode, with
// the quality its list gives.
type groupVersion struct {
	stream  stremio.Stream
	title   Title
	quality string
}

// labelled are the streams of versions, each labelled by its quality and
// its entry's category, then, where two have the same label, by its
// entry's name, else its provider identifier.
func labelled(versions []groupVersion) []stremio.Stream {
	labels := make([]string, len(versions))
	count := map[string]int{}
	for i, v := range versions {
		labels[i] = strings.Join(slices.DeleteFunc([]string{v.quality, v.title.Category}, func(s string) bool { return s == "" }), " · ")
		count[labels[i]]++
	}
	streams := make([]stremio.Stream, len(versions))
	for i, v := range versions {
		streams[i] = v.stream
		label := labels[i]
		if count[label] > 1 {
			names := 0
			for j := range versions {
				if labels[j] == label && versions[j].title.Name == v.title.Name {
					names++
				}
			}
			extra := v.title.Name
			if names > 1 {
				extra = "#" + v.title.Key
			}
			label = strings.Join(slices.DeleteFunc([]string{label, extra}, func(s string) bool { return s == "" }), " · ")
		}
		streams[i].Description = label
	}
	return streams
}

// groupMovieStreams lists the file of each listed member of a movie's
// group (see labelled).
func (s *Service) groupMovieStreams(ctx context.Context, source accounts.ID, g *titleGroup) ([]stremio.Stream, error) {
	versions := make([]groupVersion, 0, len(g.listed))
	for _, key := range g.listed {
		t, err := s.title(ctx, source, typeMovie, key)
		if errors.Is(err, stremio.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		versions = append(versions, groupVersion{stream: s.movieStream(ctx, source, t), title: t.Title, quality: t.Quality})
	}
	return labelled(versions), nil
}

// groupEpisodeStreams lists the file of the episode of numbers season and
// episode of each listed member of a series' group that has one (see
// numbered and labelled), of the quality its name gives, else its
// series entry's.
func (s *Service) groupEpisodeStreams(ctx context.Context, source accounts.ID, g *titleGroup, season, episode int) ([]stremio.Stream, error) {
	var versions []groupVersion
	for _, key := range g.listed {
		t, err := s.title(ctx, source, typeSeries, key)
		if errors.Is(err, stremio.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		d, err := s.episodeDetails(ctx, source, t)
		if err != nil {
			return nil, err
		}
		e, ok := numbered(d)[episodeNumbers{season, episode}]
		if !ok {
			continue
		}
		quality := cmp.Or(e.Quality, t.Quality)
		stream := t.vodStream(quality, accountOf(t.kind, t.manifestAddress).vodURL("series", e.Key, e.Extension), nil)
		versions = append(versions, groupVersion{stream: stream, title: t.Title, quality: quality})
	}
	return labelled(versions), nil
}

// Grouped tells the Stremio identifier of the title that a source's movie,
// series or episode of id is a version of now: the group's, for a member
// of a group other than its leader, or for an episode of a member the
// group lists under its own numbers (see numbered); for an episode of a
// group that ended, the title whose episode of the same numbers has its
// users' data now (see formerGroupEpisode); "" for any other. The library
// opens such an identifier as that title, or its episode of the same
// numbers (see library.Service.RelinkedTitle).
func (s *Service) Grouped(ctx context.Context, source accounts.ID, id string) (string, error) {
	rest, ok := strings.CutPrefix(id, prefix(source))
	if !ok {
		return "", nil
	}
	var typ, key, episode string
	if key, ok = strings.CutPrefix(rest, "vod:"); ok {
		typ = typeMovie
	} else if key, ok = strings.CutPrefix(rest, "series:"); ok {
		typ = typeSeries
	} else if rest, ok = strings.CutPrefix(rest, "ep:"); ok {
		typ = typeSeries
		if key, episode, ok = strings.Cut(rest, ":"); !ok {
			return "", nil
		}
	} else if rest, ok = strings.CutPrefix(rest, "gep:"); ok {
		leader, season, number, valid := groupEpisodeParts(rest)
		if !valid {
			return "", nil
		}
		return s.formerGroupEpisode(ctx, source, leader, episodeNumbers{season, number})
	} else {
		return "", nil
	}
	group, err := s.titleGroup(ctx, source, typ, key)
	if err != nil || group == nil {
		return "", err
	}
	if episode == "" {
		if group.leader == key {
			return "", nil
		}
		return titleID(source, typ, group.leader), nil
	}
	t, err := s.title(ctx, source, typeSeries, key)
	if errors.Is(err, stremio.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	d, err := s.episodeDetails(ctx, source, t)
	if err != nil || d == nil {
		return "", err
	}
	for _, e := range numbered(d) {
		if e.Key == episode {
			return titleID(source, typeSeries, group.leader), nil
		}
	}
	return "", nil
}

// formerGroupEpisode tells, for the episode of numbers n of the group of
// series leader, the title whose episode of those numbers has the users'
// data the group's had once the group ended, as splitMoves gave it: the
// leader's own series while listed with such an episode, else the first
// listed series of its TMDB id that has one, kept apart or not; the group
// that series is a member of now, if any. "" while the group lasts, or
// when no series has such an episode.
func (s *Service) formerGroupEpisode(ctx context.Context, source accounts.ID, leader string, n episodeNumbers) (string, error) {
	if group, err := s.titleGroup(ctx, source, typeSeries, leader); err != nil || group != nil && group.leader == leader {
		return "", err
	}
	rows, err := s.db.Query(ctx, `SELECT t.key FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id
		WHERE t.addon_id = $1 AND t.type = $2 AND t.tmdb <> ''
		AND t.tmdb = (SELECT tmdb FROM iptv_titles WHERE addon_id = $1 AND type = $2 AND key = $3) AND `+shownTitle+`
		ORDER BY t.key = $3 DESC, t.position`, source, typeSeries, leader)
	if err != nil {
		return "", err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		t, err := s.title(ctx, source, typeSeries, key)
		if errors.Is(err, stremio.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		d, err := s.episodeDetails(ctx, source, t)
		if err != nil {
			return "", err
		}
		if _, ok := numbered(d)[n]; !ok {
			continue
		}
		group, err := s.titleGroup(ctx, source, typeSeries, key)
		if err != nil {
			return "", err
		}
		if group != nil {
			key = group.leader
		}
		return titleID(source, typeSeries, key), nil
	}
	return "", nil
}

// libraryItemID is the identifier of the library's item of key, as the
// library derives it (see ChannelItemID).
func libraryItemID(key string) accounts.ID {
	sum := sha256.Sum256([]byte("polyfin:item:" + key))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

// titleItem is the library's item of a source's title, as the library
// keys a movie or a series by its Stremio identifier.
func titleItem(source accounts.ID, typ, key string) userdata.Item {
	return userdata.Item{ID: libraryItemID(typ + "|" + titleID(source, typ, key))}
}

// seasonItem is the library's item of a season of a source's series.
func seasonItem(source accounts.ID, series string, season int) userdata.Item {
	return userdata.Item{ID: libraryItemID("season|" + titleID(source, typeSeries, series) + "|" + strconv.Itoa(season)),
		Series: titleItem(source, typeSeries, series).ID}
}

// episodeItem is the library's item of a source's episode of video id,
// of season of series.
func episodeItem(source accounts.ID, series string, season int, video string) userdata.Item {
	return userdata.Item{ID: libraryItemID("episode|" + video), Series: titleItem(source, typeSeries, series).ID,
		Season: seasonItem(source, series, season).ID}
}

// plannedGroup is a group as reconcileGroups makes it: its leader, its
// members in the order they joined, and whether the provider lists each.
type plannedGroup struct {
	tmdb, leader string
	members      []string
	listed       map[string]bool
}

// face is the member whose list data describes the group: the leader
// while listed, else the member that joined first.
func (g *plannedGroup) face() string {
	for _, key := range g.members {
		if g.listed[key] {
			return key
		}
	}
	return g.leader
}

// reconcileGroups makes a source's groups follow its titles and options,
// in tx (see the package's rules above), and gives what users did with the
// items of the titles that join or leave them to the items they become.
// The groups dissolved give theirs first, so that a member joining a new
// group gives what its own items gained.
func (s *Service) reconcileGroups(ctx context.Context, tx pgx.Tx, source accounts.ID, o Options) error {
	var splits, joins []userdata.Move
	// A type turned off shows no title: its groups last, as when the
	// provider lists none of their titles.
	for _, typ := range []string{typeMovie, typeSeries} {
		split, join, err := regroup(ctx, tx, source, typ, o.TitleVersions)
		if err != nil {
			return err
		}
		splits, joins = append(splits, split...), append(joins, join...)
	}
	settings := s.settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	store := userdata.InTx(tx)
	for _, moves := range [][]userdata.Move{splits, joins} {
		if err := store.Follow(ctx, moves, thresholds); err != nil {
			return err
		}
	}
	return nil
}

// regroup makes a source's groups of titles of typ follow its titles, in
// tx, every group dissolved unless on, and returns the moves of users'
// data: of the groups dissolved, then of the titles that joined one.
func regroup(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string, on bool) (split, join []userdata.Move, err error) {
	existing, err := storedGroups(ctx, tx, source, typ)
	if err != nil {
		return nil, nil, err
	}
	if !on {
		if len(existing) == 0 {
			return nil, nil, nil
		}
		if split, err = splitMoves(ctx, tx, source, typ, existing); err != nil {
			return nil, nil, err
		}
		_, err = tx.Exec(ctx, "DELETE FROM iptv_title_groups WHERE addon_id = $1 AND type = $2", source, typ)
		return split, nil, err
	}
	// The titles that may be grouped, by TMDB id, in the provider's order.
	rows, err := tx.Query(ctx, `SELECT t.key, t.tmdb FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id
		WHERE t.addon_id = $1 AND t.type = $2 AND t.tmdb <> '' AND `+shownTitle+`
		AND NOT EXISTS (SELECT 1 FROM iptv_titles_apart a WHERE a.addon_id = t.addon_id AND a.type = t.type AND a.key = t.key)
		ORDER BY t.position`, source, typ)
	if err != nil {
		return nil, nil, err
	}
	byTMDB := map[string][]string{}
	var order []string
	var key, tmdb string
	if _, err := pgx.ForEachRow(rows, []any{&key, &tmdb}, func() error {
		if byTMDB[tmdb] == nil {
			order = append(order, tmdb)
		}
		byTMDB[tmdb] = append(byTMDB[tmdb], key)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	apart, err := apartKeys(ctx, tx, source, typ)
	if err != nil {
		return nil, nil, err
	}
	// The members shown now, with the TMDB id the list gives them.
	var members []string
	for _, g := range existing {
		members = append(members, g.members...)
	}
	shown, err := shownTitles(ctx, tx, source, typ, members)
	if err != nil {
		return nil, nil, err
	}
	// A group whose leader is kept apart, or shown under another TMDB id
	// or none, ends: the leader's title is its own again, and the others
	// may form a new group. So does a group left with one title that may
	// be grouped, or none while some are shown: the others are kept apart,
	// of another TMDB id or not shown. The leader, else the first of them
	// still shown, has its users' data (see splitMoves). A group none of
	// whose titles is shown any more lasts, with its leader alone, for when
	// they are again: nothing could have its users' data.
	var kept, dissolved []*plannedGroup
	leaders := map[string]string{}
	for _, g := range existing {
		if tmdb, ok := shown[g.leader]; apart[g.leader] || ok && tmdb != g.tmdb {
			dissolved = append(dissolved, g)
			continue
		}
		kept = append(kept, g)
		leaders[g.leader] = g.tmdb
	}
	type joined struct {
		group *plannedGroup
		key   string
	}
	var joins []joined
	var planned []*plannedGroup
	grouped := map[string]bool{}
	for _, g := range kept {
		// A leader joins no other group while it leads one.
		keys := slices.DeleteFunc(slices.Clone(byTMDB[g.tmdb]), func(k string) bool { t, leads := leaders[k]; return leads && t != g.tmdb })
		if len(keys) < 2 {
			if len(keys) == 1 || slices.ContainsFunc(g.members, func(m string) bool { _, ok := shown[m]; return ok }) {
				delete(leaders, g.leader)
				dissolved = append(dissolved, g)
			} else {
				planned = append(planned, &plannedGroup{tmdb: g.tmdb, leader: g.leader, members: []string{g.leader}, listed: map[string]bool{}})
				grouped[g.tmdb] = true
			}
			continue
		}
		listed := map[string]bool{}
		for _, k := range keys {
			listed[k] = true
		}
		next := &plannedGroup{tmdb: g.tmdb, leader: g.leader, listed: listed}
		for _, m := range g.members {
			if m == g.leader || listed[m] {
				next.members = append(next.members, m)
			}
		}
		for _, k := range keys {
			if !slices.Contains(next.members, k) {
				next.members = append(next.members, k)
				joins = append(joins, joined{next, k})
			}
		}
		planned = append(planned, next)
		grouped[g.tmdb] = true
	}
	for _, tmdb := range order {
		if grouped[tmdb] {
			continue
		}
		keys := slices.DeleteFunc(slices.Clone(byTMDB[tmdb]), func(k string) bool { _, leads := leaders[k]; return leads })
		if len(keys) < 2 {
			continue
		}
		next := &plannedGroup{tmdb: tmdb, leader: keys[0], members: keys, listed: map[string]bool{}}
		for _, k := range keys {
			next.listed[k] = true
			joins = append(joins, joined{next, k})
		}
		planned = append(planned, next)
	}
	if len(dissolved) > 0 {
		if split, err = splitMoves(ctx, tx, source, typ, dissolved); err != nil {
			return nil, nil, err
		}
	}
	if len(joins) > 0 {
		var keys []string
		for _, j := range joins {
			keys = append(keys, j.key)
		}
		details, err := storedDetails(ctx, tx, source, typ, keys)
		if err != nil {
			return nil, nil, err
		}
		for _, j := range joins {
			join = append(join, joinMoves(source, typ, j.group.leader, j.key, details[j.key])...)
		}
	}
	if samePlan(existing, planned) {
		return split, join, nil
	}
	if _, err := tx.Exec(ctx, "DELETE FROM iptv_title_groups WHERE addon_id = $1 AND type = $2", source, typ); err != nil {
		return nil, nil, err
	}
	groupRows := make([][]any, 0, len(planned))
	var memberRows [][]any
	for _, g := range planned {
		groupRows = append(groupRows, []any{source, typ, g.tmdb, g.leader, g.face()})
		for position, m := range g.members {
			memberRows = append(memberRows, []any{source, typ, m, g.tmdb, position, g.listed[m]})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"iptv_title_groups"}, []string{"addon_id", "type", "tmdb", "leader", "face"},
		pgx.CopyFromRows(groupRows)); err != nil {
		return nil, nil, err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"iptv_title_members"}, []string{"addon_id", "type", "key", "tmdb", "position", "listed"},
		pgx.CopyFromRows(memberRows))
	return split, join, err
}

// samePlan reports whether the groups planned are those stored.
func samePlan(stored, planned []*plannedGroup) bool {
	if len(stored) != len(planned) {
		return false
	}
	byTMDB := make(map[string]*plannedGroup, len(stored))
	for _, g := range stored {
		byTMDB[g.tmdb] = g
	}
	for _, g := range planned {
		was := byTMDB[g.tmdb]
		if was == nil || was.leader != g.leader || !slices.Equal(was.members, g.members) || !maps.Equal(was.listed, g.listed) {
			return false
		}
	}
	return true
}

// storedGroups reads a source's groups of titles of typ, in tx.
func storedGroups(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string) ([]*plannedGroup, error) {
	rows, err := tx.Query(ctx, `SELECT g.tmdb, g.leader, m.key, m.listed FROM iptv_title_groups g
		JOIN iptv_title_members m ON m.addon_id = g.addon_id AND m.type = g.type AND m.tmdb = g.tmdb
		WHERE g.addon_id = $1 AND g.type = $2 ORDER BY g.tmdb, m.position`, source, typ)
	if err != nil {
		return nil, err
	}
	var groups []*plannedGroup
	var tmdb, leader, key string
	var listed bool
	_, err = pgx.ForEachRow(rows, []any{&tmdb, &leader, &key, &listed}, func() error {
		if len(groups) == 0 || groups[len(groups)-1].tmdb != tmdb {
			groups = append(groups, &plannedGroup{tmdb: tmdb, leader: leader, listed: map[string]bool{}})
		}
		g := groups[len(groups)-1]
		g.members = append(g.members, key)
		if listed {
			g.listed[key] = true
		}
		return nil
	})
	return groups, err
}

// shownTitles are the TMDB ids the list gives those of a source's titles
// of typ of keys that it shows, by key.
func shownTitles(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string, keys []string) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT t.key, t.tmdb FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id
		WHERE t.addon_id = $1 AND t.type = $2 AND t.key = ANY($3) AND `+shownTitle, source, typ, keys)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	var key, tmdb string
	_, err = pgx.ForEachRow(rows, []any{&key, &tmdb}, func() error {
		result[key] = tmdb
		return nil
	})
	return result, err
}

// apartKeys are the keys of a source's titles of typ kept apart.
func apartKeys(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, "SELECT key FROM iptv_titles_apart WHERE addon_id = $1 AND type = $2", source, typ)
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = true
	}
	return result, err
}

// storedDetails reads the details kept of a source's series of keys, by
// key; a movie's tell no episodes, and are not read.
func storedDetails(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string, keys []string) (map[string]*details, error) {
	result := map[string]*details{}
	if typ != typeSeries || len(keys) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx, "SELECT key, details FROM iptv_details WHERE addon_id = $1 AND type = $2 AND key = ANY($3)", source, typ, keys)
	if err != nil {
		return nil, err
	}
	var key string
	var d *details
	_, err = pgx.ForEachRow(rows, []any{&key, &d}, func() error {
		result[key], d = d, nil
		return nil
	})
	return result, err
}

// joinMoves are the moves of users' data as member joins the group of
// leader: its title's to the group's, and for a series, its seasons' to
// the group's of the same numbers and the episodes the group matches (see
// numbered) to the group's (see groupEpisodeID). The leader's title is
// the group's; its episodes move too.
func joinMoves(source accounts.ID, typ, leader, member string, d *details) []userdata.Move {
	var moves []userdata.Move
	if member != leader {
		moves = append(moves, userdata.Move{From: []accounts.ID{titleItem(source, typ, member).ID}, To: []userdata.Item{titleItem(source, typ, leader)}})
	}
	if typ != typeSeries || d == nil {
		return moves
	}
	seasons := map[int]bool{}
	for _, e := range d.Episodes {
		if member != leader && !seasons[e.Season] {
			seasons[e.Season] = true
			moves = append(moves, userdata.Move{From: []accounts.ID{seasonItem(source, member, e.Season).ID},
				To: []userdata.Item{seasonItem(source, leader, e.Season)}})
		}
	}
	for n, e := range numbered(d) {
		moves = append(moves, userdata.Move{From: []accounts.ID{episodeItem(source, member, e.Season, episodeID(source, member, e.Key)).ID},
			To: []userdata.Item{episodeItem(source, leader, n.season, groupEpisodeID(source, leader, n))}})
	}
	return moves
}

// splitMoves are the moves of users' data as groups end, which their
// leader's own items keep while the provider lists it: else the title's
// go to the member that joined first and is listed, and for a series,
// each episode's to that member's of the same numbers, else to the first
// listed member's that has one (see numbered).
func splitMoves(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string, groups []*plannedGroup) ([]userdata.Move, error) {
	var keys []string
	for _, g := range groups {
		keys = append(keys, g.members...)
	}
	shownKeys, err := shownTitles(ctx, tx, source, typ, keys)
	if err != nil {
		return nil, err
	}
	details, err := storedDetails(ctx, tx, source, typ, keys)
	if err != nil {
		return nil, err
	}
	var moves []userdata.Move
	for _, g := range groups {
		var shown []string
		for _, m := range g.members {
			if _, ok := shownKeys[m]; ok {
				shown = append(shown, m)
			}
		}
		if len(shown) == 0 {
			continue
		}
		target := shown[0]
		if slices.Contains(shown, g.leader) {
			target = g.leader
		}
		if target != g.leader {
			moves = append(moves, userdata.Move{From: []accounts.ID{titleItem(source, typ, g.leader).ID}, To: []userdata.Item{titleItem(source, typ, target)}})
		}
		if typ != typeSeries {
			continue
		}
		// Each number the group's members give, in the order they joined.
		owners := map[episodeNumbers]string{}
		episodes := map[episodeNumbers]episodeDetails{}
		var numbers []episodeNumbers
		for _, m := range g.members {
			for n, e := range numbered(details[m]) {
				if _, known := episodes[n]; !known {
					numbers = append(numbers, n)
					episodes[n] = episodeDetails{}
				}
				if slices.Contains(shown, m) && (owners[n] == "" || m == target) {
					owners[n], episodes[n] = m, e
				}
			}
		}
		seasons := map[int]bool{}
		for _, n := range numbers {
			if target != g.leader && !seasons[n.season] {
				seasons[n.season] = true
				moves = append(moves, userdata.Move{From: []accounts.ID{seasonItem(source, g.leader, n.season).ID},
					To: []userdata.Item{seasonItem(source, target, n.season)}})
			}
			owner := owners[n]
			if owner == "" {
				continue
			}
			moves = append(moves, userdata.Move{From: []accounts.ID{episodeItem(source, g.leader, n.season, groupEpisodeID(source, g.leader, n)).ID},
				To: []userdata.Item{episodeItem(source, owner, n.season, episodeID(source, owner, episodes[n].Key))}})
		}
	}
	return moves, nil
}

// TitleEntry is an entry of a source's list that shares a title's TMDB id
// (see TitleEntries): its key, name, category and quality; whether it is
// the group's leader, one of its members, kept apart (see KeepApart), and
// listed by the provider now. A leader no longer listed has only its key.
type TitleEntry struct {
	Key, Name, Category, Quality  string
	Leader, Member, Apart, Listed bool
}

// TitleEntries tells, for the movie or series of a Stremio identifier, its
// source, its type and the entries of that source's list that share its
// TMDB id, as versions of one title or kept apart, its own included, the
// group's members first; none when the source does not group its titles,
// or the title has no TMDB id. stremio.ErrNotFound reports an identifier
// of no IPTV title.
func (s *Service) TitleEntries(ctx context.Context, id string) (accounts.ID, string, []TitleEntry, error) {
	source, typ, key, ok := titleOfID(id)
	if !ok {
		return accounts.ID{}, "", nil, stremio.ErrNotFound
	}
	var tmdb string
	err := s.db.QueryRow(ctx, `SELECT coalesce((SELECT tmdb FROM iptv_title_groups WHERE addon_id = i.addon_id AND type = $2 AND leader = $3),
		(SELECT tmdb FROM iptv_titles WHERE addon_id = i.addon_id AND type = $2 AND key = $3), '')
		FROM iptv_sources i WHERE i.addon_id = $1 AND i.title_versions`, source, typ, key).Scan(&tmdb)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && tmdb == "" {
		return source, typ, []TitleEntry{}, nil
	}
	if err != nil {
		return accounts.ID{}, "", nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT key, name, category, quality, leader, member, apart, listed FROM (
		SELECT m.key, coalesce(t.name, '') AS name, coalesce(t.category, '') AS category, coalesce(t.quality, '') AS quality, g.leader = m.key AS leader,
			true AS member, false AS apart, m.listed, 0 AS rank, m.position
		FROM iptv_title_groups g JOIN iptv_title_members m ON m.addon_id = g.addon_id AND m.type = g.type AND m.tmdb = g.tmdb
		LEFT JOIN iptv_titles t ON t.addon_id = m.addon_id AND t.type = m.type AND t.key = m.key
		WHERE g.addon_id = $1 AND g.type = $2 AND g.tmdb = $3
		UNION ALL
		SELECT t.key, t.name, t.category, t.quality, false, false, a.key IS NOT NULL, true, 1, t.position
		FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id
		LEFT JOIN iptv_titles_apart a ON a.addon_id = t.addon_id AND a.type = t.type AND a.key = t.key
		WHERE t.addon_id = $1 AND t.type = $2 AND t.tmdb = $3 AND `+shownTitle+`
		AND NOT EXISTS (SELECT 1 FROM iptv_title_members m WHERE m.addon_id = t.addon_id AND m.type = t.type AND m.key = t.key AND m.tmdb = t.tmdb)
	) entries ORDER BY rank, position`, source, typ, tmdb)
	if err != nil {
		return accounts.ID{}, "", nil, err
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TitleEntry, error) {
		var e TitleEntry
		err := row.Scan(&e.Key, &e.Name, &e.Category, &e.Quality, &e.Leader, &e.Member, &e.Apart, &e.Listed)
		return e, err
	})
	if entries == nil {
		entries = []TitleEntry{}
	}
	return source, typ, entries, err
}

// titleOfID reads a source's movie or series identifier.
func titleOfID(id string) (accounts.ID, string, string, bool) {
	rest, ok := strings.CutPrefix(id, IDPrefix)
	if !ok {
		return accounts.ID{}, "", "", false
	}
	text, rest, _ := strings.Cut(rest, ":")
	source, err := accounts.ParseID(text)
	if err != nil {
		return accounts.ID{}, "", "", false
	}
	if key, ok := strings.CutPrefix(rest, "vod:"); ok && key != "" {
		return source, typeMovie, key, true
	}
	if key, ok := strings.CutPrefix(rest, "series:"); ok && key != "" {
		return source, typeSeries, key, true
	}
	return accounts.ID{}, "", "", false
}

// KeepApart keeps a source's movie or series of key apart from the others
// of its TMDB id, or lets it be grouped again, and regroups the source's
// titles at once, in one transaction that updates the source's row (see
// CatalogVersion), then tells OnChange, as new options do.
// addons.ErrNotFound reports a source of no row.
func (s *Service) KeepApart(ctx context.Context, source accounts.ID, typ, key string, apart bool) error {
	if typ != typeMovie && typ != typeSeries || key == "" {
		return ErrInvalidOptions
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE iptv_sources SET title_versions = title_versions WHERE addon_id = $1", source)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return addons.ErrNotFound
		}
		statement := "DELETE FROM iptv_titles_apart WHERE addon_id = $1 AND type = $2 AND key = $3"
		if apart {
			statement = "INSERT INTO iptv_titles_apart (addon_id, type, key) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"
		}
		if _, err := tx.Exec(ctx, statement, source, typ, key); err != nil {
			return err
		}
		options, err := loadOptions(ctx, tx, source)
		if err != nil {
			return err
		}
		return s.reconcileGroups(ctx, tx, source, options)
	})
	if err == nil {
		s.lineupChanged(ctx, source)
	}
	return err
}
