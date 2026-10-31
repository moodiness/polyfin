// Package updates tells administrators when a new version of Polyfin is
// out. Once a day, and shortly after startup, while the settings' Check
// for new versions is on, it asks GitHub for the latest release of
// Polyfin, which leaves out pre-releases and drafts, and, while Include
// pre-releases is on too, for its latest pre-release. It compares the
// newest of them it may offer with the running version: a newer one shows
// in the admin app and in Jellyfin's system information, and is told once
// to administrators' notifications. A development build, whose version is
// "dev", asks nothing.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// API is the GitHub API the latest release is asked of. It is a variable
// so that a build pointing at a fake release endpoint can set it with
// -ldflags "-X github.com/moodiness/polyfin/internal/updates.API=…".
var API = "https://api.github.com"

// Repository is Polyfin's repository on GitHub.
const Repository = "moodiness/polyfin"

const (
	// requestTimeout bounds the request to GitHub.
	requestTimeout = 10 * time.Second
	// maxAnswer bounds the bytes read from GitHub's answer: a page of
	// releases, with their notes and files, holds up to a few hundred
	// kilobytes.
	maxAnswer = 4 << 20
)

// timing is when the checks run. Tests shorten it.
type timing struct {
	// first is how long after start the first check runs; every how
	// often a check that went through runs again, and retry how soon one
	// that failed, or found the setting off, looks again.
	first, every, retry time.Duration
}

var defaultTiming = timing{first: time.Minute, every: 24 * time.Hour, retry: time.Hour}

// Release is a version of Polyfin published on GitHub: Version, as its tag
// names it without the v, and URL, the address of its release notes.
type Release struct {
	Version string
	URL     string
}

// Options are the dependencies of a Checker.
type Options struct {
	DB *pgxpool.Pool
	// Version is the running version, set at build time.
	Version string
	// Enabled tells whether the settings' Check for new versions is on.
	Enabled func() bool
	// Prereleases tells whether the settings' Include pre-releases is on;
	// nil is never.
	Prereleases func() bool
	// Notify tells administrators about a new release, once per version;
	// nil tells no one.
	Notify func(Release)
	// API is the GitHub API asked; empty is API, the real one.
	API    string
	Logger *slog.Logger
}

// Checker asks GitHub for the latest release, and keeps it.
type Checker struct {
	db          *pgxpool.Pool
	version     string
	enabled     func() bool
	prereleases func() bool
	notify      func(Release)
	api         string
	client      *http.Client
	logger      *slog.Logger
	now         func() time.Time
	timing      timing
	// wake asks Run for a check at once.
	wake chan struct{}

	// checkMu serializes the checks.
	checkMu sync.Mutex
	mu      sync.Mutex
	// latest is the latest release found, prerelease the latest
	// pre-release newer than it, found while pre-releases were included,
	// and notified the version administrators were last told about.
	latest     Release
	prerelease Release
	notified   string
	loaded     bool
}

// New returns a checker. Run loads what an earlier run found and checks
// until its context ends.
func New(options Options) *Checker {
	api := options.API
	if api == "" {
		api = API
	}
	// The default transport goes through the proxy the environment sets.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	prereleases := options.Prereleases
	if prereleases == nil {
		prereleases = func() bool { return false }
	}
	return &Checker{db: options.DB, version: options.Version, enabled: options.Enabled, prereleases: prereleases, notify: options.Notify,
		api: strings.TrimSuffix(api, "/"), client: &http.Client{Transport: transport, Timeout: requestTimeout}, logger: options.Logger,
		now: time.Now, timing: defaultTiming, wake: make(chan struct{}, 1)}
}

// Wake has Run check at once, as when Include pre-releases is turned on.
func (c *Checker) Wake() {
	if c == nil {
		return
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Run reads what an earlier run found, then checks shortly after, and once
// a day after that, until ctx ends. While the setting is off nothing is
// asked: it is looked at again every hour, so that a check runs soon
// after it is turned on. Wake has it check at once.
func (c *Checker) Run(ctx context.Context) {
	c.load(ctx)
	timer := time.NewTimer(c.timing.first)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-c.wake:
		}
		wait := c.timing.every
		if !c.enabled() {
			wait = c.timing.retry
		} else if err := c.Check(ctx); err != nil {
			wait = c.timing.retry
			if ctx.Err() == nil {
				c.logger.Debug("The check for a new version of Polyfin failed", "error", err)
			}
		}
		timer.Reset(wait)
	}
}

// Available returns the latest release, or the latest pre-release while
// Include pre-releases is on and it is newer, when it is newer than the
// running version and the setting is on; false otherwise, and always for
// a development build.
func (c *Checker) Available() (Release, bool) {
	if c == nil || !c.enabled() {
		return Release{}, false
	}
	c.mu.Lock()
	latest := pick(c.latest, c.prerelease, c.prereleases())
	c.mu.Unlock()
	if !newer(latest.Version, c.version) {
		return Release{}, false
	}
	return latest, true
}

// Check asks GitHub for the latest release, and the latest pre-release
// while they are included, keeps them, and tells administrators about the
// one Available gives when it is newer than the running version and they
// were not told yet. It asks nothing while the setting is off, nor for a
// development build. A failed request and a refusal (such as GitHub's 403
// or 429 when too many requests were made) change nothing; nor does a
// pre-release while they are left out.
func (c *Checker) Check(ctx context.Context) error {
	if !c.enabled() {
		return nil
	}
	if _, ok := parseVersion(c.version); !ok {
		return nil
	}
	c.checkMu.Lock()
	defer c.checkMu.Unlock()
	c.load(ctx)
	include := c.prereleases()
	var stable, prerelease *Release
	var err error
	if include {
		stable, prerelease, err = c.fetchAll(ctx)
	} else {
		stable, err = c.fetchLatest(ctx)
	}
	if err != nil || stable == nil && prerelease == nil {
		return err
	}
	c.mu.Lock()
	if stable != nil {
		c.latest = *stable
	}
	c.prerelease = Release{}
	if prerelease != nil {
		c.prerelease = *prerelease
	}
	found := pick(c.latest, c.prerelease, include)
	tell := newer(found.Version, c.version) && c.notified != found.Version
	if tell {
		c.notified = found.Version
	}
	latest, notified := c.latest, c.notified
	c.mu.Unlock()
	_, err = c.db.Exec(ctx, `INSERT INTO update_check (latest_version, release_url, prerelease_version, prerelease_url, notified_version, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (singleton) DO UPDATE SET latest_version = excluded.latest_version, release_url = excluded.release_url,
			prerelease_version = excluded.prerelease_version, prerelease_url = excluded.prerelease_url,
			notified_version = excluded.notified_version, checked_at = excluded.checked_at`,
		latest.Version, latest.URL, c.prerelease.Version, c.prerelease.URL, notified, c.now())
	if err != nil && ctx.Err() == nil {
		c.logger.Warn("The latest version of Polyfin found could not be kept", "error", err)
	}
	if tell {
		c.logger.Info("A new version of Polyfin is available", "version", found.Version, "running", c.version, "release", found.URL)
		if c.notify != nil {
			c.notify(found)
		}
	}
	return nil
}

// pick is prerelease when included and newer than stable, stable
// otherwise.
func pick(stable, prerelease Release, include bool) Release {
	if prerelease.Version != "" && Latest(stable.Version, prerelease.Version, include) == prerelease.Version {
		return prerelease
	}
	return stable
}

// Latest is the version to offer of the latest release, stable, and the
// latest pre-release, prerelease, either "" when none was found: the
// pre-release when include and it is newer, the release otherwise.
func Latest(stable, prerelease string, include bool) string {
	if _, ok := parseVersion(prerelease); include && ok && (stable == "" || newer(prerelease, stable)) {
		return prerelease
	}
	return stable
}

// Prerelease tells whether version is a pre-release's, such as 1.4.0-rc.1.
func Prerelease(version string) bool {
	v, ok := parseVersion(version)
	return ok && len(v.pre) > 0
}

// load reads what an earlier run found, once.
func (c *Checker) load(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	var latest, prerelease Release
	var notified string
	err := c.db.QueryRow(ctx, "SELECT latest_version, release_url, prerelease_version, prerelease_url, notified_version FROM update_check").
		Scan(&latest.Version, &latest.URL, &prerelease.Version, &prerelease.URL, &notified)
	switch {
	case err == nil:
		c.latest, c.prerelease, c.notified = latest, prerelease, notified
	case !errors.Is(err, pgx.ErrNoRows):
		if ctx.Err() == nil {
			c.logger.Warn("The latest version of Polyfin found earlier could not be read", "error", err)
		}
		return
	}
	c.loaded = true
}

// githubRelease is what GitHub answers about a release.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// fetchLatest asks GitHub for the latest release: nil when there is none
// yet, or when it is a draft or a pre-release.
func (c *Checker) fetchLatest(ctx context.Context) (*Release, error) {
	var release githubRelease
	if found, err := c.ask(ctx, "/releases/latest", &release); err != nil || !found {
		return nil, err
	}
	version, ok := parseVersion(release.TagName)
	if !ok {
		return nil, fmt.Errorf("the latest release's tag %q is not a version", release.TagName)
	}
	if release.Draft || release.Prerelease || len(version.pre) > 0 {
		return nil, nil
	}
	return &Release{Version: strings.TrimPrefix(release.TagName, "v"), URL: releaseURL(release)}, nil
}

// recentReleases is how many of the latest releases fetchAll looks at.
const recentReleases = 20

// fetchAll asks GitHub for the latest releases, and returns the newest
// release and the newest pre-release, nil when there is none, or when the
// pre-release is older than the release. Drafts, and tags that are no
// version, are left out. A release is a pre-release when GitHub marks it
// so, or its version is a pre-release's.
func (c *Checker) fetchAll(ctx context.Context) (stable, prerelease *Release, err error) {
	var releases []githubRelease
	if found, err := c.ask(ctx, "/releases?per_page="+strconv.Itoa(recentReleases), &releases); err != nil || !found {
		return nil, nil, err
	}
	var best, bestPre semver
	for _, release := range releases {
		version, ok := parseVersion(release.TagName)
		if release.Draft || !ok {
			continue
		}
		found := &Release{Version: strings.TrimPrefix(release.TagName, "v"), URL: releaseURL(release)}
		switch {
		case release.Prerelease || len(version.pre) > 0:
			if prerelease == nil || compare(version, bestPre) > 0 {
				prerelease, bestPre = found, version
			}
		case stable == nil || compare(version, best) > 0:
			stable, best = found, version
		}
	}
	if prerelease != nil && stable != nil && compare(bestPre, best) < 0 {
		prerelease = nil
	}
	return stable, prerelease, nil
}

// ask asks GitHub for path of Polyfin's repository, decoding the answer
// into answer: false when GitHub has nothing there.
func (c *Checker) ask(ctx context.Context, path string, answer any) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api+"/repos/"+Repository+path, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Polyfin/"+c.version)
	response, err := c.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		// No release yet.
		return false, nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		return false, fmt.Errorf("GitHub asked to wait (status %d)", response.StatusCode)
	default:
		return false, fmt.Errorf("GitHub answered with status %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(answer); err != nil {
		return false, fmt.Errorf("read GitHub's answer: %w", err)
	}
	return true, nil
}

// releaseURL is the address of release's notes on GitHub: the one GitHub
// gives when it is an https address, the release's page otherwise.
func releaseURL(release githubRelease) string {
	if u, err := url.Parse(release.HTMLURL); err == nil && u.Scheme == "https" && u.Host != "" {
		return release.HTMLURL
	}
	return "https://github.com/" + Repository + "/releases/tag/" + url.PathEscape(release.TagName)
}
