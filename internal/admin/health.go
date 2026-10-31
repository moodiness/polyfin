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

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/discovery"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/dlna"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/https"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/updates"
)

// HealthSources are what the health page reads, each optional: nothing
// it reads makes a request outside the server.
type HealthSources struct {
	// Addons tells how the requests to the addons went (stremio.Client).
	Addons *stremio.Client
	// SearchesLeaveOut tells whether the searches leave out a search
	// catalog of an addon for now, as it did not answer a search in time
	// (library.Service.SearchesLeaveOut); nil tells none.
	SearchesLeaveOut func(addon accounts.ID) bool
	// Cache keeps the sources being played. Folders are the folders of
	// the run, which keep it, the segments and the cache, those the
	// settings chose that could not be written into at start among them.
	Cache   *source.Cache
	Folders config.Folders
	// Encoder runs the remuxes and conversions.
	Encoder *hls.Manager
	// NVIDIADriver tells the NVIDIA driver version NVENC needs, when the
	// GPU's driver is older (hls.Manager.NVIDIADriver), empty otherwise;
	// nil tells nothing.
	NVIDIADriver func() string
	// OlderDriverFFmpeg tells how the FFmpeg for NVIDIA drivers older than
	// 610 stands (hls.Manager.OlderDriverFFmpeg); nil tells nothing.
	OlderDriverFFmpeg func() hls.OlderDriverStatus
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
	// Updates tells the new version of Polyfin its daily check found; nil
	// tells none.
	Updates *updates.Checker
	// Proxies names the addresses that send forwarded headers without
	// being known proxies, and what the host names of Known proxies
	// resolve to; nil names none.
	Proxies *forwarded.Proxies
	// Discovery tells how local network discovery goes; nil tells
	// nothing.
	Discovery func() discovery.Status
	// DLNA tells how the sharing over DLNA goes; nil tells nothing.
	DLNA func() dlna.Status
	// HTTPS tells how HTTPS stands; nil tells nothing.
	HTTPS func() https.Status
	// MissingLanguages lists the languages, by Tesseract's codes, of the
	// picture subtitle tracks left unread for want of their data; nil
	// lists none.
	MissingLanguages func(context.Context) []string
	// Torrents plays the torrent streams; nil tells nothing.
	Torrents *torrent.Engine
	// Images keeps the artwork relayed, resized and drawn for apps
	// (jellyfin.Handler); nil tells nothing.
	Images ImageCache
}

// ImageCache keeps the images relayed, resized and drawn for apps, and can
// be emptied (see jellyfin.Handler.ClearImageCache).
type ImageCache interface {
	ImageCache() jellyfin.ImageCacheUsage
	ClearImageCache() jellyfin.ImageCacheUsage
}

type healthJSON struct {
	CheckedAt time.Time    `json:"checkedAt"`
	Process   processJSON  `json:"process"`
	Database  databaseJSON `json:"database"`
	Cache     *cacheJSON   `json:"cache"`
	// Images is what the image cache keeps, null when unknown.
	Images *imageCacheJSON `json:"images"`
	// Torrents is how torrent streams play, null while they are off.
	Torrents   *torrentHealthJSON   `json:"torrents"`
	Disks      []diskJSON           `json:"disks"`
	Transcoder *transcoderJSON      `json:"transcoder"`
	Thumbnails *thumbnailHealthJSON `json:"thumbnails"`
	Addons     []addonHealthJSON    `json:"addons"`
	// Shares is how the enabled network shares are watched for changes;
	// WatchFolders tells whether they are (Settings.WatchLocalFolders).
	Shares       []shareHealthJSON `json:"shares"`
	WatchFolders bool              `json:"watchFolders"`
	// Secrets is how the stored keys and tokens stand, null when unknown.
	Secrets *secretsHealthJSON `json:"secrets"`
	// Backup is how the database backups go, null when they are off.
	Backup *backupJSON `json:"backup"`
	// Update is the new version of Polyfin out, null when there is none,
	// or while Check for new versions is off.
	Update *updateJSON `json:"update"`
	// ProxyNames are the host names of Known proxies, with what each
	// resolved to when last looked up.
	ProxyNames []proxyNameJSON `json:"proxyNames"`
	// Ratings is how the lookups of titles' ratings on MDBList stand,
	// null while no key is saved.
	Ratings *ratingsHealthJSON `json:"ratings"`
}

// proxyNameJSON is a host name of Known proxies and the addresses it
// resolved to at ResolvedAt, sorted: none when it resolves to nothing,
// which trusts nothing.
type proxyNameJSON struct {
	Name       string    `json:"name"`
	Addresses  []string  `json:"addresses"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

// shareHealthJSON is how a network share is watched for changes: Watch is
// "notified" when its server tells of them, "polled" when its folders are
// listed again every few minutes, with why in Polled (see
// localfiles.Folder), empty when it is not watched, with why in
// Unwatched when it should be. PollMinutes is how often an S3 bucket, an
// SFTP folder or an rclone remote is listed again, 0 only at its scans,
// null for the other shares.
type shareHealthJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Share       string `json:"share"`
	Watch       string `json:"watch"`
	Polled      string `json:"polled"`
	Unwatched   string `json:"unwatched"`
	PollMinutes *int   `json:"pollMinutes"`
}

// updateJSON is a new version of Polyfin, and the address of its release
// notes.
type updateJSON struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// ratingsHealthJSON is how the lookups of titles' ratings on MDBList
// stand: the requests MDBList allows a day (Limit), those left (Remaining)
// until ResetsAt, each null until MDBList tells them; HeldUntil, when it is
// left alone after a 429 ("limit"), a key it refused ("refused") or a
// failure ("failed"), as Held tells; the titles waiting to be asked about,
// and those whose ratings are kept.
type ratingsHealthJSON struct {
	Limit     *int       `json:"limit"`
	Remaining *int       `json:"remaining"`
	ResetsAt  *time.Time `json:"resetsAt"`
	HeldUntil *time.Time `json:"heldUntil"`
	Held      string     `json:"held"`
	Waiting   int        `json:"waiting"`
	Kept      int        `json:"kept"`
}

// ratingsHealth describes how the lookups of titles' ratings stand, nil
// while no key is saved.
func (h *handler) ratingsHealth() *ratingsHealthJSON {
	status := h.Ratings.Status()
	if !status.Enabled {
		return nil
	}
	result := &ratingsHealthJSON{Held: status.Held, Waiting: status.Waiting, Kept: status.Kept}
	if status.Known {
		result.Limit, result.Remaining = new(status.Limit), new(status.Remaining)
	}
	if !status.ResetsAt.IsZero() {
		result.ResetsAt = new(status.ResetsAt)
	}
	if !status.HeldUntil.IsZero() {
		result.HeldUntil = new(status.HeldUntil)
	}
	return result
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

// cacheJSON is what the source cache keeps: Used bytes against its Limit,
// Sources being read and Kept holding blocks, and Hits and Misses, the
// bytes read since the start that were read before or kept from before
// the start, and those read for the first time since they were fetched,
// which give its hit ratio; Days is how long it keeps blocks across
// restarts, 0 for not at all.
type cacheJSON struct {
	Used    int64 `json:"used"`
	Limit   int64 `json:"limit"`
	Sources int   `json:"sources"`
	Kept    int   `json:"kept"`
	Hits    int64 `json:"hits"`
	Misses  int64 `json:"misses"`
	Days    int   `json:"days"`
	// Torrents is the part of Used the pieces of torrents take.
	Torrents int64 `json:"torrents"`
}

// imageCacheJSON is what the image cache keeps, or what clearing it freed:
// Files images on disk, of Used bytes, which Limit bounds, and
// MemoryImages in memory, of MemoryBytes bytes.
type imageCacheJSON struct {
	Files        int   `json:"files"`
	Used         int64 `json:"used"`
	Limit        int64 `json:"limit"`
	MemoryImages int   `json:"memoryImages"`
	MemoryBytes  int64 `json:"memoryBytes"`
}

func newImageCacheJSON(u jellyfin.ImageCacheUsage) *imageCacheJSON {
	return &imageCacheJSON{Files: u.Files, Used: u.Bytes, Limit: u.Limit, MemoryImages: u.MemoryImages, MemoryBytes: u.MemoryBytes}
}

// clearImageCache empties the image cache, and answers what it freed.
func (h *handler) clearImageCache(w http.ResponseWriter, r *http.Request) {
	if h.Health.Images == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, newImageCacheJSON(h.Health.Images.ClearImageCache()))
}

// torrentHealthJSON is how torrent streams play: their route, "interface",
// "proxy" or "wireguard", empty for none, Name the network interface's
// name, the SOCKS5 proxy's address or the WireGuard server, the torrents
// playing, their peers, those of them reached over uTP, and the bytes
// shared since the server started. Over a WireGuard tunnel, Addresses are
// the tunnel's addresses and LastHandshake when its last handshake was,
// null for none yet.
type torrentHealthJSON struct {
	Route         string     `json:"route"`
	Name          string     `json:"name"`
	Addresses     []string   `json:"addresses,omitempty"`
	LastHandshake *time.Time `json:"lastHandshake,omitempty"`
	Playing       int        `json:"playing"`
	AtOnce        int        `json:"atOnce"`
	Peers         int        `json:"peers"`
	UTPPeers      int        `json:"utpPeers"`
	Uploaded      int64      `json:"uploaded"`
}

// diskJSON is the disk one of the server's folders is on: Folder is
// "cache", "conversion", "reading" (the files being read), "recordings"
// or "backups".
type diskJSON struct {
	Folder string `json:"folder"`
	Path   string `json:"path"`
	// Free and Used are in bytes, -1 when the disk cannot be measured.
	Free  int64  `json:"free"`
	Used  int64  `json:"used"`
	Mount string `json:"mount"`
	// Low tells whether the disk is short of room: a problem Health shows.
	Low bool `json:"low"`
}

// measuredFolders are the folders whose disks Health measures, by name:
// those of the run (see config.Folders.Disks), then the recordings' and
// the backups', each empty while not in use.
func (h *handler) measuredFolders() []struct{ name, path string } {
	var folders []struct{ name, path string }
	for _, disk := range h.Health.Folders.Disks() {
		folders = append(folders, struct{ name, path string }{disk.Name, disk.Path})
	}
	return append(folders, struct{ name, path string }{"recordings", h.Recordings.Dir()},
		struct{ name, path string }{"backups", h.Backups.Dir()})
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
	// OlderDriverFFmpeg tells how the FFmpeg for NVIDIA drivers older than
	// 610 stands while NVENC waits for it, or failed with it; null when it
	// is used (Hardware.OlderDrivers) or not needed.
	OlderDriverFFmpeg *olderDriverJSON `json:"olderDriverFFmpeg"`
}

type hardwareJSON struct {
	Method      string   `json:"method"`
	Device      string   `json:"device"`
	Encoders    []string `json:"encoders"`
	ToneMapping bool     `json:"toneMapping"`
	// QVBR tells whether a VAAPI GPU takes a quality factor.
	QVBR bool `json:"qvbr"`
	// OlderDrivers tells the GPU encodes with the FFmpeg for NVIDIA
	// drivers older than 610, not with the one Polyfin ships.
	OlderDrivers bool `json:"olderDrivers"`
	// HDR is where HDR titles are tone mapped and why, null when the GPU
	// does not tone map.
	HDR *hdrPathJSON `json:"hdr"`
}

// newHardwareJSON describes hw, on which HDR goes as path says, unless the
// settings turn tone mapping on the GPU off (gpuToneMapping).
func newHardwareJSON(hw *hls.Hardware, path hls.HDRPath, gpuToneMapping bool) *hardwareJSON {
	return &hardwareJSON{Method: hw.Method, Device: hw.Device, Encoders: hw.Encoders, ToneMapping: hw.ToneMapping, QVBR: hw.QVBR,
		OlderDrivers: hw.FFmpeg != "", HDR: newHDRPathJSON(path, gpuToneMapping)}
}

// hdrPathJSON is where the HDR titles a GPU tone maps are tone mapped:
// Path "gpu" or "processor", Height the height the GPU tone maps them up
// to, 0 for any, and Reason why: one of hls's HDR reasons, or "turnedOff"
// when the settings turn tone mapping on the GPU off. GPUSpeed, LowSpeed
// and ProcessorSpeed are the timings at startup it follows, in times real
// time, 0 when not timed (see hls.HDRPath).
type hdrPathJSON struct {
	Path           string  `json:"path"`
	Height         int     `json:"height"`
	Reason         string  `json:"reason"`
	GPUSpeed       float64 `json:"gpuSpeed"`
	LowSpeed       float64 `json:"lowSpeed"`
	ProcessorSpeed float64 `json:"processorSpeed"`
}

// newHDRPathJSON describes path, nil for a GPU that does not tone map.
func newHDRPathJSON(path hls.HDRPath, gpuToneMapping bool) *hdrPathJSON {
	if path.Reason == "" {
		return nil
	}
	result := &hdrPathJSON{Path: "gpu", Height: path.Height, Reason: path.Reason, GPUSpeed: path.GPUSpeed, LowSpeed: path.LowSpeed,
		ProcessorSpeed: path.ProcessorSpeed}
	switch {
	case !gpuToneMapping:
		result.Path, result.Height, result.Reason = "processor", 0, "turnedOff"
	case path.Processor:
		result.Path = "processor"
	}
	return result
}

// olderDriverJSON is the FFmpeg for NVIDIA drivers older than 610 while
// NVENC waits for it, State "downloading", or could not use it: "failed"
// to download it, or "unusable" when NVENC failed with it too, Error
// telling why.
type olderDriverJSON struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// newOlderDriverJSON is how the FFmpeg for older NVIDIA drivers stands,
// nil when it is used or not needed.
func newOlderDriverJSON(encoder *hls.Manager) *olderDriverJSON {
	status := encoder.OlderDriverFFmpeg()
	if status.State == "" || status.State == "used" {
		return nil
	}
	return &olderDriverJSON{State: status.State, Error: status.Error}
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
	// CatalogsWaitUntil is when the wait the addon's server asked for ends,
	// its catalog pages not asked for until then; nil when none runs.
	// Failure is rate_limited meanwhile.
	CatalogsWaitUntil *time.Time `json:"catalogsWaitUntil"`
}

// softwareEncoders are the CPU encoders conversions use.
var softwareEncoders = []string{"libx264", "libx265", "aac", "libfdk_aac", "ac3", "eac3", "libopus", "flac"}

// health describes how the server's parts fare, from what they record:
// nothing is requested outside the server.
func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	result := healthJSON{CheckedAt: h.now(), Disks: []diskJSON{}, Addons: []addonHealthJSON{}, Shares: []shareHealthJSON{},
		ProxyNames: []proxyNameJSON{}}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result.Process = processJSON{Version: h.Version, GoVersion: runtime.Version(), StartedAt: h.Health.Started,
		Memory: memory.Sys, Heap: memory.HeapAlloc, Goroutines: runtime.NumGoroutine()}

	// The database's checks share a deadline; the torrent route's check,
	// below, has its own, since a WireGuard tunnel whose server does not
	// answer waits out the whole of it for a handshake.
	ctx, cancel := context.WithTimeout(r.Context(), readyWait)
	defer cancel()
	result.Database.Reachable = h.Database.Ping(ctx) == nil
	if result.Database.Reachable && h.Health.DatabaseSize != nil {
		if size, err := h.Health.DatabaseSize(ctx); err == nil {
			result.Database.Size = &size
		}
	}
	if result.Database.Reachable {
		backup, err := h.backupStatus(ctx)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		result.Backup = backup
	} else if h.Backups.Available() {
		result.Backup = &backupJSON{Folder: h.Backups.Dir()}
	}

	if h.Health.Images != nil {
		result.Images = newImageCacheJSON(h.Health.Images.ImageCache())
	}
	if h.Health.Cache != nil {
		usage := h.Health.Cache.Usage()
		result.Cache = &cacheJSON{Used: usage.Used, Limit: usage.Limit, Sources: usage.Open, Kept: usage.Kept, Hits: usage.Hits,
			Misses: usage.Misses, Days: h.Accounts.Settings().SourceCacheDays}
		if h.Health.Torrents != nil {
			result.Cache.Torrents = h.Health.Torrents.Usage()
		}
	}
	if h.Health.Torrents != nil {
		routeCtx, cancelRoute := context.WithTimeout(r.Context(), readyWait)
		status := h.Health.Torrents.Status(routeCtx)
		cancelRoute()
		if status.Enabled {
			result.Torrents = &torrentHealthJSON{Playing: status.Playing, AtOnce: status.AtOnce, Peers: status.Peers, UTPPeers: status.UTPPeers,
				Uploaded: h.Health.Torrents.Uploaded()}
			switch settings := h.Accounts.Settings(); {
			case settings.TorrentInterface != "":
				result.Torrents.Route, result.Torrents.Name = "interface", settings.TorrentInterface
			case settings.TorrentProxy != "":
				result.Torrents.Route, result.Torrents.Name = "proxy", settings.TorrentProxy
			case status.Tunnel != nil:
				result.Torrents.Route, result.Torrents.Name = "wireguard", status.Tunnel.Endpoint
				for _, prefix := range status.Tunnel.Addresses {
					result.Torrents.Addresses = append(result.Torrents.Addresses, prefix.String())
				}
				if !status.Tunnel.LastHandshake.IsZero() {
					result.Torrents.LastHandshake = &status.Tunnel.LastHandshake
				}
			}
		}
	}
	for _, folder := range h.measuredFolders() {
		if folder.path == "" {
			continue
		}
		disk := diskJSON{Folder: folder.name, Path: filepath.Clean(folder.path), Free: -1, Used: -1}
		if free, used, mount, ok := diskspace.Measure(folder.path); ok {
			disk.Free, disk.Used, disk.Mount, disk.Low = free, used, mount, lowOnSpace(free, used)
		}
		result.Disks = append(result.Disks, disk)
	}

	settings := h.Accounts.Settings()
	result.WatchFolders = settings.WatchLocalFolders
	if encoder := h.Health.Encoder; encoder != nil {
		t := &transcoderJSON{Encoders: []string{}, MaxHeight: settings.MaxConversionHeight, Enabled: settings.Transcoding}
		if hw := encoder.Hardware(); hw != nil {
			t.Hardware = newHardwareJSON(hw, encoder.HDRPath(hw), settings.GPUToneMapping)
		}
		for _, name := range softwareEncoders {
			if slices.Contains(encoder.Encoders(), name) {
				t.Encoders = append(t.Encoders, name)
			}
		}
		t.Conversions, t.Limit = encoder.Conversions()
		t.OlderDriverFFmpeg = newOlderDriverJSON(encoder)
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
			if addon.Local() && addon.Enabled && h.Folders != nil {
				folder, err := h.Folders.Folder(r.Context(), addon.ID)
				if err != nil {
					h.internalError(w, r, err)
					return
				}
				if folder.Share != "" {
					result.Shares = append(result.Shares, shareHealthJSON{ID: addon.ID.String(), Name: addon.Manifest.Name, Share: folder.Share,
						Watch: folder.ShareWatch, Polled: folder.Polled, Unwatched: folder.Unwatched, PollMinutes: folder.PollMinutes})
				}
				continue
			}
			// Eclipse addons' requests count in their health too.
			if !addon.Stremio() && !addon.Eclipse() {
				continue
			}
			result.Addons = append(result.Addons, h.addonHealth(addon, scope.Owner))
		}
	}
	result.Secrets = h.secretsHealth(r.Context())
	result.Ratings = h.ratingsHealth()
	if release, ok := h.Health.Updates.Available(); ok {
		result.Update = &updateJSON{Version: release.Version, URL: release.URL}
	}
	if h.Health.Proxies != nil {
		for _, name := range h.Health.Proxies.Names() {
			addresses := make([]string, len(name.Addresses))
			for i, address := range name.Addresses {
				addresses[i] = address.String()
			}
			result.ProxyNames = append(result.ProxyNames, proxyNameJSON{Name: name.Name, Addresses: addresses, ResolvedAt: name.Resolved})
		}
	}
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
	if !health.CatalogsWaitUntil.IsZero() {
		result.CatalogsWaitUntil = new(health.CatalogsWaitUntil)
	}
	return result
}

// checkInterval is how often an addon may be checked by hand, one request
// for its manifest, never more, and how often each of the server's
// actions may be asked (see serverAction).
const checkInterval = time.Minute

// spacing remembers when each request was last allowed, by key: an
// addon checked by hand, or one of the server's actions.
type spacing struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// allow reports whether the request of id may be allowed at now, and
// records it, or how long until it may.
func (c *spacing) allow(id string, now time.Time) (bool, time.Duration) {
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
				CatalogName: l.Catalog.Name, Name: l.Name, Enabled: l.Enabled, Browsable: l.Catalog.Browsable(), Genres: []string{},
				Guide: h.newLibraryGuideJSON(l, refreshHours), Guides: h.newGuidesJSON(l.Guides, refreshHours)}})
		}
	}
	writeJSON(w, http.StatusOK, result)
}
