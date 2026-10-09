package library

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Replay is a view of Polyfin's own, beside the libraries: a folder per
// channel of the user's whose provider keeps past programmes (an IPTV
// source's channel with an archive, see iptv.Service.Archives), holding
// the programmes of its guide that ended within the days the archive
// reaches back, the latest first. Each plays from the provider's archive
// as a title's version plays (see iptv.ReplayID). A user reaches the
// channels of Replay as they reach them in Live TV.

// ReplayViewID identifies the Replay view, the same on every server.
var ReplayViewID = itemID("view|replay")

// maxReplayDays bounds how many days back Replay lists programmes, and
// how long the guides keep past programmes for it (see guideArchives).
const maxReplayDays = 30

// replayChannel is a channel of the user's that has an archive, with the
// days it reaches back and the record that finds the channel again.
type replayChannel struct {
	channel Item
	record  record
	days    int
}

// guideArchives maps the channels of a guide that channels with an
// archive are mapped to, by their guide identifier, to how far back the
// guide keeps their past programmes: as far as the archives reach, at most
// maxReplayDays.
func (s *Service) guideArchives(ctx context.Context, guide accounts.ID) (map[string]time.Duration, error) {
	if s.iptv == nil {
		return nil, nil
	}
	archives, err := s.iptv.GuideArchives(ctx, guide)
	if err != nil {
		return nil, err
	}
	reach := make(map[string]time.Duration, len(archives))
	for id, days := range archives {
		reach[id] = time.Duration(min(days, maxReplayDays)) * 24 * time.Hour
	}
	return reach, nil
}

// keepArchived copies to a guide's new generation the programmes of its
// last download that ended by at within the reach of their channel's
// archive and that the new download no longer gives: guides seldom list
// past days, and a programme stays in an archive. It reports how many it
// copied.
func keepArchived(ctx context.Context, tx pgx.Tx, g addons.Guide, generation int, at time.Time, archives map[string]time.Duration) (int, error) {
	if len(archives) == 0 || g.Generation == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(archives))
	since := make([]time.Time, 0, len(archives))
	for id, reach := range archives {
		ids, since = append(ids, id), append(since, at.Add(-reach))
	}
	tag, err := tx.Exec(ctx, `INSERT INTO live_guide_programmes (guide_id, generation, xmltv_id, starts_at, ends_at, title, subtitle,
			description, categories, season, episode, icon)
		SELECT p.guide_id, $2, p.xmltv_id, p.starts_at, p.ends_at, p.title, p.subtitle, p.description, p.categories, p.season, p.episode, p.icon
		FROM unnest($4::text[], $5::timestamptz[]) AS a (xmltv_id, since)
		JOIN live_guide_programmes p ON p.guide_id = $1 AND p.generation = $3 AND p.xmltv_id = a.xmltv_id
		WHERE p.ends_at <= $6 AND p.ends_at > a.since AND NOT EXISTS (SELECT 1 FROM live_guide_programmes n
			WHERE n.guide_id = $1 AND n.generation = $2 AND n.xmltv_id = p.xmltv_id AND n.starts_at < p.ends_at AND n.ends_at > p.starts_at)`,
		g.ID, generation, g.Generation, ids, since, at)
	return int(tag.RowsAffected()), err
}

// archives maps the Stremio identifiers of the user's channels that have
// an archive to the days it reaches back, at most maxReplayDays.
func (s *Service) archives(ctx context.Context, v view) (map[string]int, error) {
	if s.iptv == nil {
		return nil, nil
	}
	result := map[string]int{}
	for _, src := range v.channels {
		if !src.addon.addon.IPTV() {
			continue
		}
		archives, err := s.iptv.Archives(ctx, src.addon.addon.ID)
		if err != nil {
			return nil, err
		}
		for id, days := range archives {
			result[id] = min(days, maxReplayDays)
		}
	}
	return result, nil
}

// HasReplay reports whether the user has the Replay view: channels that
// have an archive.
func (s *Service) HasReplay(ctx context.Context, user accounts.User) (bool, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return false, err
	}
	archives, err := s.archives(ctx, v)
	return len(archives) > 0, err
}

// replayView describes the Replay view, ErrNotFound for a user without it.
func (s *Service) replayView(ctx context.Context, v view) (Item, error) {
	archives, err := s.archives(ctx, v)
	if err != nil {
		return Item{}, err
	}
	if len(archives) == 0 {
		return Item{}, ErrNotFound
	}
	return Item{ID: ReplayViewID, Kind: KindLibrary, Name: s.words().replay, CollectionType: "folders"}, nil
}

// replayChannels lists the user's channels that have an archive, in
// channel order.
func (s *Service) replayChannels(ctx context.Context, v view) ([]replayChannel, error) {
	archives, err := s.archives(ctx, v)
	if err != nil || len(archives) == 0 {
		return nil, err
	}
	items, records := s.channels(ctx, v)
	var result []replayChannel
	var kept []record
	for i, item := range items {
		if days, ok := archives[item.StremioID]; ok {
			result = append(result, replayChannel{channel: item, record: records[i], days: days})
			kept = append(kept, records[i])
		}
	}
	// The channels' records find them again when a folder or a programme
	// is opened.
	return result, s.save(ctx, kept)
}

// replayFolder describes the folder of a channel that has an archive.
func replayFolder(channel Item) Item {
	folder := channel
	folder.ID, folder.Kind, folder.ParentID = itemID(replayFolderKey(channel.StremioID)), KindReplayFolder, ReplayViewID
	folder.Channel = &channel
	return folder
}

// replayFolders lists the folders of the Replay view.
func (s *Service) replayFolders(ctx context.Context, v view) ([]Item, error) {
	channels, err := s.replayChannels(ctx, v)
	if err != nil {
		return nil, err
	}
	folders := make([]Item, 0, len(channels))
	records := make([]record, 0, len(channels))
	for _, c := range channels {
		folder := replayFolder(c.channel)
		folders = append(folders, folder)
		records = append(records, record{ID: folder.ID, Key: replayFolderKey(c.channel.StremioID), Kind: KindReplayFolder,
			Addon: c.record.Addon, Parent: &ReplayViewID, Channel: c.channel.StremioID, Confined: c.record.Confined})
	}
	return folders, s.save(ctx, records)
}

// replayChannel finds a channel of the user's that has an archive by its
// Stremio identifier: ErrNotFound when the user no longer reaches it, or
// it has no archive any more.
func (s *Service) replayChannel(ctx context.Context, v view, stremioID string) (replayChannel, error) {
	stored, err := s.load(ctx, itemID(channelKey(stremioID)))
	if err != nil {
		return replayChannel{}, err
	}
	channel, err := s.channel(ctx, v, stored)
	if err != nil {
		return replayChannel{}, err
	}
	archives, err := s.archives(ctx, v)
	if err != nil {
		return replayChannel{}, err
	}
	days, ok := archives[stremioID]
	if !ok {
		return replayChannel{}, ErrNotFound
	}
	return replayChannel{channel: channel, record: stored, days: days}, nil
}

// replayFolderItem describes a folder of the Replay view recorded as r.
func (s *Service) replayFolderItem(ctx context.Context, v view, r record) (Item, error) {
	c, err := s.replayChannel(ctx, v, r.Channel)
	if err != nil {
		return Item{}, err
	}
	return replayFolder(c.channel), nil
}

// replayItem describes a programme of a channel's guide as one of its
// Replay folder, with the title of its episode when the guide gives one.
func replayItem(channel Item, video stremio.Video, episodeTitle string) (Item, bool) {
	item, ok := programItem(channel, video, episodeTitle)
	if !ok {
		return Item{}, false
	}
	id, ok := iptv.ReplayID(channel.StremioID, *item.StartDate, *item.EndDate)
	if !ok {
		return Item{}, false
	}
	item.ID, item.Kind = itemID(replayKey(channel.StremioID, *item.StartDate)), KindReplay
	item.ParentID, item.StremioType, item.StremioID = itemID(replayFolderKey(channel.StremioID)), channel.StremioType, id
	// A programme whose guide gives no image shows its channel's.
	if item.Images.Primary == "" {
		item.Images.Primary = channel.Images.Primary
	}
	return item, true
}

// replayable reports whether a programme of a channel whose archive
// reaches days back plays from Replay at now: it ended, and started within
// them; and whether the user's parental control and blocked genres let
// them reach it, judged as guide programmes, which have no rating.
func (v view) replayable(p Item, days int, now time.Time) bool {
	const kind = "LiveTvProgram"
	return !p.EndDate.After(now) && !p.StartDate.Before(now.Add(-time.Duration(days)*24*time.Hour)) &&
		!v.blocked(p.Genres) && (!v.parental.Judges(kind) || v.parental.Allows(kind, ""))
}

// replayProgrammes lists a Replay folder's programmes: those of its
// channel's guide that the archive still keeps, the latest first.
func (s *Service) replayProgrammes(ctx context.Context, v view, folder record) ([]Item, error) {
	c, err := s.replayChannel(ctx, v, folder.Channel)
	if err != nil {
		return nil, err
	}
	now := s.now()
	programmes, err := s.guidePrograms(ctx, v, map[string]Item{c.channel.StremioID: c.channel}, nil,
		GuideQuery{From: now.Add(-time.Duration(c.days) * 24 * time.Hour), To: now}, func(Item) bool { return true })
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(programmes))
	records := make([]record, 0, len(programmes))
	for _, p := range programmes {
		video := stremio.Video{Title: p.Name, Overview: p.Overview, Thumbnail: p.Images.Primary, Season: stremio.Number(p.ParentIndexNumber),
			Episode: stremio.Number(p.IndexNumber), StartTime: p.StartDate.UTC().Format(time.RFC3339), EndTime: p.EndDate.UTC().Format(time.RFC3339),
			Genres: p.Genres}
		item, ok := replayItem(c.channel, video, p.EpisodeTitle)
		if !ok || !v.replayable(item, c.days, now) {
			continue
		}
		items = append(items, item)
		records = append(records, record{ID: item.ID, Key: replayKey(c.channel.StremioID, *item.StartDate), Kind: KindReplay,
			Addon: c.record.Addon, Parent: &item.ParentID, Channel: c.channel.StremioID, Video: &video, EpisodeTitle: p.EpisodeTitle,
			Confined: c.record.Confined})
	}
	slices.Reverse(items)
	return items, s.save(ctx, records)
}

// replayProgramme describes a programme of Replay recorded as r, while
// its channel's archive keeps it.
func (s *Service) replayProgramme(ctx context.Context, v view, r record) (Item, error) {
	if r.Video == nil {
		return Item{}, ErrNotFound
	}
	c, err := s.replayChannel(ctx, v, r.Channel)
	if err != nil {
		return Item{}, err
	}
	item, ok := replayItem(c.channel, *r.Video, r.EpisodeTitle)
	if !ok || item.ID != r.ID || !v.replayable(item, c.days, s.now()) {
		return Item{}, ErrNotFound
	}
	return item, nil
}
