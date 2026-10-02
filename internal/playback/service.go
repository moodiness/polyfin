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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
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

// Service plays versions.
type Service struct {
	db      *pgxpool.Pool
	opener  Opener
	prober  media.Prober
	sources *sourceServer
	signer  Signer
	logger  *slog.Logger

	flight   singleflight.Group
	analyses *cache.Cache[accounts.ID, media.Analysis]
	failures *cache.Cache[accounts.ID, error]
	live     *cache.Cache[accounts.ID, struct{}]
	hosts    *cache.Cache[string, bool]
}

// New returns a playback service running ffprobe from ffprobePath.
func New(db *pgxpool.Pool, opener Opener, ffprobePath string, signer Signer, logger *slog.Logger) (*Service, error) {
	sources, err := newSourceServer(opener)
	if err != nil {
		return nil, fmt.Errorf("start the source server: %w", err)
	}
	return &Service{
		db:       db,
		opener:   opener,
		prober:   media.Prober{Path: ffprobePath, Timeout: probeTimeout},
		sources:  sources,
		signer:   signer,
		logger:   logger,
		analyses: cache.New[accounts.ID, media.Analysis](2000, time.Hour),
		failures: cache.New[accounts.ID, error](2000, failureTTL),
		live:     cache.New[accounts.ID, struct{}](2000, liveTTL),
		hosts:    cache.New[string, bool](500, 5*time.Minute),
	}, nil
}

// Close stops the source server.
func (s *Service) Close() error {
	return s.sources.Close()
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
		// ffprobe reads through the source server, so that confined sources
		// stay confined and required headers are sent.
		target, release := s.sources.register(sourceOf(version))
		defer release()
		started := time.Now()
		analysis, err := s.prober.Probe(context.WithoutCancel(ctx), target)
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

func sourceOf(version library.Version) Source {
	return Source{URL: version.URL, Headers: version.Headers, Confined: version.Confined}
}

// Serve answers a player's request for a version's bytes. The player is
// redirected to the source when it can fetch it itself: the source needs
// no headers, is on a public address, and answers now. Otherwise, or when
// relay is set, Polyfin relays the bytes, as contentType when set.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request, version library.Version, relay bool, contentType string) error {
	if !relay && len(version.Headers) == 0 && s.public(r.Context(), version.URL) {
		if err := s.check(r.Context(), version); err != nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return err
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, version.URL, http.StatusFound)
		return nil
	}
	return Relay(w, r, s.opener, sourceOf(version), contentType)
}

// check makes sure a source answers before a player is sent to it: some
// players give up on the first failure.
func (s *Service) check(ctx context.Context, version library.Version) error {
	if _, ok := s.live.Get(version.ID); ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	header := http.Header{"Range": {"bytes=0-0"}}
	response, err := s.opener.Open(ctx, http.MethodGet, version.URL, header, version.Confined)
	if err != nil {
		s.failures.Put(version.ID, err)
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		err := fmt.Errorf("%w: HTTP %d", ErrSourceUnavailable, response.StatusCode)
		s.failures.Put(version.ID, err)
		return err
	}
	s.live.Put(version.ID, struct{}{})
	return nil
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
