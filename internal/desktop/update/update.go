// Package update offers to install a new release of Polyfin Desktop. The
// server finds new releases as the admin app shows them (internal/updates:
// once a day, while Check for new versions is on, leaving out drafts, and
// pre-releases unless Include pre-releases is on, never for a development
// build); the app reads what it found from its own database, and offers
// the release's package for its system once the release's signed manifest
// (see Manifest) vouches for it. Installing downloads it, checks it
// against the checksum both GitHub and the manifest give, and replaces the
// app, keeping the previous one and a copy of the database until the new
// version answers (see Pending): the data folder stays as it is.
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/updates"
)

// API is the GitHub API releases are asked of. A build pointing at a fake
// sets it with -ldflags "-X github.com/moodiness/polyfin/internal/desktop/update.API=…".
var API = "https://api.github.com"

const (
	requestTimeout = 30 * time.Second
	maxAnswer      = 4 << 20
)

// ErrNoChecksum is a package GitHub gives no SHA-256 for: it is not
// installed.
var ErrNoChecksum = errors.New("the release gives no checksum for the package")

// ErrChecksum is a download that does not match the checksum GitHub gives.
var ErrChecksum = errors.New("the download does not match the release's checksum")

// Querier reads the database.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Found is what the server's check found.
type Found struct {
	// Enabled tells whether Check for new versions is on, Prereleases
	// whether Include pre-releases is.
	Enabled, Prereleases bool
	// Latest is the version to offer: the latest release, or the latest
	// pre-release while included and newer; "" before the first check.
	Latest string
}

// ReadFound reads what the server's check found.
func ReadFound(ctx context.Context, db Querier) (Found, error) {
	var found Found
	var stable, prerelease string
	err := db.QueryRow(ctx, `SELECT s.update_check, s.include_prereleases, coalesce(u.latest_version, ''), coalesce(u.prerelease_version, '')
		FROM settings s LEFT JOIN update_check u ON true`).Scan(&found.Enabled, &found.Prereleases, &stable, &prerelease)
	if errors.Is(err, pgx.ErrNoRows) {
		return Found{}, nil
	}
	found.Latest = updates.Latest(stable, prerelease, found.Prereleases)
	return found, err
}

// Kind is the package the app was installed from.
type Kind string

const (
	DMG       Kind = "dmg"
	Installer Kind = "exe"
	Deb       Kind = "deb"
	AppImage  Kind = "AppImage"
)

// KindOf is the package kind of the app on goos: on Linux, an AppImage
// when it runs from one, else the .deb.
func KindOf(goos string, env func(string) string) Kind {
	switch goos {
	case "darwin":
		return DMG
	case "windows":
		return Installer
	default:
		if env("APPIMAGE") != "" {
			return AppImage
		}
		return Deb
	}
}

// AssetName is the name of the release's package of version for goarch
// and kind, as the release workflow names them.
func AssetName(version, goarch string, kind Kind) string {
	switch kind {
	case DMG:
		return "Polyfin-Desktop-" + version + "-macos-" + arch(goarch, "x64", "arm64") + ".dmg"
	case Installer:
		return "Polyfin-Desktop-" + version + "-windows-" + arch(goarch, "x64", "arm64") + "-setup.exe"
	case Deb:
		return "polyfin-desktop_" + version + "_" + goarch + ".deb"
	default:
		return "Polyfin-Desktop-" + version + "-linux-" + arch(goarch, "x86_64", "aarch64") + ".AppImage"
	}
}

// arch is the name a package gives goarch: amd64's or arm64's.
func arch(goarch, amd64, arm64 string) string {
	switch goarch {
	case "amd64":
		return amd64
	case "arm64":
		return arm64
	}
	return goarch
}

// Offer is a release to install.
type Offer struct {
	Version string
	// Notes is the address of the release notes.
	Notes string
	Asset Asset
	// Expires is when the release's manifest expires: the package is not
	// installed after it.
	Expires time.Time
}

// Asset is the package of a release.
type Asset struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string
}

// Checker finds the package to offer.
type Checker struct {
	// Version is the app's own, the server's it runs.
	Version string
	GOARCH  string
	Kind    Kind
	// API is the GitHub API; empty is API.
	API    string
	Client *http.Client
	// Keys are those a manifest must be signed with; nil is ReleaseKeys.
	Keys []ed25519.PublicKey
	// Now is the time manifests expire against; nil is time.Now.
	Now func() time.Time
}

// Offer returns the release to offer when found.Latest is newer than the
// app and the setting is on; nil otherwise, for a development build, and
// for skip, the version the app went back from (see RolledBack). A
// release without a package for this system offers nothing either, nor a
// pre-release while Include pre-releases is off. The package must be
// listed by the release's manifest, signed by a key of the app and not
// expired, with the size and checksum GitHub gives: ErrNoManifest,
// ErrSignature, ErrExpired, ErrNoChecksum or ErrChecksum otherwise.
func (c *Checker) Offer(ctx context.Context, found Found, skip string) (*Offer, error) {
	latest := found.Latest
	if !found.Enabled || !updates.Newer(latest, c.Version) || latest == skip {
		return nil, nil
	}
	release, err := c.release(ctx, "v"+latest)
	if err != nil || release == nil {
		return nil, err
	}
	if release.Draft || (release.Prerelease || updates.Prerelease(latest)) && !found.Prereleases {
		return nil, nil
	}
	name := AssetName(latest, c.GOARCH, c.Kind)
	asset, ok := release.asset(name)
	if !ok {
		return nil, nil
	}
	sum, ok := strings.CutPrefix(asset.Digest, "sha256:")
	if !ok || len(sum) != sha256.Size*2 {
		return nil, fmt.Errorf("%w: %s", ErrNoChecksum, name)
	}
	if !web(asset.URL) {
		return nil, fmt.Errorf("the package %s has no address", name)
	}
	manifest, err := c.manifest(ctx, release, latest)
	if err != nil {
		return nil, err
	}
	listed, ok := manifest.File(name)
	if !ok {
		return nil, fmt.Errorf("%w: the release's manifest does not list %s", ErrNoChecksum, name)
	}
	if listed.Size != asset.Size || !strings.EqualFold(listed.SHA256, sum) {
		return nil, fmt.Errorf("%w: the release's manifest and GitHub give %s different checksums", ErrChecksum, name)
	}
	return &Offer{Version: latest, Notes: release.HTMLURL, Asset: Asset{Name: name, URL: asset.URL, Size: asset.Size, SHA256: sum},
		Expires: manifest.Expires}, nil
}

// Current refuses with ErrExpired an offer whose manifest expired since it
// was offered.
func (c *Checker) Current(offer Offer) error {
	if !c.now().Before(offer.Expires) {
		return fmt.Errorf("%w on %s", ErrExpired, offer.Expires.UTC().Format(time.RFC3339))
	}
	return nil
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// manifest downloads and checks the manifest of release, of version.
func (c *Checker) manifest(ctx context.Context, release *githubRelease, version string) (Manifest, error) {
	asset, ok := release.asset(ManifestName(version))
	if !ok || !web(asset.URL) {
		return Manifest{}, ErrNoManifest
	}
	keys := c.Keys
	if keys == nil {
		var err error
		if keys, err = ReleaseKeys(); err != nil {
			return Manifest{}, err
		}
	}
	data, err := c.get(ctx, asset.URL, MaxManifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("download the release's manifest: %w", err)
	}
	manifest, err := Verify(data, keys, c.now())
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Version != version {
		return Manifest{}, fmt.Errorf("%w: it is the manifest of %s", ErrSignature, manifest.Version)
	}
	return manifest, nil
}

// web tells whether address is an HTTP or HTTPS one.
func web(address string) bool {
	u, err := url.Parse(address)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http")
}

// get downloads address, up to limit bytes.
func (c *Checker) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Polyfin-Desktop/"+c.Version)
	response, err := c.client().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errors.New("the file is too large")
	}
	return data, err
}

type githubRelease struct {
	TagName    string         `json:"tag_name"`
	HTMLURL    string         `json:"html_url"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// asset is the release's file name.
func (r *githubRelease) asset(name string) (releaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func (c *Checker) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: requestTimeout}
}

// release asks GitHub for the release of tag: nil when there is none.
func (c *Checker) release(ctx context.Context, tag string) (*githubRelease, error) {
	api := c.API
	if api == "" {
		api = API
	}
	address := strings.TrimSuffix(api, "/") + "/repos/" + updates.Repository + "/releases/tags/" + url.PathEscape(tag)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Polyfin-Desktop/"+c.Version)
	response, err := c.client().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("GitHub answered with status %d", response.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(&release); err != nil {
		return nil, fmt.Errorf("read GitHub's answer: %w", err)
	}
	return &release, nil
}

// Download writes the offer's package into dir, checking its size and
// checksum: one that differs is removed and refused with ErrChecksum.
func Download(ctx context.Context, client *http.Client, offer Offer, dir string, progress func(done, total int64)) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, offer.Asset.URL, nil)
	if err != nil {
		return "", err
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", offer.Asset.Name, response.StatusCode)
	}
	file := filepath.Join(dir, filepath.Base(offer.Asset.Name))
	partial := file + ".part"
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	reader := io.LimitReader(response.Body, offer.Asset.Size+1)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
			if _, err = out.Write(buf[:n]); err != nil {
				break
			}
			done += int64(n)
			if progress != nil {
				progress(done, offer.Asset.Size)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				err = readErr
			}
			break
		}
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err == nil && (done != offer.Asset.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), offer.Asset.SHA256)) {
		err = fmt.Errorf("%w: %s", ErrChecksum, offer.Asset.Name)
	}
	if err != nil {
		os.Remove(partial)
		return "", err
	}
	return file, os.Rename(partial, file)
}
