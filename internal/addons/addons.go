// Package addons stores the Stremio addons of the server and of each user,
// and which of their catalogs are shown as libraries.
package addons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// DefaultLibraries is how many libraries a scope gets automatically when
// addons are installed. Each library adds a row to the home screen of
// Jellyfin apps.
const DefaultLibraries = 20

var (
	ErrNotFound           = errors.New("addon not found")
	ErrExists             = errors.New("addon already installed")
	ErrInvalidOrder       = errors.New("the order must list every addon of the scope once")
	ErrInvalidLibrary     = errors.New("unknown or unbrowsable catalog")
	ErrInvalidLibraryName = errors.New("library names are 1 to 64 printable characters")
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

// Install adds an addon at the end of the scope and enables its browsable
// movie and series catalogs until the scope has DefaultLibraries.
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

func enableDefaults(ctx context.Context, tx pgx.Tx, scope Scope, addon Addon) error {
	var count, last int
	err := tx.QueryRow(ctx, `SELECT count(*), coalesce(max(l.position), 0) FROM libraries l
		JOIN addons a ON a.id = l.addon_id WHERE a.owner_id IS NOT DISTINCT FROM $1`, scope.Owner).Scan(&count, &last)
	if err != nil {
		return err
	}
	for _, catalog := range addon.Manifest.Catalogs {
		if count >= DefaultLibraries {
			break
		}
		if !catalog.Browsable() || (catalog.Type != "movie" && catalog.Type != "series") {
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
	rows, err := db.Query(ctx, `SELECT l.addon_id, l.catalog_type, l.catalog_id, l.name FROM libraries l
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
	}
	if _, err := pgx.ForEachRow(rows, []any{&row.addon, &row.kind, &row.catID, &row.name}, func() error {
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
		enabled = append(enabled, Library{AddonID: addon.ID, AddonName: addon.Manifest.Name, Catalog: catalog,
			Name: row.name, Enabled: true, AddonActive: addon.Enabled})
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
		if _, err := tx.Exec(ctx, `DELETE FROM libraries WHERE addon_id IN
			(SELECT id FROM addons WHERE owner_id IS NOT DISTINCT FROM $1)`, scope.Owner); err != nil {
			return err
		}
		for position, choice := range choices {
			if _, err := tx.Exec(ctx, "INSERT INTO libraries (addon_id, catalog_type, catalog_id, name, position) VALUES ($1, $2, $3, $4, $5)",
				choice.AddonID, choice.CatalogType, choice.CatalogID, choice.Name, position+1); err != nil {
				return err
			}
		}
		result, err = libraries(ctx, tx, scope)
		return err
	})
	return result, err
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
