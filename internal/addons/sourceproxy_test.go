package addons

import "testing"

// A server Stremio addon keeps the source proxy off until it is turned on;
// the choice is saved and read again.
func TestUseSourceProxyIsOffUntilSet(t *testing.T) {
	t.Parallel()
	store, _ := newStore(t)
	addon := newFakeAddon(t, nil)
	installed, err := store.Install(t.Context(), Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	if installed.UseSourceProxy {
		t.Fatal("the source proxy is on by default")
	}
	saved, err := store.SetUseSourceProxy(t.Context(), Shared(), installed.ID, true)
	if err != nil || !saved.UseSourceProxy {
		t.Fatalf("turned on: %v, %v", saved.UseSourceProxy, err)
	}
	found, err := store.Find(t.Context(), installed.ID)
	if err != nil || !found.UseSourceProxy {
		t.Fatalf("read again: %v, %v", found.UseSourceProxy, err)
	}
	saved, err = store.SetUseSourceProxy(t.Context(), Shared(), installed.ID, false)
	if err != nil || saved.UseSourceProxy {
		t.Fatalf("turned off: %v, %v", saved.UseSourceProxy, err)
	}
}
