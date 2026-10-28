package library

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/serverfile"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An XMLTV guide may be a file of the server, by its path: it is read as a
// guide from an address is, again as soon as it changes, and a guide gone
// is told at once; a path where there is no file is refused. A member's
// catalog never reads a file of the server.
func TestXMLTVGuidesAreReadFromFilesOfTheServer(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	at := &clock{now: time.Now().UTC().Truncate(time.Minute)}
	e.service.SetClock(at.Now)
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"})
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.xml")
	write := func(title string, modified time.Time) {
		t.Helper()
		body := `<tv><channel id="1"><display-name>One</display-name></channel>` +
			programme("1", at.Now().Add(-time.Hour), at.Now().Add(time.Hour), title) + `</tv>`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	airing := func() []string {
		t.Helper()
		programs, err := e.service.Programs(t.Context(), e.member, at.Now(), at.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		return programTitles(programs)
	}
	if _, err := e.addons.SetGuide(t.Context(), addons.Shared(), key, filepath.Join(dir, "missing.xml")); !errors.Is(err, serverfile.ErrMissing) {
		t.Errorf("a missing file: %v", err)
	}
	write("First", at.Now().Add(-time.Hour))
	if _, err := e.addons.SetGuide(t.Context(), addons.Shared(), key, path); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if got := airing(); !slices.Equal(got, []string{"One: First"}) {
		t.Fatalf("the file's programmes: %q, %+v", got, e.guideOf(addons.Shared(), key))
	}
	if status := e.guideOf(addons.Shared(), key); status.URL != path || status.Error != "" {
		t.Errorf("the guide: %+v", status)
	}
	checked := *e.guideOf(addons.Shared(), key).CheckedAt

	// Unchanged, it waits for its turn; changed, it is read at once.
	at.advance(time.Minute)
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if status := e.guideOf(addons.Shared(), key); !status.CheckedAt.Equal(checked) {
		t.Errorf("an unchanged file was read again at %v", status.CheckedAt)
	}
	write("Second", at.Now().Add(-30*time.Second))
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if got := airing(); !slices.Equal(got, []string{"One: Second"}) {
		t.Errorf("after the file changed: %q", got)
	}

	// Gone, it is told at once, its programmes kept.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	at.advance(time.Minute)
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if status := e.guideOf(addons.Shared(), key); status.Error != guideFileUnreadable {
		t.Errorf("a guide gone: %+v", status)
	}
	if got := airing(); !slices.Equal(got, []string{"One: Second"}) {
		t.Errorf("programmes of a guide gone: %q", got)
	}
}

// A member's catalog that holds a file of the server, as one an
// administrator who left gave it, never reads it.
func TestMembersCatalogsDoNotReadFilesOfTheServer(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "guide.xml")
	if err := os.WriteFile(path, []byte(`<tv><channel id="1"><display-name>One</display-name></channel>`+
		programme("1", now.Add(-time.Hour), now.Add(time.Hour), "News")+`</tv>`), 0o600); err != nil {
		t.Fatal(err)
	}
	scope := addons.Personal(e.member.ID)
	channels := []stremio.Meta{{ID: "tv:one", Type: "tv", Name: "One"}}
	key := e.tvCatalog(scope, channels...)
	e.service.pages.Put(pageKey{addon: key.AddonID, catalogType: "tv", catalogID: "channels"}, fetched[[]stremio.Meta]{channels, now})
	if _, err := e.addons.SetGuide(t.Context(), scope, key, path); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuide(t.Context(), scope, key); err != nil {
		t.Fatal(err)
	}
	if status := e.guideOf(scope, key); status.Error != guideFileRefused || status.Programmes != 0 {
		t.Errorf("a member's guide file: %+v", status)
	}
}
