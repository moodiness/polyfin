package library

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Library images.
//
// Jellyfin apps show each library on a tile, with the library's Primary
// image. Each enabled library finds it as its scope chooses (see
// addons.Library.Image): none, as before, or automatically, from the
// titles its catalog lists first. An image uploaded for the library's item
// wins over both, as uploaded artwork does for any item (see UploadImage):
// administrators upload it from the admin app or from jellyfin-web, users
// for their own libraries from the admin app.
//
// The automatic image is the same for everyone who sees the library, and
// is remembered with the library's record, which serves it (see Artwork).
// A user under parental control or blocking genres is not shown it: the
// title it comes from may be one hidden from them.

// automaticImageWait bounds how long looking up a library's automatic
// image may take; a catalog that does not answer by then shows none until
// its pages are read again.
const automaticImageWait = 5 * time.Second

// automaticImage returns the image a library shows when it finds its own
// (see automaticImageOf), remembered as long as catalog pages are, as is
// a catalog that cannot be read, which shows none.
func (s *Service) automaticImage(ctx context.Context, entry installed, catalog stremio.Catalog) string {
	key := pageKey{addon: entry.addon.ID, catalogType: catalog.Type, catalogID: catalog.ID}
	if url, ok := s.libraryImages.Get(key); ok {
		return url
	}
	// The lookup is remembered for every request: one that goes away does
	// not cut it short.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), automaticImageWait)
	defer cancel()
	result, _, _ := s.flight.Do("library image "+entry.addon.ID.String()+"\x00"+catalog.Type+"\x00"+catalog.ID, func() (any, error) {
		url, err := s.automaticImageOf(ctx, entry, catalog)
		if err != nil {
			s.logger.Debug("A library's image could not be found", "addon", entry.addon.Manifest.Name, "catalog", catalog.ID, "error", err)
		}
		s.libraryImages.Put(key, url)
		return url, nil
	})
	return result.(string)
}

// automaticImageOf finds a library's image in the first page of its
// catalog: the first backdrop, wide as library tiles are, else the first
// wide poster, else the first poster; for a music library, the artwork of
// the first item that has some.
func (s *Service) automaticImageOf(ctx context.Context, entry installed, catalog stremio.Catalog) (string, error) {
	if entry.addon.Eclipse() {
		row, ok := entry.addon.Music.Catalog(catalog.ID)
		if !ok {
			return "", nil
		}
		page, err := s.musicCatalogPage(ctx, entry, row, 0)
		if err != nil {
			return "", err
		}
		for _, r := range itemRecords(entry, page.items, nil) {
			if r.Music != nil && r.Music.Artwork != "" {
				return r.Music.Artwork, nil
			}
		}
		return "", nil
	}
	metas, err := s.page(ctx, source{addon: entry, catalog: catalog}, 0)
	if err != nil {
		return "", err
	}
	for _, pick := range []func(stremio.Meta) string{
		func(m stremio.Meta) string { return m.Background },
		func(m stremio.Meta) string { return m.LandscapePoster },
		func(m stremio.Meta) string { return m.Poster },
	} {
		for _, meta := range metas {
			if url := pick(meta); url != "" {
				return url, nil
			}
		}
	}
	return "", nil
}

// automaticImages looks up together the automatic images of the
// libraries that find their own, "" for the others.
func (s *Service) automaticImages(ctx context.Context, libraries []library) []string {
	images := make([]string, len(libraries))
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, l := range libraries {
		if l.image != addons.LibraryImageAutomatic {
			continue
		}
		group.Go(func() error {
			images[i] = s.automaticImage(ctx, l.addon, l.catalog)
			return nil
		})
	}
	_ = group.Wait()
	return images
}

// libraryRecord is what Polyfin remembers of a library: its catalog, and
// its automatic image, if any.
func libraryRecord(l library, image string) record {
	addon := l.addon.addon.ID
	return record{ID: l.item.ID, Key: libraryKey(addon, l.catalog.Type, l.catalog.ID), Kind: KindLibrary, Addon: &addon,
		CatalogType: l.catalog.Type, CatalogID: l.catalog.ID, Confined: l.addon.confined, Poster: image}
}

// LibraryImage is the image a library shows on its tile in Jellyfin apps.
// ID is the library's item, whose Primary image it is; URL stands for the
// image (see ImageTag), empty when the library shows none; Uploaded tells
// that it is an image uploaded for the library.
type LibraryImage struct {
	ID       accounts.ID
	URL      string
	Uploaded bool
}

// LibraryImages tells the image each of a scope's catalogs shows as a
// library (see addons.Store.Libraries), confined when the scope's addons
// may only reach public addresses. Catalogs that are not enabled
// libraries have no item and show none; live TV catalogs make no library.
// Automatic images are looked up as apps would, and remembered for the
// libraries' items, so that the image can be loaded at once.
func (s *Service) LibraryImages(ctx context.Context, scope addons.Scope, confined bool, catalogs []addons.Library) ([]LibraryImage, error) {
	list, err := s.addons.Addons(ctx, scope)
	if err != nil {
		return nil, err
	}
	byID := map[accounts.ID]addons.Addon{}
	for _, addon := range list {
		byID[addon.ID] = addon
	}
	result := make([]LibraryImage, len(catalogs))
	var records []record
	var mu sync.Mutex
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, c := range catalogs {
		if !c.Enabled || LiveCatalog(c.Catalog.Type) {
			continue
		}
		id := LibraryID(c)
		result[i].ID = id
		if url, ok := s.UploadedArtwork(id, "Primary"); ok {
			result[i].URL, result[i].Uploaded = url, true
			continue
		}
		addon, ok := byID[c.AddonID]
		if !ok || !addon.Enabled {
			continue
		}
		l := library{item: Item{ID: id}, addon: installed{addon: addon, confined: confined, shared: scope.Owner == nil}, catalog: c.Catalog,
			image: c.Image}
		group.Go(func() error {
			var image string
			if l.image == addons.LibraryImageAutomatic {
				image = s.automaticImage(ctx, l.addon, l.catalog)
			}
			mu.Lock()
			defer mu.Unlock()
			result[i].URL = image
			records = append(records, libraryRecord(l, image))
			return nil
		})
	}
	_ = group.Wait()
	return result, s.save(ctx, records)
}

// DownloadImage downloads the picture at address once, from public
// addresses only when confined, and keeps it as an item's uploaded
// artwork of imageType (see UploadImage).
func (s *Service) DownloadImage(ctx context.Context, id accounts.ID, imageType, address string, confined bool) error {
	data, _, err := s.client.Image(ctx, address, confined)
	if err != nil {
		return err
	}
	return s.UploadImage(ctx, id, imageType, data)
}
