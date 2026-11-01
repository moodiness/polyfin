package library

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Titles of the Stremio types other and channel are neither movies nor
// series: the files of a cloud store, a channel's uploads, a recording.
// So are those of every type Polyfin has no use of its own for, such as
// an addon's sport, which Stremio apps show as they show other.
// Stremio apps open such a title to list its videos, each played by its
// own streams, or play the title itself when it has no videos. Polyfin
// does the same: a title whose description has videos is a folder of them
// (KindFolder, Jellyfin's Folder), each a video (KindVideo, Jellyfin's
// Video) that plays the streams the description embeds, then those the
// stream resource lists for it; a title without videos is a video itself.
// Their libraries are Jellyfin's homevideos, whose folders and videos
// every Jellyfin app browses and plays, unless the library shows them as
// movies and shows (see showastitles.go).
//
// Catalogs give no videos, only their descriptions do: a listing looks
// them up (see shapeOthers). Either way a title keeps the identifier of a
// video, derived from its Stremio identifier, so that it stays the same
// item when its description gains or loses videos.

// otherType reports whether titles of a Stremio type are videos or folders
// of videos, as their descriptions tell (see addons.OtherType).
func otherType(stremioType string) bool { return addons.OtherType(stremioType) }

// AddonVideo reports whether item is a video an addon lists as a title of
// the other or channel type, or of a type Polyfin has no use of its own
// for, or as one of such a title's videos (see otherType), shown as a
// video, or as a movie or an episode (see showastitles.go).
func AddonVideo(item Item) bool {
	return (item.Kind == KindVideo || item.Kind == KindMovie || item.Kind == KindEpisode) && otherType(item.StremioType)
}

// AddonVideoVersion reports whether version is a stream of such a video,
// listed for a title of one of those types.
func AddonVideoVersion(version Version) bool {
	return version.Torrent == nil && version.Origin.Type != originEclipse && otherType(version.Origin.Type)
}

// otherTitle reports whether r is a title of the other or channel type,
// listed by an addon.
func (r record) otherTitle() bool {
	return (r.Kind == KindVideo || r.Kind == KindFolder) && r.Meta != nil && otherType(r.Meta.Type)
}

// shapedTitle reports whether r is a title of the other or channel type
// whose kind its description gives: shown as a folder or a video, or as a
// series or a movie (see shownTitle).
func (r record) shapedTitle() bool { return r.otherTitle() || r.shownTitle() }

// homeVideo reports whether r is a video of a local home videos folder.
func (r record) homeVideo() bool {
	return r.Kind == KindVideo && r.Meta != nil && !otherType(r.Meta.Type)
}

// plainVideo reports whether r is a plain video of a local movies or shows
// folder, a file or show it left unmatched (see PlainIDPrefix), or one of
// such a show's seasons and episodes, and returns the identifier of that
// movie or show.
func (r record) plainVideo() (string, bool) {
	id := r.SeriesID
	if id == "" && r.Meta != nil {
		id = r.Meta.ID
	}
	return id, strings.HasPrefix(id, PlainIDPrefix)
}

// otherKind is the kind a description gives a title of the other or
// channel type: a folder when it has videos, else a video.
func otherKind(meta stremio.Meta) Kind {
	if len(meta.Videos) > 0 {
		return KindFolder
	}
	return KindVideo
}

func folderVideoKey(videoID string) string { return "foldervideo|" + videoID }

// folderVideo reports whether r is a video of a folder, whose description
// holds it.
func (r record) folderVideo() bool { return r.Kind == KindVideo && r.SeriesID != "" && r.Video != nil }

// folderItemID is the item of the folder of a folder's video.
func (r record) folderItemID() accounts.ID { return itemID(titleKey(KindVideo, r.SeriesID)) }

// videoName names a folder's video without a title by its place.
func (w words) videoName(number int) string {
	return w.video + " " + strconv.Itoa(number)
}

// folderContents counts a folder's videos, those released by item: what
// apps show of how many the user watched.
func folderContents(meta stremio.Meta, now time.Time) *Contents {
	c := &Contents{}
	seen := make(map[string]bool, len(meta.Videos))
	for _, video := range meta.Videos {
		if video.ID == "" || seen[video.ID] {
			continue
		}
		seen[video.ID] = true
		c.Children++
		if !released(video, now) {
			continue
		}
		c.Released++
		c.Runtime += parseRuntime(string(video.Runtime))
		if date := parseDate(video.Released); date != nil && (c.LastReleased == nil || date.After(*c.LastReleased)) {
			c.LastReleased = date
		}
		c.Videos = append(c.Videos, itemID(folderVideoKey(video.ID)))
	}
	return c
}

// folderVideos lists a folder's videos in the order of its description,
// each once.
func folderVideos(folder Item, meta stremio.Meta, w words, now time.Time) []Item {
	var result []Item
	seen := make(map[string]bool, len(meta.Videos))
	for _, video := range meta.Videos {
		if video.ID == "" || seen[video.ID] {
			continue
		}
		seen[video.ID] = true
		item := Item{
			ID:           itemID(folderVideoKey(video.ID)),
			Kind:         KindVideo,
			Name:         video.DisplayName(),
			ParentID:     folder.ID,
			FolderID:     folder.ID,
			Overview:     video.Overview,
			PremiereDate: parseDate(video.Released),
			Runtime:      parseRuntime(string(video.Runtime)),
			Images:       Images{Primary: video.Thumbnail, Backdrop: folder.Images.Backdrop},
			Genres:       slices.Clip(video.Genres),
			Available:    released(video, now),
			StremioType:  meta.Type,
			StremioID:    video.ID,
		}
		if item.Name == "" {
			item.Name = w.videoName(len(result) + 1)
		}
		if item.PremiereDate != nil {
			item.ProductionYear = item.PremiereDate.Year()
		}
		result = append(result, item)
	}
	return result
}

// shapeWait bounds how long a listing waits for the descriptions of its
// titles of the other and channel types, shapeLookups of them at a time.
// The lookups go on for the next listings; the titles still unknown are
// listed as folders meanwhile, which list themselves as their one video
// when they have none (see folderChildren).
const (
	shapeWait    = 5 * time.Second
	shapeLookups = 8
)

// shapeOthers gives the titles of the other and channel types among a
// listing's items, and their records at the same places, the kind their
// descriptions give (see otherKind and shownKind), and a folder or a
// series what it holds. The descriptions kept answer; the others are
// asked for, waiting at most shapeWait, unless the listing asks no addon.
// It returns the places of the titles whose descriptions are still
// unknown, whose kind is a guess.
func (s *Service) shapeOthers(ctx context.Context, v view, items []Item, records []record) []int {
	var unknown []int
	for i := range records {
		if !records[i].shapedTitle() {
			continue
		}
		if meta, _, ok := s.describedMeta(ctx, v, records[i], false, inMemory); ok {
			shape(&items[i], &records[i], meta, s.now())
			continue
		}
		unknown = append(unknown, i)
	}
	if len(unknown) == 0 {
		return nil
	}
	// A read that asks no addon (see withKeptOnly) finds those kept in
	// the database.
	if keptOnly(ctx) {
		return slices.DeleteFunc(unknown, func(i int) bool {
			meta, _, ok := s.describedMeta(ctx, v, records[i], false, inDatabase)
			if ok {
				shape(&items[i], &records[i], meta, s.now())
			}
			return ok
		})
	}
	ctx, cancel := context.WithTimeout(ctx, shapeWait)
	defer cancel()
	found := make([]*stremio.Meta, len(unknown))
	var g errgroup.Group
	g.SetLimit(shapeLookups)
	for j, i := range unknown {
		g.Go(func() error {
			if meta, ok := s.titleMeta(ctx, v, records[i]); ok {
				found[j] = &meta
			}
			return nil
		})
	}
	_ = g.Wait()
	var left []int
	for j, i := range unknown {
		if found[j] != nil {
			shape(&items[i], &records[i], *found[j], s.now())
		} else {
			left = append(left, i)
		}
	}
	return left
}

// shape gives an item of the other or channel type, and its record, the
// kind its description gives.
func shape(item *Item, r *record, meta stremio.Meta, now time.Time) {
	if r.shownTitle() {
		item.Kind, r.Kind = shownKind(meta), shownKind(meta)
		item.Contents = nil
		if item.Kind == KindSeries {
			item.Contents = contents(meta, nil, now)
		}
		return
	}
	item.Kind, r.Kind = otherKind(meta), otherKind(meta)
	item.Contents = nil
	if item.Kind == KindFolder {
		item.Contents = folderContents(meta, now)
	}
}

// folder describes a title of the other or channel type as the user of v
// sees it, a folder or a video, with its description: ErrNotFound when no
// addon describes it, the user's parental control hides it, or its library
// shows it as movies and shows (see misplaced).
func (s *Service) folder(ctx context.Context, v view, id accounts.ID) (Item, stremio.Meta, error) {
	r, err := s.load(ctx, id)
	if err != nil || !r.otherTitle() || v.misplaced(r) {
		return Item{}, stremio.Meta{}, ErrNotFound
	}
	meta, ok := s.titleMeta(ctx, v, r)
	if !ok || !v.allows(KindFolder, s.overriddenTraits(id, describedTraits(meta))) {
		return Item{}, stremio.Meta{}, ErrNotFound
	}
	item := Item{ID: id, Kind: otherKind(meta), ParentID: deref(r.Parent), Available: true}
	fromMeta(&item, s.shown(ctx, v, r.Meta.Type, r.Meta.ID, meta))
	item.OfficialRating = certification(meta)
	if item.Kind == KindFolder {
		item.Contents = folderContents(meta, s.now())
	}
	return item, meta, nil
}

// folderChildren lists a folder's videos, recording them. A title whose
// description has no videos, listed as a folder before its description
// was known, lists itself, the video it is. A listing narrowed by type
// (see WithKinds) keeps only the items of the kinds it asks for, its
// total counting them alone, as Jellyfin counts a folder's children.
func (s *Service) folderChildren(ctx context.Context, v view, id accounts.ID, start, count int) (Page, error) {
	folder, meta, err := s.folder(ctx, v, id)
	if err != nil {
		return Page{}, err
	}
	leftOut := func(item Item) bool { return !keepsKind(ctx, item.Kind) }
	if folder.Kind != KindFolder {
		return slicePage(slices.DeleteFunc(s.overridden([]Item{folder}), leftOut), start, count), nil
	}
	videos := folderVideos(folder, meta, v.words, s.now())
	listed := slices.DeleteFunc(slices.Clone(videos), leftOut)
	return slicePage(s.overridden(listed), start, count), s.save(ctx, folderRecords(folder.ID, meta, videos))
}

// folderRecords are the records of a folder's videos, listed from its
// description.
func folderRecords(folder accounts.ID, meta stremio.Meta, videos []Item) []record {
	byID := make(map[string]stremio.Video, len(meta.Videos))
	for _, video := range meta.Videos {
		byID[video.ID] = video
	}
	records := make([]record, 0, len(videos))
	for _, item := range videos {
		video := byID[item.StremioID]
		records = append(records, record{ID: item.ID, Key: folderVideoKey(video.ID), Kind: KindVideo, Parent: &folder, SeriesID: meta.ID,
			Video: &video})
	}
	return records
}

// folderVideoItem describes a folder's video from its folder's
// description.
func (s *Service) folderVideoItem(ctx context.Context, v view, r record) (Item, error) {
	folder, meta, err := s.folder(ctx, v, r.folderItemID())
	if err != nil || folder.Kind != KindFolder {
		return Item{}, ErrNotFound
	}
	for _, video := range folderVideos(folder, meta, v.words, s.now()) {
		if video.ID == r.ID {
			return video, nil
		}
	}
	return Item{}, ErrNotFound
}

// FolderVideos lists the videos of a folder (see folderChildren), as
// marking it played marks them.
func (s *Service) FolderVideos(ctx context.Context, user accounts.User, id accounts.ID) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	page, err := s.folderChildren(ctx, v, id, 0, math.MaxInt)
	return page.Items, err
}

// videoStreams are the streams a description embeds for one of its
// videos.
func videoStreams(meta stremio.Meta, videoID string) []stremio.Stream {
	for _, video := range meta.Videos {
		if video.ID == videoID {
			return video.Streams
		}
	}
	return nil
}

// embed sets the streams the description of holder, an episode's series or
// a folder's video's folder, embeds for t's video, from the addon that
// describes it: those of a description kept, or, when ask is set, asked
// for. A video's own streams are its first versions, as Stremio apps play
// them without asking the stream resource, which may only answer notices.
func (s *Service) embed(ctx context.Context, v view, t *target, ask bool) {
	if t.holder == nil || t.embeddedBy != nil {
		return
	}
	meta, by, ok := s.describedMeta(ctx, v, *t.holder, false, inDatabase)
	if !ok && ask {
		meta, by, ok = s.describeBy(ctx, v, *t.holder, false)
	}
	if !ok {
		return
	}
	t.embedded, t.embeddedBy, t.embeddedIn = videoStreams(meta, t.id), &by, t.holder.Meta.ID
}

// embeddedVersion describes a stream t's holder embeds for its video.
func embeddedVersion(t target, stream stremio.Stream) Version {
	version := newVersion(t, streamSource{*t.embeddedBy, t.metaType, t.id}, stream)
	version.Origin.Title = t.embeddedIn
	return version
}

// renewEmbedded asks the addon that described the title holding a
// version's video for its description again, and returns the same file
// with the link it gives now (see Renew); with kept set, the description
// kept is read instead while it is younger than a version list's life.
func (s *Service) renewEmbedded(ctx context.Context, entry installed, old Version, kept bool) (Version, error) {
	key := metaKey{entry.addon.ID, old.Origin.Type, old.Origin.Title}
	description, ok := s.metas.Get(key)
	if !kept || !ok || s.now().Sub(description.at) > s.listLife() {
		var err error
		if description, err = s.fetchKeptMeta(ctx, entry, key); err != nil {
			return Version{}, err
		}
	}
	t := target{item: old.Item, metaType: old.Origin.Type, id: old.Origin.ID, runtime: old.Runtime, embeddedBy: &entry,
		embeddedIn: old.Origin.Title}
	t.season, t.episode = old.torrentEpisode()
	for _, stream := range videoStreams(description.value, old.Origin.ID) {
		if !listable(stream) {
			continue
		}
		if version := embeddedVersion(t, stream); version.ID == old.ID {
			version.Labels = old.Labels
			s.versions.Put(version.ID, version)
			return version, nil
		}
	}
	return Version{}, ErrNotFound
}

// otherLibraryVideos lists the videos of a library of the other or channel
// type through its folders, as Jellyfin lists a homevideos library's
// videos when asked recursively for them (jellyfin-web's Videos view):
// its titles that are videos, and the videos of those that are folders,
// in the catalog's order, up to WholeListing of its titles, or the
// library's maximum when lower. A folder whose description is not known
// in time is left out, and the listing tells that more may follow.
func (s *Service) otherLibraryVideos(ctx context.Context, v view, l library, src source, start, count int) (Page, error) {
	most := WholeListing
	if l.maxItems > 0 {
		most = min(most, l.maxItems)
	}
	metas, _, err := s.window(ctx, v, src, 0, most)
	if err != nil {
		return Page{}, err
	}
	titles := make([]Item, 0, len(metas))
	records := make([]record, 0, len(metas))
	for _, meta := range metas {
		if item, r, err := titleItem(l.addon, l.catalog, meta, l.item.ID); err == nil {
			titles, records = append(titles, item), append(records, r)
		}
	}
	s.shapeOthers(ctx, v, titles, records)
	// The catalog's titles are those the user may see (see window).
	var videos []Item
	var unknown bool
	for i, title := range titles {
		if !records[i].otherTitle() || title.Kind == KindVideo {
			if keepsKind(ctx, title.Kind) {
				videos = append(videos, title)
			}
			continue
		}
		if !keepsKind(ctx, KindVideo) {
			continue
		}
		meta, _, ok := s.describedMeta(ctx, v, records[i], false, inMemory)
		if !ok {
			unknown = true
			continue
		}
		listed := folderVideos(title, meta, v.words, s.now())
		records = append(records, folderRecords(title.ID, meta, listed)...)
		videos = append(videos, listed...)
	}
	result := slicePage(s.overridden(videos), start, count)
	if unknown {
		result.More, result.Total = true, max(result.Total, start+len(result.Items)+1)
	}
	return result, s.save(ctx, records)
}
