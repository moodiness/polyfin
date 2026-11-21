package localfiles

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// linkRule is how a folder in the container treats the symbolic links
// leading out of it: follow tells whether it follows them (see
// Folder.FollowLinks); fenced are the paths of the server's other folders
// in the container that a user of it may not reach, which no link is
// followed into (see Service.linkRule).
type linkRule struct {
	follow bool
	fenced []string
}

// allows reports whether a link of the folder whose real path is real may
// lead to target, its real path: into the folder, or, when the folder
// follows links, anywhere out of it but into a fenced folder.
func (r linkRule) allows(real, target string) bool {
	return within(real, target) || r.follow && !r.fences(target)
}

// fences reports whether target lies in a fenced folder.
func (r linkRule) fences(target string) bool {
	return slices.ContainsFunc(r.fenced, func(folder string) bool { return within(folder, target) })
}

// resolve is the real path of rel, with slashes, in the folder whose real
// path is real: a file or a folder of it, or one a link of it leads to as
// the rule allows; fs.ErrNotExist for one the rule does not.
func (r linkRule) resolve(real, rel string) (string, error) {
	target, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if !r.allows(real, target) {
		return "", fs.ErrNotExist
	}
	return target, nil
}

// resolve is the real path of rel, with slashes, in the folder id in the
// container, whose real path is real, as its scans list it: follow tells
// whether the folder follows the links leading out of it, whose rule is
// read only for a link that does (see linkRule). ErrGone answers a path
// that leads nowhere, or where the rule does not let it.
func (s *Service) resolve(ctx context.Context, id accounts.ID, follow bool, real, rel string) (string, error) {
	target, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
	if err != nil || !within(real, target) && !follow {
		return "", ErrGone
	}
	if within(real, target) {
		return target, nil
	}
	rule, err := s.linkRule(ctx, id, follow)
	if err != nil {
		return "", err
	}
	if !rule.allows(real, target) {
		return "", ErrGone
	}
	return target, nil
}

// linkRule reads how the folder id treats the links leading out of it,
// follow telling whether it follows them. The files of a folder are served
// to its users alone (see addons.UsedBy): a link is never followed into
// another folder of the server one of them may not reach, disabled or used
// by others (see reaches), lest it serve them its files.
func (s *Service) linkRule(ctx context.Context, id accounts.ID, follow bool) (linkRule, error) {
	if !follow {
		return linkRule{}, nil
	}
	installed, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return linkRule{}, err
	}
	i := slices.IndexFunc(installed, func(a addons.Addon) bool { return a.ID == id })
	if i < 0 {
		// A folder removed meanwhile follows none.
		return linkRule{}, nil
	}
	self := installed[i]
	rule := linkRule{follow: true}
	for _, other := range installed {
		if !other.Local() || other.ID == id || shareKind(other.ManifestURL) != "" || other.Enabled && reaches(self.UsedBy, other.UsedBy) {
			continue
		}
		// Links are resolved to real paths: the folder is fenced by its
		// own, and by the path it was given, should it not resolve now.
		rule.fenced = append(rule.fenced, filepath.Clean(other.ManifestURL))
		if real, err := filepath.EvalSymlinks(other.ManifestURL); err == nil {
			rule.fenced = append(rule.fenced, real)
		}
	}
	return rule, nil
}

// reaches reports whether every user of an addon used by from uses one
// used by to: when everyone uses it, or when every group and user listed
// for the first is listed for it too.
func reaches(from, to addons.UsedBy) bool {
	if !to.Listed {
		return true
	}
	return from.Listed && !slices.ContainsFunc(from.Groups, func(g accounts.ID) bool { return !slices.Contains(to.Groups, g) }) &&
		!slices.ContainsFunc(from.Users, func(u accounts.ID) bool { return !slices.Contains(to.Users, u) })
}

// LinkCounts count the symbolic links the last scan of a folder in the
// container skipped: Out, those leading out of it, when it follows none;
// Fenced, those leading into another folder of the server that one of its
// users may not reach (see Service.linkRule); Broken, those leading to
// nothing, round in a loop, to a folder holding it or to one read
// already; Unreadable, those to a file or a folder Polyfin cannot read,
// whose files found before are kept.
type LinkCounts struct {
	Out, Fenced, Broken, Unreadable int
}

// Total is how many links were skipped.
func (c LinkCounts) Total() int { return c.Out + c.Fenced + c.Broken + c.Unreadable }

// logLinks tells once, after the walk of a folder's scan, the links it
// skipped, if any.
func (s *Service) logLinks(folder Folder, c LinkCounts) {
	if c.Total() == 0 {
		return
	}
	s.logger.Info("A local folder's scan skipped symbolic links", "folder", folder.Addon.LogName(), "path", folder.Addon.ManifestURL,
		"follow_links", folder.FollowLinks, "leading_out", c.Out, "into_fenced_folders", c.Fenced, "broken", c.Broken, "unreadable", c.Unreadable)
}

// localWalk walks a folder in the container (see walk): real is its real
// path, rule how it treats the links leading out of it. When it follows
// them, seen are the real paths of the folders walked, each once, and
// count bounds the walk as a share's is (see shareWalk).
type localWalk struct {
	ctx   context.Context
	real  string
	kind  string
	rule  linkRule
	list  *listing
	seen  map[string]bool
	count walkCount
}

// tree walks the folder dir, at rel in the folder walked, "" for the
// folder itself: dir is the folder's real path, or the one a link at rel
// leads to.
func (w *localWalk) tree(dir, rel string) error {
	return filepath.WalkDir(dir, func(at string, d fs.DirEntry, err error) error {
		if w.ctx.Err() != nil {
			return w.ctx.Err()
		}
		sub, relErr := filepath.Rel(dir, at)
		if relErr != nil {
			return relErr
		}
		name := path.Join(rel, filepath.ToSlash(sub))
		if name == "" {
			name = "."
		}
		if err != nil {
			if at == w.real {
				return err
			}
			// A folder it cannot list keeps what was found in it.
			w.list.skipped = append(w.list.skipped, name)
			if at == dir || d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return w.folder(at, dir, name, d)
		}
		file, side := walked(w.kind, name)
		var info fs.FileInfo
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return w.link(at, name, file, side)
		case !file && !side:
			return nil
		case d.Type().IsRegular():
			if info, err = d.Info(); err != nil {
				w.list.skipped = append(w.list.skipped, name)
				return nil
			}
		default:
			return nil
		}
		return w.add(name, file, info)
	})
}

// folder enters the folder at, at name in the folder walked, unless a
// scan of the folder's kind leaves it out; under a link leading out, the
// folders of a fenced folder are left out too. When links are followed, a
// folder is read once, however many links lead to it, and within the
// bounds of the walk.
func (w *localWalk) folder(at, dir, name string, d fs.DirEntry) error {
	if at != dir && skippedIn(w.kind, d.Name()) {
		return fs.SkipDir
	}
	if w.seen == nil {
		return nil
	}
	if w.seen[at] {
		return fs.SkipDir
	}
	if !within(w.real, at) && w.rule.fences(at) {
		w.list.links.Fenced++
		return fs.SkipDir
	}
	w.seen[at] = true
	if at == w.real {
		return nil
	}
	return w.count.folder(name, depth(name))
}

// link reads the symbolic link at, at name in the folder walked, which is
// named as one of the files a scan reads, or as a side file, or neither:
// it may lead to a folder. A link to a file of the folder is followed, as
// is one to a file out of it when the folder follows links; one to a
// folder only out of it, when it does. Those skipped are counted, but the
// links named as no file a scan reads that lead to nothing, when links are
// not followed: they may have led to anything.
func (w *localWalk) link(at, name string, file, side bool) error {
	counts := &w.list.links
	target, err := filepath.EvalSymlinks(at)
	var info fs.FileInfo
	if err == nil {
		info, err = os.Stat(target)
	}
	if err != nil {
		switch {
		case !file && !side && !w.rule.follow:
		case gone(at):
			counts.Broken++
		default:
			// It may lead somewhere later, as once a mount is back: what
			// was found through it is kept.
			counts.Unreadable++
			w.list.skipped = append(w.list.skipped, name)
		}
		return nil
	}
	inside := within(w.real, target)
	switch {
	case info.IsDir():
		switch {
		case inside || skippedIn(w.kind, path.Base(name)):
		case !w.rule.follow:
			counts.Out++
		case w.rule.fences(target):
			counts.Fenced++
		case w.seen[target] || within(target, w.real):
			counts.Broken++
		default:
			return w.tree(target, name)
		}
		return nil
	case !file && !side || !info.Mode().IsRegular():
		return nil
	case !inside && !w.rule.follow:
		counts.Out++
		return nil
	case !inside && w.rule.fences(target):
		counts.Fenced++
		return nil
	}
	f, err := os.Open(target)
	if err != nil {
		counts.Unreadable++
		w.list.skipped = append(w.list.skipped, name)
		return nil
	}
	f.Close()
	return w.add(name, file, info)
}

// add lists a file or a side file found at name.
func (w *localWalk) add(name string, file bool, info fs.FileInfo) error {
	if w.seen != nil {
		if err := w.count.file(name); err != nil {
			return err
		}
	}
	f := found{size: info.Size(), modified: info.ModTime().UTC().Truncate(time.Microsecond)}
	if file {
		w.list.files[name] = f
	} else {
		w.list.sides[name] = f
	}
	return nil
}

// gone reports whether the link at leads to nothing, or round in a loop
// (see looping), rather than somewhere Polyfin cannot read.
func gone(at string) bool {
	_, err := os.Stat(at)
	return errors.Is(err, fs.ErrNotExist) || looping(err)
}

// depth is how many folders deep rel, with slashes, is in the folder
// walked.
func depth(rel string) int {
	n := 1
	for i := range len(rel) {
		if rel[i] == '/' {
			n++
		}
	}
	return n
}
