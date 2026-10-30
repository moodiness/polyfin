package library

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Smart lists (see package smartlists) draw from catalogs of the addons a
// user reaches and keep the titles that match their rules. Each catalog is
// read up to its maximum, the library's own when it is one of the user's
// libraries, else the catalog limit, from the pages kept and refreshed as
// any library's; a single genre the rules require, which the catalog offers
// in its genre filter, is asked of the addon instead of reading the whole
// catalog. The rules are applied when the list is listed, those on what the
// user did with the titles at each listing.
//
// What a catalog page gives of a title (genres, year, IMDb rating,
// runtime, content rating, people, release date) is completed by its
// description, when one is kept: the language, and the episodes of a
// series a list sorts by its latest episode, come only with it.
// Descriptions missing are read in the background, a few at a time, so
// that the list grows as they come; until then, a title whose rules depend
// on them is left out, and a series sorted by its episodes comes last. The
// votes of a title's IMDb rating come from MDBList (see ratings.go), and
// its studios from TMDB (see studios.go), the same way.

const (
	// smartWait bounds how long a listing of a list waits for the pages it
	// needs: past it, the list is judged from the pages kept, and the
	// others are read in the background for the next listing.
	smartWait = 3 * time.Second
	// smartReadTime bounds a background read of a catalog for a list.
	smartReadTime = 5 * time.Minute
	// smartLife is how long the titles read for a list serve its next
	// listings: apps read a list page after page. smartBytes bounds what
	// they take, as the catalog pages kept are bounded: the titles of a
	// list larger than that on their own are read again from the pages at
	// each listing.
	smartLife  = 30 * time.Second
	smartBytes = 64 << 20
	// smartLookups bounds the descriptions one listing of a list asks
	// for, and smartFetches the descriptions asked for at once, across
	// lists.
	smartLookups = 50
	smartFetches = 4
	// SmartPreviewTitles bounds the titles a preview lists.
	SmartPreviewTitles = 200
)

// SmartListItemID is the item of a smart list in Jellyfin apps.
func SmartListItemID(id accounts.ID) accounts.ID { return itemID("smartlist|" + id.String()) }

// smartList is a smart list a user sees, with its item: a library, or a
// collection.
type smartList struct {
	item Item
	list smartlists.List
}

func (v view) smartList(id accounts.ID) (smartList, bool) {
	for _, l := range v.smart {
		if l.item.ID == id {
			return l, true
		}
	}
	return smartList{}, false
}

// smartItem describes a list as apps see it: a collection, or a library of
// movies or series when every catalog it draws from holds them, else a
// library without type, as the Watchlist is, which apps give a home row.
func smartItem(l smartlists.List) Item {
	item := Item{ID: SmartListItemID(l.ID), Name: l.Name}
	if l.ShownAs == smartlists.ShownAsCollection {
		item.Kind = KindCollection
		return item
	}
	item.Kind = KindLibrary
	types := map[string]bool{}
	for _, c := range l.Catalogs {
		types[collectionType(c.Type)] = true
	}
	for t := range types {
		if len(types) == 1 && (t == "movies" || t == "tvshows") {
			item.CollectionType = t
		}
	}
	return item
}

// smartLists adds to v the lists user sees: the server's when the view
// holds the server's addons, but those the user may not see, as the
// server's libraries; the user's own when it holds theirs.
func (s *Service) smartLists(ctx context.Context, v *view, user accounts.User, scopes []addons.Scope) error {
	shared := slices.ContainsFunc(scopes, func(scope addons.Scope) bool { return scope.Owner == nil })
	own := slices.ContainsFunc(scopes, func(scope addons.Scope) bool { return scope.Owner != nil })
	lists, err := s.smart.Seen(ctx, user.ID, shared, own)
	if err != nil {
		return err
	}
	for _, l := range lists {
		item := smartItem(l)
		if l.Owner == nil && slices.Contains(user.HiddenLibraries, item.ID) {
			continue
		}
		v.smart = append(v.smart, smartList{item: item, list: l})
	}
	return nil
}

// SmartCollections lists the smart lists user sees as collections, in
// order.
func (s *Service) SmartCollections(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, l := range v.smart {
		if l.item.Kind == KindCollection {
			items = append(items, l.item)
		}
	}
	s.smartCollectionImages(ctx, v, items)
	return s.overridden(items), nil
}

// IsSmartList reports whether id is one of the smart lists user sees.
func (s *Service) IsSmartList(ctx context.Context, user accounts.User, id accounts.ID) (bool, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return false, err
	}
	_, ok := v.smartList(id)
	return ok, nil
}

// smartTitle is a title a list read from its catalogs: its item, the
// record that finds it again, and what is known of it, but what the user
// did with it. described is set once its description was read; genre is
// the genre its catalog was narrowed to, if any; episodes are the dates of
// a series' episodes its description gives, when a sort needs them.
type smartTitle struct {
	item      Item
	record    record
	title     smartlists.Title
	described bool
	genre     string
	episodes  episodeDates
}

// smartKey identifies the titles read for a list, as it was saved, by the
// catalogs that read them (see smartReader): users who reach the same
// catalogs the same way share them, what each did with the titles being
// applied at each listing (see smartStates).
type smartKey struct {
	list    accounts.ID
	updated time.Time
	reader  string
}

// smartReader tells apart the ways sources are read: by which addon, at
// which address and how far, narrowed to which genre. A user whose
// parental control or blocked genres hide titles reads them apart, as the
// titles they are shown differ (see visibleMetas). The titles a user hid
// tell no reader apart: they are read for every user, and left out at
// each listing (see smartMatches).
func smartReader(v view, sources []source) string {
	var b strings.Builder
	if v.restricted() {
		b.WriteString("user " + v.user.String() + "\n")
	}
	for _, src := range sources {
		fmt.Fprintf(&b, "%s %s %t %q %q %q %d\n", src.addon.addon.ID, configOf(src.addon.addon), src.addon.confined,
			src.catalog.Type, src.catalog.ID, src.genre, v.limit(src))
	}
	return b.String()
}

// smartTitleSize estimates what a title read for a list takes in memory
// besides its listing's description: its item's own copies (its people,
// its identifiers) and the structures holding them.
const smartTitleSize = 512

// smartTitlesSize is the size the smart list cache counts for the titles
// read for a list (see cache.Sized): for each, its listing's description,
// which its item and record share, twice what the page caches count for
// it, plus smartTitleSize.
func smartTitlesSize(titles []smartTitle) int {
	size := 0
	for _, t := range titles {
		size += smartTitleSize
		if t.record.Meta != nil {
			size += 2 * cache.JSONSize(*t.record.Meta)
		}
	}
	return size
}

// smartResult is a list as it stands for a user: the titles it lists, in
// order and within its maximum, how many matched before the maximum, how
// many may match once their description is read, and whether every
// catalog was read to its maximum, without waiting past smartWait.
type smartResult struct {
	titles   []smartTitle
	matched  int
	unknown  int
	complete bool
}

// smartChildren lists a smart list's titles from start. A listing that
// keeps only some kinds of items (see WithKinds) lists those alone, out of
// those alone.
func (s *Service) smartChildren(ctx context.Context, v view, l smartList, start, count int) (Page, error) {
	result, err := s.smartMatches(ctx, v, l.list)
	if err != nil {
		return Page{}, err
	}
	titles := slices.DeleteFunc(result.titles, func(t smartTitle) bool { return !keepsKind(ctx, t.item.Kind) })
	if start >= len(titles) {
		return Page{Total: len(titles)}, nil
	}
	window := titles[start:min(start+count, len(titles))]
	items := make([]Item, 0, len(window))
	records := make([]record, 0, len(window))
	parent := l.item.ID
	for _, t := range window {
		item, r := t.item, t.record
		item.ParentID, r.Parent = parent, &parent
		items, records = append(items, item), append(records, r)
	}
	return page(items, start, len(titles)), s.save(ctx, records)
}

// smartMatches judges the titles a list reads for the user of v, sorts
// those it keeps and cuts them to its maximum. Titles whose rules depend on
// a description not read yet are left out, and their descriptions read in
// the background.
func (s *Service) smartMatches(ctx context.Context, v view, list smartlists.List) (smartResult, error) {
	candidates, complete, err := s.smartCandidates(ctx, v, list)
	if err != nil {
		return smartResult{}, err
	}
	states, err := s.smartStates(ctx, v, list, candidates)
	if err != nil {
		return smartResult{}, err
	}
	if err := s.smartElsewhere(ctx, list, candidates); err != nil {
		return smartResult{}, err
	}
	result := smartResult{complete: complete}
	var undescribed []record
	today := s.now()
	for _, c := range candidates {
		// The titles read for a list are those of every user who reads its
		// catalogs the same way (see smartCandidates): the caller's hidden
		// ones leave at each listing, as their played titles do.
		if v.hidden.Hides(c.item) {
			continue
		}
		t := c.title
		t.State, t.Today = states[c.item.ID], today
		switch list.Keeps(t) {
		case smartlists.Yes:
			result.titles = append(result.titles, c)
			// A series sorted by its episodes waits for them, listed last.
			if !c.described && c.title.Series && sortsByEpisodes(list.Sort) {
				undescribed = append(undescribed, c.record)
			}
		case smartlists.Unknown:
			result.unknown++
			if !c.described {
				undescribed = append(undescribed, c.record)
			}
		}
	}
	s.describeLater(ctx, v, list.Owner == nil, undescribed)
	sortSmart(result.titles, list, v.user, s.now())
	result.matched = len(result.titles)
	if list.MaxItems > 0 && len(result.titles) > list.MaxItems {
		result.titles = result.titles[:list.MaxItems]
	}
	return result, nil
}

// smartCandidates returns the titles a list's catalogs list, one of each
// catalog in turn, each once, with what is known of them, the titles the
// user hid included: they are left out at each listing (see smartMatches),
// so that a title hidden or shown again changes the list at once. Those
// read for a saved list whole serve its listings for smartLife, for every
// user who reads its catalogs the same way (see smartKey); the
// descriptions of those not described then are looked for again at each
// listing, as they come in the background.
func (s *Service) smartCandidates(ctx context.Context, v view, list smartlists.List) ([]smartTitle, bool, error) {
	sources := s.smartSources(v, list)
	key := smartKey{list.ID, list.UpdatedAt, smartReader(v, sources)}
	cacheable := list.ID != (accounts.ID{}) && !keptOnly(ctx) && !refreshing(ctx)
	if cacheable {
		if kept, ok := s.smartCache.Get(key); ok {
			titles := slices.Clone(kept)
			return titles, true, s.smartDescribed(ctx, v, list, titles)
		}
	}
	reader := v
	reader.hidden = nil
	lists, complete := s.smartRead(ctx, reader, sources)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	titles := s.smartFacts(sources, lists)
	if err := s.smartDescribed(ctx, v, list, titles); err != nil {
		return nil, false, err
	}
	if cacheable && complete {
		// The listing completes its own copy (see smartElsewhere).
		s.smartCache.Put(key, slices.Clone(titles))
	}
	return titles, complete, nil
}

// smartSources returns the catalogs a list draws from that the user of v
// reaches: catalogs of titles, not of collections, channels or music. Each
// is read up to the maximum of the user's library of it, if it has one,
// else to the catalog limit, and narrowed to a genre the rules require
// when its genre filter offers it.
func (s *Service) smartSources(v view, list smartlists.List) []source {
	genres := list.Rules.Genres()
	var sources []source
	for _, c := range list.Catalogs {
		addon, ok := v.addon(c.AddonID)
		if !ok || addon.addon.HasMusic() {
			continue
		}
		catalog, ok := addon.addon.Manifest.Catalog(c.Type, c.ID)
		if !ok || !smartCatalog(catalog) {
			continue
		}
		src := source{addon: addon, catalog: catalog}
		for _, l := range v.libraries {
			if l.addon.addon.ID == addon.addon.ID && l.catalog.Type == catalog.Type && l.catalog.ID == catalog.ID && l.maxItems > 0 {
				src.max = l.maxItems
				break
			}
		}
		offered := addons.GenreChoices(catalog)
		for _, genre := range genres {
			if i := slices.IndexFunc(offered, func(o string) bool { return strings.EqualFold(o, genre) }); i >= 0 {
				src.genre = offered[i]
				break
			}
		}
		sources = append(sources, src)
	}
	return sources
}

// smartCatalog reports whether a list may draw from catalog: a catalog of
// titles that can be read without a search, not of collections or
// channels.
func smartCatalog(catalog stremio.Catalog) bool {
	return !LiveCatalog(catalog.Type) && catalog.Type != "collection" && catalog.Browsable()
}

// smartRead reads each source up to its maximum, a few at a time. A
// listing waits smartWait at most, then takes the pages kept of the
// catalogs that did not answer in time, and reads them on in the
// background; complete is then false. A read that refreshes (see
// ReadCollections) waits for every page.
func (s *Service) smartRead(ctx context.Context, v view, sources []source) (lists [][]stremio.Meta, complete bool) {
	lists = make([][]stremio.Meta, len(sources))
	waiting, cancel := ctx, context.CancelFunc(func() {})
	if !keptOnly(ctx) && !refreshing(ctx) {
		waiting, cancel = context.WithTimeout(ctx, s.smartWait)
	}
	defer cancel()
	var mu sync.Mutex
	complete = true
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, src := range sources {
		group.Go(func() error {
			limit := v.limit(src)
			metas, _, err := s.window(waiting, v, src, 0, limit)
			if err != nil || waiting.Err() != nil {
				if ctx.Err() != nil {
					return nil
				}
				if err != nil && waiting.Err() == nil {
					s.logger.Debug("A catalog of a smart list could not be read", "catalog", src.catalog.ID, "error", err)
				}
				metas, _, _ = s.window(withKeptOnly(ctx), v, src, 0, limit)
				mu.Lock()
				complete = false
				mu.Unlock()
				s.smartReadLater(ctx, v, src)
			}
			lists[i] = metas
			return nil
		})
	}
	_ = group.Wait()
	return lists, complete
}

// smartReadLater reads a catalog of a list up to its maximum in the
// background, once at a time, so that the next listing finds its pages
// kept.
func (s *Service) smartReadLater(ctx context.Context, v view, src source) {
	if keptOnly(ctx) {
		return
	}
	s.background(ctx, fmt.Sprintf("smart read %v", src.key(0)), func() bool { return true }, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, smartReadTime)
		defer cancel()
		_, _, err := s.window(ctx, v.withoutWait(), src, 0, v.limit(src))
		return err
	})
}

// smartFacts merges the titles of the sources' lists, one of each in turn
// without duplicates, as an addon's collection does, each at its best place
// in a catalog, and tells what their listing gives of them, with
// administrators' edits. Movies and series only are kept.
func (s *Service) smartFacts(sources []source, lists [][]stremio.Meta) []smartTitle {
	var titles []smartTitle
	seen := map[string]bool{}
	for rank := 0; ; rank++ {
		added := false
		for i, metas := range lists {
			if rank >= len(metas) {
				continue
			}
			added = true
			meta := metas[rank]
			if seen[meta.ID] {
				continue
			}
			seen[meta.ID] = true
			item, r, err := titleItem(sources[i].addon.addon.ID, sources[i].catalog, meta, accounts.ID{}, sources[i].addon.confined)
			if err != nil || item.Kind != KindMovie && item.Kind != KindSeries {
				continue
			}
			r.Parent = nil
			titles = append(titles, smartTitle{item: item, record: r, title: smartlists.Title{Rank: rank + 1}, genre: sources[i].genre})
		}
		if !added {
			break
		}
	}
	items := make([]Item, len(titles))
	for i, t := range titles {
		items[i] = t.item
	}
	items = s.overridden(items)
	for i := range titles {
		t := &titles[i]
		t.item = items[i]
		t.title.Series = t.item.Kind == KindSeries
		t.title.Genres, t.title.GenresKnown = t.item.Genres, len(t.item.Genres) > 0
		t.title.Year = t.item.ProductionYear
		t.title.Rating, t.title.Rated = t.item.CommunityRating, t.item.CommunityRating > 0
		t.title.Runtime = int(t.item.Runtime / time.Minute)
		t.title.ContentRating = t.item.OfficialRating
		t.title.People = creditedPeople(t.item.People)
		t.title.Released = releasedDay(t.item, t.record.Meta)
		t.narrow()
	}
	return titles
}

// creditedPeople lists the names of the actors and directors of people,
// whom person rules look for.
func creditedPeople(people []Person) []string {
	var names []string
	for _, p := range people {
		if p.Type == "Actor" || p.Type == "Director" {
			names = append(names, p.Name)
		}
	}
	return names
}

// releasedDay is the day a title was released, zero when only its year
// is known: its premiere date, an administrator's or its listing's, but
// the first of January its year alone stands for (see fromMeta).
func releasedDay(item Item, meta *stremio.Meta) time.Time {
	if item.PremiereDate == nil {
		return time.Time{}
	}
	date := *item.PremiereDate
	if meta != nil && exactDay(meta.Released) == nil && date.Month() == time.January && date.Day() == 1 &&
		date.Equal(date.Truncate(24*time.Hour)) {
		return time.Time{}
	}
	return date
}

// exactDay reads a release date given to the day, nil for none or a year
// alone.
func exactDay(text string) *time.Time {
	text = strings.TrimSpace(text)
	for _, layout := range []string{time.RFC3339Nano, time.DateOnly} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return new(parsed.UTC())
		}
	}
	return nil
}

// sortsByEpisodes reports whether a sort orders series by their episodes.
func sortsByEpisodes(sort string) bool {
	return sort == smartlists.SortLatestEpisode || sort == smartlists.SortSeasonPremiere
}

// narrow adds to a title read from a catalog narrowed to a genre that
// genre, which the catalog lists.
func (t *smartTitle) narrow() {
	if t.genre != "" && !slices.ContainsFunc(t.title.Genres, func(g string) bool { return strings.EqualFold(g, t.genre) }) {
		t.title.Genres = append(slices.Clip(t.title.Genres), t.genre)
	}
}

// needsDescription reports whether a list looks at what a title's listing
// does not give: its language, or a series' episodes it sorts by, which
// only its description gives, or its genres, year, rating, runtime,
// content rating, people or release date when its listing lacks them.
func needsDescription(list smartlists.List, t smartlists.Title) bool {
	return list.Uses(smartlists.FieldLanguage) || t.Series && sortsByEpisodes(list.Sort) ||
		list.Uses(smartlists.FieldGenre) && !t.GenresKnown || list.Uses(smartlists.FieldYear) && t.Year == 0 ||
		list.Uses(smartlists.FieldRating) && !t.Rated || list.Uses(smartlists.FieldRuntime) && t.Runtime == 0 ||
		list.Uses(smartlists.FieldContentRating) && t.ContentRating == "" || list.Uses(smartlists.FieldPerson) && len(t.People) == 0 ||
		list.Uses(smartlists.FieldReleased) && t.Released.IsZero()
}

// smartDescribed completes the titles whose description the rules need,
// not described yet, with the descriptions kept since.
func (s *Service) smartDescribed(ctx context.Context, v view, list smartlists.List, titles []smartTitle) error {
	var wanted []record
	var indexes []int
	for i, t := range titles {
		if !t.described && needsDescription(list, t.title) {
			wanted, indexes = append(wanted, t.record), append(indexes, i)
		}
	}
	descriptions, err := s.keptDescriptions(ctx, v, list.Owner == nil, wanted, sortsByEpisodes(list.Sort))
	if err != nil {
		return err
	}
	now := s.now()
	for _, i := range indexes {
		t := &titles[i]
		meta, ok := descriptions[metaKey{*t.record.Addon, t.record.Meta.Type, t.record.Meta.ID}]
		if !ok {
			continue
		}
		t.described, t.title.Described = true, true
		var full Item
		fromMeta(&full, meta)
		if !t.title.GenresKnown {
			t.title.Genres, t.title.GenresKnown = full.Genres, len(full.Genres) > 0
			t.narrow()
		}
		if t.title.Year == 0 {
			t.title.Year = full.ProductionYear
		}
		if !t.title.Rated {
			t.title.Rating, t.title.Rated = full.CommunityRating, full.CommunityRating > 0
		}
		if t.title.Runtime == 0 {
			t.title.Runtime = int(full.Runtime / time.Minute)
		}
		if t.title.ContentRating == "" {
			t.title.ContentRating = full.OfficialRating
		}
		if len(t.title.People) == 0 {
			t.title.People = creditedPeople(full.People)
		}
		if t.title.Released.IsZero() {
			t.title.Released = releasedDay(full, &meta)
		}
		t.title.Languages = localization.LanguageCodes(meta.Language)
		t.episodes = episodeDatesOf(meta.Videos, now)
	}
	return nil
}

// smartElsewhere completes the titles a list judges with what neither
// their listing nor their description gives, when its rules look at it:
// the votes of their IMDb rating, from MDBList, and their studios, from
// TMDB. Those not known yet are asked about in the background.
func (s *Service) smartElsewhere(ctx context.Context, list smartlists.List, titles []smartTitle) error {
	votes, studios := list.Uses(smartlists.FieldVotes), list.Uses(smartlists.FieldStudio)
	if !votes && !studios {
		return nil
	}
	items := make([]Item, len(titles))
	for i, t := range titles {
		items[i] = t.item
	}
	if votes {
		scores := s.keptScores(items)
		for i := range titles {
			t := &titles[i]
			if scores != nil && scores[i].IMDbVotes != nil {
				t.title.Votes, t.title.VotesKnown = *scores[i].IMDbVotes, true
				continue
			}
			// Without MDBList, or once it answered without votes, a title
			// has none; it is unknown while waiting for MDBList.
			r, ok := ratingsTitle(t.item)
			t.title.Votes, t.title.VotesKnown = 0, !ok || !s.ratings.Enabled() || !s.ratings.Pending(r)
		}
	}
	if studios {
		names, known, err := s.keptStudios(ctx, items)
		if err != nil {
			return err
		}
		for i := range titles {
			titles[i].title.Studios, titles[i].title.StudiosKnown = names[i], known[i]
		}
	}
	return nil
}

// keptDescriptions returns what the descriptions kept in the database of
// titles give of them that smart lists look at, by the addon that listed
// them: that addon's description, else one of another of the view's
// addons, the server's only when shared. The dates of a series' episodes
// come only when videos is set. Polyfin's own sources keep no description.
func (s *Service) keptDescriptions(ctx context.Context, v view, shared bool, titles []record, videos bool) (map[metaKey]stremio.Meta, error) {
	if len(titles) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, t.Meta.ID)
	}
	describers := make([]accounts.ID, 0, len(v.addons))
	for _, entry := range v.addons {
		// Smart lists judge the facts the addons without a language give,
		// else those given one that stand in for them (see standIn).
		if !own(entry.addon) && (!shared || entry.shared) {
			describers = append(describers, entry.addon.ID)
		}
	}
	rows, err := s.db.Query(ctx, `SELECT addon_id, meta_type, meta_id, config,
		jsonb_strip_nulls(jsonb_build_object('genres', meta->'genres', 'releaseInfo', meta->'releaseInfo', 'year', meta->'year',
			'released', meta->'released', 'imdbRating', meta->'imdbRating', 'language', meta->'language',
			'runtime', meta->'runtime', 'director', meta->'director', 'cast', meta->'cast',
			'app_extras', jsonb_build_object('cast', meta->'app_extras'->'cast', 'certification', meta->'app_extras'->'certification',
				'certificationLocal', meta->'app_extras'->'certificationLocal'),
			'videos', CASE WHEN $4 AND jsonb_typeof(meta->'videos') = 'array' THEN (SELECT jsonb_agg(jsonb_strip_nulls(
				jsonb_build_object('id', v->'id', 'season', v->'season', 'episode', v->'episode', 'released', v->'released')))
				FROM jsonb_array_elements(meta->'videos') v) END))
		FROM metas WHERE addon_id = ANY($1) AND meta_id = ANY($2) AND fetched_at > $3`, describers, ids, s.now().Add(-metaKept), videos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type kept struct {
		addon accounts.ID
		meta  stremio.Meta
	}
	found := map[[2]string][]kept{}
	for rows.Next() {
		var addon accounts.ID
		var metaType, metaID, config string
		var data []byte
		if err := rows.Scan(&addon, &metaType, &metaID, &config, &data); err != nil {
			return nil, err
		}
		entry, ok := v.addon(addon)
		if !ok || configOf(entry.addon) != config || !v.factual(entry, metaType, metaID, shared) {
			continue
		}
		var meta stremio.Meta
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		meta.Type, meta.ID = metaType, metaID
		found[[2]string{metaType, metaID}] = append(found[[2]string{metaType, metaID}], kept{addon, meta})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make(map[metaKey]stremio.Meta, len(found))
	for _, t := range titles {
		candidates := found[[2]string{t.Meta.Type, t.Meta.ID}]
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		for _, c := range candidates {
			if c.addon == *t.Addon {
				best = c
			}
		}
		result[metaKey{*t.Addon, t.Meta.Type, t.Meta.ID}] = best.meta
	}
	return result, nil
}

// describeLater reads in the background the descriptions of titles, at
// most smartLookups of them, smartFetches at a time across lists: the
// next listings know them. The server's lists, shared, ask the server's
// addons.
func (s *Service) describeLater(ctx context.Context, v view, shared bool, records []record) {
	if keptOnly(ctx) {
		return
	}
	asked := 0
	for _, r := range records {
		if asked >= smartLookups {
			return
		}
		if entry, ok := v.addon(*r.Addon); !ok || own(entry.addon) {
			continue
		}
		asked++
		s.background(ctx, fmt.Sprintf("smart describe %s %s", r.Meta.Type, r.Meta.ID), func() bool { return true }, func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, ratingLookupTime)
			defer cancel()
			select {
			case s.smartFetches <- struct{}{}:
				defer func() { <-s.smartFetches }()
			case <-ctx.Done():
				return ctx.Err()
			}
			s.describe(ctx, v.withoutWait(), r, shared)
			return nil
		})
	}
}

// smartStates tells what the user of v did with the titles a list reads,
// when its rules look at it: nothing for a request made with an API key.
// A series is played once the user played as many of its episodes as its
// description, when kept, tells are released, and started while they
// played or started some, not all; a movie is started while it has a
// resume point.
func (s *Service) smartStates(ctx context.Context, v view, list smartlists.List, titles []smartTitle) (map[accounts.ID]smartlists.State, error) {
	if !list.Uses(smartlists.FieldState) || v.user == (accounts.ID{}) || len(titles) == 0 {
		return nil, nil
	}
	ids := make([]accounts.ID, 0, len(titles))
	var series []accounts.ID
	for _, t := range titles {
		ids = append(ids, t.item.ID)
		if t.item.Kind == KindSeries {
			series = append(series, t.item.ID)
		}
	}
	data, err := s.userData.Get(ctx, v.user, ids)
	if err != nil {
		return nil, err
	}
	counts, err := s.userData.EpisodeCounts(ctx, v.user, series)
	if err != nil {
		return nil, err
	}
	states := make(map[accounts.ID]smartlists.State, len(data)+len(counts))
	for _, t := range titles {
		d, c := data[t.item.ID], counts[t.item.ID]
		state := smartlists.State{Favorite: d.Favorite, Played: d.Played}
		switch t.item.Kind {
		case KindMovie:
			state.Started = !d.Played && d.Position > 0
		case KindSeries:
			if !state.Played && c.Played > 0 {
				if meta, _, ok := s.describedMeta(ctx, v, t.record, false, inDatabase); ok {
					released := contents(meta, nil, s.now()).Released
					state.Played = released > 0 && c.Played >= released
				}
			}
			state.Started = !state.Played && (c.Played > 0 || c.Resumable > 0)
		}
		if state != (smartlists.State{}) {
			states[t.item.ID] = state
		}
	}
	return states, nil
}

// sortSmart orders the titles a list keeps as it says: as its catalogs
// list them, merged; by premiere date, the latest first; by the latest
// regular episode aired, or season premiere aired, the latest first, a
// movie by its premiere date and a series whose episodes are not known
// last; by IMDb rating, the highest first, those without one last; by
// name; or at random, in an order drawn again each day, the same for every
// page of a listing.
func sortSmart(titles []smartTitle, list smartlists.List, user accounts.ID, now time.Time) {
	switch list.Sort {
	case smartlists.SortPremiere:
		slices.SortStableFunc(titles, func(a, b smartTitle) int { return releaseOf(b.item).Compare(releaseOf(a.item)) })
	case smartlists.SortLatestEpisode, smartlists.SortSeasonPremiere:
		latest := func(t smartTitle) time.Time {
			switch {
			case !t.title.Series:
				return releaseOf(t.item)
			case list.Sort == smartlists.SortLatestEpisode:
				return t.episodes.latest
			}
			return t.episodes.premiere
		}
		slices.SortStableFunc(titles, func(a, b smartTitle) int { return latest(b).Compare(latest(a)) })
	case smartlists.SortRating:
		slices.SortStableFunc(titles, func(a, b smartTitle) int { return cmp.Compare(b.item.CommunityRating, a.item.CommunityRating) })
	case smartlists.SortName:
		name := func(item Item) string { return strings.ToLower(cmp.Or(item.SortName, item.Name)) }
		slices.SortStableFunc(titles, func(a, b smartTitle) int { return strings.Compare(name(a.item), name(b.item)) })
	case smartlists.SortRandom:
		seed := fnv.New64a()
		_, _ = seed.Write([]byte(list.ID.String() + user.String() + now.Format(time.DateOnly)))
		random := rand.New(rand.NewPCG(seed.Sum64(), 0))
		random.Shuffle(len(titles), func(i, j int) { titles[i], titles[j] = titles[j], titles[i] })
	}
}

// smartView is the view a list is judged in for user: the user's, or for
// one of the server's lists previewed, the server's addons, with what the
// user did with the titles and those they hid.
func (s *Service) smartView(ctx context.Context, user accounts.User, shared bool) (view, error) {
	if !shared {
		return s.view(ctx, user)
	}
	v, err := s.buildView(ctx, accounts.User{})
	if err != nil {
		return view{}, err
	}
	v.user = user.ID
	v.hidden, err = s.hiddenTitles(ctx, user.ID)
	return v, err
}

// SmartPreview is a list as it stands for a user, before it is saved:
// its first titles, with the languages known of each, how many it lists
// within its maximum, how many may match once their description is read,
// and whether every catalog was read to its maximum in time.
type SmartPreview struct {
	Titles    []Item
	Languages [][]string
	Listed    int
	Unknown   int
	Complete  bool
}

// PreviewSmartList judges a list for user, as their apps would list it
// now: one of the server's lists among the server's addons, a user's own
// among theirs.
func (s *Service) PreviewSmartList(ctx context.Context, user accounts.User, list smartlists.List) (SmartPreview, error) {
	v, err := s.smartView(ctx, user, list.Owner == nil)
	if err != nil {
		return SmartPreview{}, err
	}
	result, err := s.smartMatches(ctx, v, list)
	if err != nil {
		return SmartPreview{}, err
	}
	preview := SmartPreview{Listed: len(result.titles), Unknown: result.unknown, Complete: result.complete}
	for _, t := range result.titles[:min(len(result.titles), SmartPreviewTitles)] {
		preview.Titles = append(preview.Titles, t.item)
		preview.Languages = append(preview.Languages, t.title.Languages)
	}
	return preview, nil
}

// SmartCatalog is a catalog a list may draw from, of one of the addons of
// a scope's lists: the server's addons for the server's lists, the user's
// own and the server's they use for theirs. Genres are those its genre
// filter offers.
type SmartCatalog struct {
	AddonID   accounts.ID
	AddonName string
	Shared    bool
	Catalog   stremio.Catalog
	Genres    []string
}

// SmartCatalogs lists the catalogs user's lists of a scope may draw from,
// by addon in order.
func (s *Service) SmartCatalogs(ctx context.Context, user accounts.User, shared bool) ([]SmartCatalog, error) {
	v, err := s.smartView(ctx, user, shared)
	if err != nil {
		return nil, err
	}
	var result []SmartCatalog
	for _, entry := range v.addons {
		if entry.addon.HasMusic() {
			continue
		}
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if smartCatalog(catalog) {
				result = append(result, SmartCatalog{AddonID: entry.addon.ID, AddonName: entry.addon.Manifest.Name, Shared: entry.shared,
					Catalog: catalog, Genres: slices.Clone(addons.GenreChoices(catalog))})
			}
		}
	}
	return result, nil
}

// readSmartLists reads the server's lists whole, as ReadCollections reads
// the collections, so that they open at once; it returns how many it read
// and how many failed.
func (s *Service) readSmartLists(ctx context.Context, v view) (read, failed int, err error) {
	for _, l := range v.smart {
		if _, err := s.smartMatches(ctx, v, l.list); err != nil {
			if ctx.Err() != nil {
				return read, failed, ctx.Err()
			}
			s.logger.Debug("A smart list could not be read", "list", l.item.Name, "error", err)
			failed++
			continue
		}
		read++
	}
	return read, failed, nil
}
