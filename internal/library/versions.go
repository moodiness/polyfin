package library

import (
	"cmp"
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
	"github.com/moodiness/polyfin/internal/logs"
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
	// Addon is the name of the addon that listed the stream, as Polyfin
	// shows it (see addons.Addon.Name), and AddonLog as logs name it (see
	// addons.Addon.LogName).
	Addon, AddonLog string
	// Origin is where the stream was listed, to ask again for it when its
	// link expires.
	Origin Origin
	// LocalFolder is set for a file of one of Polyfin's own local folders,
	// a network share's included; not for the address a .strm file of
	// theirs names, which plays as an addon's link does.
	LocalFolder bool
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
	// Cached is whether the debrid service the addon names holds the file
	// ready, nil when the addon does not say (see stremio.StreamData).
	// Seeders are its torrent's seeders, as the addon's stream data, else
	// its labels, tell them (see LabelSeeders); 0 when they do not.
	Cached  *bool
	Seeders int
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
	// identity is the file the version is (see streamIdentity), which
	// administrators' blocks name (see fileKey).
	identity string
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
// filename names (see torrent.Info.File). Title and Year are a movie's,
// which tell it in a pack of several movies.
type Torrent struct {
	InfoHash        string
	FileIdx         *int
	Sources         []string
	Season, Episode int
	Title           string
	Year            int
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
	r, err := s.loadListed(ctx, v, id)
	if errors.Is(err, ErrNotFound) {
		// An extra plays its file, as its folder streams it; an addon
		// trailer its link, as its title's description gives it.
		extra, err := s.localExtra(ctx, v, id)
		if errors.Is(err, ErrNotFound) {
			trailer, err := s.addonTrailer(ctx, v, id)
			if err != nil {
				return target{}, view{}, err
			}
			return trailer.target(), v, nil
		}
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
			ids: s.providerIDs(r.ID, *meta)}
		if r.Kind == KindMovie {
			t.year = metaYear(*meta)
		}
		// A title shown as a movie of one video plays that video's streams
		// (see shownKind).
		if r.shownTitle() && len(meta.Videos) == 1 {
			t.id, t.holder = meta.Videos[0].ID, &r
			s.embed(ctx, v, &t, false)
		}
		return t, v, nil
	case r.Kind == KindChannel && r.Meta != nil:
		// A channel plays while the user reaches it: from one of their live
		// TV catalogs, and shown by their channel rule.
		if _, err := s.channel(ctx, v, r); err != nil {
			return target{}, view{}, err
		}
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
			ids: s.providerIDs(series.ID, *series.Meta)}
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
// the versions it lists, already analyzed, can start at once. Once it has a
// version to play, it waits for each further addon's answer no more than
// the play's grace (see defaultPlayGrace): an addon that answers later is
// left out of this play, and its answer is kept for the next ones and the
// title's page. complete reports whether no list was stale and no addon was
// left out; when none of the versions plays, the caller waits for the new
// answers with Versions, which joins the requests under way.
func (s *Service) VersionsToPlay(ctx context.Context, user accounts.User, id accounts.ID) (versions []Version, complete bool, err error) {
	return s.versionsOf(ctx, user, id, staleNow)
}

// defaultPlayGrace is how long a play that has a version to play waits for
// the next addon to answer: as long again for each one that does. An addon
// usually answers within a few seconds of the others; one that never
// answers would otherwise hold every first play of a title for the whole
// stremio.StreamTimeout.
const defaultPlayGrace = 5 * time.Second

// SetPlayGrace replaces, before the service is used, how long a play that
// has a version to play waits for the next addon's answer (see
// VersionsToPlay). Tests shorten it; the server keeps defaultPlayGrace.
func (s *Service) SetPlayGrace(grace time.Duration) { s.playGrace = grace }

// listing is how far versionsOf and subtitlesOf go for the addons whose
// lists for a title are not remembered, or stale (see staleLists).
type listing int

const (
	// knownOnly asks them nothing, and lists a stale list as it is.
	knownOnly listing = iota
	// askLater asks them in the background, for later requests (see
	// VersionsNow), and lists a stale list meanwhile.
	askLater
	// staleNow waits for the addons whose lists are not remembered, but
	// for those still asked a play's grace after it has a version to play
	// (see VersionsToPlay), and lists a stale list as it is while its
	// addon is asked again in the background, as askLater does.
	staleNow
	// waitAll asks them, those of stale lists included, and waits for
	// their answers.
	waitAll
)

// versionsOf lists an item's versions, and reports whether every addon's
// streams are in: known, and not asked again. A movie waiting for its
// digital release has none, and its addons are not asked.
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
	if t.kind == KindMovie {
		if _, held, err := s.upcoming(ctx, v, id); err != nil || held {
			return nil, err == nil, err
		}
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
	if mode != knownOnly && !markOf(ctx).background {
		// A user opens or plays the title.
		s.userListing(serving)
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
// and those whose lists are stale, but those that failed lately (see
// failedFor); for staleNow, those whose lists are stale, and those a play
// went on without (see VersionsToPlay); for waitAll, none. Torrents' files
// are listed only with torrents set, for a user who may play them (see
// accounts.Settings.TorrentsAllowed).
func (s *Service) listVersions(ctx context.Context, t target, serving []streamSource, mode listing, torrents bool) ([]Version, []streamSource) {
	lists, unknown := s.gatherStreams(ctx, t, serving, mode, torrents)
	return s.buildVersions(t, serving, lists, torrents, s.versionChoices(ctx), nil), unknown
}

// gatherStreams gathers the lists of streams serving give for t, as far as
// mode goes (see listVersions), each at the place of its source, nil for
// a source whose list is not known. torrents tells whether the user may
// play torrents' files, for a play to tell whether it has a version to
// play (see VersionsToPlay).
func (s *Service) gatherStreams(ctx context.Context, t target, serving []streamSource, mode listing, torrents bool) ([][]stremio.Stream, []streamSource) {
	lists := make([][]stremio.Stream, len(serving))
	var unknown []streamSource
	// asked holds the sources asked, whose answers come on answers.
	type answer struct {
		source  int
		streams []stremio.Stream
	}
	answers := make(chan answer, len(serving))
	asked := map[int]bool{}
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
		case mode == askLater && s.failed(entry.key()):
			// An addon that failed lately is not asked again in the
			// background: its stale list is listed meanwhile, if any.
			lists[i] = l.items
		case ok:
			// A stale list is listed until the addon answers again.
			lists[i] = l.items
			unknown = append(unknown, entry)
		case mode == waitAll || mode == staleNow:
			asked[i] = true
			go func() {
				// A failure, or an answer of notices alone, is logged where
				// the addon is asked, once (see askStreamsOf).
				streams, _ := s.streams(ctx, entry.installed, entry.contentType, entry.id)
				answers <- answer{i, streams}
			}()
		default:
			unknown = append(unknown, entry)
		}
	}
	// A play waits for the next answer a grace at most once it has a
	// version to play; anything else waits for them all.
	playable := func(streams []stremio.Stream) bool {
		return mode == staleNow && slices.ContainsFunc(streams, func(stream stremio.Stream) bool {
			return stream.Playable() || torrents && stream.Torrent()
		})
	}
	var grace *time.Timer
	if slices.ContainsFunc(lists, playable) {
		grace = time.NewTimer(s.playGrace)
	}
	var late <-chan time.Time
	for len(asked) > 0 {
		if grace != nil {
			late = grace.C
		}
		select {
		case a := <-answers:
			delete(asked, a.source)
			lists[a.source] = a.streams
			switch {
			case grace != nil:
				grace.Reset(s.playGrace)
			case playable(a.streams):
				grace = time.NewTimer(s.playGrace)
			}
		case <-late:
			// The addons still asked are left out of this play: their
			// answers are kept for the next ones, and told to the title's
			// page (see askStreams).
			for i := range asked {
				unknown = append(unknown, serving[i])
			}
			s.logger.Debug("A play goes on without the addons that did not answer yet", "addons", len(asked), "grace", s.playGrace)
			clear(asked)
		}
	}
	if grace != nil {
		grace.Stop()
	}
	return lists, unknown
}

// What became of a stream an addon listed for a title, as buildVersions
// tells it (see VersionsExplained): a version, or left out by one of the
// steps of docs/playback.md, Which streams become versions.
const (
	// StepVersion: the stream is a version of the title.
	StepVersion = "version"
	// StepNothing: the stream has nothing to play, as a web page, a
	// YouTube video or a notice.
	StepNothing = "nothing"
	// StepTorrents: the stream is a torrent's file, for a user who may
	// not play torrents.
	StepTorrents = "torrents"
	// StepBlocked: an administrator blocked the file.
	StepBlocked = "blocked"
	// StepDuplicate: the same file was listed before it.
	StepDuplicate = "duplicate"
	// StepMirror: the same file was listed before it by another addon,
	// and the stream is one of that version's mirrors.
	StepMirror = "mirror"
)

// buildVersions makes the versions of t from lists, the streams each of
// serving gave, as listVersions lists them. A file listed twice, by its
// identity (see streamIdentity), is one version, the first listed of those
// the user may play: a torrent left out leaves its place to a link to the
// same file listed after it. The versions remembered by identifier (see
// Version) are those of every stream, torrents and files blocked included,
// as a user who may play torrents gets them, so that each identifier names
// the same version for every user; another user's Version lists the title
// again rather than take a torrent. The links to the same file listed
// after it are its mirrors (see withMirror). The files administrators
// blocked, for t or the server (see BlockVersion), are listed nowhere.
//
// note, when set, is told of every stream in the order they come, those
// t embeds first under the source -1: the version it makes, empty for a
// stream with nothing to play, and what became of it (see StepVersion).
func (s *Service) buildVersions(t target, serving []streamSource, lists [][]stremio.Stream, torrents bool, choices choiceSet,
	note func(source int, stream stremio.Stream, version Version, step string)) []Version {
	var versions []Version
	seen, remembered := map[accounts.ID]bool{}, map[accounts.ID]bool{}
	// kept are the versions remembered, and listed where each is among
	// versions, which take their mirrors as these come. before are those
	// remembered until now, from any user's listing: their links from
	// addons this listing does not ask stay mirrors (see carryMirrors).
	kept, listed, before := map[accounts.ID]Version{}, map[accounts.ID]int{}, map[accounts.ID]Version{}
	add := func(version Version) string {
		// A version naming another title is hidden as the version rules
		// hide others; it is told once, when first listed.
		if version.Labels.Elsewhere != "" && !remembered[version.ID] {
			if _, known := s.versions.Get(version.ID); !known {
				s.logger.Debug("A version names another title", "addon", version.AddonLog, "names", version.Labels.Elsewhere)
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
			return StepTorrents
		}
		if _, blocked := choices.blocked(version); blocked {
			return StepBlocked
		}
		if seen[version.ID] {
			if first, ok := withMirror(versions[listed[version.ID]], version); ok {
				versions[listed[version.ID]] = first
				return StepMirror
			}
			return StepDuplicate
		}
		seen[version.ID] = true
		listed[version.ID] = len(versions)
		versions = append(versions, version)
		return StepVersion
	}
	if note == nil {
		note = func(int, stremio.Stream, Version, string) {}
	}
	// A video's own streams come first (see embed).
	for _, stream := range t.embedded {
		if !listable(stream) {
			note(-1, stream, Version{}, StepNothing)
			continue
		}
		version := embeddedVersion(t, stream)
		note(-1, stream, version, add(version))
	}
	for i, streams := range lists {
		for _, stream := range streams {
			if !listable(stream) {
				note(i, stream, Version{}, StepNothing)
				continue
			}
			version := newVersion(t, serving[i], stream)
			note(i, stream, version, add(version))
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
	s.keepOwners(kept, before)
	return versions
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
// version that does not exist. A file an administrator blocked is
// ErrNotFound.
func (s *Service) Version(ctx context.Context, user accounts.User, item, id accounts.ID) (Version, error) {
	if version, ok := s.versions.Get(id); ok && version.Item == item && version.fresh(s.now()) &&
		(version.Torrent == nil || s.settings().TorrentsAllowed(user)) {
		if _, blocked := s.versionChoices(ctx).blocked(version); blocked {
			return Version{}, ErrNotFound
		}
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
// link they give. It keeps what old's labels told, and the movie a
// torrent's file was listed for, read knowing the title's name, which a
// renewal does not load.
func (s *Service) renewed(t target, entry installed, streams []stremio.Stream, old Version) (Version, bool) {
	for _, stream := range streams {
		// The stream's identity tells it before its labels are read.
		if !listable(stream) || versionID(t.item, streamIdentity(stream)) != old.ID {
			continue
		}
		version := newVersion(t, streamSource{entry, t.metaType, t.id}, stream)
		version.Labels = old.Labels
		if version.Torrent != nil && old.Torrent != nil {
			version.Torrent.Title, version.Torrent.Year = old.Torrent.Title, old.Torrent.Year
		}
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
		Addon:      entry.addon.Name(),
		AddonLog:   entry.addon.LogName(),
		Origin:     Origin{Addon: entry.addon.ID, Type: source.contentType, ID: source.id},
		Confined:   entry.confined,
		Runtime:    t.runtime,
		Height:     LabelHeight(stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename),
		Labels:     ReadLabels(t.name, stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename),
		Cached:     stream.Data.Service.Cached,
		Seeders: cmp.Or(int(stream.Data.Torrent.Seeders), LabelSeeders(stream.Name, stream.Title, stream.Description,
			stream.BehaviorHints.Filename)),
		// A replay holds one of its provider's connections, as a channel does.
		HoldsConnection: t.kind == KindReplay,
	}
	version.Labels.Releases = ReleaseKinds([]string{t.name, t.videoName}, stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename)
	version.Labels.Tier, version.Labels.Audio, version.Labels.Edition = ReadRelease([]string{t.name, t.videoName}, stream.Name, stream.Title,
		stream.Description, stream.BehaviorHints.Filename)
	version.Labels.AudioCodecs = ReadAudioCodecs([]string{t.name, t.videoName}, stream.Name, stream.Title, stream.Description,
		stream.BehaviorHints.Filename)
	// A local folder's file is the episode its folder placed it at, which
	// its show's numbering may map from the numbers its name tells (see
	// localfiles' numbering.go): its name names no other title.
	if !entry.addon.Local() {
		version.Labels.Elsewhere = OtherTitle(t.name, t.season, t.episode, t.year, stream.Name, stream.Title, stream.Description,
			stream.BehaviorHints.Filename)
	}
	if stream.Torrent() {
		version.URL = torrentLink(stream)
		version.Torrent = &Torrent{InfoHash: strings.ToLower(stream.InfoHash), FileIdx: stream.FileIdx, Sources: slices.Clone(stream.Sources),
			Season: t.season, Episode: t.episode}
		if t.kind == KindMovie {
			version.Torrent.Title, version.Torrent.Year = t.name, t.year
		}
	}
	version.identity = streamIdentity(stream)
	version.ID = versionID(t.item, version.identity)
	// The subtitle files listed with a stream are its version's own: those
	// beside a local file, or those an addon gives with its stream.
	for _, subtitle := range stream.Subtitles {
		if !subtitle.Valid() {
			continue
		}
		version.Subtitles = append(version.Subtitles, ExternalSubtitle{ID: subtitleID(entry.addon.ID, subtitle), Language: subtitle.Lang,
			URL: subtitle.URL, Addon: entry.addon.Name(), Confined: entry.confined, Format: subtitle.Format, Title: subtitle.Title,
			Forced: subtitle.Forced, HearingImpaired: subtitle.HearingImpaired, Default: subtitle.Default})
	}
	// A stream whose address its source's owner gave is theirs to reach.
	if stream.Confined != nil {
		version.Confined = *stream.Confined
	}
	// A local folder's stream naming an address, a .strm file's, plays
	// as an addon's link does: it is no file of the folder's.
	version.LocalFolder = entry.addon.Local() && !stream.Remote
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
// notices alone, nothing to play, is not stored: it is remembered, as an
// answer with nothing to play is (see nothingAnswer), during which the
// callers get it again without asking the addon (see rememberedAnswer). A
// refresh of the title while the addon is asked leaves its answer out: the
// addon is asked again, for the callers waiting too (see forgetLists).
// Background work's request waits for its turn before it is shared (see
// streamTurn), so that a user's request never waits behind it.
func (s *Service) streamAnswer(ctx context.Context, entry installed, key streamKey) ([]stremio.Stream, error) {
	for {
		if streams, ok, err := s.rememberedAnswer(ctx, entry, key); ok {
			return streams, err
		}
		asking, release, answered, err := s.streamTurn(ctx, entry, key)
		if err != nil {
			return nil, err
		}
		if answered != nil {
			release()
			return answered()
		}
		streams, err := s.askStreamsOf(asking, entry, key)
		release()
		if errors.Is(err, errOnlyNotices) {
			// Shared requests pass no answer with an error: the list kept
			// is read here.
			if kept, _, ok := s.streamListOf(ctx, key); ok {
				streams = kept.items
			}
		}
		if !errors.Is(err, errForgotten) {
			return streams, err
		}
	}
}

// streamTurn waits, for background work's request for an addon's streams
// for a title, for its turn (see pacing), and returns the context to ask
// on, with what ends the turn. A request under way for them is joined at
// once instead. When another request answered while this one waited,
// answered returns that answer, as streamAnswer would. Any other request
// asks at once.
func (s *Service) streamTurn(ctx context.Context, entry installed, key streamKey) (asking context.Context, release func(),
	answered func() ([]stremio.Stream, error), err error) {
	if !paces(ctx, entry.addon) || s.askingFor(followKey{listStreams, key}) {
		return ctx, func() {}, nil, nil
	}
	waited := s.now()
	if asking, release, err = s.turn(ctx, entry.addon, paceStreams); err != nil {
		return ctx, nil, nil, err
	}
	if l, _, ok := s.streamListOf(ctx, key); ok && !l.at.Before(waited) {
		return asking, release, func() ([]stremio.Stream, error) { return l.items, nil }, nil
	}
	if streams, ok, err := s.rememberedAnswer(ctx, entry, key); ok {
		return asking, release, func() ([]stremio.Stream, error) { return streams, err }, nil
	}
	return asking, release, nil, nil
}

// askStreamsOf asks an addon for its streams for a title once, as
// streamAnswer does. A failure, and an answer of notices alone, are
// logged here, once whoever waits for it (see streamsFailed), and the
// notices remembered (see rememberedAnswer), as is the failure (see
// failedFor) until the addon answers, and an empty answer (see
// streamsListed).
func (s *Service) askStreamsOf(ctx context.Context, entry installed, key streamKey) ([]stremio.Stream, error) {
	flight := streamsFlight(key)
	s.streamLogs.waits(ctx, flight)
	return shared(ctx, s, flight, func(ctx context.Context) ([]stremio.Stream, error) {
		asked, done := s.ask(followKey{listStreams, key})
		defer done()
		s.logger.Debug("An addon is asked for streams", "addon", entry.addon.LogName(), "type", key.contentType, "id", key.id)
		streams, err := s.fetchStreams(ctx, entry, key.contentType, key.id)
		userWaited := s.streamLogs.ended(flight)
		if err != nil {
			s.streamsFailed(ctx, entry, key, userWaited, err)
			s.failedNow(key, asked)
			return nil, err
		}
		s.failedLately.Delete(key)
		if len(streams) > 0 && streamKind.count(streams) == 0 {
			return s.noticesAnswered(ctx, entry, key, asked, userWaited, streams)
		}
		listed, err := s.followStreams(ctx, entry, key, asked, streams)
		if err == nil {
			s.streamsListed(entry, key, asked, len(streams) == 0)
		}
		return listed, err
	})
}

// failedNow remembers, for failedFor, that an addon failed to list a
// title's streams to request asked (see ask), unless a refresh forgot the
// request meanwhile: the addon is asked anew then.
func (s *Service) failedNow(key streamKey, asked uint64) {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	if s.followUps.asking[followKey{listStreams, key}] == asked {
		s.failedLately.Put(key, struct{}{})
	}
}

// failed reports whether an addon failed to list a title's streams within
// failedFor, and has not answered since.
func (s *Service) failed(key streamKey) bool {
	_, ok := s.failedLately.Get(key)
	return ok
}

// noticeLife is how long an addon's answer of notices alone refusing the
// request for its rate (see limitedNotices) is remembered: it refuses at
// once, and asking it again at once would only prolong the refusal.
const noticeLife = time.Minute

// noticesAnswered remembers notices, an addon's answer of notices alone
// to request asked (see ask), and logs it (see noticesLogged), userWaited
// telling whether a user waited for it: for noticeLife when it refuses
// the request for its rate, else as an answer with nothing to play (see
// nothingNow). A request a refresh forgot meanwhile is errForgotten, its
// answer left out. It returns what rememberedAnswer returns.
func (s *Service) noticesAnswered(ctx context.Context, entry installed, key streamKey, asked uint64, userWaited bool,
	notices []stremio.Stream) ([]stremio.Stream, error) {
	limited := limitedNotices(notices)
	s.followUps.mu.Lock()
	if s.followUps.asking[followKey{listStreams, key}] != asked {
		s.followUps.mu.Unlock()
		return nil, errForgotten
	}
	if limited {
		s.noticed.Put(key, notices)
	} else {
		s.nothingNow(entry, key, notices)
	}
	s.followUps.mu.Unlock()
	streams, _, err := s.rememberedAnswer(ctx, entry, key)
	message := "An addon answered only notices, nothing to play"
	if err != nil {
		message = "An addon answered only notices, nothing to play; the streams it listed before are kept"
	}
	s.noticesLogged(ctx, entry, key, message, userWaited, limited, notices)
	return streams, err
}

// rememberedAnswer returns, while an addon's answer of notices alone for a
// title is remembered (ok), what the callers get instead of asking it:
// the list kept that has streams to play, with errOnlyNotices, else the
// notices. A refusal for the rate is remembered noticeLife, and an answer
// with nothing to play as nothingFor tells; an empty one, which is kept as
// its list, for background work only, which gets the list unless one with
// streams to play replaced it: freshness tells the others (see streams).
func (s *Service) rememberedAnswer(ctx context.Context, entry installed, key streamKey) (streams []stremio.Stream, ok bool, err error) {
	notices, ok := s.noticed.Get(key)
	if !ok {
		var nothing nothingAnswer
		if nothing, ok = s.nothingFor(ctx, entry, key); !ok {
			return nil, false, nil
		}
		notices = nothing.notices
	}
	kept, _, listed := s.streamListOf(ctx, key)
	playable := listed && streamKind.count(kept.items) > 0
	switch {
	case notices == nil && (!listed || playable || !markOf(ctx).background):
		return nil, false, nil
	case notices == nil:
		return kept.items, true, nil
	case playable:
		return kept.items, true, errOnlyNotices
	}
	return notices, true, nil
}

// noticeText is what notices tell, for a log line: each one's name, title
// and description, on one line, cut short and redacted. Their links are
// left out.
func noticeText(notices []stremio.Stream) string {
	const most = 300
	texts := make([]string, 0, len(notices))
	for _, notice := range notices {
		if text := notice.NoticeText(); text != "" {
			texts = append(texts, text)
		}
	}
	text := strings.Join(texts, " | ")
	if runes := []rune(text); len(runes) > most {
		text = string(runes[:most]) + "…"
	}
	return logs.Redact(text)
}

// errOnlyNotices is an addon's answer listing only notices for a title it
// listed streams for, such as a notice that it limits requests: it would
// list them again a moment later. The list kept stays as it was, and the
// callers get it with the error, the answer already logged once (see
// noticesAnswered). An empty answer is the addon having nothing for the
// title any more: it replaces the list as any answer does.
var errOnlyNotices = errors.New("its answer has only notices, nothing to play: the streams it listed before are kept")

// streamsFlight names the request asking an addon for its streams for a
// title, which callers share (see streamAnswer).
func streamsFlight(key streamKey) string {
	return "streams " + key.addon.String() + " " + key.contentType + " " + key.id
}

// serves reports whether an addon serves a resource for a title: a home
// video's and a plain video's are its folder's alone, whatever other
// addons declare, so that none of them ever hears of it; an extra's
// stream is its local folder's, which answers for its own extras only;
// an addon trailer is no addon's to answer for (see addontrailers.go).
func serves(entry installed, resource, metaType, id string) bool {
	if metaType == ExtraType || strings.HasPrefix(id, ExtraIDPrefix) {
		return entry.addon.Local() && resource == "stream"
	}
	if strings.HasPrefix(id, TrailerIDPrefix) {
		return false
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
	return s.subtitlesOf(ctx, user, id, SubtitleHints{}, knownOnly)
}

// shared runs fetch once for every caller asking for key at the same time,
// in s.flight. fetch runs detached from the callers, bounded by the
// timeouts of what it asks (the client's, for addons): a caller that stops
// waiting, when its ctx ends, does not cut it short for the others; the
// server stopping does (see detached). A caller that gave up already
// starts no fetch.
func shared[T any](ctx context.Context, s *Service, key string, fetch func(context.Context) (T, error)) (T, error) {
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	answer := s.doShared(key, func() (any, error) {
		detached, done := s.detached(ctx)
		defer done()
		return fetch(detached)
	})
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
// streams of a title. Background work's requests to an addon are paced
// (see request), and an answer of notices alone saying the addon refused
// it for its rate pauses them.
func (s *Service) fetchStreams(ctx context.Context, entry installed, contentType, id string) ([]stremio.Stream, error) {
	switch {
	case entry.addon.Stremio():
		streams, err := request(ctx, s, entry.addon, paceStreams, func(ctx context.Context) ([]stremio.Stream, error) {
			return s.client.Streams(ctx, entry.addon.ManifestURL, contentType, id, entry.confined)
		})
		if len(streams) > 0 && streamKind.count(streams) == 0 && refusedNotices(streams) {
			s.refusedFor(entry.addon, paceWait)
		}
		return streams, err
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
	carried, _ := carriedIdentities(answer, before)
	return carried
}

// carryCost is what carrying identities took: the links parsed, and the
// pairs of streams compared (see sameFile).
type carryCost struct{ parsed, compared int }

// carriedIdentities is carryIdentities, and tells what it took. An addon
// lists as many streams as it likes, and Polyfin compares its answers by
// itself (see followUpList): so each link is parsed once, and each stream
// compared with a few others (see sameFiles), whatever the lists hold.
func carriedIdentities(answer, before []stremio.Stream) (carried []stremio.Stream, cost carryCost) {
	listed := make(map[string]bool, len(answer))
	for _, stream := range answer {
		listed[streamIdentity(stream)] = true
	}
	known := make(map[string]bool, len(before))
	// gone are the streams of before the answer no longer lists, and
	// added, below, those of answer that may be one of them: both by
	// their place in their list.
	var gone []int
	for j, stream := range before {
		identity := streamIdentity(stream)
		if !known[identity] && !listed[identity] && listable(stream) {
			gone = append(gone, j)
		}
		known[identity] = true
	}
	if len(gone) == 0 {
		return answer, cost
	}
	var added []int
	for i, stream := range answer {
		if listable(stream) && !known[streamIdentity(stream)] {
			added = append(added, i)
		}
	}
	was, is := make([]streamFile, len(gone)), make([]streamFile, len(added))
	for n, j := range gone {
		was[n] = fileOf(before[j], &cost)
	}
	for n, i := range added {
		is[n] = fileOf(answer[i], &cost)
	}
	// match holds, for each stream added, the streams of gone it may be,
	// and claims, for each stream of gone, the streams added that may be
	// it: two of them when there are more. A stream that may be several,
	// or that several may be, keeps its own identity: none is picked for
	// it, the first found no more than another.
	match, claims := sameFiles(was, is, &cost), sameFiles(is, was, &cost)
	for n, i := range added {
		m := match[n].of[0]
		if match[n].count != 1 || claims[m].count != 1 {
			continue
		}
		if carried == nil {
			carried = slices.Clone(answer)
		}
		carried[i].Identity = streamIdentity(before[gone[m]])
	}
	if carried == nil {
		return answer, cost
	}
	return carried, cost
}

// streamFile is what tells the file of a stream (see sameFile), its link
// parsed once.
type streamFile struct {
	// torrent tells a torrent's file, whose link is its magnet link (see
	// torrentLink). Another stream's link is its address, of which host
	// and last are the host and the last part of the path (see linkFile).
	torrent          bool
	link, host, last string
	// filename and size are the file's, as the addon tells them: "" and 0
	// when it does not.
	filename string
	size     int64
	// name is the stream's name and description (see versionName), for an
	// address with a last part.
	name string
}

// fileOf is what tells the file of stream.
func fileOf(stream stremio.Stream, cost *carryCost) streamFile {
	file := streamFile{torrent: stream.Torrent(), link: stream.URL,
		filename: stream.BehaviorHints.Filename, size: max(int64(stream.BehaviorHints.VideoSize), 0)}
	if file.torrent {
		file.link = torrentLink(stream)
		return file
	}
	cost.parsed++
	if file.host, file.last = linkFile(stream.URL); file.last != "" {
		file.name = versionName(stream)
	}
	return file
}

// The ways two streams are the same file (see sameFile), each with what
// both streams share then (see sameKey): the magnet link of two torrents'
// files; else the address; the file's name; the address's host and the
// last part of its path, with the stream's name and description; that
// host and last part, which is the file's name as one of the two streams
// tells it (see streamFile.keys).
const (
	sameMagnet = iota
	sameAddress
	sameFilename
	sameName
	sameLastNamed
	sameLast
)

// sameKey is what two streams share when they are the same file in one of
// the ways above.
type sameKey struct {
	way   int
	texts [3]string
}

// keys appends to keys what f shares with the streams that are its file,
// one key for each way sameFile tells: the keys to index f under or,
// looking, those to look for the streams indexed.
func (f streamFile) keys(keys []sameKey, looking bool) []sameKey {
	if f.torrent {
		return append(keys, sameKey{sameMagnet, [3]string{f.link}})
	}
	keys = append(keys, sameKey{sameAddress, [3]string{f.link}})
	if f.filename != "" {
		keys = append(keys, sameKey{sameFilename, [3]string{f.filename}})
	}
	if f.last == "" {
		return keys
	}
	keys = append(keys, sameKey{sameName, [3]string{f.host, f.last, f.name}})
	// The last part of the path is the file's name as one of the two
	// streams tells it. The streams that tell it are indexed apart, where
	// every stream looks for them, and they look among all the streams.
	named := f.filename == f.last
	if named || looking {
		keys = append(keys, sameKey{sameLastNamed, [3]string{f.host, f.last}})
	}
	if named || !looking {
		keys = append(keys, sameKey{sameLast, [3]string{f.host, f.last}})
	}
	return keys
}

// pair holds two streams at most, by their place in a list: the first two
// added.
type pair struct {
	count int
	of    [2]int
}

// add adds the stream at place i, unless p holds it, or two others.
func (p *pair) add(i int) {
	if p.count < len(p.of) && (p.count == 0 || p.of[0] != i) {
		p.of[p.count] = i
		p.count++
	}
}

// sharers are the streams that share a key, by their place in their list:
// the first two and, once there are more, all of them by class, two at
// most for each (see fileClass).
type sharers struct {
	first   pair
	classes map[fileClass]pair
}

// fileClass tells apart the streams that share a key by what else
// sameFile compares, the file's name and size: a class holds the streams
// that tell its filename and its size, whatever name they tell when
// anyName is set, whatever size when anySize is.
type fileClass struct {
	filename         string
	size             int64
	anyName, anySize bool
}

// add adds the stream at place i of files.
func (s *sharers) add(i int, files []streamFile) {
	if s.classes == nil && s.first.count < len(s.first.of) {
		s.first.add(i)
		return
	}
	if s.classes == nil {
		s.classes = make(map[fileClass]pair)
		for _, j := range s.first.of {
			s.classify(j, files[j])
		}
	}
	s.classify(i, files[i])
}

// classify adds the stream at place i, whose file is file, to the four
// classes that hold it.
func (s *sharers) classify(i int, file streamFile) {
	for _, class := range [...]fileClass{
		{file.filename, file.size, false, false},
		{"", file.size, true, false},
		{file.filename, 0, false, true},
		{"", 0, true, true},
	} {
		p := s.classes[class]
		p.add(i)
		s.classes[class] = p
	}
}

// allowed appends to among the streams of s that file may be: both while s
// holds no more, else two at most of each class whose name and size agree
// with file's (see sameFile): its own or none, or any where file tells
// none. Every stream of these classes agrees with file, so two of each
// tell whether file has one match, or several.
func (s sharers) allowed(among []int, file streamFile) []int {
	if s.classes == nil {
		return append(among, s.first.of[:s.first.count]...)
	}
	names, sizes := []string{file.filename, ""}, []int64{file.size, 0}
	anyName, anySize := file.filename == "", file.size == 0
	if anyName {
		names = names[1:]
	}
	if anySize {
		sizes = sizes[1:]
	}
	for _, filename := range names {
		for _, size := range sizes {
			p := s.classes[fileClass{filename, size, anyName, anySize}]
			among = append(among, p.of[:p.count]...)
		}
	}
	return among
}

// sameFiles returns, for each of streams, which of others are its file
// (see sameFile), by their place in others: two of them when there are
// more. A stream is compared with the others that share a key with it
// (see streamFile.keys), a few of them however many do (see
// sharers.allowed): the work grows as the streams, not as their product.
func sameFiles(others, streams []streamFile, cost *carryCost) []pair {
	index := make(map[sameKey]sharers, len(others))
	var keys []sameKey
	for i, other := range others {
		keys = other.keys(keys[:0], false)
		for _, key := range keys {
			sharing := index[key]
			sharing.add(i, others)
			index[key] = sharing
		}
	}
	found := make([]pair, len(streams))
	var among []int
	for n, stream := range streams {
		keys = stream.keys(keys[:0], true)
		for _, key := range keys {
			among = index[key].allowed(among[:0], stream)
			for _, i := range among {
				cost.compared++
				if sameFile(others[i], stream) {
					found[n].add(i)
				}
			}
		}
	}
	return found
}

// sameFile tells whether b, in an addon's answer, is the file a was in the
// list the answer replaces, under another identity. The file's names and
// sizes must agree where both streams tell them; then either the link is
// the same, the addon telling the name or size only now, or so is the
// file's name, or the link differs by a token only: it keeps its host and
// the last part of its path, and either that part is the file's name, as
// one of the streams tells it, or the stream keeps its name and
// description.
func sameFile(a, b streamFile) bool {
	if a.filename != "" && b.filename != "" && a.filename != b.filename || a.size > 0 && b.size > 0 && a.size != b.size {
		return false
	}
	if a.torrent || b.torrent {
		// A torrent's file is the same one at the same magnet link alone.
		return a.torrent && b.torrent && a.link == b.link
	}
	if a.link == b.link || a.filename != "" && a.filename == b.filename {
		return true
	}
	if a.last == "" || a.host != b.host || a.last != b.last {
		return false
	}
	return a.last == a.filename || a.last == b.filename || a.name == b.name
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
	subtitles, _, err := s.subtitlesOf(ctx, user, id, SubtitleHints{}, waitAll)
	return subtitles, err
}

// SubtitlesNow lists, as Subtitles does, the subtitle files known now,
// without waiting for addons: it asks those whose lists are not remembered,
// or stale, in the background, for later requests, a stale list being
// listed meanwhile. complete reports whether every addon's list was known,
// none asked again.
func (s *Service) SubtitlesNow(ctx context.Context, user accounts.User, id accounts.ID) (subtitles []ExternalSubtitle, complete bool, err error) {
	return s.subtitlesOf(ctx, user, id, SubtitleHints{}, askLater)
}

func (s *Service) subtitlesOf(ctx context.Context, user accounts.User, id accounts.ID, hints SubtitleHints, mode listing) ([]ExternalSubtitle, bool, error) {
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
				subtitles, err := s.subtitleList(ctx, entry, t, hints)
				if err != nil && hints != (SubtitleHints{}) && ctx.Err() == nil {
					subtitles, err = s.subtitleList(ctx, entry, t, SubtitleHints{})
				}
				if err != nil && ctx.Err() == nil {
					s.logger.Warn("An addon could not list subtitles", "addon", entry.addon.LogName(), "error", err)
				}
				lists[i] = subtitles
			})
		}
		wg.Wait()
	} else {
		for i, entry := range serving {
			l, fresh, ok := listOf(s, s.subtitleLists, streamKey{entry.addon.ID, t.metaType, t.id})
			lists[i] = l.items
			// A stale list is listed until the addon answers again.
			if !ok || !fresh && mode == askLater {
				complete = false
				if mode == askLater {
					go func() {
						detached, done := s.detached(ctx)
						defer done()
						if _, err := s.subtitleList(detached, entry, t, SubtitleHints{}); err != nil && detached.Err() == nil {
							s.logger.Warn("An addon could not list subtitles", "addon", entry.addon.LogName(), "error", err)
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
				Addon: serving[i].addon.Name(), Confined: serving[i].confined})
		}
	}
	return result, complete, nil
}

// subtitleList lists an addon's subtitles for a title, asked with hints
// (see subtitlesKey): those remembered while fresh, else its answer, which
// every caller asking meanwhile shares. An answer stored is followed up;
// until the follow-ups end, it is listed with the items it lacks of the
// stale list it replaced (see followUpList). A refresh of the title while
// the addon is asked leaves its answer out: the addon is asked again (see
// forgetLists).
func (s *Service) subtitleList(ctx context.Context, entry installed, t target, hints SubtitleHints) ([]stremio.Subtitle, error) {
	key := subtitlesKey(entry.addon.ID, t, hints)
	if l, fresh, _ := listOf(s, s.subtitleLists, key); fresh {
		return l.items, nil
	}
	for {
		subtitles, err := shared(ctx, s, subtitlesFlight(key), func(ctx context.Context) ([]stremio.Subtitle, error) {
			asked, done := s.ask(followKey{listSubtitles, key})
			defer done()
			subtitles, err := s.fetchSubtitles(ctx, entry, key.contentType, key.id)
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

// fetchSubtitles asks an addon for the subtitles of a title, with the
// hints its list is kept under (see subtitlesKey).
func (s *Service) fetchSubtitles(ctx context.Context, entry installed, contentType, id string) ([]stremio.Subtitle, error) {
	id, extra := subtitlesAsked(id)
	return s.client.Subtitles(ctx, entry.addon.ManifestURL, contentType, id, extra, entry.confined)
}
