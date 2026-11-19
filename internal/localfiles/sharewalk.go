package localfiles

import "fmt"

// The failures of a share's scan whose walk met a bound (see walkBounds),
// beside errBoundListing: the share's folders go deeper than one walk
// follows them, or it holds more folders or more files than one walk
// reads.
const (
	errBoundDepth   = "too_deep"
	errBoundFolders = "too_many_folders"
	errBoundFiles   = "too_many_files"
)

// errBoundTime is the failure of a share's scan that lasted longer than
// one scan may (see shareScanTime).
const errBoundTime = "too_slow"

// metBound tells whether a scan's failure is a bound its walk met.
func metBound(failure string) bool {
	switch failure {
	case errBoundDepth, errBoundFolders, errBoundFiles, errBoundListing:
		return true
	}
	return false
}

// in tells the folder a listing met its bound in.
func (e *shareBound) in(rel string) *shareBound {
	return &shareBound{failure: e.failure, detail: fmt.Sprintf("%s, in the folder %.200q", e.detail, rel)}
}

// walkBounds bound one walk of a share (see walkShare): depth, how deep
// its folders go under the folder walked; folders, the folders it lists;
// files, the files and side files it keeps; paths, the bytes of the
// paths it keeps, of the folders to list, the files and the side files.
// A folder's listing is bounded as it is read (see listBounds).
type walkBounds struct {
	depth, folders, files int
	paths                 int
}

// shareWalk are the bounds of a share's walk: folders 32 deep, where a
// library goes a few deep; 1,000,000 folders and 4,000,000 files and side
// files, several times what the largest libraries hold; and 512 MiB of
// paths, 128 bytes for each file of a walk at its bound.
var shareWalk = walkBounds{depth: 32, folders: 1_000_000, files: 4_000_000, paths: 512 << 20}

// walkCount counts what one walk of a share found, within bounds.
type walkCount struct {
	bounds                walkBounds
	folders, files, paths int
}

// folder counts a folder to list, at rel, depth folders under the folder
// walked.
func (w *walkCount) folder(rel string, depth int) error {
	switch {
	case depth > w.bounds.depth:
		return &shareBound{failure: errBoundDepth, detail: fmt.Sprintf("folders more than %d deep, at %.200q", w.bounds.depth, rel)}
	case w.folders >= w.bounds.folders:
		return &shareBound{failure: errBoundFolders, detail: fmt.Sprintf("more than %d folders", w.bounds.folders)}
	}
	w.folders++
	return w.path(rel)
}

// file counts a file or a side file kept, at rel.
func (w *walkCount) file(rel string) error {
	if w.files >= w.bounds.files {
		return &shareBound{failure: errBoundFiles, detail: fmt.Sprintf("more than %d files", w.bounds.files)}
	}
	w.files++
	return w.path(rel)
}

// path counts the bytes of a path kept.
func (w *walkCount) path(rel string) error {
	if w.paths += len(rel); w.paths > w.bounds.paths {
		return &shareBound{failure: errBoundFiles, detail: fmt.Sprintf("paths of more than %d bytes together", w.bounds.paths)}
	}
	return nil
}
