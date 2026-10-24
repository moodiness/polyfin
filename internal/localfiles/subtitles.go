package localfiles

import (
	"bufio"
	"context"
	"io"
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
// or WebVTT files, PGS files (.sup) and VobSub pairs (an .idx file and the
// .sub file of the same name) in the video's folder whose name begins with
// the video's, then tells their language, title and flags, as in
// "Movie (2001).srt", "Movie (2001).fr.srt", "Movie (2001).en.forced.sup"
// or "Movie (2001).en.sdh.idx". A disc's are in its disc folder, named
// after the folder. A scan stores them with their video; the folder's
// streams offer them as the video's subtitle files (see Streams), read
// from the folder or the share as its videos are (see Open).

// subtitleFormats are the subtitle files a scan lists, by their extension,
// and their format. A VobSub pair is listed by its .idx file, whose .sub
// file is its pair.
var subtitleFormats = map[string]string{".srt": "srt", ".ass": "ass", ".ssa": "ssa", ".vtt": "vtt", ".sup": "sup", ".idx": "idx"}

// subtitleTags is what a subtitle file's name tells past its video's (see
// playback.SubtitleTags), as a scan stores it.
type subtitleTags struct {
	language, title                    string
	forced, hearingImpaired, isDefault bool
}

// readSubtitleTags reads a subtitle file's name past its video's as the
// subtitle files inside torrents are read (see playback.ReadSubtitleTags).
func readSubtitleTags(rest string) subtitleTags {
	tags := playback.ReadSubtitleTags(rest)
	return subtitleTags{language: tags.Language, title: tags.Title, forced: tags.Forced, hearingImpaired: tags.HearingImpaired,
		isDefault: tags.Default}
}

// sidecar is a subtitle file found beside a video: the video's path, the
// file's size and modification time, its format and what its name tells.
// A VobSub pair's .sub file is pair; its size is both files' and its
// modification time the later one.
type sidecar struct {
	video  string
	file   found
	format string
	pair   string
	subtitleTags
}

// sidecars finds the video each subtitle file found belongs to, by the
// subtitle file's path: a video in its folder whose name its own begins
// with, followed by a dot or its extension, the longest such name when
// several are; names are compared in any case. A disc's name is its
// folder's, the folder's own (root) for a disc at its root. Subtitle files
// that belong to no video are left out, and so are .idx files without
// their .sub file; the titles of shows' discs have none. entries are the
// folder's files, by path, as a scan reads them in a folder of kind.
func sidecars(kind, root string, entries map[string]entry, sides map[string]found) map[string]sidecar {
	type video struct{ name, path string }
	videos := map[string][]video{}
	for rel, e := range entries {
		dir, name := path.Dir(rel), stem(rel)
		if _, _, ok := showTitle(rel); ok && kind == KindShows {
			continue
		}
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
	// A VobSub pair's .sub file, by its .idx file's path, in any case.
	subs := map[string]string{}
	for rel := range sides {
		if strings.EqualFold(path.Ext(rel), ".sub") {
			subs[strings.ToLower(strings.TrimSuffix(rel, path.Ext(rel)))] = rel
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
		var rest string
		for _, v := range videos[path.Dir(rel)] {
			if after, ok := playback.SubtitleNamed(v.name, name); ok && len(v.name) > len(owner.name) {
				owner, rest = v, after
			}
		}
		if owner.path == "" {
			continue
		}
		side := sidecar{video: owner.path, file: f, format: format, subtitleTags: readSubtitleTags(rest)}
		if format == "idx" {
			if side.pair = subs[strings.ToLower(strings.TrimSuffix(rel, path.Ext(rel)))]; side.pair == "" {
				continue
			}
			sub := sides[side.pair]
			side.file.size += sub.size
			if sub.modified.After(side.file.modified) {
				side.file.modified = sub.modified
			}
		}
		result[rel] = side
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
		s.format == found.format && s.pair == found.pair && s.subtitleTags == found.subtitleTags
}

// storeSubtitles stores the subtitle files found beside a folder's videos
// (see sidecars), those new or changed, and forgets those gone, but those
// in the folders the walk skipped. A video's files are forgotten with it.
// A VobSub pair whose name tells no language has its .idx file's, read
// when the pair is new or changed (see idxLanguage).
func (s *Service) storeSubtitles(ctx context.Context, id accounts.ID, list listing, found map[string]sidecar) error {
	// The videos are the folder's files and its extras (see extras.go).
	rows, err := s.db.Query(ctx, `SELECT path, id, false FROM local_files WHERE addon_id = $1
		UNION ALL SELECT path, id, true FROM local_extras WHERE addon_id = $1`, id)
	if err != nil {
		return err
	}
	videos := map[string]accounts.ID{}
	extras := map[accounts.ID]bool{}
	var rel string
	var video accounts.ID
	var extra bool
	if _, err := pgx.ForEachRow(rows, []any{&rel, &video, &extra}, func() error {
		videos[rel], extras[video] = video, extra
		return nil
	}); err != nil {
		return err
	}
	rows, err = s.db.Query(ctx, `SELECT path, COALESCE(file_id, extra_id), size, modified, format, pair, language, title, forced,
			hearing_impaired, is_default
		FROM local_subtitles WHERE addon_id = $1`, id)
	if err != nil {
		return err
	}
	stored := map[string]storedSubtitle{}
	var row storedSubtitle
	if _, err := pgx.ForEachRow(rows, []any{&rel, &row.video, &row.file.size, &row.file.modified, &row.format, &row.pair, &row.language, &row.title,
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
		if f.format == "idx" && f.language == "" {
			if old, ok := stored[rel]; ok && old.language != "" && old.file.size == f.file.size && old.file.modified.Equal(f.file.modified) {
				f.language = old.language
			} else {
				f.language = idxLanguage(ctx, list, rel)
			}
			found[rel] = f
		}
		if old, ok := stored[rel]; ok && old.same(video, f) {
			continue
		}
		var file, extra *accounts.ID
		if extras[video] {
			extra = &video
		} else {
			file = &video
		}
		batch.Queue(`INSERT INTO local_subtitles (addon_id, file_id, extra_id, path, size, modified, format, pair, language, title, forced,
				hearing_impaired, is_default) VALUES ($1, $2, $13, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (addon_id, path) DO UPDATE SET file_id = $2, extra_id = $13, size = $4, modified = $5, format = $6, pair = $7,
				language = $8, title = $9, forced = $10, hearing_impaired = $11, is_default = $12`,
			id, file, rel, f.file.size, f.file.modified, f.format, f.pair, f.language, f.title, f.forced, f.hearingImpaired, f.isDefault, extra)
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

// idxLanguage is the language of a VobSub pair's first track, as its .idx
// file names it, as in "id: fr, index: 0": empty when it names none
// Polyfin knows, or cannot be read. A pair offers its first track only.
func idxLanguage(ctx context.Context, list listing, rel string) string {
	file, err := list.open(ctx, rel)
	if err != nil {
		return ""
	}
	defer file.Close()
	lines := bufio.NewScanner(io.LimitReader(file, 1<<20))
	for lines.Scan() {
		if value, ok := strings.CutPrefix(strings.TrimSpace(lines.Text()), "id:"); ok {
			code, _, _ := strings.Cut(value, ",")
			if code = strings.TrimSpace(code); playback.KnownLanguage(code) {
				return code
			}
			return ""
		}
	}
	return ""
}

// fileSubtitles lists the subtitle files of a folder's videos, by video:
// as an addon lists a stream's, served by the loopback interface as the
// videos are (see UseFiles). A file's identifier changes with it, so that
// the text kept of it is read again.
func (s *Service) fileSubtitles(ctx context.Context, videos []accounts.ID) (map[accounts.ID][]stremio.Subtitle, error) {
	rows, err := s.db.Query(ctx, `SELECT COALESCE(file_id, extra_id), id, size, modified, format, language, title, forced, hearing_impaired,
			is_default
		FROM local_subtitles WHERE file_id = ANY($1) OR extra_id = ANY($1) ORDER BY path`, videos)
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
