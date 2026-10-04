package library

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	streamsTTL  = 10 * time.Minute
	versionsTTL = 12 * time.Hour
)

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
}

// Origin is the addon that listed a stream and what it listed it for.
type Origin struct {
	Addon    accounts.ID
	Type, ID string
}

// ExternalSubtitle is a subtitle file an addon offers for an item.
type ExternalSubtitle struct {
	ID accounts.ID
	// Language is the addon's code, usually ISO 639-2 ("fre").
	Language string
	URL      string
	Addon    string
	Confined bool
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
	if len(s.visible(ctx, v, []record{r})) == 0 {
		return target{}, view{}, ErrNotFound
	}
	switch {
	case r.Kind == KindMovie && r.Meta != nil:
		meta := r.Meta
		if full, ok := s.cachedMeta(r); ok {
			meta = &full
		}
		return target{item: id, kind: KindMovie, metaType: meta.Type, id: meta.ID, runtime: parseRuntime(string(meta.Runtime))}, v, nil
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
// addons, in addon order, each file once.
func (s *Service) Versions(ctx context.Context, user accounts.User, id accounts.ID) ([]Version, error) {
	versions, _, err := s.versionsOf(ctx, user, id, true)
	return versions, err
}

// CachedVersions lists an item's versions only if every addon's streams for
// it are still remembered, without asking the addons: listings must not
// ask addons for the streams of every title they show.
func (s *Service) CachedVersions(ctx context.Context, user accounts.User, id accounts.ID) ([]Version, bool) {
	versions, complete, err := s.versionsOf(ctx, user, id, false)
	return versions, err == nil && complete
}

func (s *Service) versionsOf(ctx context.Context, user accounts.User, id accounts.ID, fetch bool) ([]Version, bool, error) {
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return nil, false, err
	}
	var serving []installed
	for _, entry := range v.addons {
		if entry.addon.Manifest.Serves("stream", t.metaType, t.id) {
			serving = append(serving, entry)
		}
	}
	lists := make([][]stremio.Stream, len(serving))
	complete := true
	if fetch {
		var wg sync.WaitGroup
		for i, entry := range serving {
			wg.Go(func() {
				streams, err := s.streams(ctx, entry, t.metaType, t.id)
				// An app that stops waiting cancels ctx: nothing failed.
				if err != nil && ctx.Err() == nil {
					s.logger.Warn("An addon could not list streams", "addon", entry.addon.Manifest.Name, "error", err)
				}
				lists[i] = streams
			})
		}
		wg.Wait()
	} else {
		for i, entry := range serving {
			streams, ok := s.streamLists.Get(streamKey{entry.addon.ID, t.metaType, t.id})
			lists[i], complete = streams, complete && ok
		}
	}
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
	return versions, complete, nil
}

// Version finds a version of an item, listing the item's versions again
// when it is no longer remembered. The item's own identifier stands for its
// first version, as Jellyfin apps name a single-version item's source after
// the item.
func (s *Service) Version(ctx context.Context, user accounts.User, item, id accounts.ID) (Version, error) {
	if version, ok := s.versions.Get(id); ok && version.Item == item {
		// A version remembered from another user's listing is no way around
		// the user's parental control.
		if user.Parental.Restricted() {
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

// Renew asks the addon that listed a version for its streams again and
// returns the same file with the link the addon gives now, for links that
// expired. Every listing of the version uses the new link afterwards.
func (s *Service) Renew(ctx context.Context, old Version) (Version, error) {
	addon, err := s.addons.Find(ctx, old.Origin.Addon)
	if errors.Is(err, addons.ErrNotFound) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	entry := installed{addon: addon, confined: old.Confined}
	streams, err := s.client.Streams(ctx, addon.ManifestURL, old.Origin.Type, old.Origin.ID, old.Confined)
	if err != nil {
		return Version{}, err
	}
	s.streamLists.Put(streamKey{addon.ID, old.Origin.Type, old.Origin.ID}, streams)
	t := target{item: old.Item, metaType: old.Origin.Type, id: old.Origin.ID, runtime: old.Runtime}
	for _, stream := range streams {
		if !stream.Playable() {
			continue
		}
		if version := newVersion(t, entry, stream); version.ID == old.ID {
			s.versions.Put(version.ID, version)
			return version, nil
		}
	}
	return Version{}, ErrNotFound
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
	}
	version.ID = versionID(t.item, version)
	return version
}

func (s *Service) streams(ctx context.Context, entry installed, contentType, id string) ([]stremio.Stream, error) {
	key := streamKey{entry.addon.ID, contentType, id}
	if streams, ok := s.streamLists.Get(key); ok {
		return streams, nil
	}
	result, err, _ := s.flight.Do("streams "+key.addon.String()+" "+contentType+" "+id, func() (any, error) {
		streams, err := s.client.Streams(ctx, entry.addon.ManifestURL, contentType, id, entry.confined)
		if err != nil {
			return nil, err
		}
		s.streamLists.Put(key, streams)
		return streams, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]stremio.Stream), nil
}

// versionID identifies a stream of an item by its file when the addon names
// it, and by its URL otherwise.
func versionID(item accounts.ID, version Version) accounts.ID {
	identity := version.URL
	if version.Filename != "" && version.Size > 0 {
		identity = version.Filename + "|" + strconv.FormatInt(version.Size, 10)
	}
	return itemID("version|" + item.String() + "|" + identity)
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

// Subtitles lists the subtitle files the user's addons offer for a movie or
// an episode, in addon order.
func (s *Service) Subtitles(ctx context.Context, user accounts.User, id accounts.ID) ([]ExternalSubtitle, error) {
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return nil, err
	}
	var serving []installed
	for _, entry := range v.addons {
		if entry.addon.Manifest.Serves("subtitles", t.metaType, t.id) {
			serving = append(serving, entry)
		}
	}
	lists := make([][]stremio.Subtitle, len(serving))
	var wg sync.WaitGroup
	for i, entry := range serving {
		wg.Go(func() {
			key := streamKey{entry.addon.ID, t.metaType, t.id}
			if subtitles, ok := s.subtitleLists.Get(key); ok {
				lists[i] = subtitles
				return
			}
			subtitles, err := s.client.Subtitles(ctx, entry.addon.ManifestURL, t.metaType, t.id, nil, entry.confined)
			if err != nil {
				if ctx.Err() == nil {
					s.logger.Warn("An addon could not list subtitles", "addon", entry.addon.Manifest.Name, "error", err)
				}
				return
			}
			s.subtitleLists.Put(key, subtitles)
			lists[i] = subtitles
		})
	}
	wg.Wait()
	var result []ExternalSubtitle
	seen := map[accounts.ID]bool{}
	for i, subtitles := range lists {
		for _, subtitle := range subtitles {
			if !subtitle.Valid() {
				continue
			}
			sum := sha256.Sum256([]byte(serving[i].addon.ID.String() + "|" + string(subtitle.ID) + "|" + subtitle.URL))
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
	return result, nil
}
