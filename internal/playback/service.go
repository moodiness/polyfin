// Package playback plays the versions addons offer: it analyzes them with
// ffprobe, decides whether a Jellyfin app can play them as they are, and
// delivers their bytes, by redirecting the app to the source or relaying it.
package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/subtitles"
)

const (
	// failureTTL is how long a version that could not be analyzed or
	// reached is not tried again.
	failureTTL = 15 * time.Minute
	// liveTTL is how long a source that answered is redirected to without
	// checking it again.
	liveTTL      = 2 * time.Minute
	checkTimeout = 10 * time.Second
	probeTimeout = 45 * time.Second
)

// ErrStandIn reports a source that is a short clip standing in for the
// title, such as the videos debrid proxies serve while a file downloads or
// after it failed. It may become the title later, so it is retried like
// other failures and its analysis is not kept.
var ErrStandIn = errors.New("the source is a short clip, not the title")

// Renewer asks the addon that listed a version for a fresh link to the
// same file; library.Service.Renew is one.
type Renewer func(ctx context.Context, version library.Version) (library.Version, error)

// Service plays versions.
type Service struct {
	db       *pgxpool.Pool
	opener   source.Opener
	prober   media.Prober
	sources  *source.Cache
	segments *hls.Manager
	loopback *loopback
	renew    Renewer
	signer   Signer
	logger   *slog.Logger

	flight   singleflight.Group
	analyses *cache.Cache[accounts.ID, media.Analysis]
	failures *cache.Cache[accounts.ID, error]
	live     *cache.Cache[accounts.ID, struct{}]
	hosts    *cache.Cache[string, bool]
	// indexes are keyframe times, and unindexed the versions whose index
	// could not be read.
	indexes   *cache.Cache[accounts.ID, []time.Duration]
	unindexed *cache.Cache[accounts.ID, error]
	// extractions are the subtitles remuxes extract, shared by the remuxes
	// of a version.
	extractedMu sync.Mutex
	extractions *cache.Cache[accounts.ID, *extracted]
	// locations are the subtitle streams versions' indexes list, and
	// unlocated the versions whose index could not be read.
	locations *cache.Cache[accounts.ID, []int]
	unlocated *cache.Cache[accounts.ID, error]
	// tracks are subtitle tracks read whole through indexes, untracked
	// those that could not be, and trackReads and hostReads bound the
	// reads under way, over every host and from each.
	tracks     *cache.Cache[trackKey, Track]
	untracked  *cache.Cache[trackKey, error]
	trackReads chan struct{}
	hostReads  hostSlots
	// trackCues are the cues of tracks read whole, for their HLS segments.
	trackCues *cache.Cache[trackKey, []subtitles.Cue]
	// attached are the files versions carry, read once for the fonts a
	// track asks for together.
	attached *cache.Cache[accounts.ID, []container.Attachment]
}

// New returns a playback service running ffprobe from ffprobePath, reading
// sources through sources, remuxing them with segments, and renewing
// expired links with renew, which may be nil.
func New(db *pgxpool.Pool, opener source.Opener, ffprobePath string, signer Signer, sources *source.Cache, segments *hls.Manager, renew Renewer, logger *slog.Logger) (*Service, error) {
	server, err := newLoopback()
	if err != nil {
		return nil, fmt.Errorf("start the source server: %w", err)
	}
	return &Service{
		db:          db,
		opener:      opener,
		prober:      media.Prober{Path: ffprobePath, Timeout: probeTimeout},
		sources:     sources,
		segments:    segments,
		loopback:    server,
		renew:       renew,
		signer:      signer,
		logger:      logger,
		analyses:    cache.New[accounts.ID, media.Analysis](2000, time.Hour),
		failures:    cache.New[accounts.ID, error](2000, failureTTL),
		live:        cache.New[accounts.ID, struct{}](2000, liveTTL),
		hosts:       cache.New[string, bool](500, 5*time.Minute),
		indexes:     cache.New[accounts.ID, []time.Duration](200, time.Hour),
		unindexed:   cache.New[accounts.ID, error](2000, failureTTL),
		extractions: cache.New[accounts.ID, *extracted](200, 6*time.Hour),
		locations:   cache.New[accounts.ID, []int](2000, time.Hour),
		unlocated:   cache.New[accounts.ID, error](2000, failureTTL),
		tracks:      cache.New[trackKey, Track](100, time.Hour),
		untracked:   cache.New[trackKey, error](2000, failureTTL),
		trackReads:  make(chan struct{}, maxTrackReads),
		trackCues:   cache.New[trackKey, []subtitles.Cue](50, time.Hour),
		attached:    cache.New[accounts.ID, []container.Attachment](8, 10*time.Minute),
	}, nil
}

// Close stops the source server.
func (s *Service) Close() error {
	return s.loopback.Close()
}

// open opens a version's source in the cache, renewing its link when it
// expires. The caller releases it.
func (s *Service) open(version library.Version) *source.Source {
	var renew source.Renewer
	if s.renew != nil {
		renew = func(ctx context.Context) (source.Location, error) {
			fresh, err := s.renew(ctx, version)
			return locationOf(fresh), err
		}
	}
	return s.sources.Open(version.ID, locationOf(version), renew)
}

func locationOf(version library.Version) source.Location {
	return source.Location{URL: version.URL, Headers: version.Headers, Confined: version.Confined}
}

// Signer returns the service's grant signer.
func (s *Service) Signer() Signer {
	return s.signer
}

// Analyzed returns the analysis of a version if it was analyzed before.
func (s *Service) Analyzed(ctx context.Context, version accounts.ID) (media.Analysis, bool) {
	if analysis, ok := s.analyses.Get(version); ok {
		return analysis, true
	}
	var data []byte
	err := s.db.QueryRow(ctx, "SELECT analysis FROM media_analyses WHERE version_id = $1", version).Scan(&data)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("Reading a media analysis failed", "error", err)
		}
		return media.Analysis{}, false
	}
	var analysis media.Analysis
	if err := json.Unmarshal(data, &analysis); err != nil {
		return media.Analysis{}, false
	}
	s.analyses.Put(version, analysis)
	return analysis, true
}

// Analyze returns what ffprobe finds in a version, analyzing it on its
// first use. A version that cannot be analyzed is not tried again for a
// while.
func (s *Service) Analyze(ctx context.Context, version library.Version) (media.Analysis, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return analysis, nil
	}
	if err, failed := s.failures.Get(version.ID); failed {
		return media.Analysis{}, err
	}
	result, err, _ := s.flight.Do("analyze "+version.ID.String(), func() (any, error) {
		// Requests for the version share this analysis, and it is kept: it
		// runs to the end, and is saved, even once the request that started
		// it is canceled, as when an app stops waiting.
		ctx := context.WithoutCancel(ctx)
		// ffprobe reads through the source cache, so that confined sources
		// stay confined, required headers are sent, and what it reads is
		// kept for playback.
		src := s.open(version)
		defer src.Release()
		target, release := s.loopback.register(src)
		defer release()
		started := time.Now()
		analysis, err := s.prober.Probe(ctx, target)
		if err == nil && standIn(analysis, version.Runtime) {
			err = fmt.Errorf("%w: %s long, where the title lasts %s", ErrStandIn, analysis.Duration.Round(time.Second), version.Runtime)
		}
		if err != nil {
			s.failures.Put(version.ID, err)
			return nil, err
		}
		if analysis.Size == 0 {
			analysis.Size = version.Size
		}
		data, err := json.Marshal(analysis)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)
			ON CONFLICT (version_id) DO UPDATE SET analysis = excluded.analysis, analyzed_at = now()`, version.ID, data); err != nil {
			s.logger.Warn("Saving a media analysis failed", "error", err)
		}
		s.analyses.Put(version.ID, analysis)
		s.logger.Debug("Analyzed a version", "addon", version.Addon, "duration", time.Since(started))
		return analysis, nil
	})
	if err != nil {
		return media.Analysis{}, err
	}
	return result.(media.Analysis), nil
}

// standIn reports whether an analysis is of a clip too short to be the
// title: under a tenth of its runtime, which leaves room for runtimes that
// addons round or give per series rather than per episode.
func standIn(analysis media.Analysis, runtime time.Duration) bool {
	return analysis.Duration > 0 && runtime > 0 && analysis.Duration*10 < runtime
}

// Failed reports whether a version recently failed to be analyzed or
// reached.
func (s *Service) Failed(version accounts.ID) bool {
	_, failed := s.failures.Get(version)
	return failed
}

// Delivery is how a version's bytes reach a player.
type Delivery struct {
	// Relay sends the bytes through Polyfin even when the player could
	// fetch the source itself.
	Relay bool
	// ContentType, when set, replaces the source's.
	ContentType string
	// Attachment, when set, is the name of the file the bytes are saved
	// as: the player downloads them. A player sent to the source gets the
	// name too, though the source's answer decides there.
	Attachment string
}

// Serve answers a player's request for a version's bytes. The player is
// redirected to the source when it can fetch it itself: the source needs
// no headers, is on a public address, and answers now. Otherwise, or when
// the delivery says so, Polyfin relays the bytes. An expired link is
// renewed once.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request, version library.Version, delivery Delivery) error {
	if !delivery.Relay && len(version.Headers) == 0 && s.public(r.Context(), version.URL) {
		current, err := s.check(r.Context(), version)
		if err != nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return err
		}
		w.Header().Set("Cache-Control", "no-store")
		setAttachment(w, delivery.Attachment)
		http.Redirect(w, r, current.URL, http.StatusFound)
		return nil
	}
	return s.relay(w, r, version, delivery)
}

// setAttachment names the file an answer saves as, the way Jellyfin
// (ASP.NET) does: an ASCII name, quoted unless it is a token, for old
// clients, with other characters as underscores, and the exact name
// encoded as RFC 8187 asks.
func setAttachment(w http.ResponseWriter, name string) {
	if name == "" {
		return
	}
	ascii := []byte(strings.Map(func(c rune) rune {
		if c > 0x7e || c < ' ' {
			return '_'
		}
		return c
	}, name))
	plain := string(ascii)
	if slices.ContainsFunc(ascii, func(c byte) bool { return !tokenByte(c) }) {
		plain = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(plain) + `"`
	}
	var encoded strings.Builder
	for _, c := range []byte(name) {
		if tokenByte(c) && c != '%' && c != '\'' && c != '*' {
			encoded.WriteByte(c)
		} else {
			fmt.Fprintf(&encoded, "%%%02X", c)
		}
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+plain+"; filename*=UTF-8''"+encoded.String())
}

// tokenByte reports whether c may appear in an HTTP token.
func tokenByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}

// check makes sure a source answers before a player is sent to it, as some
// players give up on the first failure, and returns the version with the
// link that answered.
func (s *Service) check(ctx context.Context, version library.Version) (library.Version, error) {
	if _, ok := s.live.Get(version.ID); ok {
		return version, nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	status, err := s.firstByte(ctx, version)
	if err == nil && expired(status) && s.renew != nil {
		fresh, renewErr := s.renew(ctx, version)
		if renewErr != nil {
			err = fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrSourceUnavailable, status, renewErr)
		} else {
			version = fresh
			status, err = s.firstByte(ctx, version)
		}
	}
	if err == nil && status != http.StatusOK && status != http.StatusPartialContent {
		err = fmt.Errorf("%w: HTTP %d", ErrSourceUnavailable, status)
	}
	if err != nil {
		s.failures.Put(version.ID, err)
		return library.Version{}, err
	}
	s.live.Put(version.ID, struct{}{})
	return version, nil
}

// firstByte asks a source for its first byte and returns the status.
func (s *Service) firstByte(ctx context.Context, version library.Version) (int, error) {
	header := http.Header{"Range": {"bytes=0-0"}}
	for name, value := range version.Headers {
		header.Set(name, value)
	}
	response, err := s.opener.Open(ctx, http.MethodGet, version.URL, header, version.Confined)
	if err != nil {
		return 0, err
	}
	response.Body.Close()
	return response.StatusCode, nil
}

// public reports whether every address of the URL's host is public, so that
// a player outside the server's network can reach it.
func (s *Service) public(ctx context.Context, raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		return publicAddress(ip)
	}
	if public, ok := s.hosts.Get(host); ok {
		return public
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return false
	}
	public := len(addresses) > 0
	for _, ip := range addresses {
		public = public && publicAddress(ip)
	}
	s.hosts.Put(host, public)
	return public
}

var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedAddressSpace.Contains(ip)
}
