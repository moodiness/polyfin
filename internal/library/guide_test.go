package library

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// guideServer serves an XMLTV guide, compressed, counting downloads; status
// replaces it with an error while set.
type guideServer struct {
	mu        sync.Mutex
	body      string
	status    int
	downloads atomic.Int32
	url       string
}

func newGuideServer(t *testing.T, body string) *guideServer {
	t.Helper()
	g := &guideServer{body: body}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		g.downloads.Add(1)
		g.mu.Lock()
		body, status := g.body, g.status
		g.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		writer := gzip.NewWriter(w)
		_, _ = writer.Write([]byte(body))
		_ = writer.Close()
	}))
	t.Cleanup(server.Close)
	// The name says nothing of the compression: it is told by content.
	g.url = server.URL + "/guide.xml?token=secret"
	return g
}

func (g *guideServer) set(body string, status int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.body, g.status = body, status
}

// xmltvTime formats a time as XMLTV does, in a zone of its own.
func xmltvTime(at time.Time) string {
	return at.In(time.FixedZone("", 3600)).Format("20060102150405 -0700")
}

func programme(channel string, start, stop time.Time, title string) string {
	return fmt.Sprintf(`<programme channel="%s" start="%s" stop="%s"><title>%s</title></programme>`, channel, xmltvTime(start), xmltvTime(stop), title)
}

// tvCatalog installs, in the server's addons, an addon whose live TV
// catalog lists channels, without a Native EPG guide.
func (e env) tvCatalog(scope addons.Scope, channels ...stremio.Meta) addons.LibraryKey {
	e.t.Helper()
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "iptv", Name: "IPTV", Version: "1", Types: []string{"tv"},
			Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "tv", ID: "channels", Name: "Channels"}}},
		catalogs: map[string][]stremio.Meta{"tv/channels": channels},
	}
	e.install(scope, addon)
	list, err := e.addons.Addons(e.t.Context(), scope)
	if err != nil || len(list) == 0 {
		e.t.Fatal(list, err)
	}
	return addons.LibraryKey{AddonID: list[len(list)-1].ID, CatalogType: "tv", CatalogID: "channels"}
}

func (e env) guideOf(scope addons.Scope, key addons.LibraryKey) addons.Guide {
	e.t.Helper()
	libraries, err := e.addons.Libraries(e.t.Context(), scope)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, l := range libraries {
		if l.AddonID == key.AddonID && l.Catalog.ID == key.CatalogID && l.Guide != nil {
			return *l.Guide
		}
	}
	e.t.Fatalf("no guide for %v", key)
	return addons.Guide{}
}

func programTitles(programs []Item) []string {
	result := make([]string, 0, len(programs))
	for _, p := range programs {
		result = append(result, p.Channel.Name+": "+p.Name)
	}
	return result
}

// A guide's channels match the catalog's by Stremio ID first, else by
// name once dressing is folded away; only the programmes of a day ago to
// eight days ahead are kept, and they feed the programme listings.
func TestXMLTVGuideFeedsTheCatalogsChannels(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	key := e.tvCatalog(addons.Shared(),
		stremio.Meta{ID: "tv:one", Type: "tv", Name: "FR: Une ᴴᴰ"},
		stremio.Meta{ID: "two.fr", Type: "tv", Name: "Deux"},
		stremio.Meta{ID: "tv:three", Type: "tv", Name: "Trois"},
	)
	guide := newGuideServer(t, `<?xml version="1.0" encoding="UTF-8"?><tv>
		<channel id="une"><display-name>Une</display-name></channel>
		<channel id="deux-by-name"><display-name>DEUX</display-name></channel>
		<channel id="other"><display-name>Autre</display-name></channel>`+
		programme("une", now.Add(-30*time.Minute), now.Add(30*time.Minute), "Le Journal")+
		programme("une", now.Add(30*time.Minute), now.Add(2*time.Hour), "Le Film")+
		programme("une", now.Add(-50*time.Hour), now.Add(-49*time.Hour), "Too old")+
		programme("une", now.Add(9*24*time.Hour), now.Add(9*24*time.Hour+time.Hour), "Too far")+
		programme("deux-by-name", now.Add(-time.Hour), now.Add(time.Hour), "By name")+
		// Declared nowhere, but the channel's Stremio ID: it wins.
		programme("two.fr", now.Add(-time.Hour), now.Add(time.Hour), "By id")+
		programme("other", now.Add(-time.Hour), now.Add(time.Hour), "Unmatched")+
		`</tv>`)
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	status := e.guideOf(addons.Shared(), key)
	if status.Error != "" || status.FetchedAt == nil || !status.FetchedAt.Equal(now) || status.Channels != 3 || status.Matched != 2 {
		t.Fatalf("guide status: %+v", status)
	}

	programs, err := e.service.Programs(t.Context(), e.member, now.Add(-3*24*time.Hour), now.Add(10*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deux: By id", "FR: Une ᴴᴰ: Le Journal", "FR: Une ᴴᴰ: Le Film"}
	if got := programTitles(programs); !slices.Equal(got, want) {
		t.Errorf("programmes %q, want %q", got, want)
	}
	airing, err := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := programTitles(airing); !slices.Equal(got, []string{"Deux: By id", "FR: Une ᴴᴰ: Le Journal"}) {
		t.Errorf("airing now: %q", got)
	}
	// A programme is found again by its identifier.
	item, err := e.service.Item(t.Context(), e.member, airing[1].ID)
	if err != nil || item.Name != "Le Journal" || item.Kind != KindProgram || item.Channel.Name != "FR: Une ᴴᴰ" {
		t.Errorf("programme by id: %+v %v", item, err)
	}
	// A user without Live TV reaches none of it.
	member, err := e.users.UpdateUser(t.Context(), e.member.ID, accounts.UserChanges{LiveTv: new(false)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if programs, _ := e.service.Programs(t.Context(), member, now, now.Add(time.Second)); len(programs) != 0 {
		t.Errorf("programmes of a user without Live TV: %q", programTitles(programs))
	}
}

// Guides are fetched again every 12 hours; a failed fetch keeps the last
// programmes and tells why it failed.
func TestXMLTVGuidesAreRefreshed(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"})
	first := `<tv><channel id="1"><display-name>One</display-name></channel>` + programme("1", now.Add(-time.Hour), now.Add(time.Hour), "First") + `</tv>`
	guide := newGuideServer(t, first)
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	airing := func() []string {
		t.Helper()
		programs, err := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		return programTitles(programs)
	}
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if got := airing(); !slices.Equal(got, []string{"One: First"}) || guide.downloads.Load() != 1 {
		t.Fatalf("first fetch: %q after %d downloads", got, guide.downloads.Load())
	}

	guide.set(strings.Replace(first, "First", "Second", 1), 0)
	now = now.Add(11 * time.Hour)
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if guide.downloads.Load() != 1 {
		t.Errorf("a guide fetched 11 hours ago was fetched again")
	}
	now = now.Add(time.Hour)
	if err := e.service.RefreshGuides(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(-12 * time.Hour)
	if got := airing(); !slices.Equal(got, []string{"One: Second"}) || guide.downloads.Load() != 2 {
		t.Errorf("after 12 hours: %q after %d downloads", got, guide.downloads.Load())
	}
	// Run by hand, every guide is fetched, due or not.
	if err := e.service.RefreshGuides(t.Context(), true); err != nil || guide.downloads.Load() != 3 {
		t.Errorf("by hand: %v after %d downloads", err, guide.downloads.Load())
	}
	fetched := e.guideOf(addons.Shared(), key).FetchedAt

	// The button fetches at once, due or not.
	guide.set("", http.StatusInternalServerError)
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	status := e.guideOf(addons.Shared(), key)
	if status.Error != "unreachable" || !status.FetchedAt.Equal(*fetched) || !status.CheckedAt.Equal(now) {
		t.Errorf("failed fetch: %+v", status)
	}
	if got := airing(); !slices.Equal(got, []string{"One: Second"}) {
		t.Errorf("programmes after a failed fetch: %q", got)
	}
	for body, code := range map[string]string{"<html>Sign in</html>": "malformed", "<tv></tv>": ""} {
		guide.set(body, 0)
		if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
			t.Fatal(err)
		}
		if status := e.guideOf(addons.Shared(), key); status.Error != code {
			t.Errorf("%s: error %q, want %q", body, status.Error, code)
		}
	}
	if got := airing(); len(got) != 0 {
		t.Errorf("a guide fetched empty keeps programmes: %q", got)
	}
}

// Guides are due again after the hours the settings give.
func TestGuidesFollowTheRefreshSetting(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	e.setting(func(s *accounts.Settings) { s.LiveTvRefreshHours = 2 })
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"})
	guide := newGuideServer(t, `<tv><channel id="1"><display-name>One</display-name></channel></tv>`)
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		after     time.Duration
		downloads int32
	}{{0, 1}, {119 * time.Minute, 1}, {time.Minute, 2}} {
		now = now.Add(step.after)
		if err := e.service.RefreshGuides(t.Context(), false); err != nil {
			t.Fatal(err)
		}
		if got := guide.downloads.Load(); got != step.downloads {
			t.Errorf("after %v more: %d downloads, want %d", step.after, got, step.downloads)
		}
	}
}

// A guide published as a ZIP archive is spooled in the cache folder while
// it is read, then removed, after a success as after a failure.
func TestXMLTVGuidesInZIPArchives(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	dir := filepath.Join(t.TempDir(), "guides")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "guide-left.zip"), []byte("left by a crash"), 0o600)
	if err := e.service.SpoolGuidesIn(dir); err != nil {
		t.Fatal(err)
	}
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"})
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	w, _ := writer.Create("epg.xml")
	_, _ = io.WriteString(w, `<tv><channel id="1"><display-name>One</display-name></channel>`+
		programme("1", now.Add(-time.Hour), now.Add(time.Hour), "Zipped")+`</tv>`)
	_ = writer.Close()
	var body atomic.Pointer[[]byte]
	body.Store(new(archive.Bytes()))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(*body.Load()) }))
	t.Cleanup(server.Close)
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, server.URL+"/epg"); err != nil {
		t.Fatal(err)
	}
	empty := func(when string) {
		t.Helper()
		if left, err := os.ReadDir(dir); err != nil || len(left) > 0 {
			t.Errorf("%s: left in the cache folder: %v %v", when, left, err)
		}
	}
	empty("before")
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	empty("after a success")
	programs, err := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second))
	if err != nil || !slices.Equal(programTitles(programs), []string{"One: Zipped"}) {
		t.Errorf("programmes of a zipped guide: %q %v", programTitles(programs), err)
	}
	body.Store(new(archive.Bytes()[:archive.Len()/2]))
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	if status := e.guideOf(addons.Shared(), key); status.Error != "malformed" {
		t.Errorf("truncated archive: %+v", status)
	}
	empty("after a failure")
}

// Saving the libraries keeps the guides of those kept; a new address, or
// none, forgets the programmes of the previous one.
func TestGuidesFollowTheirCatalog(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"})
	guide := newGuideServer(t, `<tv><channel id="1"><display-name>One</display-name></channel>`+
		programme("1", now.Add(-time.Hour), now.Add(time.Hour), "News")+`</tv>`)
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	if _, err := e.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: key.AddonID, CatalogType: "tv", CatalogID: "channels"}}); err != nil {
		t.Fatal(err)
	}
	if status := e.guideOf(addons.Shared(), key); status.URL != guide.url || status.Matched != 1 {
		t.Errorf("guide after saving the libraries: %+v", status)
	}
	if programs, _ := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second)); len(programs) != 1 {
		t.Errorf("programmes after saving the libraries: %q", programTitles(programs))
	}
	for _, bad := range []string{"ftp://guide.example/a.xml", "guide.xml", "https://", "https://x/" + strings.Repeat("a", 4096)} {
		if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, bad); err != addons.ErrInvalidGuideURL {
			t.Errorf("%.30q: %v", bad, err)
		}
	}
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: key.AddonID, CatalogType: "movie", CatalogID: "channels"}, guide.url); err != addons.ErrInvalidLibrary {
		t.Errorf("a guide for another catalog: %v", err)
	}
	if err := e.addons.SetGuide(t.Context(), addons.Shared(), key, ""); err != nil {
		t.Fatal(err)
	}
	if programs, _ := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second)); len(programs) != 0 {
		t.Errorf("programmes of a removed guide: %q", programTitles(programs))
	}
	if status := e.guideOf(addons.Shared(), key); status != (addons.Guide{}) {
		t.Errorf("status of a removed guide: %+v", status)
	}
}

// A user's own guide, like their addons, may only reach public addresses
// unless they are an administrator.
func TestUsersGuidesStayOnPublicAddresses(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	body := `<tv><channel id="1"><display-name>One</display-name></channel>` + programme("1", now.Add(-time.Hour), now.Add(time.Hour), "News") + `</tv>`
	for _, user := range []accounts.User{e.member, e.admin} {
		scope := addons.Personal(user.ID)
		channels := []stremio.Meta{{ID: "tv:one", Type: "tv", Name: "One"}}
		key := e.tvCatalog(scope, channels...)
		// The test addon is on this machine too: its channels are listed
		// already.
		e.service.pages.Put(pageKey{addon: key.AddonID, catalogType: "tv", catalogID: "channels"}, channels)
		guide := newGuideServer(t, body)
		if err := e.addons.SetGuide(t.Context(), scope, key, guide.url); err != nil {
			t.Fatal(err)
		}
		if err := e.service.RefreshGuide(t.Context(), scope, key); err != nil {
			t.Fatal(err)
		}
		want := "private_network"
		if user.IsAdministrator {
			want = ""
		}
		if status := e.guideOf(scope, key); status.Error != want {
			t.Errorf("%s: %+v, want error %q", user.Name, status, want)
		}
	}
	// One's own catalog is not another scope's.
	key := e.tvCatalog(addons.Personal(e.admin.ID), stremio.Meta{ID: "tv:x", Type: "tv", Name: "X"})
	if err := e.addons.SetGuide(t.Context(), addons.Personal(e.member.ID), key, "https://guide.example/x.xml"); err != addons.ErrInvalidLibrary {
		t.Errorf("a guide set on another user's catalog: %v", err)
	}
}
