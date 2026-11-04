package library

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// fakeArtworkFailures is an image relay whose failing images are given,
// and which keeps the addresses it is told to forget.
type fakeArtworkFailures struct {
	mu        sync.Mutex
	failing   map[string]bool
	forgotten []string
}

func (f *fakeArtworkFailures) ArtworkFailing(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failing[url]
}

func (f *fakeArtworkFailures) ForgetArtwork(urls []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgotten = append(f.forgotten, urls...)
	for _, url := range urls {
		delete(f.failing, url)
	}
}

func (f *fakeArtworkFailures) forgot(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.forgotten, url)
}

// A person whose photo from the addon is found dead shows TMDB's, looked
// up as for a person without one; once TMDB's is found dead too, none, so
// that apps draw their placeholder. The addon describing the title again
// keeps the photo dead, for a day, unless it gives a new one.
func TestADeadPhotoGivesWayToTMDBsThenToNone(t *testing.T) {
	t.Parallel()
	addonPhoto := "https://images.example/dana.jpg"
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Dana Actor", Photo: addonPhoto}})
	e.tmdb.searches["Dana Actor"] = []map[string]any{{"id": 71, "name": "Dana Actor", "popularity": 3, "profile_path": "/dana.jpg"}}
	e.tmdb.addPerson("71", "Dana Actor", "/dana.jpg", "Dana's life.", nil)
	tmdbPhoto := e.tmdb.URL + "/img/original/dana.jpg"
	dana := PersonID("Dana Actor")
	photos := func(when string, want string) {
		t.Helper()
		if got := e.cast(e.admin)["Dana Actor"].Image; got != want {
			t.Errorf("%s: the cast shows %q, want %q", when, got, want)
		}
		if got := e.person(e.admin, "Dana Actor").Images.Primary; got != want {
			t.Errorf("%s: the person's page shows %q, want %q", when, got, want)
		}
	}

	photos("the addon's photo alive", addonPhoto)
	if err := e.service.PhotoFailed(t.Context(), dana, addonPhoto); err != nil {
		t.Fatal(err)
	}
	photos("the addon's photo dead", tmdbPhoto)
	// The addon credits Dana again, with the same photo: it stays dead.
	if err := e.service.saveCredits(t.Context(), e.heist, []Person{{Name: "Dana Actor", Type: "Actor", Image: addonPhoto}}, false); err != nil {
		t.Fatal(err)
	}
	photos("described again", tmdbPhoto)
	if err := e.service.PhotoFailed(t.Context(), dana, tmdbPhoto); err != nil {
		t.Fatal(err)
	}
	photos("both photos dead", "")

	e.clock.advance(deadPhotoFor + time.Second)
	photos("a day later", addonPhoto)

	// A new photo from the addon is not dead.
	if err := e.service.PhotoFailed(t.Context(), dana, addonPhoto); err != nil {
		t.Fatal(err)
	}
	newPhoto := "https://images.example/dana-new.jpg"
	if err := e.service.saveCredits(t.Context(), e.heist, []Person{{Name: "Dana Actor", Type: "Actor", Image: newPhoto}}, false); err != nil {
		t.Fatal(err)
	}
	r, err := e.service.load(t.Context(), dana)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.personPhoto(e.clock.Now()); got != newPhoto {
		t.Errorf("a new photo: %q", got)
	}
	if _, kept := r.DeadPhotos[addonPhoto]; kept {
		t.Errorf("the former photo is still marked dead: %v", r.DeadPhotos)
	}
}

// Refresh metadata on a title forgets the failures of its artwork and of
// its people's photos, and looks those without a working photo up on TMDB
// again, without the answers TMDB gave before; the others are not asked
// about.
func TestRefreshLooksPeopleWithoutAWorkingPhotoUpAgain(t *testing.T) {
	t.Parallel()
	cleoPhoto, finnPhoto := "https://images.example/cleo.jpg", "https://images.example/finn.jpg"
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Cleo Actor", Photo: cleoPhoto}, {Name: "Finn Actor", Photo: finnPhoto}, {Name: "Gus Actor"}})
	relay := &fakeArtworkFailures{failing: map[string]bool{}}
	e.service.UseArtworkFailures(relay)
	e.tmdb.searches["Cleo Actor"] = []map[string]any{{"id": 51, "name": "Cleo Actor", "popularity": 3, "profile_path": "/cleo.jpg"}}
	e.tmdb.addPerson("51", "Cleo Actor", "/cleo.jpg", "Cleo's life.", nil)
	e.tmdb.searches["Finn Actor"] = []map[string]any{{"id": 61, "name": "Finn Actor", "popularity": 3, "profile_path": "/finn.jpg"}}
	e.tmdb.addPerson("61", "Finn Actor", "/finn.jpg", "Finn's life.", nil)

	// TMDB does not know Gus yet: he shows no photo, and is not asked
	// about again for a month.
	if got := e.cast(e.admin)["Gus Actor"].Image; got != "" {
		t.Fatalf("Gus Actor unknown to TMDB: %q", got)
	}
	e.mu(func() {
		e.tmdb.searches["Gus Actor"] = []map[string]any{{"id": 91, "name": "Gus Actor", "popularity": 3, "profile_path": "/gus.jpg"}}
		e.tmdb.people["91"] = map[string]any{"id": 91, "name": "Gus Actor", "profile_path": "/gus.jpg"}
	})
	if got := e.cast(e.admin)["Gus Actor"].Image; got != "" {
		t.Fatalf("Gus Actor looked up again without a refresh: %q", got)
	}
	// Cleo's photo fails to download.
	relay.mu.Lock()
	relay.failing[cleoPhoto] = true
	relay.mu.Unlock()

	if err := e.service.Refresh(t.Context(), e.admin, e.heist); err != nil {
		t.Fatal(err)
	}
	e.service.personLookups.wg.Wait()
	cast := e.cast(e.admin)
	if got := cast["Gus Actor"].Image; got != e.tmdb.URL+"/img/original/gus.jpg" {
		t.Errorf("Gus Actor after Refresh metadata: %q", got)
	}
	if got := e.tmdb.requests("/3/search/person?Cleo Actor"); got != 1 {
		t.Errorf("Cleo Actor, whose photo failed, looked up %d times", got)
	}
	if got := cast["Cleo Actor"].Image; got != cleoPhoto || !relay.forgot(cleoPhoto) {
		t.Errorf("Cleo Actor's photo after Refresh metadata: %q, forgotten %v", got, relay.forgot(cleoPhoto))
	}
	if got := e.tmdb.requests("/3/search/person?Finn Actor"); got != 0 {
		t.Errorf("Finn Actor, whose photo works, looked up %d times", got)
	}
}

// Refresh metadata on a person forgets their photo's failures and looks
// them up on TMDB again.
func TestRefreshLooksAPersonUpAgain(t *testing.T) {
	t.Parallel()
	hanaPhoto := "https://images.example/hana.jpg"
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Hana Actor", Photo: hanaPhoto}})
	relay := &fakeArtworkFailures{failing: map[string]bool{}}
	e.service.UseArtworkFailures(relay)
	e.tmdb.searches["Hana Actor"] = []map[string]any{{"id": 81, "name": "Hana Actor", "popularity": 3, "profile_path": "/hana.jpg"}}
	e.tmdb.addPerson("81", "Hana Actor", "/hana.jpg", "Hana's life.", nil)
	e.cast(e.admin)
	if got := e.person(e.admin, "Hana Actor"); got.Overview != "Hana's life." {
		t.Fatalf("Hana Actor's page: %q", got.Overview)
	}
	e.mu(func() { e.tmdb.people["81/en"] = map[string]any{"biography": "Hana's new life."} })
	if err := e.service.PhotoFailed(t.Context(), PersonID("Hana Actor"), hanaPhoto); err != nil {
		t.Fatal(err)
	}
	if got := e.person(e.admin, "Hana Actor"); got.Overview != "Hana's life." || got.Images.Primary == hanaPhoto {
		t.Fatalf("before Refresh metadata: %q, photo %q", got.Overview, got.Images.Primary)
	}

	if err := e.service.Refresh(t.Context(), e.admin, PersonID("Hana Actor")); err != nil {
		t.Fatal(err)
	}
	e.service.personLookups.wg.Wait()
	if got := e.person(e.admin, "Hana Actor"); got.Overview != "Hana's new life." || got.Images.Primary != hanaPhoto || !relay.forgot(hanaPhoto) {
		t.Errorf("after Refresh metadata: %q, photo %q, forgotten %v", got.Overview, got.Images.Primary, relay.forgot(hanaPhoto))
	}
}

// Health tells the people TMDB found, did not find, and whose lookup
// failed.
func TestPeopleStatusTellsTheLookups(t *testing.T) {
	t.Parallel()
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Ivy Actor"}, {Name: "Jon Actor"}, {Name: "Kai Actor"}})
	e.tmdb.searches["Ivy Actor"] = []map[string]any{{"id": 41, "name": "Ivy Actor", "popularity": 3, "profile_path": "/ivy.jpg"}}
	e.tmdb.addPerson("41", "Ivy Actor", "/ivy.jpg", "Ivy's life.", nil)
	e.tmdb.failing["Kai Actor"] = true
	e.cast(e.admin)
	status, err := e.service.PeopleStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Found != 1 || status.NotFound != 1 || status.Failing != 1 || status.Waiting != 0 || !status.LimitedAt.IsZero() {
		t.Errorf("status: %+v", status)
	}
}
