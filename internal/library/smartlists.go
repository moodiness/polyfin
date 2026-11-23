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
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Smart lists (see package smartlists) draw from catalogs of the addons a
// user reaches, of their IPTV sources' movies and series, and of their
// local folders and network shares, and keep the titles that match their
// rules. Each addon's catalog is read up to its maximum, the library's own
// when it is one of the user's libraries, else the catalog limit, from the
// pages kept and refreshed as any library's; a single genre the rules
// require, which the catalog offers in its genre filter, is asked of the
// addon instead of reading the whole catalog. Polyfin's own sources store
// their titles: each of their catalogs is read whole in one query (see
// smartStored). The rules are applied when the list is listed, those on
// what the user did with the titles at each listing.
//
// What a catalog page gives of a title (genres, year, IMDb rating,
// runtime, content rating, people, release date) is completed by its
// description, when one is kept: the language, the countries, and the
// episodes of a series a list sorts by its latest episode, come only with
// it. A local folder's title is described by the addons that describe its
// identifier, an IPTV source's by those that describe its IMDb identifier;
// one of neither has its listing alone. Descriptions missing are read in
// the background, a few at a time, so that the list grows as they come;
// until then, a title whose rules depend on them is left out, and a series
// sorted by its episodes comes last. The votes of a title's IMDb rating
// and its critics' and Metacritic scores come from MDBList (see
// ratings.go), and its studios from TMDB (see studios.go), the same way.
// Its tags are an administrator's, and whether a local folder holds a file
// of it is asked of the folders in one query (see smartLocalFiles).

const (
	// smartWait bounds how long a listing of a list waits for the pages it
	// needs: past it, the list is judged from the pages kept, and the
	// others are read in the background for the next listing.
	smartWait = 3 * time.Second
	// smartReadTime bounds a background read of a catalog for a list.
	smartReadTime = 5 * time.Minute
	// smartBytes bounds what the titles kept for lists take (see
	// smartEntry), as the catalog pages kept are bounded: the titles of a
	// list larger than that on their own are read again from the pages at
	// each listing.
	smartBytes = 128 << 20
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
// collection. search is set for the collection of a search in plain words
// (see plainsearch.go), which is neither in the Collections view nor given
// an image.
type smartList struct {
	item   Item
	list   smartlists.List
	search bool
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
// library without type, which apps give a home row.
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
		if l.item.Kind == KindCollection && !l.search {
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
// a series' episodes its description gives, when a sort needs them; held
// is set for a movie the wait for its digital release holds back.
type smartTitle struct {
	item      Item
	record    record
	title     smartlists.Title
	described bool
	genre     string
	episodes  episodeDates
	held      bool
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
// which address and how far, narrowed to which genre. A listing that
// leaves titles out by their rating or genres reads them apart (see
// visibleMetas): a user whose parental control or blocked genres hide
// titles, as the titles they are shown differ, and a listing narrowed by
// its titles' ratings, by its rating filter. The titles a user hid tell no
// reader apart: they are read for every user, and left out at each
// listing (see smartMatches).
func smartReader(v view, sources []source) string {
	var b strings.Builder
	if v.restricted() {
		b.WriteString("user " + v.user.String() + "\n")
	}
	if v.query.active {
		b.WriteString("rating " + v.query.signature() + "\n")
	}
	for _, src := range sources {
		fmt.Fprintf(&b, "%s %s %t %q %q %q %d\n", src.addon.addon.ID, configOf(src.addon.addon), src.addon.confined,
			src.catalog.Type, src.catalog.ID, src.genre, v.limit(src))
	}
	return b.String()
}

// signature tells apart the rating filters that keep different titles,
// for smartReader.
func (q ratingQuery) signature() string {
	score := func(s *localization.Score) string {
		if s == nil {
			return "-"
		}
		return fmt.Sprintf("%d.%d", s.Score, subScore(*s))
	}
	has := "-"
	if q.hasOfficial != nil {
		has = fmt.Sprint(*q.hasOfficial)
	}
	return fmt.Sprintf("%s %s %t %s %q", score(q.min), score(q.max), q.unratedOnly, has, q.officials)
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
	now := s.now()
	for _, t := range window {
		item, r := t.item, t.record
		item.ParentID, r.Parent = parent, &parent
		// A movie to come shows when it is out at home.
		if out := t.title.HomeRelease; !t.title.Series && t.title.HomeReleaseKnown && out.After(now) {
			item.PremiereDate = &out
			homeReleaseNote(&item, v.words, out)
		}
		items, records = append(items, item), append(records, r)
	}
	return page(items, start, len(titles)), s.save(ctx, records)
}

// smartMatches judges the titles a list reads for the user of v, sorts
// those it keeps and cuts them to its maximum. Titles whose rules depend on
// a description not read yet are left out, and their descriptions read in
// the background. Movies waiting for their digital release are left out
// but by lists of upcoming titles (see smartlists.Rule.Upcoming).
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
	if err := s.smartReleases(ctx, v, list, candidates); err != nil {
		return smartResult{}, err
	}
	if err := s.smartLocalFiles(ctx, v, list, candidates); err != nil {
		return smartResult{}, err
	}
	upcoming := list.Rules.Upcoming()
	result := smartResult{complete: complete}
	var undescribed []record
	today := s.now()
	for _, c := range candidates {
		// The titles read for a list are those of every user who reads its
		// catalogs the same way (see smartCandidates): the caller's hidden
		// ones leave at each listing, as their played titles do.
		if v.hidden.Hides(c.item) || c.held && !upcoming {
			continue
		}
		c.title.State, c.title.Today = states[c.item.ID], today
		switch list.Keeps(c.title) {
		case smartlists.Yes:
			result.titles = append(result.titles, c)
			// A series sorted by its episodes waits for them, listed last.
			if !c.described && c.title.Series && sortsByEpisodes(list.Sort) {
				undescribed = append(undescribed, v.describedAs(c.record))
			}
		case smartlists.Unknown:
			result.unknown++
			if !c.described {
				undescribed = append(undescribed, v.describedAs(c.record))
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

// smartEntry is what a list's catalogs gave, kept for its next listings
// (see smartCandidates): each catalog's titles as read, at which version
// of its pages (see catalogVersions), and whether pages it lacked left it
// short; at is when every catalog was last read. The titles are merged
// from them again at each listing, which takes a fraction of reading
// them, and keeps an administrator's edits current.
type smartEntry struct {
	lists    [][]stremio.Meta
	versions []uint64
	missing  []bool
	at       time.Time
}

// smartEntrySize is the memory entry holds, a size for the cache's bound
// (see cache.MemorySize), its strings included: they outlive the pages
// they were read from.
func smartEntrySize(entry smartEntry) int {
	size := 0
	for _, metas := range entry.lists {
		size += cache.MemorySize(metas)
	}
	return size
}

// newSmartCache returns the cache of the titles read for lists, bounded
// to maxBytes. Its entries last as long as the pages kept: what reads
// them again is a change of their catalogs (see smartCandidates).
func (s *Service) newSmartCache(maxBytes int) *cache.Cache[smartKey, smartEntry] {
	return cache.NewLasting[smartKey, smartEntry](200, s.keptCatalogLife, func() time.Time { return s.now() }).Sized(maxBytes, smartEntrySize)
}

// smartCandidates returns the titles a list's catalogs list, one of each
// catalog in turn, each once, with what is known of them, the titles the
// user hid included: they are left out at each listing (see smartMatches),
// so that a title hidden or shown again changes the list at once.
//
// Those read for a saved list serve its next listings, for every user who
// reads its catalogs the same way (see smartKey), as long as its catalogs'
// pages stay as they were read (see catalogVersions), at most until they
// are past their refresh age: the list's rules edited, or a catalog
// added, removed or read to another maximum, make another key. A catalog
// whose pages changed since, or which lacked pages, is read again alone,
// and the titles merged again. Past the refresh age, every catalog is
// read again, which refreshes their pages in the background. A list whose
// catalogs lacked pages is not complete. A read that takes kept pages
// only (see withKeptOnly) takes the titles kept whatever their age, and
// keeps none; one that refreshes neither takes nor keeps any. The
// descriptions of the titles not described then are looked for again at
// each listing, as they come in the background.
func (s *Service) smartCandidates(ctx context.Context, v view, list smartlists.List) ([]smartTitle, bool, error) {
	sources := s.smartSources(v, list)
	key := smartKey{list.ID, list.UpdatedAt, smartReader(v, sources)}
	saved := list.ID != (accounts.ID{}) && !refreshing(ctx)
	var kept smartEntry
	found := false
	if saved {
		kept, found = s.smartCache.Get(key)
	}
	if found && keptOnly(ctx) {
		titles := s.smartFacts(sources, kept.lists, v.country)
		return titles, !slices.Contains(kept.missing, true), s.smartDescribed(ctx, v, list, titles)
	}
	read := make([]bool, len(sources))
	whole := !found || s.now().Sub(kept.at) >= s.catalogLife()
	for i, src := range sources {
		read[i] = whole || kept.missing[i] || kept.versions[i] != s.smartVersion(src)
	}
	if found && !slices.Contains(read, true) {
		titles := s.smartFacts(sources, kept.lists, v.country)
		return titles, true, s.smartDescribed(ctx, v, list, titles)
	}
	reader := v
	reader.hidden = nil
	// Movies waiting for their digital release are read too: each listing
	// tells whether the list keeps them (see smartMatches).
	reader.upcoming = true
	lists, versions, missing := s.smartRead(ctx, reader, sources, read)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	at := s.now()
	if !whole {
		at = kept.at
		for i := range sources {
			if !read[i] {
				lists[i], versions[i] = kept.lists[i], kept.versions[i]
			}
		}
	}
	titles := s.smartFacts(sources, lists, v.country)
	if err := s.smartDescribed(ctx, v, list, titles); err != nil {
		return nil, false, err
	}
	if saved && !keptOnly(ctx) {
		s.smartCache.Put(key, smartEntry{lists: lists, versions: versions, missing: missing, at: at})
	}
	return titles, !slices.Contains(missing, true), nil
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
// titles that can be read without a search, not of collections, channels
// or home videos.
func smartCatalog(catalog stremio.Catalog) bool {
	return !LiveCatalog(catalog.Type) && catalog.Type != "collection" && catalog.Type != VideoType && catalog.Browsable()
}

// smartVersion is the version of a source's titles a list reads (see
// smartCandidates): its catalog's pages' (see catalogVersions), or for
// Polyfin's own sources, which keep no pages, the count of the changes to
// what they store, so that a list follows a scan or a refresh at once.
func (s *Service) smartVersion(src source) uint64 {
	switch {
	case src.addon.addon.IPTV():
		return s.storedChanges.Load()
	case src.addon.addon.Local() && s.local != nil:
		return s.local.Changes()
	}
	return s.catalogVersions.of(src)
}

// smartStored reads a catalog of Polyfin's own sources up to its maximum,
// as stored, in one query: an IPTV source's movies or series, or a local
// folder's titles. ok is false for an addon's catalog, and for a view
// that narrows its listings (see view.narrows), which reads it page by
// page to leave titles out.
func (s *Service) smartStored(ctx context.Context, v view, src source) (metas []stremio.Meta, ok bool, err error) {
	switch a := src.addon.addon; {
	case v.narrows() || src.date != "" || src.search != "":
		return nil, false, nil
	case a.IPTV() && s.iptv != nil && !LiveCatalog(src.catalog.Type):
		metas, _, err = s.iptv.CatalogWindow(ctx, a.ID, src.catalog.Type, src.catalog.ID, src.genre, "", 0, v.limit(src))
	case a.Local() && s.local != nil:
		metas, err = s.local.CatalogWindow(ctx, a.ID, src.catalog.Type, src.catalog.ID, 0, v.limit(src))
	default:
		return nil, false, nil
	}
	return metas, true, err
}

// smartRead reads each source that read marks up to its maximum, a few at
// a time, and tells the version of its titles once read (see
// smartVersion): a page its read fetched counts in it, and one that
// changes later has the next listing read it again. The others' lists
// stay nil. A listing waits smartWait at most, then takes the pages kept
// of the catalogs that did not answer in time, and reads them on in the
// background: missing marks them. A read that refreshes (see
// ReadCollections) waits for every page. Polyfin's own sources are read
// in one query each (see smartStored).
func (s *Service) smartRead(ctx context.Context, v view, sources []source, read []bool) (lists [][]stremio.Meta, versions []uint64, missing []bool) {
	lists = make([][]stremio.Meta, len(sources))
	versions = make([]uint64, len(sources))
	missing = make([]bool, len(sources))
	waiting, cancel := ctx, context.CancelFunc(func() {})
	if !keptOnly(ctx) && !refreshing(ctx) {
		waiting, cancel = context.WithTimeout(ctx, s.smartWait)
	}
	defer cancel()
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, src := range sources {
		if !read[i] {
			continue
		}
		group.Go(func() error {
			// The version is taken before the read: a change while it
			// reads has the next listing read again.
			version := s.smartVersion(src)
			metas, stored, err := s.smartStored(waiting, v, src)
			if stored {
				if err != nil && ctx.Err() == nil {
					s.logger.Debug("A stored catalog of a smart list could not be read", "catalog", src.catalog.ID, "error", err)
					missing[i] = true
				}
				lists[i], versions[i] = metas, version
				return nil
			}
			limit := v.limit(src)
			metas, _, err = s.window(waiting, v, src, 0, limit)
			if err != nil || waiting.Err() != nil {
				if ctx.Err() != nil {
					return nil
				}
				if err != nil && waiting.Err() == nil {
					s.logger.Debug("A catalog of a smart list could not be read", "catalog", src.catalog.ID, "error", err)
				}
				metas, _, _ = s.window(withKeptOnly(ctx), v, src, 0, limit)
				missing[i] = true
				s.smartReadLater(ctx, v, src)
			}
			lists[i], versions[i] = metas, s.catalogVersions.of(src)
			return nil
		})
	}
	_ = group.Wait()
	return lists, versions, missing
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
// administrators' edits, their tags among them, rated as country rates
// them; a local folder's titles have a local file. Movies and series only
// are kept.
func (s *Service) smartFacts(sources []source, lists [][]stremio.Meta, country string) []smartTitle {
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
			item, r, err := s.titleItem(sources[i].addon, sources[i].catalog, meta, accounts.ID{}, country)
			if err != nil || item.Kind != KindMovie && item.Kind != KindSeries {
				continue
			}
			r.Parent = nil
			titles = append(titles, smartTitle{item: item, record: r, genre: sources[i].genre,
				title: smartlists.Title{Rank: rank + 1, RatingCountry: country, LocalFile: sources[i].addon.addon.Local()}})
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
		t.title.Tags = t.item.Tags
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
// does not give: its language or its countries, or a series' episodes it
// sorts by, which only its description gives, or its genres, year,
// rating, runtime, content rating, people or release date when its
// listing lacks them, a series' release date standing for its home
// release.
func needsDescription(list smartlists.List, t smartlists.Title) bool {
	return list.Uses(smartlists.FieldLanguage) || list.Uses(smartlists.FieldCountry) || t.Series && sortsByEpisodes(list.Sort) ||
		list.Uses(smartlists.FieldGenre) && !t.GenresKnown || list.Uses(smartlists.FieldYear) && t.Year == 0 ||
		list.Uses(smartlists.FieldRating) && !t.Rated || list.Uses(smartlists.FieldRuntime) && t.Runtime == 0 ||
		list.Uses(smartlists.FieldContentRating) && t.ContentRating == "" || list.Uses(smartlists.FieldPerson) && len(t.People) == 0 ||
		(list.Uses(smartlists.FieldReleased) || t.Series && list.Uses(smartlists.FieldHomeRelease)) && t.Released.IsZero()
}

// smartDescribed completes the titles whose description the rules need,
// not described yet, with the descriptions kept since. A title no addon
// describes (see view.describable) is described by its listing alone.
func (s *Service) smartDescribed(ctx context.Context, v view, list smartlists.List, titles []smartTitle) error {
	var wanted []record
	var indexes []int
	for i, t := range titles {
		if t.described || !needsDescription(list, t.title) {
			continue
		}
		if !v.describable(t.record) {
			titles[i].described, titles[i].title.Described = true, true
			continue
		}
		wanted, indexes = append(wanted, v.describedAs(t.record)), append(indexes, i)
	}
	descriptions, err := s.keptDescriptions(ctx, v, list.Owner == nil, wanted, sortsByEpisodes(list.Sort))
	if err != nil {
		return err
	}
	now := s.now()
	for n, i := range indexes {
		t := &titles[i]
		meta, ok := descriptions[metaKey{*wanted[n].Addon, wanted[n].Meta.Type, wanted[n].Meta.ID}]
		if !ok {
			continue
		}
		t.described, t.title.Described = true, true
		full := Item{ID: t.record.ID, Kind: t.record.Kind}
		s.fromMeta(&full, meta, v.country)
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
		t.title.Countries = countryCodes(meta.Country)
		t.episodes = episodeDatesOf(meta.Videos, now)
	}
	return nil
}

// smartElsewhere completes the titles a list judges with what neither
// their listing nor their description gives, when its rules look at it:
// the votes of their IMDb rating and their critics' and Metacritic
// scores, from MDBList, and their studios, from TMDB. Those not known yet
// are asked about in the background.
func (s *Service) smartElsewhere(ctx context.Context, list smartlists.List, titles []smartTitle) error {
	votes, studios := list.Uses(smartlists.FieldVotes), list.Uses(smartlists.FieldStudio)
	critics := list.Uses(smartlists.FieldCritics) || list.Uses(smartlists.FieldMetacritic)
	if !votes && !critics && !studios {
		return nil
	}
	items := make([]Item, len(titles))
	for i, t := range titles {
		items[i] = t.item
	}
	if votes || critics {
		scores := s.keptScores(items)
		// Without MDBList, or once it answered without a score, a title
		// has none; it is unknown while waiting for MDBList.
		answered := func(item Item) bool {
			r, ok := ratingsTitle(item)
			return !ok || !s.ratings.Enabled() || !s.ratings.Pending(r)
		}
		for i := range titles {
			t := &titles[i]
			var score ratings.Scores
			if scores != nil {
				score = scores[i]
			}
			if votes {
				t.title.Votes, t.title.VotesKnown = 0, score.IMDbVotes != nil || answered(t.item)
				if score.IMDbVotes != nil {
					t.title.Votes = *score.IMDbVotes
				}
			}
			if critics {
				t.title.CriticsKnown = score.Critics != nil || score.Metacritic != nil || answered(t.item)
				t.title.Critics, t.title.Metacritic = scoreOf(score.Critics), scoreOf(score.Metacritic)
			}
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

// scoreOf is a score MDBList gives, 0 for none.
func scoreOf(score *float64) float64 {
	if score == nil {
		return 0
	}
	return *score
}

// smartReleases completes the movies a list judges with the day they are
// out at home, from TMDB, when its rules look at it, and marks those the
// wait for their digital release holds back for the user of v (see
// releases.go). Those not known, or known for too long, are asked about in
// the background: a movie not known yet is held back, and unknown to the
// rules. Without TMDB key or identifier, a movie's home release cannot be
// known: it has none.
func (s *Service) smartReleases(ctx context.Context, v view, list smartlists.List, titles []smartTitle) error {
	rule := list.Uses(smartlists.FieldHomeRelease)
	if !rule && !v.waitRelease {
		return nil
	}
	enabled := s.tmdb.Enabled()
	keys := make([][]releaseKey, len(titles))
	var all []releaseKey
	for i, t := range titles {
		if t.item.Kind == KindMovie && t.record.Meta != nil && enabled {
			keys[i] = releaseKeys(s.providerIDs(t.record.ID, *t.record.Meta))
			all = append(all, keys[i]...)
		}
	}
	kept, err := s.keptReleases(ctx, all)
	if err != nil {
		return err
	}
	now := s.now()
	var due []releaseKey
	for i := range titles {
		t := &titles[i]
		if t.item.Kind != KindMovie {
			continue
		}
		known, ok := keptRelease(kept, keys[i])
		waits := len(keys[i]) > 0 && s.waits(v, t.record)
		if len(keys[i]) > 0 && (rule || waits) && (!ok || known.due(now)) {
			due = append(due, keys[i][0])
		}
		if rule && (len(keys[i]) == 0 || ok) {
			t.title.HomeRelease, t.title.HomeReleaseKnown = known.available, true
		}
		t.held = waits && (!ok || known.found && known.waiting(now))
	}
	s.releasesLater(ctx, due, smartLookups)
	return nil
}

// keptDescriptions returns what the descriptions kept in the database of
// titles give of them that smart lists look at, by the addon that listed
// them: that addon's description, else one of another of the view's
// addons, the server's only when shared. The dates of a series' episodes
// come only when videos is set. Polyfin's own sources keep no description:
// their titles are found by the identifier other addons describe them by
// (see view.describedAs).
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
			'released', meta->'released', 'imdbRating', meta->'imdbRating', 'language', meta->'language', 'country', meta->'country',
			'runtime', meta->'runtime', 'director', meta->'director', 'cast', meta->'cast',
			'app_extras', jsonb_build_object('cast', meta->'app_extras'->'cast', 'certification', meta->'app_extras'->'certification',
				'certificationLocal', meta->'app_extras'->'certificationLocal', 'certifications', meta->'app_extras'->'certifications'),
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

// describedAs is the record of a title that the descriptions of addons
// are looked for by: its own, but for an IPTV source's title, the record
// of its IMDb identifier, of none for a title without one.
func (v view) describedAs(r record) record {
	if entry, ok := v.addon(*r.Addon); ok && entry.addon.IPTV() {
		r.Meta = &stremio.Meta{Type: r.Meta.Type, ID: r.Meta.ImdbID}
	}
	return r
}

// describable reports whether an addon may describe a title: an IPTV
// source's title without IMDb identifier, and a local folder's plain
// video, are described by their listing alone.
func (v view) describable(r record) bool {
	entry, ok := v.addon(*r.Addon)
	switch {
	case !ok:
		return true
	case entry.addon.IPTV():
		return r.Meta.ImdbID != ""
	case entry.addon.Local():
		return !strings.HasPrefix(r.Meta.ID, PlainIDPrefix)
	}
	return true
}

// countryAliases are the names descriptions give countries that are not
// their English short names (see localization.Countries), in lower case.
var countryAliases = map[string]string{
	"usa": "US", "united states of america": "US", "uk": "GB", "great britain": "GB", "england": "GB",
	"czech republic": "CZ", "hong kong": "HK", "macau": "MO", "macao": "MO", "south korea": "KR", "korea": "KR",
	"republic of korea": "KR", "korea, republic of": "KR", "russian federation": "RU", "turkey": "TR", "west germany": "DE",
	"iran, islamic republic of": "IR", "viet nam": "VN", "ivory coast": "CI", "the netherlands": "NL", "holland": "NL",
	"taiwan, province of china": "TW", "palestine": "PS",
}

// countryCodes reads the countries a description names, by their English
// names, their usual other names or their ISO 3166-1 codes, as their
// alpha-2 codes, each once; those it does not know are left out.
func countryCodes(names []string) []string {
	var codes []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		code, ok := countryAliases[strings.ToLower(name)]
		if !ok {
			for _, c := range localization.Countries() {
				if strings.EqualFold(c.Name, name) || strings.EqualFold(c.Alpha2, name) || strings.EqualFold(c.Alpha3, name) {
					code, ok = c.Alpha2, true
					break
				}
			}
		}
		if ok && !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	return codes
}

// describeLater reads in the background the descriptions of titles, at
// most smartLookups of them, smartFetches at a time across lists: the
// next listings know them. The server's lists, shared, ask the server's
// addons. A title of Polyfin's own sources, by the identifier addons
// describe it by (see view.describedAs), is described by the other
// addons that describe it (see describeElsewhere).
func (s *Service) describeLater(ctx context.Context, v view, shared bool, records []record) {
	if keptOnly(ctx) {
		return
	}
	asked := 0
	for _, r := range records {
		if asked >= smartLookups {
			return
		}
		entry, ok := v.addon(*r.Addon)
		if !ok || r.Meta.ID == "" {
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
			if own(entry.addon) {
				s.describeElsewhere(ctx, v.withoutWait(), r, shared)
				return nil
			}
			s.describe(ctx, v.withoutWait(), r, shared)
			return nil
		})
	}
}

// describeElsewhere has the first of the view's addons, but Polyfin's own
// sources, that describes a title of those sources describe it, by the
// identifier of r, so that its description is kept (see keptMeta).
func (s *Service) describeElsewhere(ctx context.Context, v view, r record, shared bool) {
	elsewhere := r
	elsewhere.Addon = nil
	for _, candidate := range describers(v, elsewhere, shared) {
		if own(candidate.addon) {
			continue
		}
		if _, err := s.keptMeta(ctx, candidate, r.Meta.Type, r.Meta.ID); err == nil || ctx.Err() != nil {
			return
		}
	}
}

// smartStates tells what the user of v did with the titles a list reads,
// when its rules look at it, or when it sorts by the date they last played
// them: nothing for a request made with an API key. A series is played
// once the user played as many of its episodes as its description, when
// kept, tells are released, and started while they played or started
// some, not all; a movie is started while it has a resume point. A title
// is on the user's watchlist as watchlisted tells. A series was last
// played when the user last played one of its episodes through.
func (s *Service) smartStates(ctx context.Context, v view, list smartlists.List, titles []smartTitle) (map[accounts.ID]smartlists.State, error) {
	rules, played := list.Uses(smartlists.FieldState), list.Sort == smartlists.SortDatePlayed
	if !rules && !played || v.user == (accounts.ID{}) || len(titles) == 0 {
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
	var counts map[accounts.ID]userdata.Counts
	if rules {
		if counts, err = s.userData.EpisodeCounts(ctx, v.user, series); err != nil {
			return nil, err
		}
	}
	var lastPlayed map[accounts.ID]time.Time
	if played {
		if lastPlayed, err = s.userData.SeriesLastPlayed(ctx, v.user, series); err != nil {
			return nil, err
		}
	}
	var watchlisted map[accounts.ID]bool
	if list.UsesState(smartlists.StateWatchlist) {
		if watchlisted, err = s.watchlisted(ctx, v.user); err != nil {
			return nil, err
		}
	}
	states := make(map[accounts.ID]smartlists.State, len(data)+len(counts)+len(watchlisted))
	for _, t := range titles {
		d, c := data[t.item.ID], counts[t.item.ID]
		state := smartlists.State{Favorite: d.Favorite, Played: d.Played, Watchlist: watchlisted[t.item.ID]}
		switch t.item.Kind {
		case KindMovie:
			state.Started = !d.Played && d.Position > 0
			if d.LastPlayed != nil {
				state.LastPlayed = *d.LastPlayed
			}
		case KindSeries:
			if rules && !state.Played && c.Played > 0 {
				if meta, _, ok := s.describedMeta(ctx, v, t.record, false, inDatabase); ok {
					released := contents(meta, nil, s.now()).Released
					state.Played = released > 0 && c.Played >= released
				}
			}
			state.Started = !state.Played && (c.Played > 0 || c.Resumable > 0)
			state.LastPlayed = lastPlayed[t.item.ID]
		}
		if state != (smartlists.State{}) {
			states[t.item.ID] = state
		}
	}
	return states, nil
}

// watchlisted tells the titles on user's watchlist: its movies, the
// series of its episodes, and the titles waiting for their entries (see
// watchlist.Store.Pending).
func (s *Service) watchlisted(ctx context.Context, user accounts.ID) (map[accounts.ID]bool, error) {
	held, err := s.watchlist.Held(ctx, user)
	if err != nil {
		return nil, err
	}
	pending, err := s.watchlist.Pending(ctx, user)
	if err != nil {
		return nil, err
	}
	on := make(map[accounts.ID]bool, len(held)+len(pending))
	for _, h := range held {
		on[h.ID] = true
		if h.Series != (accounts.ID{}) {
			on[h.Series] = true
		}
	}
	for _, p := range pending {
		on[p.ID] = true
	}
	return on, nil
}

// smartLocalFiles marks the titles a list judges that a local folder or a
// network share of the view holds a file of, when its rules look at it,
// in one query: those a folder listed do; the others are looked for by
// the identifier addons describe them by (see view.describedAs).
func (s *Service) smartLocalFiles(ctx context.Context, v view, list smartlists.List, titles []smartTitle) error {
	if !list.Uses(smartlists.FieldLocalFile) || s.local == nil {
		return nil
	}
	var folders []accounts.ID
	for _, entry := range v.addons {
		if entry.addon.Local() && !entry.addon.HasMusic() {
			folders = append(folders, entry.addon.ID)
		}
	}
	keys := make([]string, len(titles))
	var ids []string
	for i, t := range titles {
		if t.title.LocalFile {
			continue
		}
		if keys[i] = v.describedAs(t.record).Meta.ID; keys[i] != "" {
			ids = append(ids, keys[i])
		}
	}
	if len(folders) == 0 || len(ids) == 0 {
		return nil
	}
	held, err := s.local.HeldTitles(ctx, folders, ids)
	if err != nil {
		return err
	}
	for i := range titles {
		if keys[i] != "" && held[keys[i]] {
			titles[i].title.LocalFile = true
		}
	}
	return nil
}

// sortSmart orders the titles a list keeps as it says: as its catalogs
// list them, merged; by premiere date, the latest first; by the latest
// regular episode aired, or season premiere aired, the latest first, a
// movie by its premiere date and a series whose episodes are not known
// last; by IMDb rating, the highest first, those without one last; by
// name; at random, in an order drawn again each day, the same for every
// page of a listing; or by the date the user last played them, the latest
// first, those never played last, as their catalogs list them.
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
	case smartlists.SortDatePlayed:
		slices.SortStableFunc(titles, func(a, b smartTitle) int { return b.title.State.LastPlayed.Compare(a.title.State.LastPlayed) })
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

// What a catalog a list may draw from belongs to: an addon, a local folder
// or network share, or an IPTV source.
const (
	SmartSourceAddon  = "addon"
	SmartSourceFolder = "folder"
	SmartSourceIPTV   = "iptv"
)

// SmartCatalog is a catalog a list may draw from, of one of the addons or
// own sources of a scope's lists: the server's for the server's lists, the
// user's own and the server's they use for theirs. Source is what it
// belongs to (see SmartSourceAddon). Genres are those its genre filter
// offers.
type SmartCatalog struct {
	AddonID   accounts.ID
	AddonName string
	Shared    bool
	Source    string
	Catalog   stremio.Catalog
	Genres    []string
}

// SmartCatalogs lists the catalogs user's lists of a scope may draw from,
// by addon in order: catalogs of movies and series, not of music, whose
// titles lists judge by rules on movies and series.
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
		source := SmartSourceAddon
		switch {
		case entry.addon.Local():
			source = SmartSourceFolder
		case entry.addon.IPTV():
			source = SmartSourceIPTV
		}
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if smartCatalog(catalog) {
				result = append(result, SmartCatalog{AddonID: entry.addon.ID, AddonName: entry.addon.Name(), Shared: entry.shared,
					Source: source, Catalog: catalog, Genres: slices.Clone(addons.GenreChoices(catalog))})
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
