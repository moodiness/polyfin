package playback

import (
	"context"
	"crypto/sha256"
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/episodename"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/torrent"
)

// Subtitle files inside torrents: once a torrent version's torrent tells
// its files, at the version's first play, the text subtitle files that go
// with the video it plays are listed as the version's own (see
// torrentSubtitles), and kept in the database with the version, as the
// tracks read from files are. Each file is fetched from the swarm once,
// then kept: the one in the user's preferred language as the play starts,
// the others when a user picks them (see TorrentSubtitle).

// torrentSubtitleFormats are the subtitle files listed inside torrents, by
// their extension, and their format: text alone, which apps get in the
// format they ask for.
var torrentSubtitleFormats = map[string]string{".srt": "srt", ".ass": "ass", ".ssa": "ssa", ".vtt": "vtt"}

// maxTorrentSubtitle bounds a subtitle file inside a torrent, as apps'
// subtitle downloads are bounded: a larger one is not listed.
const maxTorrentSubtitle = 8 << 20

// torrentSubtitleWait bounds the fetch of a subtitle file: the torrent's
// start, when it stopped, and the wait for its pieces.
const torrentSubtitleWait = torrentHeaders + torrent.PieceTimeout

// subtitleFolders are the names of the folders, in any letter case, that
// hold a video's subtitle files beside it.
var subtitleFolders = map[string]bool{"subs": true, "sub": true, "subtitles": true, "subtitle": true}

// numbered matches the number some releases put before the name of each
// file in a subtitles folder, as in "2_English.srt".
var numbered = regexp.MustCompile(`^\d+_`)

// torrentSubtitle is a subtitle file inside a torrent that goes with the
// video a version plays: its index among the torrent's files, its path,
// its format, and what its name tells.
type torrentSubtitle struct {
	index  int
	path   string
	format string
	SubtitleTags
}

// torrentSubtitles lists, among a torrent's files, in their order, the
// text subtitle files that go with played, the video a version plays:
//   - in its folder, those whose name begins with its own, as Jellyfin
//     reads the files beside a video and Polyfin those beside local files
//     ("Movie.fr.srt"), unless they begin with another video's longer name;
//   - in a subtitles folder beside it ("Subs", "Subtitles"), those whose
//     name begins with its own, those whose name tells its episode
//     ("Show.S01E02.English.srt"), and those whose name tells none
//     ("English.srt", "2_English.srt") when it is the only video of its
//     folder;
//   - in a folder of that subtitles folder named after the video or
//     telling its episode ("Subs/Show.S01E02.1080p/English.srt"), all.
//
// In a season's pack, a file of another episode is thus left out. What a
// name tells past the video's, or past its episode, or the whole name
// but a leading number, gives the file's language, title and flags (see
// ReadSubtitleTags). Files larger than maxTorrentSubtitle are left out.
func torrentSubtitles(played torrent.File, files []torrent.File) []torrentSubtitle {
	dir, stem := path.Dir(played.Path), fileStem(played.Path)
	episode, isEpisode := episodename.Of(played.Path)
	var longer []string
	videos := 0
	for _, f := range files {
		if f.Padding || path.Dir(f.Path) != dir || !episodename.Video(f.Path) {
			continue
		}
		videos++
		if other := fileStem(f.Path); len(other) > len(stem) {
			longer = append(longer, other)
		}
	}
	// named reads the name of a file named after the video, not after a
	// video whose longer name begins with the same.
	named := func(name string) (string, bool) {
		rest, ok := SubtitleNamed(stem, name)
		for _, other := range longer {
			if _, theirs := SubtitleNamed(other, name); theirs {
				return "", false
			}
		}
		return rest, ok
	}
	// ofEpisode reads what a name tells past the episode it tells, and
	// reports whether it tells one, and whether that one is the video's.
	ofEpisode := func(name string) (rest string, told, ours bool) {
		e, told := episodename.Of(name + ".srt")
		if !told {
			return "", false, false
		}
		if loc := episodename.Tag.FindStringIndex(name); loc != nil {
			rest = name[loc[1]:]
		} else if loc := episodename.Cross.FindStringIndex(name); loc != nil {
			rest = name[loc[1]:]
		}
		return rest, true, isEpisode && e == episode
	}
	var list []torrentSubtitle
	for i, f := range files {
		format := torrentSubtitleFormats[strings.ToLower(path.Ext(f.Path))]
		if format == "" || f.Padding || f.Length > maxTorrentSubtitle {
			continue
		}
		rel := f.Path
		if dir != "." {
			var inside bool
			if rel, inside = strings.CutPrefix(f.Path, dir+"/"); !inside {
				continue
			}
		}
		parts := strings.Split(rel, "/")
		name := fileStem(f.Path)
		var rest string
		var ok bool
		switch {
		case len(parts) == 1:
			rest, ok = named(name)
		case len(parts) == 2 && subtitleFolders[strings.ToLower(parts[0])]:
			if rest, ok = named(name); ok {
				break
			}
			var told bool
			if rest, told, ok = ofEpisode(name); !told {
				rest, ok = numbered.ReplaceAllString(name, ""), videos == 1
			}
		case len(parts) == 3 && subtitleFolders[strings.ToLower(parts[0])]:
			_, _, ours := ofEpisode(parts[1])
			rest, ok = numbered.ReplaceAllString(name, ""), ours || strings.EqualFold(parts[1], stem)
		}
		if ok {
			list = append(list, torrentSubtitle{index: i, path: f.Path, format: format, SubtitleTags: ReadSubtitleTags(rest)})
		}
	}
	return list
}

// fileStem is a file's name without its folder and extension.
func fileStem(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}

// torrentSubtitleState is what is known of the subtitle files inside the
// torrents of versions: lists holds those of every version listed, read
// from the database at the first use.
type torrentSubtitleState struct {
	mu     sync.Mutex
	loaded bool
	lists  map[accounts.ID][]torrentSubtitle
}

// torrentListed returns the subtitle files listed for a version, and
// whether they were, reading those of every version listed from the
// database at the first call.
func (s *Service) torrentListed(ctx context.Context, version accounts.ID) ([]torrentSubtitle, bool, error) {
	state := &s.torrentFiles
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.loaded {
		lists, err := s.readTorrentLists(ctx)
		if err != nil {
			return nil, false, err
		}
		state.lists, state.loaded = lists, true
	}
	list, listed := state.lists[version]
	return list, listed, nil
}

// readTorrentLists reads the subtitle files of every version listed.
func (s *Service) readTorrentLists(ctx context.Context) (map[accounts.ID][]torrentSubtitle, error) {
	lists := map[accounts.ID][]torrentSubtitle{}
	rows, err := s.db.Query(ctx, "SELECT version_id FROM torrent_subtitle_lists")
	if err != nil {
		return nil, err
	}
	var version accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&version}, func() error {
		lists[version] = nil
		return nil
	}); err != nil {
		return nil, err
	}
	rows, err = s.db.Query(ctx, `SELECT version_id, file_index, path, format, language, title, forced, hearing_impaired, is_default
		FROM torrent_subtitles ORDER BY version_id, file_index`)
	if err != nil {
		return nil, err
	}
	var f torrentSubtitle
	if _, err := pgx.ForEachRow(rows, []any{&version, &f.index, &f.path, &f.format, &f.Language, &f.Title, &f.Forced,
		&f.HearingImpaired, &f.Default}, func() error {
		lists[version] = append(lists[version], f)
		return nil
	}); err != nil {
		return nil, err
	}
	return lists, nil
}

// TorrentSubtitles returns the text subtitle files inside the torrent of
// version that go with the file it plays, its own after those listed with
// its stream: none until its torrent told its files at a play (see
// listTorrentSubtitles), then those listed then, whether the torrent runs
// or not, fetched or not. A link has none.
func (s *Service) TorrentSubtitles(ctx context.Context, version library.Version) []library.ExternalSubtitle {
	if version.Torrent == nil {
		return nil
	}
	list, _, err := s.torrentListed(ctx, version.ID)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Reading the subtitle files of torrents failed", "error", err)
		}
		return nil
	}
	files := make([]library.ExternalSubtitle, len(list))
	for i, f := range list {
		files[i] = library.ExternalSubtitle{ID: torrentSubtitleID(version.ID, f.index), Language: f.Language, Addon: version.Addon,
			Format: f.format, Title: f.Title, Forced: f.Forced, HearingImpaired: f.HearingImpaired, Default: f.Default,
			Torrent: &library.TorrentSubtitle{Version: version.ID, Torrent: *version.Torrent, Index: f.index}}
	}
	return files
}

// torrentSubtitleID names the subtitle file at index among the files of
// the torrent of a version.
func torrentSubtitleID(version accounts.ID, index int) accounts.ID {
	sum := sha256.Sum256([]byte("torrent subtitle|" + version.String() + "|" + strconv.Itoa(index)))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

// listTorrentSubtitles lists, for a play of a torrent version, the
// subtitle files inside its torrent that go with its file (see
// torrentSubtitles), unless they were listed at an earlier play: the play
// joins the swarm, as its stream does next, and waits for the torrent's
// files as long as the stream would. A version analyzed before its files
// were listed has them listed so, at the play's start, rather than as its
// stream opens: the indexes of its streams then hold for the whole play.
// It returns why the torrent's files could not be known, as a torrent
// that did not start in time; a list that could not be saved is listed at
// the next play.
func (s *Service) listTorrentSubtitles(ctx context.Context, version library.Version) error {
	if version.Torrent == nil || background(ctx) || s.torrents == nil {
		return nil
	}
	if _, listed, err := s.torrentListed(ctx, version.ID); err != nil || listed {
		return nil
	}
	spec, err := torrentSpec(version)
	if err != nil {
		return nil
	}
	_, err, _ = s.flight.Do("torrent subtitles "+version.ID.String(), func() (any, error) {
		stop := s.torrents.Play(spec)
		defer stop()
		wait, cancel := context.WithTimeout(context.WithoutCancel(ctx), torrentHeaders)
		defer cancel()
		played, files, err := s.torrents.Files(wait, spec)
		if err != nil {
			return nil, err
		}
		list := torrentSubtitles(played, files)
		if err := s.keepTorrentSubtitles(wait, version.ID, list); err != nil {
			s.logger.Warn("Saving the subtitle files of a torrent failed", "error", err)
			return nil, nil
		}
		s.torrentFiles.mu.Lock()
		s.torrentFiles.lists[version.ID] = list
		s.torrentFiles.mu.Unlock()
		s.logger.Debug("Listed the subtitle files of a torrent", "addon", version.Addon, "files", len(list))
		return nil, nil
	})
	return err
}

// keepTorrentSubtitles saves the subtitle files listed for a version, in
// the order of their keys.
func (s *Service) keepTorrentSubtitles(ctx context.Context, version accounts.ID, list []torrentSubtitle) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "INSERT INTO torrent_subtitle_lists (version_id) VALUES ($1) ON CONFLICT DO NOTHING", version)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		for _, f := range list {
			if _, err := tx.Exec(ctx, `INSERT INTO torrent_subtitles (version_id, file_index, path, format, language, title, forced,
					hearing_impaired, is_default) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				version, f.index, f.path, f.format, f.Language, f.Title, f.Forced, f.HearingImpaired, f.Default); err != nil {
				return err
			}
		}
		return nil
	})
}

// TorrentSubtitle returns the text of file, a subtitle file inside a
// version's torrent, as the torrent holds it: kept once fetched, whether
// the torrent runs or not, else fetched now from the swarm. The fetch
// joins the torrent as a playback does, through the route torrents take,
// starting it again when it stopped; it then stops as after a playback.
// It fetches the pieces that hold the file alone, and runs to its end
// whatever its caller does, for the next to ask.
func (s *Service) TorrentSubtitle(ctx context.Context, file library.ExternalSubtitle) ([]byte, error) {
	at := file.Torrent
	if at == nil {
		return nil, errors.New("not a subtitle file of a torrent")
	}
	if data, ok, err := s.keptTorrentSubtitle(ctx, at); ok || err != nil {
		return data, err
	}
	result, err, _ := s.flight.Do("torrent subtitle "+at.Version.String()+" "+strconv.Itoa(at.Index), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), torrentSubtitleWait)
		defer cancel()
		if data, ok, err := s.keptTorrentSubtitle(ctx, at); ok || err != nil {
			return data, err
		}
		if s.torrents == nil {
			return nil, errNoTorrents
		}
		spec, err := torrentSpec(library.Version{Torrent: &at.Torrent})
		if err != nil {
			return nil, err
		}
		started := time.Now()
		stop := s.torrents.Play(spec)
		defer stop()
		data, err := s.torrents.ReadFile(ctx, spec, at.Index, maxTorrentSubtitle)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, `UPDATE torrent_subtitles SET data = $3, fetched_at = now()
			WHERE version_id = $1 AND file_index = $2`, at.Version, at.Index, data); err != nil {
			s.logger.Warn("Saving a subtitle file of a torrent failed", "error", err)
		}
		s.logger.Debug("Fetched a subtitle file of a torrent", "bytes", len(data), "duration", time.Since(started))
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// keptTorrentSubtitle reads the text of a subtitle file inside a torrent
// fetched before: ok is false for one not fetched yet.
func (s *Service) keptTorrentSubtitle(ctx context.Context, at *library.TorrentSubtitle) (data []byte, ok bool, err error) {
	err = s.db.QueryRow(ctx, "SELECT data FROM torrent_subtitles WHERE version_id = $1 AND file_index = $2",
		at.Version, at.Index).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, library.ErrNotFound
	}
	return data, err == nil && data != nil, err
}

// PrefetchTorrentSubtitle fetches a subtitle file inside a version's
// torrent in the background, unless it was fetched before (see
// TorrentSubtitle), so that it is ready when the app asks for it.
func (s *Service) PrefetchTorrentSubtitle(file library.ExternalSubtitle) {
	go func() {
		if _, err := s.TorrentSubtitle(context.Background(), file); err != nil {
			s.logger.Info("A subtitle file of a torrent could not be fetched", "addon", file.Addon, "error", err)
		}
	}()
}
