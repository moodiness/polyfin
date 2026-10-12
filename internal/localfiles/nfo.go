package localfiles

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
)

// NFO files, which Jellyfin and media managers write beside a title's
// files, tell its identifiers, and for a home video, its title, plot and
// date. A scan reads those alone, never the artwork or people they hold,
// from the NFO file of each unit (see unitNFOs), and only once it is new
// or changed; an identifier found matches the unit when neither a link nor
// its name does.

// maxNFO bounds what is read of an NFO file: its identifiers come before
// its long lists of actors.
const maxNFO = 1 << 20

var (
	// imdbAddress is the address of an IMDb title's page, which an NFO
	// file that is not XML may hold: its path is /title/tt…, after a
	// language for some.
	imdbAddress = regexp.MustCompile(`(?i)\bhttps?://[^\s/]+/(?:[a-z]{2}/)?title/(tt\d{1,12})\b`)
	// tmdbPattern is a TMDB identifier.
	tmdbPattern = regexp.MustCompile(`^\d{1,10}$`)
)

// nfoInfo is what an NFO file gives: the IMDb and TMDB identifiers, and a
// title, a plot, a date and a year, empty or zero for none.
type nfoInfo struct {
	imdb, tmdb, title, plot string
	released                *time.Time
	year                    int
}

// id is the identifier a unit is matched by from what its NFO file gives
// (see nfoID).
func (n nfoInfo) id() string { return nfoID(n.imdb, n.tmdb) }

// nfoDates are the elements of an NFO file that give a date, in the order
// they are preferred.
var nfoDates = []string{"premiered", "aired", "releasedate"}

// readNFO reads what an NFO file gives: the IMDb and TMDB identifiers of
// the <uniqueid type="imdb"> and <uniqueid type="tmdb"> elements of its
// root, else of the older <imdbid> and <tmdbid>; its <title>, <plot> and
// <year>, and its date, the first of nfoDates read as a day; an IMDb
// title's address in a file that is not XML. A file that stops being
// well-formed gives what was read before.
func readNFO(r io.Reader) (nfoInfo, error) {
	var info nfoInfo
	data, err := io.ReadAll(io.LimitReader(r, maxNFO))
	if err != nil {
		return info, err
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if !bytes.HasPrefix(data, []byte("<")) {
		if m := imdbAddress.FindSubmatch(data); m != nil {
			info.imdb = strings.ToLower(string(m[1]))
		}
		return info, nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(bytes.ToValidUTF8(data, []byte("?"))))
	decoder.Strict = false
	// Identifiers are ASCII whatever the file's encoding: the bytes of
	// another one that are not UTF-8 are replaced.
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	var olderIMDb, olderTMDB, element, kind string
	dates := map[string]time.Time{}
	var text strings.Builder
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				element, kind = strings.ToLower(t.Name.Local), ""
				text.Reset()
				for _, attr := range t.Attr {
					if strings.EqualFold(attr.Name.Local, "type") {
						kind = strings.ToLower(strings.TrimSpace(attr.Value))
					}
				}
			}
		case xml.CharData:
			if depth == 2 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 2 {
				raw := strings.TrimSpace(text.String())
				value := strings.ToLower(raw)
				switch {
				case element == "uniqueid" && kind == "imdb" && info.imdb == "" && imdbPattern.MatchString(value):
					info.imdb = value
				case element == "uniqueid" && kind == "tmdb" && info.tmdb == "" && tmdbPattern.MatchString(value):
					info.tmdb = value
				case element == "imdbid" && olderIMDb == "" && imdbPattern.MatchString(value):
					olderIMDb = value
				case element == "tmdbid" && olderTMDB == "" && tmdbPattern.MatchString(value):
					olderTMDB = value
				case element == "title" && info.title == "":
					info.title = raw
				case element == "plot" && info.plot == "":
					info.plot = raw
				case element == "year" && info.year == 0:
					if year, err := strconv.Atoi(raw); err == nil && year > 0 && year < 10000 {
						info.year = year
					}
				case slices.Contains(nfoDates, element):
					if _, ok := dates[element]; !ok && len(raw) >= 10 {
						if day, err := time.Parse(time.DateOnly, raw[:10]); err == nil {
							dates[element] = day
						}
					}
				}
			}
			depth--
		}
	}
	if info.imdb == "" {
		info.imdb = olderIMDb
	}
	if info.tmdb == "" {
		info.tmdb = olderTMDB
	}
	for _, element := range nfoDates {
		if day, ok := dates[element]; ok {
			info.released = &day
			break
		}
	}
	return info, nil
}

// nfoID is the identifier a unit is matched by from what its NFO file
// gives: its IMDb identifier, else "tmdb:" and its TMDB identifier, else
// empty.
func nfoID(imdb, tmdb string) string {
	switch {
	case imdb != "":
		return imdb
	case tmdb != "":
		return "tmdb:" + tmdb
	}
	return ""
}

// unitNFOs finds the NFO file of each unit of a folder of kind among its
// side files, by unit, empty for none; names are compared in any case. A
// show's is its folder's tvshow.nfo, a disc's its folder's movie.nfo. A
// movie file's is its own, its name with .nfo, else its folder's movie.nfo
// when that folder is the movie's own: its video files all read as one
// title, the versions of one movie. A home video's is the same, its folder
// its own when it is the only video in it (see videoFolders). entries are
// the folder's files, by path.
func unitNFOs(kind string, entries map[string]entry, sides map[string]found) map[string]string {
	units := map[string]string{}
	names := map[string]string{}
	for rel := range sides {
		if !strings.EqualFold(path.Ext(rel), ".nfo") {
			continue
		}
		key := strings.ToLower(rel)
		if other, ok := names[key]; !ok || rel < other {
			names[key] = rel
		}
	}
	if len(names) == 0 {
		return units
	}
	in := func(dir, file string) string { return names[strings.ToLower(path.Join(dir, file))] }
	// The folders holding the files of one movie, by the title their files
	// read as.
	own := map[string]bool{}
	switch kind {
	case KindMovies:
		titles := map[string]string{}
		for rel, e := range entries {
			if e.unit != rel {
				continue
			}
			dir, title := path.Dir(rel), fold(e.title.title)+"|"+strconv.Itoa(e.title.year)
			if first, ok := titles[dir]; !ok {
				titles[dir], own[dir] = title, true
			} else if first != title {
				own[dir] = false
			}
		}
	case KindHomeVideos:
		own = videoFolders(entries)
	}
	for rel, e := range entries {
		if _, done := units[e.unit]; done {
			continue
		}
		nfo := ""
		switch {
		case kind == KindShows:
			// A show's folder; an episode at the folder's top has none.
			if e.unit != rel {
				nfo = in(e.unit, "tvshow.nfo")
			}
		case e.unit != rel:
			nfo = in(e.unit, "movie.nfo")
		default:
			nfo = names[strings.ToLower(strings.TrimSuffix(rel, path.Ext(rel))+".nfo")]
			if nfo == "" && own[path.Dir(rel)] {
				nfo = in(path.Dir(rel), "movie.nfo")
			}
		}
		units[e.unit] = nfo
	}
	return units
}

// storedNFO is an NFO file a scan read: its size and modification time
// then, and what it gave.
type storedNFO struct {
	file found
	info nfoInfo
}

// readNFOs reads the NFO files of a folder's units, places (see unitNFOs),
// those new or changed since they were read, a few at once; it stores what
// they give, and forgets the files no unit has now, but those in the
// folders the walk skipped. It returns what each file gives, by path. A
// file that cannot be read gives what it gave before, and is read again at
// the next scan.
func (s *Service) readNFOs(ctx context.Context, id accounts.ID, list listing, places map[string]string) (map[string]nfoInfo, error) {
	rows, err := s.db.Query(ctx, "SELECT path, size, modified, imdb_id, tmdb_id, title, plot, released, year FROM local_side_files WHERE addon_id = $1", id)
	if err != nil {
		return nil, err
	}
	stored := map[string]storedNFO{}
	for rows.Next() {
		var rel string
		var n storedNFO
		if err := rows.Scan(&rel, &n.file.size, &n.file.modified, &n.info.imdb, &n.info.tmdb, &n.info.title, &n.info.plot, &n.info.released,
			&n.info.year); err != nil {
			rows.Close()
			return nil, err
		}
		stored[rel] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	infos := map[string]nfoInfo{}
	read := map[string]storedNFO{}
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for _, rel := range places {
		if _, ok := infos[rel]; ok || rel == "" {
			continue
		}
		old, known := stored[rel]
		infos[rel] = old.info
		f := list.sides[rel]
		if known && old.file.size == f.size && old.file.modified.Equal(f.modified) {
			continue
		}
		group.Go(func() error {
			readCtx, cancel := context.WithTimeout(groupCtx, listTimeout)
			defer cancel()
			n := storedNFO{file: f}
			file, err := list.open(readCtx, rel)
			if err == nil {
				n.info, err = readNFO(file)
				file.Close()
			}
			if err != nil {
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				s.logger.Debug("An NFO file cannot be read", "path", rel, "error", err)
				return nil
			}
			mu.Lock()
			read[rel] = n
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	for rel, n := range read {
		infos[rel] = n.info
	}
	var forgotten []string
	for rel := range stored {
		if _, ok := infos[rel]; !ok && !under(rel, list.skipped) {
			forgotten = append(forgotten, rel)
		}
	}
	if len(read) == 0 && len(forgotten) == 0 {
		return infos, nil
	}
	batch := &pgx.Batch{}
	for rel, n := range read {
		batch.Queue(`INSERT INTO local_side_files (addon_id, path, size, modified, imdb_id, tmdb_id, title, plot, released, year)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, imdb_id = $5, tmdb_id = $6, title = $7, plot = $8,
				released = $9, year = $10`,
			id, rel, n.file.size, n.file.modified, n.info.imdb, n.info.tmdb, n.info.title, n.info.plot, n.info.released, n.info.year)
	}
	if len(forgotten) > 0 {
		batch.Queue("DELETE FROM local_side_files WHERE addon_id = $1 AND path = ANY($2)", id, forgotten)
	}
	return infos, s.db.SendBatch(ctx, batch).Close()
}
