package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
)

// Windows refuses to remove a file another process holds open, as the
// quit command does each time it reads the app's record, and as a file
// scanner may: the app removes its record once the file is free, rather
// than leave it behind.
func TestTheRecordHeldOpenIsRemovedOnceFree(t *testing.T) {
	folders := desktop.Folders{Data: t.TempDir()}
	_, stop, err := ListenQuit(folders)
	if err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(folders.Instance())
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(300*time.Millisecond, func() { held.Close() })
	stop()
	if _, err := os.Stat(folders.Instance()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the record is left: %v", err)
	}
}
