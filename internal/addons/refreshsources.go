package addons

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// RemovalConfirmation is how long after RefreshSources first found the
// catalog of a library missing from its addon's manifest a later run must
// find it missing too for the library to be removed. It is shorter than
// the task's shortest interval, an hour, so that its next run confirms,
// and long enough that a run started by hand right after does not.
const RemovalConfirmation = 30 * time.Minute

// SourcesRefreshed counts what RefreshSources did with the addons: those
// whose manifest it stored, those it held while catalogs of their
// libraries were missing, those it could not read or that answered as
// another kind of addon, and those of users whose own addons are off.
type SourcesRefreshed struct {
	Refreshed, Held, Failed, Skipped int
}

// RefreshSources downloads again the manifest of every Stremio and Eclipse
// addon, the server's, then each of users' own, as its Refresh button
// does: with the same writes, libraries following renamed catalogs and
// what Refresh has done once stored. The server's addons and the
// administrators' own are read as an administrator's Refresh reads them,
// a member's own confined to public addresses, as their own Refresh is.
// The addons of a user who may not refresh them, as their own addons are
// off for them or kept from them by parental control or blocked genres,
// are skipped.
//
// Unlike Refresh, one look removes no library: a manifest that no longer
// lists the catalog of a library, not renamed, is held, nothing of it
// stored, until a run at least RemovalConfirmation after the first that
// found that catalog missing finds it missing still (see holdRemovals).
// An addon that cannot be read, or answers as another kind, is left as it
// is and logged; the run goes on with the next. Only ctx ending, or the
// addons not being listed, ends it with an error.
func (s *Store) RefreshSources(ctx context.Context, users []accounts.User, settings accounts.Settings, logger *slog.Logger) (SourcesRefreshed, error) {
	type source struct {
		scope    Scope
		addon    Addon
		confined bool
	}
	var done SourcesRefreshed
	var sources []source
	add := func(scope Scope, confined, refused bool) error {
		installed, err := s.Addons(ctx, scope)
		if err != nil {
			return err
		}
		for _, addon := range installed {
			switch {
			case !addon.Stremio() && !addon.Eclipse():
			case refused:
				done.Skipped++
			default:
				sources = append(sources, source{scope, addon, confined})
			}
		}
		return nil
	}
	if err := add(Shared(), false, false); err != nil {
		return done, err
	}
	for _, user := range users {
		refused := !settings.PersonalAddonsAllowed(user) || user.Restricted()
		if err := add(Personal(user.ID), !user.IsAdministrator, refused); err != nil {
			return done, err
		}
	}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		_, changes, err := s.refresh(ctx, src.scope, src.addon.ID, src.confined, RemovalConfirmation)
		switch {
		case errors.Is(err, ErrNotFound):
			// Removed since it was listed.
		case err != nil:
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			done.Failed++
			logger.Warn("An addon's manifest could not be downloaded again: the addon is left as it is", "addon", src.addon.LogName(),
				"error", err)
		case len(changes.Held) > 0:
			done.Held++
			logger.Info("An addon's manifest no longer lists the catalogs of libraries: it is not stored until a later run finds them missing too",
				"addon", src.addon.LogName(), "libraries", libraryNames(changes.Held), "confirm_after", RemovalConfirmation)
		default:
			done.Refreshed++
			if len(changes.Followed) > 0 || len(changes.Removed) > 0 {
				logger.Info("An addon's manifest downloaded again changed its libraries", "addon", src.addon.LogName(),
					"followed", libraryNames(changes.Followed), "removed", libraryNames(changes.Removed))
			}
		}
	}
	return done, nil
}

// libraryNames lists the names of libraries, for the log.
func libraryNames(libraries []ChangedLibrary) []string {
	names := make([]string, len(libraries))
	for i, library := range libraries {
		names[i] = library.Name
	}
	return names
}

// holdRemovals returns, in tx, the libraries manifest would remove, those
// of the catalogs of addon, as it was before manifest, that it no longer
// lists and does not rename, while one of them was first found missing
// less than confirmAfter ago: the manifest is then held, nothing of it
// stored. It notes when each is first found missing, and forgets those
// found again. Nil lets the manifest be stored, removing those confirmed.
func holdRemovals(ctx context.Context, tx pgx.Tx, addon Addon, manifest stremio.Manifest, renames []Rename,
	confirmAfter time.Duration) ([]ChangedLibrary, error) {
	gone, err := goneLibraries(ctx, tx, addon, manifest)
	if err != nil {
		return nil, err
	}
	gone = slices.DeleteFunc(gone, func(l ChangedLibrary) bool {
		return slices.ContainsFunc(renames, func(r Rename) bool { return r.CatalogType == l.CatalogType && r.From == l.CatalogID })
	})
	if len(gone) == 0 {
		return nil, nil
	}
	types, ids := make([]string, len(gone)), make([]string, len(gone))
	for i, library := range gone {
		types[i], ids[i] = library.CatalogType, library.CatalogID
	}
	if _, err := tx.Exec(ctx, `DELETE FROM missing_catalogs m WHERE m.addon_id = $1 AND NOT EXISTS (
			SELECT FROM unnest($2::text[], $3::text[]) g (catalog_type, catalog_id) WHERE g.catalog_type = m.catalog_type AND g.catalog_id = m.catalog_id)`,
		addon.ID, types, ids); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO missing_catalogs (addon_id, catalog_type, catalog_id)
			SELECT $1, catalog_type, catalog_id FROM unnest($2::text[], $3::text[]) g (catalog_type, catalog_id) ON CONFLICT DO NOTHING`,
		addon.ID, types, ids); err != nil {
		return nil, err
	}
	var recent bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT FROM missing_catalogs WHERE addon_id = $1 AND since > now() - make_interval(secs => $2))",
		addon.ID, confirmAfter.Seconds()).Scan(&recent); err != nil {
		return nil, err
	}
	if !recent {
		return nil, nil
	}
	return gone, nil
}
