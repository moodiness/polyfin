package library

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A server addon given a language (see addons.Addon.Language) describes
// titles in it for the users who chose that language as their metadata
// language, and for no one else: it is none of describers, so a title's
// facts, its genres, ratings, people and identifiers, are those the
// addons without a language give, whatever the user's language, and
// parental control, blocked genres, hidden titles and smart lists judge
// every user alike. What a translator gives is a title's name, overview,
// poster, backdrop and logo, and its episodes' names, overviews and
// images (see translation). Its descriptions are kept as any addon's,
// by addon: one language's are never another's.
//
// Translators stand in for the addons without a language where none of
// those describes a title (see standIn): they then describe it for every
// user, facts and episodes included, the user's language's first, else in
// the order of the sources. So a server whose metadata addons all have a
// language works as if they had none. Search, people search and similar
// titles likewise ask the translators' catalogs, for every user, when no
// addon without a language offers the catalogs they need.

// translationReaders bounds the listings whose descriptions in a user's
// language are read in the background at once, each one title at a time
// (see translate); translationReach bounds the titles one listing reads.
const (
	translationReaders = 3
	translationReach   = 100
)

// useLanguage sets the view's metadata language from the user's: theirs
// when one of the view's server addons has it, with those addons as its
// translators, else none. The names Polyfin generates follow it when it is
// English or French, else the server's language.
func (v *view) useLanguage(metadata, server string) {
	if metadata != "" {
		for _, entry := range v.addons {
			if entry.shared && entry.addon.Language == metadata {
				v.translators = append(v.translators, entry)
			}
		}
	}
	if len(v.translators) > 0 {
		v.language = metadata
	}
	v.words = vocabularyOf(generatedLanguage(v.language, server))
}

// generatedLanguage is the language Polyfin names what it generates in for
// a metadata language: its own when it is English or French, region aside,
// else the server's.
func generatedLanguage(metadata, server string) string {
	base, _, _ := strings.Cut(metadata, "-")
	if _, ok := vocabulary[base]; ok {
		return base
	}
	return server
}

// translator reports whether an addon describes titles for some users
// only: those of its language.
func translator(entry installed) bool { return entry.addon.Language != "" }

// translatorsOf lists the view's translators that describe titles of a
// type and identifier.
func (v view) translatorsOf(metaType, id string) []installed {
	var result []installed
	for _, entry := range v.translators {
		if serves(entry, "meta", metaType, id) {
			result = append(result, entry)
		}
	}
	return result
}

// standIn reports whether the translators of v describe titles of a type
// and identifier for every user: none of the addons of v without a
// language, Polyfin's own sources aside, the server's only when shared is
// set, describes them.
func (v view) standIn(metaType, id string, shared bool) bool {
	for _, entry := range v.addons {
		if !translator(entry) && !own(entry.addon) && (!shared || entry.shared) && serves(entry, "meta", metaType, id) {
			return false
		}
	}
	return true
}

// factual reports whether an addon of v gives the facts of titles of a
// type and identifier: it has no language, or translators stand in for
// those titles (see standIn), the server's addons only when shared is set.
func (v view) factual(entry installed, metaType, id string, shared bool) bool {
	return !translator(entry) || v.standIn(metaType, id, shared)
}

// standsInForAll reports whether translators stand in for every user for
// titles of a type and identifier (see standIn): no enabled addon without
// a language, the server's or a user's own, describes them. Only then are
// a translator's people the title's: they are recorded for everyone.
func (s *Service) standsInForAll(ctx context.Context, metaType, id string) bool {
	list, err := s.addons.All(ctx)
	if err != nil {
		return false
	}
	var v view
	for _, addon := range list {
		if addon.Enabled {
			v.addons = append(v.addons, installed{addon: addon})
		}
	}
	return v.standIn(metaType, id, false)
}

// inLanguageOrder orders addons of v the user's language's first, keeping
// the order of the others.
func (v view) inLanguageOrder(entries []installed) {
	if v.language == "" {
		return
	}
	slices.SortStableFunc(entries, func(a, b installed) int {
		switch {
		case (a.addon.Language == v.language) == (b.addon.Language == v.language):
			return 0
		case a.addon.Language == v.language:
			return -1
		}
		return 1
	})
}

// standInCatalogs returns, as sources in the order of the sources, the
// catalogs of the addons of v that a feature asks: offers picks those it
// needs, among the catalogs of kinds. Of the translators' catalogs, those
// of the user's translators are asked when mine is set, and all of them,
// for every user, for the kinds no addon without a language offers a
// catalog of.
func standInCatalogs(v view, kinds []Kind, mine bool, offers func(installed, stremio.Catalog) (source, bool)) []source {
	var found []source
	covered := map[Kind]bool{}
	for _, entry := range v.addons {
		for _, catalog := range entry.addon.Manifest.Catalogs {
			kind, ok := titleKind(catalog.Type)
			if !ok || !slices.Contains(kinds, kind) {
				continue
			}
			if src, ok := offers(entry, catalog); ok {
				found = append(found, src)
				covered[kind] = covered[kind] || !translator(entry)
			}
		}
	}
	return slices.DeleteFunc(found, func(src source) bool {
		kind, _ := titleKind(src.catalog.Type)
		return translator(src.addon) && covered[kind] && !(mine && v.translates(src.addon))
	})
}

// translates reports whether an addon is one of the user's translators.
func (v view) translates(entry installed) bool {
	return slices.ContainsFunc(v.translators, func(t installed) bool { return t.addon.ID == entry.addon.ID })
}

// listingKey marks the context of a listing of many titles described one
// by one (see withListing).
type listingKey struct{}

// withListing marks a listing that describes its titles one by one, as a
// home row of titles watched does: their descriptions in the user's
// language are those kept, the others read in the background (see shown).
func withListing(ctx context.Context) context.Context {
	return context.WithValue(ctx, listingKey{}, true)
}

func inListing(ctx context.Context) bool {
	marked, _ := ctx.Value(listingKey{}).(bool)
	return marked
}

// shown returns the description of a title the user of v sees: base, the
// description describe gave, with the name, overview, images and episodes'
// texts and images of the first of the view's translators that describes
// it, asking it when its description is not kept. Within a listing (see
// withListing), it asks no translator: a description not kept is read in
// the background, and base shows meanwhile. Without a translator that
// describes the title, it is base.
func (s *Service) shown(ctx context.Context, v view, metaType, id string, base stremio.Meta) stremio.Meta {
	translators := v.translatorsOf(metaType, id)
	if inListing(ctx) {
		for _, entry := range translators {
			if meta, ok := s.keptDescription(ctx, entry, metaKey{entry.addon.ID, metaType, id}, inDatabase); ok {
				return translation(base, meta)
			}
		}
		if len(translators) > 0 {
			s.readTranslations(ctx, []untranslated{{translators[0], metaType, id}})
		}
		return base
	}
	for _, entry := range translators {
		kept, err := s.keptMeta(ctx, entry, metaType, id)
		if err == nil {
			return translation(base, kept.value)
		}
		s.logger.Debug("An addon could not describe a title in its language", "addon", entry.addon.Manifest.Name, "error", err)
	}
	return base
}

// untranslated is a title whose description by a translator is not kept.
type untranslated struct {
	entry    installed
	metaType string
	id       string
}

// readTranslations reads the descriptions of titles in the background,
// one at a time, unless translationReaders reads are under way already:
// the next listing asks again.
func (s *Service) readTranslations(ctx context.Context, titles []untranslated) {
	select {
	case s.translations <- struct{}{}:
	default:
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		defer func() { <-s.translations }()
		for _, t := range titles {
			if _, err := s.keptMeta(detached, t.entry, t.metaType, t.id); err != nil {
				s.logger.Debug("An addon could not describe a title in its language", "addon", t.entry.addon.Manifest.Name, "error", err)
			}
		}
	}()
}

// translation returns base with what translated gives of its name,
// overview, poster, backdrop and logo, and of its episodes' names,
// overviews and thumbnails, matched by identifier, else by season and
// number. What translated leaves empty stays base's; the rest of base,
// its facts, is kept whole. base is not changed.
func translation(base, translated stremio.Meta) stremio.Meta {
	shown := base
	if name := strings.TrimSpace(translated.Name); name != "" {
		shown.Name = translated.Name
	}
	set := func(field *string, value string) {
		if value != "" {
			*field = value
		}
	}
	set(&shown.Description, translated.Description)
	set(&shown.Poster, translated.Poster)
	set(&shown.Background, translated.Background)
	set(&shown.Logo, translated.Logo)
	set(&shown.LandscapePoster, translated.LandscapePoster)
	if len(base.Videos) == 0 || len(translated.Videos) == 0 {
		return shown
	}
	byID := make(map[string]stremio.Video, len(translated.Videos))
	type place struct{ season, episode int }
	byPlace := make(map[place]stremio.Video, len(translated.Videos))
	for _, video := range translated.Videos {
		byID[video.ID] = video
		byPlace[place{int(video.Season), video.EpisodeNumber()}] = video
	}
	shown.Videos = make([]stremio.Video, len(base.Videos))
	for i, video := range base.Videos {
		other, ok := byID[video.ID]
		if !ok {
			other, ok = byPlace[place{int(video.Season), video.EpisodeNumber()}]
		}
		if ok {
			if other.DisplayName() != "" {
				video.Title, video.Name = other.Title, other.Name
			}
			set(&video.Overview, other.Overview)
			set(&video.Thumbnail, other.Thumbnail)
		}
		shown.Videos[i] = video
	}
	return shown
}

// translate gives the movies and series of a listing the name, overview
// and images of their description in the user's language, when it is
// kept in memory. The others show their catalog's, and their descriptions
// are read in the background, for the next listing: up to
// translationReach of them (see readTranslations).
func (s *Service) translate(ctx context.Context, v view, items []Item) {
	if v.language == "" {
		return
	}
	var unknown []untranslated
	for i := range items {
		item := &items[i]
		if item.Kind != KindMovie && item.Kind != KindSeries || item.StremioID == "" {
			continue
		}
		translators := v.translatorsOf(item.StremioType, item.StremioID)
		found := false
		for _, entry := range translators {
			if meta, ok := s.keptDescription(ctx, entry, metaKey{entry.addon.ID, item.StremioType, item.StremioID}, inMemory); ok {
				translateItem(item, meta)
				found = true
				break
			}
		}
		if !found && len(translators) > 0 && len(unknown) < translationReach {
			unknown = append(unknown, untranslated{translators[0], item.StremioType, item.StremioID})
		}
	}
	if len(unknown) > 0 {
		s.readTranslations(ctx, unknown)
	}
}

// translateItem gives a listed title what its description in a user's
// language gives of its name, overview and images (see translation).
func translateItem(item *Item, translated stremio.Meta) {
	var shown Item
	fromMeta(&shown, translation(stremio.Meta{Name: item.Name, Description: item.Overview, Poster: item.Images.Primary,
		Background: item.Images.Backdrop, Logo: item.Images.Logo, LandscapePoster: item.Images.Thumb}, translated))
	item.Name, item.Overview = shown.Name, shown.Overview
	if item.Kind == KindMovie || item.Kind == KindSeries {
		item.OriginalTitle = shown.Name
	}
	item.Images.Primary, item.Images.Backdrop, item.Images.Logo, item.Images.Thumb =
		shown.Images.Primary, shown.Images.Backdrop, shown.Images.Logo, shown.Images.Thumb
}

// TaggedArtwork returns the image of an item whose tag is tag (see
// ImageTag), when it is one of the images users see of it in their
// languages or in the server's, but not the one Artwork finds: image
// requests come without a user, and their tag tells which description the
// app was given. It looks among the descriptions kept, of the title or of
// an episode's or season's series, or of an extra's title, asking no
// addon.
func (s *Service) TaggedArtwork(ctx context.Context, id accounts.ID, tag string) (Artwork, bool) {
	if tag == "" {
		return Artwork{}, false
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return s.taggedExtraArtwork(ctx, id, tag)
	}
	if err != nil {
		return Artwork{}, false
	}
	title := r
	if r.Kind == KindSeason || r.Kind == KindEpisode {
		if title, err = s.load(ctx, r.seriesItemID()); err != nil {
			return Artwork{}, false
		}
	}
	if title.Meta == nil || title.Kind != KindMovie && title.Kind != KindSeries {
		return Artwork{}, false
	}
	v, err := s.artworkView(ctx, title)
	if err != nil {
		return Artwork{}, false
	}
	matches := func(meta stremio.Meta) (string, bool) {
		urls := []string{meta.Poster, meta.Background, meta.Logo, meta.LandscapePoster}
		for _, video := range meta.Videos {
			urls = append(urls, video.Thumbnail)
		}
		for _, url := range urls {
			if url != "" && ImageTag(url) == tag {
				return url, true
			}
		}
		return "", false
	}
	if url, ok := matches(*title.Meta); ok {
		return Artwork{URL: url, Confined: title.Confined}, true
	}
	if meta, describer, ok := s.describedMeta(ctx, v, title, false, inDatabase); ok {
		if url, ok := matches(meta); ok {
			return Artwork{URL: url, Confined: title.Confined || describer.confined}, true
		}
	}
	for _, entry := range v.addons {
		if !translator(entry) || !serves(entry, "meta", title.Meta.Type, title.Meta.ID) {
			continue
		}
		if meta, ok := s.keptDescription(ctx, entry, metaKey{entry.addon.ID, title.Meta.Type, title.Meta.ID}, inDatabase); ok {
			if url, ok := matches(meta); ok {
				return Artwork{URL: url, Confined: title.Confined || entry.confined}, true
			}
		}
	}
	return Artwork{}, false
}
