package accounts

import "testing"

// SettingsChanged is closed by the next change of the settings, once the
// new settings are those read, and a new channel waits for the one after.
func TestSettingsChangedTellsTheNextChange(t *testing.T) {
	store := newStore(t)
	changed := store.SettingsChanged()
	if again := store.SettingsChanged(); again != changed {
		t.Error("a second channel before any change")
	}
	select {
	case <-changed:
		t.Fatal("closed before any change")
	default:
	}
	settings := store.Settings()
	settings.LocalDiscovery = false
	if _, err := store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("not closed by the change")
	}
	if store.Settings().LocalDiscovery {
		t.Error("closed before the change is read")
	}
	next := store.SettingsChanged()
	select {
	case <-next:
		t.Error("the next channel is closed already")
	default:
	}
}
