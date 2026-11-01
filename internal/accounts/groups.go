package accounts

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrInvalidGroup reports a group to join that does not exist.
	ErrInvalidGroup = errors.New("invalid group")
	// ErrAdministratorGroup refuses an administrator in a group: their
	// permissions differ from those a group holds.
	ErrAdministratorGroup = errors.New("an administrator cannot join a group")
	// ErrFollowsGroup refuses a change to a setting the user's group holds
	// while they follow it (see UserChanges.DetachFromGroup).
	ErrFollowsGroup = errors.New("the user follows their group")
	// ErrInvalidGroupName reports a group name that is empty, longer than
	// maxNameLength or unprintable; ErrGroupNameTaken, one another group
	// has, compared without regard to case.
	ErrInvalidGroupName = errors.New("invalid group name")
	ErrGroupNameTaken   = errors.New("group name already taken")
	// ErrInvalidGroupCopy reports a user to copy a new group from that does
	// not exist.
	ErrInvalidGroupCopy = errors.New("invalid user to copy the group from")
)

// groupColumns are the settings a group holds, columns of user_groups and
// users alike: those of a user's page but the name, the password,
// administrator status, the sign-in screen, the disabled switch and the end
// date. Limits holds them in this order.
const groupColumns = "max_parental_rating, max_parental_sub_rating, block_unrated_items, " +
	"video_transcoding, audio_transcoding, content_downloading, personal_addons, " +
	"max_playbacks, max_bitrate, live_tv, sync_play, remote_control, " +
	"hidden_libraries, blocked_genres, access_schedules, collection_management, " +
	"subtitle_management, live_tv_management, quality_group, can_share, torrent_streams, remote_access, live_tv_channels, local_first, " +
	"boost_volume, convert_sound"

// Limits are the settings a group holds; see User for each.
type Limits struct {
	Parental             ParentalControl
	VideoTranscoding     bool
	AudioTranscoding     bool
	ContentDownloading   bool
	PersonalAddons       bool
	MaxPlaybacks         int
	MaxBitrate           int
	LiveTv               bool
	SyncPlay             SyncPlayAccess
	RemoteControl        bool
	HiddenLibraries      []ID
	BlockedGenres        []string
	AccessSchedules      []AccessSchedule
	CollectionManagement bool
	SubtitleManagement   bool
	LiveTvManagement     bool
	QualityGroup         int
	CanShare             bool
	TorrentStreams       bool
	RemoteAccess         bool
	LiveTvChannels       ChannelRule
	LocalFirst           bool
	BoostVolume          bool
	ConvertSound         bool
}

// fields lists where the groupColumns of a row go.
func (l *Limits) fields() []any {
	return []any{&l.Parental.MaxRating, &l.Parental.MaxSubRating, &l.Parental.BlockUnrated,
		&l.VideoTranscoding, &l.AudioTranscoding, &l.ContentDownloading, &l.PersonalAddons,
		&l.MaxPlaybacks, &l.MaxBitrate, &l.LiveTv, &l.SyncPlay, &l.RemoteControl,
		&l.HiddenLibraries, &l.BlockedGenres, &l.AccessSchedules, &l.CollectionManagement,
		&l.SubtitleManagement, &l.LiveTvManagement, &l.QualityGroup, &l.CanShare, &l.TorrentStreams, &l.RemoteAccess, &l.LiveTvChannels, &l.LocalFirst,
		&l.BoostVolume, &l.ConvertSound}
}

// Limits returns the settings of u a group holds.
func (u User) Limits() Limits {
	return Limits{Parental: u.Parental, VideoTranscoding: u.VideoTranscoding, AudioTranscoding: u.AudioTranscoding,
		ContentDownloading: u.ContentDownloading, PersonalAddons: u.PersonalAddons, MaxPlaybacks: u.MaxPlaybacks,
		MaxBitrate: u.MaxBitrate, LiveTv: u.LiveTv, SyncPlay: u.SyncPlay, RemoteControl: u.RemoteControl,
		HiddenLibraries: u.HiddenLibraries, BlockedGenres: u.BlockedGenres, AccessSchedules: u.AccessSchedules,
		CollectionManagement: u.CollectionManagement, SubtitleManagement: u.SubtitleManagement,
		LiveTvManagement: u.LiveTvManagement, QualityGroup: u.QualityGroup, CanShare: u.CanShare, TorrentStreams: u.TorrentStreams,
		RemoteAccess: u.RemoteAccess, LiveTvChannels: u.LiveTvChannels, LocalFirst: u.LocalFirst,
		BoostVolume: u.BoostVolume, ConvertSound: u.ConvertSound}
}

// with returns l with the settings changes, prepared as u, sets.
func (l Limits) with(changes UserChanges, u userUpdate) Limits {
	if changes.Parental != nil {
		l.Parental = u.parental
	}
	set(&l.VideoTranscoding, changes.VideoTranscoding)
	set(&l.AudioTranscoding, changes.AudioTranscoding)
	set(&l.ContentDownloading, changes.ContentDownloading)
	set(&l.PersonalAddons, changes.PersonalAddons)
	set(&l.MaxPlaybacks, changes.MaxPlaybacks)
	set(&l.MaxBitrate, changes.MaxBitrate)
	set(&l.LiveTv, changes.LiveTv)
	set(&l.SyncPlay, changes.SyncPlay)
	set(&l.RemoteControl, changes.RemoteControl)
	set(&l.HiddenLibraries, u.hidden)
	set(&l.BlockedGenres, u.genres)
	set(&l.AccessSchedules, u.schedules)
	set(&l.CollectionManagement, changes.CollectionManagement)
	set(&l.SubtitleManagement, changes.SubtitleManagement)
	set(&l.LiveTvManagement, changes.LiveTvManagement)
	set(&l.QualityGroup, changes.QualityGroup)
	set(&l.CanShare, changes.CanShare)
	set(&l.TorrentStreams, changes.TorrentStreams)
	set(&l.RemoteAccess, changes.RemoteAccess)
	set(&l.LiveTvChannels, u.channels)
	set(&l.LocalFirst, changes.LocalFirst)
	set(&l.BoostVolume, changes.BoostVolume)
	set(&l.ConvertSound, changes.ConvertSound)
	return l
}

func set[T any](field, value *T) {
	if value != nil {
		*field = *value
	}
}

// equal reports whether l and o allow the same: the hidden libraries are
// compared as a set, the rest as they are kept.
func (l Limits) equal(o Limits) bool {
	return equalPointers(l.Parental.MaxRating, o.Parental.MaxRating) &&
		equalPointers(l.Parental.MaxSubRating, o.Parental.MaxSubRating) &&
		slices.Equal(l.Parental.BlockUnrated, o.Parental.BlockUnrated) &&
		l.VideoTranscoding == o.VideoTranscoding && l.AudioTranscoding == o.AudioTranscoding &&
		l.ContentDownloading == o.ContentDownloading && l.PersonalAddons == o.PersonalAddons &&
		l.MaxPlaybacks == o.MaxPlaybacks && l.MaxBitrate == o.MaxBitrate && l.LiveTv == o.LiveTv &&
		l.SyncPlay == o.SyncPlay && l.RemoteControl == o.RemoteControl &&
		SameLibraries(l.HiddenLibraries, o.HiddenLibraries) && slices.Equal(l.BlockedGenres, o.BlockedGenres) &&
		slices.Equal(l.AccessSchedules, o.AccessSchedules) && l.CollectionManagement == o.CollectionManagement &&
		l.SubtitleManagement == o.SubtitleManagement && l.LiveTvManagement == o.LiveTvManagement &&
		l.QualityGroup == o.QualityGroup && l.CanShare == o.CanShare && l.TorrentStreams == o.TorrentStreams &&
		l.RemoteAccess == o.RemoteAccess && l.LiveTvChannels.equal(o.LiveTvChannels) &&
		l.LocalFirst == o.LocalFirst && l.BoostVolume == o.BoostVolume && l.ConvertSound == o.ConvertSound
}

func equalPointers(a, b *int) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// SameLibraries reports whether a and b name the same libraries, in any
// order.
func SameLibraries(a, b []ID) bool {
	return !slices.ContainsFunc(a, func(id ID) bool { return !slices.Contains(b, id) }) &&
		!slices.ContainsFunc(b, func(id ID) bool { return !slices.Contains(a, id) })
}

// Group holds the limits of several users at once, its members: those who
// follow it have its values, at once after each change.
type Group struct {
	ID        ID
	Name      string
	CreatedAt time.Time
	// Members counts the users in the group, following it or not.
	Members int
	Limits
}

const groupSelect = "SELECT id, name, created_at, (SELECT count(*) FROM users WHERE group_id = g.id), " +
	groupColumns + " FROM user_groups g"

func scanGroup(row pgx.Row) (Group, error) {
	var group Group
	err := row.Scan(append([]any{&group.ID, &group.Name, &group.CreatedAt, &group.Members}, group.fields()...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return group, ErrNotFound
	}
	return group, err
}

// GroupChanges lists what to change in a group; nil fields are kept, and
// only the settings a group holds are read from Limits.
type GroupChanges struct {
	Name   *string
	Limits UserChanges
}

// The sections of a user's page a group holds, as GroupChanges.Sections
// names them.
const (
	SectionName      = "name"
	SectionAccess    = "access"
	SectionPlayback  = "playback"
	SectionParental  = "parental"
	SectionLibraries = "libraries"
	SectionGenres    = "genres"
	SectionHours     = "hours"
	SectionChannels  = "channels"
)

// Sections names what changes sets, in the order of a group's page.
func (changes GroupChanges) Sections() []string {
	l := changes.Limits
	var sections []string
	for _, section := range []struct {
		name string
		set  bool
	}{
		{SectionName, changes.Name != nil},
		{SectionAccess, l.VideoTranscoding != nil || l.AudioTranscoding != nil || l.ContentDownloading != nil ||
			l.PersonalAddons != nil || l.CollectionManagement != nil || l.SubtitleManagement != nil || l.CanShare != nil ||
			l.TorrentStreams != nil || l.RemoteAccess != nil},
		{SectionPlayback, l.MaxPlaybacks != nil || l.MaxBitrate != nil || l.QualityGroup != nil || l.SyncPlay != nil ||
			l.LiveTv != nil || l.RemoteControl != nil || l.LiveTvManagement != nil || l.LocalFirst != nil ||
			l.BoostVolume != nil || l.ConvertSound != nil},
		{SectionParental, l.Parental != nil},
		{SectionLibraries, l.HiddenLibraries != nil},
		{SectionGenres, l.BlockedGenres != nil},
		{SectionHours, l.AccessSchedules != nil},
		{SectionChannels, l.LiveTvChannels != nil},
	} {
		if section.set {
			sections = append(sections, section.name)
		}
	}
	return sections
}

// normalizeGroupName trims a group name and checks it.
func normalizeGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength ||
		strings.ContainsFunc(name, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return "", ErrInvalidGroupName
	}
	return name, nil
}

// Groups lists every group, sorted by name.
func (s *Store) Groups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.Query(ctx, groupSelect+" ORDER BY lower(name), id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Group, error) { return scanGroup(row) })
}

// Group returns one group.
func (s *Store) Group(ctx context.Context, id ID) (Group, error) {
	return scanGroup(s.db.QueryRow(ctx, groupSelect+" WHERE id = $1", id))
}

// CreateGroup adds a group named name, with the settings of copyFrom, a
// user, or else those of a user CreateUser makes.
func (s *Store) CreateGroup(ctx context.Context, name string, copyFrom *ID) (Group, error) {
	name, err := normalizeGroupName(name)
	if err != nil {
		return Group{}, err
	}
	var created Group
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var id ID
		var err error
		if copyFrom == nil {
			err = tx.QueryRow(ctx, "INSERT INTO user_groups (name) VALUES ($1) RETURNING id", name).Scan(&id)
		} else {
			err = tx.QueryRow(ctx, "INSERT INTO user_groups (name, "+groupColumns+") SELECT $1, "+groupColumns+
				" FROM users WHERE id = $2 RETURNING id", name, *copyFrom).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidGroupCopy
			}
		}
		if uniqueViolation(err) {
			return ErrGroupNameTaken
		}
		if err != nil {
			return err
		}
		created, err = scanGroup(tx.QueryRow(ctx, groupSelect+" WHERE id = $1", id))
		return err
	})
	return created, err
}

// UpdateGroup applies changes to a group, and to the members who follow
// it, as changing each of them would: they have its values from their next
// request, and lose their share links when it may no longer share. It
// returns the group as it is now.
func (s *Store) UpdateGroup(ctx context.Context, id ID, changes GroupChanges) (Group, error) {
	var name *string
	if changes.Name != nil {
		normalized, err := normalizeGroupName(*changes.Name)
		if err != nil {
			return Group{}, err
		}
		name = &normalized
	}
	l := changes.Limits
	u, err := l.prepare()
	if err != nil {
		return Group{}, err
	}
	var updated Group
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE user_groups SET
				name = coalesce($2, name),
				max_parental_rating = CASE WHEN $3 THEN $4 ELSE max_parental_rating END,
				max_parental_sub_rating = CASE WHEN $3 THEN $5 ELSE max_parental_sub_rating END,
				block_unrated_items = CASE WHEN $3 THEN $6 ELSE block_unrated_items END,
				video_transcoding = coalesce($7, video_transcoding),
				audio_transcoding = coalesce($8, audio_transcoding),
				content_downloading = coalesce($9, content_downloading),
				personal_addons = coalesce($10, personal_addons),
				max_playbacks = coalesce($11, max_playbacks),
				max_bitrate = coalesce($12, max_bitrate),
				live_tv = coalesce($13, live_tv),
				sync_play = coalesce($14, sync_play),
				remote_control = coalesce($15, remote_control),
				hidden_libraries = coalesce($16, hidden_libraries),
				blocked_genres = coalesce($17, blocked_genres),
				access_schedules = coalesce($18::jsonb, access_schedules),
				collection_management = coalesce($19, collection_management),
				subtitle_management = coalesce($20, subtitle_management),
				live_tv_management = coalesce($21, live_tv_management),
				quality_group = coalesce($22, quality_group),
				can_share = coalesce($23, can_share),
				torrent_streams = coalesce($24, torrent_streams),
				remote_access = coalesce($25, remote_access),
				live_tv_channels = coalesce($26::jsonb, live_tv_channels),
				local_first = coalesce($27, local_first),
				boost_volume = coalesce($28, boost_volume),
				convert_sound = coalesce($29, convert_sound)
			WHERE id = $1`,
			id, name, l.Parental != nil, u.parental.MaxRating, u.parental.MaxSubRating, u.parental.BlockUnrated,
			l.VideoTranscoding, l.AudioTranscoding, l.ContentDownloading, l.PersonalAddons,
			l.MaxPlaybacks, l.MaxBitrate, l.LiveTv, l.SyncPlay, l.RemoteControl,
			u.hidden, u.genres, u.schedules, l.CollectionManagement,
			l.SubtitleManagement, l.LiveTvManagement, l.QualityGroup, l.CanShare, l.TorrentStreams, l.RemoteAccess, u.channels, l.LocalFirst,
			l.BoostVolume, l.ConvertSound)
		if uniqueViolation(err) {
			return ErrGroupNameTaken
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if updated, err = scanGroup(tx.QueryRow(ctx, groupSelect+" WHERE id = $1", id)); err != nil {
			return err
		}
		return spreadGroup(ctx, tx, updated)
	})
	s.forgetSignIns()
	return updated, err
}

// spreadGroup gives group's values to the members who follow it, in the
// order of their identifiers, and takes the share links of those who may
// no longer share.
func spreadGroup(ctx context.Context, tx pgx.Tx, group Group) error {
	rows, err := tx.Query(ctx, "SELECT id FROM users WHERE group_id = $1 AND follow_group ORDER BY id FOR UPDATE", group.ID)
	if err != nil {
		return err
	}
	followers, err := pgx.CollectRows(rows, pgx.RowTo[ID])
	if err != nil || len(followers) == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET ("+groupColumns+") = (SELECT "+groupColumns+
		" FROM user_groups WHERE id = $1) WHERE id = ANY($2)", group.ID, followers); err != nil {
		return err
	}
	if !group.CanShare {
		return revokeShareLinks(ctx, tx, followers...)
	}
	return nil
}

// DeleteGroup deletes a group, and returns it as it was. Its members stay
// as they are, with its values as their own; the invites that named it
// create accounts in no group.
func (s *Store) DeleteGroup(ctx context.Context, id ID) (Group, error) {
	var deleted Group
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		var err error
		if deleted, err = scanGroup(tx.QueryRow(ctx, groupSelect+" WHERE id = $1 FOR UPDATE", id)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET group_id = NULL, follow_group = true WHERE id IN (
			SELECT id FROM users WHERE group_id = $1 ORDER BY id FOR UPDATE)`, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM user_groups WHERE id = $1", id)
		return err
	})
	s.forgetSignIns()
	return deleted, err
}

// GroupMembers lists the users in a group, sorted by name.
func (s *Store) GroupMembers(ctx context.Context, id ID) ([]User, error) {
	if _, err := s.Group(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, "SELECT "+userColumns+" FROM users WHERE group_id = $1 ORDER BY lower(name), id", id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) })
}

// AddGroupMembers has users join a group and follow it, leaving the group
// they were in, all of them or none: ErrNotFound reports a user or a group
// that does not exist, ErrAdministratorGroup an administrator among them.
// The users already in the group stay as they are. It returns those who
// joined, as they are now.
func (s *Store) AddGroupMembers(ctx context.Context, id ID, users []ID) ([]User, error) {
	users = slices.Clone(users)
	slices.SortFunc(users, func(a, b ID) int { return strings.Compare(string(a[:]), string(b[:])) })
	users = slices.Compact(users)
	var joined []User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		if _, err := scanGroup(tx.QueryRow(ctx, groupSelect+" WHERE id = $1", id)); err != nil {
			return err
		}
		for _, user := range users {
			current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", user))
			if err != nil {
				return err
			}
			if current.GroupID != nil && *current.GroupID == id {
				continue
			}
			updated, _, err := applyUserChanges(ctx, tx, user, UserChanges{Group: &id}, userUpdate{}, signIn{})
			if err != nil {
				return err
			}
			joined = append(joined, updated)
		}
		return nil
	})
	s.forgetSignIns()
	if err != nil {
		return nil, err
	}
	return joined, nil
}

// membership is the group a change leaves a user in, nil for none; follow
// tells whether they follow it, and takeValues whether the group's values
// are to be written to them.
type membership struct {
	group      *ID
	follow     bool
	takeValues bool
}

// groupMembership decides the group of current after changes, prepared as
// u, administrator telling whether they are one after them.
func groupMembership(ctx context.Context, tx pgx.Tx, current User, administrator bool, changes UserChanges, u userUpdate) (membership, error) {
	m := membership{group: current.GroupID, follow: current.FollowGroup}
	joined := false
	if g := changes.Group; g != nil {
		if *g == (ID{}) {
			m.group = nil
		} else {
			if administrator {
				return m, ErrAdministratorGroup
			}
			joined = current.GroupID == nil || *current.GroupID != *g
			m.group = g
		}
	}
	// Making a member an administrator takes them out of their group, with
	// its values kept as their own.
	if administrator || m.group == nil {
		return membership{follow: true}, nil
	}
	if joined {
		m.follow = true
	}
	if f := changes.FollowGroup; f != nil {
		m.follow = *f
	}
	if !joined && !m.follow {
		return m, nil
	}
	var limits Limits
	err := tx.QueryRow(ctx, "SELECT "+groupColumns+" FROM user_groups WHERE id = $1", *m.group).Scan(limits.fields()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrInvalidGroup
	}
	if err != nil || !m.follow {
		return m, err
	}
	m.takeValues = true
	// A change that keeps the group's values leaves the user following it.
	if !joined && current.FollowGroup && !limits.with(changes, u).equal(limits) {
		if !changes.DetachFromGroup {
			return m, ErrFollowsGroup
		}
		m.follow, m.takeValues = false, false
	}
	return m, nil
}
