package library

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/mosaic"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Generated images.
//
// An addon's collection without a poster, a smart list shown as a
// collection, and a library whose image is set to be generated
// (addons.LibraryImageGenerated) show an image Polyfin draws: the posters
// of the first titles they list, in their order, under a gradient, with
// their name (see package mosaic). A collection gets two: its Primary
// image, a poster, and its Thumb, a wide image, for apps that show wide
// cards; a library its Primary image, wide as library tiles are, from its
// first titles, or for a collection library from its first collections'
// posters, a collection without one giving the first poster of its own
// generated image. Collections get theirs while Settings.GenerateImages
// is on; a library set to a generated image gets it whatever the setting.
//
// Nothing is drawn while listing: an item gets a URL standing for its
// image (see GeneratedImage), which holds its name and the posters it
// shows, as soon as the catalog pages Polyfin keeps give them; a listing
// asks no addon for them. Polyfin draws the image the first time an app
// asks for it, from the posters it relays (see the jellyfin package). Only
// the images Polyfin chose are drawn: Artwork tells them apart from the
// item's other artwork (see Artwork.Generated), never by their URL, and
// the artwork addons give in the URLs' scheme is dropped as it comes in
// (see withoutGenerated).
//
// The titles an image shows are remembered for its item, in memory and in
// the database, so that it does not change with each reordering of its
// catalogs: they are chosen again when its name changes, when half of
// those it shows, as a poster or as a wide image, are no longer among its
// first generatedReach titles, or when it shows fewer than it could. Its
// URL, and so its tag, then changes, and apps that keep images by tag
// fetch the new one.
//
// The image is the same for everyone who sees the item. A user under
// parental control or blocking genres is not shown it, as for a library's
// automatic image, since a title it shows may be hidden from them; nor is
// a user who hid one of the titles it shows. An image uploaded for the
// item wins over it (see overridden, Artwork).

const (
	// generatedReach is how many of an item's first titles are compared
	// with those its image shows.
	generatedReach = 20
	// generatedCheck is how long the titles an image shows are kept
	// without being compared again with the item's first titles.
	generatedCheck = time.Minute
	// generatedChecks bounds the items whose first titles a listing reads
	// at once.
	generatedChecks = 8
	// generatedScheme starts the URLs standing for generated images, and
	// generatedVersion changes with how they are drawn, which changes
	// their tags.
	generatedScheme  = "polyfin-generated:"
	generatedVersion = "1"
)

// GeneratedImage is an image Polyfin draws: a poster or a wide image of
// Name from Posters, in order, downloaded from public addresses only when
// Confined.
type GeneratedImage struct {
	Wide     bool
	Name     string
	Posters  []string
	Confined bool
}

// URL stands for the image in Images: its ImageTag changes with the name,
// the posters and the shape.
func (g GeneratedImage) URL() string {
	shape := "poster"
	if g.Wide {
		shape = "wide"
	}
	values := url.Values{"v": {generatedVersion}, "name": {g.Name}, "poster": g.Posters}
	if g.Confined {
		values.Set("confined", "1")
	}
	return generatedScheme + shape + "?" + values.Encode()
}

// artwork is the image as Artwork returns it.
func (g *GeneratedImage) artwork() Artwork {
	return Artwork{URL: g.URL(), Generated: g}
}

// withoutGenerated drops from a description the artwork whose URL uses the
// scheme of generated images: an addon's is no image Polyfin draws, and
// would otherwise be listed with a tag no image answers.
func withoutGenerated(meta *stremio.Meta) {
	drop := func(address *string) {
		if strings.HasPrefix(*address, generatedScheme) {
			*address = ""
		}
	}
	for _, address := range []*string{&meta.Poster, &meta.Background, &meta.Logo, &meta.LandscapePoster} {
		drop(address)
	}
	for i := range meta.Videos {
		drop(&meta.Videos[i].Thumbnail)
	}
	if meta.Extras != nil {
		for i := range meta.Extras.SeasonPosters {
			drop(&meta.Extras.SeasonPosters[i])
		}
		for number, poster := range meta.Extras.SeasonPosterByNumber {
			if strings.HasPrefix(poster, generatedScheme) {
				delete(meta.Extras.SeasonPosterByNumber, number)
			}
		}
		for i := range meta.Extras.Cast {
			drop(&meta.Extras.Cast[i].Photo)
		}
	}
	if meta.Collection != nil {
		for i := range meta.Collection.Items {
			withoutGenerated(&meta.Collection.Items[i])
		}
	}
}

// generatedChoice is what an item's generated image shows: its name, and
// its first titles with a poster, in order, with their posters.
type generatedChoice struct {
	name     string
	titles   []accounts.ID
	posters  []string
	confined bool
}

// generated is the image of the choice, nil when it shows no poster.
func (c generatedChoice) generated(wide bool) *GeneratedImage {
	if len(c.posters) == 0 {
		return nil
	}
	return &GeneratedImage{Wide: wide, Name: c.name, Posters: c.posters[:min(len(c.posters), mosaic.Tiles(wide))],
		Confined: c.confined}
}

// image is the URL of the image of the choice, "" when it shows no
// poster.
func (c generatedChoice) image(wide bool) string {
	if g := c.generated(wide); g != nil {
		return g.URL()
	}
	return ""
}

// candidate is one of an item's first titles, with its poster, if any.
type candidate struct {
	id     accounts.ID
	poster string
}

// choose returns the choice an item named name keeps, given the one
// remembered, if any, and its first titles as candidates, known when
// known is set, and whether it changed.
func choose(kept generatedChoice, found bool, name string, candidates []candidate, known, confined bool) (generatedChoice, bool) {
	var fresh generatedChoice
	fresh.name, fresh.confined = name, confined
	for _, c := range candidates {
		if c.poster != "" && len(fresh.titles) < mosaic.Tiles(true) {
			fresh.titles, fresh.posters = append(fresh.titles, c.id), append(fresh.posters, c.poster)
		}
	}
	switch {
	case !found:
		return fresh, len(fresh.titles) > 0
	case !known || len(fresh.titles) == 0:
		// Nothing to compare with: the titles shown stay, under the name.
		renamed := kept.name != name
		kept.name = name
		return kept, renamed
	case kept.name != name || kept.confined != confined || len(kept.titles) < len(fresh.titles):
		return fresh, true
	}
	listed := map[accounts.ID]bool{}
	for _, c := range candidates {
		listed[c.id] = true
	}
	for _, shown := range [][]accounts.ID{kept.titles[:min(len(kept.titles), mosaic.Tiles(false))], kept.titles} {
		gone := 0
		for _, id := range shown {
			if !listed[id] {
				gone++
			}
		}
		if 2*gone >= len(shown) {
			return fresh, true
		}
	}
	return kept, false
}

// generatedEntry is the choice remembered for an item, if found, and when
// it was last compared with the item's first titles.
type generatedEntry struct {
	choice  generatedChoice
	found   bool
	checked time.Time
	// unknown tells that none was found as the item's first titles were
	// not known yet, kept tells how many answers were kept then (see
	// generatedImages.kept).
	unknown bool
	kept    uint64
}

// generatedImages remembers the choices of the items' generated images,
// read from the database on first use.
type generatedImages struct {
	mu      sync.Mutex
	loaded  bool
	entries map[accounts.ID]generatedEntry
	// kept counts the catalog pages and descriptions addons answered and
	// Polyfin kept: an item that had none as its first titles were not
	// known looks again once one more is kept, rather than after
	// generatedCheck, so that a new library's image shows once an app
	// opened it.
	kept atomic.Uint64
}

// LookUpGeneratedImage has the generated image of an item looked up again
// as it next shows, as when an administrator chooses one for a library: a
// library made with a generated image, listed before its titles were read,
// may have been found to have none a moment ago.
func (s *Service) LookUpGeneratedImage(id accounts.ID) {
	s.generated.mu.Lock()
	defer s.generated.mu.Unlock()
	if entry, ok := s.generated.entries[id]; ok {
		entry.checked = time.Time{}
		s.generated.entries[id] = entry
	}
}

// generatedEntryOf returns the entry of item id.
func (s *Service) generatedEntryOf(ctx context.Context, id accounts.ID) (generatedEntry, error) {
	g := &s.generated
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.loaded {
		entries := map[accounts.ID]generatedEntry{}
		rows, err := s.db.Query(ctx, "SELECT item_id, name, titles, posters, confined FROM generated_images")
		if err != nil {
			return generatedEntry{}, err
		}
		var id accounts.ID
		var c generatedChoice
		if _, err := pgx.ForEachRow(rows, []any{&id, &c.name, &c.titles, &c.posters, &c.confined}, func() error {
			entries[id] = generatedEntry{choice: c, found: true}
			c = generatedChoice{}
			return nil
		}); err != nil {
			return generatedEntry{}, err
		}
		g.entries, g.loaded = entries, true
	}
	return g.entries[id], nil
}

// generatedChoiceOf returns the choice of the image of item id, named
// name, compared with the first titles find gives once generatedCheck has
// passed since it last was (see choose), and whether it shows any. A
// choice that changes is saved.
func (s *Service) generatedChoiceOf(ctx context.Context, id accounts.ID, name string, find func() (titles []candidate, known, confined bool)) (generatedChoice, bool) {
	entry, err := s.generatedEntryOf(ctx, id)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("The generated images could not be read", "error", err)
		}
		return generatedChoice{}, false
	}
	now := s.now()
	if entry.found && entry.choice.name == name && now.Sub(entry.checked) < generatedCheck {
		return entry.choice, true
	}
	kept := s.generated.kept.Load()
	if !entry.found && now.Sub(entry.checked) < generatedCheck && (!entry.unknown || entry.kept == kept) {
		return generatedChoice{}, false
	}
	titles, known, confined := find()
	choice, changed := choose(entry.choice, entry.found, name, titles, known, confined)
	found := entry.found || changed
	if changed {
		if _, err := s.db.Exec(ctx, `INSERT INTO generated_images (item_id, name, titles, posters, confined) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (item_id) DO UPDATE SET name = excluded.name, titles = excluded.titles, posters = excluded.posters,
				confined = excluded.confined, chosen_at = now()`, id, choice.name, choice.titles, choice.posters, choice.confined); err != nil {
			if ctx.Err() == nil {
				s.logger.Debug("A generated image could not be saved", "error", err)
			}
			return choice, found
		}
	}
	s.generated.mu.Lock()
	s.generated.entries[id] = generatedEntry{choice: choice, found: found, checked: now, unknown: !found && !known, kept: kept}
	s.generated.mu.Unlock()
	return choice, found
}

// generatesFor reports whether collections get generated images in v.
func (s *Service) generatesFor(v view) bool {
	return s.settings().GenerateImages && !v.restricted()
}

// shows reports whether the user of v is shown an image of choice: not
// when they hid one of the titles it shows.
func (v view) shows(choice generatedChoice) bool {
	return !slices.ContainsFunc(choice.titles, func(id accounts.ID) bool { return v.hidden[id] })
}

// shownName is the name item id shows under, administrators' edits
// included.
func (s *Service) shownName(id accounts.ID, name string) string {
	if o, ok := s.overridesNow().fields[id]; ok && o.Name != nil {
		return *o.Name
	}
	return name
}

// unfiltered is v for reading an item's first titles for its generated
// image: as everyone sees them, without parental control, blocked genres
// or hidden titles, and without the request's state.
func (v view) unfiltered() view {
	v.user, v.parental, v.genres, v.hidden = accounts.ID{}, accounts.ParentalControl{}, nil, nil
	v.lookups, v.held = new(atomic.Int32), new(atomic.Bool)
	return v
}

// generate gives item, a collection without a poster, its generated
// images in v, from the first titles find gives.
func (s *Service) generate(ctx context.Context, v view, item *Item, find func() ([]candidate, bool, bool)) {
	choice, ok := s.generatedChoiceOf(ctx, item.ID, s.shownName(item.ID, item.Name), find)
	if !ok || !v.shows(choice) {
		return
	}
	item.Images.Primary = choice.image(false)
	if item.Images.Thumb == "" {
		item.Images.Thumb = choice.image(true)
	}
}

// addGenerated gives the addons' collections among items, recorded as
// records, the generated images of those without a poster.
func (s *Service) addGenerated(ctx context.Context, v view, items []Item, records []record) {
	if !s.generatesFor(v) {
		return
	}
	var group errgroup.Group
	group.SetLimit(generatedChecks)
	for i := range items {
		if items[i].Kind != KindCollection || items[i].Images.Primary != "" {
			continue
		}
		group.Go(func() error {
			s.generate(ctx, v, &items[i], func() ([]candidate, bool, bool) { return s.collectionCandidates(ctx, v, records[i]) })
			return nil
		})
	}
	_ = group.Wait()
}

// collectionCandidates reads the first titles of an addon's collection
// from the pages and descriptions Polyfin keeps, asking no addon, and
// tells whether they are known and whether its addon is confined.
func (s *Service) collectionCandidates(ctx context.Context, v view, r record) ([]candidate, bool, bool) {
	if r.Addon == nil || r.Meta == nil {
		return nil, false, false
	}
	addon, ok := v.addon(*r.Addon)
	if !ok {
		return nil, false, false
	}
	ctx = context.WithValue(withKeptOnly(ctx), descriptionsKeptKey{}, true)
	meta, ok := s.keptDescription(ctx, addon, metaKey{addon.addon.ID, "collection", r.Meta.ID}, inDatabase)
	if !ok {
		if r.Meta.Collection == nil {
			return nil, false, addon.confined
		}
		meta = *r.Meta
	}
	reader := v.unfiltered()
	sources, _ := s.collectionSources(ctx, addon, meta, s.collectionMax(ctx, reader, r), false)
	if len(sources) == 0 {
		return nil, false, addon.confined
	}
	titles, _, err := s.merged(ctx, reader, sources, 0, generatedReach)
	if err != nil || len(titles) == 0 {
		return nil, false, addon.confined
	}
	candidates := make([]candidate, 0, len(titles))
	for _, title := range titles {
		if item, _, err := title.title(r.ID); err == nil {
			candidates = append(candidates, candidate{item.ID, title.meta.Poster})
		}
	}
	return candidates, true, addon.confined
}

// descriptionsKeptKey marks the context of a read of a collection's titles
// for its generated image, which takes only the descriptions of the
// collections within it already kept (see collectionSources). Other reads
// that ask no addon for pages still ask for those descriptions.
type descriptionsKeptKey struct{}

func descriptionsKept(ctx context.Context) bool {
	kept, _ := ctx.Value(descriptionsKeptKey{}).(bool)
	return kept
}

// smartCollectionImages gives the smart lists shown as collections among
// items their generated images.
func (s *Service) smartCollectionImages(ctx context.Context, v view, items []Item) {
	if !s.generatesFor(v) {
		return
	}
	for i := range items {
		l, ok := v.smartList(items[i].ID)
		if !ok || items[i].Kind != KindCollection || items[i].Images.Primary != "" {
			continue
		}
		s.generate(ctx, v, &items[i], func() ([]candidate, bool, bool) { return s.smartListCandidates(ctx, v, l.list) })
	}
}

// smartListCandidates reads the first titles of a smart list from the
// pages Polyfin keeps, as a user who did nothing with them yet sees them.
func (s *Service) smartListCandidates(ctx context.Context, v view, list smartlists.List) ([]candidate, bool, bool) {
	result, err := s.smartMatches(withKeptOnly(ctx), v.unfiltered(), list)
	if err != nil || len(result.titles) == 0 {
		return nil, false, false
	}
	confined := false
	candidates := make([]candidate, 0, min(len(result.titles), generatedReach))
	for _, t := range result.titles[:min(len(result.titles), generatedReach)] {
		poster := ""
		if t.record.Meta != nil {
			poster = t.record.Meta.Poster
		}
		candidates = append(candidates, candidate{t.item.ID, poster})
		confined = confined || t.record.Confined
	}
	return candidates, true, confined
}

// libraryChoice returns the choice of the generated image of a library set
// to one, and whether it shows any.
func (s *Service) libraryChoice(ctx context.Context, v view, l library) (generatedChoice, bool) {
	return s.generatedChoiceOf(ctx, l.item.ID, s.shownName(l.item.ID, l.serverName), func() ([]candidate, bool, bool) {
		return s.libraryCandidates(ctx, v, l)
	})
}

// libraryImage returns the generated image of a library set to one, "" when
// it shows none, and whether the user of v is shown it.
func (s *Service) libraryImage(ctx context.Context, v view, l library) (string, bool) {
	choice, ok := s.libraryChoice(ctx, v, l)
	if !ok {
		return "", false
	}
	return choice.image(true), !v.restricted() && v.shows(choice)
}

// libraryCandidates reads the first titles of a library, or the first
// collections of a collection library, from the pages Polyfin keeps. A
// collection without a poster gives the first poster of its own generated
// image. A music library has none.
func (s *Service) libraryCandidates(ctx context.Context, v view, l library) ([]candidate, bool, bool) {
	if l.addon.addon.HasMusic() {
		return nil, false, l.addon.confined
	}
	reader := v.unfiltered()
	metas, _, err := s.window(withKeptOnly(ctx), reader, l.catalogSource(), 0, generatedReach)
	if err != nil || len(metas) == 0 {
		return nil, false, l.addon.confined
	}
	addon := l.addon.addon.ID
	candidates := make([]candidate, 0, len(metas))
	for _, meta := range metas {
		if l.catalog.Type != "collection" {
			if item, _, err := titleItem(addon, l.catalog, meta, l.item.ID, l.addon.confined); err == nil {
				candidates = append(candidates, candidate{item.ID, meta.Poster})
			}
			continue
		}
		item, r := collectionItem(addon, meta, l.item.ID, l.addon.confined)
		poster := meta.Poster
		if poster == "" && s.settings().GenerateImages {
			choice, ok := s.generatedChoiceOf(ctx, item.ID, s.shownName(item.ID, item.Name), func() ([]candidate, bool, bool) {
				return s.collectionCandidates(ctx, v, r)
			})
			if ok && len(choice.posters) > 0 {
				poster = choice.posters[0]
			}
		}
		candidates = append(candidates, candidate{item.ID, poster})
	}
	return candidates, true, l.addon.confined
}

// generatedArtwork returns the generated image of type imageType of item
// id, a collection without a poster, as it was last chosen; nil without
// one.
func (s *Service) generatedArtwork(ctx context.Context, id accounts.ID, imageType string) *GeneratedImage {
	if !s.settings().GenerateImages {
		return nil
	}
	entry, err := s.generatedEntryOf(ctx, id)
	if err != nil || !entry.found {
		return nil
	}
	switch strings.ToLower(imageType) {
	case "primary":
		return entry.choice.generated(false)
	case "thumb":
		return entry.choice.generated(true)
	}
	return nil
}

// generatedLibraryArtwork returns the generated image of the library
// recorded as r, when the image its record remembers is the one chosen
// for it; nil otherwise, such as for the automatic image of a library
// that finds its own.
func (s *Service) generatedLibraryArtwork(ctx context.Context, r record) *GeneratedImage {
	if r.Poster == "" {
		return nil
	}
	entry, err := s.generatedEntryOf(ctx, r.ID)
	if err != nil || !entry.found {
		return nil
	}
	if g := entry.choice.generated(true); g != nil && g.URL() == r.Poster {
		return g
	}
	return nil
}
