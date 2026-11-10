package addons

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A catalog an addon gives a new identifier keeps its library when the
// match is strict (see matchRenames): the library follows the new
// identifier and keeps its own, the one apps know it by, through the
// addon's LibraryKeys; what names the catalog follows it, in the same
// transaction (see SetCarry).

// CatalogKey is a catalog's type and identifier.
type CatalogKey struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// KeptKey is a catalog, by its current type and identifier, whose library
// takes its identifier from another key (see Addon.LibraryKeys).
type KeptKey struct {
	Catalog CatalogKey `json:"catalog"`
	Key     CatalogKey `json:"key"`
}

// LibraryKey returns the catalog type and identifier the library of the
// addon's catalog takes its identifier from: the key it kept, or the
// catalog's own.
func (a Addon) LibraryKey(catalogType, catalogID string) CatalogKey {
	catalog := CatalogKey{Type: catalogType, ID: catalogID}
	if i := slices.IndexFunc(a.LibraryKeys, func(k KeptKey) bool { return k.Catalog == catalog }); i >= 0 {
		return a.LibraryKeys[i].Key
	}
	return catalog
}

// Rename is a catalog of CatalogType an addon's manifest gave a new
// identifier, From the old one To the new one (see matchRenames).
type Rename struct {
	CatalogType string
	From, To    string
}

// CatalogChanges tells what a manifest downloaded again did to an addon's
// libraries: those that followed their catalog's new identifier, and those
// removed with their catalog, each in the order of the scope's libraries.
type CatalogChanges struct {
	Followed []ChangedLibrary
	Removed  []ChangedLibrary
}

// ChangedLibrary is a library of a catalog renamed or gone: its name in
// apps, or its catalog's, its catalog's type and identifier, and NewID,
// the identifier it followed, empty for a library removed.
type ChangedLibrary struct {
	Name        string
	CatalogType string
	CatalogID   string
	NewID       string
}

// CarryRenames rewrites, in tx, the transaction storing addon's manifest,
// what else names the catalogs renamed, and returns what to do once tx is
// stored (see SetCarry).
type CarryRenames func(ctx context.Context, tx pgx.Tx, addon Addon, renames []Rename) (after func(), err error)

// SetCarry has Refresh and Replace run carry when a manifest renames
// catalogs; the libraries table and its foreign keys alone follow without
// it.
func (s *Store) SetCarry(carry CarryRenames) { s.carry = carry }

// matchRenames returns the catalogs of manifest old that manifest gives a
// new identifier: a catalog gone and a catalog new of the same type, with
// the same name, spaces trimmed and case ignored, and the same extra
// properties, by name and whether each is required. The pair must be the
// only one: two catalogs gone, or two new, of that type and name match
// none. A name changed too, a type changed or extra properties changed
// make a catalog removed and another added.
func matchRenames(old, manifest stremio.Manifest) []Rename {
	type named struct {
		catalogType, name string
	}
	type candidates struct {
		gone, added []stremio.Catalog
	}
	byName := map[named]*candidates{}
	var order []named
	group := func(c stremio.Catalog) *candidates {
		key := named{c.Type, strings.ToLower(strings.TrimSpace(c.Name))}
		found, ok := byName[key]
		if !ok {
			found = &candidates{}
			byName[key] = found
			order = append(order, key)
		}
		return found
	}
	for _, c := range old.Catalogs {
		if _, kept := manifest.Catalog(c.Type, c.ID); !kept {
			g := group(c)
			g.gone = append(g.gone, c)
		}
	}
	for _, c := range manifest.Catalogs {
		if _, existed := old.Catalog(c.Type, c.ID); !existed {
			g := group(c)
			g.added = append(g.added, c)
		}
	}
	var renames []Rename
	for _, key := range order {
		g := byName[key]
		if len(g.gone) == 1 && len(g.added) == 1 && sameExtra(g.gone[0], g.added[0]) {
			renames = append(renames, Rename{CatalogType: key.catalogType, From: g.gone[0].ID, To: g.added[0].ID})
		}
	}
	return renames
}

// sameExtra reports whether two catalogs take the same extra properties,
// by name and whether each is required; their options, which change with
// what the catalog holds, are not compared.
func sameExtra(a, b stremio.Catalog) bool {
	type property struct {
		name     string
		required bool
	}
	properties := func(c stremio.Catalog) []property {
		list := make([]property, 0, len(c.Extra))
		for _, e := range c.Extra {
			list = append(list, property{e.Name, e.IsRequired})
		}
		slices.SortFunc(list, func(x, y property) int {
			return cmp.Or(strings.Compare(x.name, y.name), cmp.Compare(boolInt(x.required), boolInt(y.required)))
		})
		return list
	}
	return slices.Equal(properties(a), properties(b))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// settleKeys returns an addon's LibraryKeys for manifest, from keys, its
// LibraryKeys before, and moved, the catalogs whose type or identifier
// changed, by their new key, with the key they had before:
//   - a catalog kept keeps its key, and a catalog moved takes the key of
//     the catalog it was;
//   - any other takes its own, unless a library kept it: an addon that
//     brings back a catalog under an identifier a library followed away
//     from makes a library of its own, with its own key, its identifier
//     followed by |2, or the first of |3, |4… no other catalog's library
//     has;
//   - the keys of catalogs gone, and those equal to their catalog's own,
//     are dropped.
func settleKeys(keys []KeptKey, manifest stremio.Manifest, moved map[CatalogKey]CatalogKey) []KeptKey {
	before := make(map[CatalogKey]CatalogKey, len(keys))
	for _, k := range keys {
		before[k.Catalog] = k.Key
	}
	settled := []KeptKey{}
	used := map[CatalogKey]bool{}
	var own []CatalogKey
	for _, c := range manifest.Catalogs {
		catalog := CatalogKey{Type: c.Type, ID: c.ID}
		key, ok := before[catalog]
		if from, isMoved := moved[catalog]; isMoved {
			if key, ok = before[from]; !ok {
				key, ok = from, true
			}
		}
		if !ok || key == catalog {
			own = append(own, catalog)
			continue
		}
		used[key] = true
		settled = append(settled, KeptKey{Catalog: catalog, Key: key})
	}
	var taken []CatalogKey
	for _, catalog := range own {
		if used[catalog] {
			taken = append(taken, catalog)
		} else {
			used[catalog] = true
		}
	}
	for _, catalog := range taken {
		key := catalog
		for n := 2; used[key]; n++ {
			key = CatalogKey{Type: catalog.Type, ID: fmt.Sprintf("%s|%d", catalog.ID, n)}
		}
		used[key] = true
		settled = append(settled, KeptKey{Catalog: catalog, Key: key})
	}
	return settled
}

// followLibraries has, in tx, the libraries of addon's catalogs that
// manifest renames follow their new identifier, and removes those of its
// catalogs gone, and tells which did which. addon is as it was before
// manifest.
func followLibraries(ctx context.Context, tx pgx.Tx, scope Scope, addon Addon, manifest stremio.Manifest, renames []Rename) (CatalogChanges, error) {
	var changes CatalogChanges
	rows, err := tx.Query(ctx, "SELECT catalog_type, catalog_id, name FROM libraries WHERE addon_id = $1 ORDER BY position, catalog_type, catalog_id",
		addon.ID)
	if err != nil {
		return changes, err
	}
	var gone []ChangedLibrary
	var catalogType, catalogID string
	var name *string
	if _, err := pgx.ForEachRow(rows, []any{&catalogType, &catalogID, &name}, func() error {
		if _, ok := manifest.Catalog(catalogType, catalogID); ok {
			return nil
		}
		library := ChangedLibrary{CatalogType: catalogType, CatalogID: catalogID}
		if name != nil {
			library.Name = *name
		} else if catalog, ok := addon.Manifest.Catalog(catalogType, catalogID); ok {
			library.Name = catalog.Name
		}
		library.Name = cmp.Or(library.Name, catalogID)
		gone = append(gone, library)
		return nil
	}); err != nil {
		return changes, err
	}
	if len(gone) > 0 && len(renames) > 0 {
		if err := lockScope(ctx, tx, scope); err != nil {
			return changes, err
		}
	}
	for _, library := range gone {
		i := slices.IndexFunc(renames, func(r Rename) bool { return r.CatalogType == library.CatalogType && r.From == library.CatalogID })
		if i < 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM libraries WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3", addon.ID,
				library.CatalogType, library.CatalogID); err != nil {
				return changes, err
			}
			changes.Removed = append(changes.Removed, library)
			continue
		}
		// Its guides, their channel choices and programmes follow by
		// their foreign keys.
		library.NewID = renames[i].To
		if _, err := tx.Exec(ctx, "UPDATE libraries SET catalog_id = $4 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3", addon.ID,
			library.CatalogType, library.CatalogID, library.NewID); err != nil {
			return changes, err
		}
		changes.Followed = append(changes.Followed, library)
	}
	return changes, nil
}
