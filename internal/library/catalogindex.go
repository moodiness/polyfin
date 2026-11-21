package library

import (
	"context"
	"fmt"
	"hash/maphash"
	"slices"
	"sort"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Where a catalog's pages start in its listings.
//
// A Stremio catalog tells no count of its titles, and its pages may repeat
// titles of the pages before, which a listing shows once: a listing at
// some position counts the titles before it. Reading every page before it
// made a deep page cost as much as the catalog up to there, each page read
// from the database after a restart. So a read of a catalog whose listing
// is the catalog itself (see indexes) remembers where each page it read
// starts among the titles listed, and which of the page's titles it left
// out as listed before: the catalog's index. A listing within the pages
// indexed then reads only the pages it shows, and one past them reads on
// from the last page indexed. An index is kept for the catalog and the
// most titles its listings read (see view.limit).
//
// An index holds while the catalog's pages hold the titles it was made of:
// the catalog's version (see catalogVersions) is the one read once its
// pages were checked against the pages kept, and each page a listing reads
// through it holds the titles indexed. Otherwise the listing reads the
// catalog from its first page, as without an index, and makes a new one.
//
// Once a user opens a library whose catalog is indexed so, the rest of the
// catalog is read in the background (see readThrough): the first scroll
// through it then waits for no addon past its first pages.

// indexKey is a catalog's pages as listings read them, up to limit
// titles.
type indexKey struct {
	catalog catalogRef
	limit   int
}

// catalogIndex is where each page read of a catalog starts in its
// listing. Once kept, it is not changed: a read that goes further keeps a
// new one.
type catalogIndex struct {
	// version is the catalog's version the pages were read at.
	version uint64
	pages   []indexedPage
	// received is the position in the catalog past the last page indexed,
	// listed the titles listed up to there, and size the longest page.
	received, listed, size int
	// ended is set when the catalog ends past the last page indexed: a
	// page empty or holding only titles listed before, its last page, or
	// the most titles its listings read.
	ended bool
}

// indexedPage is a page of an indexed catalog.
type indexedPage struct {
	// skip is the page's position in the catalog, and length the titles
	// it holds, print what identifies them (see printOf).
	skip, length int
	print        uint64
	// before is how many titles are listed before the page's, and dropped
	// the positions in the page, in order, of those listed before.
	before  int
	dropped []int
}

// listed is how many titles of the page are listed.
func (p indexedPage) listed() int { return p.length - len(p.dropped) }

// indexBytes bounds the memory the indexes take, and indexedPageBytes is
// what a page of one takes, without its dropped positions: an index of
// 10,000 titles in pages of 100 takes about 7 kB.
const (
	indexBytes       = 8 << 20
	indexedPageBytes = 64
)

func indexSize(index *catalogIndex) int {
	size := 128 + len(index.pages)*indexedPageBytes
	for _, page := range index.pages {
		size += 8 * len(page.dropped)
	}
	return size
}

// printSeed seeds the prints of pages: they are compared within the
// process only.
var printSeed = maphash.MakeSeed()

// printOf identifies the titles of a page, in their order.
func printOf(metas []stremio.Meta) uint64 {
	var h maphash.Hash
	h.SetSeed(printSeed)
	for _, meta := range metas {
		h.WriteString(meta.ID)
		h.WriteByte(0)
	}
	return h.Sum64()
}

// indexes reports whether the listings of src for v are indexed: those of
// a paged addon catalog, read past its first page, that leave out none of
// its titles. A user's parental control or blocked genres, a rating
// filter, titles they hid, movies waiting for their digital release and
// the titles of local folders a library leaves out each leave out titles
// as they are read: those listings read the catalog from its first page.
func (s *Service) indexes(v view, src source) bool {
	return src.paged() && !src.first && src.search == "" && src.date == "" && !own(src.addon.addon) &&
		!v.narrows() && len(v.hidden) == 0 && (!v.waitRelease || v.upcoming) &&
		(!src.skipLocal || len(v.listedFolders) == 0 || s.local == nil)
}

func indexKeyOf(src source, limit int) indexKey {
	return indexKey{catalogRef{src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre}, limit}
}

// indexOf returns the index of src's catalog read up to limit titles,
// nil when none holds.
func (s *Service) indexOf(src source, limit int) *catalogIndex {
	index, ok := s.catalogIndexes.Get(indexKeyOf(src, limit))
	if !ok || index.version != s.catalogVersions.of(src) {
		return nil
	}
	return index
}

// indexedWindow returns the titles [start, start+count) of the listing of
// src through its index, and the listing's count of titles, plus one when
// more may follow, as window counts them: it reads the pages that hold
// them only. done is false when the index does not reach the window, one
// of its pages could not be read, or it no longer holds, then stale.
func (s *Service) indexedWindow(ctx context.Context, src source, index *catalogIndex, start, count int) (
	metas []stremio.Meta, total int, done, stale bool, err error) {
	end := start + count
	if index == nil || end > index.listed && !index.ended {
		return nil, 0, false, false, nil
	}
	if start >= index.listed {
		return nil, index.listed, true, false, nil
	}
	first := sort.Search(len(index.pages), func(i int) bool { return index.pages[i].before > start }) - 1
	last := first
	for last+1 < len(index.pages) && index.pages[last].before+index.pages[last].listed() < end {
		last++
	}
	shown := index.pages[first : last+1]
	offsets := make([]int, len(shown))
	for i, page := range shown {
		offsets[i] = page.skip
	}
	s.loadMissingPages(ctx, src, offsets, offsets[len(offsets)-1]+1)
	pages, missing, errs := s.pagesAt(ctx, src, offsets)
	if err := ctx.Err(); err != nil {
		return nil, 0, true, false, err
	}
	if slices.Contains(missing, true) || slices.ContainsFunc(errs, func(err error) bool { return err != nil }) {
		return nil, 0, false, false, nil
	}
	if index.version != s.catalogVersions.of(src) {
		return nil, 0, false, true, nil
	}
	for i, page := range shown {
		if len(pages[i]) != page.length || printOf(pages[i]) != page.print {
			return nil, 0, false, true, nil
		}
	}
	metas = make([]stremio.Meta, 0, min(count, index.listed-start))
	for i, page := range shown {
		position, dropped := page.before, page.dropped
		for at, meta := range pages[i] {
			if len(dropped) > 0 && dropped[0] == at {
				dropped = dropped[1:]
				continue
			}
			if position >= start && position < end {
				metas = append(metas, meta)
			}
			position++
		}
	}
	tail := shown[len(shown)-1]
	if total = tail.before + tail.listed(); total >= end {
		total++
	}
	return metas, total, true, false, nil
}

// resumeIndex returns a copy of index to read on from, with the titles of
// its pages marked in seen, and those it lists from start on: false when
// it ended, or when memory lacks one of its pages or holds another.
func (s *Service) resumeIndex(index *catalogIndex, src source, seen map[string]bool, start int) (*catalogIndex, []stremio.Meta, bool) {
	if index == nil || index.ended {
		return nil, nil, false
	}
	var listed []stremio.Meta
	for _, page := range index.pages {
		metas, ok := s.cachedPage(src.key(page.skip))
		if !ok || len(metas) != page.length || printOf(metas) != page.print {
			clear(seen)
			return nil, nil, false
		}
		position, dropped := page.before, page.dropped
		for at, meta := range metas {
			seen[meta.ID] = true
			if len(dropped) > 0 && dropped[0] == at {
				dropped = dropped[1:]
				continue
			}
			if position >= start {
				listed = append(listed, meta)
			}
			position++
		}
	}
	resumed := *index
	resumed.pages = slices.Clip(index.pages)
	return &resumed, listed, true
}

// add indexes the page of metas at skip, which listed added titles and
// dropped those at the positions dropped.
func (index *catalogIndex) add(skip int, metas []stremio.Meta, dropped []int) {
	index.pages = append(index.pages, indexedPage{skip: skip, length: len(metas), print: printOf(metas), before: index.listed,
		dropped: dropped})
	index.received = skip + len(metas)
	index.listed += len(metas) - len(dropped)
	index.size = max(index.size, len(metas))
}

// pageCheck is a page a read found, by what identifies its titles.
type pageCheck struct {
	skip   int
	length int
	print  uint64
}

// keepIndex keeps index, made by a read of src's catalog up to limit
// titles, if the pages it was made of, and end, the page that ended the
// catalog when one did, are still those memory keeps once the catalog's
// version is read: a page replaced before is found so, and one replaced
// after changes the version (see fetchPage).
func (s *Service) keepIndex(src source, limit int, index *catalogIndex, end *pageCheck) {
	if len(index.pages) == 0 && !index.ended {
		return
	}
	version := s.catalogVersions.of(src)
	holds := func(skip, length int, print uint64) bool {
		metas, ok := s.cachedPage(src.key(skip))
		return ok && len(metas) == length && printOf(metas) == print
	}
	for _, page := range index.pages {
		if !holds(page.skip, page.length, page.print) {
			return
		}
	}
	if end != nil && !holds(end.skip, end.length, end.print) {
		return
	}
	index.version = version
	s.catalogIndexes.Put(indexKeyOf(src, limit), index)
}

// readThrough reads, in the background, the rest of the catalog of a
// library a user opens, up to the most titles its listings read, so that
// their first scroll through the library waits for no addon past its
// first pages. Its pages take a place among their host's after those a
// request waits for (see withYielding), and a request that needs one of
// them meanwhile waits for it rather than asking again (see pageFlight).
// It reads only a catalog indexed for the user (see indexes), not yet
// read to its end, and nothing from an addon whose last request failed.
func (s *Service) readThrough(ctx context.Context, v view, src source) {
	if !s.readAhead || !opening(ctx) || keptOnly(ctx) || whole(ctx) || !s.indexes(v, src) {
		return
	}
	limit := v.limit(src)
	due := func() bool {
		index := s.indexOf(src, limit)
		return index == nil || !index.ended && index.listed < limit
	}
	if !due() {
		return
	}
	if health, ok := s.client.Health(src.addon.addon.ManifestURL); ok && health.Failure != "" {
		return
	}
	s.background(withYielding(ctx), fmt.Sprintf("read through %v %d", src.key(0), limit), due, func(ctx context.Context) error {
		_, _, err := s.window(ctx, v, src, limit, 1)
		return err
	})
}
