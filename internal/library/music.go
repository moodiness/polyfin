package library

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Eclipse music addons list tracks, albums, artists and playlists in catalog
// rows, and answer searches and album, artist and playlist pages. Each row
// enabled under Libraries is a music library (an audiobook addon's, a books
// library): its folder lists the row's items, and a listing of another kind
// derives it from them, the albums of a row of tracks or the tracks of a
// row of albums. An album or an artist a track only names is known by that
// name, and found again through the addon's search when it is opened.
const (
	// musicExpansion bounds the albums, artists or playlists whose pages a
	// listing reads to list what they hold.
	musicExpansion = 50
	// musicFetches bounds the pages one listing reads at once, and
	// addonFetches those read from one addon at once, over every listing.
	musicFetches = 8
	addonFetches = 8
	// searchTTL is how long a search's results are kept.
	searchTTL = 10 * time.Minute
	// staleMusic is how long a catalog, album, artist or playlist page is
	// kept once it is due to be read again: it is served meanwhile, while
	// the addon is asked again in the background.
	staleMusic = 24 * time.Hour
	// originEclipse is the Origin type of a track's version.
	originEclipse = "eclipse"
	// streamMargin renews a link this long before it expires, unless it was
	// given less than justResolved ago.
	streamMargin = time.Minute
	justResolved = 15 * time.Second
)

// MusicKind reports whether items of kind come from music addons.
func MusicKind(kind Kind) bool {
	switch kind {
	case KindArtist, KindAlbum, KindTrack, KindAudiobook, KindMusicPlaylist:
		return true
	}
	return false
}

// AudioKind reports whether items of kind play audio: tracks and
// audiobooks.
func AudioKind(kind Kind) bool {
	return kind == KindTrack || kind == KindAudiobook
}

// musicEntry is what Polyfin keeps of an item of a music addon: its type
// and identifier in the addon (empty for an album or artist only named by
// a track, whose Resolved is the addon's identifier once found), and what
// describes it. Duration is in seconds; Index is a track's place on its
// album or playlist; Count, an album's or playlist's tracks.
type musicEntry struct {
	Type        string   `json:"type"`
	ID          string   `json:"id,omitempty"`
	Resolved    string   `json:"resolved,omitempty"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist,omitempty"`
	AlbumArtist string   `json:"albumArtist,omitempty"`
	Album       string   `json:"album,omitempty"`
	AlbumID     string   `json:"albumId,omitempty"`
	Duration    float64  `json:"duration,omitempty"`
	Artwork     string   `json:"artwork,omitempty"`
	ISRC        string   `json:"isrc,omitempty"`
	Explicit    bool     `json:"explicit,omitempty"`
	Year        int      `json:"year,omitempty"`
	Index       int      `json:"index,omitempty"`
	Count       int      `json:"count,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Overview    string   `json:"overview,omitempty"`
	StreamURL   string   `json:"streamUrl,omitempty"`
	Format      string   `json:"format,omitempty"`
	Content     string   `json:"content,omitempty"`
}

func trackEntry(t eclipse.Track, index int, content string) musicEntry {
	return musicEntry{Type: eclipse.TypeTrack, ID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album, AlbumID: t.AlbumID,
		Duration: t.Duration.Duration().Seconds(), Artwork: t.ArtworkURL, ISRC: t.ISRC, Explicit: t.Explicit, Year: t.Year, Index: index,
		StreamURL: t.StreamURL, Format: t.Format, Content: content}
}

func albumEntry(a eclipse.Album, content string) musicEntry {
	e := musicEntry{Type: eclipse.TypeAlbum, ID: a.ID, Title: a.Title, Artist: a.Artist, Artwork: a.ArtworkURL, Year: a.Year,
		Explicit: a.Explicit, Overview: a.Description, Count: a.TrackCount, Content: content}
	if len(a.Tracks) > 0 {
		e.Count = len(a.Tracks)
		for _, t := range a.Tracks {
			e.Duration += t.Duration.Duration().Seconds()
			e.Explicit = e.Explicit || t.Explicit
		}
	}
	return e
}

func artistEntry(a eclipse.Artist, content string) musicEntry {
	return musicEntry{Type: eclipse.TypeArtist, ID: a.ID, Title: a.Name, Artwork: a.ArtworkURL, Genres: a.Genres, Overview: a.Bio,
		Count: len(a.Albums), Content: content}
}

func playlistEntry(p eclipse.Playlist, content string) musicEntry {
	e := musicEntry{Type: eclipse.TypePlaylist, ID: p.ID, Title: p.Title, Artist: p.Creator, Artwork: p.ArtworkURL, Overview: p.Description,
		Count: p.TrackCount, Content: content}
	if len(p.Tracks) > 0 {
		e.Count = len(p.Tracks)
		for _, t := range p.Tracks {
			e.Duration += t.Duration.Duration().Seconds()
		}
	}
	return e
}

// nameKey compares names as Jellyfin compares artists: ignoring case and
// surrounding blanks.
func musicName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func albumKey(addon accounts.ID, id string) string { return "album|" + addon.String() + "|" + id }

func namedAlbumKey(addon accounts.ID, artist, title string) string {
	return "album|" + addon.String() + "|~" + musicName(artist) + "\x1f" + musicName(title)
}

func artistKey(addon accounts.ID, id string) string { return "artist|" + addon.String() + "|" + id }

func namedArtistKey(addon accounts.ID, name string) string {
	return "artist|" + addon.String() + "|~" + musicName(name)
}

// albumArtist is the artist of a track's album: its own when known, else
// the track's.
func (e musicEntry) albumArtist() string {
	if e.AlbumArtist != "" {
		return e.AlbumArtist
	}
	return e.Artist
}

// key is the stable key of an entry's item.
func (e musicEntry) key(addon accounts.ID) string {
	switch e.Type {
	case eclipse.TypeTrack:
		return "track|" + addon.String() + "|" + e.ID
	case eclipse.TypeAlbum:
		if e.ID == "" {
			return namedAlbumKey(addon, e.Artist, e.Title)
		}
		return albumKey(addon, e.ID)
	case eclipse.TypeArtist:
		if e.ID == "" {
			return namedArtistKey(addon, e.Title)
		}
		return artistKey(addon, e.ID)
	default:
		return "musicplaylist|" + addon.String() + "|" + e.ID
	}
}

// kind is the kind of an entry's item: tracks of audiobook addons are
// audiobooks, and podcasts' episodes are tracks.
func (e musicEntry) kind() Kind {
	switch e.Type {
	case eclipse.TypeTrack:
		if e.Content == eclipse.ContentAudiobook {
			return KindAudiobook
		}
		return KindTrack
	case eclipse.TypeAlbum:
		return KindAlbum
	case eclipse.TypeArtist:
		return KindArtist
	default:
		return KindMusicPlaylist
	}
}

// albumRef is the key of the album a track belongs to, empty when it
// names none.
func (e musicEntry) albumRef(addon accounts.ID) string {
	switch {
	case e.AlbumID != "":
		return albumKey(addon, e.AlbumID)
	case e.Album != "":
		return namedAlbumKey(addon, e.albumArtist(), e.Album)
	}
	return ""
}

func musicRecord(entry installed, e musicEntry, parent *accounts.ID) record {
	key := e.key(entry.addon.ID)
	addon := entry.addon.ID
	return record{ID: itemID(key), Key: key, Kind: e.kind(), Addon: &addon, Parent: parent, Music: &e, Confined: entry.confined}
}

// stubs are the records of the album and artists a track names, which
// stand for them until they are listed themselves (see saveNew).
func stubs(entry installed, records []record) []record {
	var result []record
	for _, r := range records {
		e := r.Music
		switch {
		case e == nil:
		case e.Type == eclipse.TypeTrack:
			if e.Album != "" {
				album := musicEntry{Type: eclipse.TypeAlbum, ID: e.AlbumID, Title: e.Album, Artist: e.albumArtist(), Artwork: e.Artwork,
					Year: e.Year, Content: e.Content}
				result = append(result, musicRecord(entry, album, nil))
			}
			for _, name := range uniqueNames(e.Artist, e.albumArtist()) {
				result = append(result, musicRecord(entry, musicEntry{Type: eclipse.TypeArtist, Title: name, Content: e.Content}, nil))
			}
		case e.Type == eclipse.TypeAlbum && e.Artist != "":
			result = append(result, musicRecord(entry, musicEntry{Type: eclipse.TypeArtist, Title: e.Artist, Content: e.Content}, nil))
		}
	}
	return result
}

func uniqueNames(names ...string) []string {
	var result []string
	for _, name := range names {
		if strings.TrimSpace(name) != "" && !slices.ContainsFunc(result, func(n string) bool { return musicName(n) == musicName(name) }) {
			result = append(result, name)
		}
	}
	return result
}

func credit(addon accounts.ID, name string) Credit {
	return Credit{ID: itemID(namedArtistKey(addon, name)), Name: name}
}

// musicItem describes the item of a record.
func musicItem(r record) Item {
	e := r.Music
	addon := deref(r.Addon)
	item := Item{ID: r.ID, Kind: r.Kind, Name: e.Title, ParentID: deref(r.Parent), Overview: e.Overview, Genres: e.Genres,
		ProductionYear: e.Year, Runtime: time.Duration(e.Duration * float64(time.Second)), Images: Images{Primary: e.Artwork},
		Explicit: e.Explicit, ISRC: e.ISRC, Available: true}
	if e.Year > 0 {
		// Jellyfin dates a release from its year when it knows no more.
		item.PremiereDate = new(time.Date(e.Year, time.January, 1, 0, 0, 0, 0, time.UTC))
	}
	switch e.Type {
	case eclipse.TypeTrack:
		item.IndexNumber = e.Index
		item.Container = e.Format
		item.Album = e.Album
		if ref := e.albumRef(addon); ref != "" {
			item.AlbumID = itemID(ref)
			item.AlbumPoster = e.Artwork
		}
		if e.Artist != "" {
			item.Artists = []Credit{credit(addon, e.Artist)}
		}
		if artist := e.albumArtist(); artist != "" {
			item.AlbumArtist = new(credit(addon, artist))
		}
	case eclipse.TypeAlbum:
		if e.Artist != "" {
			item.Artists = []Credit{credit(addon, e.Artist)}
			item.AlbumArtist = new(credit(addon, e.Artist))
		}
	case eclipse.TypePlaylist:
		if e.Artist != "" {
			item.Artists = []Credit{credit(addon, e.Artist)}
		}
	}
	if e.Count > 0 {
		item.ChildCount = new(e.Count)
	}
	return item
}

// allowsMusic reports whether the user may reach a music addon's item: not
// an explicit one when their parental control has a highest rating, since
// such content is rated for adults; none when they block unrated music (or
// books, for audiobooks), as music is never rated; and none of the genres
// they block.
func (v view) allowsMusic(r record) bool {
	e := r.Music
	if e == nil || r.Addon == nil {
		return false
	}
	if _, ok := v.addon(*r.Addon); !ok {
		return false
	}
	if e.Explicit && v.parental.MaxRating != nil {
		return false
	}
	kind := "Music"
	if e.Content == eclipse.ContentAudiobook {
		kind = "Book"
	}
	return !slices.Contains(v.parental.BlockUnrated, kind) && !v.blocked(e.Genres)
}

// eclipseAddon is what a request to an Eclipse addon needs.
func (e installed) eclipseAddon() eclipse.Addon {
	return eclipse.Addon{ManifestURL: e.addon.ManifestURL, Manifest: *e.addon.Music, Settings: e.addon.Settings, Confined: e.confined}
}

// content is what the addon's tracks are: music, audiobooks or podcasts.
func (e installed) content() string {
	return e.addon.Music.ContentType
}

// musicKey keys what an addon answered, for the settings it was asked with.
type musicKey struct {
	addon    accounts.ID
	settings string
	resource string
	id       string
	skip     int
}

func (e installed) musicKey(resource, id string, skip int) musicKey {
	return musicKey{addon: e.addon.ID, settings: e.addon.Music.Query(e.addon.Settings).Encode(), resource: resource, id: id, skip: skip}
}

// musicPage is a page of a catalog row and its length as sent.
type musicPage struct {
	items  []eclipse.Item
	length int
}

// musicCaches keep what music addons answered: catalog pages as long as
// other catalogs' pages (the settings' CatalogRefreshMinutes), album,
// artist and playlist pages as long as descriptions, then, stale,
// staleMusic more (see remember); searches briefly. Each is bounded in
// bytes too, as the JSON of its answers measures them: a playlist can hold
// hundreds of tracks. fetches bounds the requests for them under way to
// each addon.
type musicCaches struct {
	pages     musicCache[musicPage]
	albums    musicCache[eclipse.Album]
	artists   musicCache[eclipse.Artist]
	playlists musicCache[eclipse.Playlist]
	searches  musicCache[eclipse.Results]
	fetches   *addonSlots
}

func newMusicCaches(s *Service) musicCaches {
	clock := func() time.Time { return s.now() }
	fixed := func(life time.Duration) func() time.Duration { return func() time.Duration { return life } }
	return musicCaches{
		pages:     newMusicCache(2000, 16<<20, func(p musicPage) int { return cache.JSONSize(p.items) }, s.catalogLife, staleMusic, clock),
		albums:    newMusicCache(2000, 8<<20, cache.JSONSize[eclipse.Album], fixed(metaTTL), staleMusic, clock),
		artists:   newMusicCache(2000, 8<<20, cache.JSONSize[eclipse.Artist], fixed(metaTTL), staleMusic, clock),
		playlists: newMusicCache(1000, 16<<20, cache.JSONSize[eclipse.Playlist], fixed(metaTTL), staleMusic, clock),
		searches:  newMusicCache(500, 4<<20, cache.JSONSize[eclipse.Results], fixed(searchTTL), 0, clock),
		fetches:   &addonSlots{slots: map[accounts.ID]chan struct{}{}},
	}
}

// musicCache keeps answers of one kind with the time they were given:
// fresh for life, then kept stale for stale more.
type musicCache[V any] struct {
	answers *cache.Cache[musicKey, answered[V]]
	life    func() time.Duration
}

// answered is an addon's answer, given at.
type answered[V any] struct {
	value V
	at    time.Time
}

// newMusicCache returns a cache of at most capacity answers and maxBytes,
// as size measures each answer.
func newMusicCache[V any](capacity, maxBytes int, size func(V) int, life func() time.Duration, stale time.Duration,
	now func() time.Time) musicCache[V] {
	answers := cache.NewLasting[musicKey, answered[V]](capacity, func() time.Duration { return life() + stale }, now).
		Sized(maxBytes, func(a answered[V]) int { return size(a.value) })
	return musicCache[V]{answers: answers, life: life}
}

// addonSlots bounds the requests under way to each addon.
type addonSlots struct {
	mu    sync.Mutex
	slots map[accounts.ID]chan struct{}
}

// acquire waits for a place for a request to addon, which release frees.
func (a *addonSlots) acquire(ctx context.Context, addon accounts.ID) (release func(), err error) {
	a.mu.Lock()
	slots, ok := a.slots[addon]
	if !ok {
		slots = make(chan struct{}, addonFetches)
		a.slots[addon] = slots
	}
	a.mu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// remember returns what key holds in c while it is fresh. A stale answer
// is returned too, while the addon is asked again in the background, for
// later requests; with none kept, the addon's answer is waited for.
// Concurrent requests share one request to the addon, which waits for a
// place among those under way to it.
func remember[V any](ctx context.Context, s *Service, c musicCache[V], key musicKey, fetch func(context.Context) (V, error)) (V, error) {
	flightKey := fmt.Sprintf("music %v", key)
	ask := func(ctx context.Context) (V, error) {
		release, err := s.musicCache.fetches.acquire(ctx, key.addon)
		if err != nil {
			var zero V
			return zero, err
		}
		defer release()
		value, err := fetch(ctx)
		if err == nil {
			c.answers.Put(key, answered[V]{value: value, at: s.now()})
		}
		return value, err
	}
	if kept, ok := c.answers.Get(key); ok {
		if s.now().Sub(kept.at) > c.life() {
			detached := context.WithoutCancel(ctx)
			s.flight.DoChan(flightKey, func() (any, error) {
				// A refresh that ended between the read above and this one
				// already asked the addon.
				if newer, ok := c.answers.Get(key); ok && newer.at.After(kept.at) {
					return newer.value, nil
				}
				value, err := ask(detached)
				if err != nil {
					s.logger.Debug("A music page could not be read again", "resource", key.resource, "error", err)
				}
				return value, err
			})
		}
		return kept.value, nil
	}
	return shared(ctx, &s.flight, flightKey, ask)
}

func (s *Service) musicCatalogPage(ctx context.Context, entry installed, catalog eclipse.Catalog, skip int) (musicPage, error) {
	return remember(ctx, s, s.musicCache.pages, entry.musicKey("catalog", catalog.ID, skip), func(ctx context.Context) (musicPage, error) {
		items, length, err := s.music.Catalog(ctx, entry.eclipseAddon(), catalog, skip)
		return musicPage{items: items, length: length}, err
	})
}

func (s *Service) albumPage(ctx context.Context, entry installed, id string) (eclipse.Album, error) {
	return remember(ctx, s, s.musicCache.albums, entry.musicKey("album", id, 0), func(ctx context.Context) (eclipse.Album, error) {
		return s.music.Album(ctx, entry.eclipseAddon(), id)
	})
}

func (s *Service) artistPage(ctx context.Context, entry installed, id string) (eclipse.Artist, error) {
	return remember(ctx, s, s.musicCache.artists, entry.musicKey("artist", id, 0), func(ctx context.Context) (eclipse.Artist, error) {
		return s.music.Artist(ctx, entry.eclipseAddon(), id)
	})
}

func (s *Service) playlistPage(ctx context.Context, entry installed, id string) (eclipse.Playlist, error) {
	return remember(ctx, s, s.musicCache.playlists, entry.musicKey("playlist", id, 0), func(ctx context.Context) (eclipse.Playlist, error) {
		return s.music.Playlist(ctx, entry.eclipseAddon(), id)
	})
}

func (s *Service) musicSearch(ctx context.Context, entry installed, term string) (eclipse.Results, error) {
	return remember(ctx, s, s.musicCache.searches, entry.musicKey("search", musicName(term), 0), func(ctx context.Context) (eclipse.Results, error) {
		return s.music.Search(ctx, entry.eclipseAddon(), term)
	})
}

// musicCatalog reads a catalog row from its start, a page of
// eclipse.CatalogPage at a time, until a shorter page ends it or the
// catalog limit is reached. A page that fails ends the row there, unless it
// is the first.
func (s *Service) musicCatalog(ctx context.Context, v view, entry installed, catalogType, catalogID string) ([]eclipse.Item, error) {
	catalog, ok := entry.addon.Music.Catalog(catalogID)
	if !ok || catalog.Type != catalogType {
		return nil, ErrNotFound
	}
	var items []eclipse.Item
	for skip := 0; skip < v.catalogLimit; skip += eclipse.CatalogPage {
		page, err := s.musicCatalogPage(ctx, entry, catalog, skip)
		if err != nil {
			if skip == 0 {
				return nil, err
			}
			s.logger.Warn("A music catalog could not be read further", "addon", entry.addon.Manifest.Name, "error", err)
			break
		}
		items = append(items, page.items...)
		if page.length < eclipse.CatalogPage {
			break
		}
	}
	return items[:min(len(items), v.catalogLimit)], nil
}

// itemRecords makes records of catalog items, listed in parent.
func itemRecords(entry installed, items []eclipse.Item, parent *accounts.ID) []record {
	records := make([]record, 0, len(items))
	content := entry.content()
	for _, item := range items {
		switch {
		case item.Track != nil:
			records = append(records, musicRecord(entry, trackEntry(*item.Track, 0, content), parent))
		case item.Album != nil:
			records = append(records, musicRecord(entry, albumEntry(*item.Album, content), parent))
		case item.Artist != nil:
			records = append(records, musicRecord(entry, artistEntry(*item.Artist, content), parent))
		case item.Playlist != nil:
			records = append(records, musicRecord(entry, playlistEntry(*item.Playlist, content), parent))
		}
	}
	return records
}

// trackRecords makes records of tracks listed in parent: an album's,
// numbered in its order when numbered, as their number is their place on
// their album; a playlist's, an artist's or a search's, unnumbered.
func trackRecords(entry installed, tracks []eclipse.Track, parent *accounts.ID, albumArtist string, numbered bool) []record {
	records := make([]record, 0, len(tracks))
	for i, t := range tracks {
		index := 0
		if numbered {
			index = i + 1
		}
		e := trackEntry(t, index, entry.content())
		if albumArtist != "" && !strings.EqualFold(albumArtist, e.Artist) {
			e.AlbumArtist = albumArtist
		}
		records = append(records, musicRecord(entry, e, parent))
	}
	return records
}

// musicLibraryRecords lists the items of a music library's catalog row.
func (s *Service) musicLibraryRecords(ctx context.Context, v view, l library) ([]record, error) {
	items, err := s.musicCatalog(ctx, v, l.addon, l.catalog.Type, l.catalog.ID)
	if err != nil {
		return nil, err
	}
	return itemRecords(l.addon, items, &l.item.ID), nil
}

// albumTracks lists an album's tracks: from its page, or, for an album
// only named by tracks, from the page of the album the addon's search
// finds by that name, else from the tracks the search finds on it.
func (s *Service) albumTracks(ctx context.Context, v view, entry installed, r record) ([]record, error) {
	e := r.Music
	id := e.ID
	if id == "" {
		id = e.Resolved
	}
	if id == "" {
		found, tracks, err := s.findAlbum(ctx, entry, e.Title, e.Artist)
		if err != nil {
			return nil, err
		}
		if found == "" {
			return trackRecords(entry, tracks, &r.ID, e.Artist, false), nil
		}
		id = found
		resolved := *e
		resolved.Resolved = id
		_ = s.save(ctx, []record{{ID: r.ID, Key: r.Key, Kind: r.Kind, Addon: r.Addon, Parent: r.Parent, Music: &resolved, Confined: r.Confined}})
	}
	album, err := s.albumPage(ctx, entry, id)
	if err != nil {
		return nil, err
	}
	tracks := trackRecords(entry, album.Tracks, &r.ID, album.Artist, true)
	if e.ID == "" {
		// The tracks name the album as their tracks named it, so that they
		// lead back to the same item.
		for i := range tracks {
			tracks[i].Music.AlbumID, tracks[i].Music.Album, tracks[i].Music.AlbumArtist = "", e.Title, e.Artist
		}
	}
	return tracks, nil
}

// findAlbum looks an album up by name in the addon's search: its
// identifier when an album of that title (and artist, when known) comes
// up, else the tracks found on an album of that title.
func (s *Service) findAlbum(ctx context.Context, entry installed, title, artist string) (string, []eclipse.Track, error) {
	results, err := s.musicSearch(ctx, entry, strings.TrimSpace(title+" "+artist))
	if err != nil {
		return "", nil, err
	}
	for _, album := range results.Albums {
		if musicName(album.Title) == musicName(title) && (artist == "" || album.Artist == "" || musicName(album.Artist) == musicName(artist)) {
			return album.ID, nil, nil
		}
	}
	var tracks []eclipse.Track
	for _, t := range results.Tracks {
		if musicName(t.Album) == musicName(title) && (artist == "" || musicName(t.Artist) == musicName(artist)) {
			tracks = append(tracks, t)
		}
	}
	return "", tracks, nil
}

// artistOf returns an artist's page: theirs, or, for an artist only named
// by tracks, that of the artist of that name the addon's search finds,
// else one made of the tracks and albums the search finds by that name.
func (s *Service) artistOf(ctx context.Context, entry installed, r record) (eclipse.Artist, error) {
	e := r.Music
	id := e.ID
	if id == "" {
		id = e.Resolved
	}
	if id != "" {
		return s.artistPage(ctx, entry, id)
	}
	results, err := s.musicSearch(ctx, entry, e.Title)
	if err != nil {
		return eclipse.Artist{}, err
	}
	for _, artist := range results.Artists {
		if musicName(artist.Name) != musicName(e.Title) {
			continue
		}
		page, err := s.artistPage(ctx, entry, artist.ID)
		if err != nil {
			// An addon without artist pages: the search's artist.
			page = artist
		}
		resolved := *e
		resolved.Resolved, resolved.Artwork, resolved.Genres = artist.ID, page.ArtworkURL, page.Genres
		_ = s.save(ctx, []record{{ID: r.ID, Key: r.Key, Kind: r.Kind, Addon: r.Addon, Parent: r.Parent, Music: &resolved, Confined: r.Confined}})
		return page, nil
	}
	page := eclipse.Artist{Name: e.Title}
	for _, t := range results.Tracks {
		if musicName(t.Artist) == musicName(e.Title) {
			page.TopTracks = append(page.TopTracks, t)
		}
	}
	for _, album := range results.Albums {
		if musicName(album.Artist) == musicName(e.Title) {
			page.Albums = append(page.Albums, album)
		}
	}
	return page, nil
}

// artistRecords lists an artist's albums, or their tracks: their top
// tracks, then those of their albums, at most musicExpansion of them read.
func (s *Service) artistRecords(ctx context.Context, v view, entry installed, r record, tracks bool) ([]record, error) {
	artist, err := s.artistOf(ctx, entry, r)
	if err != nil {
		return nil, err
	}
	content := entry.content()
	if !tracks {
		records := make([]record, 0, len(artist.Albums))
		for _, album := range artist.Albums {
			records = append(records, musicRecord(entry, albumEntry(album, content), &r.ID))
		}
		return records, nil
	}
	records := trackRecords(entry, artist.TopTracks, &r.ID, "", false)
	albums := make([]record, 0, len(artist.Albums))
	for _, album := range artist.Albums {
		albums = append(albums, musicRecord(entry, albumEntry(album, content), &r.ID))
	}
	return append(records, s.expand(ctx, v, entry, albums)...), nil
}

// playlistTracks lists a playlist's tracks.
func (s *Service) playlistTracks(ctx context.Context, entry installed, r record) ([]record, error) {
	playlist, err := s.playlistPage(ctx, entry, r.Music.ID)
	if err != nil {
		return nil, err
	}
	return trackRecords(entry, playlist.Tracks, &r.ID, "", false), nil
}

// expand lists the tracks of albums, playlists or artists (their top
// tracks), reading at most musicExpansion of their pages, a few at once. A
// page that cannot be read leaves its tracks out.
func (s *Service) expand(ctx context.Context, v view, entry installed, folders []record) []record {
	folders = folders[:min(len(folders), musicExpansion)]
	lists := make([][]record, len(folders))
	var g errgroup.Group
	g.SetLimit(musicFetches)
	for i, folder := range folders {
		g.Go(func() error {
			var err error
			switch folder.Music.Type {
			case eclipse.TypeAlbum:
				lists[i], err = s.albumTracks(ctx, v, entry, folder)
			case eclipse.TypePlaylist:
				lists[i], err = s.playlistTracks(ctx, entry, folder)
			case eclipse.TypeArtist:
				var artist eclipse.Artist
				if artist, err = s.artistOf(ctx, entry, folder); err == nil {
					lists[i] = trackRecords(entry, artist.TopTracks, &folder.ID, "", false)
				}
			}
			if err != nil && ctx.Err() == nil {
				s.logger.Debug("A music page could not be read", "addon", entry.addon.Manifest.Name, "error", err)
			}
			return nil
		})
	}
	_ = g.Wait()
	return slices.Concat(lists...)
}

// derived lists, in order and each once, the albums or the artists the
// records of a library name, listed in parent.
func derived(entry installed, records []record, kind Kind, parent *accounts.ID) []record {
	var result []record
	seen := map[accounts.ID]bool{}
	for _, stub := range stubs(entry, records) {
		if stub.Kind != kind || seen[stub.ID] {
			continue
		}
		seen[stub.ID] = true
		stub.Parent = parent
		result = append(result, stub)
	}
	return result
}

// MusicQuery is what a music listing asks for: the children of Parent, a
// music library, album, artist or playlist, or of every music library of
// the user when it is zero; or, of every kind in Kinds, the items Parent
// holds at any depth (Recursive), which the library derives from its row
// when it lists another kind (see musicLibrary); or the tracks or albums
// of the albums and artists named.
type MusicQuery struct {
	Parent    accounts.ID
	Kinds     []Kind
	Recursive bool
	ArtistIDs []accounts.ID
	AlbumIDs  []accounts.ID
	// Shallow derives other kinds from a library's row only when its own
	// items name them, reading no album, artist or playlist page.
	Shallow bool
}

// MusicFolder reports whether an item holds music: a music library, an
// album, an artist or a playlist of an Eclipse addon.
func (s *Service) MusicFolder(ctx context.Context, user accounts.User, id accounts.ID) (bool, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return false, err
	}
	if l, ok := v.library(id); ok {
		return l.addon.addon.Eclipse(), nil
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil && MusicKind(r.Kind) && !AudioKind(r.Kind), err
}

// Music lists music, as asked by q, of the addons the user reaches, leaving
// out what their parental control hides (see allowsMusic).
func (s *Service) Music(ctx context.Context, user accounts.User, q MusicQuery) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var records, named []record
	add := func(entry installed, list []record) {
		records = append(records, list...)
		named = append(named, stubs(entry, list)...)
	}
	switch {
	case len(q.AlbumIDs) > 0 || len(q.ArtistIDs) > 0:
		for _, id := range slices.Concat(q.AlbumIDs, q.ArtistIDs) {
			r, entry, err := s.musicFolder(ctx, v, id)
			if err != nil {
				continue
			}
			for _, kind := range kindsOr(q.Kinds, KindTrack) {
				var list []record
				switch {
				case r.Kind == KindAlbum && AudioKind(kind):
					list, err = s.albumTracks(ctx, v, entry, r)
				case r.Kind == KindArtist && (AudioKind(kind) || kind == KindAlbum):
					list, err = s.artistRecords(ctx, v, entry, r, kind != KindAlbum)
				}
				if err != nil {
					s.logger.Debug("A music page could not be read", "addon", entry.addon.Manifest.Name, "error", err)
					continue
				}
				add(entry, list)
			}
		}
	case q.Parent == (accounts.ID{}):
		for _, l := range v.libraries {
			if !l.addon.addon.Eclipse() {
				continue
			}
			list, err := s.musicLibrary(ctx, v, l, q.Kinds, true, q.Shallow)
			if err != nil {
				s.logger.Debug("A music library could not be listed", "addon", l.addon.addon.Manifest.Name, "error", err)
				continue
			}
			add(l.addon, list)
		}
	default:
		if l, ok := v.library(q.Parent); ok {
			if !l.addon.addon.Eclipse() {
				return nil, ErrNotFound
			}
			list, err := s.musicLibrary(ctx, v, l, q.Kinds, q.Recursive, q.Shallow)
			if err != nil {
				return nil, err
			}
			add(l.addon, list)
			break
		}
		r, entry, err := s.musicFolder(ctx, v, q.Parent)
		if err != nil {
			return nil, err
		}
		list, err := s.folderMusic(ctx, v, entry, r, q.Kinds)
		if err != nil {
			return nil, err
		}
		add(entry, list)
	}
	items, err := s.musicItems(ctx, v, records, named)
	return s.overridden(items), err
}

func kindsOr(kinds []Kind, fallback Kind) []Kind {
	if len(kinds) == 0 {
		return []Kind{fallback}
	}
	return kinds
}

// musicFolder loads an album, artist or playlist the user reaches, with
// its addon.
func (s *Service) musicFolder(ctx context.Context, v view, id accounts.ID) (record, installed, error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return record{}, installed{}, err
	}
	if !MusicKind(r.Kind) || AudioKind(r.Kind) || !v.allowsMusic(r) {
		return record{}, installed{}, ErrNotFound
	}
	entry, _ := v.addon(*r.Addon)
	if !entry.addon.Eclipse() {
		return record{}, installed{}, ErrNotFound
	}
	return r, entry, nil
}

// folderMusic lists what an album, artist or playlist holds: an album's or
// playlist's tracks, and an artist's albums, or their tracks when tracks
// are asked for.
func (s *Service) folderMusic(ctx context.Context, v view, entry installed, r record, kinds []Kind) ([]record, error) {
	switch r.Kind {
	case KindAlbum:
		return s.albumTracks(ctx, v, entry, r)
	case KindMusicPlaylist:
		return s.playlistTracks(ctx, entry, r)
	default:
		return s.artistRecords(ctx, v, entry, r, slices.ContainsFunc(kinds, AudioKind))
	}
}

// musicLibrary lists a music library: its row's items when no kind is
// asked for, else those of each kind asked for, derived from the row's
// items when they are of another kind: the tracks of albums, playlists or
// artists (their top tracks), read for at most musicExpansion of them; the
// albums and artists the tracks or albums name; an artist's albums.
func (s *Service) musicLibrary(ctx context.Context, v view, l library, kinds []Kind, recursive, shallow bool) ([]record, error) {
	records, err := s.musicLibraryRecords(ctx, v, l)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		return records, nil
	}
	parent := &l.item.ID
	rowKind := map[string]Kind{eclipse.TypeTrack: KindTrack, eclipse.TypeAlbum: KindAlbum, eclipse.TypeArtist: KindArtist,
		eclipse.TypePlaylist: KindMusicPlaylist}[l.catalog.Type]
	if rowKind == KindTrack && l.addon.content() == eclipse.ContentAudiobook {
		rowKind = KindAudiobook
	}
	var tracks []record
	tracksOf := func() []record {
		if tracks == nil {
			switch rowKind {
			case KindTrack, KindAudiobook:
				tracks = records
			default:
				tracks = s.expand(ctx, v, l.addon, slices.DeleteFunc(slices.Clone(records), func(r record) bool { return !v.allowsMusic(r) }))
			}
			for i := range tracks {
				tracks[i].Parent = parent
			}
		}
		return tracks
	}
	var result []record
	for _, kind := range kinds {
		switch {
		case kind == rowKind:
			result = append(result, records...)
		case !recursive:
		case shallow && rowKind != KindTrack && rowKind != KindAudiobook && !(kind == KindArtist && rowKind == KindAlbum):
		case AudioKind(kind):
			result = append(result, slices.DeleteFunc(slices.Clone(tracksOf()), func(r record) bool { return r.Kind != kind })...)
		case kind == KindAlbum && rowKind == KindArtist:
			result = append(result, s.expandArtistAlbums(ctx, v, l.addon, records, parent)...)
		case kind == KindAlbum:
			result = append(result, derived(l.addon, tracksOf(), KindAlbum, parent)...)
		case kind == KindArtist && rowKind == KindAlbum:
			result = append(result, derived(l.addon, records, KindArtist, parent)...)
		case kind == KindArtist:
			result = append(result, derived(l.addon, tracksOf(), KindArtist, parent)...)
		}
	}
	return result, nil
}

// expandArtistAlbums lists the albums of the artists of a row, reading at
// most musicExpansion of their pages.
func (s *Service) expandArtistAlbums(ctx context.Context, v view, entry installed, artists []record, parent *accounts.ID) []record {
	artists = artists[:min(len(artists), musicExpansion)]
	lists := make([][]record, len(artists))
	var g errgroup.Group
	g.SetLimit(musicFetches)
	for i, artist := range artists {
		g.Go(func() error {
			if !v.allowsMusic(artist) {
				return nil
			}
			albums, err := s.artistRecords(ctx, v, entry, artist, false)
			if err == nil {
				for j := range albums {
					albums[j].Parent = parent
				}
				lists[i] = albums
			}
			return nil
		})
	}
	_ = g.Wait()
	return slices.Concat(lists...)
}

// musicItems keeps the records the user may reach, each once, saves them
// and the albums and artists they name, and describes them.
func (s *Service) musicItems(ctx context.Context, v view, records, named []record) ([]Item, error) {
	var kept []record
	seen := map[accounts.ID]bool{}
	for _, r := range records {
		if seen[r.ID] || !v.allowsMusic(r) {
			continue
		}
		seen[r.ID] = true
		kept = append(kept, r)
	}
	if err := s.completeTracks(ctx, kept); err != nil {
		return nil, err
	}
	if err := s.save(ctx, kept); err != nil {
		return nil, err
	}
	if err := s.saveNew(ctx, named); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(kept))
	for _, r := range kept {
		items = append(items, musicItem(r))
	}
	return items, nil
}

// completeTracks completes tracks listed without some of what describes
// them, such as the search results and top tracks addons list without
// their number or year, with what an earlier listing told of them.
func (s *Service) completeTracks(ctx context.Context, records []record) error {
	var ids []accounts.ID
	for _, r := range records {
		if r.Music != nil && r.Music.Type == eclipse.TypeTrack {
			ids = append(ids, r.ID)
		}
	}
	stored, err := s.loadAll(ctx, ids)
	if err != nil {
		return err
	}
	known := make(map[accounts.ID]*musicEntry, len(stored))
	for _, r := range stored {
		if r.Music != nil {
			known[r.ID] = r.Music
		}
	}
	for i, r := range records {
		old := known[r.ID]
		if old == nil {
			continue
		}
		e := *r.Music
		e.Index = cmpOr(e.Index, old.Index)
		e.Year = cmpOr(e.Year, old.Year)
		e.Duration = cmpOr(e.Duration, old.Duration)
		e.Artwork = cmpOr(e.Artwork, old.Artwork)
		e.Format = cmpOr(e.Format, old.Format)
		e.ISRC = cmpOr(e.ISRC, old.ISRC)
		if e.Album == "" && e.AlbumID == "" {
			e.Album, e.AlbumID = old.Album, old.AlbumID
		}
		if e.AlbumArtist == "" && e.Album == old.Album {
			e.AlbumArtist = old.AlbumArtist
		}
		records[i].Music = &e
	}
	return nil
}

// musicChildren lists a music library's or music folder's children.
func (s *Service) musicChildren(ctx context.Context, v view, records []record, entry installed, start, count int) (Page, error) {
	items, err := s.musicItems(ctx, v, records, stubs(entry, records))
	if err != nil {
		return Page{}, err
	}
	return slicePage(items, start, count), nil
}

// musicDetails describes a music item the user reaches. An album, artist
// or playlist is completed from its page, which it is read for.
func (s *Service) musicDetails(ctx context.Context, v view, r record) (Item, error) {
	if !v.allowsMusic(r) {
		return Item{}, ErrNotFound
	}
	entry, _ := v.addon(*r.Addon)
	if !entry.addon.Eclipse() {
		return Item{}, ErrNotFound
	}
	e := *r.Music
	switch r.Kind {
	case KindAlbum:
		if tracks, err := s.albumTracks(ctx, v, entry, r); err == nil {
			e.Count, e.Duration = 0, 0
			for _, t := range tracks {
				if v.allowsMusic(t) {
					e.Count++
					e.Duration += t.Music.Duration
				}
			}
			if id := cmpOr(e.ID, e.Resolved); id != "" {
				if album, err := s.albumPage(ctx, entry, id); err == nil {
					e.Overview = cmpOr(album.Description, e.Overview)
					e.Artwork = cmpOr(album.ArtworkURL, e.Artwork)
					e.Year = cmpOr(album.Year, e.Year)
				}
			}
		}
	case KindArtist:
		if artist, err := s.artistOf(ctx, entry, r); err == nil {
			e.Overview = cmpOr(artist.Bio, e.Overview)
			e.Artwork = cmpOr(artist.ArtworkURL, e.Artwork)
			if len(artist.Genres) > 0 {
				e.Genres = artist.Genres
			}
			e.Count = len(artist.Albums)
		}
	case KindMusicPlaylist:
		if playlist, err := s.playlistPage(ctx, entry, e.ID); err == nil {
			found := playlistEntry(playlist, e.Content)
			e.Count, e.Duration = found.Count, found.Duration
			e.Overview = cmpOr(found.Overview, e.Overview)
		}
	}
	if !v.allowsMusic(record{Addon: r.Addon, Music: &e}) {
		return Item{}, ErrNotFound
	}
	if e.Artwork != r.Music.Artwork {
		// Its artwork is served from the record.
		saved := r
		saved.Music = &e
		if err := s.save(ctx, []record{saved}); err != nil {
			return Item{}, err
		}
	}
	r.Music = &e
	return musicItem(r), nil
}

// ArtistByName finds an artist by name among the artists the user's music
// addons' tracks and albums named, Jellyfin apps opening artists by name.
func (s *Service) ArtistByName(ctx context.Context, user accounts.User, name string) (Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Item{}, err
	}
	for _, entry := range v.addons {
		if !entry.addon.Eclipse() {
			continue
		}
		r, err := s.load(ctx, itemID(namedArtistKey(entry.addon.ID, name)))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Item{}, err
		}
		if item, err := s.musicDetails(ctx, v, r); err == nil {
			return s.overriddenItem(item), nil
		}
	}
	return Item{}, ErrNotFound
}

func cmpOr[T comparable](values ...T) T {
	var zero T
	for _, value := range values {
		if value != zero {
			return value
		}
	}
	return zero
}

// SearchMusic looks a term up in the searches of the user's music addons,
// for items of the given kinds, interleaving the addons' results.
func (s *Service) SearchMusic(ctx context.Context, user accounts.User, term string, kinds []Kind, limit int) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var entries []installed
	for _, entry := range v.addons {
		if entry.addon.Eclipse() && entry.addon.Music.Has("search") {
			entries = append(entries, entry)
		}
	}
	lists := make([][]record, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Go(func() {
			results, err := s.musicSearch(ctx, entry, term)
			if err != nil {
				if ctx.Err() == nil {
					s.logger.Warn("A music addon could not search", "addon", entry.addon.Manifest.Name, "error", err)
				}
				return
			}
			content := entry.content()
			for _, kind := range kinds {
				switch kind {
				case KindTrack, KindAudiobook:
					for _, t := range results.Tracks {
						if r := musicRecord(entry, trackEntry(t, 0, content), nil); r.Kind == kind {
							lists[i] = append(lists[i], r)
						}
					}
				case KindAlbum:
					for _, a := range results.Albums {
						lists[i] = append(lists[i], musicRecord(entry, albumEntry(a, content), nil))
					}
				case KindArtist:
					for _, a := range results.Artists {
						lists[i] = append(lists[i], musicRecord(entry, artistEntry(a, content), nil))
					}
				case KindMusicPlaylist:
					for _, p := range results.Playlists {
						lists[i] = append(lists[i], musicRecord(entry, playlistEntry(p, content), nil))
					}
				}
			}
		})
	}
	wg.Wait()
	var records, named []record
	for i, list := range lists {
		named = append(named, stubs(entries[i], list)...)
	}
	for n := 0; ; n++ {
		more := false
		for _, list := range lists {
			if n < len(list) {
				records, more = append(records, list[n]), true
			}
		}
		if !more {
			break
		}
	}
	// Search results keep the folder they were last listed in.
	items, err := s.musicItems(ctx, v, records, named)
	if err != nil {
		return nil, err
	}
	items = items[:min(len(items), max(limit, 0))]
	return s.overridden(append(items, s.renamed(ctx, v, term, kinds, items, limit)...)), nil
}

// InstantMix makes a list of tracks to play from an item, as Jellyfin's
// instant mixes: Jellyfin picks tracks sharing the item's genres, which
// addons rarely give, so Polyfin mixes what the addon itself relates to
// the item instead. A track comes first, followed by the other tracks of
// its album and of its artist; an album gives its tracks and its artist's;
// an artist, their tracks; a playlist or a music library, its tracks. The
// order is shuffled but for that first track. limit bounds the mix.
func (s *Service) InstantMix(ctx context.Context, user accounts.User, id accounts.ID, limit int) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var first []record
	var pool []record
	var named []record
	if l, ok := v.library(id); ok {
		if !l.addon.addon.Eclipse() {
			return nil, ErrNotFound
		}
		if pool, err = s.musicLibrary(ctx, v, l, []Kind{KindTrack, KindAudiobook}, true, false); err != nil {
			return nil, err
		}
		named = stubs(l.addon, pool)
	} else {
		r, err := s.load(ctx, id)
		if err != nil {
			return nil, err
		}
		if !MusicKind(r.Kind) || !v.allowsMusic(r) {
			return nil, ErrNotFound
		}
		entry, _ := v.addon(*r.Addon)
		if !entry.addon.Eclipse() {
			return nil, ErrNotFound
		}
		related := func(id accounts.ID) []record {
			folder, err := s.load(ctx, id)
			if err != nil || folder.Music == nil {
				return nil
			}
			list, err := s.folderMusic(ctx, v, entry, folder, []Kind{KindTrack})
			if err != nil {
				return nil
			}
			return list
		}
		switch {
		case AudioKind(r.Kind):
			first = []record{r}
			item := musicItem(r)
			if item.AlbumID != (accounts.ID{}) {
				pool = append(pool, related(item.AlbumID)...)
			}
			for _, artist := range item.Artists {
				pool = append(pool, related(artist.ID)...)
			}
		case r.Kind == KindAlbum:
			pool = related(r.ID)
			if item := musicItem(r); item.AlbumArtist != nil {
				pool = append(pool, related(item.AlbumArtist.ID)...)
			}
		default:
			pool, err = s.folderMusic(ctx, v, entry, r, []Kind{KindTrack})
			if err != nil {
				return nil, err
			}
		}
		named = stubs(entry, slices.Concat(first, pool))
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	pool = slices.DeleteFunc(pool, func(r record) bool { return !AudioKind(r.Kind) })
	items, err := s.musicItems(ctx, v, slices.Concat(first, pool), named)
	if err != nil {
		return nil, err
	}
	return s.overridden(items[:min(len(items), max(limit, 0))]), nil
}

// AudioSource is what an addon tells of a track's stream: its format and
// quality as labels, its codec, container and manifest ("none", "hls" or
// "dash"), sample rate and bit depth when it gives them, which playback
// trusts without probing the stream, and the chapters of an audiobook. A
// known format gives the codec and container the addon leaves out (see
// audioFormats). Resolved is when the addon gave the link.
type AudioSource struct {
	Format, Quality            string
	Codec, Container, Manifest string
	SampleRate, BitDepth       int
	Chapters                   []eclipse.Chapter
	Resolved                   time.Time
}

// audioFormats are the codec and container of the formats addons name
// tracks' files by.
var audioFormats = map[string]struct{ codec, container string }{
	"flac": {"flac", "flac"},
	"mp3":  {"mp3", "mp3"},
	"aac":  {"aac", "aac"},
	"m4a":  {"aac", "m4a"},
	"opus": {"opus", "ogg"},
	"ogg":  {"vorbis", "ogg"},
	"wav":  {"pcm_s16le", "wav"},
}

// completeFromFormat fills in the codec and container a's format gives,
// when the addon gives neither, or gives the one the format agrees with. A
// manifest (HLS or DASH) is not a file of that format.
func (a *AudioSource) completeFromFormat() {
	known, ok := audioFormats[a.Format]
	if !ok || a.Manifest == "hls" || a.Manifest == "dash" {
		return
	}
	switch {
	case a.Codec == "" && a.Container == "":
		a.Codec, a.Container = known.codec, known.container
	case a.Container == "" && a.Codec == known.codec:
		a.Container = known.container
	case a.Codec == "" && a.Container == known.container:
		a.Codec = known.codec
	}
}

// trackVersion resolves where a track plays from: the stream address the
// addon listed it with, else the one its stream resource gives, kept until
// it expires. renew asks the stream resource again, for a link that
// failed.
func (s *Service) trackVersion(ctx context.Context, v view, r record, renew bool) (Version, error) {
	if !v.allowsMusic(r) {
		return Version{}, ErrNotFound
	}
	entry, _ := v.addon(*r.Addon)
	if !entry.addon.Eclipse() {
		return Version{}, ErrNotFound
	}
	return s.resolveTrack(ctx, entry, r, renew)
}

func trackVersionID(item accounts.ID) accounts.ID {
	return itemID("version|" + item.String() + "|" + originEclipse)
}

// resolveTrack resolves where a track plays from. Requests for the same
// track at the same time share one request to the stream resource.
func (s *Service) resolveTrack(ctx context.Context, entry installed, r record, renew bool) (Version, error) {
	e := r.Music
	version := Version{ID: trackVersionID(r.ID), Item: r.ID, Name: e.Title, Addon: entry.addon.Manifest.Name,
		Origin: Origin{Addon: entry.addon.ID, Type: originEclipse, ID: e.ID}, Confined: entry.confined,
		Runtime: time.Duration(e.Duration * float64(time.Second))}
	if !renew && e.StreamURL != "" {
		version.URL = e.StreamURL
		version.Audio = &AudioSource{Format: e.Format, Resolved: s.now()}
		version.Audio.completeFromFormat()
		s.versions.Put(version.ID, version)
		return version, nil
	}
	return shared(ctx, &s.flight, "track "+r.ID.String(), func(ctx context.Context) (Version, error) {
		stream, err := s.music.Stream(ctx, entry.eclipseAddon(), e.ID)
		if errors.Is(err, stremio.ErrNotFound) {
			return Version{}, ErrNotFound
		}
		if err != nil {
			return Version{}, err
		}
		version.URL, version.Expires = stream.URL, stream.Expires
		version.Name = cmpOr(stream.Quality, e.Title)
		version.Audio = &AudioSource{Format: cmpOr(stream.Format, e.Format), Quality: stream.Quality, Codec: stream.Codec,
			Container: stream.Container, Manifest: stream.Manifest, SampleRate: stream.SampleRate, BitDepth: stream.BitDepth,
			Chapters: stream.Chapters, Resolved: s.now()}
		version.Audio.completeFromFormat()
		s.versions.Put(version.ID, version)
		return version, nil
	})
}

// fresh reports whether a version's link may still be used at now: until
// streamMargin before it expires, or, given less than justResolved ago,
// until it expires, since asking again would give a link as short-lived.
func (v Version) fresh(now time.Time) bool {
	if v.Expires.IsZero() || now.Add(streamMargin).Before(v.Expires) {
		return true
	}
	return v.Audio != nil && now.Sub(v.Audio.Resolved) < justResolved && now.Before(v.Expires)
}
