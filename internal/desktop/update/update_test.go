package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"time"

	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// fakeGitHub answers the releases of tags with the packages of releases,
// and serves their files. Each release carries a manifest signed with key,
// unless told otherwise.
type fakeGitHub struct {
	*httptest.Server
	key      ed25519.PrivateKey
	releases map[string]map[string]any
	files    map[string][]byte
	asked    atomic.Int32
}

// now is the time of the tests' checks.
var now = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func newFakeGitHub(t *testing.T) *fakeGitHub {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGitHub{key: key, releases: map[string]map[string]any{}, files: map[string][]byte{}}
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
// checksum is digest, or the package's own when empty, and a manifest
// that lists the package as GitHub does, valid for a week.
func (f *fakeGitHub) publish(t *testing.T, version, name string, body []byte, digest string, extra map[string]any) {
	t.Helper()
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
			{"name": ManifestName(version), "browser_download_url": f.URL + "/download/" + ManifestName(version), "size": 1, "digest": digest},
		},
	}
	for k, v := range extra {
		release[k] = v
	}
	f.releases["v"+version] = release
	listed, _ := strings.CutPrefix(digest, "sha256:")
	f.sign(t, Manifest{Version: version, Issued: now.Add(-time.Hour), Expires: now.Add(7 * 24 * time.Hour),
		Files: []File{{Name: name, Size: int64(len(body)), SHA256: listed}}}, f.key)
}

// sign puts manifest, signed with key, in place of the release's.
func (f *fakeGitHub) sign(t *testing.T, manifest Manifest, key ed25519.PrivateKey) {
	t.Helper()
	data, err := Sign(manifest, key)
	if err != nil {
		t.Fatal(err)
	}
	f.files["/download/"+ManifestName(manifest.Version)] = data
}

func checker(f *fakeGitHub, version string) *Checker {
	return &Checker{Version: version, GOARCH: "arm64", Kind: DMG, API: f.URL, Client: f.Client(),
		Keys: []ed25519.PublicKey{f.key.Public().(ed25519.PublicKey)}, Now: func() time.Time { return now }}
}

// on is what the server found with Check for new versions on.
func on(latest string) Found { return Found{Enabled: true, Latest: latest} }

// A newer release the server found offers its package for this system,
// which downloads checked against GitHub's checksum and the manifest's.
func TestOfferAndDownload(t *testing.T) {
	f := newFakeGitHub(t)
	body := []byte("a new disk image")
	f.publish(t, "0.43.0", "Polyfin-Desktop-0.43.0-macos-arm64.dmg", body, "", nil)
	ctx := context.Background()
	offer, err := checker(f, "0.42.0").Offer(ctx, on("0.43.0"), "")
	if err != nil || offer == nil {
		t.Fatalf("Offer = %+v, %v", offer, err)
	}
	if offer.Version != "0.43.0" || offer.Asset.Name != "Polyfin-Desktop-0.43.0-macos-arm64.dmg" || offer.Notes == "" ||
		!offer.Expires.Equal(now.Add(7*24*time.Hour)) {
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
// newer, for a development build, the version the app went back from, a
// pre-release while they are left out, or a release without a package
// for this system; GitHub is not asked for the first four.
func TestNothingOffered(t *testing.T) {
	f := newFakeGitHub(t)
	f.publish(t, "0.43.0", "Polyfin-Desktop-0.43.0-windows-x64-setup.exe", []byte("x"), "", nil)
	f.publish(t, "0.44.0", "Polyfin-Desktop-0.44.0-macos-arm64.dmg", []byte("x"), "", map[string]any{"prerelease": true})
	ctx := context.Background()
	for _, c := range []struct {
		name, version, latest, skip string
		enabled                     bool
	}{
		{"setting off", "0.42.0", "0.43.0", "", false},
		{"same version", "0.43.0", "0.43.0", "", true},
		{"older", "0.43.1", "0.43.0", "", true},
		{"development build", "dev", "0.43.0", "", true},
		{"nothing found yet", "0.42.0", "", "", true},
		{"gone back from", "0.42.0", "0.43.0", "0.43.0", true},
	} {
		found := Found{Enabled: c.enabled, Latest: c.latest}
		if offer, err := checker(f, c.version).Offer(ctx, found, c.skip); err != nil || offer != nil {
			t.Errorf("%s: %+v, %v", c.name, offer, err)
		}
	}
	if f.asked.Load() != 0 {
		t.Errorf("GitHub was asked %d times", f.asked.Load())
	}
	// macOS on Apple silicon: the release has only Windows' package.
	if offer, err := checker(f, "0.42.0").Offer(ctx, on("0.43.0"), ""); err != nil || offer != nil {
		t.Errorf("another system's package: %+v, %v", offer, err)
	}
	if offer, err := checker(f, "0.42.0").Offer(ctx, on("0.45.0"), ""); err != nil || offer != nil {
		t.Errorf("a release GitHub does not have: %+v, %v", offer, err)
	}
}

// A pre-release, as GitHub marks it or as its version reads, is offered
// only while Include pre-releases is on.
func TestPreReleasesOnlyWithTheSwitch(t *testing.T) {
	f := newFakeGitHub(t)
	f.publish(t, "0.44.0", "Polyfin-Desktop-0.44.0-macos-arm64.dmg", []byte("x"), "", map[string]any{"prerelease": true})
	f.publish(t, "0.45.0-rc.1", "Polyfin-Desktop-0.45.0-rc.1-macos-arm64.dmg", []byte("y"), "", nil)
	ctx := context.Background()
	for _, latest := range []string{"0.44.0", "0.45.0-rc.1"} {
		if offer, err := checker(f, "0.42.0").Offer(ctx, Found{Enabled: true, Latest: latest}, ""); err != nil || offer != nil {
			t.Errorf("%s with pre-releases left out: %+v, %v", latest, offer, err)
		}
		offer, err := checker(f, "0.42.0").Offer(ctx, Found{Enabled: true, Prereleases: true, Latest: latest}, "")
		if err != nil || offer == nil || offer.Version != latest {
			t.Errorf("%s with pre-releases included: %+v, %v", latest, offer, err)
		}
	}
}

// A release whose manifest was changed, is signed by another key, has
// expired, belongs to another version, or lists another checksum than
// GitHub's, or that has no manifest, is refused, and offers nothing.
func TestManifestsAreChecked(t *testing.T) {
	f := newFakeGitHub(t)
	name := "Polyfin-Desktop-0.43.0-macos-arm64.dmg"
	body := []byte("a new disk image")
	sum := sha256.Sum256(body)
	valid := Manifest{Version: "0.43.0", Issued: now.Add(-time.Hour), Expires: now.Add(time.Hour),
		Files: []File{{Name: name, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}}}
	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, c := range []struct {
		name   string
		change func()
		want   error
	}{
		{"tampered", func() {
			// The same manifest, but for one byte of the checksum it
			// lists: as if the package were swapped, and the
			// manifest edited to match it.
			data, err := Sign(valid, f.key)
			if err != nil {
				t.Fatal(err)
			}
			listed := valid.Files[0].SHA256
			changed := strings.Replace(string(data), listed, "0"+listed[1:], 1)
			if listed[0] == '0' {
				changed = strings.Replace(string(data), listed, "1"+listed[1:], 1)
			}
			f.files["/download/"+ManifestName("0.43.0")] = []byte(changed)
		}, ErrSignature},
		{"wrong key", func() { f.sign(t, valid, otherKey) }, ErrSignature},
		{"expired", func() {
			expired := valid
			expired.Issued, expired.Expires = now.Add(-31*24*time.Hour), now.Add(-time.Minute)
			f.sign(t, expired, f.key)
		}, ErrExpired},
		{"another version's", func() {
			other := valid
			other.Version = "0.42.1"
			data, err := Sign(other, f.key)
			if err != nil {
				t.Fatal(err)
			}
			f.files["/download/"+ManifestName("0.43.0")] = data
		}, ErrSignature},
		{"hash mismatch", func() {
			mismatch := valid
			mismatch.Files = []File{{Name: name, Size: int64(len(body)), SHA256: strings.Repeat("ab", 32)}}
			f.sign(t, mismatch, f.key)
		}, ErrChecksum},
		{"size mismatch", func() {
			mismatch := valid
			mismatch.Files = []File{{Name: name, Size: 1, SHA256: valid.Files[0].SHA256}}
			f.sign(t, mismatch, f.key)
		}, ErrChecksum},
		{"package not listed", func() {
			unlisted := valid
			unlisted.Files = []File{{Name: "Polyfin-Desktop-0.43.0-macos-x64.dmg", Size: 1, SHA256: valid.Files[0].SHA256}}
			f.sign(t, unlisted, f.key)
		}, ErrNoChecksum},
	} {
		f.publish(t, "0.43.0", name, body, "", nil)
		f.sign(t, valid, f.key)
		if offer, err := checker(f, "0.42.0").Offer(ctx, on("0.43.0"), ""); err != nil || offer == nil {
			t.Fatalf("%s: the valid manifest is refused: %+v, %v", c.name, offer, err)
		}
		c.change()
		offer, err := checker(f, "0.42.0").Offer(ctx, on("0.43.0"), "")
		if offer != nil || !errors.Is(err, c.want) {
			t.Errorf("%s: Offer = %+v, %v; want %v", c.name, offer, err, c.want)
		}
	}
	// A release that does not even list a manifest.
	f.publish(t, "0.43.0", name, body, "", nil)
	release := f.releases["v0.43.0"]
	release["assets"] = release["assets"].([]map[string]any)[:2]
	if offer, err := checker(f, "0.42.0").Offer(ctx, on("0.43.0"), ""); offer != nil || !errors.Is(err, ErrNoManifest) {
		t.Errorf("without a manifest: Offer = %+v, %v", offer, err)
	}
	// A build carrying no key accepts none.
	f.publish(t, "0.43.0", name, body, "", nil)
	keyless := checker(f, "0.42.0")
	keyless.Keys = []ed25519.PublicKey{}
	if offer, err := keyless.Offer(ctx, on("0.43.0"), ""); offer != nil || !errors.Is(err, ErrSignature) {
		t.Errorf("without keys: Offer = %+v, %v", offer, err)
	}
}

// A manifest stays valid indented, and either of two keys may sign it.
func TestManifestKeys(t *testing.T) {
	_, first, _ := ed25519.GenerateKey(rand.Reader)
	_, second, _ := ed25519.GenerateKey(rand.Reader)
	keys := []ed25519.PublicKey{first.Public().(ed25519.PublicKey), second.Public().(ed25519.PublicKey)}
	m := Manifest{Version: "1.0.0", Issued: now, Expires: now.Add(time.Hour), Files: []File{{Name: "a", Size: 1, SHA256: "ab"}}}
	for _, key := range []ed25519.PrivateKey{first, second} {
		data, err := Sign(m, key)
		if err != nil {
			t.Fatal(err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, data); err != nil {
			t.Fatal(err)
		}
		for _, form := range [][]byte{data, compact.Bytes()} {
			if got, err := Verify(form, keys, now); err != nil || got.Version != "1.0.0" {
				t.Errorf("Verify = %+v, %v", got, err)
			}
		}
	}
	text := EncodeKey(keys[0]) + ", " + EncodeKey(keys[1])
	if parsed, err := ParseKeys(text); err != nil || len(parsed) != 2 || !parsed[1].Equal(keys[1]) {
		t.Errorf("ParseKeys = %v, %v", parsed, err)
	}
	if _, err := ParseKeys("not a key"); err == nil {
		t.Error("ParseKeys accepted a word")
	}
}

// A package without a checksum is not offered; one that does not match
// it is refused, and nothing is left. An offer whose manifest expired
// since is refused.
func TestChecksumsAreRequired(t *testing.T) {
	f := newFakeGitHub(t)
	f.publish(t, "0.43.0", "Polyfin-Desktop-0.43.0-macos-arm64.dmg", []byte("x"), "none", nil)
	f.publish(t, "0.44.0", "Polyfin-Desktop-0.44.0-macos-arm64.dmg", []byte("tampered"), "sha256:"+strings.Repeat("ab", 32), nil)
	ctx := context.Background()
	if _, err := checker(f, "0.42.0").Offer(ctx, on("0.43.0"), ""); !errors.Is(err, ErrNoChecksum) {
		t.Errorf("Offer without a checksum = %v", err)
	}
	c := checker(f, "0.42.0")
	offer, err := c.Offer(ctx, on("0.44.0"), "")
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
	if err := c.Current(*offer); err != nil {
		t.Errorf("Current = %v", err)
	}
	c.Now = func() time.Time { return offer.Expires }
	if err := c.Current(*offer); !errors.Is(err, ErrExpired) {
		t.Errorf("Current once expired = %v", err)
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

// The new AppImage, or bundle, takes the old one's place, which is kept
// beside it; the data folder is elsewhere and untouched.
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
	if data, _ := os.ReadFile(current + ".old"); string(data) != "old" {
		t.Errorf("the previous AppImage holds %q", data)
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
	if data, _ := os.ReadFile(filepath.Join(bundle+".old", "Contents", "Info.plist")); string(data) != "old" {
		t.Errorf("the previous bundle holds %q", data)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Errorf("%s is left", staged)
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

// What the server's check found is read from its database: the latest
// pre-release only while Include pre-releases is on and it is newer.
func TestFoundReadsTheServersCheck(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	ctx := context.Background()
	found, err := ReadFound(ctx, pool)
	if err != nil || found != (Found{Enabled: true}) {
		t.Fatalf("before a check: %+v %v", found, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO update_check (latest_version, release_url, prerelease_version) VALUES ('0.43.0', 'https://example.org', '0.44.0-rc.1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settings SET update_check = false`); err != nil {
		t.Fatal(err)
	}
	found, err = ReadFound(ctx, pool)
	if err != nil || found != (Found{Latest: "0.43.0"}) {
		t.Fatalf("after a check: %+v %v", found, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settings SET update_check = true, include_prereleases = true`); err != nil {
		t.Fatal(err)
	}
	found, err = ReadFound(ctx, pool)
	if err != nil || found != (Found{Enabled: true, Prereleases: true, Latest: "0.44.0-rc.1"}) {
		t.Fatalf("with pre-releases: %+v %v", found, err)
	}
}
