package library

import (
	"context"
	"crypto/sha256"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/drm"
	"github.com/moodiness/polyfin/internal/stremio"
)

// versionsTTL is how long a version listed stays known by its identifier.
const versionsTTL = 12 * time.Hour

// Version is a stream of a movie or an episode, one of its versions in
// Jellyfin apps.
type Version struct {
	// ID is stable for the same file of the same item, whatever the URL the
	// addon gives for it this time.
	ID   accounts.ID
	Item accounts.ID
	// Name is one line describing the stream, as the addon formats it.
	Name string
	URL  string
	// Headers must be sent with every request for the stream.
	Headers map[string]string
	// UserAgent is the User-Agent Polyfin's own requests for the stream
	// send when Headers name none (see RequestHeaders), empty for
	// Polyfin's: an IPTV source's default (see stremio.Stream.UserAgent).
	UserAgent string
	// Protection is how an IPTV list says the stream is encrypted, with
	// its ClearKey keys (see stremio.Stream.Protection); nil when it says
	// nothing.
	Protection *drm.Protection
	Filename   string
	Size       int64
	// Addon is the name of the addon that listed the stream.
	Addon string
	// Origin is where the stream was listed, to ask again for it when its
	// link expires.
	Origin Origin
	// Confined is true when the stream may only be fetched from public
	// addresses.
	Confined bool
	// Runtime is the item's runtime, used until the stream is analyzed.
	Runtime time.Duration
	// Height is the video height the addon's labels give the stream (see
	// LabelHeight), 0 when they give none; its analysis, once there, is
	// what counts.
	Height int
	// Labels is what the addon's labels tell of the stream's video codec,
	// dynamic range and audio languages (see ReadLabels); its analysis,
	// once there, is what counts.
	Labels Labels
	// Expires is when the link stops working, zero when the addon does
	// not say; Audio is what a music addon tells of a track's stream.
	Expires time.Time
	Audio   *AudioSource
	// HoldsConnection marks a stream that takes one of its source's
	// connections while it plays, as a channel's stream does: a replay
	// programme's, from an IPTV provider's archive.
	HoldsConnection bool
	// Subtitles are the version's own subtitle files, which it offers
	// after its title's: those beside a local file, and those the addon
	// lists with the stream (see VersionSubtitle).
	Subtitles []ExternalSubtitle
	// Torrent is the torrent whose file the version is, nil for a link:
	// URL is then the torrent's magnet link (see torrentLink), which
	// identifies it, and to which no app is sent.
	Torrent *Torrent
	// Mirrors are the same file as other addons list it, by its name and
	// size, in the order they come: links the source cache reads from
	// once this one fails (see withMirror). They are listed nowhere else.
	Mirrors []Version
}

// RequestHeaders are the headers Polyfin's own requests for the stream
// send: Headers, with UserAgent when they name no User-Agent.
func (v Version) RequestHeaders() map[string]string {
	if v.UserAgent == "" {
		return v.Headers
	}
	for name := range v.Headers {
		if strings.EqualFold(name, "User-Agent") {
			return v.Headers
		}
	}
	headers := maps.Clone(v.Headers)
	if headers == nil {
		headers = map[string]string{}
	}
	headers["User-Agent"] = v.UserAgent
	return headers
}

// Torrent is a torrent's file as an addon lists it: its info hash, the
// file's index, nil when the addon gives none, and the stream's sources,
// its trackers and DHT nodes. Season and Episode are the episode it was
// listed for, Episode 0 for another title: without an index, the file
// whose name tells that episode plays, after the one the stream's
// filename names (see torrent.Info.File).
type Torrent struct {
	InfoHash        string
	FileIdx         *int
	Sources         []string
	Season, Episode int
}

// torrentEpisode is the episode a torrent version was listed for, which
// its renewal keeps.
func (v Version) torrentEpisode() (season, episode int) {
	if v.Torrent == nil {
		return 0, 0
	}
	return v.Torrent.Season, v.Torrent.Episode
}

// listable reports whether a stream is one Polyfin may play: a link, or a
// torrent's file, listed to the users who may play torrents (see
// accounts.Settings.TorrentsAllowed).
func listable(stream stremio.Stream) bool {
	return stream.Playable() || stream.Torrent()
}

// torrentLink is the magnet link of a torrent stream's file, its index as
// BEP 53 selects it.
func torrentLink(stream stremio.Stream) string {
	link := "magnet:?xt=urn:btih:" + strings.ToLower(stream.InfoHash)
	if stream.FileIdx != nil {
		link += "&so=" + strconv.Itoa(*stream.FileIdx)
	}
	return link
}

// Origin is the addon that listed a stream and what it listed it for.
// Title is set for a stream a title's description embeds for its video
// ID (see embed), the title's Stremio identifier, of type Type.
type Origin struct {
	Addon           accounts.ID
	Type, ID, Title string
}

// ExternalSubtitle is a subtitle file offered for an item: by an addon, or
// added by a user (see UploadSubtitle); or for a version, beside a local
// file or listed with the stream (see Version.Subtitles), or inside its
// torrent (see TorrentSubtitle).
type ExternalSubtitle struct {
	ID accounts.ID
	// Language is the addon's code, usually ISO 639-2 ("fre").
	Language string
	URL      string
	Addon    string
	Confined bool
	// Uploaded is set for a file a user added, whose text Polyfin keeps
	// (see UploadedSubtitleText). A file a user added, a local file or a
	// torrent's tells its Format (srt, vtt, ass or ssa) and its flags; a
	// local file and a torrent's, a Title too.
	Uploaded        bool
	Format          string
	Title           string
	Forced          bool
	HearingImpaired bool
	Default         bool
	// Torrent is set for a file inside a version's torrent, which has no
	// URL: its text is fetched from the swarm once, then kept.
	Torrent *TorrentSubtitle
}

// TorrentSubtitle is a subtitle file inside the torrent of a version: the
// version, its torrent, and the file's index among the torrent's files.
type TorrentSubtitle struct {
	Version accounts.ID
	Torrent Torrent
	Index   int
}

// target is what to ask addons for: a title or an episode.
type target struct {
	item     accounts.ID
	kind     Kind
	metaType string
	id       string
	runtime  time.Duration
	// name is the movie's or the series' name, whose words labels may
	// repeat (see ReadLabels); videoName an episode's, or a folder's
	// video's, own name (see ReleaseKinds).
	name, videoName string
	// season and episode are the episode's, episode 0 for another title:
	// a torrent's file without an index is found by them.
	season, episode int
	// year is a movie's, 0 for another title or when unknown: a version
	// naming another year is another movie's (see OtherTitle).
	year int
	// ids are the identifiers a movie's, or an episode's series',
	// description gives, under Jellyfin's names (see providerIDs): those
	// it may be asked streams by when an addon does not accept its own
	// (see streamRequests).
	ids map[string]string
	// described is an episode's series' complete description, when the
	// caller has read it already; else streamRequests reads it.
	described *stremio.Meta
	// holder is the record of the title whose description holds the
	// video: an episode's series, or a folder's video's folder. embedded
	// are the streams it embeds for the video, described by embeddedBy,
	// nil until known, the title embeddedIn (see embed).
	holder     *record
	embedded   []stremio.Stream
	embeddedBy *installed
	embeddedIn string
}

type streamKey struct {
	addon       accounts.ID
	contentType string
	id          string
}

// target resolves the movie or episode identified by id from what Polyfin
// remembers of it, without asking addons unless the user's parental
// control needs the title's rating.
func (s *Service) target(ctx context.Context, user accounts.User, id accounts.ID) (target, view, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return target{}, view{}, err
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		// An extra plays its file, as its folder streams it.
		extra, err := s.extra(ctx, v, id)
		if err != nil {
			return target{}, view{}, err
		}
		return target{item: id, kind: extra.Kind, metaType: ExtraType, id: extra.StremioID, name: extra.Name}, v, nil
	}
	if err != nil {
		return target{}, view{}, err
	}
	if kept, _ := s.visible(ctx, v, []record{r}); len(kept) == 0 || s.withholdsHolder(ctx, v, r) {
		return target{}, view{}, ErrNotFound
	}
	switch {
	case AudioKind(r.Kind) && r.Music != nil:
		if !v.allowsMusic(r) {
			return target{}, view{}, ErrNotFound
		}
		return target{item: id, kind: r.Kind, metaType: originEclipse, id: r.Music.ID}, v, nil
	case r.folderVideo():
		folder, err := s.load(ctx, r.folderItemID())
		if err != nil || folder.Meta == nil {
			return target{}, view{}, ErrNotFound
		}
		t := target{item: id, kind: KindVideo, metaType: folder.Meta.Type, id: r.Video.ID, runtime: parseRuntime(string(r.Video.Runtime)),
			name: folder.Meta.Name, videoName: r.Video.DisplayName(), holder: &folder}
		s.embed(ctx, v, &t, false)
		return t, v, nil
	case (r.Kind == KindMovie || r.Kind == KindVideo || r.Kind == KindFolder) && r.Meta != nil:
		meta := r.Meta
		if full, _, ok := s.describedMeta(ctx, v, r, false, inDatabase); ok {
			meta = &full
		}
		t := target{item: id, kind: r.Kind, metaType: meta.Type, id: meta.ID, runtime: parseRuntime(string(meta.Runtime)), name: meta.Name,
			ids: providerIDs(*meta)}
		if r.Kind == KindMovie {
			t.year = metaYear(*meta)
		}
		return t, v, nil
	case r.Kind == KindChannel && r.Meta != nil:
		return target{item: id, kind: KindChannel, metaType: r.Meta.Type, id: r.Meta.ID}, v, nil
	case r.Kind == KindReplay:
		// A programme plays while its channel's archive keeps it.
		replay, err := s.replayProgramme(ctx, v, r)
		if err != nil {
			return target{}, view{}, err
		}
		return target{item: id, kind: KindReplay, metaType: replay.StremioType, id: replay.StremioID, runtime: replay.Runtime}, v, nil
	case r.Kind == KindEpisode && r.Video != nil:
		series, err := s.load(ctx, r.seriesItemID())
		if err != nil || series.Meta == nil {
			return target{}, view{}, ErrNotFound
		}
		t := target{item: id, kind: KindEpisode, metaType: series.Meta.Type, id: r.Video.ID, runtime: parseRuntime(string(r.Video.Runtime)),
			name: series.Meta.Name, videoName: r.Video.DisplayName(), season: int(r.Video.Season), episode: r.Video.EpisodeNumber(), holder: &series,
			ids: providerIDs(*series.Meta)}
		s.embed(ctx, v, &t, false)
		return t, v, nil
	default:
		return target{}, view{}, ErrNotFound
	}
}

// Versions lists the streams of a movie or an episode from the user's
// addons, in addon order, each file once, waiting for every addon.
func (s *Service) Versions(ctx context.Context, user accounts.User, id accounts.ID) ([]Version, error) {
	versions, _, err := s.versionsOf(ctx, user, id, waitAll)
	return versions, err
}

// CachedVersions lists an item's versions only if every addon's streams for
// it are still remembered, stale ones included (see staleLists), without
// asking the addons: listings must not ask addons for the streams of every
// title they show.
func (s *Service) CachedVersions(ctx context.Context, user accounts.User, id accounts.ID) ([]Version, bool) {
	versions, complete, err := s.versionsOf(ctx, user, id, knownOnly)
	return versions, err == nil && complete
}

// VersionsToPlay lists the versions of a movie or an episode for a play:
// it waits for the addons whose streams are not remembered, as Versions
// does, but lists a stale list as it is (see staleLists) while its addon is
// asked again in the background, for a play not to wait for an addon when
// the versions it lists, already analyzed, can start at once. complete
// reports whether no list was stale; when none of the versions plays, the
// caller waits for the new answers with Versions, which joins the requests
// under way.
func (s *Service) VersionsToPlay(ctx context.Context, user accounts.User, id accounts.ID) (versions []Version, complete bool, err error) {
	return s.versionsOf(ctx, user, id, staleNow)
}

// listing is how far versionsOf and subtitlesOf go for the addons whose
// lists for a title are not remembered, or stale (see staleLists).
type listing int

const (
	// knownOnly asks them nothing, and lists a stale list as it is.
	knownOnly listing = iota
	// askLater asks them in the background, for later requests (see
	// VersionsNow), and lists a stale list meanwhile.
	askLater
	// staleNow waits for the addons whose lists are not remembered, and
	// lists a stale list as it is while its addon is asked again in the
	// background, as askLater does (see VersionsToPlay).
	staleNow
	// waitAll asks them, those of stale lists included, and waits for
	// their answers.
	waitAll
)

// versionsOf lists an item's versions, and reports whether every addon's
// streams are in: known, and not asked again.
func (s *Service) versionsOf(ctx context.Context, user accounts.User, id accounts.ID, mode listing) ([]Version, bool, error) {
	// A channel Polyfin airs has one version: its playout.
	if version, ok, err := s.virtualVersion(ctx, user, id); err != nil {
		return nil, false, err
	} else if ok {
		return []Version{version}, true, nil
	}
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return nil, false, err
	}
	if AudioKind(t.kind) {
		// A track has one version: where its addon streams it.
		if version, ok := s.versions.Get(trackVersionID(id)); ok && version.fresh(s.now()) {
			return []Version{version}, true, nil
		}
		if mode != waitAll && mode != staleNow {
			return nil, false, nil
		}
		r, err := s.load(ctx, id)
		if err != nil {
			return nil, false, err
		}
		version, err := s.trackVersion(ctx, v, r, false)
		if err != nil {
			return nil, false, err
		}
		return []Version{version}, true, nil
	}
	if mode == waitAll || mode == staleNow {
		s.embed(ctx, v, &t, true)
	}
	serving := s.streamServing(ctx, v, t)
	if len(t.embedded) > 0 {
		// An addon that embeds a video's streams in its description is
		// not asked for them again, as Stremio apps do not ask it.
		serving = slices.DeleteFunc(serving, func(source streamSource) bool { return source.addon.ID == t.embeddedBy.addon.ID })
	}
	tracked := s.tracked()
	tracked.titles.Put(listedTitle{t.metaType, t.id}, t.item)
	for _, source := range serving {
		// An addon asked by another identifier tells the item too.
		if source.id != t.id || source.contentType != t.metaType {
			tracked.titles.Put(listedTitle{source.contentType, source.id}, t.item)
		}
	}
	if mode == askLater {
		// Item details: the title's page follows its versions from now on
		// (see FollowedProgress).
		tracked.pages.Put(askedKey{user.ID, t.item}, titlePage{t: t, serving: serving})
	}
	versions, unknown := s.listVersions(ctx, t, serving, mode, s.settings().TorrentsAllowed(user))
	if mode == askLater || mode == staleNow {
		s.askStreams(ctx, user, t, unknown)
	}
	return versions, len(unknown) == 0, nil
}

// listVersions lists the versions serving list for t, as far as mode goes,
// and returns the addons whose streams are not known, or are asked again:
// for knownOnly, those whose lists are not remembered; for askLater, those
// and those whose lists are stale; for staleNow, those whose lists are
// stale; for waitAll, none. Torrents' files are listed only with torrents
// set, for a user who may play them (see accounts.Settings.TorrentsAllowed).
//
// A file listed twice, by its identity (see streamIdentity), is one
// version, the first listed of those the user may play: a torrent left out
// leaves its place to a link to the same file listed after it. The
// versions remembered by identifier (see Version) are those of every
// stream, torrents included, as a user who may play torrents gets them, so
// that each identifier names the same version for every user; another
// user's Version lists the title again rather than take a torrent. The
// links to the same file listed after it are its mirrors (see withMirror).
func (s *Service) listVersions(ctx context.Context, t target, serving []streamSource, mode listing, torrents bool) ([]Version, []streamSource) {
	lists := make([][]stremio.Stream, len(serving))
	var unknown []streamSource
	var wg sync.WaitGroup
	for i, entry := range serving {
		// An IPTV source's or a local folder's streams are Polyfin's own:
		// reading them asks no addon.
		if own(entry.addon) && mode != waitAll {
			lists[i], _ = s.fetchStreams(ctx, entry.installed, entry.contentType, entry.id)
			continue
		}
		var l list[stremio.Stream]
		var fresh, ok bool
		if mode != waitAll {
			l, fresh, ok = s.streamListOf(ctx, entry.key())
		}
		switch {
		case fresh || ok && mode == knownOnly:
			lists[i] = l.items
		case ok:
			// A stale list is listed until the addon answers again.
			lists[i] = l.items
			unknown = append(unknown, entry)
		case mode == waitAll || mode == staleNow:
			wg.Go(func() {
				streams, err := s.streams(ctx, entry.installed, entry.contentType, entry.id)
				// An app that stops waiting cancels ctx: nothing failed.
				if err != nil && ctx.Err() == nil {
					s.logger.Warn("An addon could not list streams", "addon", entry.addon.Manifest.Name, "error", err)
				}
				lists[i] = streams
			})
		default:
			unknown = append(unknown, entry)
		}
	}
	wg.Wait()
	var versions []Version
	seen, remembered := map[accounts.ID]bool{}, map[accounts.ID]bool{}
	// kept are the versions remembered, and listed where each is among
	// versions, which take their mirrors as these come. before are those
	// remembered until now, from any user's listing: their links from
	// addons this listing does not ask stay mirrors (see carryMirrors).
	kept, listed, before := map[accounts.ID]Version{}, map[accounts.ID]int{}, map[accounts.ID]Version{}
	add := func(version Version) {
		// A version naming another title is hidden as the version rules
		// hide others; it is told once, when first listed.
		if version.Labels.Elsewhere != "" && !remembered[version.ID] {
			if _, known := s.versions.Get(version.ID); !known {
				s.logger.Debug("A version names another title", "addon", version.Addon, "names", version.Labels.Elsewhere)
			}
		}
		if !remembered[version.ID] {
			remembered[version.ID] = true
			if known, ok := s.versions.Get(version.ID); ok && known.Item == version.Item {
				before[version.ID] = known
			}
			kept[version.ID] = version
			s.versions.Put(version.ID, version)
		} else if first, ok := withMirror(kept[version.ID], version); ok {
			kept[version.ID] = first
			s.versions.Put(version.ID, first)
		}
		if version.Torrent != nil && !torrents {
			return
		}
		if seen[version.ID] {
			if first, ok := withMirror(versions[listed[version.ID]], version); ok {
				versions[listed[version.ID]] = first
			}
			return
		}
		seen[version.ID] = true
		listed[version.ID] = len(versions)
		versions = append(versions, version)
	}
	// A video's own streams come first (see embed).
	for _, stream := range t.embedded {
		if listable(stream) {
			add(embeddedVersion(t, stream))
		}
	}
	for i, streams := range lists {
		for _, stream := range streams {
			if listable(stream) {
				add(newVersion(t, serving[i], stream))
			}
		}
	}
	if len(before) > 0 {
		asked := make(map[accounts.ID]bool, len(serving)+1)
		for _, entry := range serving {
			asked[entry.addon.ID] = true
		}
		if len(t.embedded) > 0 {
			asked[t.embeddedBy.addon.ID] = true
		}
		for id, known := range before {
			if merged, ok := carryMirrors(kept[id], known, asked, s.now()); ok {
				s.versions.Put(id, merged)
			}
		}
	}
	return versions, unknown
}

// Version finds a version of an item, listing the item's versions again
// when it is no longer remembered. The item's own identifier stands for its
// first version, as Jellyfin apps name a single-version item's source after
// the item. A torrent remembered under id, for a user who may not play
// torrents, lists the versions again too: the user's version of that file
// may be a link to it listed after the torrent (see listVersions).
//
// A version is remembered from any user's listing, while a user only
// reaches the versions their own sources list (see listedFor): one
// remembered from another user's sources lists the item again as the
// user, and a file none of their sources lists is ErrNotFound, as a
// version that does not exist.
func (s *Service) Version(ctx context.Context, user accounts.User, item, id accounts.ID) (Version, error) {
	if version, ok := s.versions.Get(id); ok && version.Item == item && version.fresh(s.now()) &&
		(version.Torrent == nil || s.settings().TorrentsAllowed(user)) {
		mine, listed, err := s.listedFor(ctx, user, version)
		if err != nil {
			return Version{}, err
		}
		if listed && mine.fresh(s.now()) {
			// A version remembered from another user's listing is no way
			// around the user's parental control or blocked genres, nor
			// to a title only an addon they do not use knows.
			if user.Restricted() || s.withholdsAny(ctx, user) {
				if _, _, err := s.target(ctx, user, item); err != nil {
					return Version{}, err
				}
			}
			return mine, nil
		}
	}
	versions, err := s.Versions(ctx, user, item)
	if err != nil {
		return Version{}, err
	}
	for i, version := range versions {
		if version.ID == id || (id == item && i == 0) {
			return version, nil
		}
	}
	return Version{}, ErrNotFound
}

// VersionOwner returns the item a version listed earlier belongs to:
// Jellyfin apps open a version as an item by its identifier.
func (s *Service) VersionOwner(id accounts.ID) (accounts.ID, bool) {
	version, ok := s.versions.Get(id)
	return version.Item, ok
}

// KnownVersion returns a version listed earlier, by its identifier,
// without asking addons; for the server's own bookkeeping, not for users,
// whose access it does not check.
func (s *Service) KnownVersion(id accounts.ID) (Version, bool) {
	return s.versions.Get(id)
}

// renewWait bounds how long a renewal waits for the addon's follow-up when
// the addon's answer lacks the version's file (see Renew).
const renewWait = 10 * time.Second

// Renew asks the addon that listed a version for its streams again and
// returns the same file with the link the addon gives now, for links that
// expired. Every listing of the version uses the new link afterwards. The
// addon is asked as a title's page asks it (see streams): a request under
// way is joined, and the answer is stored and followed up, keeping the
// items of the list it replaces listed meanwhile (see keepAnswer). An addon
// that gathers other addons' streams may answer first without the file:
// its follow-up is then asked at once, and waited for at most renewWait.
// Only the addon's answers count, not the items kept from the list
// replaced, whose links are those that expired.
func (s *Service) Renew(ctx context.Context, old Version) (Version, error) {
	return s.renew(ctx, old, false)
}

// renew is Renew; with kept set, the addon's answer or description kept,
// given less than a version list's life ago (see listLife), is read as it
// is rather than asked for again: a version its answer lacks is then
// ErrNotFound.
func (s *Service) renew(ctx context.Context, old Version, kept bool) (Version, error) {
	addon, err := s.addons.Find(ctx, old.Origin.Addon)
	if errors.Is(err, addons.ErrNotFound) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	entry := installed{addon: addon, confined: old.Confined}
	if old.Origin.Type == originEclipse {
		// A track's stream resource gives a fresh link, which is now the
		// track's.
		r, err := s.load(ctx, old.Item)
		if err != nil || r.Music == nil || !addon.HasMusic() {
			return Version{}, ErrNotFound
		}
		return s.resolveTrack(ctx, entry, r, true)
	}
	if old.Origin.Title != "" {
		// A stream a description embeds is renewed by describing its
		// title again.
		return s.renewEmbedded(ctx, entry, old, kept)
	}
	t := target{item: old.Item, metaType: old.Origin.Type, id: old.Origin.ID, runtime: old.Runtime}
	t.season, t.episode = old.torrentEpisode()
	if old.HoldsConnection {
		t.kind = KindReplay
	}
	if own(addon) {
		streams, err := s.fetchStreams(ctx, entry, t.metaType, t.id)
		if err != nil {
			return Version{}, err
		}
		if version, ok := s.renewed(t, entry, streams, old); ok {
			return version, nil
		}
		return Version{}, ErrNotFound
	}
	key := streamKey{addon.ID, t.metaType, t.id}
	if kept {
		if _, fresh, _ := s.streamListOf(ctx, key); fresh {
			if version, ok := s.renewedFromAnswer(t, entry, key, old); ok {
				return version, nil
			}
			return Version{}, ErrNotFound
		}
	}
	s.tracked().titles.Put(listedTitle{t.metaType, t.id}, t.item)
	if _, err := s.streamAnswer(ctx, entry, key); err != nil {
		return Version{}, err
	}
	if version, ok := s.renewedFromAnswer(t, entry, key, old); ok {
		return version, nil
	}
	asked := s.hurryFollowUp(key)
	if asked == nil {
		return Version{}, ErrNotFound
	}
	timer := time.NewTimer(renewWait)
	defer timer.Stop()
	select {
	case <-asked:
	case <-timer.C:
	case <-ctx.Done():
		return Version{}, ctx.Err()
	}
	if version, ok := s.renewedFromAnswer(t, entry, key, old); ok {
		return version, nil
	}
	return Version{}, ErrNotFound
}

// renewedFromAnswer finds the version old among the streams of the addon's
// last answer kept under key.
func (s *Service) renewedFromAnswer(t target, entry installed, key streamKey, old Version) (Version, bool) {
	l, ok := s.streamLists.Get(key)
	if !ok {
		return Version{}, false
	}
	return s.renewed(t, entry, l.items[:l.answer], old)
}

// renewed finds the version old among streams, and remembers it with the
// link they give. It keeps what old's labels told, read knowing the
// title's name, which a renewal does not load.
func (s *Service) renewed(t target, entry installed, streams []stremio.Stream, old Version) (Version, bool) {
	for _, stream := range streams {
		// The stream's identity tells it before its labels are read.
		if !listable(stream) || versionID(t.item, streamIdentity(stream)) != old.ID {
			continue
		}
		version := newVersion(t, streamSource{entry, t.metaType, t.id}, stream)
		version.Labels = old.Labels
		version, kept := s.keepMirrors(version)
		s.versions.Put(version.ID, kept)
		return version, true
	}
	return Version{}, false
}

// newVersion describes a stream an addon listed for a target, asked as
// source tells (see streamServing): a version is renewed by asking the
// addon the same again.
func newVersion(t target, source streamSource, stream stremio.Stream) Version {
	entry := source.installed
	version := Version{
		Item:    t.item,
		Name:    versionName(stream),
		URL:     stream.URL,
		Headers: stream.RequestHeaders(),
		// An IPTV source's default User-Agent: no addon sets it.
		UserAgent:  stream.UserAgent,
		Protection: stream.Protection,
		Filename:   stream.BehaviorHints.Filename,
		Size:       int64(stream.BehaviorHints.VideoSize),
		Addon:      entry.addon.Manifest.Name,
		Origin:     Origin{Addon: entry.addon.ID, Type: source.contentType, ID: source.id},
		Confined:   entry.confined,
		Runtime:    t.runtime,
		Height:     LabelHeight(stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename),
		Labels:     ReadLabels(t.name, stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename),
		// A replay holds one of its provider's connections, as a channel does.
		HoldsConnection: t.kind == KindReplay,
	}
	version.Labels.Releases = ReleaseKinds([]string{t.name, t.videoName}, stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename)
	version.Labels.Elsewhere = OtherTitle(t.name, t.season, t.episode, t.year, stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename)
	if stream.Torrent() {
		version.URL = torrentLink(stream)
		version.Torrent = &Torrent{InfoHash: strings.ToLower(stream.InfoHash), FileIdx: stream.FileIdx, Sources: slices.Clone(stream.Sources),
			Season: t.season, Episode: t.episode}
	}
	version.ID = versionID(t.item, streamIdentity(stream))
	// The subtitle files listed with a stream are its version's own: those
	// beside a local file, or those an addon gives with its stream.
	for _, subtitle := range stream.Subtitles {
		if !subtitle.Valid() {
			continue
		}
		version.Subtitles = append(version.Subtitles, ExternalSubtitle{ID: subtitleID(entry.addon.ID, subtitle), Language: subtitle.Lang,
			URL: subtitle.URL, Addon: entry.addon.Manifest.Name, Confined: entry.confined, Format: subtitle.Format, Title: subtitle.Title,
			Forced: subtitle.Forced, HearingImpaired: subtitle.HearingImpaired, Default: subtitle.Default})
	}
	return version
}

// VersionSubtitle returns file, one of the version's own subtitle files
// (see Version.Subtitles), as it is now. A file beside a local file is its
// folder's. A file an addon lists with its stream may expire with the
// stream's link: it lasts as long as a version list (see listLife), then
// is the file the addon lists with the stream now, from its answer kept
// while fresh, else asked again as Renew asks it. It is the file of the
// same language, at the same place among those. A version, or a file, the
// addon no longer lists is ErrNotFound.
func (s *Service) VersionSubtitle(ctx context.Context, version Version, file ExternalSubtitle) (ExternalSubtitle, error) {
	// Only Polyfin's own local folders tell a file's format.
	if file.Format != "" {
		return file, nil
	}
	current, err := s.renew(ctx, version, true)
	if err != nil {
		return ExternalSubtitle{}, err
	}
	place := 0
	for _, before := range version.Subtitles {
		if before.ID == file.ID {
			break
		}
		if before.Language == file.Language {
			place++
		}
	}
	for _, now := range current.Subtitles {
		if now.Language != file.Language {
			continue
		}
		if place == 0 {
			return now, nil
		}
		place--
	}
	return ExternalSubtitle{}, ErrNotFound
}

// streams lists an addon's streams for a title: those remembered while
// fresh, else its answer (see streamAnswer).
func (s *Service) streams(ctx context.Context, entry installed, contentType, id string) ([]stremio.Stream, error) {
	// An IPTV source's line-up, and a local folder's scans, change its
	// streams at once.
	if own(entry.addon) {
		return s.fetchStreams(ctx, entry, contentType, id)
	}
	key := streamKey{entry.addon.ID, contentType, id}
	if l, fresh, _ := s.streamListOf(ctx, key); fresh {
		return l.items, nil
	}
	return s.streamAnswer(ctx, entry, key)
}

// streamAnswer asks an addon for its streams for a title. Every caller
// asking meanwhile shares the answer, item details' background requests
// and renewals included (see VersionsNow and Renew). An answer stored is
// followed up; until the follow-ups end, it is listed with the items it
// lacks of the stale list it replaced (see followUpList). An answer of
// notices alone, nothing to play, is not stored over a list that has
// streams to play: it is errOnlyNotices, an addon failing. A refresh of
// the title while the addon is asked leaves its answer out: the addon is
// asked again, for the callers waiting too (see forgetLists).
func (s *Service) streamAnswer(ctx context.Context, entry installed, key streamKey) ([]stremio.Stream, error) {
	for {
		streams, err := s.askStreamsOf(ctx, entry, key)
		if !errors.Is(err, errForgotten) {
			return streams, err
		}
	}
}

// askStreamsOf asks an addon for its streams for a title once, as
// streamAnswer does.
func (s *Service) askStreamsOf(ctx context.Context, entry installed, key streamKey) ([]stremio.Stream, error) {
	return shared(ctx, s, streamsFlight(key), func(ctx context.Context) ([]stremio.Stream, error) {
		asked, done := s.ask(followKey{listStreams, key})
		defer done()
		s.logger.Debug("An addon is asked for streams", "addon", entry.addon.Manifest.Name, "type", key.contentType, "id", key.id)
		streams, err := s.fetchStreams(ctx, entry, key.contentType, key.id)
		if err != nil {
			return nil, err
		}
		if len(streams) > 0 && streamKind.count(streams) == 0 {
			if kept, _, ok := s.streamListOf(ctx, key); ok && streamKind.count(kept.items) > 0 {
				return nil, errOnlyNotices
			}
		}
		return s.followStreams(ctx, entry, key, asked, streams)
	})
}

// errOnlyNotices is an addon's answer listing only notices for a title it
// listed streams for, such as a notice that it limits requests: it would
// list them again a moment later. The list kept stays as it was, listed
// while kept, and asked for again as it would have been. An empty answer
// is the addon having nothing for the title any more: it replaces the
// list as any answer does.
var errOnlyNotices = errors.New("its answer has only notices, nothing to play: the streams it listed before are kept")

// streamsFlight names the request asking an addon for its streams for a
// title, which callers share (see streamAnswer).
func streamsFlight(key streamKey) string {
	return "streams " + key.addon.String() + " " + key.contentType + " " + key.id
}

// serves reports whether an addon serves a resource for a title: a home
// video's and a plain video's are its folder's alone, whatever other
// addons declare, so that none of them ever hears of it; an extra's
// stream is its local folder's, which answers for its own extras only.
func serves(entry installed, resource, metaType, id string) bool {
	if metaType == ExtraType || strings.HasPrefix(id, ExtraIDPrefix) {
		return entry.addon.Local() && resource == "stream"
	}
	if (metaType == VideoType || strings.HasPrefix(id, VideoIDPrefix) || strings.HasPrefix(id, PlainIDPrefix)) && !entry.addon.Local() {
		return false
	}
	return entry.addon.Manifest.Serves(resource, metaType, id)
}

// KnownSubtitles lists, as Subtitles does, the subtitle files known now,
// stale lists' included, asking no addon. complete reports whether every
// addon's list was known.
func (s *Service) KnownSubtitles(ctx context.Context, user accounts.User, id accounts.ID) (subtitles []ExternalSubtitle, complete bool, err error) {
	return s.subtitlesOf(ctx, user, id, knownOnly)
}

// shared runs fetch once for every caller asking for key at the same time,
// in s.flight. fetch runs detached from the callers, bounded by the
// timeouts of what it asks (the client's, for addons): a caller that stops
// waiting, when its ctx ends, does not cut it short for the others.
func shared[T any](ctx context.Context, s *Service, key string, fetch func(context.Context) (T, error)) (T, error) {
	detached := context.WithoutCancel(ctx)
	answer := s.flight.DoChan(key, func() (any, error) { return fetch(detached) })
	if s.joined != nil {
		s.joined(key)
	}
	select {
	case result := <-answer:
		if result.Err != nil {
			var zero T
			return zero, result.Err
		}
		return result.Val.(T), nil
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// fetchStreams asks an addon, an IPTV source or a local folder for the
// streams of a title.
func (s *Service) fetchStreams(ctx context.Context, entry installed, contentType, id string) ([]stremio.Stream, error) {
	switch {
	case entry.addon.Stremio():
		return s.client.Streams(ctx, entry.addon.ManifestURL, contentType, id, entry.confined)
	case entry.addon.Local():
		if s.local == nil {
			return nil, nil
		}
		return s.local.Streams(ctx, entry.addon.ID, id)
	case s.iptv == nil:
		return nil, nil
	}
	return s.iptv.Streams(ctx, entry.addon.ID, id)
}

// versionID identifies a stream of an item by the file it is (see
// streamIdentity).
func versionID(item accounts.ID, identity string) accounts.ID {
	return itemID("version|" + item.String() + "|" + identity)
}

// streamIdentity is the file a stream is, the same in every answer of its
// addon however its link changes: the identity an earlier answer gave it,
// when its link or hints changed since (see carryIdentities), else its own
// (see fileIdentity).
func streamIdentity(stream stremio.Stream) string {
	if stream.Identity != "" {
		return stream.Identity
	}
	link := stream.URL
	if stream.Torrent() {
		link = torrentLink(stream)
	}
	return fileIdentity(link, stream.BehaviorHints.Filename, int64(stream.BehaviorHints.VideoSize))
}

// fileIdentity tells a file by its name and size when the addon names it,
// and by its link otherwise.
func fileIdentity(link, filename string, size int64) string {
	if filename != "" && size > 0 {
		return filename + "|" + strconv.FormatInt(size, 10)
	}
	return link
}

// carryIdentities returns answer, an addon's answer for a title, where
// each stream whose identity is not in before, the list the answer
// replaces, takes the identity of the stream of before it is (see
// sameFile), when answer lacks that identity. Otherwise a link given with
// a new token, or a file whose name or size the addon tells only now,
// would be a new version: a title's page open would lose it as the version
// picked, and Polyfin what it learned of the file. Only a match that is
// one stream on each side counts, so that two streams of an answer never
// share an identity. answer itself is left as it is.
func carryIdentities(answer, before []stremio.Stream) []stremio.Stream {
	listed := make(map[string]bool, len(answer))
	for _, stream := range answer {
		listed[streamIdentity(stream)] = true
	}
	known := make(map[string]bool, len(before))
	var gone []stremio.Stream
	for _, stream := range before {
		identity := streamIdentity(stream)
		if !known[identity] && !listed[identity] && listable(stream) {
			gone = append(gone, stream)
		}
		known[identity] = true
	}
	if len(gone) == 0 {
		return answer
	}
	// match is, for each stream of answer, the one stream of gone it may
	// be, -1 for none or several; claims counts, for each stream of gone,
	// the streams of answer that may be it.
	match := make([]int, len(answer))
	claims := make([]int, len(gone))
	for i, stream := range answer {
		match[i] = -1
		if !listable(stream) || known[streamIdentity(stream)] {
			continue
		}
		several := false
		for j, old := range gone {
			if !sameFile(old, stream) {
				continue
			}
			claims[j]++
			several = several || match[i] >= 0
			match[i] = j
		}
		if several {
			match[i] = -1
		}
	}
	var carried []stremio.Stream
	for i, j := range match {
		if j < 0 || claims[j] != 1 {
			continue
		}
		if carried == nil {
			carried = slices.Clone(answer)
		}
		carried[i].Identity = streamIdentity(gone[j])
	}
	if carried == nil {
		return answer
	}
	return carried
}

// sameFile tells whether b, in an addon's answer, is the file a was in the
// list the answer replaces, under another identity. The file's names and
// sizes must agree where both streams tell them; then either the link is
// the same, the addon telling the name or size only now, or so is the
// file's name, or the link differs by a token only: it keeps its host and
// the last part of its path, and either that part is the file's name, as
// one of the streams tells it, or the stream keeps its name and
// description.
func sameFile(a, b stremio.Stream) bool {
	ha, hb := a.BehaviorHints, b.BehaviorHints
	if ha.Filename != "" && hb.Filename != "" && ha.Filename != hb.Filename || ha.VideoSize > 0 && hb.VideoSize > 0 && ha.VideoSize != hb.VideoSize {
		return false
	}
	if a.Torrent() || b.Torrent() {
		// A torrent's file is the same one at the same magnet link alone.
		return a.Torrent() && b.Torrent() && torrentLink(a) == torrentLink(b)
	}
	if a.URL == b.URL || ha.Filename != "" && ha.Filename == hb.Filename {
		return true
	}
	host, file := linkFile(a.URL)
	if otherHost, otherFile := linkFile(b.URL); file == "" || host != otherHost || file != otherFile {
		return false
	}
	return file == ha.Filename || file == hb.Filename || versionName(a) == versionName(b)
}

// linkFile is a link's host and the last part of its path, which is empty
// when the path ends with "/".
func linkFile(link string) (host, file string) {
	parsed, err := url.Parse(link)
	if err != nil {
		return "", ""
	}
	return parsed.Host, parsed.Path[strings.LastIndexByte(parsed.Path, '/')+1:]
}

// versionName joins the stream's name and description lines into one line,
// as version pickers show a single line.
func versionName(stream stremio.Stream) string {
	var parts []string
	for _, text := range []string{stream.Name, stream.Details()} {
		for line := range strings.SplitSeq(text, "\n") {
			if line = strings.Join(strings.Fields(line), " "); line != "" {
				parts = append(parts, line)
			}
		}
	}
	if len(parts) == 0 {
		return stream.BehaviorHints.Filename
	}
	return strings.Join(parts, " · ")
}

// Subtitles lists the subtitle files users added to a movie or an episode,
// then those the user's addons offer for it, in addon order, waiting for
// every addon.
func (s *Service) Subtitles(ctx context.Context, user accounts.User, id accounts.ID) ([]ExternalSubtitle, error) {
	subtitles, _, err := s.subtitlesOf(ctx, user, id, waitAll)
	return subtitles, err
}

// SubtitlesNow lists, as Subtitles does, the subtitle files known now,
// without waiting for addons: it asks those whose lists are not remembered,
// or stale, in the background, for later requests, a stale list being
// listed meanwhile. complete reports whether every addon's list was known,
// none asked again.
func (s *Service) SubtitlesNow(ctx context.Context, user accounts.User, id accounts.ID) (subtitles []ExternalSubtitle, complete bool, err error) {
	return s.subtitlesOf(ctx, user, id, askLater)
}

func (s *Service) subtitlesOf(ctx context.Context, user accounts.User, id accounts.ID, mode listing) ([]ExternalSubtitle, bool, error) {
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return nil, false, err
	}
	var serving []installed
	for _, entry := range v.addons {
		if serves(entry, "subtitles", t.metaType, t.id) {
			serving = append(serving, entry)
		}
	}
	lists := make([][]stremio.Subtitle, len(serving))
	complete := true
	if mode == waitAll {
		var wg sync.WaitGroup
		for i, entry := range serving {
			wg.Go(func() {
				subtitles, err := s.subtitleList(ctx, entry, t)
				if err != nil && ctx.Err() == nil {
					s.logger.Warn("An addon could not list subtitles", "addon", entry.addon.Manifest.Name, "error", err)
				}
				lists[i] = subtitles
			})
		}
		wg.Wait()
	} else {
		detached := context.WithoutCancel(ctx)
		for i, entry := range serving {
			l, fresh, ok := listOf(s, s.subtitleLists, streamKey{entry.addon.ID, t.metaType, t.id})
			lists[i] = l.items
			// A stale list is listed until the addon answers again.
			if !ok || !fresh && mode == askLater {
				complete = false
				if mode == askLater {
					go func() {
						if _, err := s.subtitleList(detached, entry, t); err != nil {
							s.logger.Warn("An addon could not list subtitles", "addon", entry.addon.Manifest.Name, "error", err)
						}
					}()
				}
			}
		}
	}
	result, err := s.uploaded(ctx, id)
	if err != nil {
		return nil, false, err
	}
	seen := map[accounts.ID]bool{}
	for i, subtitles := range lists {
		for _, subtitle := range subtitles {
			if !subtitle.Valid() {
				continue
			}
			file := subtitleID(serving[i].addon.ID, subtitle)
			if seen[file] {
				continue
			}
			seen[file] = true
			result = append(result, ExternalSubtitle{ID: file, Language: subtitle.Lang, URL: subtitle.URL,
				Addon: serving[i].addon.Manifest.Name, Confined: serving[i].confined})
		}
	}
	return result, complete, nil
}

// subtitleList lists an addon's subtitles for a title: those remembered
// while fresh, else its answer, which every caller asking meanwhile shares.
// An answer stored is followed up; until the follow-ups end, it is listed
// with the items it lacks of the stale list it replaced (see followUpList).
// A refresh of the title while the addon is asked leaves its answer out:
// the addon is asked again (see forgetLists).
func (s *Service) subtitleList(ctx context.Context, entry installed, t target) ([]stremio.Subtitle, error) {
	key := streamKey{entry.addon.ID, t.metaType, t.id}
	if l, fresh, _ := listOf(s, s.subtitleLists, key); fresh {
		return l.items, nil
	}
	for {
		subtitles, err := shared(ctx, s, subtitlesFlight(key), func(ctx context.Context) ([]stremio.Subtitle, error) {
			asked, done := s.ask(followKey{listSubtitles, key})
			defer done()
			subtitles, err := s.fetchSubtitles(ctx, entry, t.metaType, t.id)
			if err != nil {
				return nil, err
			}
			return s.followSubtitles(ctx, entry, key, asked, subtitles)
		})
		if !errors.Is(err, errForgotten) {
			return subtitles, err
		}
	}
}

// subtitlesFlight names the request asking an addon for its subtitles for
// a title, which callers share (see subtitleList).
func subtitlesFlight(key streamKey) string {
	return "subtitles " + key.addon.String() + " " + key.contentType + " " + key.id
}

// subtitleID identifies a subtitle of an addon, without revealing either.
func subtitleID(addon accounts.ID, subtitle stremio.Subtitle) accounts.ID {
	sum := sha256.Sum256([]byte(addon.String() + "|" + subtitleIdentity(subtitle)))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

// subtitleIdentity is the file a subtitle is among those of its addon.
func subtitleIdentity(subtitle stremio.Subtitle) string {
	return string(subtitle.ID) + "|" + subtitle.URL
}

// fetchSubtitles asks an addon for the subtitles of a title.
func (s *Service) fetchSubtitles(ctx context.Context, entry installed, contentType, id string) ([]stremio.Subtitle, error) {
	return s.client.Subtitles(ctx, entry.addon.ManifestURL, contentType, id, nil, entry.confined)
}
