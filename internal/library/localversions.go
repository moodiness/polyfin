package library

import (
	"context"
	"errors"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The server's own work on the files of local folders, as the thumbnails
// task does, lists their versions as no user: through the server's
// addons, its local folders alone asked for streams.

// serverUser is the server itself: an administrator without identifier,
// who browses the server's addons and libraries, every one of them.
var serverUser = accounts.User{IsAdministrator: true}

// localPage is how many children a walk of the local folders reads at a
// time.
const localPage = 200

// LocalVersions calls each with every version the server's local folders
// hold of the movies, episodes and videos under parent, with the title it
// is of: parent itself, or those of a series, a season, a folder of videos
// or one of the local folders' libraries; those of every local folder's
// library when parent is zero. It stops at the first error each returns.
// Their catalogs and the titles' descriptions are read as the server's
// libraries list them, and no other addon is asked for streams.
func (s *Service) LocalVersions(ctx context.Context, parent accounts.ID, each func(Item, Version) error) error {
	libraries, err := s.Libraries(ctx, serverUser)
	if err != nil {
		return err
	}
	for _, l := range libraries {
		if l.Kind == KindLibrary && l.LocalFolder && (parent == accounts.ID{} || parent == l.ID) {
			if err := s.walkLocal(ctx, l, each); err != nil {
				return err
			}
		}
	}
	if parent == (accounts.ID{}) || slices.ContainsFunc(libraries, func(l Item) bool { return l.ID == parent }) {
		return nil
	}
	item, err := s.Item(ctx, serverUser, parent)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.walkLocal(ctx, item, each)
}

// walkLocal calls each with the local versions of item, or of the items
// under it: a video standing for a folder of videos has those of its
// videos.
func (s *Service) walkLocal(ctx context.Context, item Item, each func(Item, Version) error) error {
	switch item.Kind {
	case KindMovie, KindEpisode, KindVideo:
		versions, err := s.localVersionsOf(ctx, item.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		for _, version := range versions {
			if err := each(item, version); err != nil {
				return err
			}
		}
		if item.Kind != KindVideo || len(versions) > 0 {
			return nil
		}
	case KindLibrary, KindSeries, KindSeason, KindFolder:
	default:
		return nil
	}
	for start := 0; ; {
		page, err := s.Children(ctx, serverUser, item.ID, start, localPage, "")
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, child := range page.Items {
			if err := s.walkLocal(ctx, child, each); err != nil {
				return err
			}
		}
		start += len(page.Items)
		if len(page.Items) == 0 || !page.More && start >= page.Total {
			return nil
		}
	}
}

// localVersionsOf lists the versions the server's local folders hold of a
// movie, an episode or a video, asking no other addon.
func (s *Service) localVersionsOf(ctx context.Context, id accounts.ID) ([]Version, error) {
	t, v, err := s.target(ctx, serverUser, id)
	if err != nil {
		return nil, err
	}
	v.addons = slices.DeleteFunc(slices.Clone(v.addons), func(entry installed) bool { return !entry.addon.Local() })
	s.embed(ctx, v, &t, false)
	serving := s.streamServing(ctx, v, t)
	if len(t.embedded) > 0 {
		serving = slices.DeleteFunc(serving, func(source streamSource) bool { return source.addon.ID == t.embeddedBy.addon.ID })
	}
	versions, _ := s.listVersions(ctx, t, serving, knownOnly, false)
	return slices.DeleteFunc(versions, func(version Version) bool { return !version.LocalFolder }), nil
}
