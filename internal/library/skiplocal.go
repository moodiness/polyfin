package library

import (
	"cmp"
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A library of movies or series may leave out the titles a local folder
// holds (see addons.Library.SkipLocal): those the library of a movies or
// shows folder lists, by the same identifier, which is the title's own
// item. A file linked to the IMDb identifier a catalog lists a title
// under is a version of that title, so both libraries would list it.
// With the option, only the folder's does: the title keeps its item, its
// watched state and its versions, and search, Next Up and Continue
// Watching list it once, as they did. Only the library's own listings
// leave it out, its home row included; smart lists, collections, genre
// pages and searches read the catalog as it is.
//
// The folders are those whose libraries the user sees: a user whose
// visible libraries leave out the folder's library still finds the title
// in the catalog's library, as everyone does once the folder, or its
// library, is turned off. A title is matched by its identifier alone: one
// the catalog lists under another identifier, such as a TMDB one, is
// another title, which the folder's files do not play, and stays.

// listedFolders returns the local folders of libraries, by the kind of
// the titles they list: movies or series.
func listedFolders(libraries []library) map[Kind][]accounts.ID {
	var folders map[Kind][]accounts.ID
	for _, l := range libraries {
		if !l.addon.addon.Local() || l.addon.addon.HasMusic() {
			continue
		}
		kind, ok := titleKind(l.catalog.Type)
		if !ok || kind != KindMovie && kind != KindSeries {
			continue
		}
		if folders == nil {
			folders = map[Kind][]accounts.ID{}
		}
		if !slices.Contains(folders[kind], l.addon.addon.ID) {
			folders[kind] = append(folders[kind], l.addon.addon.ID)
		}
	}
	return folders
}

// withoutLocal leaves out of a catalog page of src, when src leaves out
// what local folders hold, the movies and series the folders of v hold;
// other entries stay.
func (s *Service) withoutLocal(ctx context.Context, v view, src source, metas []stremio.Meta) ([]stremio.Meta, error) {
	if !src.skipLocal || len(v.listedFolders) == 0 || s.local == nil || len(metas) == 0 {
		return metas, nil
	}
	ids := map[Kind][]string{}
	for _, meta := range metas {
		kind, ok := titleKind(cmp.Or(meta.Type, src.catalog.Type))
		if ok && meta.ID != "" && len(v.listedFolders[kind]) > 0 {
			ids[kind] = append(ids[kind], meta.ID)
		}
	}
	held := map[Kind]map[string]bool{}
	for kind, list := range ids {
		found, err := s.local.HeldTitles(ctx, v.listedFolders[kind], list)
		if err != nil {
			return nil, err
		}
		held[kind] = found
	}
	return slices.DeleteFunc(slices.Clone(metas), func(meta stremio.Meta) bool {
		kind, ok := titleKind(cmp.Or(meta.Type, src.catalog.Type))
		return ok && held[kind][meta.ID]
	}), nil
}
