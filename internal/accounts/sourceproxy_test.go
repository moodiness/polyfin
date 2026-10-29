package accounts

import (
	"errors"
	"testing"
)

// The source proxy is trimmed, saved, read again by a new start, emptied,
// and refused when it is not a proxy URL.
func TestSourceProxyIsSaved(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	changed := store.Settings()
	changed.SourceProxy = " socks5h://warp:1080 "
	saved, err := store.UpdateSettings(ctx, changed)
	if err != nil || saved.SourceProxy != "socks5h://warp:1080" {
		t.Fatalf("saved: %q, %v", saved.SourceProxy, err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings().SourceProxy; got != "socks5h://warp:1080" {
		t.Errorf("read again: %q", got)
	}
	changed.SourceProxy = "warp:1080"
	if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidSourceProxy) {
		t.Errorf("an invalid proxy: %v", err)
	}
	changed.SourceProxy = ""
	if saved, err := store.UpdateSettings(ctx, changed); err != nil || saved.SourceProxy != "" {
		t.Errorf("emptied: %q, %v", saved.SourceProxy, err)
	}
}
