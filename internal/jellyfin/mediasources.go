package jellyfin

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// MediaSourceInfo is a version of a video as Jellyfin describes it. Field
// order follows Jellyfin's.
type MediaSourceInfo struct {
	Protocol                            string
	Id                                  string
	Path                                string `json:",omitempty"`
	Type                                string
	Container                           string
	Size                                *int64 `json:",omitempty"`
	Name                                string
	IsRemote                            bool
	ETag                                string
	RunTimeTicks                        *int64 `json:",omitempty"`
	ReadAtNativeFramerate               bool
	IgnoreDts                           bool
	IgnoreIndex                         bool
	GenPtsInput                         bool
	SupportsTranscoding                 bool
	SupportsDirectStream                bool
	SupportsDirectPlay                  bool
	IsInfiniteStream                    bool
	UseMostCompatibleTranscodingProfile bool
	RequiresOpening                     bool
	RequiresClosing                     bool
	RequiresLooping                     bool
	SupportsProbing                     bool
	VideoType                           string `json:",omitempty"`
	HasSegments                         bool
	MediaStreams                        []playback.MediaStream
	MediaAttachments                    []MediaAttachment
	Formats                             []string
	Bitrate                             *int64 `json:",omitempty"`
	// RequiredHttpHeaders is always empty: players ignore it, and sources
	// that need headers are relayed.
	RequiredHttpHeaders        map[string]string
	TranscodingUrl             string `json:",omitempty"`
	TranscodingSubProtocol     string
	TranscodingContainer       string `json:",omitempty"`
	DefaultAudioStreamIndex    *int   `json:",omitempty"`
	DefaultSubtitleStreamIndex *int   `json:",omitempty"`
}

// playable is a movie or an episode with its versions and the subtitle
// files addons offer for it, and the track preferences of the user it is
// described for.
type playable struct {
	item      library.Item
	versions  []library.Version
	subtitles []library.ExternalSubtitle
	tracks    trackPreferences
}

// playable gathers a title's versions and subtitles, both asked of the
// addons at once. Versions that recently failed are left out; so are those
// taller than the user's quality group, while one fits (see inGroup), and
// subtitles that cannot be listed.
func (h *Handler) playable(ctx context.Context, user accounts.User, item library.Item) (playable, error) {
	p := playable{item: item, tracks: h.trackPreferences(ctx, user)}
	if item.Kind == library.KindRecording {
		p.versions = h.recordingVersions(ctx, item)
		return p, nil
	}
	var versionsErr error
	var wg sync.WaitGroup
	wg.Go(func() { p.versions, versionsErr = h.Library.Versions(ctx, user, item.ID) })
	wg.Go(func() {
		if subtitles, err := h.Library.Subtitles(ctx, user, item.ID); err == nil {
			p.subtitles = subtitles
		}
	})
	wg.Wait()
	p.versions = h.inGroup(ctx, user, slices.DeleteFunc(p.versions, func(v library.Version) bool { return h.Playback.Failed(v.ID) }))
	h.subtitleFiles.Put(item.ID, p.subtitles)
	return p, versionsErr
}

// cachedPlayable is what listings show of a title's versions: only what is
// already known, as asking addons for every listed title is too costly.
// Like playable, it leaves out versions taller than the user's group.
func (h *Handler) cachedPlayable(ctx context.Context, user accounts.User, item library.Item) playable {
	p := playable{item: item, tracks: h.trackPreferences(ctx, user)}
	if item.Kind == library.KindRecording {
		p.versions = h.recordingVersions(ctx, item)
		return p
	}
	if versions, ok := h.Library.CachedVersions(ctx, user, item.ID); ok {
		p.versions = h.inGroup(ctx, user, slices.DeleteFunc(versions, func(v library.Version) bool { return h.Playback.Failed(v.ID) }))
	}
	p.subtitles, _ = h.subtitleFiles.Get(item.ID)
	return p
}

func (p playable) externals() []playback.ExternalSubtitle {
	result := make([]playback.ExternalSubtitle, len(p.subtitles))
	for i, subtitle := range p.subtitles {
		// Addon files are served as SubRip whatever they are; a file a user
		// added keeps its format, as Jellyfin names its external files.
		codec := "subrip"
		if subtitle.Uploaded {
			codec = uploadedCodecs[subtitle.Format]
		}
		result[i] = playback.ExternalSubtitle{Language: subtitle.Language, Codec: codec,
			Forced: subtitle.Forced, HearingImpaired: subtitle.HearingImpaired}
	}
	return result
}

// ordered puts the version an item was opened as first; an item opened by
// its own identifier keeps the addons' order.
func (p playable) ordered(opened accounts.ID) []library.Version {
	i := slices.IndexFunc(p.versions, func(v library.Version) bool { return v.ID == opened })
	if i <= 0 {
		return p.versions
	}
	versions := make([]library.Version, 0, len(p.versions))
	versions = append(versions, p.versions[i])
	versions = append(versions, p.versions[:i]...)
	return append(versions, p.versions[i+1:]...)
}

// sourceID names a version as a media source. The first version takes the
// identifier the item was opened with, as Jellyfin names a single version
// after its item and apps look for the source carrying the item's id.
func sourceID(opened accounts.ID, version library.Version, first bool) accounts.ID {
	if first {
		return opened
	}
	return version.ID
}

// mediaSources describes an item's versions for item details and listings,
// where Jellyfin evaluates no device profile: every version is playable.
// opened is the identifier the item was asked by.
func (h *Handler) mediaSources(r *http.Request, p playable, opened accounts.ID) []MediaSourceInfo {
	versions := p.ordered(opened)
	sources := make([]MediaSourceInfo, len(versions))
	for i, version := range versions {
		sources[i] = h.describedSource(r, p, version, sourceID(opened, version, i == 0))
	}
	return sources
}

// describedSource describes a version as item details do, without a
// device profile: playable as it is, its tracks as far as it was analyzed.
func (h *Handler) describedSource(r *http.Request, p playable, version library.Version, id accounts.ID) MediaSourceInfo {
	analysis, analyzed := h.Playback.Analyzed(r.Context(), version.ID)
	source := h.baseSource(r, p, version, id, analysis, analyzed)
	source.SupportsDirectPlay, source.SupportsDirectStream = true, true
	if analyzed {
		source.Container = playback.DisplayContainer(analysis, version.Filename)
		source.DefaultAudioStreamIndex = p.tracks.audio(source.MediaStreams)
	}
	source.DefaultSubtitleStreamIndex = p.tracks.subtitle(source.MediaStreams, source.DefaultAudioStreamIndex)
	return source
}

// placeholderSource stands for a title's versions in a listing when they are
// not known yet: its identifier is the item's, which plays the first
// version.
func (h *Handler) placeholderSource(r *http.Request, item library.Item) MediaSourceInfo {
	version := library.Version{ID: item.ID, Item: item.ID, Name: item.Name, Runtime: item.Runtime}
	source := h.baseSource(r, playable{item: item}, version, item.ID, media.Analysis{}, false)
	source.SupportsDirectPlay, source.SupportsDirectStream = true, true
	return source
}

// baseSource fills what every description of a version shares.
func (h *Handler) baseSource(r *http.Request, p playable, version library.Version, id accounts.ID, analysis media.Analysis, analyzed bool) MediaSourceInfo {
	language := h.Accounts.Settings().Language
	source := MediaSourceInfo{
		Protocol:               "Http",
		Id:                     id.String(),
		Type:                   "Default",
		Container:              containerOfName(version.Filename),
		Name:                   version.Name,
		IsRemote:               true,
		ETag:                   version.ID.String(),
		SupportsProbing:        true,
		VideoType:              "VideoFile",
		MediaAttachments:       []MediaAttachment{},
		Formats:                []string{},
		RequiredHttpHeaders:    map[string]string{},
		TranscodingSubProtocol: "http",
	}
	source.Path = h.streamURL(r, p.item.ID, id, version, source.Container, mustRelay(r, version))
	size, runtime := version.Size, version.Runtime
	if analyzed {
		source.MediaStreams = playback.MediaStreams(analysis, p.externals(), language)
		source.MediaAttachments = mediaAttachments(analysis)
		size = cmp.Or(analysis.Size, size)
		runtime = cmp.Or(analysis.Duration, runtime)
		if analysis.Bitrate > 0 {
			source.Bitrate = new(analysis.Bitrate)
		}
	} else {
		source.MediaStreams = playback.ExternalStreams(p.externals(), language)
	}
	if size > 0 {
		source.Size = new(size)
	}
	if runtime > 0 {
		source.RunTimeTicks = new(int64(runtime / 100))
		if source.Bitrate == nil && size > 0 {
			source.Bitrate = new(int64(float64(size*8) / runtime.Seconds()))
		}
	}
	return source
}

// streamURL is the address players fetch a version from when they use a
// media source's Path. Players send no credentials with it, so it carries a
// grant signed for the caller, which tells whether Polyfin must relay the
// source rather than redirect the player to it.
func (h *Handler) streamURL(r *http.Request, item, sourceID accounts.ID, version library.Version, container string, relay bool) string {
	user := callerFrom(r.Context()).User
	grant := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID, Relay: relay})
	query := url.Values{
		"static":        {"true"},
		"mediaSourceId": {sourceID.String()},
		"Tag":           {version.ID.String()},
		grantParameter:  {grant},
	}
	extension := container
	if container == "hls" {
		// Players recognize HLS by the extension.
		extension = "m3u8"
	}
	return baseURL(r) + "/Videos/" + item.String() + "/stream." + extension + "?" + query.Encode()
}

// grantParameter carries a signed grant in the URLs Polyfin makes.
const grantParameter = "PolyfinGrant"

// mustRelay reports whether a player cannot follow a redirect to the
// version's source. Findroid's ExoPlayer refuses redirects from http to
// https, so a server reached over http relays https sources to it.
func mustRelay(r *http.Request, version library.Version) bool {
	device := callerFrom(r.Context()).Device
	return strings.Contains(strings.ToLower(device.Client), "findroid") &&
		requestScheme(r) == "http" && strings.HasPrefix(version.URL, "https:")
}

// baseURL is the address the player reached Polyfin at.
func baseURL(r *http.Request) string {
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host, _, _ = strings.Cut(forwarded, ",")
	}
	return requestScheme(r) + "://" + strings.TrimSpace(host)
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return "https"
	}
	return "http"
}

// containerOfName guesses a container from a file name until the version
// is analyzed.
func containerOfName(filename string) string {
	switch strings.ToLower(strings.TrimPrefix(path.Ext(filename), ".")) {
	case "mp4", "m4v":
		return "mp4"
	case "webm":
		return "webm"
	case "avi":
		return "avi"
	case "mov":
		return "mov"
	case "ts", "m2ts", "mts":
		return "ts"
	case "m3u8":
		return "hls"
	default:
		return "mkv"
	}
}

// subtitleURL is a subtitle's DeliveryUrl: relative, with the caller's
// token, as Jellyfin writes it.
func subtitleURL(r *http.Request, item, sourceID accounts.ID, index int, format string) string {
	target := "/Videos/" + hyphenated(item) + "/" + sourceID.String() + "/Subtitles/" + strconv.Itoa(index) + "/0/Stream." + format
	if token := callerFrom(r.Context()).Token; token != "" {
		target += "?ApiKey=" + url.QueryEscape(token)
	}
	return target
}
