package localfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A music folder holds audio files, read with their tags, and answers as
// an Eclipse addon does (see library.Local): one row of its albums, album
// and artist pages, searches and streams. Its tracks are grouped into
// albums by album artist and album name, and its albums into artists by
// album artist; an album's artwork is the cover a track embeds, else an
// image beside its tracks (see folderImageNames). A scan reads the tags of
// the files new or changed with ffprobe; those of a share through a server
// on the loopback interface that serves each at most probeBudget bytes.

const (
	// probeBudget bounds what the tags and cover of a share's file are read
	// from: its head, and its end for files that keep their index there.
	probeBudget = 16 << 20
	// maxCover bounds an embedded cover.
	maxCover = 10 << 20
	// probeTimeout bounds the reading of one file's tags or cover.
	probeTimeout = time.Minute
	// artistTracks bounds the tracks an artist's page lists, and
	// searchResults what a search finds of each kind.
	artistTracks  = 100
	searchResults = 30
	// variousArtists is the album artist of a compilation that names none.
	variousArtists = "Various Artists"
)

// trackInfo is what a scan read of an audio file: its tags (or, without
// them, its file and folder names), and its audio.
type trackInfo struct {
	title, artist, album, albumArtist string
	track, disc, year                 int
	genres                            []string
	compilation                       bool
	duration                          float64
	codec                             string
	sampleRate, bitDepth              int
	cover                             bool
}

// probed is the part of ffprobe's JSON a scan reads.
type probed struct {
	Streams []struct {
		CodecType        string            `json:"codec_type"`
		CodecName        string            `json:"codec_name"`
		SampleRate       string            `json:"sample_rate"`
		BitsPerRawSample string            `json:"bits_per_raw_sample"`
		BitsPerSample    int               `json:"bits_per_sample"`
		Disposition      map[string]int    `json:"disposition"`
		Tags             map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

var (
	// leadingNumber is a number heading a file's name, a disc's joined
	// to it ("1-02"), as in "02 - Title" or "1-02 Title".
	leadingNumber = regexp.MustCompile(`^(?:(\d{1,2})[-.])?(\d{1,3})(?:[ ._-]+|$)`)
	// firstNumber is the first number of a tag such as "3/12".
	firstNumber = regexp.MustCompile(`\d+`)
)

// readProbe reads tags and audio from ffprobe's JSON: the container's tags,
// then the audio stream's (Ogg files keep theirs there), with keys in any
// case.
func readProbe(data []byte) (trackInfo, error) {
	var p probed
	if err := json.Unmarshal(data, &p); err != nil {
		return trackInfo{}, err
	}
	tags := map[string]string{}
	add := func(m map[string]string) {
		for key, value := range m {
			key = strings.ToLower(key)
			if _, ok := tags[key]; !ok && strings.TrimSpace(value) != "" {
				tags[key] = strings.TrimSpace(value)
			}
		}
	}
	add(p.Format.Tags)
	var info trackInfo
	for _, stream := range p.Streams {
		switch {
		case stream.CodecType == "video" && stream.Disposition["attached_pic"] == 1:
			info.cover = true
		case stream.CodecType == "audio" && info.codec == "":
			info.codec = stream.CodecName
			info.sampleRate, _ = strconv.Atoi(stream.SampleRate)
			info.bitDepth, _ = strconv.Atoi(stream.BitsPerRawSample)
			if info.bitDepth == 0 {
				info.bitDepth = stream.BitsPerSample
			}
			add(stream.Tags)
		}
	}
	first := func(keys ...string) string {
		for _, key := range keys {
			if value := tags[key]; value != "" {
				return value
			}
		}
		return ""
	}
	number := func(keys ...string) int {
		n, _ := strconv.Atoi(firstNumber.FindString(first(keys...)))
		return n
	}
	info.title = first("title")
	info.artist = first("artist", "album_artist", "albumartist")
	info.album = first("album")
	info.albumArtist = first("album_artist", "albumartist", "album artist")
	info.track = number("track", "tracknumber")
	info.disc = number("disc", "discnumber")
	if year := firstNumber.FindString(first("date", "year", "originaldate", "tdrc", "tyer")); len(year) >= 4 {
		info.year, _ = strconv.Atoi(year[:4])
	}
	for genre := range strings.SplitSeq(first("genre"), ";") {
		if genre = strings.TrimSpace(genre); genre != "" && !slices.ContainsFunc(info.genres, func(g string) bool { return strings.EqualFold(g, genre) }) {
			info.genres = append(info.genres, genre)
		}
	}
	info.compilation = first("compilation", "itunescompilation", "tcmp") == "1"
	info.duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	return info, nil
}

// named completes what a file's tags tell by its path, as in
// "Artist/Album/01 Title.flac": its title is its name without the number
// heading it, which is its track number; its album is its folder's name,
// the folder above's for a disc's folder ("CD1"), which gives its disc; its
// artist, the folder's above. It returns why the track cannot be listed:
// "no_title" or "no_album", empty when it can.
func named(rel string, info trackInfo) (trackInfo, string) {
	folders := strings.Split(rel, "/")
	folders = folders[:len(folders)-1]
	if n := len(folders); n > 0 && discMarker.MatchString(folders[n-1]) {
		if info.disc == 0 {
			info.disc, _ = strconv.Atoi(firstNumber.FindString(folders[n-1]))
		}
		folders = folders[:n-1]
	}
	if info.title == "" {
		name := strings.TrimSpace(stem(rel))
		if m := leadingNumber.FindStringSubmatch(name); m != nil && len(name) > len(m[0]) {
			if info.track == 0 {
				info.track, _ = strconv.Atoi(m[2])
			}
			if info.disc == 0 && m[1] != "" {
				info.disc, _ = strconv.Atoi(m[1])
			}
			name = strings.TrimSpace(name[len(m[0]):])
		}
		info.title = name
	}
	if info.album == "" && len(folders) > 0 {
		info.album = folders[len(folders)-1]
	}
	if info.artist == "" && len(folders) > 1 {
		info.artist = folders[len(folders)-2]
	}
	switch {
	case info.albumArtist != "":
	case info.compilation:
		info.albumArtist = variousArtists
	default:
		info.albumArtist = info.artist
	}
	if info.artist == "" {
		info.artist = info.albumArtist
	}
	switch {
	case info.title == "":
		return info, "no_title"
	case info.album == "":
		return info, "no_album"
	}
	return info, ""
}

// nameID identifies an artist or an album of a folder by its names, in any
// letter case and without the blanks around them, as Jellyfin compares
// artists: the same names give the same identifier through every scan.
func nameID(prefix string, names ...string) string {
	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
		h.Write([]byte{0x1f})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

// probeTarget is what ffprobe and FFmpeg read a file of a listing from:
// its path in the container, or, for a share, its address on the loopback
// server (see probeServer).
type probeTarget func(rel string) string

// probeServer serves the files of a share's listing on the loopback
// interface for ffprobe and FFmpeg, with byte ranges, each at most
// probeBudget bytes over every request for it. stop ends it.
func probeServer(list listing) (probeTarget, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	var mu sync.Mutex
	paths := map[string]string{}
	served := map[string]*int64{}
	server := &http.Server{ReadHeaderTimeout: probeTimeout, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		rel, ok := paths[strings.TrimPrefix(r.URL.Path, "/")]
		count := served[rel]
		mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		file, err := list.open(r.Context(), rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		seeker, ok := file.(io.ReadSeeker)
		if !ok {
			http.Error(w, "not seekable", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, path.Base(rel), time.Time{}, &budgeted{ReadSeeker: seeker, mu: &mu, served: count})
	})}
	go func() { _ = server.Serve(listener) }()
	base := "http://" + listener.Addr().String() + "/"
	target := func(rel string) string {
		mu.Lock()
		defer mu.Unlock()
		key := strconv.Itoa(len(paths)) + strings.ToLower(path.Ext(rel))
		paths[key], served[rel] = rel, new(int64)
		return base + key
	}
	return target, func() { _ = server.Close() }, nil
}

// budgeted reads a file until probeBudget bytes of it were served.
type budgeted struct {
	io.ReadSeeker
	mu     *sync.Mutex
	served *int64
}

var errBudget = errors.New("the file's head was read: tags and covers are bounded")

func (b *budgeted) Read(p []byte) (int, error) {
	b.mu.Lock()
	left := probeBudget - *b.served
	b.mu.Unlock()
	if left <= 0 {
		return 0, errBudget
	}
	n, err := b.ReadSeeker.Read(p[:min(int64(len(p)), left)])
	b.mu.Lock()
	*b.served += int64(n)
	b.mu.Unlock()
	return n, err
}

// probeTrack reads an audio file's tags and audio with ffprobe.
func (s *Service) probeTrack(ctx context.Context, target string) (trackInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, s.ffprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", target).Output()
	if err != nil {
		return trackInfo{}, err
	}
	return readProbe(out)
}

// readCover reads the cover an audio file embeds with FFmpeg, as it is,
// and its content type: JPEG or PNG.
func (s *Service) readCover(ctx context.Context, target string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.ffmpeg, "-v", "error", "-nostdin", "-i", target, "-map", "0:v:0", "-frames:v", "1", "-c", "copy",
		"-f", "image2pipe", "pipe:1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, maxCover+1))
	if err := cmd.Wait(); err != nil {
		return nil, "", err
	}
	if readErr != nil {
		return nil, "", readErr
	}
	switch {
	case len(data) > maxCover:
		return nil, "", errors.New("the cover is too large")
	case bytes.HasPrefix(data, []byte{0xff, 0xd8}):
		return data, "image/jpeg", nil
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return data, "image/png", nil
	}
	return nil, "", errors.New("the cover is neither JPEG nor PNG")
}

// scanMusic reads a music folder's listing: tracks gone are forgotten,
// new and changed ones read and stored, and the artwork of albums chosen
// again when their tracks or images changed.
func (s *Service) scanMusic(ctx context.Context, id accounts.ID, list listing) error {
	rows, err := s.db.Query(ctx, "SELECT path, size, modified FROM local_tracks WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	known := map[string]found{}
	var rel string
	var f found
	if _, err := pgx.ForEachRow(rows, []any{&rel, &f.size, &f.modified}, func() error {
		known[rel] = f
		return nil
	}); err != nil {
		return err
	}
	var gone []string
	for rel := range known {
		if _, ok := list.files[rel]; !ok && !under(rel, list.skipped) {
			gone = append(gone, rel)
		}
	}
	if len(gone) > 0 {
		if _, err := s.db.Exec(ctx, "DELETE FROM local_tracks WHERE addon_id = $1 AND path = ANY($2)", id, gone); err != nil {
			return err
		}
	}
	target := probeTarget(func(rel string) string { return filepath.Join(list.root, filepath.FromSlash(rel)) })
	if list.root == "" {
		var stop func()
		if target, stop, err = probeServer(list); err != nil {
			return err
		}
		defer stop()
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for rel, f := range list.files {
		if old, ok := known[rel]; ok && old.size == f.size && old.modified.Equal(f.modified) {
			continue
		}
		group.Go(func() error {
			info, err := s.probeTrack(groupCtx, target(rel))
			if err != nil {
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				// Read by its names alone.
				s.logger.Debug("The tags of an audio file cannot be read", "path", rel, "error", err)
				info = trackInfo{}
			}
			info, reason := named(rel, info)
			_, err = s.db.Exec(groupCtx, `INSERT INTO local_tracks (addon_id, path, size, modified, title, artist, album, album_artist, artist_id,
					album_artist_id, album_id, track, disc, year, genres, duration, codec, container, sample_rate, bit_depth, cover, reason)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
				ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, title = $5, artist = $6, album = $7, album_artist = $8,
					artist_id = $9, album_artist_id = $10, album_id = $11, track = $12, disc = $13, year = $14, genres = $15, duration = $16,
					codec = $17, container = $18, sample_rate = $19, bit_depth = $20, cover = $21, reason = $22`,
				id, rel, f.size, f.modified, info.title, info.artist, info.album, info.albumArtist, nameID("ar", info.artist),
				nameID("ar", info.albumArtist), nameID("al", info.albumArtist, info.album), info.track, info.disc, info.year,
				append([]string{}, info.genres...), info.duration, info.codec, audioContainers[strings.ToLower(path.Ext(rel))],
				info.sampleRate, info.bitDepth, info.cover, reason)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return s.storeCovers(ctx, id, list, target)
}

// coverSource is where an album's artwork comes from: a track embedding a
// cover, or an image beside its tracks, with its size and modification
// time.
type coverSource struct {
	path     string
	embedded bool
	file     found
}

// storeCovers chooses each album's artwork: the cover its first track
// embedding one embeds (by disc, number and path), else the first of
// folderImageNames beside its first track. The cover of a source unchanged
// since is not read again; the artwork of albums gone is forgotten.
func (s *Service) storeCovers(ctx context.Context, id accounts.ID, list listing, target probeTarget) error {
	rows, err := s.db.Query(ctx, `SELECT album_id, path, cover, size, modified FROM local_tracks WHERE addon_id = $1 AND reason = ''
		ORDER BY album_id, disc, track, path`, id)
	if err != nil {
		return err
	}
	type album struct {
		first    string
		embedded *coverSource
	}
	albums := map[string]*album{}
	var albumID, rel string
	var cover bool
	var f found
	if _, err := pgx.ForEachRow(rows, []any{&albumID, &rel, &cover, &f.size, &f.modified}, func() error {
		a := albums[albumID]
		if a == nil {
			a = &album{first: rel}
			albums[albumID] = a
		}
		if cover && a.embedded == nil {
			a.embedded = &coverSource{path: rel, embedded: true, file: f}
		}
		return nil
	}); err != nil {
		return err
	}
	images := map[string]string{}
	for rel := range list.sides {
		key := strings.ToLower(rel)
		if other, ok := images[key]; !ok || rel < other {
			images[key] = rel
		}
	}
	desired := map[string]coverSource{}
	for albumID, a := range albums {
		if a.embedded != nil {
			desired[albumID] = *a.embedded
			continue
		}
		dir := path.Dir(a.first)
	images:
		for _, name := range folderImageNames {
			for _, ext := range []string{".jpg", ".jpeg", ".png"} {
				if image, ok := images[strings.ToLower(path.Join(dir, name+ext))]; ok {
					desired[albumID] = coverSource{path: image, file: list.sides[image]}
					break images
				}
			}
		}
	}
	rows, err = s.db.Query(ctx, "SELECT album_id, source, data IS NOT NULL, size, modified FROM local_covers WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	stored := map[string]coverSource{}
	var source string
	var embedded bool
	if _, err := pgx.ForEachRow(rows, []any{&albumID, &source, &embedded, &f.size, &f.modified}, func() error {
		stored[albumID] = coverSource{path: source, embedded: embedded, file: f}
		return nil
	}); err != nil {
		return err
	}
	for albumID, want := range desired {
		if old, ok := stored[albumID]; ok && old.path == want.path && old.embedded == want.embedded && old.file.size == want.file.size &&
			old.file.modified.Equal(want.file.modified) {
			continue
		}
		var data []byte
		var contentType, image string
		if want.embedded {
			if data, contentType, err = s.readCover(ctx, target(want.path)); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				s.logger.Debug("The cover of an audio file cannot be read", "path", want.path, "error", err)
				delete(desired, albumID)
				continue
			}
		} else {
			image = want.path
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO local_covers (addon_id, album_id, path, data, content_type, source, size, modified)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (addon_id, album_id) DO UPDATE SET path = $3, data = $4, content_type = $5, source = $6, size = $7, modified = $8`,
			id, albumID, image, data, contentType, want.path, want.file.size, want.file.modified); err != nil {
			return err
		}
	}
	var forgotten []string
	for albumID, old := range stored {
		if _, ok := desired[albumID]; !ok && !under(old.path, list.skipped) {
			forgotten = append(forgotten, albumID)
		}
	}
	if len(forgotten) > 0 {
		_, err = s.db.Exec(ctx, "DELETE FROM local_covers WHERE addon_id = $1 AND album_id = ANY($2)", id, forgotten)
	}
	return err
}

// albumsQuery lists a music folder's albums, those whose tracks filter
// keeps: their identifier, title and artist (their first track's), year,
// track count, genres (joined by the unit separator), and artwork.
const albumsQuery = `WITH albums AS (
	SELECT album_id, (array_agg(album ORDER BY disc, track, path))[1] AS album,
		(array_agg(album_artist ORDER BY disc, track, path))[1] AS artist, max(year) AS year, count(*) AS tracks,
		coalesce(string_agg(array_to_string(genres, chr(31)), chr(31)), '') AS genres
	FROM local_tracks WHERE addon_id = $1 AND reason = '' %s GROUP BY album_id)
SELECT a.album_id, a.album, a.artist, a.year, a.tracks, a.genres, c.id, coalesce(c.path, ''), coalesce(c.content_type, ''), c.modified
FROM albums a LEFT JOIN local_covers c ON c.addon_id = $1 AND c.album_id = a.album_id
ORDER BY lower(a.artist), lower(a.album), a.album_id`

// albums lists the albums albumsQuery finds, filter narrowing its tracks
// with args from $2 on, page appended.
func (s *Service) albums(ctx context.Context, id accounts.ID, filter, page string, args ...any) ([]eclipse.Album, error) {
	rows, err := s.db.Query(ctx, strings.Replace(albumsQuery, "%s", filter, 1)+page, append([]any{id}, args...)...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (eclipse.Album, error) {
		var a eclipse.Album
		var genres, image, contentType string
		var cover *accounts.ID
		var modified *time.Time
		if err := row.Scan(&a.ID, &a.Title, &a.Artist, &a.Year, &a.TrackCount, &genres, &cover, &image, &contentType, &modified); err != nil {
			return a, err
		}
		a.ArtworkURL = s.coverURL(cover, image, contentType, modified)
		a.Genres = splitGenres(genres)
		return a, nil
	})
}

// splitGenres lists the genres joined by the unit separator, each once.
func splitGenres(joined string) []string {
	var genres []string
	for genre := range strings.SplitSeq(joined, "\x1f") {
		if genre != "" && !slices.ContainsFunc(genres, func(g string) bool { return strings.EqualFold(g, genre) }) {
			genres = append(genres, genre)
		}
	}
	return genres
}

// coverURL is the address an album's artwork is served at by the loopback
// interface (see Open), changing with it; empty for none.
func (s *Service) coverURL(cover *accounts.ID, image, contentType string, modified *time.Time) string {
	if cover == nil || s.files == "" {
		return ""
	}
	extension := strings.ToLower(path.Ext(image))
	if image == "" {
		extension = ".jpg"
		if contentType == "image/png" {
			extension = ".png"
		}
	}
	version := ""
	if modified != nil {
		version = "?v=" + strconv.FormatInt(modified.UnixMicro(), 36)
	}
	return s.files + cover.String() + extension + version
}

// tracksQuery lists a music folder's tracks with their album's artwork.
const tracksQuery = `SELECT t.id, t.title, t.artist, t.album, t.album_artist, t.album_id, t.track, t.disc, t.year, t.genres, t.duration,
	t.container, c.id, coalesce(c.path, ''), coalesce(c.content_type, ''), c.modified
FROM local_tracks t LEFT JOIN local_covers c ON c.addon_id = t.addon_id AND c.album_id = t.album_id
WHERE t.addon_id = $1 AND t.reason = '' `

// tracks lists the tracks tracksQuery finds, narrowed by filter with args
// from $2 on, in order.
func (s *Service) tracks(ctx context.Context, id accounts.ID, filter string, args ...any) ([]eclipse.Track, error) {
	rows, err := s.db.Query(ctx, tracksQuery+filter, append([]any{id}, args...)...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (eclipse.Track, error) {
		var t eclipse.Track
		var track accounts.ID
		var duration float64
		var image, contentType string
		var cover *accounts.ID
		var modified *time.Time
		if err := row.Scan(&track, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist, &t.AlbumID, &t.Number, &t.Disc, &t.Year, &t.Genres,
			&duration, &t.Format, &cover, &image, &contentType, &modified); err != nil {
			return t, err
		}
		t.ID = track.String()
		t.Duration = eclipse.Seconds(time.Duration(duration * float64(time.Second)))
		t.ArtworkURL = s.coverURL(cover, image, contentType, modified)
		return t, nil
	})
}

// MusicCatalog lists a page of a music folder's row of albums, from skip.
func (s *Service) MusicCatalog(ctx context.Context, source accounts.ID, catalog eclipse.Catalog, skip int) ([]eclipse.Item, int, error) {
	if catalog.ID != catalogID || catalog.Type != eclipse.TypeAlbum {
		return nil, 0, stremio.ErrNotFound
	}
	albums, err := s.albums(ctx, source, "", " LIMIT $2 OFFSET $3", eclipse.CatalogPage, skip)
	if err != nil {
		return nil, 0, err
	}
	items := make([]eclipse.Item, len(albums))
	for i := range albums {
		items[i] = eclipse.Item{Album: &albums[i]}
	}
	return items, len(items), nil
}

// MusicAlbum returns an album's page: the album and its tracks, by disc,
// number and path.
func (s *Service) MusicAlbum(ctx context.Context, source accounts.ID, id string) (eclipse.Album, error) {
	albums, err := s.albums(ctx, source, "AND album_id = $2", "", id)
	if err != nil {
		return eclipse.Album{}, err
	}
	if len(albums) == 0 {
		return eclipse.Album{}, stremio.ErrNotFound
	}
	album := albums[0]
	if album.Tracks, err = s.tracks(ctx, source, "AND t.album_id = $2 ORDER BY t.disc, t.track, t.path", id); err != nil {
		return eclipse.Album{}, err
	}
	return album, nil
}

// MusicArtist returns an artist's page: the albums they are the artist of,
// and their tracks, on their albums or others', at most artistTracks of
// them; their genres are their tracks'.
func (s *Service) MusicArtist(ctx context.Context, source accounts.ID, id string) (eclipse.Artist, error) {
	artist := eclipse.Artist{ID: id}
	err := s.db.QueryRow(ctx, `SELECT CASE WHEN album_artist_id = $2 THEN album_artist ELSE artist END FROM local_tracks
		WHERE addon_id = $1 AND reason = '' AND (album_artist_id = $2 OR artist_id = $2) ORDER BY path LIMIT 1`, source, id).Scan(&artist.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return eclipse.Artist{}, stremio.ErrNotFound
	}
	if err != nil {
		return eclipse.Artist{}, err
	}
	if artist.Albums, err = s.albums(ctx, source, "AND album_artist_id = $2", "", id); err != nil {
		return eclipse.Artist{}, err
	}
	if artist.TopTracks, err = s.tracks(ctx, source, "AND (t.artist_id = $2 OR t.album_artist_id = $2) ORDER BY lower(t.album), t.disc, t.track, t.path LIMIT $3",
		id, artistTracks); err != nil {
		return eclipse.Artist{}, err
	}
	for _, album := range artist.Albums {
		if artist.ArtworkURL == "" {
			artist.ArtworkURL = album.ArtworkURL
		}
	}
	for _, track := range artist.TopTracks {
		for _, genre := range track.Genres {
			if !slices.ContainsFunc(artist.Genres, func(g string) bool { return strings.EqualFold(g, genre) }) {
				artist.Genres = append(artist.Genres, genre)
			}
		}
	}
	return artist, nil
}

// searchPatterns are the LIKE patterns of a search's words: each must be
// found, in any letter case.
func searchPatterns(term string) []string {
	var patterns []string
	for _, word := range strings.Fields(strings.ToLower(term)) {
		word = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(word)
		patterns = append(patterns, "%"+word+"%")
	}
	return patterns
}

// MusicSearch looks a term up among a music folder's tracks (by title,
// artist and album), albums (by title and artist) and artists (by name):
// every word of the term must be found, in any letter case.
func (s *Service) MusicSearch(ctx context.Context, source accounts.ID, term string) (eclipse.Results, error) {
	patterns := searchPatterns(term)
	if len(patterns) == 0 {
		return eclipse.Results{}, nil
	}
	var results eclipse.Results
	var err error
	if results.Tracks, err = s.tracks(ctx, source, `AND lower(t.title || ' ' || t.artist || ' ' || t.album) LIKE ALL($2)
		ORDER BY lower(t.title), t.path LIMIT $3`, patterns, searchResults); err != nil {
		return results, err
	}
	if results.Albums, err = s.albums(ctx, source, `AND album_id IN (SELECT album_id FROM local_tracks WHERE addon_id = $1
		AND lower(album || ' ' || album_artist) LIKE ALL($2))`, " LIMIT $3", patterns, searchResults); err != nil {
		return results, err
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (lower(name)) name FROM (
			SELECT artist AS name FROM local_tracks WHERE addon_id = $1 AND reason = ''
			UNION ALL SELECT album_artist FROM local_tracks WHERE addon_id = $1 AND reason = '') n
		WHERE name <> '' AND lower(name) LIKE ALL($2) ORDER BY lower(name), name LIMIT $3`, source, patterns, searchResults)
	if err != nil {
		return results, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	for _, name := range names {
		results.Artists = append(results.Artists, eclipse.Artist{ID: nameID("ar", name), Name: name})
	}
	return results, err
}

// MusicStream tells where a track plays from, the loopback interface (see
// UseFiles), and what its audio is, as its scan read it.
func (s *Service) MusicStream(ctx context.Context, source accounts.ID, id string) (eclipse.Stream, error) {
	track, err := accounts.ParseID(id)
	if err != nil || s.files == "" {
		return eclipse.Stream{}, stremio.ErrNotFound
	}
	var stream eclipse.Stream
	var rel string
	err = s.db.QueryRow(ctx, `SELECT path, container, codec, sample_rate, bit_depth FROM local_tracks WHERE addon_id = $1 AND id = $2`,
		source, track).Scan(&rel, &stream.Container, &stream.Codec, &stream.SampleRate, &stream.BitDepth)
	if errors.Is(err, pgx.ErrNoRows) {
		return eclipse.Stream{}, stremio.ErrNotFound
	}
	if err != nil {
		return eclipse.Stream{}, err
	}
	stream.URL = s.files + track.String() + strings.ToLower(path.Ext(rel))
	stream.Format, stream.Manifest = stream.Container, "none"
	return stream, nil
}
