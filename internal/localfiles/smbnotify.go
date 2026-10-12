package localfiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// The NTSTATUS codes an SMB server answers a change notification it does
// not support with.
const (
	statusNotSupported         = 0xC00000BB
	statusNotImplemented       = 0xC0000002
	statusInvalidDeviceRequest = 0xC0000010
)

// splitFolders is how many folders of a share's root a split watch
// watches at most, each with its tree: Windows serves at most 512
// asynchronous requests at once on a connection by default, and the
// share's reads need some.
const splitFolders = 256

// closeWait bounds the closing of a watched folder: a server that stopped
// answering is not waited for.
const closeWait = 5 * time.Second

var (
	// errNotifyUnsupported reports a server that does not tell of changes.
	errNotifyUnsupported = errors.New("the server does not tell of changes")
	// errTooManyFolders reports a share's root holding more than
	// splitFolders folders, for a split watch.
	errTooManyFolders = errors.New("too many folders to watch each")
)

// notifier is a share whose server tells of changes: an SMB share.
type notifier interface {
	// watch watches the share's folder until ctx ends, then returns nil,
	// or until the watch fails. It calls how.armed once the server holds
	// the watch, and how.changed after each change it tells of.
	watch(ctx context.Context, how notifyHow) error
	// splittable reports whether the share's folder is the root of the
	// share, which a split watch is for.
	splittable() bool
}

// notifyHow is how a share is watched: with one watch of the folder's
// tree, or, split, with one of the folder's own entries and one of the
// tree of each folder in it but those skip leaves out. changed gets the
// paths changed, relative to the folder, with slashes; none when the
// server had too many changes to tell (STATUS_NOTIFY_ENUM_DIR).
type notifyHow struct {
	split   bool
	skip    func(name string) bool
	armed   func()
	changed func(paths []string)
}

// notifyUnsupported reports whether err is a server's refusal of change
// notifications.
func notifyUnsupported(err error) bool {
	var response *smb2.ResponseError
	if errors.As(err, &response) {
		switch response.Code {
		case statusNotSupported, statusNotImplemented, statusInvalidDeviceRequest:
			return true
		}
	}
	return false
}

func (c *smbShare) splittable() bool { return c.dir == "" }

func (c *smbShare) watch(ctx context.Context, how notifyHow) error {
	share, alive, err := c.mounted(ctx)
	if err != nil {
		return err
	}
	err = c.watchIn(ctx, share, alive, how)
	switch {
	case ctx.Err() != nil:
		return nil
	case notifyUnsupported(err):
		return fmt.Errorf("%w: %v", errNotifyUnsupported, err)
	case errors.Is(err, errTooManyFolders):
		return err
	case brokenSession(err):
		// The next watch signs in again.
		c.drop(share)
	}
	return smbError(err)
}

// smbNotice is what a folder's watch heard: the paths changed, relative
// to the watched folder, all when the server could not tell them, or the
// error that ended the watch.
type smbNotice struct {
	watched *smbWatched
	paths   []string
	all     bool
	err     error
}

// smbWatched is a folder of a share watched: its path relative to the
// share's folder, its handle, and the end of its watch.
type smbWatched struct {
	rel  string
	dir  *smb2.File
	stop context.CancelFunc
	done chan struct{}
}

// watchIn watches the share's folder in the session share belongs to, as
// how asks, until ctx ends or a watch fails. The folders are opened and
// closed with the session's context, alive, ended early when the server
// does not answer in time.
func (c *smbShare) watchIn(ctx context.Context, share *smb2.Share, alive context.Context, how notifyHow) error {
	held, release := context.WithCancel(alive)
	defer release()
	in := share.WithContext(held)
	notices := make(chan smbNotice)
	watches := map[string]*smbWatched{}
	end := func(w *smbWatched) {
		w.stop()
		<-w.done
		timer := time.AfterFunc(closeWait, release)
		_ = w.dir.Close()
		timer.Stop()
		delete(watches, w.rel)
	}
	defer func() {
		for _, w := range watches {
			end(w)
		}
	}()
	start := func(rel string, recursive bool) error {
		// Opening is bounded as listing is, and by ctx.
		timer := time.AfterFunc(listTimeout, release)
		stop := context.AfterFunc(ctx, release)
		dir, err := in.OpenNotify(c.path(rel))
		timer.Stop()
		stop()
		if err != nil {
			return err
		}
		watchCtx, cancel := context.WithCancel(ctx)
		w := &smbWatched{rel: rel, dir: dir, stop: cancel, done: make(chan struct{})}
		watches[rel] = w
		go func() {
			defer close(w.done)
			for {
				found, err := dir.NotifyContext(watchCtx, smb2.NotifyOptions{Recursive: recursive})
				notice := smbNotice{watched: w, err: err}
				switch {
				case errors.Is(err, smb2.ErrNotifyEnumDir), err == nil && len(found) == 0:
					notice.err, notice.all = nil, true
				case err == nil:
					for _, n := range found {
						notice.paths = append(notice.paths, strings.ReplaceAll(n.Name, `\`, "/"))
					}
				}
				select {
				case notices <- notice:
				case <-watchCtx.Done():
					return
				}
				if notice.err != nil {
					return
				}
			}
		}()
		return nil
	}
	// refresh watches each folder of the root, and no other, for a split
	// watch.
	refresh := func() error {
		listCtx, cancel := context.WithTimeout(held, listTimeout)
		entries, err := listIn(share.WithContext(listCtx), c.path(""))
		cancel()
		if err != nil {
			return err
		}
		want := map[string]bool{}
		for _, e := range entries {
			if e.folder && !how.skip(e.name) {
				want[e.name] = true
			}
		}
		if len(want) > splitFolders {
			return errTooManyFolders
		}
		for rel, w := range watches {
			if rel != "" && !want[rel] {
				end(w)
			}
		}
		for name := range want {
			if watches[name] != nil {
				continue
			}
			// A folder gone meanwhile is no longer to watch.
			if err := start(name, true); err != nil && brokenSession(err) {
				return err
			}
		}
		return nil
	}

	if err := start("", !how.split); err != nil {
		return err
	}
	if how.split {
		if err := refresh(); err != nil {
			return err
		}
	}
	how.armed()
	for {
		var notice smbNotice
		select {
		case <-ctx.Done():
			return nil
		case notice = <-notices:
		}
		w := notice.watched
		switch {
		case notice.err != nil && w.rel == "":
			return notice.err
		case notice.err != nil:
			// A folder of the root that can no longer be watched, deleted
			// (STATUS_DELETE_PENDING) or else, is let go, and the root
			// listed again.
			if brokenSession(notice.err) {
				return notice.err
			}
			end(w)
			how.changed([]string{w.rel})
			if err := refresh(); err != nil {
				return err
			}
		case notice.all:
			how.changed(nil)
		default:
			paths := notice.paths
			if w.rel != "" {
				for i, p := range paths {
					paths[i] = w.rel + "/" + p
				}
			}
			how.changed(paths)
		}
		if how.split && w.rel == "" && notice.err == nil {
			if err := refresh(); err != nil {
				return err
			}
		}
	}
}

// smbShare tells of changes: a share whose reader is not a notifier is
// polled.
var _ notifier = (*smbShare)(nil)
