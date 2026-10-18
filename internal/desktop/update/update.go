// Package update offers to install a new release of Polyfin Desktop. The
// server finds new releases as the admin app shows them (internal/updates:
// once a day, while Check for new versions is on, leaving out drafts and
// pre-releases, never for a development build); the app reads what it
// found from its own database, and offers the release's package for its
// system. Installing downloads it, checks it against the checksum GitHub
// gives, and replaces the app: the data folder stays as it is.
package update

import (
	"context"
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

// Found reads what the server's check found: whether Check for new
// versions is on, and the latest version it found, "" before its first
// check.
func Found(ctx context.Context, db Querier) (enabled bool, latest string, err error) {
	err = db.QueryRow(ctx, `SELECT s.update_check, coalesce(u.latest_version, '')
		FROM settings s LEFT JOIN update_check u ON true`).Scan(&enabled, &latest)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	return enabled, latest, err
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
}

// Offer returns the release to offer when latest is newer than the app
// and the setting is on; nil otherwise, and for a development build. A
// release without a package for this system offers nothing either.
func (c *Checker) Offer(ctx context.Context, enabled bool, latest string) (*Offer, error) {
	if !enabled || !updates.Newer(latest, c.Version) {
		return nil, nil
	}
	release, err := c.release(ctx, "v"+latest)
	if err != nil || release == nil {
		return nil, err
	}
	if release.Draft || release.Prerelease {
		return nil, nil
	}
	name := AssetName(latest, c.GOARCH, c.Kind)
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		sum, ok := strings.CutPrefix(asset.Digest, "sha256:")
		if !ok || len(sum) != sha256.Size*2 {
			return nil, fmt.Errorf("%w: %s", ErrNoChecksum, name)
		}
		if u, err := url.Parse(asset.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, fmt.Errorf("the package %s has no address", name)
		}
		return &Offer{Version: latest, Notes: release.HTMLURL, Asset: Asset{Name: name, URL: asset.URL, Size: asset.Size, SHA256: sum}}, nil
	}
	return nil, nil
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	} `json:"assets"`
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
