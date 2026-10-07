// Package remuxdb describes the versions of movies and episodes that were
// never analyzed, from RemuxDB: a community database of what probing
// release files found in them, their tracks, length and size. It asks
// RemuxDB about a title by its IMDb identifier, and finds each version
// among the files RemuxDB lists by the version's file name and size.
package remuxdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

const (
	// describedFor keeps what RemuxDB said of a version, whether it knew
	// its file or not: it learns of more files as they are probed, but a
	// title is rarely opened again within hours.
	describedFor = 6 * time.Hour
	// failedFor leaves RemuxDB unasked for a while after it failed to
	// answer, so that item details do not wait on a server that is down.
	failedFor = time.Minute
	// maxPause bounds how long a Retry-After leaves RemuxDB unasked.
	maxPause = time.Hour
	// askTimeout bounds each request to RemuxDB. Its answer is kept even
	// once item details stopped waiting for it (see describeWait).
	askTimeout = 10 * time.Second
	// describeWait is how long item details wait for RemuxDB's answer: one
	// that comes later describes the versions at the next request.
	describeWait = time.Second
	// maxAnswer bounds the bytes read of an answer: a popular movie's takes
	// a few MB, most of them its files' chapters, which are not read.
	maxAnswer = 32 << 20
	// keptDescriptions bounds the versions whose descriptions are kept,
	// and keptBytes the memory they take.
	keptDescriptions = 20000
	keptBytes        = 32 << 20
	// sizeSlack is how far the size an addon gives a file may be from its
	// actual size: some round it to a tenth of a GiB.
	sizeSlack = 64 << 20
)

// Service describes versions from RemuxDB, while the settings turn it on.
// A nil Service describes none.
type Service struct {
	// settings turn RemuxDB on and give its address, read at each request
	// so that a change applies at once.
	settings  func() accounts.Settings
	client    *http.Client
	userAgent string
	// clientID names the server to RemuxDB, which requires a name.
	clientID string
	logger   *slog.Logger
	now      func() time.Time
	// wait is how long Describe waits for RemuxDB (see describeWait).
	wait time.Duration
	// descriptions holds what RemuxDB said lately of each version.
	descriptions *cache.Cache[accounts.ID, description]

	mu sync.Mutex
	// asking holds the titles RemuxDB is being asked about.
	asking map[string]*lookup
	// pausedUntil is when RemuxDB may be asked again after it failed.
	pausedUntil time.Time
}

// description is what RemuxDB said of a version: the analysis of its file
// when it knew it.
type description struct {
	Analysis media.Analysis
	Found    bool
}

// lookup is a request to RemuxDB about a title under way, with the
// versions its answer describes: those of every Describe that joined it.
type lookup struct {
	versions []library.Version
	done     chan struct{}
}

// New returns a service asking RemuxDB as settings tell. serverID is the
// server's identifier: RemuxDB is given a name derived from it, which
// tells this server's requests apart without being the identifier apps
// see.
func New(serverID, version string, logger *slog.Logger, settings func() accounts.Settings) *Service {
	sum := sha256.Sum256([]byte("polyfin remuxdb client " + serverID))
	return &Service{
		settings:     settings,
		client:       &http.Client{},
		userAgent:    "Polyfin/" + version,
		clientID:     hex.EncodeToString(sum[:16]),
		logger:       logger,
		now:          time.Now,
		wait:         describeWait,
		descriptions: cache.New[accounts.ID, description](keptDescriptions, describedFor).Sized(keptBytes, cache.JSONSize[description]),
		asking:       map[string]*lookup{},
	}
}

// Describe asks RemuxDB about the titles of those of versions it was not
// asked about lately, and describes them from its answers (see Described).
// It waits for the answers describeWait at most, or until ctx is done: a
// later answer describes the versions once it comes. RemuxDB is not asked
// while the settings turn it off, nor for a while after it failed.
func (s *Service) Describe(ctx context.Context, versions []library.Version) {
	if s == nil {
		return
	}
	settings := s.settings()
	if !settings.RemuxDB {
		return
	}
	titles := map[string][]library.Version{}
	for _, version := range versions {
		if _, kept := s.descriptions.Get(version.ID); kept {
			continue
		}
		if title, ok := mediaID(version.Origin.ID); ok {
			titles[title] = append(titles[title], version)
		}
	}
	if len(titles) == 0 {
		return
	}
	s.mu.Lock()
	if s.now().Before(s.pausedUntil) {
		s.mu.Unlock()
		return
	}
	answered := make([]chan struct{}, 0, len(titles))
	for title, versions := range titles {
		l, asking := s.asking[title]
		if !asking {
			l = &lookup{done: make(chan struct{})}
			s.asking[title] = l
			go s.run(title, settings.RemuxDBURL, l)
		}
		l.versions = append(l.versions, versions...)
		answered = append(answered, l.done)
	}
	s.mu.Unlock()
	timer := time.NewTimer(s.wait)
	defer timer.Stop()
	for _, done := range answered {
		select {
		case <-done:
		case <-timer.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Described returns RemuxDB's description of a version, as Describe got
// it: an analysis of its file without its chapters, attachments nor what
// only playback needs, such as time bases. It never asks RemuxDB.
func (s *Service) Described(version library.Version) (media.Analysis, bool) {
	if s == nil || !s.settings().RemuxDB {
		return media.Analysis{}, false
	}
	kept, ok := s.descriptions.Get(version.ID)
	return kept.Analysis, ok && kept.Found
}

// run asks RemuxDB at base about title, then describes the versions of l
// from its answer. When RemuxDB fails, none is described: they are asked
// about again once RemuxDB may be.
func (s *Service) run(title, base string, l *lookup) {
	defer close(l.done)
	started := s.now()
	ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
	files, err := s.fetch(ctx, base, title)
	cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.asking, title)
	if err != nil {
		s.pause(title, err)
		return
	}
	described := 0
	for _, version := range l.versions {
		d := describe(files, version)
		if d.Found {
			described++
		}
		s.descriptions.Put(version.ID, d)
	}
	s.logger.Debug("RemuxDB described versions", "title", title, "files", len(files), "versions", len(l.versions),
		"described", described, "took", s.now().Sub(started))
}

// pause leaves RemuxDB unasked for failedFor after it failed to answer
// about title, or as long as the Retry-After of a 429 says.
func (s *Service) pause(title string, err error) {
	until := s.now().Add(failedFor)
	var limited *rateLimited
	if errors.As(err, &limited) {
		until = limited.until
	}
	s.pausedUntil = until
	s.logger.Info("RemuxDB did not answer", "title", title, "error", err, "until", until)
}

// rateLimited reports a 429 with a Retry-After, and when RemuxDB may be
// asked again.
type rateLimited struct {
	until time.Time
}

func (e *rateLimited) Error() string {
	return "HTTP 429 until " + e.until.Format(time.RFC3339)
}

// fetch reads what RemuxDB at base knows of title: the files it probed of
// it, none when it knows nothing of it.
func (s *Service) fetch(ctx context.Context, base, title string) ([]file, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/media/"+url.PathEscape(title)+"/versions", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", s.userAgent)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Client-Id", s.clientID)
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	switch status := response.StatusCode; {
	case status == http.StatusNotFound:
		return nil, nil
	case status == http.StatusTooManyRequests:
		if until, ok := retryAfter(response.Header.Get("Retry-After"), s.now()); ok {
			return nil, &rateLimited{until: until}
		}
		return nil, fmt.Errorf("HTTP %d", status)
	case status != http.StatusOK:
		return nil, fmt.Errorf("HTTP %d", status)
	}
	var files []file
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(&files); err != nil {
		return nil, fmt.Errorf("read the answer: %w", err)
	}
	return files, nil
}

// retryAfter reads a Retry-After header, in seconds or as a date, into when
// RemuxDB may be asked again, at most maxPause after now.
func retryAfter(value string, now time.Time) (time.Time, bool) {
	var until time.Time
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		until = now.Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(value); err == nil {
		until = date
	} else {
		return time.Time{}, false
	}
	if latest := now.Add(maxPause); until.After(latest) {
		until = latest
	}
	return until, true
}

// titlePattern matches the identifiers RemuxDB knows titles by: a movie's
// IMDb identifier, or an episode's, its series' followed by its season and
// episode numbers ("tt0903747:1:1"), as addons are asked for streams.
var titlePattern = regexp.MustCompile(`^tt[0-9]{1,10}(:[0-9]{1,4}:[0-9]{1,4})?$`)

// mediaID is the identifier RemuxDB knows the title addons were asked
// streams for by id. Titles addons know by other identifiers are not
// asked about.
func mediaID(id string) (string, bool) {
	return id, titlePattern.MatchString(id)
}

// file is a file RemuxDB probed: what it found in it, and the names it was
// found under. Its tracks are read only for the files versions match.
type file struct {
	// Container is ffprobe's name for the file's format.
	Container string `json:"container"`
	// Duration is in seconds, Bitrate in bits per second.
	Duration float64         `json:"duration"`
	Size     int64           `json:"size"`
	Bitrate  int64           `json:"bitrate"`
	Sources  []source        `json:"sources"`
	Tracks   json.RawMessage `json:"tracks"`
}

// source is where RemuxDB found a file: in a torrent or an NZB, under a
// name that may hold the folder it is in.
type source struct {
	Filename string `json:"filename"`
}

// track is a track RemuxDB found in a file, with ffprobe's names and
// numbering mostly.
type track struct {
	Kind            string  `json:"kind"`
	Index           int     `json:"idx"`
	Codec           string  `json:"codec"`
	Profile         string  `json:"profile"`
	Language        string  `json:"language"`
	Title           string  `json:"title"`
	Bitrate         int64   `json:"bit_rate"`
	BitDepth        int     `json:"bit_depth"`
	Default         bool    `json:"is_default"`
	Forced          bool    `json:"is_forced"`
	HearingImpaired bool    `json:"is_hearing_impaired"`
	External        bool    `json:"is_external"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	FrameRate       float64 `json:"fps"`
	AspectRatio     string  `json:"aspect_ratio"`
	PixelFormat     string  `json:"pixel_format"`
	Level           int     `json:"level"`
	RefFrames       int     `json:"ref_frames"`
	ColorRange      string  `json:"color_range"`
	ColorSpace      string  `json:"color_space"`
	ColorTransfer   string  `json:"color_transfer"`
	ColorPrimaries  string  `json:"color_primaries"`
	// DolbyVision is the Dolby Vision profile, 0 without.
	DolbyVision   int    `json:"dv_profile"`
	HDR10Plus     bool   `json:"hdr10_plus_present"`
	Channels      int    `json:"channels"`
	ChannelLayout string `json:"channel_layout"`
	SampleRate    int    `json:"sample_rate"`
}

// describe is what RemuxDB's files of a title tell of a version: the
// analysis of the file it is (see match), if any lists tracks.
func describe(files []file, version library.Version) description {
	f := match(files, version)
	if f == nil {
		return description{}
	}
	var tracks []track
	if json.Unmarshal(f.Tracks, &tracks) != nil {
		return description{}
	}
	analysis := f.analysis(tracks)
	if len(analysis.Streams) == 0 {
		return description{}
	}
	return description{Analysis: analysis, Found: true}
}

// match finds the file a version is among those RemuxDB probed of its
// title: a file found under the version's file name, folders left out and
// case ignored. When the addon gives the version's size, the file's must
// agree (see sameSize), the closest winning among files of the same name;
// without it, a name several files share matches none. A file too short
// to be the title, a sample, matches none either.
func match(files []file, version library.Version) *file {
	name := baseName(version.Filename)
	if name == "" {
		return nil
	}
	var found *file
	several := false
	for i := range files {
		f := &files[i]
		if !f.named(name) || f.tooShort(version.Runtime) {
			continue
		}
		if version.Size <= 0 {
			several = several || found != nil
			found = f
			continue
		}
		if sameSize(version.Size, f.Size) && (found == nil || gap(version.Size, f.Size) < gap(version.Size, found.Size)) {
			found = f
		}
	}
	if several {
		return nil
	}
	return found
}

// named reports whether the file was found under name, a base name.
func (f *file) named(name string) bool {
	for _, source := range f.Sources {
		if strings.EqualFold(baseName(source.Filename), name) {
			return true
		}
	}
	return false
}

// tooShort reports whether the file lasts under a tenth of the title's
// runtime, as analyses of stand-ins do.
func (f *file) tooShort(runtime time.Duration) bool {
	return f.Duration > 0 && runtime > 0 && seconds(f.Duration)*10 < runtime
}

// baseName is a file name without its folders: RemuxDB names some files
// with the folder they are in, where addons give only theirs.
func baseName(name string) string {
	return name[strings.LastIndexAny(name, `/\`)+1:]
}

// sameSize reports whether a file of actual bytes may be one an addon says
// takes given, which it may have rounded.
func sameSize(given, actual int64) bool {
	return actual > 0 && gap(given, actual) <= max(sizeSlack, given/100)
}

func gap(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

// analysis is the file as an analysis of it with tracks, its own: those
// RemuxDB found in another file of a torrent, as subtitle files, are not.
// It is remote, as the analyses of addons' streams are.
func (f *file) analysis(tracks []track) media.Analysis {
	analysis := media.Analysis{Format: f.Container, Duration: seconds(f.Duration), Size: f.Size, Bitrate: f.Bitrate,
		Streams: make([]media.Stream, 0, len(tracks)), Remote: true}
	for _, t := range tracks {
		if t.External {
			continue
		}
		stream := media.Stream{Index: t.Index, Type: t.Kind, Codec: codecName(t.Codec), Profile: t.Profile, Bitrate: t.Bitrate,
			Language: t.Language, Title: t.Title, Default: t.Default, Forced: t.Forced, HearingImpaired: t.HearingImpaired,
			BitDepth: t.BitDepth}
		switch t.Kind {
		case "video":
			stream.AttachedPicture = pictureCodecs[stream.Codec]
			stream.Width, stream.Height = t.Width, t.Height
			stream.FrameRate, stream.AverageRate = t.FrameRate, t.FrameRate
			stream.AspectRatio, stream.PixelFormat = t.AspectRatio, t.PixelFormat
			stream.Level, stream.RefFrames = t.Level, t.RefFrames
			stream.ColorRange, stream.ColorSpace = colorName(t.ColorRange), colorName(t.ColorSpace)
			stream.ColorTransfer, stream.ColorPrimaries = colorName(t.ColorTransfer), colorName(t.ColorPrimaries)
			stream.HDR10Plus = t.HDR10Plus
			if t.DolbyVision > 0 {
				stream.DolbyVision = &media.DolbyVision{Profile: t.DolbyVision, RPU: true, BL: true,
					Compatibility: compatibility(t.DolbyVision, stream.ColorTransfer)}
			}
		case "audio":
			stream.Channels, stream.ChannelLayout, stream.SampleRate = t.Channels, t.ChannelLayout, t.SampleRate
		case "subtitle":
		default:
			continue
		}
		analysis.Streams = append(analysis.Streams, stream)
	}
	return analysis
}

// pictureCodecs are the codecs of cover art stored as a video track.
var pictureCodecs = map[string]bool{"mjpeg": true, "png": true, "bmp": true, "gif": true, "webp": true}

// codecNames are ffprobe's names for the codecs some of RemuxDB's files
// name otherwise, as tools other than ffprobe do.
var codecNames = map[string]string{"srt": "subrip", "pgssub": "hdmv_pgs_subtitle", "tx3g": "mov_text"}

func codecName(name string) string {
	if ffprobe, ok := codecNames[name]; ok {
		return ffprobe
	}
	return name
}

// colorNames are ffprobe's names for the color properties RemuxDB names
// otherwise: "unknown" is a property ffprobe leaves out.
var colorNames = map[string]string{
	"limited": "tv", "full": "pc",
	"bt2020_nc": "bt2020nc", "bt2020_c": "bt2020c", "bt470_bg": "bt470bg", "bt470_m": "bt470m",
	"smpte170_m": "smpte170m", "smpte240_m": "smpte240m",
	"arib_std_b67": "arib-std-b67", "bt2020_10": "bt2020-10", "bt2020_12": "bt2020-12", "iec61966_2_1": "iec61966-2-1",
	"unknown": "",
}

func colorName(name string) string {
	if ffprobe, ok := colorNames[name]; ok {
		return ffprobe
	}
	return name
}

// compatibility is the signal compatibility of a Dolby Vision profile's
// base layer, which RemuxDB does not give: none for profile 5, Blu-ray's
// for 7, and for 8 and 10 that of the base layer's transfer function, PQ
// for HDR10, HLG, or SDR. [INFERENCE]: the profiles' definitions.
func compatibility(profile int, transfer string) int {
	switch profile {
	case 7:
		return 6
	case 8, 10:
		switch transfer {
		case "smpte2084":
			return 1
		case "arib-std-b67":
			return 4
		case "bt709", "bt2020-10", "bt2020-12":
			return 2
		}
	}
	return 0
}
