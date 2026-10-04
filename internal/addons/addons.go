// Package addons stores the Stremio and Eclipse addons of the server and of
// each user, and which of their catalogs are shown as libraries.
package addons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// DefaultLibraries bounds how many libraries a scope gets automatically when
// addons are installed; it is not a limit on the libraries an administrator
// enables. Each library adds a row to the home screen of Jellyfin apps.
const DefaultLibraries = 20

var (
	ErrNotFound           = errors.New("addon not found")
	ErrExists             = errors.New("addon already installed")
	ErrInvalidOrder       = errors.New("the order must list every addon of the scope once")
	ErrInvalidLibrary     = errors.New("unknown or unbrowsable catalog")
	ErrInvalidLibraryName = errors.New("library names are 1 to 64 printable characters")
	ErrInvalidGuideURL    = errors.New("guide addresses are http or https URLs of at most 4096 characters")
	ErrTooManyGuides      = errors.New("a live TV catalog has at most 10 guides")
	ErrInvalidGuide       = errors.New("unknown guide")
	// ErrNotStremio reports an IPTV source asked for a manifest.
	ErrNotStremio = errors.New("not a Stremio addon")
	// ErrNotEclipse reports settings given to an addon that has none.
	ErrNotEclipse = errors.New("not an Eclipse addon")
	// ErrKindChanged reports a new address of an addon that serves another
	// kind of addon than it did.
	ErrKindChanged = errors.New("the address serves another kind of addon")
)

// Scope owns addons: the server (no owner) or one user.
type Scope struct {
	Owner *accounts.ID
}

// Shared is the scope of the server's addons.
func Shared() Scope { return Scope{} }

// Personal is the scope of a user's own addons.
func Personal(user accounts.ID) Scope { return Scope{Owner: &user} }

// Kinds of addons: Stremio addons and Eclipse music addons, installed from
// their manifest, and Polyfin's own IPTV sources, an M3U playlist or an
// Xtream Codes account, which answer as an addon with one live TV catalog
// (see package iptv).
const (
	KindStremio = "stremio"
	KindEclipse = "eclipse"
	KindM3U     = "m3u"
	KindXtream  = "xtream"
)

// Addon is an installed Stremio or Eclipse addon, or an IPTV source. For an
// IPTV source, ManifestURL is the address of its list, which embeds its
// credentials, and Manifest describes its live TV catalog. For an Eclipse
// addon, Music is its manifest, Manifest what libraries see of it (its
// name, icon and catalog rows), and Settings the values chosen for its
// settings.
type Addon struct {
	ID          accounts.ID
	Kind        string
	ManifestURL string
	Manifest    stremio.Manifest
	Music       *eclipse.Manifest
	Settings    map[string]string
	Enabled     bool
	RefreshedAt time.Time
}

// Stremio reports whether the addon is a Stremio addon, whose resources
// are asked for over HTTP.
func (a Addon) Stremio() bool { return a.Kind == KindStremio }

// IPTV reports whether the addon is one of Polyfin's own IPTV sources.
func (a Addon) IPTV() bool { return a.Kind == KindM3U || a.Kind == KindXtream }

// Eclipse reports whether the addon is an Eclipse music addon.
func (a Addon) Eclipse() bool { return a.Kind == KindEclipse && a.Music != nil }

// Library is a catalog of an addon of the scope, shown as a library when
// Enabled.
type Library struct {
	AddonID     accounts.ID
	AddonName   string
	Catalog     stremio.Catalog
	Name        *string
	Enabled     bool
	AddonActive bool
	// Guides are the XMLTV guides of an enabled live TV catalog, in order;
	// nil for any other library. GuideChannels counts the catalog's channels
	// when they were last mapped to guide channels, GuideMapped those mapped.
	Guides        []Guide
	GuideChannels int
	GuideMapped   int
}

// MaxGuides bounds the guides of a live TV catalog.
const MaxGuides = 10

// Guide is an XMLTV guide of a live TV catalog: its address, which may
// embed credentials, and how its last download went. FetchedAt is the last
// success, CheckedAt the last attempt; Channels and Programmes count what
// the last success held, Generation names it; Error is the code of the last
// failure, empty after a success.
type Guide struct {
	ID         accounts.ID
	Position   int
	URL        string
	Generation int
	CheckedAt  *time.Time
	FetchedAt  *time.Time
	Channels   int
	Programmes int
	Error      string
}

const guideColumns = "g.id, g.position, g.url, g.generation, g.checked_at, g.fetched_at, g.channels, g.programmes, g.error"

func scanGuide(row pgx.CollectableRow) (Guide, error) {
	var g Guide
	err := row.Scan(&g.ID, &g.Position, &g.URL, &g.Generation, &g.CheckedAt, &g.FetchedAt, &g.Channels, &g.Programmes, &g.Error)
	return g, err
}

// LibraryKey identifies a catalog of an addon.
type LibraryKey struct {
	AddonID     accounts.ID
	CatalogType string
	CatalogID   string
}

// LibraryChoice selects a catalog as a library, with an optional name.
type LibraryChoice struct {
	AddonID     accounts.ID
	CatalogType string
	CatalogID   string
	Name        *string
}

// Store is the addons repository.
type Store struct {
	db     *pgxpool.Pool
	client *stremio.Client
}

func New(db *pgxpool.Pool, client *stremio.Client) *Store {
	return &Store{db: db, client: client}
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanAddon(row pgx.Row) (Addon, error) {
	var addon Addon
	var manifest []byte
	err := row.Scan(&addon.ID, &addon.Kind, &addon.ManifestURL, &manifest, &addon.Settings, &addon.Enabled, &addon.RefreshedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return addon, ErrNotFound
	}
	if err != nil {
		return addon, err
	}
	if addon.Kind == KindEclipse {
		var music eclipse.Manifest
		if err := json.Unmarshal(manifest, &music); err != nil {
			return addon, err
		}
		addon.Music, addon.Manifest = &music, music.Stremio()
		return addon, nil
	}
	return addon, json.Unmarshal(manifest, &addon.Manifest)
}

const addonColumns = "id, kind, manifest_url, manifest, settings, enabled, refreshed_at"

// Addons lists the scope's addons in order.
func (s *Store) Addons(ctx context.Context, scope Scope) ([]Addon, error) {
	return addons(ctx, s.db, scope)
}

func addons(ctx context.Context, db queryer, scope Scope) ([]Addon, error) {
	rows, err := db.Query(ctx, "SELECT "+addonColumns+" FROM addons WHERE owner_id IS NOT DISTINCT FROM $1 ORDER BY position, created_at", scope.Owner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Addon, error) { return scanAddon(row) })
}

func (s *Store) addon(ctx context.Context, db queryer, scope Scope, id accounts.ID) (Addon, error) {
	return scanAddon(db.QueryRow(ctx, "SELECT "+addonColumns+" FROM addons WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2", id, scope.Owner))
}

// Find returns an installed addon by identifier, whoever installed it.
func (s *Store) Find(ctx context.Context, id accounts.ID) (Addon, error) {
	return scanAddon(s.db.QueryRow(ctx, "SELECT "+addonColumns+" FROM addons WHERE id = $1", id))
}

// fetched is a downloaded manifest: its normalized URL, the kind of addon
// it describes, what is stored of it, and what libraries see of it.
type fetched struct {
	url      string
	kind     string
	encoded  []byte
	manifest stremio.Manifest
}

// fetch normalizes a manifest URL and downloads the manifest, telling a
// Stremio addon's from an Eclipse addon's by what it declares. An Eclipse
// addon may also be given by its base address, without /manifest.json,
// as Eclipse takes it. confined keeps the request on public addresses (see
// stremio.Client).
func (s *Store) fetch(ctx context.Context, rawURL string, confined bool) (fetched, error) {
	manifestURL, err := stremio.NormalizeManifestURL(rawURL)
	based := false
	if errors.Is(err, stremio.ErrInvalidManifestURL) {
		if candidate, ok := eclipseManifestURL(rawURL); ok {
			manifestURL, err, based = candidate, nil, true
		}
	}
	if err != nil {
		return fetched{}, err
	}
	body, err := s.client.Fetch(ctx, manifestURL, manifestURL, confined)
	switch {
	case errors.Is(err, stremio.ErrNotFound):
		return fetched{}, fmt.Errorf("%w: HTTP 404", stremio.ErrUnreachable)
	case errors.Is(err, stremio.ErrInvalidResponse):
		return fetched{}, fmt.Errorf("%w: %v", stremio.ErrInvalidManifest, err)
	case err != nil:
		return fetched{}, err
	}
	if eclipse.Detect(body) {
		music, err := eclipse.ParseManifest(body)
		if err != nil {
			return fetched{}, fmt.Errorf("%w: %w", stremio.ErrInvalidManifest, err)
		}
		encoded, err := json.Marshal(music)
		return fetched{url: manifestURL, kind: KindEclipse, encoded: encoded, manifest: music.Stremio()}, err
	}
	if based {
		// Only Eclipse addons are installed by their base address.
		return fetched{}, stremio.ErrInvalidManifestURL
	}
	manifest, err := stremio.ParseManifest(body)
	if err != nil {
		return fetched{}, err
	}
	encoded, err := json.Marshal(manifest)
	return fetched{url: manifestURL, kind: KindStremio, encoded: encoded, manifest: manifest}, err
}

// eclipseManifestURL is the manifest URL of an addon given by its base
// address, such as https://addon.example/{token}/: an http or https URL
// without credentials nor query, naming a folder rather than a file;
// the manifest is /manifest.json in it.
func eclipseManifestURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	// A base address is a folder: an address naming a file is no addon's.
	if last := parsed.Path[strings.LastIndex(parsed.Path, "/")+1:]; strings.Contains(last, ".") {
		return "", false
	}
	parsed.Scheme, parsed.Fragment, parsed.RawFragment = scheme, "", ""
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/manifest.json"
	parsed.RawPath = ""
	return parsed.String(), true
}

// Install adds an addon at the end of the scope and enables some of its
// catalogs as libraries (see enableDefaults).
func (s *Store) Install(ctx context.Context, scope Scope, rawURL string, confined bool) (Addon, error) {
	f, err := s.fetch(ctx, rawURL, confined)
	if err != nil {
		return Addon{}, err
	}
	var addon Addon
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockScope(ctx, tx, scope); err != nil {
			return err
		}
		addon, err = scanAddon(tx.QueryRow(ctx, `INSERT INTO addons (owner_id, kind, manifest_url, manifest, position)
			VALUES ($1, $2, $3, $4, (SELECT coalesce(max(position), 0) + 1 FROM addons WHERE owner_id IS NOT DISTINCT FROM $1))
			RETURNING `+addonColumns, scope.Owner, f.kind, f.url, f.encoded))
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrExists
		}
		if err != nil {
			return err
		}
		return enableDefaults(ctx, tx, scope, addon)
	})
	return addon, err
}

// Create adds an IPTV source of kind at the end of the scope, with address
// as its manifest URL and the manifest build makes from its new
// identifier; setup stores, in the same transaction, what the source needs.
// Its live TV catalog is enabled whatever the number of libraries: it is
// the source's reason to be.
func (s *Store) Create(ctx context.Context, scope Scope, kind, address string, build func(accounts.ID) stremio.Manifest,
	setup func(pgx.Tx, Addon) error) (Addon, error) {
	var addon Addon
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockScope(ctx, tx, scope); err != nil {
			return err
		}
		var id accounts.ID
		err := tx.QueryRow(ctx, `INSERT INTO addons (owner_id, kind, manifest_url, manifest, position)
			VALUES ($1, $2, $3, '{}', (SELECT coalesce(max(position), 0) + 1 FROM addons WHERE owner_id IS NOT DISTINCT FROM $1))
			RETURNING id`, scope.Owner, kind, address).Scan(&id)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrExists
		}
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(build(id))
		if err != nil {
			return err
		}
		if addon, err = scanAddon(tx.QueryRow(ctx, "UPDATE addons SET manifest = $2 WHERE id = $1 RETURNING "+addonColumns, id, encoded)); err != nil {
			return err
		}
		var last int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(l.position), 0) FROM libraries l
			JOIN addons a ON a.id = l.addon_id WHERE a.owner_id IS NOT DISTINCT FROM $1`, scope.Owner).Scan(&last); err != nil {
			return err
		}
		for i, catalog := range addon.Manifest.Catalogs {
			if _, err := tx.Exec(ctx, "INSERT INTO libraries (addon_id, catalog_type, catalog_id, position) VALUES ($1, $2, $3, $4)",
				addon.ID, catalog.Type, catalog.ID, last+i+1); err != nil {
				return err
			}
		}
		return setup(tx, addon)
	})
	return addon, err
}

// Update changes an IPTV source's address and manifest, keeping its
// libraries; setup stores, in the same transaction, what changes with
// them. It answers ErrNotFound for an addon of another kind.
func (s *Store) Update(ctx context.Context, scope Scope, id accounts.ID, kind, address string, manifest stremio.Manifest,
	setup func(pgx.Tx) error) (Addon, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Addon{}, err
	}
	var addon Addon
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		addon, err = scanAddon(tx.QueryRow(ctx, `UPDATE addons SET manifest_url = $4, manifest = $5, refreshed_at = now()
			WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2 AND kind = $3 RETURNING `+addonColumns, id, scope.Owner, kind, address, encoded))
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrExists
		}
		if err != nil {
			return err
		}
		return setup(tx)
	})
	return addon, err
}

// enableDefaults makes libraries of a new addon's catalogs. An addon that
// groups its catalogs into collections (such as AIOMetadata) already says how
// to organize them: its browsable collection catalogs become the libraries.
// Otherwise its browsable movie, series and live TV catalogs do, or an
// Eclipse addon's catalog rows, until the scope has DefaultLibraries.
func enableDefaults(ctx context.Context, tx pgx.Tx, scope Scope, addon Addon) error {
	var count, last int
	err := tx.QueryRow(ctx, `SELECT count(*), coalesce(max(l.position), 0) FROM libraries l
		JOIN addons a ON a.id = l.addon_id WHERE a.owner_id IS NOT DISTINCT FROM $1`, scope.Owner).Scan(&count, &last)
	if err != nil {
		return err
	}
	wanted := func(catalog stremio.Catalog) bool {
		return catalog.Type == "movie" || catalog.Type == "series" || catalog.Type == "tv"
	}
	if addon.Eclipse() {
		wanted = func(catalog stremio.Catalog) bool { return eclipse.CatalogType(catalog.Type) }
	} else if slices.ContainsFunc(addon.Manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == "collection" && c.Browsable() }) {
		wanted = func(catalog stremio.Catalog) bool { return catalog.Type == "collection" }
	}
	for _, catalog := range addon.Manifest.Catalogs {
		if count >= DefaultLibraries {
			break
		}
		if !catalog.Browsable() || !wanted(catalog) {
			continue
		}
		last++
		count++
		if _, err := tx.Exec(ctx, "INSERT INTO libraries (addon_id, catalog_type, catalog_id, position) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING",
			addon.ID, catalog.Type, catalog.ID, last); err != nil {
			return err
		}
	}
	return nil
}

// lockScope serializes changes to the order of a scope's addons and
// libraries.
func lockScope(ctx context.Context, tx pgx.Tx, scope Scope) error {
	key := "polyfin.addons.shared"
	if scope.Owner != nil {
		key = "polyfin.addons." + scope.Owner.String()
	}
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", key)
	return err
}

// SetEnabled turns an addon on or off; a disabled addon keeps its libraries.
func (s *Store) SetEnabled(ctx context.Context, scope Scope, id accounts.ID, enabled bool) (Addon, error) {
	return scanAddon(s.db.QueryRow(ctx, "UPDATE addons SET enabled = $3 WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2 RETURNING "+addonColumns,
		id, scope.Owner, enabled))
}

// SetSettings replaces the values chosen for an Eclipse addon's settings,
// checked against its manifest (see eclipse.Manifest.CheckSettings). A
// setting left out is sent with its default.
func (s *Store) SetSettings(ctx context.Context, scope Scope, id accounts.ID, values map[string]string) (Addon, error) {
	var addon Addon
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		current, err := scanAddon(tx.QueryRow(ctx, "SELECT "+addonColumns+" FROM addons WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2 FOR UPDATE",
			id, scope.Owner))
		if err != nil {
			return err
		}
		if !current.Eclipse() {
			return ErrNotEclipse
		}
		checked, err := current.Music.CheckSettings(values)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(checked)
		if err != nil {
			return err
		}
		addon, err = scanAddon(tx.QueryRow(ctx, "UPDATE addons SET settings = $2 WHERE id = $1 RETURNING "+addonColumns, id, encoded))
		return err
	})
	return addon, err
}

// Replace installs a new manifest URL for an addon, keeping the libraries
// whose catalogs still exist. The address must serve the same kind of
// addon: a Stremio addon stays one, and so does an Eclipse addon.
func (s *Store) Replace(ctx context.Context, scope Scope, id accounts.ID, rawURL string, confined bool) (Addon, error) {
	addon, err := s.addon(ctx, s.db, scope, id)
	if err != nil {
		return Addon{}, err
	} else if !addon.Stremio() && !addon.Eclipse() {
		return Addon{}, ErrNotStremio
	}
	f, err := s.fetch(ctx, rawURL, confined)
	if err != nil {
		return Addon{}, err
	}
	if f.kind != addon.Kind {
		return Addon{}, ErrKindChanged
	}
	return s.store(ctx, scope, id, f)
}

// Refresh downloads an addon's manifest again.
func (s *Store) Refresh(ctx context.Context, scope Scope, id accounts.ID, confined bool) (Addon, error) {
	addon, err := s.addon(ctx, s.db, scope, id)
	if err != nil {
		return Addon{}, err
	}
	if !addon.Stremio() && !addon.Eclipse() {
		return Addon{}, ErrNotStremio
	}
	f, err := s.fetch(ctx, addon.ManifestURL, confined)
	if err != nil {
		return Addon{}, err
	}
	if f.kind != addon.Kind {
		return Addon{}, ErrKindChanged
	}
	return s.store(ctx, scope, id, f)
}

func (s *Store) store(ctx context.Context, scope Scope, id accounts.ID, f fetched) (Addon, error) {
	manifestURL, encoded, manifest := f.url, f.encoded, f.manifest
	var addon Addon
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		addon, err = scanAddon(tx.QueryRow(ctx, `UPDATE addons SET manifest_url = $3, manifest = $4, refreshed_at = now()
			WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2 RETURNING `+addonColumns, id, scope.Owner, manifestURL, encoded))
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrExists
		}
		if err != nil {
			return err
		}
		// Forget libraries whose catalog the addon no longer offers.
		rows, err := tx.Query(ctx, "SELECT catalog_type, catalog_id FROM libraries WHERE addon_id = $1", id)
		if err != nil {
			return err
		}
		var catalogType, catalogID string
		var gone [][2]string
		if _, err := pgx.ForEachRow(rows, []any{&catalogType, &catalogID}, func() error {
			if _, ok := manifest.Catalog(catalogType, catalogID); !ok {
				gone = append(gone, [2]string{catalogType, catalogID})
			}
			return nil
		}); err != nil {
			return err
		}
		for _, key := range gone {
			if _, err := tx.Exec(ctx, "DELETE FROM libraries WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3", id, key[0], key[1]); err != nil {
				return err
			}
		}
		return nil
	})
	return addon, err
}

// Remove uninstalls an addon with its libraries.
func (s *Store) Remove(ctx context.Context, scope Scope, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM addons WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2", id, scope.Owner)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Reorder sets the order of the scope's addons; ids must list each of them
// exactly once.
func (s *Store) Reorder(ctx context.Context, scope Scope, ids []accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockScope(ctx, tx, scope); err != nil {
			return err
		}
		current, err := addons(ctx, tx, scope)
		if err != nil {
			return err
		}
		if len(ids) != len(current) {
			return ErrInvalidOrder
		}
		for _, addon := range current {
			if !slices.Contains(ids, addon.ID) {
				return ErrInvalidOrder
			}
		}
		for position, id := range ids {
			if _, err := tx.Exec(ctx, "UPDATE addons SET position = $2 WHERE id = $1", id, position+1); err != nil {
				return err
			}
		}
		return nil
	})
}

// Libraries lists every catalog of the scope's addons: enabled libraries
// first in their order, then the other catalogs by addon and manifest order.
func (s *Store) Libraries(ctx context.Context, scope Scope) ([]Library, error) {
	return libraries(ctx, s.db, scope)
}

func libraries(ctx context.Context, db queryer, scope Scope) ([]Library, error) {
	installed, err := addons(ctx, db, scope)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT l.addon_id, l.catalog_type, l.catalog_id, l.name, l.guide_channels, l.guide_matched FROM libraries l
		JOIN addons a ON a.id = l.addon_id WHERE a.owner_id IS NOT DISTINCT FROM $1 ORDER BY l.position`, scope.Owner)
	if err != nil {
		return nil, err
	}
	type key struct {
		addon       accounts.ID
		kind, catID string
	}
	var enabled []Library
	chosen := map[key]bool{}
	var row struct {
		addon             accounts.ID
		kind, catID       string
		name              *string
		channels, matched int
	}
	if _, err := pgx.ForEachRow(rows, []any{&row.addon, &row.kind, &row.catID, &row.name, &row.channels, &row.matched}, func() error {
		index := slices.IndexFunc(installed, func(a Addon) bool { return a.ID == row.addon })
		if index < 0 {
			return nil
		}
		addon := installed[index]
		catalog, ok := addon.Manifest.Catalog(row.kind, row.catID)
		if !ok {
			return nil
		}
		chosen[key{row.addon, row.kind, row.catID}] = true
		library := Library{AddonID: addon.ID, AddonName: addon.Manifest.Name, Catalog: catalog,
			Name: row.name, Enabled: true, AddonActive: addon.Enabled}
		if catalog.Type == "tv" {
			library.Guides, library.GuideChannels, library.GuideMapped = []Guide{}, row.channels, row.matched
		}
		enabled = append(enabled, library)
		return nil
	}); err != nil {
		return nil, err
	}
	rows, err = db.Query(ctx, `SELECT g.addon_id, g.catalog_type, g.catalog_id, `+guideColumns+` FROM live_guides g
		JOIN addons a ON a.id = g.addon_id WHERE a.owner_id IS NOT DISTINCT FROM $1 ORDER BY g.position`, scope.Owner)
	if err != nil {
		return nil, err
	}
	var g Guide
	var k key
	if _, err := pgx.ForEachRow(rows, []any{&k.addon, &k.kind, &k.catID, &g.ID, &g.Position, &g.URL, &g.Generation, &g.CheckedAt,
		&g.FetchedAt, &g.Channels, &g.Programmes, &g.Error}, func() error {
		for i := range enabled {
			if l := &enabled[i]; l.AddonID == k.addon && l.Catalog.Type == k.kind && l.Catalog.ID == k.catID && l.Guides != nil {
				l.Guides = append(l.Guides, g)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	result := enabled
	for _, addon := range installed {
		for _, catalog := range addon.Manifest.Catalogs {
			if chosen[key{addon.ID, catalog.Type, catalog.ID}] {
				continue
			}
			result = append(result, Library{AddonID: addon.ID, AddonName: addon.Manifest.Name, Catalog: catalog,
				AddonActive: addon.Enabled})
		}
	}
	return result, nil
}

// SetLibraries replaces the scope's libraries with choices, in order.
func (s *Store) SetLibraries(ctx context.Context, scope Scope, choices []LibraryChoice) ([]Library, error) {
	for i, choice := range choices {
		if choice.Name == nil {
			continue
		}
		name := strings.TrimSpace(*choice.Name)
		if name == "" {
			choices[i].Name = nil
			continue
		}
		if !validName(name) {
			return nil, ErrInvalidLibraryName
		}
		choices[i].Name = &name
	}
	var result []Library
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockScope(ctx, tx, scope); err != nil {
			return err
		}
		installed, err := addons(ctx, tx, scope)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, choice := range choices {
			index := slices.IndexFunc(installed, func(a Addon) bool { return a.ID == choice.AddonID })
			if index < 0 {
				return ErrInvalidLibrary
			}
			catalog, ok := installed[index].Manifest.Catalog(choice.CatalogType, choice.CatalogID)
			unique := fmt.Sprintf("%s\x00%s\x00%s", choice.AddonID, choice.CatalogType, choice.CatalogID)
			if !ok || !catalog.Browsable() || seen[unique] {
				return ErrInvalidLibrary
			}
			seen[unique] = true
		}
		// Libraries kept keep their guide: only those dropped are deleted,
		// with their guides.
		rows, err := tx.Query(ctx, `SELECT addon_id, catalog_type, catalog_id FROM libraries WHERE addon_id IN
			(SELECT id FROM addons WHERE owner_id IS NOT DISTINCT FROM $1)`, scope.Owner)
		if err != nil {
			return err
		}
		var existing LibraryKey
		var dropped []LibraryKey
		if _, err := pgx.ForEachRow(rows, []any{&existing.AddonID, &existing.CatalogType, &existing.CatalogID}, func() error {
			if !seen[fmt.Sprintf("%s\x00%s\x00%s", existing.AddonID, existing.CatalogType, existing.CatalogID)] {
				dropped = append(dropped, existing)
			}
			return nil
		}); err != nil {
			return err
		}
		for _, key := range dropped {
			if _, err := tx.Exec(ctx, "DELETE FROM libraries WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3",
				key.AddonID, key.CatalogType, key.CatalogID); err != nil {
				return err
			}
		}
		for position, choice := range choices {
			if _, err := tx.Exec(ctx, `INSERT INTO libraries (addon_id, catalog_type, catalog_id, name, position) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (addon_id, catalog_type, catalog_id) DO UPDATE SET name = excluded.name, position = excluded.position`,
				choice.AddonID, choice.CatalogType, choice.CatalogID, choice.Name, position+1); err != nil {
				return err
			}
		}
		result, err = libraries(ctx, tx, scope)
		return err
	})
	return result, err
}

// validGuideURL checks a guide address.
func validGuideURL(guideURL string) bool {
	parsed, err := url.Parse(guideURL)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && len(guideURL) <= 4096
}

// lockCatalog locks one of the scope's enabled live TV catalogs in tx.
func lockCatalog(ctx context.Context, tx pgx.Tx, scope Scope, key LibraryKey) error {
	var found int
	err := tx.QueryRow(ctx, `SELECT 1 FROM libraries l JOIN addons a ON a.id = l.addon_id
		WHERE l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3 AND l.catalog_type = 'tv'
		AND a.owner_id IS NOT DISTINCT FROM $4 FOR UPDATE OF l`, key.AddonID, key.CatalogType, key.CatalogID, scope.Owner).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidLibrary
	}
	return err
}

func catalogGuides(ctx context.Context, db queryer, key LibraryKey) ([]Guide, error) {
	rows, err := db.Query(ctx, "SELECT "+guideColumns+` FROM live_guides g
		WHERE g.addon_id = $1 AND g.catalog_type = $2 AND g.catalog_id = $3 ORDER BY g.position`, key.AddonID, key.CatalogType, key.CatalogID)
	if err != nil {
		return nil, err
	}
	guides, err := pgx.CollectRows(rows, scanGuide)
	if guides == nil {
		guides = []Guide{}
	}
	return guides, err
}

// SetGuide sets the address of the first XMLTV guide of one of the scope's
// enabled live TV catalogs; an empty address removes that guide. A new
// address forgets what the previous one gave. It reports whether the
// address changed.
func (s *Store) SetGuide(ctx context.Context, scope Scope, key LibraryKey, rawURL string) (bool, error) {
	guideURL := strings.TrimSpace(rawURL)
	if guideURL != "" && !validGuideURL(guideURL) {
		return false, ErrInvalidGuideURL
	}
	changed := false
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockCatalog(ctx, tx, scope, key); err != nil {
			return err
		}
		guides, err := catalogGuides(ctx, tx, key)
		if err != nil {
			return err
		}
		switch {
		case len(guides) > 0 && guides[0].URL == guideURL, len(guides) == 0 && guideURL == "":
			return nil
		case len(guides) > 0:
			_, err = tx.Exec(ctx, "DELETE FROM live_guides WHERE id = $1", guides[0].ID)
			if err != nil {
				return err
			}
		}
		changed = true
		if guideURL == "" {
			_, err = tx.Exec(ctx, `UPDATE live_guides SET position = position - 1 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
				key.AddonID, key.CatalogType, key.CatalogID)
			if err != nil {
				return err
			}
			return recountMapped(ctx, tx, key)
		}
		_, err = tx.Exec(ctx, "INSERT INTO live_guides (addon_id, catalog_type, catalog_id, position, url) VALUES ($1, $2, $3, 1, $4)",
			key.AddonID, key.CatalogType, key.CatalogID, guideURL)
		return err
	})
	return changed, err
}

// GuideAddress is a guide of SetGuides: one already in the list, by ID,
// or an address.
type GuideAddress struct {
	ID  *accounts.ID
	URL string
}

// SetGuides replaces the XMLTV guides of one of the scope's enabled live
// TV catalogs, in order. A guide given by its ID, or whose address is
// already in the list, keeps what it downloaded. It reports the guides
// that are new.
func (s *Store) SetGuides(ctx context.Context, scope Scope, key LibraryKey, list []GuideAddress) ([]accounts.ID, error) {
	if len(list) > MaxGuides {
		return nil, ErrTooManyGuides
	}
	for i := range list {
		list[i].URL = strings.TrimSpace(list[i].URL)
		if list[i].ID == nil && !validGuideURL(list[i].URL) {
			return nil, ErrInvalidGuideURL
		}
	}
	var added []accounts.ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockCatalog(ctx, tx, scope, key); err != nil {
			return err
		}
		current, err := catalogGuides(ctx, tx, key)
		if err != nil {
			return err
		}
		kept := map[accounts.ID]int{}
		for position, entry := range list {
			index := slices.IndexFunc(current, func(g Guide) bool {
				_, taken := kept[g.ID]
				return !taken && (entry.ID != nil && g.ID == *entry.ID || entry.ID == nil && g.URL == entry.URL)
			})
			if index < 0 {
				if entry.ID != nil {
					return ErrInvalidGuide
				}
				continue
			}
			kept[current[index].ID] = position + 1
		}
		for _, g := range current {
			if _, ok := kept[g.ID]; !ok {
				if _, err := tx.Exec(ctx, "DELETE FROM live_guides WHERE id = $1", g.ID); err != nil {
					return err
				}
			}
		}
		// Positions are moved out of the way before they are set.
		if _, err := tx.Exec(ctx, `UPDATE live_guides SET position = position + 1000 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
			key.AddonID, key.CatalogType, key.CatalogID); err != nil {
			return err
		}
		for position, entry := range list {
			index := slices.IndexFunc(current, func(g Guide) bool { return kept[g.ID] == position+1 })
			if index >= 0 {
				if _, err := tx.Exec(ctx, "UPDATE live_guides SET position = $2 WHERE id = $1", current[index].ID, position+1); err != nil {
					return err
				}
				continue
			}
			var id accounts.ID
			if err := tx.QueryRow(ctx, `INSERT INTO live_guides (addon_id, catalog_type, catalog_id, position, url) VALUES ($1, $2, $3, $4, $5)
				RETURNING id`, key.AddonID, key.CatalogType, key.CatalogID, position+1, entry.URL).Scan(&id); err != nil {
				return err
			}
			added = append(added, id)
		}
		return recountMapped(ctx, tx, key)
	})
	return added, err
}

// recountMapped counts again a catalog's channels mapped to a guide
// channel, once guides went away with their mappings.
func recountMapped(ctx context.Context, tx pgx.Tx, key LibraryKey) error {
	_, err := tx.Exec(ctx, `UPDATE libraries l SET guide_matched = least(guide_channels, (SELECT count(*) FROM live_guide_maps m
		WHERE m.addon_id = l.addon_id AND m.catalog_type = l.catalog_type AND m.catalog_id = l.catalog_id AND m.guide_id IS NOT NULL))
		WHERE l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3`, key.AddonID, key.CatalogType, key.CatalogID)
	return err
}

// LiveCatalog is a live TV catalog, and what reaching it needs: Confined is
// true when it may only reach public addresses, one of a user's own
// catalogs unless they are an administrator.
type LiveCatalog struct {
	Scope    Scope
	Key      LibraryKey
	Addon    Addon
	Catalog  stremio.Catalog
	Confined bool
	Guides   []Guide
}

// LiveCatalogs lists the enabled live TV catalogs of enabled addons, of
// every scope, with their guides, those with a guide only when guided is
// set, in no particular order.
func (s *Store) LiveCatalogs(ctx context.Context, guided bool) ([]LiveCatalog, error) {
	filter := ""
	if guided {
		filter = " AND EXISTS (SELECT 1 FROM live_guides g WHERE g.addon_id = l.addon_id AND g.catalog_type = l.catalog_type AND g.catalog_id = l.catalog_id)"
	}
	return s.liveCatalogs(ctx, filter)
}

// LiveCatalogOf returns one of the scope's enabled live TV catalogs, whether
// its addon is turned on or not.
func (s *Store) LiveCatalogOf(ctx context.Context, scope Scope, key LibraryKey) (LiveCatalog, error) {
	catalogs, err := s.liveCatalogsOf(ctx, ` AND l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3
		AND a.owner_id IS NOT DISTINCT FROM $4`, false, key.AddonID, key.CatalogType, key.CatalogID, scope.Owner)
	if err == nil && len(catalogs) == 0 {
		err = ErrInvalidLibrary
	}
	if err != nil {
		return LiveCatalog{}, err
	}
	return catalogs[0], nil
}

func (s *Store) liveCatalogs(ctx context.Context, filter string, args ...any) ([]LiveCatalog, error) {
	return s.liveCatalogsOf(ctx, filter, true, args...)
}

func (s *Store) liveCatalogsOf(ctx context.Context, filter string, enabledOnly bool, args ...any) ([]LiveCatalog, error) {
	if enabledOnly {
		filter += " AND a.enabled"
	}
	rows, err := s.db.Query(ctx, `SELECT a.id, a.kind, a.manifest_url, a.manifest, a.enabled, a.refreshed_at, a.owner_id,
		coalesce(u.is_administrator, true), l.catalog_type, l.catalog_id
		FROM libraries l JOIN addons a ON a.id = l.addon_id LEFT JOIN users u ON u.id = a.owner_id
		WHERE l.catalog_type = 'tv' AND a.kind <> '`+KindEclipse+`'`+filter, args...)
	if err != nil {
		return nil, err
	}
	var result []LiveCatalog
	var c LiveCatalog
	var manifest []byte
	var administrator bool
	if _, err := pgx.ForEachRow(rows, []any{&c.Addon.ID, &c.Addon.Kind, &c.Addon.ManifestURL, &manifest, &c.Addon.Enabled,
		&c.Addon.RefreshedAt, &c.Scope.Owner, &administrator, &c.Key.CatalogType, &c.Key.CatalogID}, func() error {
		c.Addon.Manifest = stremio.Manifest{}
		if err := json.Unmarshal(manifest, &c.Addon.Manifest); err != nil {
			return err
		}
		catalog, ok := c.Addon.Manifest.Catalog(c.Key.CatalogType, c.Key.CatalogID)
		if !ok {
			return nil
		}
		c.Key.AddonID, c.Catalog = c.Addon.ID, catalog
		// The server's addons were installed by an administrator; a user's
		// own may only reach the local network if that user is one.
		c.Confined = c.Scope.Owner != nil && !administrator
		result = append(result, c)
		return nil
	}); err != nil {
		return nil, err
	}
	for i := range result {
		if result[i].Guides, err = catalogGuides(ctx, s.db, result[i].Key); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func validName(name string) bool {
	if utf8.RuneCountInString(name) > 64 {
		return false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// UsesSharedAddons reports whether a user sees the server's addons.
func (s *Store) UsesSharedAddons(ctx context.Context, user accounts.ID) (bool, error) {
	var uses bool
	err := s.db.QueryRow(ctx, "SELECT use_shared_addons FROM users WHERE id = $1", user).Scan(&uses)
	return uses, err
}

// SetUsesSharedAddons chooses whether a user sees the server's addons.
func (s *Store) SetUsesSharedAddons(ctx context.Context, user accounts.ID, uses bool) error {
	_, err := s.db.Exec(ctx, "UPDATE users SET use_shared_addons = $2 WHERE id = $1", user, uses)
	return err
}
