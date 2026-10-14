package letterboxd

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// MaxExportBytes bounds the export's ZIP file.
const MaxExportBytes = 20 << 20

// maxFileBytes bounds each file read out of the ZIP: an export's are a
// few megabytes at most, and a ZIP may unpack to far more than it weighs.
const maxFileBytes = 64 << 20

// ErrInvalidExport reports a file that is not a Letterboxd export: not a
// ZIP, without watched.csv or diary.csv, or with a file read whose columns
// are not an export's.
var ErrInvalidExport = errors.New("not a Letterboxd export")

// The files of the export read, under its root. The movies liked are those
// of the likes folder; its other files, the reviews and lists liked, are
// not read.
const (
	watchedFile   = "watched.csv"
	diaryFile     = "diary.csv"
	ratingsFile   = "ratings.csv"
	watchlistFile = "watchlist.csv"
	likesFile     = "likes/films.csv"
)

// dateLayout is how the export writes dates.
const dateLayout = "2006-01-02"

// Export is what an export tells of each movie, one Movie per name and
// year, in the order the files first give them.
type Export struct {
	Movies []Movie
}

// Movie is a movie of the export, and what the member did with it.
type Movie struct {
	// Name and Year name the movie as Letterboxd does; Year is zero when
	// the export gives none.
	Name string
	Year int
	// Link is the movie's Letterboxd address, else a diary entry's when
	// only the diary gives the movie; empty when the export gives none
	// that is a web address.
	Link string
	// Watched is set for a movie of watched.csv or of the diary. PlayedAt
	// is its latest diary date, else the date of watched.csv, nil when
	// neither gives one; Plays counts its diary lines, at least 1.
	Watched  bool
	PlayedAt *time.Time
	Plays    int
	// Rating is the member's, on Polyfin's scale of 0 to 10, from
	// ratings.csv, else from the diary; nil when neither rates it.
	Rating *float64
	// Watchlist is set for a movie of watchlist.csv, added at AddedAt.
	Watchlist bool
	AddedAt   *time.Time
	// Liked is set for a movie the member liked.
	Liked bool
}

// Parse reads an export's ZIP file, in memory: the files listed above,
// under the folder that holds watched.csv or diary.csv, the ZIP's root
// or a folder of it. Each file's columns are read by their names. It
// fails with ErrInvalidExport.
func Parse(data []byte) (Export, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Export{}, ErrInvalidExport
	}
	root, ok := exportRoot(archive)
	if !ok {
		return Export{}, ErrInvalidExport
	}
	files := map[string]*zip.File{}
	for _, f := range archive.File {
		files[f.Name] = f
	}
	p := &parser{byKey: map[movieKey]int{}}
	// The movie's own link comes first: watched.csv's, then those of the
	// files whose links name movies, the diary's last.
	for _, read := range []struct {
		name string
		row  func(row)
	}{
		{watchedFile, p.watched},
		{ratingsFile, p.rated},
		{watchlistFile, p.listed},
		{likesFile, p.liked},
		{diaryFile, p.logged},
	} {
		f := files[path.Join(root, read.name)]
		if f == nil {
			continue
		}
		if err := readCSV(f, read.row); err != nil {
			return Export{}, err
		}
	}
	return p.export(), nil
}

// exportRoot finds the folder that holds watched.csv or diary.csv: the
// ZIP's root, else the shallowest folder, a folder zipped whole.
func exportRoot(archive *zip.Reader) (string, bool) {
	root, depth := "", -1
	for _, f := range archive.File {
		dir, base := path.Split(f.Name)
		if base != watchedFile && base != diaryFile {
			continue
		}
		dir = strings.TrimSuffix(dir, "/")
		d := 0
		if dir != "" {
			d = strings.Count(dir, "/") + 1
		}
		if d <= 1 && (depth < 0 || d < depth) {
			root, depth = dir, d
		}
	}
	return root, depth >= 0
}

// row is a line of a CSV file, read by its columns' names.
type row struct {
	columns map[string]int
	values  []string
}

// get is the value of the column named name, trimmed, empty when the file
// has no such column.
func (r row) get(name string) string {
	i, ok := r.columns[name]
	if !ok || i >= len(r.values) {
		return ""
	}
	return strings.TrimSpace(r.values[i])
}

// readCSV reads f, a CSV file whose first line names its columns, and
// hands each line naming a movie to read. A file without a Name column is
// not an export's.
func readCSV(f *zip.File, read func(row)) error {
	opened, err := f.Open()
	if err != nil {
		return ErrInvalidExport
	}
	defer opened.Close()
	limited := &io.LimitedReader{R: opened, N: maxFileBytes + 1}
	r := csv.NewReader(limited)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return ErrInvalidExport
	}
	columns := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))
		if _, seen := columns[name]; !seen {
			columns[name] = i
		}
	}
	if _, ok := columns["Name"]; !ok {
		return ErrInvalidExport
	}
	for {
		values, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || limited.N <= 0 {
			return ErrInvalidExport
		}
		line := row{columns: columns, values: values}
		if line.get("Name") != "" {
			read(line)
		}
	}
	if limited.N <= 0 {
		return ErrInvalidExport
	}
	return nil
}

// movieKey names a movie in every file of the export: by its name and
// year, as diary lines link to the diary entry, not to the movie.
type movieKey struct {
	name string
	year int
}

// parser gathers the movies of an export's files.
type parser struct {
	movies []Movie
	byKey  map[movieKey]int
	// diary are the latest diary line with a date of each movie, by its
	// index, and the latest one with a rating.
	diaryDate   map[int]time.Time
	diaryRating map[int]diaryRating
}

type diaryRating struct {
	at     time.Time
	rating float64
}

// movie is the movie line names, added when new; link, the line's link,
// is kept when the movie has none.
func (p *parser) movie(line row) *Movie {
	name := line.get("Name")
	year, _ := strconv.Atoi(line.get("Year"))
	key := movieKey{name, max(year, 0)}
	i, ok := p.byKey[key]
	if !ok {
		i = len(p.movies)
		p.byKey[key] = i
		p.movies = append(p.movies, Movie{Name: key.name, Year: key.year})
	}
	m := &p.movies[i]
	if m.Link == "" {
		m.Link = webAddress(line.get("Letterboxd URI"))
	}
	return m
}

func (p *parser) watched(line row) {
	m := p.movie(line)
	m.Watched = true
	if at := date(line.get("Date")); at != nil && m.PlayedAt == nil {
		m.PlayedAt = at
	}
}

func (p *parser) rated(line row) {
	m := p.movie(line)
	if rating, ok := stars(line.get("Rating")); ok {
		m.Rating = &rating
	}
}

func (p *parser) listed(line row) {
	m := p.movie(line)
	m.Watchlist = true
	m.AddedAt = date(line.get("Date"))
}

func (p *parser) liked(line row) {
	p.movie(line).Liked = true
}

// logged counts a diary line: a viewing of its movie, on its Watched Date.
func (p *parser) logged(line row) {
	m := p.movie(line)
	i := p.byKey[movieKey{m.Name, m.Year}]
	m.Watched = true
	m.Plays++
	at := date(line.get("Watched Date"))
	if at == nil {
		at = date(line.get("Date"))
	}
	if p.diaryDate == nil {
		p.diaryDate, p.diaryRating = map[int]time.Time{}, map[int]diaryRating{}
	}
	if at != nil {
		if latest, ok := p.diaryDate[i]; !ok || at.After(latest) {
			p.diaryDate[i] = *at
		}
	}
	if rating, ok := stars(line.get("Rating")); ok {
		var when time.Time
		if at != nil {
			when = *at
		}
		if latest, ok := p.diaryRating[i]; !ok || !when.Before(latest.at) {
			p.diaryRating[i] = diaryRating{when, rating}
		}
	}
}

// export ends the reading: a movie's latest diary date stands over the
// date of watched.csv, and its diary rating counts when ratings.csv does
// not rate it.
func (p *parser) export() Export {
	for i := range p.movies {
		m := &p.movies[i]
		if at, ok := p.diaryDate[i]; ok {
			m.PlayedAt = &at
		}
		if r, ok := p.diaryRating[i]; ok && m.Rating == nil {
			m.Rating = &r.rating
		}
		if m.Watched {
			m.Plays = max(m.Plays, 1)
		}
	}
	return Export{Movies: p.movies}
}

// date reads a date of the export, at noon UTC, the same day in most of
// the world; nil when value is not one.
func date(value string) *time.Time {
	day, err := time.Parse(dateLayout, value)
	if err != nil {
		return nil
	}
	return new(day.Add(12 * time.Hour))
}

// stars reads a rating of half a star to five stars, as Polyfin's 1 to 10.
func stars(value string) (float64, bool) {
	rating, err := strconv.ParseFloat(value, 64)
	if err != nil || rating < 0.5 || rating > 5 {
		return 0, false
	}
	return rating * 2, true
}

// webAddress is value when it is an http or https address, else empty: a
// link the result shows must lead to a web page only.
func webAddress(value string) string {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return u.String()
}
