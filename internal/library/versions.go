package library

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
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
	Headers  map[string]string
	Filename string
	Size     int64
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
	// Expires is when the link stops working, zero when the addon does
	// not say; Audio is what a music addon tells of a track's stream.
	Expires time.Time
	Audio   *AudioSource
}

// Origin is the addon that listed a stream and what it listed it for.
type Origin struct {
	Addon    accounts.ID
	Type, ID string
}

// ExternalSubtitle is a subtitle file offered for an item: by an addon, or
// added by a user (see UploadSubtitle).
type ExternalSubtitle struct {
	ID accounts.ID
	// Language is the addon's code, usually ISO 639-2 ("fre").
	Language string
	URL      string
	Addon    string
	Confined bool
	// Uploaded is set for a file a user added, whose text Polyfin keeps
	// (see UploadedSubtitleText), in Format, with its flags.
	Uploaded        bool
	Format          string
	Forced          bool
	HearingImpaired bool
}

// target is what to ask addons for: a title or an episode.
type target struct {
	item     accounts.ID
	kind     Kind
	metaType string
	id       string
	runtime  time.Duration
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
	if err != nil {
		return target{}, view{}, err
	}
	if kept, _ := s.visible(ctx, v, []record{r}); len(kept) == 0 {
		return target{}, view{}, ErrNotFound
	}
	switch {
	case AudioKind(r.Kind) && r.Music != nil:
		if !v.allowsMusic(r) {
			return target{}, view{}, ErrNotFound
		}
		return target{item: id, kind: r.Kind, metaType: originEclipse, id: r.Music.ID}, v, nil
	case r.Kind == KindMovie && r.Meta != nil:
		meta := r.Meta
		if full, ok := s.cachedMeta(r); ok {
			meta = &full
		}
		return target{item: id, kind: KindMovie, metaType: meta.Type, id: meta.ID, runtime: parseRuntime(string(meta.Runtime))}, v, nil
	case r.Kind == KindChannel && r.Meta != nil:
		return target{item: id, kind: KindChannel, metaType: r.Meta.Type, id: r.Meta.ID}, v, nil
	case r.Kind == KindEpisode && r.Video != nil:
		series, err := s.load(ctx, r.seriesItemID())
		if err != nil || series.Meta == nil {
			return target{}, view{}, ErrNotFound
		}
		return target{item: id, kind: KindEpisode, metaType: series.Meta.Type, id: r.Video.ID, runtime: parseRuntime(string(r.Video.Runtime))}, v, nil
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
	serving := streamServing(v, t)
	tracked := s.tracked()
	tracked.titles.Put(listedTitle{t.metaType, t.id}, t.item)
	if mode == askLater {
		// Item details: the title's page follows its versions from now on
		// (see FollowedProgress).
		tracked.pages.Put(askedKey{user.ID, t.item}, titlePage{t: t, serving: serving})
	}
	versions, unknown := s.listVersions(ctx, t, serving, mode)
	if mode == askLater || mode == staleNow {
		s.askStreams(ctx, user, t, unknown)
	}
	return versions, len(unknown) == 0, nil
}

// listVersions lists the versions serving list for t, as far as mode goes,
// and returns the addons whose streams are not known, or are asked again:
// for knownOnly, those whose lists are not remembered; for askLater, those
// and those whose lists are stale; for staleNow, those whose lists are
// stale; for waitAll, none.
func (s *Service) listVersions(ctx context.Context, t target, serving []installed, mode listing) ([]Version, []installed) {
	lists := make([][]stremio.Stream, len(serving))
	var unknown []installed
	var wg sync.WaitGroup
	for i, entry := range serving {
		// An IPTV source's streams are Polyfin's own: reading them asks
		// no addon.
		if entry.addon.IPTV() && mode != waitAll {
			lists[i], _ = s.fetchStreams(ctx, entry, t.metaType, t.id)
			continue
		}
		var l list[stremio.Stream]
		var fresh, ok bool
		if mode != waitAll {
			l, fresh, ok = s.streamListOf(ctx, streamKey{entry.addon.ID, t.metaType, t.id})
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
				streams, err := s.streams(ctx, entry, t.metaType, t.id)
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
	seen := map[accounts.ID]bool{}
	for i, streams := range lists {
		for _, stream := range streams {
			if !stream.Playable() {
				continue
			}
			version := newVersion(t, serving[i], stream)
			if seen[version.ID] {
				continue
			}
			seen[version.ID] = true
			versions = append(versions, version)
			s.versions.Put(version.ID, version)
		}
	}
	return versions, unknown
}

// Version finds a version of an item, listing the item's versions again
// when it is no longer remembered. The item's own identifier stands for its
// first version, as Jellyfin apps name a single-version item's source after
// the item.
func (s *Service) Version(ctx context.Context, user accounts.User, item, id accounts.ID) (Version, error) {
	if version, ok := s.versions.Get(id); ok && version.Item == item && version.fresh(s.now()) {
		// A version remembered from another user's listing is no way around
		// the user's parental control or blocked genres.
		if user.Restricted() {
			if _, _, err := s.target(ctx, user, item); err != nil {
				return Version{}, err
			}
		}
		return version, nil
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
		if err != nil || r.Music == nil || !addon.Eclipse() {
			return Version{}, ErrNotFound
		}
		return s.resolveTrack(ctx, entry, r, true)
	}
	t := target{item: old.Item, metaType: old.Origin.Type, id: old.Origin.ID, runtime: old.Runtime}
	if addon.IPTV() {
		streams, err := s.fetchStreams(ctx, entry, t.metaType, t.id)
		if err != nil {
			return Version{}, err
		}
		if version, ok := s.renewed(t, entry, streams, old.ID); ok {
			return version, nil
		}
		return Version{}, ErrNotFound
	}
	key := streamKey{addon.ID, t.metaType, t.id}
	s.tracked().titles.Put(listedTitle{t.metaType, t.id}, t.item)
	if _, err := s.streamAnswer(ctx, entry, key); err != nil {
		return Version{}, err
	}
	if version, ok := s.renewedFromAnswer(t, entry, key, old.ID); ok {
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
	if version, ok := s.renewedFromAnswer(t, entry, key, old.ID); ok {
		return version, nil
	}
	return Version{}, ErrNotFound
}

// renewedFromAnswer finds the version id among the streams of the addon's
// last answer kept under key.
func (s *Service) renewedFromAnswer(t target, entry installed, key streamKey, id accounts.ID) (Version, bool) {
	l, ok := s.streamLists.Get(key)
	if !ok {
		return Version{}, false
	}
	return s.renewed(t, entry, l.items[:l.answer], id)
}

// renewed finds the version id among streams, and remembers it with the
// link they give.
func (s *Service) renewed(t target, entry installed, streams []stremio.Stream, id accounts.ID) (Version, bool) {
	for _, stream := range streams {
		if !stream.Playable() {
			continue
		}
		if version := newVersion(t, entry, stream); version.ID == id {
			s.versions.Put(version.ID, version)
			return version, true
		}
	}
	return Version{}, false
}

// newVersion describes a stream an addon listed for a target.
func newVersion(t target, entry installed, stream stremio.Stream) Version {
	version := Version{
		Item:     t.item,
		Name:     versionName(stream),
		URL:      stream.URL,
		Headers:  stream.RequestHeaders(),
		Filename: stream.BehaviorHints.Filename,
		Size:     int64(stream.BehaviorHints.VideoSize),
		Addon:    entry.addon.Manifest.Name,
		Origin:   Origin{Addon: entry.addon.ID, Type: t.metaType, ID: t.id},
		Confined: entry.confined,
		Runtime:  t.runtime,
		Height:   LabelHeight(stream.Name, stream.Title, stream.Description, stream.BehaviorHints.Filename),
	}
	version.ID = versionID(t.item, version)
	return version
}

// streams lists an addon's streams for a title: those remembered while
// fresh, else its answer (see streamAnswer).
func (s *Service) streams(ctx context.Context, entry installed, contentType, id string) ([]stremio.Stream, error) {
	// An IPTV source's line-up changes its streams at once.
	if entry.addon.IPTV() {
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
// lacks of the stale list it replaced (see followUpList).
func (s *Service) streamAnswer(ctx context.Context, entry installed, key streamKey) ([]stremio.Stream, error) {
	return shared(ctx, &s.flight, streamsFlight(key), func(ctx context.Context) ([]stremio.Stream, error) {
		streams, err := s.fetchStreams(ctx, entry, key.contentType, key.id)
		if err != nil {
			return nil, err
		}
		return s.followStreams(ctx, entry, key, streams), nil
	})
}

// streamsFlight names the request asking an addon for its streams for a
// title, which callers share (see streamAnswer).
func streamsFlight(key streamKey) string {
	return "streams " + key.addon.String() + " " + key.contentType + " " + key.id
}

// streamServing lists the addons of v that list streams for t.
func streamServing(v view, t target) []installed {
	var serving []installed
	for _, entry := range v.addons {
		// An IPTV source's channel is only its source's to play.
		if entry.addon.Manifest.Serves("stream", t.metaType, t.id) && !(entry.addon.Stremio() && strings.HasPrefix(t.id, iptv.IDPrefix)) {
			serving = append(serving, entry)
		}
	}
	return serving
}

// KnownSubtitles lists, as Subtitles does, the subtitle files known now,
// stale lists' included, asking no addon. complete reports whether every
// addon's list was known.
func (s *Service) KnownSubtitles(ctx context.Context, user accounts.User, id accounts.ID) (subtitles []ExternalSubtitle, complete bool, err error) {
	return s.subtitlesOf(ctx, user, id, knownOnly)
}

// shared runs fetch once for every caller asking for key at the same time.
// fetch runs detached from the callers, bounded by the timeouts of what it
// asks (the client's, for addons): a caller that stops waiting, when its
// ctx ends, does not cut it short for the others.
func shared[T any](ctx context.Context, flight *singleflight.Group, key string, fetch func(context.Context) (T, error)) (T, error) {
	detached := context.WithoutCancel(ctx)
	answer := flight.DoChan(key, func() (any, error) { return fetch(detached) })
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

// fetchStreams asks an addon, or an IPTV source, for the streams of a
// title.
func (s *Service) fetchStreams(ctx context.Context, entry installed, contentType, id string) ([]stremio.Stream, error) {
	if entry.addon.Stremio() {
		return s.client.Streams(ctx, entry.addon.ManifestURL, contentType, id, entry.confined)
	}
	if s.iptv == nil {
		return nil, nil
	}
	return s.iptv.Streams(ctx, entry.addon.ID, id)
}

// versionID identifies a stream of an item by its file (see fileIdentity).
func versionID(item accounts.ID, version Version) accounts.ID {
	return itemID("version|" + item.String() + "|" + fileIdentity(version.URL, version.Filename, version.Size))
}

// streamIdentity is the file a stream is, the same in every answer of its
// addon however its link changes (see fileIdentity).
func streamIdentity(stream stremio.Stream) string {
	return fileIdentity(stream.URL, stream.BehaviorHints.Filename, int64(stream.BehaviorHints.VideoSize))
}

// fileIdentity tells a file by its name and size when the addon names it,
// and by its URL otherwise.
func fileIdentity(url, filename string, size int64) string {
	if filename != "" && size > 0 {
		return filename + "|" + strconv.FormatInt(size, 10)
	}
	return url
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
		if entry.addon.Manifest.Serves("subtitles", t.metaType, t.id) {
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
			sum := sha256.Sum256([]byte(serving[i].addon.ID.String() + "|" + subtitleIdentity(subtitle)))
			var subtitleID accounts.ID
			copy(subtitleID[:], sum[:16])
			if seen[subtitleID] {
				continue
			}
			seen[subtitleID] = true
			result = append(result, ExternalSubtitle{ID: subtitleID, Language: subtitle.Lang, URL: subtitle.URL,
				Addon: serving[i].addon.Manifest.Name, Confined: serving[i].confined})
		}
	}
	return result, complete, nil
}

// subtitleList lists an addon's subtitles for a title: those remembered
// while fresh, else its answer, which every caller asking meanwhile shares.
// An answer stored is followed up; until the follow-ups end, it is listed
// with the items it lacks of the stale list it replaced (see followUpList).
func (s *Service) subtitleList(ctx context.Context, entry installed, t target) ([]stremio.Subtitle, error) {
	key := streamKey{entry.addon.ID, t.metaType, t.id}
	if l, fresh, _ := listOf(s, s.subtitleLists, key); fresh {
		return l.items, nil
	}
	return shared(ctx, &s.flight, "subtitles "+key.addon.String()+" "+t.metaType+" "+t.id, func(ctx context.Context) ([]stremio.Subtitle, error) {
		subtitles, err := s.fetchSubtitles(ctx, entry, t.metaType, t.id)
		if err != nil {
			return nil, err
		}
		return s.followSubtitles(ctx, entry, key, subtitles), nil
	})
}

// subtitleIdentity is the file a subtitle is among those of its addon.
func subtitleIdentity(subtitle stremio.Subtitle) string {
	return string(subtitle.ID) + "|" + subtitle.URL
}

// fetchSubtitles asks an addon for the subtitles of a title.
func (s *Service) fetchSubtitles(ctx context.Context, entry installed, contentType, id string) ([]stremio.Subtitle, error) {
	return s.client.Subtitles(ctx, entry.addon.ManifestURL, contentType, id, nil, entry.confined)
}
