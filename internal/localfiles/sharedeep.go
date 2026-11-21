package localfiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// errFlat tells a deep listing of a share that did not give all a walk
// folder by folder would: the share is then walked so.
var errFlat = errors.New("the share gives no deep listing")

// deepRetry is how long a share whose deep listing failed, while a walk
// folder by folder did not, is walked folder by folder before a deep
// listing is asked again.
const deepRetry = 24 * time.Hour

// deepBounds bound a deep listing, which gives at once what a walk reads
// over one listing per folder: as many entries as a walk keeps folders
// and files at its bounds, a WebDAV server's answer within 1 GiB, each
// entry and name within the bounds of a folder's listing. The walk's own
// bounds are counted as the listing is read (see walkDeep); a listing
// past these has the share walked folder by folder.
var deepBounds = listBounds{answer: 1 << 30, entry: folderBounds.entry, entries: 5_000_000, name: folderBounds.name}

// deepRemote is a share that may list its folder with every folder under
// it at once: a WebDAV server answering PROPFIND at infinite depth, in one
// answer, or an S3 bucket listed without a delimiter, a page per thousand
// objects. That does what a walk does with a listing per folder.
type deepRemote interface {
	remote
	// listDeep gives each entry under the share's folder, at any depth,
	// with the path of the folder holding it; a folder comes before what
	// it holds. An error each returns ends the listing with it.
	listDeep(ctx context.Context, each func(dir string, e remoteEntry) error) error
	// deepLists tells whether a deep listing is worth asking for.
	deepLists() *deepLists
}

// deepLists keeps when a share last failed a deep listing that a walk
// folder by folder then did not: none is asked again until deepRetry
// later.
type deepLists struct {
	// failedAt is that time in Unix nanoseconds, 0 for never.
	failedAt atomic.Int64
}

func (d *deepLists) worth(now time.Time) bool {
	at := d.failedAt.Load()
	return at == 0 || now.Sub(time.Unix(0, at)) >= deepRetry
}

func (d *deepLists) failed(now time.Time) { d.failedAt.Store(now.UnixNano()) }

// walkDeep walks a share as walkFolders does, from one listing of every
// folder under its folder, within b: errFlat when that listing cannot
// stand for the walk. A bound of the walk met fails it as it fails the
// walk folder by folder.
//
// A server may answer a deep listing as if it were of one folder: each
// folder under the share's then shows empty. Each folder the listing shows
// empty is therefore listed alone, and one that holds anything tells a
// listing that is not deep. Root is the listing of the share's folder
// alone, which the share's own folder is checked against.
func (b walkBounds) walkDeep(ctx context.Context, r deepRemote, root []remoteEntry, kind string) (listing, error) {
	list := newShareListing(r)
	count := walkCount{bounds: b}
	list.prints[""] = 0
	// empty are the folders entered that the listing shows nothing in so
	// far; ignored, those a walk does not enter and the folders in them.
	empty := map[string]bool{"": true}
	ignored := map[string]bool{}
	var walkErr error
	err := r.listDeep(ctx, func(dir string, e remoteEntry) error {
		if ignored[dir] {
			if e.folder {
				ignored[joinRel(dir, e.name)] = true
			}
			return nil
		}
		print, entered := list.prints[dir]
		if !entered {
			return fmt.Errorf("%w: an entry of %.200q comes before the folder", errFlat, dir)
		}
		delete(empty, dir)
		list.prints[dir] = print + entryPrint(kind, dir, e)
		depth := 0
		if dir != "" {
			depth = strings.Count(dir, "/") + 1
		}
		rel, enter, err := list.add(&count, kind, dir, depth, e)
		switch {
		case err != nil:
			walkErr = err
			return err
		case enter:
			if _, twice := list.prints[rel]; twice {
				return fmt.Errorf("%w: the folder %.200q is listed twice", errFlat, rel)
			}
			list.prints[rel] = 0
			empty[rel] = true
		case e.folder:
			ignored[rel] = true
		}
		return nil
	})
	switch {
	case ctx.Err() != nil:
		return listing{}, ctx.Err()
	case walkErr != nil:
		return listing{}, walkErr
	case errors.Is(err, errFlat):
		return listing{}, err
	case err != nil:
		return listing{}, fmt.Errorf("%w: %v", errFlat, err)
	}
	for rel := range empty {
		if rel == "" {
			if len(root) > 0 {
				return listing{}, fmt.Errorf("%w: the share's folder shows empty", errFlat)
			}
			continue
		}
		entries, err := listFolder(ctx, r, rel)
		switch {
		case ctx.Err() != nil:
			return listing{}, ctx.Err()
		case err != nil:
			return listing{}, fmt.Errorf("%w: %v", errFlat, err)
		case len(entries) > 0:
			return listing{}, fmt.Errorf("%w: the folder %.200q shows empty", errFlat, rel)
		}
	}
	return list, nil
}

// joinRel is the path of the entry name of the share's folder at dir.
func joinRel(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// splitRel splits the path of an entry of a share's folder into the path
// of the folder holding it, "" for the share's, and its name.
func splitRel(rel string) (dir, name string) {
	i := strings.LastIndexByte(rel, '/')
	if i < 0 {
		return "", rel
	}
	return rel[:i], rel[i+1:]
}
