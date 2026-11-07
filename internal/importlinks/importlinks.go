// Package importlinks keeps the titles each import did not find, and the
// links users make by hand from them to the movie or series they are.
//
// Every import (apps built on Stremio addons, tracking services, the
// import by name and year, other media servers) names the titles it reads
// by what its source knows them by: identifiers, or a name and a year.
// Those Polyfin does not find are kept per user and source (see Store.Save)
// with what the import would have brought of them: the played mark, its
// date and count, the resume point, the watchlist, and the rating when the
// source carries one. An import that reads its source whole replaces the
// list it kept before; an incremental one adds to it.
//
// A user, or an administrator for them, links one to an IMDb or TMDB
// identifier, or to an IMDb or TMDB title address (see Parse): the kind
// must match, a movie to a movie, a series or an episode's series to a
// series (see Store.Link). The link applies at once what was kept of the
// entry, as the import would have, and every later import of the source
// asks for the links before it resolves its titles (see Store.Linked), so
// that the link applies again whenever it meets the entry. Removing the
// link stops that; what it set stays, as imports never take data back.
package importlinks

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// The kinds of an entry.
const (
	KindMovie   = "movie"
	KindSeries  = "series"
	KindEpisode = "episode"
)

// The errors of Link and Unlink.
var (
	// ErrUnknownEntry reports an entry the user's list of the source does
	// not hold, or, to Unlink, one not linked.
	ErrUnknownEntry = errors.New("no such title not found")
	// ErrInvalidTarget reports a target that is neither an IMDb or TMDB
	// identifier nor an IMDb or TMDB title address.
	ErrInvalidTarget = errors.New("not an IMDb or TMDB identifier or address")
	// ErrKindMismatch reports a target of the other kind: a series for a
	// movie, or a movie for a series or an episode.
	ErrKindMismatch = errors.New("the target is of another kind")
	// ErrTargetNotFound reports a TMDB target Polyfin finds no title of.
	ErrTargetNotFound = errors.New("no title has this identifier")
)

// Label describes an entry to the user: its Kind (KindMovie, KindSeries
// or KindEpisode), its Name, the series' for an episode, empty when the
// source gives none, its Year, zero when unknown, IDs, the identifiers
// the source names it by as the user may read them, and for an episode its
// Season and Episode numbers.
type Label struct {
	Kind, Name, IDs string
	Year            int
	Season, Episode int
}

// Entry is a title an import did not find, and what the import would
// have brought of it.
type Entry struct {
	// Key names the title the way the source does, stable from one import
	// to the next: for an episode, its series'. Links are made by it.
	Key string
	Label
	// Reason is why the import did not find it, as the import names it.
	Reason string
	// Played marks the title played, at PlayedAt when the source tells
	// when, Plays times when it counts them.
	Played   bool
	PlayedAt *time.Time
	Plays    int
	// Resume is the title's resume point, nil when none.
	Resume *userdata.ImportedResume
	// Watchlist is the watchlist list the title goes on (see package
	// watchlist), empty when none; AddedAt is when the source added it,
	// nil when it does not tell.
	Watchlist string
	AddedAt   *time.Time
	// Rating is the user's rating, from 0 to 10, nil when none or when
	// the import does not bring ratings.
	Rating *float64
}

// Link is the title an entry was linked to: a movie or a series, by its
// IMDb identifier, its TMDB one, or both; the Name and Year of the title
// found, empty and zero when no metadata addon described it; who linked
// it, nil once that user is deleted, with their name, and when.
type Link struct {
	Kind         string
	IMDb         string
	TMDB         int
	Name         string
	Year         int
	LinkedBy     *accounts.ID
	LinkedByName string
	LinkedAt     time.Time
}

// Titles finds the items of titles, and describes them (see
// library.Service).
type Titles interface {
	Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error)
	RemoteLookup(ctx context.Context, kind library.Kind, id string) (library.RemoteResult, bool, error)
	LookUpIDs(ctx context.Context, kind library.Kind, ids map[string]string) (*library.RemoteResult, error)
}

// Options are the dependencies of the store.
type Options struct {
	DB *pgxpool.Pool
	// Settings give the thresholds resume points are kept and titles
	// played at.
	Settings  func() accounts.Settings
	Titles    Titles
	UserData  *userdata.Store
	Watchlist *watchlist.Store
	Logger    *slog.Logger
}

// Store keeps the titles imports did not find and the links made of them,
// in PostgreSQL.
type Store struct {
	db        *pgxpool.Pool
	settings  func() accounts.Settings
	titles    Titles
	userData  *userdata.Store
	watchlist *watchlist.Store
	logger    *slog.Logger
	now       func() time.Time
}

// New returns a store with options.
func New(options Options) *Store {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Store{db: options.DB, settings: options.Settings, titles: options.Titles, userData: options.UserData,
		watchlist: options.Watchlist, logger: logger, now: time.Now}
}

// Key joins the parts that name an entry into its key, as each import
// chooses them: they are lowercased and joined with ":", the empty ones
// left out.
func Key(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ":")
}

// RefKey is the key of the title ref names by its identifiers, for the
// imports that name titles by IMDb, TMDB and TVDB identifiers: the first
// it has, of the movie or series ("imdb:tt…", "tmdb:…", "tvdb:…", TVDB for
// a series only), empty when none.
func RefKey(ref library.TitleRef) string {
	kind := KindMovie
	if ref.Series || ref.Episode {
		kind = KindSeries
	}
	switch {
	case ref.IMDb != "":
		return Key(kind, "imdb", ref.IMDb)
	case ref.TMDB > 0:
		return Key(kind, "tmdb", strconv.Itoa(ref.TMDB))
	case ref.TVDB > 0 && kind == KindSeries:
		return Key(kind, "tvdb", strconv.Itoa(ref.TVDB))
	}
	return ""
}

// RefIDs describes the identifiers of ref to the user, as Label.IDs: "tt…",
// "tmdb:…" and "tvdb:…", in that order, joined by spaces.
func RefIDs(ref library.TitleRef) string {
	var ids []string
	if ref.IMDb != "" {
		ids = append(ids, ref.IMDb)
	}
	if ref.TMDB > 0 {
		ids = append(ids, "tmdb:"+strconv.Itoa(ref.TMDB))
	}
	if ref.TVDB > 0 && (ref.Series || ref.Episode) {
		ids = append(ids, "tvdb:"+strconv.Itoa(ref.TVDB))
	}
	return strings.Join(ids, " ")
}

// Linked are a user's links of one source, by entry key: what each import
// asks for before it resolves its titles (see Store.Linked).
type Linked map[string]Link

// Ref names the title the link of the entry of key and label is, as
// imports name titles to resolve them: the movie, or the series, or its
// episode of the label's season and number. It reports false when the
// entry is not linked, or linked to a title of another kind.
func (l Linked) Ref(key string, label Label) (library.TitleRef, bool) {
	link, ok := l[key]
	if !ok || (link.Kind == KindMovie) != (label.Kind == KindMovie) {
		return library.TitleRef{}, false
	}
	return link.ref(label), true
}

// ref names the title of l for the entry label describes.
func (l Link) ref(label Label) library.TitleRef {
	ref := library.TitleRef{IMDb: l.IMDb, TMDB: l.TMDB, Name: cmp.Or(l.Name, label.Name)}
	switch label.Kind {
	case KindSeries:
		ref.Series = true
	case KindEpisode:
		ref.Episode, ref.Season, ref.Number = true, label.Season, label.Episode
	}
	return ref
}

// Linked returns user's links of source: the hook every import calls
// before it resolves its titles, so that an entry linked by hand is the
// title of its link (see Linked.Ref).
func (s *Store) Linked(ctx context.Context, user accounts.ID, source string) (Linked, error) {
	rows, err := s.db.Query(ctx, `SELECT l.entry_key, `+linkColumns+` FROM import_title_links l
		LEFT JOIN users b ON b.id = l.linked_by WHERE l.user_id = $1 AND l.source = $2`, user, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	linked := Linked{}
	for rows.Next() {
		var key string
		var l scannedLink
		if err := rows.Scan(append([]any{&key}, l.targets()...)...); err != nil {
			return nil, err
		}
		linked[key] = l.link()
	}
	return linked, rows.Err()
}

// linkColumns are the columns of a link l, its linker b joined, scanned by
// scannedLink.
const linkColumns = `l.kind, l.imdb_id, l.tmdb_id, l.name, l.year, l.linked_by, coalesce(b.name, ''), l.linked_at`

// scannedLink is a link as read from import_title_links.
type scannedLink struct {
	kind, name, byName string
	imdb               *string
	tmdb, year         *int
	by                 *accounts.ID
	at                 time.Time
}

func (l *scannedLink) targets() []any {
	return []any{&l.kind, &l.imdb, &l.tmdb, &l.name, &l.year, &l.by, &l.byName, &l.at}
}

func (l scannedLink) link() Link {
	link := Link{Kind: l.kind, Name: l.name, LinkedBy: l.by, LinkedByName: l.byName, LinkedAt: l.at}
	if l.imdb != nil {
		link.IMDb = *l.imdb
	}
	if l.tmdb != nil {
		link.TMDB = *l.tmdb
	}
	if l.year != nil {
		link.Year = *l.year
	}
	return link
}
