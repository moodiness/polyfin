package admin

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// HealthSources are what the health page reads, each optional: nothing
// it reads makes a request outside the server.
type HealthSources struct {
	// Addons tells how the requests to the addons went (stremio.Client).
	Addons *stremio.Client
	// Cache keeps the sources being played, CacheDir holds it.
	Cache    *source.Cache
	CacheDir string
	// Encoder runs the remuxes and conversions.
	Encoder *hls.Manager
	// Thumbnails makes the scrubbing thumbnails and chapter images.
	Thumbnails *thumbnails.Service
	// DatabaseSize measures the database.
	DatabaseSize func(context.Context) (int64, error)
	// Started is when the server started.
	Started time.Time
	// SecretKey tells whether POLYFIN_SECRET_KEY is set, and Secrets reads
	// how the stored secrets stand with it.
	SecretKey bool
	Secrets   func(context.Context) (secrets.Report, error)
}

type healthJSON struct {
	CheckedAt  time.Time            `json:"checkedAt"`
	Process    processJSON          `json:"process"`
	Database   databaseJSON         `json:"database"`
	Cache      *cacheJSON           `json:"cache"`
	Disks      []diskJSON           `json:"disks"`
	Transcoder *transcoderJSON      `json:"transcoder"`
	Thumbnails *thumbnailHealthJSON `json:"thumbnails"`
	Addons     []addonHealthJSON    `json:"addons"`
	// Secrets is how the stored keys and tokens stand, null when unknown.
	Secrets *secretsHealthJSON `json:"secrets"`
}

type processJSON struct {
	Version   string    `json:"version"`
	GoVersion string    `json:"goVersion"`
	StartedAt time.Time `json:"startedAt"`
	// Memory is the bytes the process holds from the system, Heap those of
	// live objects.
	Memory     uint64 `json:"memory"`
	Heap       uint64 `json:"heap"`
	Goroutines int    `json:"goroutines"`
}

type databaseJSON struct {
	Reachable bool `json:"reachable"`
	// Size is in bytes, null when it could not be read.
	Size *int64 `json:"size"`
}

type cacheJSON struct {
	Used    int64 `json:"used"`
	Limit   int64 `json:"limit"`
	Sources int   `json:"sources"`
}

// diskJSON is the disk one of the server's folders is on: Folder is
// "cache" or "recordings".
type diskJSON struct {
	Folder string `json:"folder"`
	Path   string `json:"path"`
	// Free and Used are in bytes, -1 when the disk cannot be measured.
	Free  int64  `json:"free"`
	Used  int64  `json:"used"`
	Mount string `json:"mount"`
}

type transcoderJSON struct {
	// Hardware is the GPU video is converted on, null for the CPU.
	Hardware *hardwareJSON `json:"hardware"`
	// Encoders are the video and audio encoders Polyfin uses that FFmpeg
	// has, on the CPU.
	Encoders []string `json:"encoders"`
	// Conversions counts the playbacks whose video is converted; Limit is
	// the settings' cap, 0 for none.
	Conversions int `json:"conversions"`
	Limit       int `json:"limit"`
	// Remuxes counts the encodings copying the video.
	Remuxes int `json:"remuxes"`
	// MaxHeight is the height converted video is scaled down to, 0 for
	// the original's.
	MaxHeight int  `json:"maxHeight"`
	Enabled   bool `json:"enabled"`
}

type hardwareJSON struct {
	Method      string   `json:"method"`
	Device      string   `json:"device"`
	Encoders    []string `json:"encoders"`
	ToneMapping bool     `json:"toneMapping"`
	// QVBR tells whether a VAAPI GPU takes a quality factor.
	QVBR bool `json:"qvbr"`
}

func newHardwareJSON(hw *hls.Hardware) *hardwareJSON {
	return &hardwareJSON{Method: hw.Method, Device: hw.Device, Encoders: hw.Encoders, ToneMapping: hw.ToneMapping, QVBR: hw.QVBR}
}

type thumbnailHealthJSON struct {
	Enabled     bool             `json:"enabled"`
	Waiting     int              `json:"waiting"`
	QueueLength int              `json:"queueLength"`
	Working     bool             `json:"working"`
	PausedHosts []pausedHostJSON `json:"pausedHosts"`
}

type pausedHostJSON struct {
	Host  string    `json:"host"`
	Until time.Time `json:"until"`
}

// addonHealthJSON is how the requests made to an addon for apps went since
// the server started; Requests is 0 when none was. Owner is the user whose
// own addon it is, nil for the server's.
type addonHealthJSON struct {
	ID            string     `json:"id"`
	Owner         *ownerJSON `json:"owner"`
	Name          string     `json:"name"`
	Enabled       bool       `json:"enabled"`
	RefreshedAt   time.Time  `json:"refreshedAt"`
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
	LastFailureAt *time.Time `json:"lastFailureAt"`
	// Failure is the code of the last failure, empty after a success.
	Failure string `json:"failure"`
	// ResponseTime is in milliseconds, of the last answer.
	ResponseTime int64 `json:"responseTime"`
	Requests     int   `json:"requests"`
	Failures     int   `json:"failures"`
}

// softwareEncoders are the CPU encoders conversions use.
var softwareEncoders = []string{"libx264", "libx265", "aac", "libfdk_aac", "ac3", "eac3", "libopus", "flac"}

// health describes how the server's parts fare, from what they record:
// nothing is requested outside the server.
func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	result := healthJSON{CheckedAt: h.now(), Disks: []diskJSON{}, Addons: []addonHealthJSON{}}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result.Process = processJSON{Version: h.Version, GoVersion: runtime.Version(), StartedAt: h.Health.Started,
		Memory: memory.Sys, Heap: memory.HeapAlloc, Goroutines: runtime.NumGoroutine()}

	ctx, cancel := context.WithTimeout(r.Context(), readyWait)
	defer cancel()
	result.Database.Reachable = h.Database.Ping(ctx) == nil
	if result.Database.Reachable && h.Health.DatabaseSize != nil {
		if size, err := h.Health.DatabaseSize(ctx); err == nil {
			result.Database.Size = &size
		}
	}

	if h.Health.Cache != nil {
		used, sources, limit := h.Health.Cache.Usage()
		result.Cache = &cacheJSON{Used: used, Limit: limit, Sources: sources}
	}
	for _, folder := range []struct{ name, path string }{{"cache", h.Health.CacheDir}, {"recordings", h.RecordingsDir}} {
		if folder.path == "" {
			continue
		}
		disk := diskJSON{Folder: folder.name, Path: filepath.Clean(folder.path), Free: -1, Used: -1}
		if free, used, mount, ok := diskspace.Measure(folder.path); ok {
			disk.Free, disk.Used, disk.Mount = free, used, mount
		}
		result.Disks = append(result.Disks, disk)
	}

	settings := h.Accounts.Settings()
	if encoder := h.Health.Encoder; encoder != nil {
		t := &transcoderJSON{Encoders: []string{}, MaxHeight: settings.MaxConversionHeight, Enabled: settings.Transcoding}
		if hw := encoder.Hardware(); hw != nil {
			t.Hardware = newHardwareJSON(hw)
		}
		for _, name := range softwareEncoders {
			if slices.Contains(encoder.Encoders(), name) {
				t.Encoders = append(t.Encoders, name)
			}
		}
		t.Conversions, t.Limit = encoder.Conversions()
		for _, running := range encoder.Running() {
			if !running.Key.Converts {
				t.Remuxes++
			}
		}
		result.Transcoder = t
	}

	if h.Health.Thumbnails != nil {
		status := h.Health.Thumbnails.Status()
		th := &thumbnailHealthJSON{Enabled: settings.Trickplay || settings.ChapterImages, Waiting: status.Waiting,
			QueueLength: status.QueueLength, Working: status.Working, PausedHosts: []pausedHostJSON{}}
		for _, paused := range status.Paused {
			th.PausedHosts = append(th.PausedHosts, pausedHostJSON{Host: paused.Host, Until: paused.Until})
		}
		result.Thumbnails = th
	}

	// The server's addons come first, then each user's own, by name.
	scopes, err := h.ownedScopes(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, scope := range scopes {
		list, err := h.Addons.Addons(r.Context(), scope.Scope)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, addon := range list {
			// Eclipse addons' requests count in their health too.
			if !addon.Stremio() && !addon.Eclipse() {
				continue
			}
			result.Addons = append(result.Addons, h.addonHealth(addon, scope.Owner))
		}
	}
	result.Secrets = h.secretsHealth(r.Context())
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) addonHealth(addon addons.Addon, owner *ownerJSON) addonHealthJSON {
	result := addonHealthJSON{ID: addon.ID.String(), Owner: owner, Name: addon.Manifest.Name, Enabled: addon.Enabled,
		RefreshedAt: addon.RefreshedAt}
	if h.Health.Addons == nil {
		return result
	}
	health, ok := h.Health.Addons.Health(addon.ManifestURL)
	if !ok {
		return result
	}
	if !health.LastSuccess.IsZero() {
		result.LastSuccessAt = new(health.LastSuccess)
	}
	if !health.LastFailure.IsZero() {
		result.LastFailureAt = new(health.LastFailure)
	}
	result.Failure, result.ResponseTime = health.Failure, health.ResponseTime.Milliseconds()
	result.Requests, result.Failures = health.Requests, health.Failures
	return result
}

// checkInterval is how often an addon may be checked by hand: one request
// for its manifest, never more.
const checkInterval = time.Minute

// addonChecks remembers when each addon was last checked by hand.
type addonChecks struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// allow reports whether id may be checked at now, and records it, or how
// long until it may.
func (c *addonChecks) allow(id string, now time.Time) (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		c.last = map[string]time.Time{}
	}
	for key, at := range c.last {
		if now.Sub(at) >= checkInterval {
			delete(c.last, key)
		}
	}
	if at, ok := c.last[id]; ok {
		return false, checkInterval - now.Sub(at)
	}
	c.last[id] = now
	return true, 0
}

// checkAddon asks an addon, the server's or a user's own, for its
// manifest, once, to learn whether it answers, and answers its health. An
// addon is checked at most once a minute.
func (h *handler) checkAddon(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	scopes, err := h.ownedScopes(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	var found *addons.Addon
	var owner ownedScope
	for _, scope := range scopes {
		list, err := h.Addons.Addons(r.Context(), scope.Scope)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if i := slices.IndexFunc(list, func(a addons.Addon) bool { return a.ID == id && (a.Stremio() || a.Eclipse()) }); i >= 0 {
			found, owner = &list[i], scope
			break
		}
	}
	if found == nil || h.Health.Addons == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if allowed, wait := h.checks.allow(id.String(), h.now()); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too_soon")
		return
	}
	// The request reaches what the addon's own requests may: a member's
	// addon only public addresses. The answer is recorded whatever it is.
	_, _ = h.Health.Addons.Manifest(r.Context(), found.ManifestURL, owner.Confined)
	writeJSON(w, http.StatusOK, h.addonHealth(*found, owner.Owner))
}

// ownerJSON is the user whose own addon, source or guide a row is.
type ownerJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ownedScope is a scope of addons with its owner: nil for the server's.
// Confined is set for a user who is not an administrator, whose addons'
// requests may only reach public addresses.
type ownedScope struct {
	Scope    addons.Scope
	Owner    *ownerJSON
	Confined bool
}

// ownedScopes lists the server's scope, then every user's own, by name:
// what the dashboard describes to administrators.
func (h *handler) ownedScopes(ctx context.Context) ([]ownedScope, error) {
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		return nil, err
	}
	scopes := make([]ownedScope, 0, len(users)+1)
	scopes = append(scopes, ownedScope{Scope: addons.Shared()})
	for _, user := range users {
		scopes = append(scopes, ownedScope{Scope: addons.Personal(user.ID), Owner: &ownerJSON{ID: user.ID.String(), Name: user.Name},
			Confined: !user.IsAdministrator})
	}
	return scopes, nil
}

// ownedAddonJSON is an addon as the addon routes describe it, with its
// owner; ownedGuideJSON, a live TV catalog with an XMLTV guide.
type ownedAddonJSON struct {
	addonJSON
	Owner *ownerJSON `json:"owner"`
}

type ownedGuideJSON struct {
	libraryJSON
	Owner *ownerJSON `json:"owner"`
}

// sources lists, for the dashboard, the addons and IPTV sources of the
// server and of every user, as the addon routes describe them, and the
// live TV catalogs that have an XMLTV guide, the server's first. Nothing
// is fetched: this reads what the last fetches recorded.
func (h *handler) sources(w http.ResponseWriter, r *http.Request) {
	scopes, err := h.ownedScopes(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	refreshHours := h.Accounts.Settings().LiveTvRefreshHours
	result := struct {
		Addons []ownedAddonJSON `json:"addons"`
		Guides []ownedGuideJSON `json:"guides"`
	}{Addons: []ownedAddonJSON{}, Guides: []ownedGuideJSON{}}
	for _, scope := range scopes {
		list, err := h.Addons.Addons(r.Context(), scope.Scope)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, addon := range list {
			described, err := h.addonJSON(r, scope.Scope, addon)
			if err != nil {
				h.internalError(w, r, err)
				return
			}
			result.Addons = append(result.Addons, ownedAddonJSON{addonJSON: described, Owner: scope.Owner})
		}
		libraries, err := h.Addons.Libraries(r.Context(), scope.Scope)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, l := range libraries {
			if len(l.Guides) == 0 {
				continue
			}
			// A live TV catalog lists channels, not a library: it has no
			// name in apps.
			result.Guides = append(result.Guides, ownedGuideJSON{Owner: scope.Owner, libraryJSON: libraryJSON{
				AddonID: l.AddonID.String(), AddonName: l.AddonName, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID,
				CatalogName: l.Catalog.Name, Name: l.Name, Enabled: l.Enabled, Browsable: l.Catalog.Browsable(),
				Guide: newLibraryGuideJSON(l, refreshHours), Guides: newGuidesJSON(l.Guides, refreshHours)}})
		}
	}
	writeJSON(w, http.StatusOK, result)
}
