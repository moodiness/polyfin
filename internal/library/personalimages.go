package library

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Personal images.
//
// Playlists, a user's watchlist among them (see package watchlist), the
// smart lists shown as libraries and the folders of home rows list titles
// as each user sees them: a playlist leaves out what the user may not
// reach, a list keeps what they did with its titles, a row is read for
// them. Each user who sees one gets their own generated image of it, wide
// as a library's, from the first titles it lists for them, chosen and
// kept as a collection's is (see generated.go): compared with its first
// titles when it is listed, at most once per generatedCheck, and saved
// when it changes. A playlist's image, and that of a row of a playlist,
// shows only titles it still lists: one taken off gives way to the next
// at once (see choose, strict). Changing a watchlist through the library
// has its images compared again at its next listing (see
// AddToWatchlist).
//
// Image requests come without a user, and a playlist's image differs with
// whoever sees it: the image is found by its tag (see PersonalArtwork),
// which its URL gives. What each image shows is kept in the database, so
// that a tag given before a restart is still answered; it goes with its
// user, its playlist and its smart list. As a library's generated image,
// it is drawn whatever Settings.GenerateImages says, and not shown to a
// user under parental control or blocking genres, nor when it shows a
// title the user hid.

// personalKey names a user's image of a view.
type personalKey struct {
	user, view accounts.ID
}

// personalTag names an image by its view and tag. Users whose images of
// a view show the same titles share it.
type personalTag struct {
	view accounts.ID
	tag  string
}

// personalShared is an image found by its tag, and how many users show it.
type personalShared struct {
	image GeneratedImage
	users int
}

// personalImages remembers the choices of the personal images, read from
// the database on first use, and finds them by their tag: the current one
// of each user's view, and the one before, which apps that keep the
// user's views for a while, as jellyfin-web does for a minute, still ask
// for.
type personalImages struct {
	mu       sync.Mutex
	loaded   bool
	entries  map[personalKey]generatedEntry
	tags     map[personalTag]*personalShared
	previous map[personalKey]personalTag
}

// tagOf names the image of entry, a choice of view, if it shows one.
func tagOf(view accounts.ID, entry generatedEntry) (personalTag, *GeneratedImage, bool) {
	if !entry.found {
		return personalTag{}, nil, false
	}
	g := entry.choice.generated(true)
	if g == nil {
		return personalTag{}, nil, false
	}
	return personalTag{view, ImageTag(g.URL())}, g, true
}

// release lets go of one user's hold on tag.
func (p *personalImages) release(tag personalTag) {
	if shared := p.tags[tag]; shared != nil {
		if shared.users--; shared.users <= 0 {
			delete(p.tags, tag)
		}
	}
}

// remember sets the entry of key, the tags following its choice: its
// image before stays found until the next change. The caller holds the
// lock.
func (p *personalImages) remember(key personalKey, entry generatedEntry) {
	old, hadOld := p.currentTag(key)
	p.entries[key] = entry
	current, g, ok := tagOf(key.view, entry)
	if ok {
		if shared := p.tags[current]; shared != nil {
			shared.users++
		} else {
			p.tags[current] = &personalShared{image: *g, users: 1}
		}
	}
	switch {
	case hadOld && (!ok || old != current):
		if previous, kept := p.previous[key]; kept {
			p.release(previous)
		}
		p.previous[key] = old
	case hadOld:
		p.release(old)
	}
}

// currentTag names the current image of key, if any.
func (p *personalImages) currentTag(key personalKey) (personalTag, bool) {
	tag, _, ok := tagOf(key.view, p.entries[key])
	return tag, ok
}

// personalEntryOf returns the entry of key, the images read first.
func (s *Service) personalEntryOf(ctx context.Context, key personalKey) (generatedEntry, error) {
	p := &s.personal
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := s.loadPersonal(ctx); err != nil {
		return generatedEntry{}, err
	}
	return p.entries[key], nil
}

// loadPersonal reads the personal images once. The caller holds the lock.
func (s *Service) loadPersonal(ctx context.Context) error {
	p := &s.personal
	if p.loaded {
		return nil
	}
	p.entries, p.tags, p.previous = map[personalKey]generatedEntry{}, map[personalTag]*personalShared{}, map[personalKey]personalTag{}
	rows, err := s.db.Query(ctx, "SELECT user_id, view_id, name, titles, posters, confined FROM personal_images")
	if err != nil {
		return err
	}
	var key personalKey
	var c generatedChoice
	if _, err := pgx.ForEachRow(rows, []any{&key.user, &key.view, &c.name, &c.titles, &c.posters, &c.confined}, func() error {
		p.remember(key, generatedEntry{choice: c, found: true})
		c = generatedChoice{}
		return nil
	}); err != nil {
		return err
	}
	p.loaded = true
	return nil
}

// recheckPersonal has every user's image of view compared with its first
// titles at its next listing.
func (s *Service) recheckPersonal(view accounts.ID) {
	p := &s.personal
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, entry := range p.entries {
		if key.view == view {
			entry.checked = time.Time{}
			p.entries[key] = entry
		}
	}
}

// personalSource is what a personal image is of: a smart list, a
// playlist, or a home row's folder, which may show a playlist.
type personalSource struct {
	smartList, playlist, homeRow *accounts.ID
}

// personalImage returns the URL of the image of view, named name, as the
// user of v is shown it, "" when there is none: from the first titles find
// gives, compared as rechoose does, strict for a watchlist.
func (s *Service) personalImage(ctx context.Context, v view, id accounts.ID, name string, source personalSource,
	find func() ([]candidate, bool, bool)) string {
	if v.user == (accounts.ID{}) || v.restricted() {
		return ""
	}
	key := personalKey{v.user, id}
	entry, err := s.personalEntryOf(ctx, key)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("The personal images could not be read", "error", err)
		}
		return ""
	}
	next, remember := s.rechoose(ctx, entry, name, find, source.playlist != nil, func(choice generatedChoice) error {
		_, err := s.db.Exec(ctx, `INSERT INTO personal_images (user_id, view_id, smart_list_id, playlist_id, home_row_id, name, titles, posters, confined)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (user_id, view_id) DO UPDATE SET name = excluded.name, titles = excluded.titles, posters = excluded.posters,
				confined = excluded.confined, chosen_at = now()`,
			key.user, key.view, source.smartList, source.playlist, source.homeRow, choice.name, choice.titles, choice.posters, choice.confined)
		return err
	})
	if remember {
		s.personal.mu.Lock()
		s.personal.remember(key, next)
		s.personal.mu.Unlock()
	}
	if !next.found || !v.shows(next.choice) {
		return ""
	}
	return next.choice.image(true)
}

// PlaylistImage returns the URL of the image of playlist, named name, as
// user is shown it, whose first titles are ids, a series standing for its
// episodes: "" when there is none.
func (s *Service) PlaylistImage(ctx context.Context, user accounts.User, playlist accounts.ID, name string, ids []accounts.ID) string {
	v, err := s.view(ctx, user)
	if err != nil {
		return ""
	}
	return s.personalImage(ctx, v, playlist, name, personalSource{playlist: &playlist}, func() ([]candidate, bool, bool) {
		return s.posterCandidates(ctx, ids)
	})
}

// HomeRowImage returns the URL of the image of folder, the folder of the
// home row row, named name, as user is shown it, "" when there is none:
// drawn from the first titles titles gives, a series standing for its
// episodes, read only when the image is compared with them. playlist,
// unless zero, is the playlist the row lists, whose image it shows: chosen
// again as soon as one of its titles leaves, as a playlist's is.
func (s *Service) HomeRowImage(ctx context.Context, user accounts.User, folder, row, playlist accounts.ID, name string, titles func() []accounts.ID) string {
	v, err := s.view(ctx, user)
	if err != nil {
		return ""
	}
	source := personalSource{homeRow: &row}
	if playlist != (accounts.ID{}) {
		source.playlist = &playlist
	}
	return s.personalImage(ctx, v, folder, name, source, func() ([]candidate, bool, bool) {
		return s.posterCandidates(ctx, titles())
	})
}

// posterCandidates describes the first of ids, in order, with their
// posters, as candidates of a personal image.
func (s *Service) posterCandidates(ctx context.Context, ids []accounts.ID) ([]candidate, bool, bool) {
	ids = ids[:min(len(ids), generatedReach)]
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, false, false
	}
	byID := make(map[accounts.ID]record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	confined := false
	candidates := make([]candidate, 0, len(ids))
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			continue
		}
		poster := ""
		if r.Meta != nil && (strings.HasPrefix(r.Meta.Poster, "https://") || strings.HasPrefix(r.Meta.Poster, "http://")) {
			poster = r.Meta.Poster
			confined = confined || r.Confined
		}
		candidates = append(candidates, candidate{id, poster})
	}
	return candidates, true, confined
}

// withSmartLibraryImage gives item, the smart list l shown as a library,
// its image for the user of v, wide: from the first titles the list keeps
// for them.
func (s *Service) withSmartLibraryImage(ctx context.Context, v view, l smartList, item *Item) {
	list := l.list.ID
	image := s.personalImage(ctx, v, l.item.ID, s.shownName(l.item.ID, l.item.Name), personalSource{smartList: &list}, func() ([]candidate, bool, bool) {
		return s.smartListCandidates(ctx, v, l.list)
	})
	if image != "" {
		item.Images.Primary, item.WidePrimary = image, true
	}
}

// PersonalArtwork returns the image of view id whose tag is tag, when it
// is one of the images users are shown of a watchlist or of a smart list
// shown as a library (see personal images).
func (s *Service) PersonalArtwork(ctx context.Context, id accounts.ID, tag string) (Artwork, bool) {
	if tag == "" {
		return Artwork{}, false
	}
	p := &s.personal
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := s.loadPersonal(ctx); err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("The personal images could not be read", "error", err)
		}
		return Artwork{}, false
	}
	shared, ok := p.tags[personalTag{id, tag}]
	if !ok {
		return Artwork{}, false
	}
	image := shared.image
	return image.artwork(), true
}
