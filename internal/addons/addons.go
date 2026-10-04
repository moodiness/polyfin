// Package addons stores the Stremio addons of the server and of each user,
// and which of their catalogs are shown as libraries.
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
)

// Scope owns addons: the server (no owner) or one user.
type Scope struct {
	Owner *accounts.ID
}

// Shared is the scope of the server's addons.
func Shared() Scope { return Scope{} }

// Personal is the scope of a user's own addons.
func Personal(user accounts.ID) Scope { return Scope{Owner: &user} }

// Addon is an installed Stremio addon.
type Addon struct {
	ID          accounts.ID
	ManifestURL string
	Manifest    stremio.Manifest
	Enabled     bool
	RefreshedAt time.Time
}

// Library is a catalog of an addon of the scope, shown as a library when
// Enabled.
type Library struct {
	AddonID     accounts.ID
	AddonName   string
	Catalog     stremio.Catalog
	Name        *string
	Enabled     bool
	AddonActive bool
	// Guide is the XMLTV guide of an enabled live TV catalog; nil for any
	// other library.
	Guide *Guide
}

// Guide is the XMLTV guide of a live TV catalog: its address, empty when
// it has none, which may embed credentials, and how its last fetch went.
// FetchedAt is the last success, CheckedAt the last attempt; Channels
// counts the catalog's channels then and Matched those the guide covers;
// Error is the code of the last failure, empty after a success.
type Guide struct {
	URL       string
	CheckedAt *time.Time
	FetchedAt *time.Time
	Channels  int
	Matched   int
	Error     string
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
	err := row.Scan(&addon.ID, &addon.ManifestURL, &manifest, &addon.Enabled, &addon.RefreshedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return addon, ErrNotFound
	}
	if err != nil {
		return addon, err
	}
	return addon, json.Unmarshal(manifest, &addon.Manifest)
}

const addonColumns = "id, manifest_url, manifest, enabled, refreshed_at"

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

// fetch normalizes a manifest URL and downloads the manifest. confined
// keeps the request on public addresses (see stremio.Client).
func (s *Store) fetch(ctx context.Context, rawURL string, confined bool) (string, stremio.Manifest, error) {
	manifestURL, err := stremio.NormalizeManifestURL(rawURL)
	if err != nil {
		return "", stremio.Manifest{}, err
	}
	manifest, err := s.client.Manifest(ctx, manifestURL, confined)
	return manifestURL, manifest, err
}

// Install adds an addon at the end of the scope and enables some of its
// catalogs as libraries (see enableDefaults).
func (s *Store) Install(ctx context.Context, scope Scope, rawURL string, confined bool) (Addon, error) {
	manifestURL, manifest, err := s.fetch(ctx, rawURL, confined)
	if err != nil {
		return Addon{}, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Addon{}, err
	}
	var addon Addon
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockScope(ctx, tx, scope); err != nil {
			return err
		}
		addon, err = scanAddon(tx.QueryRow(ctx, `INSERT INTO addons (owner_id, manifest_url, manifest, position)
			VALUES ($1, $2, $3, (SELECT coalesce(max(position), 0) + 1 FROM addons WHERE owner_id IS NOT DISTINCT FROM $1))
			RETURNING `+addonColumns, scope.Owner, manifestURL, encoded))
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

// enableDefaults makes libraries of a new addon's catalogs. An addon that
// groups its catalogs into collections (such as AIOMetadata) already says how
// to organize them: its browsable collection catalogs become the libraries.
// Otherwise its browsable movie, series and live TV catalogs do, until the
// scope has DefaultLibraries.
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
	if slices.ContainsFunc(addon.Manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == "collection" && c.Browsable() }) {
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

// Replace installs a new manifest URL for an addon, keeping the libraries
// whose catalogs still exist.
func (s *Store) Replace(ctx context.Context, scope Scope, id accounts.ID, rawURL string, confined bool) (Addon, error) {
	if _, err := s.addon(ctx, s.db, scope, id); err != nil {
		return Addon{}, err
	}
	manifestURL, manifest, err := s.fetch(ctx, rawURL, confined)
	if err != nil {
		return Addon{}, err
	}
	return s.store(ctx, scope, id, manifestURL, manifest)
}

// Refresh downloads an addon's manifest again.
func (s *Store) Refresh(ctx context.Context, scope Scope, id accounts.ID, confined bool) (Addon, error) {
	addon, err := s.addon(ctx, s.db, scope, id)
	if err != nil {
		return Addon{}, err
	}
	manifest, err := s.client.Manifest(ctx, addon.ManifestURL, confined)
	if err != nil {
		return Addon{}, err
	}
	return s.store(ctx, scope, id, addon.ManifestURL, manifest)
}

func (s *Store) store(ctx context.Context, scope Scope, id accounts.ID, manifestURL string, manifest stremio.Manifest) (Addon, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Addon{}, err
	}
	var addon Addon
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
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
	rows, err := db.Query(ctx, `SELECT l.addon_id, l.catalog_type, l.catalog_id, l.name, l.guide_url, l.guide_checked_at,
		l.guide_fetched_at, l.guide_channels, l.guide_matched, l.guide_error FROM libraries l
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
		addon       accounts.ID
		kind, catID string
		name        *string
		guide       Guide
	}
	if _, err := pgx.ForEachRow(rows, []any{&row.addon, &row.kind, &row.catID, &row.name, &row.guide.URL, &row.guide.CheckedAt,
		&row.guide.FetchedAt, &row.guide.Channels, &row.guide.Matched, &row.guide.Error}, func() error {
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
			library.Guide = new(row.guide)
		}
		enabled = append(enabled, library)
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
		// with their guide's programmes.
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

// SetGuide sets the address of the XMLTV guide of one of the scope's
// enabled live TV catalogs; an empty address removes it. A new address
// forgets the programmes and the fetch status of the previous one.
func (s *Store) SetGuide(ctx context.Context, scope Scope, key LibraryKey, rawURL string) error {
	guideURL := strings.TrimSpace(rawURL)
	if guideURL != "" {
		parsed, err := url.Parse(guideURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || len(guideURL) > 4096 {
			return ErrInvalidGuideURL
		}
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var current string
		err := tx.QueryRow(ctx, `SELECT l.guide_url FROM libraries l JOIN addons a ON a.id = l.addon_id
			WHERE l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3 AND l.catalog_type = 'tv'
			AND a.owner_id IS NOT DISTINCT FROM $4 FOR UPDATE OF l`, key.AddonID, key.CatalogType, key.CatalogID, scope.Owner).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidLibrary
		}
		if err != nil || current == guideURL {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE libraries SET guide_url = $4, guide_checked_at = NULL, guide_fetched_at = NULL,
			guide_channels = 0, guide_matched = 0, guide_error = '' WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
			key.AddonID, key.CatalogType, key.CatalogID, guideURL); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM guide_programmes WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3",
			key.AddonID, key.CatalogType, key.CatalogID)
		return err
	})
}

// GuideSource is a live TV catalog with an XMLTV guide, and what fetching
// it needs.
type GuideSource struct {
	Scope     Scope
	Key       LibraryKey
	Addon     Addon
	Catalog   stremio.Catalog
	URL       string
	CheckedAt *time.Time
	// Confined is true when the guide may only be fetched from public
	// addresses: one of a user's own catalogs, unless they are an
	// administrator.
	Confined bool
}

// GuideSources lists the catalogs with an XMLTV guide, of every scope, in
// no particular order; those of turned-off addons, or that the addon no
// longer offers, are left out.
func (s *Store) GuideSources(ctx context.Context) ([]GuideSource, error) {
	return s.guideSources(ctx, "")
}

// GuideSource returns one of the scope's catalogs with an XMLTV guide.
func (s *Store) GuideSource(ctx context.Context, scope Scope, key LibraryKey) (GuideSource, error) {
	sources, err := s.guideSources(ctx, ` AND l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3
		AND a.owner_id IS NOT DISTINCT FROM $4`, key.AddonID, key.CatalogType, key.CatalogID, scope.Owner)
	if err == nil && len(sources) == 0 {
		err = ErrInvalidLibrary
	}
	if err != nil {
		return GuideSource{}, err
	}
	return sources[0], nil
}

func (s *Store) guideSources(ctx context.Context, filter string, args ...any) ([]GuideSource, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, a.manifest_url, a.manifest, a.enabled, a.refreshed_at, a.owner_id,
		coalesce(u.is_administrator, true), l.catalog_type, l.catalog_id, l.guide_url, l.guide_checked_at
		FROM libraries l JOIN addons a ON a.id = l.addon_id LEFT JOIN users u ON u.id = a.owner_id
		WHERE l.guide_url <> '' AND a.enabled`+filter, args...)
	if err != nil {
		return nil, err
	}
	var result []GuideSource
	var source GuideSource
	var manifest []byte
	var administrator bool
	if _, err := pgx.ForEachRow(rows, []any{&source.Addon.ID, &source.Addon.ManifestURL, &manifest, &source.Addon.Enabled,
		&source.Addon.RefreshedAt, &source.Scope.Owner, &administrator, &source.Key.CatalogType, &source.Key.CatalogID,
		&source.URL, &source.CheckedAt}, func() error {
		source.Addon.Manifest = stremio.Manifest{}
		if err := json.Unmarshal(manifest, &source.Addon.Manifest); err != nil {
			return err
		}
		catalog, ok := source.Addon.Manifest.Catalog(source.Key.CatalogType, source.Key.CatalogID)
		if !ok {
			return nil
		}
		source.Key.AddonID, source.Catalog = source.Addon.ID, catalog
		// The server's addons were installed by an administrator; a user's
		// own may only reach the local network if that user is one.
		source.Confined = source.Scope.Owner != nil && !administrator
		result = append(result, source)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// GuideFetch is how fetching a guide went: Error is the code of a failure,
// else Channels counts the catalog's channels and Matched those the guide
// covers.
type GuideFetch struct {
	At       time.Time
	Channels int
	Matched  int
	Error    string
}

// StoreGuideFetch records how fetching a catalog's guide from guideURL
// went, unless its address changed meanwhile. A success runs replace in
// the same transaction, which stores the programmes; a failure keeps those
// of the last success.
func (s *Store) StoreGuideFetch(ctx context.Context, key LibraryKey, guideURL string, fetch GuideFetch, replace func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		query := `UPDATE libraries SET guide_checked_at = $5, guide_error = $6
			WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND guide_url = $4`
		args := []any{key.AddonID, key.CatalogType, key.CatalogID, guideURL, fetch.At, fetch.Error}
		if fetch.Error == "" {
			query = `UPDATE libraries SET guide_checked_at = $5, guide_error = $6, guide_fetched_at = $5,
				guide_channels = $7, guide_matched = $8
				WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND guide_url = $4`
			args = append(args, fetch.Channels, fetch.Matched)
		}
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil || tag.RowsAffected() == 0 || fetch.Error != "" {
			return err
		}
		return replace(tx)
	})
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
