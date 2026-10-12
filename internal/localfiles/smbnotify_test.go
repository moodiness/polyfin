package localfiles

import (
	"context"
	"crypto/rand"
	"net"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// An SMB share's server tells of the files added, renamed and removed in
// its folder's tree, which the watch does not keep from renaming; the
// watch is set up again once the session was cut, and split, it tells of
// the changes of folders added since. It runs when POLYFIN_TEST_SMB names a
// share the user may write to (see TestSMBSharesAreScannedAndRead): it
// works in a folder of its own, removed after.
func TestSMBShareTellsOfChanges(t *testing.T) {
	raw := os.Getenv("POLYFIN_TEST_SMB")
	if raw == "" {
		t.Skip("POLYFIN_TEST_SMB is not set")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		t.Fatal("POLYFIN_TEST_SMB is not smb://user:password@host/share/folder")
	}
	user := u.User.Username()
	password, _ := u.User.Password()
	u.User = nil
	port := u.Port()
	if port == "" {
		port = "445"
	}
	server := net.JoinHostPort(u.Hostname(), port)

	// The test writes through a session of its own.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	dialer := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: password}}
	session, err := dialer.Dial(ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Logoff()
	name, dir, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	share, err := session.WithContext(ctx).Mount(name)
	if err != nil {
		t.Fatal(err)
	}
	scratch := path.Join(dir, "polyfin-notify-"+rand.Text()[:8])
	if err := share.Mkdir(scratch, 0o755); err != nil {
		t.Fatalf("the share cannot be written to: %v", err)
	}
	// Removed before the session ends, whatever ctx became.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := share.WithContext(ctx).RemoveAll(scratch); err != nil {
			t.Errorf("the test's folder could not be removed: %v", err)
		}
	}()
	create := func(rel string) {
		t.Helper()
		if err := share.MkdirAll(path.Dir(path.Join(scratch, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := share.WriteFile(path.Join(scratch, rel), []byte("movie"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	network, address := newRelay(t, server)
	u.Host = address
	u.Path = "/" + path.Join(name, scratch)
	watched := newSMB(u, user, password)
	defer watched.close()

	// watch watches the test's folder until it fails, and returns the
	// paths it tells of, and its end.
	watch := func(ctx context.Context, split bool) (<-chan []string, <-chan error) {
		armed := make(chan struct{})
		changes := make(chan []string, 64)
		ended := make(chan error, 1)
		go func() {
			ended <- watched.watch(ctx, notifyHow{split: split, skip: func(name string) bool { return skippedIn(KindMovies, name) },
				armed: func() { close(armed) }, changed: func(paths []string) { changes <- paths }})
		}()
		select {
		case <-armed:
		case err := <-ended:
			t.Fatalf("the watch ended before it was set up: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("the watch was not set up")
		}
		return changes, ended
	}
	// told waits for the watch to tell of a change of want.
	told := func(changes <-chan []string, want string) {
		t.Helper()
		timeout := time.After(30 * time.Second)
		for {
			select {
			case paths := <-changes:
				if paths == nil || slices.Contains(paths, want) {
					return
				}
			case <-timeout:
				t.Fatalf("no change of %q told", want)
			}
		}
	}

	watchCtx, stop := context.WithCancel(ctx)
	changes, ended := watch(watchCtx, false)
	create("Metropolis (1927)/Metropolis (1927).mkv")
	told(changes, "Metropolis (1927)/Metropolis (1927).mkv")
	// The watched folder's own folders can be renamed while it is watched.
	if err := share.Rename(path.Join(scratch, "Metropolis (1927)"), path.Join(scratch, "Metropolis (1927) 2")); err != nil {
		t.Fatalf("a folder of the watched tree cannot be renamed: %v", err)
	}
	told(changes, "Metropolis (1927) 2")
	if err := share.Remove(path.Join(scratch, "Metropolis (1927) 2", "Metropolis (1927).mkv")); err != nil {
		t.Fatal(err)
	}
	told(changes, "Metropolis (1927) 2/Metropolis (1927).mkv")

	// A cut session ends the watch; the next one signs in again.
	network.cut()
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("the watch of a cut session ended without an error")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the watch went on after its session was cut")
	}
	changes, ended = watch(watchCtx, false)
	create("Nosferatu (1922).mkv")
	told(changes, "Nosferatu (1922).mkv")
	stop()
	if err := <-ended; err != nil {
		t.Fatalf("the watch ended with %v", err)
	}

	// Split, the folders added are watched too.
	splitCtx, stopSplit := context.WithCancel(ctx)
	defer stopSplit()
	changes, _ = watch(splitCtx, true)
	create("Metropolis (1927) 2/Metropolis (1927).fr.srt")
	told(changes, "Metropolis (1927) 2/Metropolis (1927).fr.srt")
	create("The General (1926)/The General (1926).mkv")
	told(changes, "The General (1926)")
	create("The General (1926)/Extras/Trailer.mkv")
	told(changes, "The General (1926)/Extras/Trailer.mkv")
	// go-smb2's Rename shares no read access: it cannot rename a folder
	// open to watch, which clients sharing it can.
	if err := share.Rename(path.Join(scratch, "The General (1926)", "The General (1926).mkv"),
		path.Join(scratch, "The General (1926)", "The General (1926) 2.mkv")); err != nil {
		t.Fatal(err)
	}
	told(changes, "The General (1926)/The General (1926) 2.mkv")
}
