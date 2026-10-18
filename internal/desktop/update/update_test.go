package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// fakeGitHub answers the releases of tags with the packages of releases,
// and serves their files.
type fakeGitHub struct {
	*httptest.Server
	releases map[string]map[string]any
	files    map[string][]byte
	asked    atomic.Int32
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{releases: map[string]map[string]any{}, files: map[string][]byte{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tag, ok := strings.CutPrefix(r.URL.Path, "/repos/moodiness/polyfin/releases/tags/"); ok {
			f.asked.Add(1)
			release, ok := f.releases[tag]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(release)
			return
		}
		body, ok := f.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.Close)
	return f
}

// publish adds a release of version with one package, whose GitHub
// checksum is digest, or the package's own when empty.
func (f *fakeGitHub) publish(version, name string, body []byte, digest string, extra map[string]any) {
	sum := sha256.Sum256(body)
	if digest == "" {
		digest = "sha256:" + hex.EncodeToString(sum[:])
	}
	f.files["/download/"+name] = body
	release := map[string]any{
		"tag_name": "v" + version, "html_url": "https://github.com/moodiness/polyfin/releases/tag/v" + version,
		"assets": []map[string]any{
			{"name": "Polyfin-Desktop-" + version + "-unrelated.zip", "browser_download_url": f.URL + "/download/other", "size": 1, "digest": digest},
			{"name": name, "browser_download_url": f.URL + "/download/" + name, "size": len(body), "digest": digest},
		},
	}
	for k, v := range extra {
		release[k] = v
	}
	f.releases["v"+version] = release
}

func checker(f *fakeGitHub, version string) *Checker {
	return &Checker{Version: version, GOARCH: "arm64", Kind: DMG, API: f.URL, Client: f.Client()}
}

// A newer release the server found offers its package for this system,
// which downloads checked against GitHub's checksum.
func TestOfferAndDownload(t *testing.T) {
	f := newFakeGitHub(t)
	body := []byte("a new disk image")
	f.publish("0.43.0", "Polyfin-Desktop-0.43.0-macos-arm64.dmg", body, "", nil)
	ctx := context.Background()
	offer, err := checker(f, "0.42.0").Offer(ctx, true, "0.43.0")
	if err != nil || offer == nil {
		t.Fatalf("Offer = %+v, %v", offer, err)
	}
	if offer.Version != "0.43.0" || offer.Asset.Name != "Polyfin-Desktop-0.43.0-macos-arm64.dmg" || offer.Notes == "" {
		t.Errorf("offer = %+v", offer)
	}
	var last int64
	file, err := Download(ctx, f.Client(), *offer, t.TempDir(), func(done, total int64) { last = done })
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if data, _ := os.ReadFile(file); string(data) != string(body) || last != int64(len(body)) {
		t.Errorf("downloaded %q, progress %d", data, last)
	}
}

// Nothing is offered while the setting is off, for a version that is not
// newer, for a development build, a pre-release, or a release without a
// package for this system; GitHub is not asked for the first three.
func TestNothingOffered(t *testing.T) {
	f := newFakeGitHub(t)
	f.publish("0.43.0", "Polyfin-Desktop-0.43.0-windows-x64-setup.exe", []byte("x"), "", nil)
	f.publish("0.44.0", "Polyfin-Desktop-0.44.0-macos-arm64.dmg", []byte("x"), "", map[string]any{"prerelease": true})
	ctx := context.Background()
	for _, c := range []struct {
		name, version, latest string
		enabled               bool
	}{
		{"setting off", "0.42.0", "0.43.0", false},
		{"same version", "0.43.0", "0.43.0", true},
		{"older", "0.43.1", "0.43.0", true},
		{"development build", "dev", "0.43.0", true},
		{"nothing found yet", "0.42.0", "", true},
	} {
		if offer, err := checker(f, c.version).Offer(ctx, c.enabled, c.latest); err != nil || offer != nil {
			t.Errorf("%s: %+v, %v", c.name, offer, err)
		}
	}
	if f.asked.Load() != 0 {
		t.Errorf("GitHub was asked %d times", f.asked.Load())
	}
	// macOS on Apple silicon: the release has only Windows' package.
	if offer, err := checker(f, "0.42.0").Offer(ctx, true, "0.43.0"); err != nil || offer != nil {
		t.Errorf("another system's package: %+v, %v", offer, err)
	}
	if offer, err := checker(f, "0.42.0").Offer(ctx, true, "0.44.0"); err != nil || offer != nil {
		t.Errorf("a pre-release: %+v, %v", offer, err)
	}
	if offer, err := checker(f, "0.42.0").Offer(ctx, true, "0.45.0"); err != nil || offer != nil {
		t.Errorf("a release GitHub does not have: %+v, %v", offer, err)
	}
}

// A package without a checksum is not offered; one that does not match
// it is refused, and nothing is left.
func TestChecksumsAreRequired(t *testing.T) {
	f := newFakeGitHub(t)
	f.publish("0.43.0", "Polyfin-Desktop-0.43.0-macos-arm64.dmg", []byte("x"), "none", nil)
	f.publish("0.44.0", "Polyfin-Desktop-0.44.0-macos-arm64.dmg", []byte("tampered"), "sha256:"+strings.Repeat("ab", 32), nil)
	ctx := context.Background()
	if _, err := checker(f, "0.42.0").Offer(ctx, true, "0.43.0"); !errors.Is(err, ErrNoChecksum) {
		t.Errorf("Offer without a checksum = %v", err)
	}
	offer, err := checker(f, "0.42.0").Offer(ctx, true, "0.44.0")
	if err != nil || offer == nil {
		t.Fatalf("Offer = %+v, %v", offer, err)
	}
	dir := t.TempDir()
	if _, err := Download(ctx, f.Client(), *offer, dir, nil); !errors.Is(err, ErrChecksum) {
		t.Errorf("Download = %v, want ErrChecksum", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the refused download left %v", entries)
	}
}

// Each system's package, as the release workflow names them.
func TestAssetNames(t *testing.T) {
	for _, c := range []struct {
		goarch string
		kind   Kind
		want   string
	}{
		{"arm64", DMG, "Polyfin-Desktop-1.2.3-macos-arm64.dmg"},
		{"amd64", DMG, "Polyfin-Desktop-1.2.3-macos-x64.dmg"},
		{"amd64", Installer, "Polyfin-Desktop-1.2.3-windows-x64-setup.exe"},
		{"amd64", Deb, "polyfin-desktop_1.2.3_amd64.deb"},
		{"arm64", Deb, "polyfin-desktop_1.2.3_arm64.deb"},
		{"amd64", AppImage, "Polyfin-Desktop-1.2.3-linux-x86_64.AppImage"},
		{"arm64", AppImage, "Polyfin-Desktop-1.2.3-linux-aarch64.AppImage"},
	} {
		if got := AssetName("1.2.3", c.goarch, c.kind); got != c.want {
			t.Errorf("AssetName(%s, %s) = %q, want %q", c.goarch, c.kind, got, c.want)
		}
	}
	env := func(name string) string {
		if name == "APPIMAGE" {
			return "/home/a/Polyfin.AppImage"
		}
		return ""
	}
	if KindOf("linux", env) != AppImage || KindOf("linux", func(string) string { return "" }) != Deb || KindOf("darwin", env) != DMG || KindOf("windows", env) != Installer {
		t.Error("KindOf")
	}
}

// The new AppImage, or bundle, takes the old one's place; the data folder
// is elsewhere and untouched.
func TestReplacingTheApp(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Polyfin.AppImage")
	pkg := filepath.Join(dir, "updates", "new.AppImage")
	if err := os.MkdirAll(filepath.Dir(pkg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkg, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(pkg, current); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(current); string(data) != "new" {
		t.Errorf("the AppImage holds %q", data)
	}
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Error("the download was kept")
	}

	bundle := filepath.Join(dir, "Polyfin.app")
	staged := bundle + ".new"
	for path, content := range map[string]string{filepath.Join(bundle, "Contents", "Info.plist"): "old", filepath.Join(staged, "Contents", "Info.plist"): "new"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := swap(staged, bundle); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist")); string(data) != "new" {
		t.Errorf("the bundle holds %q", data)
	}
	for _, gone := range []string{staged, bundle + ".old"} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s is left", gone)
		}
	}
	if err := CanInstall(DMG, "/Users/alex/dev/polyfin-desktop"); !errors.Is(err, ErrNotPackaged) {
		t.Errorf("CanInstall outside a bundle = %v", err)
	}
	if err := CanInstall(DMG, "/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop"); err != nil {
		t.Errorf("CanInstall in a bundle = %v", err)
	}
	if _, err := Install(context.Background(), pkg, DMG, filepath.Join(dir, "polyfin-desktop")); !errors.Is(err, ErrNotPackaged) {
		t.Errorf("Install outside a bundle = %v", err)
	}
}

// What the server's check found is read from its database.
func TestFoundReadsTheServersCheck(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	ctx := context.Background()
	enabled, latest, err := Found(ctx, pool)
	if err != nil || !enabled || latest != "" {
		t.Fatalf("before a check: %v %q %v", enabled, latest, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO update_check (latest_version, release_url) VALUES ('0.43.0', 'https://example.org')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settings SET update_check = false`); err != nil {
		t.Fatal(err)
	}
	enabled, latest, err = Found(ctx, pool)
	if err != nil || enabled || latest != "0.43.0" {
		t.Fatalf("after a check: %v %q %v", enabled, latest, err)
	}
}
