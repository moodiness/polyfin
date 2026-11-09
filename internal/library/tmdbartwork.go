package library

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Titles' artwork from TMDB.
//
// Some addons give a movie or series no poster, or one that no longer
// loads, found dead by the image relay (see ArtworkFailures). While the
// settings hold the server's TMDB key, such a title of a TMDB or IMDb
// identifier shows TMDB's poster in its place, and so its backdrop, which
// comes with the same answer, when the addon gives none or a dead one.
// They are chosen in the server's metadata language, as Edit images and
// Jellyfin choose them (see tmdb.Client.TitleArtwork): in English, those
// of the title's details, which box sets' lookups of movies read too. The
// poster address template, a local folder's artwork and an
// administrator's upload still come first. Without a key, nothing changes
// and TMDB is never asked.
//
// TMDB is asked only for the titles whose images apps ask for. A title
// not looked up yet advertises a poster tag of its own (see
// Item.TMDBPoster): without one, apps would never ask for the image, and
// looking up every title a listing holds, such as an app reading a whole
// library, would ask TMDB about titles no one sees. The image's request
// then looks the title up, in the queue of TMDB's background lookups
// ahead of the passes (see tmdb.Queue), waiting titleArtworkWait at most:
// time for the posters of a screen of search results at the queue's pace.
// The tag is the only one given before Polyfin knows whether an image
// will be served: a title TMDB has no poster of answers 404 once, the
// empty card apps then draw shows that once, and the title advertises no
// tag afterwards, so that apps draw their placeholder. A title looked up
// advertises the tags of TMDB's images, or none.
//
// What TMDB gave is kept in the database, for every user, by the
// identifier the title was looked up with and the language, and in
// memory, and asked again after titleArtworkFresh, as titles' credits
// are; TMDB's answers are kept by the client too. Another metadata
// language looks titles up again as their images are asked for. Refresh
// metadata on a title asks again. The images are relayed by Polyfin, as
// all artwork is.

const (
	// titleArtworkFresh is how long what TMDB gave of a title's artwork,
	// or that it has none, is kept before TMDB is asked again.
	titleArtworkFresh = 30 * 24 * time.Hour
	// titleArtworkWait bounds how long an image's request waits for its
	// title's lookup: twenty titles at the queue's four a second.
	titleArtworkWait = 5 * time.Second
	// titleArtworkSize is the size of TMDB's images relayed, as box sets'
	// posters are: apps' requests get them resized.
	titleArtworkSize = "original"
	// keptArtworkLoad bounds the reading of what the database keeps.
	keptArtworkLoad = 10 * time.Second
)

// artworkKey identifies a title's artwork on TMDB as it is looked up and
// kept: by the key of the title's credits and the language it is chosen
// in, empty for English (see artworkLanguage).
type artworkKey struct {
	creditsKey
	language string
}

// keptArtwork is what is kept of a title's artwork on TMDB: the title's
// TMDB identifier, 0 when TMDB does not know it, the paths of its poster
// and backdrop on the image server, each empty when TMDB has none, and
// when TMDB was asked.
type keptArtwork struct {
	tmdbID           int
	poster, backdrop string
	asked            time.Time
}

// fresh reports whether TMDB is not asked again yet.
func (k keptArtwork) fresh(now time.Time) bool { return now.Sub(k.asked) < titleArtworkFresh }

// path is the path of the image of type imageType, Primary or Backdrop.
func (k keptArtwork) path(imageType string) string {
	switch {
	case strings.EqualFold(imageType, "Primary"):
		return k.poster
	case strings.EqualFold(imageType, "Backdrop"):
		return k.backdrop
	}
	return ""
}

// titleArtworks keeps titles' artwork on TMDB in memory, what the
// database keeps read the first time it is needed.
type titleArtworks struct {
	mu     sync.Mutex
	loaded bool
	kept   map[artworkKey]keptArtwork
}

// artworkLanguage is the language titles' artwork is chosen in: the
// server's metadata language, empty for English, whose artwork is that of
// the titles' details.
func (s *Service) artworkLanguage() string {
	language := strings.TrimSpace(s.settings().Language)
	if base, _, _ := strings.Cut(strings.ToLower(language), "-"); base == "en" {
		return ""
	}
	return language
}

// titleArtworkKey is the key of the artwork of a movie or series (kind) of
// identifiers ids, in the server's metadata language; false for another
// kind, or a title of neither a TMDB nor an IMDb identifier.
func (s *Service) titleArtworkKey(kind Kind, ids map[string]string) (artworkKey, bool) {
	key, ok := titleCreditsKey(kind, ids)
	return artworkKey{key, s.artworkLanguage()}, ok
}

// keptTitleArtwork returns what is kept of a title's artwork on TMDB,
// fresh or not.
func (s *Service) keptTitleArtwork(ctx context.Context, key artworkKey) (keptArtwork, bool) {
	a := &s.titleArtwork
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.loaded {
		s.loadTitleArtworkLocked(ctx)
	}
	kept, ok := a.kept[key]
	return kept, ok
}

// loadTitleArtworkLocked reads what the database keeps of titles' artwork
// on TMDB, keeping what memory holds already. One that fails is read again
// the next time.
func (s *Service) loadTitleArtworkLocked(ctx context.Context) {
	a := &s.titleArtwork
	if a.kept == nil {
		a.kept = map[artworkKey]keptArtwork{}
	}
	ctx, cancel := context.WithTimeout(ctx, keptArtworkLoad)
	defer cancel()
	rows, err := s.db.Query(ctx, `SELECT kind, provider, id, language, COALESCE(tmdb_id, 0), poster, backdrop, asked_at FROM title_artwork`)
	if err != nil {
		s.logger.Warn("The titles' artwork kept from TMDB could not be read", "error", err)
		return
	}
	defer rows.Close()
	read := map[artworkKey]keptArtwork{}
	for rows.Next() {
		var key artworkKey
		var kind string
		var k keptArtwork
		if err := rows.Scan(&kind, &key.provider, &key.id, &key.language, &k.tmdbID, &k.poster, &k.backdrop, &k.asked); err != nil {
			s.logger.Warn("The titles' artwork kept from TMDB could not be read", "error", err)
			return
		}
		key.kind = tmdb.Kind(kind)
		read[key] = k
	}
	if err := rows.Err(); err != nil {
		s.logger.Warn("The titles' artwork kept from TMDB could not be read", "error", err)
		return
	}
	for key, k := range read {
		if _, ok := a.kept[key]; !ok {
			a.kept[key] = k
		}
	}
	a.loaded = true
}

// keepTitleArtwork keeps what TMDB gave of a title's artwork, in memory
// and in the database.
func (s *Service) keepTitleArtwork(ctx context.Context, key artworkKey, kept keptArtwork) error {
	var tmdbID *int
	if kept.tmdbID > 0 {
		tmdbID = &kept.tmdbID
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO title_artwork (kind, provider, id, language, tmdb_id, poster, backdrop, asked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (kind, provider, id, language) DO UPDATE SET tmdb_id = excluded.tmdb_id, poster = excluded.poster,
			backdrop = excluded.backdrop, asked_at = excluded.asked_at`,
		string(key.kind), key.provider, key.id, key.language, tmdbID, kept.poster, kept.backdrop, kept.asked); err != nil {
		return err
	}
	a := &s.titleArtwork
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.kept == nil {
		a.kept = map[artworkKey]keptArtwork{}
	}
	a.kept[key] = kept
	return nil
}

// titleArtworkShown reports whether movies and series lacking artwork
// show TMDB's: while the server's TMDB key is saved.
func (s *Service) titleArtworkShown() bool { return s.tmdb != nil && s.lookups.Enabled() }

// artworkDead reports whether the image relay found the artwork at url
// dead (see ArtworkFailures).
func (s *Service) artworkDead(url string) bool {
	return s.artwork != nil && s.artwork.ArtworkDead(url)
}

// lacking reports whether a title lacks the image at url: it has none, or
// a dead one.
func (s *Service) lacking(url string) bool { return url == "" || s.artworkDead(url) }

// tmdbImage is the address of a title's image on TMDB's image server, of
// its path there, empty for none or a dead one.
func (s *Service) tmdbImage(path string) string {
	if path == "" {
		return ""
	}
	url := s.tmdb.ImageURL(path, titleArtworkSize)
	if s.artworkDead(url) {
		return ""
	}
	return url
}

// posterLookupKey stands for the poster TMDB may give a title before it
// is looked up: its tag is advertised (see Item.TMDBPoster).
func posterLookupKey(key artworkKey) string {
	return "tmdb-poster:" + string(key.kind) + "/" + key.provider + "/" + key.id + "/" + key.language
}

// withTMDBArtwork gives a movie or series TMDB's poster in place of the
// one it lacks, unless templated (the poster address template gave its
// own), and TMDB's backdrop in place of the one it lacks, as kept and
// fresh; a title whose poster TMDB was not asked about yet gets the key of
// its lookup in Item.TMDBPoster. ctx bounds the reading of what the
// database keeps.
func (s *Service) withTMDBArtwork(ctx context.Context, item *Item, templated bool) {
	if !s.titleArtworkShown() || item.Kind != KindMovie && item.Kind != KindSeries {
		return
	}
	poster := !templated && s.lacking(item.Images.Primary)
	backdrop := s.lacking(item.Images.Backdrop)
	if !poster && !backdrop {
		return
	}
	key, ok := s.titleArtworkKey(item.Kind, item.ProviderIDs)
	if !ok {
		return
	}
	kept, ok := s.keptTitleArtwork(ctx, key)
	if !ok || !kept.fresh(s.now()) {
		if poster {
			item.TMDBPoster, item.PosterShape = posterLookupKey(key), ""
		}
		return
	}
	if url := s.tmdbImage(kept.poster); poster && url != "" {
		item.Images.Primary, item.PosterShape = url, ""
	}
	if url := s.tmdbImage(kept.backdrop); backdrop && url != "" {
		item.Images.Backdrop = url
	}
}

// keptTMDBImage returns the image of type imageType (Primary or Backdrop)
// TMDB gives a movie or series of identifiers ids, as kept and fresh,
// asking TMDB nothing: Artwork's in place of the one the title lacks.
func (s *Service) keptTMDBImage(ctx context.Context, kind Kind, ids map[string]string, imageType string) string {
	if !s.titleArtworkShown() || kind != KindMovie && kind != KindSeries {
		return ""
	}
	key, ok := s.titleArtworkKey(kind, ids)
	if !ok {
		return ""
	}
	kept, ok := s.keptTitleArtwork(ctx, key)
	if !ok || !kept.fresh(s.now()) {
		return ""
	}
	return s.tmdbImage(kept.path(imageType))
}

// TMDBArtwork returns the image of type imageType, Primary or Backdrop,
// TMDB gives a movie or series, for the image relay when the title has
// none of its own or its own is dead: as kept and fresh, else looked up on
// TMDB ahead of the passes, waiting titleArtworkWait at most, else as kept
// before. It reports false without a key, for another item or image type,
// a title of no TMDB or IMDb identifier, a poster the poster address
// template gives, and when TMDB has no such image, does not know the
// title, or did not answer in time.
func (s *Service) TMDBArtwork(ctx context.Context, id accounts.ID, imageType string) (Artwork, bool) {
	if !s.titleArtworkShown() || !strings.EqualFold(imageType, "Primary") && !strings.EqualFold(imageType, "Backdrop") {
		return Artwork{}, false
	}
	r, err := s.load(ctx, id)
	if err != nil || r.Meta == nil || r.Kind != KindMovie && r.Kind != KindSeries {
		return Artwork{}, false
	}
	ids := s.providerIDs(r.ID, *r.Meta)
	if _, templated := PosterAddress(s.settings().PosterTemplate, r.Kind, ids); templated && strings.EqualFold(imageType, "Primary") {
		return Artwork{}, false
	}
	key, ok := s.titleArtworkKey(r.Kind, ids)
	if !ok {
		return Artwork{}, false
	}
	kept, ok := s.keptTitleArtwork(ctx, key)
	if !ok || !kept.fresh(s.now()) {
		lookup := s.artworkLookup(key)
		s.lookups.Add([]tmdb.Lookup{lookup}, true)
		s.lookups.Wait(ctx, []string{lookup.Key}, titleArtworkWait)
		kept, _ = s.keptTitleArtwork(ctx, key)
	}
	if url := s.tmdbImage(kept.path(imageType)); url != "" {
		return Artwork{URL: url}, true
	}
	return Artwork{}, false
}

// artworkLookup is the lookup of a title's artwork on TMDB.
func (s *Service) artworkLookup(key artworkKey) tmdb.Lookup {
	return tmdb.Lookup{Key: "artwork|" + string(key.kind) + "|" + key.provider + "|" + key.id + "|" + key.language,
		Run: func(ctx context.Context) error { return s.lookUpTitleArtwork(ctx, key) }}
}

// lookUpTitleArtwork reads a title's poster and backdrop from TMDB, in the
// key's language, through its TMDB identifier, found by its IMDb one when
// that is the key, unless they are kept and fresh, and keeps them. A
// title TMDB does not know is kept as one without images; a lookup that
// fails keeps nothing, and fails with tmdb.RateLimited when TMDB refused
// it for its rate.
func (s *Service) lookUpTitleArtwork(ctx context.Context, key artworkKey) error {
	if !s.tmdb.Enabled() {
		return nil
	}
	if kept, ok := s.keptTitleArtwork(ctx, key); ok && kept.fresh(s.now()) {
		return nil
	}
	id := key.id
	var err error
	if key.provider == "imdb" {
		if id = s.learntTMDB(ctx, key.creditsKey); id == "" {
			id, err = s.tmdb.FindByIMDb(ctx, key.kind, key.id)
		}
	}
	var artwork tmdb.TitleArtwork
	if err == nil {
		artwork, err = s.tmdb.TitleArtwork(ctx, key.kind, id, key.language)
	}
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return err
	}
	kept := keptArtwork{poster: artwork.PosterPath, backdrop: artwork.BackdropPath, asked: s.now()}
	if n, convErr := strconv.Atoi(id); err == nil && convErr == nil && n > 0 {
		kept.tmdbID = n
	}
	return s.keepTitleArtwork(ctx, key, kept)
}

// refreshTitleArtwork has a title's artwork on TMDB asked for again, in
// every language, as Jellyfin's refresh of an item asks its providers
// again: what is kept grows old, and TMDB's answers kept by the client are
// forgotten, so that the next request of its image looks it up anew. It
// returns the addresses of the images kept, whose failures the refresh
// forgets.
func (s *Service) refreshTitleArtwork(ctx context.Context, kind Kind, ids map[string]string) ([]string, error) {
	title, ok := titleCreditsKey(kind, ids)
	if !ok || s.tmdb == nil {
		return nil, nil
	}
	// What the database keeps is read first, so that it is not read
	// afterwards over what is marked here.
	s.keptTitleArtwork(ctx, artworkKey{creditsKey: title})
	if _, err := s.db.Exec(ctx, `UPDATE title_artwork SET asked_at = 'epoch' WHERE kind = $1 AND provider = $2 AND id = $3`,
		string(title.kind), title.provider, title.id); err != nil {
		return nil, err
	}
	a := &s.titleArtwork
	a.mu.Lock()
	defer a.mu.Unlock()
	var urls []string
	for key, kept := range a.kept {
		if key.creditsKey != title {
			continue
		}
		if kept.tmdbID > 0 {
			s.tmdb.ForgetTitleArtwork(title.kind, strconv.Itoa(kept.tmdbID))
		}
		for _, path := range []string{kept.poster, kept.backdrop} {
			if path != "" {
				urls = append(urls, s.tmdb.ImageURL(path, titleArtworkSize))
			}
		}
		kept.asked = time.Time{}
		a.kept[key] = kept
	}
	return urls, nil
}
