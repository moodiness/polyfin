// Package playback plays the versions addons offer: it analyzes them with
// ffprobe, decides whether a Jellyfin app can play them as they are, and
// delivers their bytes, by redirecting the app to the source or relaying it.
package playback

import (
	"bytes"
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
	"github.com/moodiness/polyfin/internal/torrent"
)

const (
	// failureTTL is how long a version that could not be analyzed or
	// reached is not tried again.
	failureTTL = 15 * time.Minute
	// liveTTL is how long a source that answered is redirected to without
	// checking it again.
	liveTTL      = 2 * time.Minute
	checkTimeout = 10 * time.Second
)

// ErrStandIn reports a source that is a short clip standing in for the
// title, such as the videos debrid proxies serve while a file downloads or
// after it failed. It may become the title later, so it is retried like
// other failures and its analysis is not kept.
var ErrStandIn = errors.New("the source is a short clip, not the title")

// ErrPlaylistReplay reports a replay whose provider's archive answers an
// HLS playlist. Replays play as titles' files do, read through the source
// cache, and the HLS path Polyfin has for sources is the live one, which
// keeps a sliding window of an endless stream: neither plays a finite
// playlist, so such a replay is refused rather than played wrong.
var ErrPlaylistReplay = errors.New("the provider's archive answered an HLS playlist, which a replay cannot play")

// Renewer asks the addon that listed a version for a fresh link to the
// same file; library.Service.Renew is one.
type Renewer func(ctx context.Context, version library.Version) (library.Version, error)

// Service plays versions.
type Service struct {
	db       *pgxpool.Pool
	opener   source.Opener
	prober   media.Prober
	settings func() accounts.Settings
	sources  *source.Cache
	segments *hls.Manager
	loopback *loopback
	renew    Renewer
	signer   Signer
	logger   *slog.Logger

	flight   singleflight.Group
	analyses *cache.Cache[accounts.ID, media.Analysis]
	// failures are why versions could not be analyzed or reached, kept
	// for failureTTL (see fail).
	failures *cache.Cache[accounts.ID, versionFailure]
	// unmastered are the versions whose first HDR frame could not be read
	// (see LearnMasteringDisplay).
	unmastered *cache.Cache[accounts.ID, error]
	live       *cache.Cache[accounts.ID, struct{}]
	hosts      *cache.Cache[string, bool]
	// indexes are keyframe times, unindexed the versions whose index
	// cannot be read, and offsets where the keyframes start in the file.
	indexes   *cache.Cache[accounts.ID, []time.Duration]
	unindexed *cache.Cache[accounts.ID, error]
	offsets   *cache.Cache[accounts.ID, []container.Keyframe]
	// warming are the warms under way, by version, and warmed the start
	// each version was warmed from lately (see Warm).
	warmMu  sync.Mutex
	warming map[accounts.ID]*warmRun
	warmed  *cache.Cache[accounts.ID, time.Duration]
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
	// rangeHosts tells, by host, whether it serves several ranges with one
	// request.
	rangeHosts *cache.Cache[string, bool]
	// trackCues are the cues of tracks read whole, for their HLS segments.
	trackCues *cache.Cache[trackKey, []subtitles.Cue]
	// attached are the files versions carry, read once for the fonts a
	// track asks for together.
	attached *cache.Cache[accounts.ID, []container.Attachment]
	// feeds are the live streams being read (see feed.go).
	feeds liveState
	// speech is what is known of versions' speech, and of the subtitles
	// synced to it (see speech.go).
	speech speechState
	// pictures reads picture subtitles into text (see pictures.go).
	pictures pictureReader
	// torrents plays the versions that are torrents' files, nil when
	// Polyfin plays none (see Torrents).
	torrents *torrent.Engine
}

// New returns a playback service running ffprobe from ffprobePath, reading
// sources through sources, remuxing them with segments, and renewing
// expired links with renew, which may be nil. settings returns the server
// settings, read as they apply: the analysis timeout as each analysis
// starts, the tuning of conversions as each encoding opens, and, which it
// sets on segments, the limit of the playbacks converting video, whenever
// one more would start, the segments made ahead of the player, and whether
// converted video is kept when the audio track changes, as each request
// comes.
func New(db *pgxpool.Pool, opener source.Opener, ffprobePath string, signer Signer, sources *source.Cache, segments *hls.Manager, renew Renewer, logger *slog.Logger,
	settings func() accounts.Settings) (*Service, error) {
	server, err := newLoopback()
	if err != nil {
		return nil, fmt.Errorf("start the source server: %w", err)
	}
	segments.LimitConversions(func() int { return settings().MaxConversions })
	segments.LimitAhead(func() time.Duration { return time.Duration(settings().AheadSeconds) * time.Second })
	segments.KeepVideo(func() bool { return settings().KeepConvertedVideo })
	s := &Service{
		db:          db,
		opener:      opener,
		settings:    settings,
		prober:      media.Prober{Path: ffprobePath},
		sources:     sources,
		segments:    segments,
		loopback:    server,
		renew:       renew,
		signer:      signer,
		logger:      logger,
		analyses:    cache.New[accounts.ID, media.Analysis](2000, time.Hour),
		failures:    cache.New[accounts.ID, versionFailure](2000, failureTTL),
		unmastered:  cache.New[accounts.ID, error](2000, failureTTL),
		live:        cache.New[accounts.ID, struct{}](2000, liveTTL),
		hosts:       cache.New[string, bool](500, 5*time.Minute),
		indexes:     cache.New[accounts.ID, []time.Duration](200, time.Hour),
		unindexed:   cache.New[accounts.ID, error](2000, failureTTL),
		offsets:     cache.New[accounts.ID, []container.Keyframe](50, time.Hour),
		warming:     map[accounts.ID]*warmRun{},
		warmed:      cache.New[accounts.ID, time.Duration](500, warmedFor),
		extractions: cache.New[accounts.ID, *extracted](200, 6*time.Hour),
		locations:   cache.New[accounts.ID, []int](2000, time.Hour),
		unlocated:   cache.New[accounts.ID, error](2000, failureTTL),
		tracks:      cache.New[trackKey, Track](100, time.Hour).Sized(maxKeptTracks, func(t Track) int { return len(t.Data) }),
		untracked:   cache.New[trackKey, error](2000, failureTTL),
		trackReads:  make(chan struct{}, maxTrackReads),
		rangeHosts:  cache.New[string, bool](500, 24*time.Hour),
		trackCues:   cache.New[trackKey, []subtitles.Cue](50, time.Hour),
		attached:    cache.New[accounts.ID, []container.Attachment](8, 10*time.Minute),
		speech:      newSpeechState(),
	}
	server.live = s.ServeLive
	return s, nil
}

// Close stops the source server.
func (s *Service) Close() error {
	return s.loopback.Close()
}

// open opens a version's source in the cache, renewing its link when it
// expires. The caller releases it.
func (s *Service) open(version library.Version) *source.Source {
	if version.Torrent != nil {
		src := s.sources.Open(version.ID, s.torrentLocation(version), nil)
		// A torrent that failed for its route is tried again as soon as the
		// route is back, not after the source's cooldown.
		if failure := src.Failure(); failure != nil && torrent.RouteFailure(failure) && s.torrentReady() == nil {
			src.Retry()
		}
		return src
	}
	var renew source.Renewer
	if s.renew != nil {
		renew = func(ctx context.Context) (source.Location, error) {
			fresh, err := s.renew(ctx, version)
			return locationOf(fresh), err
		}
	}
	if location, ok := s.storedLocation(version); ok {
		return s.sources.Open(version.ID, location, nil)
	}
	return s.sources.Open(version.ID, locationOf(version), renew)
}

// storedLookup bounds finding where a local file is read from (see
// LocalSources): one query, for reads that have their own deadlines.
const storedLookup = 10 * time.Second

// storedLocation is where the source cache reads a version of a local
// folder read straight from its storage (see LocalSources).
func (s *Service) storedLocation(version library.Version) (source.Location, bool) {
	key, ok := strings.CutPrefix(version.URL, s.localBase())
	if !ok || key == "" {
		return source.Location{}, false
	}
	s.loopback.mu.Lock()
	find := s.loopback.stored
	s.loopback.mu.Unlock()
	if find == nil {
		return source.Location{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), storedLookup)
	defer cancel()
	return find(ctx, key)
}

// locationOf is where a version's bytes are, with the size its addon
// announced: the source cache refuses another file at the first byte once
// the addon's sizes prove right.
func locationOf(version library.Version) source.Location {
	return source.Location{URL: version.URL, Headers: version.Headers, Confined: version.Confined, Size: version.Size,
		Announcer: version.Addon}
}

// Torrents has the source cache read the versions that are torrents'
// files from engine.
func (s *Service) Torrents(engine *torrent.Engine) {
	s.torrents = engine
}

// EndTorrent ends the linger of the torrent of infoHash, as versions name
// it: a playback of another title took the place of the one that read it
// (see torrent.Engine.End).
func (s *Service) EndTorrent(infoHash string) {
	if hash, err := torrent.ParseHash(infoHash); err == nil && s.torrents != nil {
		s.torrents.End(hash)
	}
}

// TorrentRunning reports whether the torrent of infoHash, as versions name
// it, runs: a playback joined it, and it did not stop.
func (s *Service) TorrentRunning(infoHash string) bool {
	hash, err := torrent.ParseHash(infoHash)
	return err == nil && s.torrents != nil && s.torrents.Running(hash)
}

// TorrentSettings are the torrent engine's settings among the server's.
func TorrentSettings(s accounts.Settings) torrent.Settings {
	return torrent.Settings{Enabled: s.TorrentStreams, Interface: s.TorrentInterface, Proxy: s.TorrentProxy,
		ProxyUser: s.TorrentProxyUser, ProxyPassword: s.TorrentProxyPassword, WireGuard: s.TorrentWireGuard,
		DownloadMbps: s.TorrentDownloadMbps, UploadMbps: s.TorrentUploadMbps}
}

// errNoTorrents reports a torrent's file read without an engine.
var errNoTorrents = errors.New("this server plays no torrent")

// torrentLocation is where the source cache reads a torrent's file: from
// its swarm, which the playbacks reading it join (see source.Location's
// Playing) and nothing else. A swarm is given longer than a host to
// answer (see torrent.StartTimeout and torrent.PieceTimeout): the engine
// tells why a torrent did not start before the cache stops waiting.
// Without the file's index, the stream's filename and the episode tell
// the file (see torrent.Info.File).
func (s *Service) torrentLocation(version library.Version) source.Location {
	spec, err := torrent.NewSpec(version.Torrent.InfoHash, version.Torrent.FileIdx, version.Torrent.Sources)
	spec.Name, spec.Season, spec.Episode = version.Filename, version.Torrent.Season, version.Torrent.Episode
	if err == nil && s.torrents == nil {
		err = errNoTorrents
	}
	engine := s.torrents
	location := source.Location{URL: version.URL, Size: version.Size, Announcer: version.Addon,
		HeaderTimeout: torrentHeaders, StallTimeout: torrent.PieceTimeout,
		Opener: source.FileOpener(func(ctx context.Context, _ string) (source.RandomFile, int64, error) {
			if err != nil {
				return nil, 0, err
			}
			r, err := engine.Open(ctx, spec)
			var unstarted *torrent.StartError
			if errors.As(err, &unstarted) {
				// The swarm was waited for as long as a start may take.
				return nil, 0, source.Final(err)
			}
			if err != nil {
				return nil, 0, err
			}
			return r, r.Size(), nil
		})}
	if err == nil {
		location.Playing = func() func() { return engine.Play(spec) }
	}
	return location
}

// torrentHeaders bounds the wait for a torrent's file to open, past the
// engine's own bound on its start, and torrentHead the wait for its first
// bytes, which an analysis waits for before ffprobe's timeout runs.
const (
	torrentHeaders = torrent.StartTimeout + 15*time.Second
	torrentHead    = torrentHeaders + torrent.PieceTimeout
)

// TorrentUnstarted reports whether err tells of a torrent that could not
// start: its route cannot carry it, torrents are off, or no peer sent it
// in time. It may play later: a play finding nothing else answers that
// the title cannot be played now, rather than that it suits no app.
func TorrentUnstarted(err error) bool {
	var unstarted *torrent.StartError
	return torrent.RouteFailure(err) || errors.As(err, &unstarted) || errors.Is(err, errNoTorrents)
}

// Unstarted reports whether a version is kept as failed for a torrent
// that could not start (see TorrentUnstarted).
func (s *Service) Unstarted(version accounts.ID) bool {
	failure, failed := s.failures.Get(version)
	return failed && TorrentUnstarted(failure.err)
}

// routeCheck bounds the check of the torrents' route before a read.
const routeCheck = 10 * time.Second

// torrentReady reports why torrents cannot play now, nil when they can:
// a torrent version is skipped meanwhile, as one that cannot be read,
// without that counting as its failure.
func (s *Service) torrentReady() error {
	if s.torrents == nil {
		return errNoTorrents
	}
	ctx, cancel := context.WithTimeout(context.Background(), routeCheck)
	defer cancel()
	return s.torrents.Ready(ctx)
}

// ErrTorrentIdle reports background work on a torrent's file no playback
// reads now: only a playback joins a torrent's swarm, so the version is
// read once played. It is not the version's failure.
var ErrTorrentIdle = errors.New("a torrent's file is read only while it plays")

// idleTorrent reports background work on a torrent's file whose swarm no
// playback joined.
func (s *Service) idleTorrent(ctx context.Context, version library.Version) bool {
	if version.Torrent == nil || !background(ctx) {
		return false
	}
	hash, err := torrent.ParseHash(version.Torrent.InfoHash)
	return err != nil || s.torrents == nil || !s.torrents.Running(hash)
}

// OpenSource opens a version's source as playback reads it, through the
// shared cache, renewing its link when it expires. The caller releases it.
func (s *Service) OpenSource(version library.Version) *source.Source {
	return s.open(version)
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
// while, unless its host held the reads back (see rateLimited). The
// keyframe index of a Matroska file is read meanwhile, as an HLS play
// reads it next (see indexAhead). An analysis for a playback, ctx not
// marked Background, reads its source as a playback, even when it joins
// one started in the background; a torrent's playback joins its swarm
// before anything reads it, the index read ahead included. A version read
// from its input is analyzed there (see LocalInputs).
func (s *Service) Analyze(ctx context.Context, version library.Version) (media.Analysis, error) {
	if version.Torrent != nil {
		// Over a route that cannot carry it, a torrent's file is not read:
		// it plays again once the route is back.
		if err := s.torrentReady(); err != nil {
			return media.Analysis{}, err
		}
	}
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return analysis, nil
	}
	if s.ReadsInput(version) {
		return s.analyzeInput(ctx, version)
	}
	if s.idleTorrent(ctx, version) {
		return media.Analysis{}, ErrTorrentIdle
	}
	if version.Torrent != nil && !background(ctx) {
		src := s.open(version)
		defer src.Release()
		defer src.Urge()()
	}
	if _, failed := s.failures.Get(version.ID); !failed {
		s.indexAhead(version)
	}
	return s.analyze(ctx, version, media.Prober.Probe)
}

// analyze is Analyze, the version read with probe. ffprobe is stopped as
// soon as the version's source fails, rather than left to its deadline,
// and the source's failure is the analysis's: what is kept as the
// version's failure tells what its host answered. A version that holds
// one of its source's connections takes it meanwhile (see Hold): the
// source's connections all in use fail the analysis, which is not kept as
// the version's failure.
func (s *Service) analyze(ctx context.Context, version library.Version, probe func(media.Prober, context.Context, string) (media.Analysis, error)) (media.Analysis, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return analysis, nil
	}
	if failure, failed := s.failures.Get(version.ID); failed {
		return media.Analysis{}, failure.err
	}
	if s.idleTorrent(ctx, version) {
		return media.Analysis{}, ErrTorrentIdle
	}
	release, err := s.Hold(ctx, version)
	if err != nil {
		return media.Analysis{}, err
	}
	defer release()
	if !background(ctx) {
		src := s.open(version)
		defer src.Release()
		defer src.Urge()()
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
		probing, stop := context.WithCancel(ctx)
		failed := src.Failed()
		go func() {
			select {
			case <-failed:
				stop()
			case <-probing.Done():
			}
		}()
		var analysis media.Analysis
		var err error
		if version.HoldsConnection {
			err = notPlaylist(probing, src)
		}
		if version.Torrent != nil {
			// A torrent's swarm takes longer to start than ffprobe's timeout
			// allows: its first bytes are waited for first.
			head, cancel := context.WithTimeout(probing, torrentHead)
			_, err = src.Size(head)
			cancel()
		}
		if err == nil {
			analysis, err = probe(s.ffprobe(), probing, target)
		}
		stop()
		if err != nil {
			if failure := failureOf(src, failed); failure != nil {
				err = failure
			}
		}
		if err == nil && standIn(analysis, version.Runtime) {
			err = fmt.Errorf("%w: %s long, where the title lasts %s", ErrStandIn, analysis.Duration.Round(time.Second), version.Runtime)
		}
		if err != nil {
			if !rateLimited(src, started, err) {
				s.fail(version, err)
			}
			return nil, err
		}
		if analysis.Size == 0 {
			analysis.Size = version.Size
		}
		// A replay's MPEG-TS file, read from an IPTV archive that tells not
		// its length, lasts as long as its programme.
		if analysis.Duration <= 0 && version.HoldsConnection && version.Runtime > 0 && transportStream(analysis) {
			analysis.Duration = version.Runtime
		}
		return s.keepAnalysis(ctx, version, analysis, started), nil
	})
	if err != nil {
		return media.Analysis{}, err
	}
	return result.(media.Analysis), nil
}

// keepAnalysis saves and remembers the analysis of a version, started at
// a time, and returns it.
func (s *Service) keepAnalysis(ctx context.Context, version library.Version, analysis media.Analysis, started time.Time) media.Analysis {
	data, err := json.Marshal(analysis)
	if err == nil {
		_, err = s.db.Exec(ctx, `INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)
			ON CONFLICT (version_id) DO UPDATE SET analysis = excluded.analysis, analyzed_at = now()`, version.ID, data)
	}
	if err != nil {
		s.logger.Warn("Saving a media analysis failed", "error", err)
	}
	s.analyses.Put(version.ID, analysis)
	s.logger.Debug("Analyzed a version", "addon", version.Addon, "duration", time.Since(started))
	return analysis
}

// notPlaylist fails with ErrPlaylistReplay when src starts as an HLS
// playlist does. A source that cannot be read is left to the analysis,
// which tells why.
func notPlaylist(ctx context.Context, src *source.Source) error {
	head := make([]byte, 16)
	n, err := src.ReadAt(ctx, head, 0)
	if n == 0 && err != nil {
		return nil
	}
	if bytes.HasPrefix(bytes.TrimPrefix(head[:n], []byte("\xef\xbb\xbf")), []byte("#EXTM3U")) {
		return ErrPlaylistReplay
	}
	return nil
}

// failureOf is why src failed, when failed, its Failed channel, closed.
func failureOf(src *source.Source, failed <-chan struct{}) error {
	select {
	case <-failed:
		if failure := src.Failure(); failure != nil {
			return failure
		}
		return source.ErrUnavailable
	default:
		return nil
	}
}

// rateLimited reports whether err, met reading src since a time, comes of
// its host's limits rather than of the file: the host asked to slow down
// (429, 503), or, for what is not the source's own failure, as an
// analysis out of time, the reads were held back meanwhile. A version is
// not kept as failed for it.
func rateLimited(src *source.Source, since time.Time, err error) bool {
	return errors.Is(err, source.ErrSlowDown) || !errors.Is(err, source.ErrUnavailable) && src.HeldBack(since)
}

// backgroundKey marks a context as background work's (see Background).
type backgroundKey struct{}

// Background marks ctx as the context of background work: reading ahead
// of a play to come, or analyzing a version a play may not choose. The
// sources it reads wait for their hosts' other connections, and give way
// to the playbacks' (see source.Source.Urge).
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

// background reports whether ctx is background work's.
func background(ctx context.Context) bool {
	marked, _ := ctx.Value(backgroundKey{}).(bool)
	return marked
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

// versionFailure is why a version could not be analyzed or reached, the
// addon that listed it and its item.
type versionFailure struct {
	addon, item accounts.ID
	err         error
}

// fail keeps err as why a version could not be analyzed or reached: it is
// not tried again for failureTTL, unless its addon changes or its item is
// tried again (see Retry and RetryItem).
func (s *Service) fail(version library.Version, err error) {
	if torrent.RouteFailure(err) {
		// The torrents' route failed, not the file: it plays once the
		// route is back.
		return
	}
	s.failures.Put(version.ID, versionFailure{addon: version.Origin.Addon, item: version.Item, err: err})
}

// Retry forgets the failures of the versions an addon listed, so that they
// are tried again at once: those of a local folder whose address,
// credentials or host key changed, which may have failed only for them.
func (s *Service) Retry(addon accounts.ID) {
	s.failures.DeleteFunc(func(_ accounts.ID, failure versionFailure) bool { return failure.addon == addon })
}

// RetryItem forgets the failures of the versions an addon listed for an
// item, so that they are tried again at once: those of a channel of an
// IPTV source whose streams an administrator asked to try again.
func (s *Service) RetryItem(addon, item accounts.ID) {
	s.failures.DeleteFunc(func(_ accounts.ID, failure versionFailure) bool {
		return failure.addon == addon && failure.item == item
	})
}

// ffprobe is the prober, an analysis bounded by the settings' timeout when
// it starts: a change applies to the next analysis.
func (s *Service) ffprobe() media.Prober {
	prober := s.prober
	prober.Timeout = time.Duration(s.settings().AnalysisTimeout) * time.Second
	return prober
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
// the delivery says so, Polyfin relays the bytes, as it always does those
// of a version that holds one of its source's connections (see Hold). An
// expired link is renewed once. A version that is not one file is refused
// with ErrNoFile.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request, version library.Version, delivery Delivery) error {
	if s.ReadsInput(version) {
		http.Error(w, "not one file", http.StatusNotFound)
		return ErrNoFile
	}
	if !delivery.Relay && !version.HoldsConnection && version.Torrent == nil && len(version.Headers) == 0 && s.public(r.Context(), version.URL) {
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
// link that answered. A source that answered the server's own reads at
// that link lately, as PlaybackInfo's analysis, is not asked again. A
// link that could not be renewed, or a host asking to slow down (429,
// 503), is not kept as the file's failure: that tells nothing of the file.
func (s *Service) check(ctx context.Context, version library.Version) (library.Version, error) {
	if _, ok := s.live.Get(version.ID); ok {
		return version, nil
	}
	if answered := s.sources.Answered(version.ID, version.URL); time.Since(answered) < liveTTL {
		return version, nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	status, err := s.firstByte(ctx, version)
	if err == nil && expired(status) && s.renew != nil {
		fresh, renewErr := s.renew(ctx, version)
		if renewErr != nil {
			return library.Version{}, fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrSourceUnavailable, status, renewErr)
		}
		version = fresh
		status, err = s.firstByte(ctx, version)
	}
	if err == nil && status != http.StatusOK && status != http.StatusPartialContent {
		err = fmt.Errorf("%w: HTTP %d", ErrSourceUnavailable, status)
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			return library.Version{}, err
		}
	}
	if err != nil {
		s.fail(version, err)
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
