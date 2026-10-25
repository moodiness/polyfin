package themecopy

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// cdn is a fake CDN serving a theme: a stylesheet importing another, which
// names a font and an image, a script, and what each test adds. version
// changes what every file says, as a new release of the theme would.
type cdn struct {
	server  *httptest.Server
	version atomic.Int32
	files   map[string]cdnFile
}

type cdnFile struct {
	contentType string
	body        func(c *cdn) string
}

func newCDN(t *testing.T) *cdn {
	t.Helper()
	c := &cdn{files: map[string]cdnFile{
		"/theme/theme.css": {"text/css", func(c *cdn) string {
			return `@import url("parts/colors.css");` + "\n" + `body { background: url('img/back.png'); } /* v` + c.v() + ` */`
		}},
		"/theme/parts/colors.css": {"text/css", func(c *cdn) string {
			return `@font-face { font-family: X; src: url(../fonts/x.woff2) format("woff2"); } .v` + c.v() + ` {}`
		}},
		"/theme/fonts/x.woff2": {"font/woff2", func(c *cdn) string { return "wOF2" + c.v() }},
		"/theme/img/back.png":  {"image/png", func(c *cdn) string { return "\x89PNG" + c.v() }},
		"/theme/theme.js": {"application/javascript", func(c *cdn) string {
			return "window.themeVersion = " + c.v() + ";\nconst more = '" + c.server.URL + "/theme/extra/';\n"
		}},
	}}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, ok := c.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", file.contentType)
		_, _ = w.Write([]byte(file.body(c)))
	}))
	t.Cleanup(c.server.Close)
	return c
}

func (c *cdn) v() string { return string(rune('0' + c.version.Load())) }

// allowOnly is a client reaching the given servers only, as the copier's
// own client reaches the internet only: the check is the same, made at
// each connection.
func allowOnly(servers ...*httptest.Server) *http.Client {
	allowed := map[netip.AddrPort]bool{}
	for _, server := range servers {
		address := server.Listener.Addr().(*net.TCPAddr).AddrPort()
		allowed[netip.AddrPortFrom(address.Addr().Unmap(), address.Port())] = true
	}
	return newClient(func(address netip.AddrPort) bool {
		return allowed[netip.AddrPortFrom(address.Addr().Unmap(), address.Port())]
	})
}

type fixture struct {
	store  *accounts.Store
	copier *Copier
	dir    string
}

func newFixture(t *testing.T, client *http.Client, css, js string) fixture {
	t.Helper()
	pool := migrated.New(t)
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.WebTheme, settings.CustomCss, settings.CustomJs = accounts.WebThemeCustom, css, js
	if _, err := store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "themes")
	copier, err := New(t.Context(), Options{DB: pool, Accounts: store, Dir: dir, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: store, copier: copier, dir: dir}
}

func (f fixture) read(t *testing.T, id, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(f.dir, id, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// fileNamed returns the file of the copy that came from address.
func fileNamed(t *testing.T, kept *Copy, address string) File {
	t.Helper()
	for _, file := range kept.Files {
		if file.Address == address {
			return file
		}
	}
	t.Fatalf("no file from %s in %+v", address, kept.Files)
	return File{}
}

// A copy saves every file the texts load, through @import and url() and
// the script's address, rewrites the texts and the stylesheets to the
// copy's files, and tells what it could not follow. Making it again takes
// the files again, as a new copy whose addresses browsers have not kept,
// and removes the former one; removing it puts the texts back.
func TestACopyKeepsUpdatesAndRemovesTheThemesFiles(t *testing.T) {
	t.Parallel()
	c := newCDN(t)
	css := `@import url('` + c.server.URL + `/theme/theme.css');` + "\n.mine { color: red; }"
	js := `(() => { const s = document.createElement('script'); s.src = "` + c.server.URL + `/theme/theme.js"; document.head.appendChild(s); })();`
	f := newFixture(t, allowOnly(c.server), css, js)

	status, err := f.copier.Keep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	kept := status.Copy
	if kept == nil || len(kept.Files) != 5 {
		t.Fatalf("the copy: %+v", kept)
	}
	stylesheet := fileNamed(t, kept, c.server.URL+"/theme/theme.css")
	colors := fileNamed(t, kept, c.server.URL+"/theme/parts/colors.css")
	font := fileNamed(t, kept, c.server.URL+"/theme/fonts/x.woff2")
	image := fileNamed(t, kept, c.server.URL+"/theme/img/back.png")
	script := fileNamed(t, kept, c.server.URL+"/theme/theme.js")
	if stylesheet.Kind != KindStylesheet || font.Kind != KindFont || image.Kind != KindImage || script.Kind != KindScript ||
		!strings.HasSuffix(font.Name, ".woff2") || !strings.HasSuffix(image.Name, ".png") {
		t.Errorf("kinds and names: %+v", kept.Files)
	}
	if len(kept.Unsaved) != 1 || kept.Unsaved[0].Reason != ReasonBuiltAtRunTime || kept.Unsaved[0].Address != c.server.URL+"/theme/extra/" {
		t.Errorf("unsaved: %+v", kept.Unsaved)
	}
	settings := f.store.Settings()
	prefix := Path + kept.ID + "/"
	if want := `@import url('` + prefix + stylesheet.Name + `');` + "\n.mine { color: red; }"; settings.CustomCss != want {
		t.Errorf("the CSS became %q, want %q", settings.CustomCss, want)
	}
	if !strings.Contains(settings.CustomJs, `s.src = "`+prefix+script.Name+`"`) || strings.Contains(settings.CustomJs, c.server.URL) {
		t.Errorf("the script became %q", settings.CustomJs)
	}
	// The stylesheets name their neighbours in the copy's folder.
	if got := f.read(t, kept.ID, stylesheet.Name); !strings.Contains(got, `@import url("`+colors.Name+`")`) || !strings.Contains(got, `url('`+image.Name+`')`) {
		t.Errorf("the stylesheet saved: %q", got)
	}
	if got := f.read(t, kept.ID, colors.Name); !strings.Contains(got, `url(`+font.Name+`)`) {
		t.Errorf("the imported stylesheet saved: %q", got)
	}
	if got := f.read(t, kept.ID, font.Name); got != "wOF20" {
		t.Errorf("the font saved: %q", got)
	}
	if status.Outside == nil || len(status.Outside) != 0 || len(status.Missing) != 0 {
		t.Errorf("after the copy, outside %v, missing %v", status.Outside, status.Missing)
	}

	// The theme changes on its CDN: the copy made again holds the new
	// files, under a new id, and the former folder is gone.
	c.version.Store(1)
	status, err = f.copier.Keep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	again := status.Copy
	if again.ID == kept.ID || len(again.Files) != 5 {
		t.Fatalf("the copy made again: %+v", again)
	}
	if got := f.read(t, again.ID, fileNamed(t, again, c.server.URL+"/theme/fonts/x.woff2").Name); got != "wOF21" {
		t.Errorf("the font made again: %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, kept.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the former copy's folder: %v", err)
	}
	if got := f.store.Settings().CustomCss; !strings.Contains(got, Path+again.ID+"/") || strings.Contains(got, kept.ID) {
		t.Errorf("the CSS after the copy made again: %q", got)
	}

	status, err = f.copier.Remove(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Copy != nil {
		t.Errorf("a copy is still told: %+v", status.Copy)
	}
	if got := f.store.Settings(); got.CustomCss != css || got.CustomJs != js {
		t.Errorf("the texts after removing the copy: %q and %q", got.CustomCss, got.CustomJs)
	}
	if _, err := os.Stat(filepath.Join(f.dir, again.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the copy's folder after removing it: %v", err)
	}
	if _, err := f.copier.Remove(t.Context()); !errors.Is(err, ErrNoCopy) {
		t.Errorf("removing no copy: %v", err)
	}
}

// Only files on the internet are asked for: an address on the local
// network is refused, before connecting, and so is one an allowed address
// redirects to.
func TestACopyRefusesPrivateAddressesAndRedirectsToThem(t *testing.T) {
	t.Parallel()
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body {}"))
	}))
	t.Cleanup(private.Close)
	var asked atomic.Int32
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		http.Redirect(w, r, private.URL+"/theme.css", http.StatusFound)
	}))
	t.Cleanup(counting.Close)

	// The copier's own client: the test servers are on the loopback, which
	// is no public address.
	f := newFixture(t, nil, `@import url("`+private.URL+`/theme.css");`, "")
	_, err := f.copier.Keep(t.Context())
	var nothing *NothingSavedError
	if !errors.As(err, &nothing) || len(nothing.Unsaved) != 1 || nothing.Unsaved[0].Reason != ReasonPrivate {
		t.Fatalf("a private address: %v %+v", err, nothing)
	}
	if got := f.store.Settings().CustomCss; got != `@import url("`+private.URL+`/theme.css");` {
		t.Errorf("the texts changed: %q", got)
	}

	// An allowed address redirecting to one that is not.
	f = newFixture(t, allowOnly(counting), `@import url("`+counting.URL+`/theme.css");`, "")
	_, err = f.copier.Keep(t.Context())
	if !errors.As(err, &nothing) || len(nothing.Unsaved) != 1 || nothing.Unsaved[0].Reason != ReasonPrivate || asked.Load() != 1 {
		t.Fatalf("a redirect to a private address: %v %+v, asked %d", err, nothing, asked.Load())
	}
	entries, _ := os.ReadDir(f.dir)
	if len(entries) != 0 {
		t.Errorf("a refused copy left %v", entries)
	}
}

// A file above MaxFileBytes and one of a type the place asks for is not
// are not saved, and told; the others are, an image's fragment kept.
func TestACopyRefusesOversizeFilesAndWrongTypes(t *testing.T) {
	t.Parallel()
	c := newCDN(t)
	c.files["/big.png"] = cdnFile{"image/png", func(*cdn) string { return strings.Repeat("a", MaxFileBytes+1) }}
	c.files["/page.css"] = cdnFile{"text/html", func(*cdn) string { return "<html></html>" }}
	c.files["/icons.svg"] = cdnFile{"image/svg+xml", func(*cdn) string { return "<svg/>" }}
	c.files["/style.css"] = cdnFile{"text/css", func(*cdn) string {
		return `@import "` + c.server.URL + `/page.css"; .a { background: url(` + c.server.URL + `/big.png); } .b { background: url(` + c.server.URL + `/theme/img/back.png); }` +
			` .c { background: url(` + c.server.URL + `/icons.svg#home); }`
	}}
	f := newFixture(t, allowOnly(c.server), `@import url("`+c.server.URL+`/style.css");`, "")
	status, err := f.copier.Keep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]Unsaved{}
	for _, unsaved := range status.Copy.Unsaved {
		reasons[strings.TrimPrefix(unsaved.Address, c.server.URL)] = unsaved
	}
	if got := reasons["/big.png"]; got.Reason != ReasonTooLarge {
		t.Errorf("an oversize file: %+v", got)
	}
	if got := reasons["/page.css"]; got.Reason != ReasonWrongType || got.Detail != "text/html" {
		t.Errorf("a page imported as a stylesheet: %+v", got)
	}
	if len(status.Copy.Files) != 3 {
		t.Errorf("files saved: %+v", status.Copy.Files)
	}
	saved := f.read(t, status.Copy.ID, status.Copy.Files[0].Name)
	// An image's fragment, such as an SVG symbol, stays on the file saved.
	if icons := fileNamed(t, status.Copy, c.server.URL+"/icons.svg"); !strings.Contains(saved, "url("+icons.Name+"#home)") {
		t.Errorf("the SVG symbol: %q", saved)
	}
	if !strings.Contains(saved, c.server.URL+"/big.png") || !strings.Contains(saved, c.server.URL+"/page.css") {
		t.Errorf("the addresses not saved should stay as they were: %q", saved)
	}
}

// Texts loading nothing from elsewhere have nothing to copy.
func TestACopyOfNothingIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil, `@import url('polyfin/themes/lumaaglaass/x/assets/lumaaglaass.css'); .a { background: url(data:image/png;base64,AA==); }`, "console.log('local')")
	if _, err := f.copier.Keep(t.Context()); !errors.Is(err, ErrNothingToSave) {
		t.Errorf("nothing to save: %v", err)
	}
}

// Only a copy's own files are opened, never a path out of the folder, nor
// a name a copy never gives.
func TestOnlyACopysFilesAreOpened(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	id := "0123456789abcdef"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id, "001-theme.css"), []byte("a {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-secret.css")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(dir, id, "002-link.css")); err != nil {
		t.Fatal(err)
	}
	file, err := OpenFile(dir, id+"/001-theme.css")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	for _, rest := range []string{
		id + "/002-link.css",
		id + "/../" + filepath.Base(outside),
		"../" + filepath.Base(dir) + "-secret.css",
		id + "/001-theme.html",
		id + "/theme.css",
		"0123/001-theme.css",
		id,
	} {
		if file, err := OpenFile(dir, rest); err == nil {
			file.Close()
			t.Errorf("%q was opened", rest)
		}
	}
}

// A file of the copy missing from its folder, as after a backup restored on
// another system, is told, with the copy in use by the texts, and the
// address it came from is given for it, by a Copier started anew too, but
// for no other copy, no other name, and no copy once removed.
func TestAMissingFileOfTheCopyIsToldWithWhereItCameFrom(t *testing.T) {
	t.Parallel()
	c := newCDN(t)
	css := `@import url('` + c.server.URL + `/theme/theme.css');`
	f := newFixture(t, allowOnly(c.server), css, "")
	status, err := f.copier.Keep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	kept := status.Copy
	font := fileNamed(t, kept, c.server.URL+"/theme/fonts/x.woff2")
	if !status.InUse || len(status.Missing) != 0 {
		t.Fatalf("after the copy: in use %v, missing %v", status.InUse, status.Missing)
	}
	if err := os.Remove(filepath.Join(f.dir, kept.ID, font.Name)); err != nil {
		t.Fatal(err)
	}
	status, err = f.copier.Status(t.Context())
	if err != nil || !status.InUse || len(status.Missing) != 1 || status.Missing[0] != font.Name {
		t.Fatalf("with the font missing: in use %v, missing %v, %v", status.InUse, status.Missing, err)
	}
	restarted, err := New(t.Context(), Options{DB: f.copier.options.DB, Accounts: f.store, Dir: f.dir, Client: f.copier.options.Client})
	if err != nil {
		t.Fatal(err)
	}
	for name, copier := range map[string]*Copier{"the copier": f.copier, "a copier started anew": restarted} {
		if address, ok := copier.Original(kept.ID, font.Name); !ok || address != font.Address {
			t.Errorf("%s: the font's original is %q, %v", name, address, ok)
		}
		if _, ok := copier.Original("0123456789abcdef", font.Name); ok {
			t.Errorf("%s: another copy's file has an original", name)
		}
		if _, ok := copier.Original(kept.ID, "009-other.woff2"); ok {
			t.Errorf("%s: a name the copy does not hold has an original", name)
		}
	}
	// An address that is not http, as a backup made by hand could hold, is
	// never given.
	if _, err := f.copier.options.DB.Exec(t.Context(), "UPDATE theme_copy SET files = jsonb_set(files, '{0,address}', '\"javascript:alert(1)\"')"); err != nil {
		t.Fatal(err)
	}
	crafted, err := New(t.Context(), Options{DB: f.copier.options.DB, Accounts: f.store, Dir: f.dir, Client: f.copier.options.Client})
	if err != nil {
		t.Fatal(err)
	}
	if address, ok := crafted.Original(kept.ID, kept.Files[0].Name); ok {
		t.Errorf("an address that is not http was given: %q", address)
	}
	// Texts that no longer name the copy do not use it.
	settings := f.store.Settings()
	settings.CustomCss = ".mine {}"
	if _, err := f.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if status, err := f.copier.Status(t.Context()); err != nil || status.InUse {
		t.Errorf("texts without the copy: in use %v, %v", status.InUse, err)
	}
	settings.CustomCss = `@import url('` + Path + kept.ID + `/` + fileNamed(t, kept, c.server.URL+"/theme/theme.css").Name + `');`
	if _, err := f.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if _, err := f.copier.Remove(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.copier.Original(kept.ID, font.Name); ok {
		t.Error("a removed copy's file has an original")
	}
}
