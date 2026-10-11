package localfiles

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Subtitle files beside a video, as Jellyfin reads them: SubRip, ASS, SSA
// or WebVTT files in the video's folder whose name begins with the
// video's, then tells their language, title and flags, as in
// "Movie (2001).srt", "Movie (2001).fr.srt", "Movie (2001).en.forced.srt"
// or "Movie (2001).en.sdh.ass". A disc's are in its disc folder, named
// after the folder. A scan stores them with their video; the folder's
// streams offer them as the video's subtitle files (see Streams), read
// from the folder or the share as its videos are (see Open).

// subtitleFormats are the subtitle files a scan lists, by their extension,
// and their format.
var subtitleFormats = map[string]string{".srt": "srt", ".ass": "ass", ".ssa": "ssa", ".vtt": "vtt"}

// subtitleTags is what a subtitle file's name tells past its video's: its
// language as written ("fr", "fre", "French"), its title (the words that
// are neither a language nor a flag), and whether it is forced, for the
// hearing impaired, or the one to show by default.
type subtitleTags struct {
	language, title                    string
	forced, hearingImpaired, isDefault bool
}

// readSubtitleTags reads the words of a subtitle file's name past its
// video's, separated by dots, in any order and letter case: "forced" or
// "foreign", "sdh", "hi" or "cc" for the hearing impaired, "default", and
// the first word naming a language; the others make its title.
func readSubtitleTags(rest string) subtitleTags {
	var tags subtitleTags
	var title []string
	for word := range strings.SplitSeq(rest, ".") {
		word = strings.TrimSpace(word)
		switch strings.ToLower(word) {
		case "":
		case "forced", "foreign":
			tags.forced = true
		case "sdh", "hi", "cc":
			tags.hearingImpaired = true
		case "default":
			tags.isDefault = true
		default:
			if tags.language == "" && playback.KnownLanguage(word) {
				tags.language = word
			} else {
				title = append(title, word)
			}
		}
	}
	tags.title = strings.Join(title, " ")
	return tags
}

// sidecar is a subtitle file found beside a video: the video's path, the
// file's size and modification time, its format and what its name tells.
type sidecar struct {
	video  string
	file   found
	format string
	subtitleTags
}

// sidecars finds the video each subtitle file found belongs to, by the
// subtitle file's path: a video in its folder whose name its own begins
// with, followed by a dot or its extension, the longest such name when
// several are; names are compared in any case. A disc's name is its
// folder's, the folder's own (root) for a disc at its root. Subtitle files
// that belong to no video are left out. entries are the folder's files, by
// path, as a scan reads them in a folder of kind.
func sidecars(kind, root string, entries map[string]entry, sides map[string]found) map[string]sidecar {
	type video struct{ name, path string }
	videos := map[string][]video{}
	for rel, e := range entries {
		dir, name := path.Dir(rel), stem(rel)
		if kind == KindMovies && e.unit != rel {
			dir, name = e.unit, path.Base(e.unit)
			if e.unit == "." {
				name = root
			}
		}
		if name != "" {
			videos[dir] = append(videos[dir], video{name: name, path: rel})
		}
	}
	result := map[string]sidecar{}
	for rel, f := range sides {
		format := subtitleFormats[strings.ToLower(path.Ext(rel))]
		if format == "" {
			continue
		}
		name := stem(rel)
		var owner video
		for _, v := range videos[path.Dir(rel)] {
			if len(v.name) > len(owner.name) && len(name) >= len(v.name) && strings.EqualFold(name[:len(v.name)], v.name) &&
				(len(name) == len(v.name) || name[len(v.name)] == '.') {
				owner = v
			}
		}
		if owner.path == "" {
			continue
		}
		result[rel] = sidecar{video: owner.path, file: f, format: format, subtitleTags: readSubtitleTags(name[len(owner.name):])}
	}
	return result
}

// storedSubtitle is a subtitle file a scan stored: its video's identifier
// and what it found of it.
type storedSubtitle struct {
	video accounts.ID
	sidecar
}

// same reports whether a stored subtitle file is the one found now.
func (s storedSubtitle) same(video accounts.ID, found sidecar) bool {
	return s.video == video && s.file.size == found.file.size && s.file.modified.Equal(found.file.modified) &&
		s.format == found.format && s.subtitleTags == found.subtitleTags
}

// storeSubtitles stores the subtitle files found beside a folder's videos
// (see sidecars), those new or changed, and forgets those gone, but those
// in the folders the walk skipped. A video's files are forgotten with it.
func (s *Service) storeSubtitles(ctx context.Context, id accounts.ID, list listing, found map[string]sidecar) error {
	rows, err := s.db.Query(ctx, "SELECT path, id FROM local_files WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	videos := map[string]accounts.ID{}
	var rel string
	var video accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&rel, &video}, func() error {
		videos[rel] = video
		return nil
	}); err != nil {
		return err
	}
	rows, err = s.db.Query(ctx, `SELECT path, file_id, size, modified, format, language, title, forced, hearing_impaired, is_default
		FROM local_subtitles WHERE addon_id = $1`, id)
	if err != nil {
		return err
	}
	stored := map[string]storedSubtitle{}
	var row storedSubtitle
	if _, err := pgx.ForEachRow(rows, []any{&rel, &row.video, &row.file.size, &row.file.modified, &row.format, &row.language, &row.title,
		&row.forced, &row.hearingImpaired, &row.isDefault}, func() error {
		stored[rel] = row
		return nil
	}); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for rel, f := range found {
		video, ok := videos[f.video]
		if !ok {
			delete(found, rel)
			continue
		}
		if old, ok := stored[rel]; ok && old.same(video, f) {
			continue
		}
		batch.Queue(`INSERT INTO local_subtitles (addon_id, file_id, path, size, modified, format, language, title, forced, hearing_impaired,
				is_default) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (addon_id, path) DO UPDATE SET file_id = $2, size = $4, modified = $5, format = $6, language = $7, title = $8,
				forced = $9, hearing_impaired = $10, is_default = $11`,
			id, video, rel, f.file.size, f.file.modified, f.format, f.language, f.title, f.forced, f.hearingImpaired, f.isDefault)
	}
	var gone []string
	for rel := range stored {
		if _, ok := found[rel]; !ok && !under(rel, list.skipped) {
			gone = append(gone, rel)
		}
	}
	if len(gone) > 0 {
		batch.Queue("DELETE FROM local_subtitles WHERE addon_id = $1 AND path = ANY($2)", id, gone)
	}
	if batch.Len() == 0 {
		return nil
	}
	return s.db.SendBatch(ctx, batch).Close()
}

// fileSubtitles lists the subtitle files of a folder's videos, by video:
// as an addon lists a stream's, served by the loopback interface as the
// videos are (see UseFiles). A file's identifier changes with it, so that
// the text kept of it is read again.
func (s *Service) fileSubtitles(ctx context.Context, videos []accounts.ID) (map[accounts.ID][]stremio.Subtitle, error) {
	rows, err := s.db.Query(ctx, `SELECT file_id, id, size, modified, format, language, title, forced, hearing_impaired, is_default
		FROM local_subtitles WHERE file_id = ANY($1) ORDER BY path`, videos)
	if err != nil {
		return nil, err
	}
	result := map[accounts.ID][]stremio.Subtitle{}
	var video, file accounts.ID
	var size int64
	var modified time.Time
	var subtitle stremio.Subtitle
	_, err = pgx.ForEachRow(rows, []any{&video, &file, &size, &modified, &subtitle.Format, &subtitle.Lang, &subtitle.Title, &subtitle.Forced,
		&subtitle.HearingImpaired, &subtitle.Default}, func() error {
		subtitle.ID = stremio.Text(file.String() + "|" + strconv.FormatInt(size, 10) + "|" + strconv.FormatInt(modified.UnixMicro(), 10))
		subtitle.URL = s.files + file.String() + "." + subtitle.Format
		result[video] = append(result[video], subtitle)
		return nil
	})
	return result, err
}
