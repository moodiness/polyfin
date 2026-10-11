package localfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
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
	"github.com/moodiness/polyfin/internal/subtitles"
)

// A music folder holds audio files, read with their tags, and answers as
// an Eclipse addon does (see library.Local): one row of its albums, album,
// artist and playlist pages, searches, streams and lyrics. Its tracks are
// grouped into albums by album artist and album name, and its albums into
// artists by album artist; a track names each of its artists (see
// splitNames). An album's artwork is the cover a track embeds, else an
// image beside its tracks (see folderImageNames); an artist's picture is an
// image in their folder (see artistFolders). Its .m3u and .m3u8 files are
// its playlists (see readM3U), and its .lrc files the lyrics of the
// tracks of the same name. A scan reads the tags of the files new or
// changed with ffprobe; those of a share through a server on the loopback
// interface that serves each at most probeBudget bytes.

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
	// maxText bounds a lyrics file or a playlist.
	maxText = 1 << 20
)

// trackInfo is what a scan read of an audio file: its tags (or, without
// them, its file and folder names), and its audio. artist and albumArtist
// are as written; artists and albumArtists name them one by one.
type trackInfo struct {
	title, artist, album, albumArtist string
	artists, albumArtists             []string
	track, disc, year                 int
	genres                            []string
	compilation                       bool
	lyrics                            string
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
// case. A track's artists are those its artists tag names, else its artist
// tag; its album artists, those its album artists tag names, else its
// album artist tag (see splitNames). Its lyrics are its lyrics tag's:
// synced, plain, or one of an ID3 tag's (lyrics-eng), as ffprobe names
// them.
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
	info.artists = splitNames(first("artists"))
	info.artist = first("artist")
	if info.artist == "" {
		info.artist = strings.Join(info.artists, "; ")
	}
	if info.artist == "" {
		info.artist = first("album_artist", "albumartist")
	}
	info.album = first("album")
	info.albumArtists = splitNames(first("albumartists", "album_artists"))
	info.albumArtist = first("album_artist", "albumartist", "album artist")
	if info.albumArtist == "" {
		info.albumArtist = strings.Join(info.albumArtists, "; ")
	}
	if len(info.albumArtists) == 0 {
		info.albumArtists = splitNames(info.albumArtist)
	}
	info.track = number("track", "tracknumber")
	info.disc = number("disc", "discnumber")
	if year := firstNumber.FindString(first("date", "year", "originaldate", "tdrc", "tyer")); len(year) >= 4 {
		info.year, _ = strconv.Atoi(year[:4])
	}
	info.genres = splitNames(first("genre"))
	info.compilation = first("compilation", "itunescompilation", "tcmp") == "1"
	info.lyrics = first("syncedlyrics", "lyrics", "unsyncedlyrics")
	if info.lyrics == "" {
		// An ID3 tag's, named after its language and description.
		for _, key := range slices.Sorted(maps.Keys(tags)) {
			if strings.HasPrefix(key, "lyrics-") {
				info.lyrics = tags[key]
				break
			}
		}
	}
	info.duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	return info, nil
}

// splitNames lists the names a tag holds, separated by ";" as written, or
// as ffprobe joins the values of a tag given several times: each without
// the blanks around it, once in any letter case.
func splitNames(tag string) []string {
	var names []string
	for name := range strings.SplitSeq(tag, ";") {
		if name = strings.TrimSpace(name); name != "" && !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) }) {
			names = append(names, name)
		}
	}
	return names
}

// named completes what a file's tags tell by its path, as in
// "Artist/Album/01 Title.flac": its title is its name without the number
// heading it, which is its track number; its album is its folder's name,
// the folder above's for a disc's folder ("CD1"), which gives its disc; its
// artist, the folder's above. Without an album artist tag, its album
// artist is "Various Artists" for a compilation, else its artist, as
// written, one name. It returns why the track cannot be listed: "no_title"
// or "no_album", empty when it can.
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
		if len(info.albumArtists) == 0 {
			info.albumArtists = splitNames(info.albumArtist)
		}
	case info.compilation:
		info.albumArtist, info.albumArtists = variousArtists, []string{variousArtists}
	case info.artist != "":
		info.albumArtist, info.albumArtists = info.artist, []string{info.artist}
	}
	if info.artist == "" {
		info.artist = info.albumArtist
	}
	if len(info.artists) == 0 {
		info.artists = splitNames(info.artist)
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
// new and changed ones read and stored, and the artwork of albums and
// artists chosen again when their tracks or images changed; its lyrics
// files and playlists are read when new or changed. root is the folder's
// path in the container, empty for a share: a playlist's absolute paths
// are read within it.
func (s *Service) scanMusic(ctx context.Context, id accounts.ID, list listing, root string) error {
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
			_, err = s.db.Exec(groupCtx, `INSERT INTO local_tracks (addon_id, path, size, modified, title, artist, album, album_artist, artists,
					artist_ids, album_artists, album_artist_ids, album_id, track, disc, year, genres, duration, codec, container, sample_rate,
					bit_depth, cover, lyrics, reason)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
				ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, title = $5, artist = $6, album = $7, album_artist = $8,
					artists = $9, artist_ids = $10, album_artists = $11, album_artist_ids = $12, album_id = $13, track = $14, disc = $15,
					year = $16, genres = $17, duration = $18, codec = $19, container = $20, sample_rate = $21, bit_depth = $22, cover = $23,
					lyrics = $24, reason = $25`,
				id, rel, f.size, f.modified, info.title, info.artist, info.album, info.albumArtist, nonNil(info.artists),
				artistIDs(info.artists), nonNil(info.albumArtists), artistIDs(info.albumArtists), nameID("al", info.albumArtist, info.album),
				info.track, info.disc, info.year, nonNil(info.genres), info.duration, info.codec, audioContainers[strings.ToLower(path.Ext(rel))],
				info.sampleRate, info.bitDepth, info.cover, info.lyrics, reason)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if err := s.storeLyrics(ctx, id, list); err != nil {
		return err
	}
	roots := []string{}
	if root != "" {
		roots = append(roots, filepath.ToSlash(filepath.Clean(root)), filepath.ToSlash(list.root))
	}
	if err := s.storePlaylists(ctx, id, list, roots); err != nil {
		return err
	}
	return s.storeCovers(ctx, id, list, target)
}

// nonNil is names, an empty list for none, which the database keeps as
// such rather than as null.
func nonNil(names []string) []string { return append([]string{}, names...) }

// artistIDs identifies artists by their names (see nameID).
func artistIDs(names []string) []string {
	ids := make([]string, len(names))
	for i, name := range names {
		ids[i] = nameID("ar", name)
	}
	return ids
}

// storedSides reads the side files of a folder a table keeps, by path,
// with their size and modification time when they were read.
func (s *Service) storedSides(ctx context.Context, table string, id accounts.ID) (map[string]found, error) {
	rows, err := s.db.Query(ctx, "SELECT path, size, modified FROM "+table+" WHERE addon_id = $1", id)
	if err != nil {
		return nil, err
	}
	stored := map[string]found{}
	var rel string
	var f found
	_, err = pgx.ForEachRow(rows, []any{&rel, &f.size, &f.modified}, func() error {
		stored[rel] = f
		return nil
	})
	return stored, err
}

// sideTexts reads the side files of a listing that keep tells, those new
// or changed since stored by size and modification time, as text (see
// subtitles.DecodeText): a file larger than maxText, or that cannot be
// read, is left out, and what was read of it before is kept. gone are the
// files stored no longer listed, but for those of folders skipped.
func (s *Service) sideTexts(ctx context.Context, list listing, stored map[string]found, keep func(name string) bool) (map[string]string, []string, error) {
	texts := map[string]string{}
	for rel, f := range list.sides {
		if !keep(path.Base(rel)) {
			continue
		}
		if old, ok := stored[rel]; ok && old.size == f.size && old.modified.Equal(f.modified) {
			continue
		}
		text, err := s.readText(ctx, list, rel)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			s.logger.Debug("A file of a music folder cannot be read", "path", rel, "error", err)
			continue
		}
		texts[rel] = text
	}
	var gone []string
	for rel := range stored {
		if _, ok := list.sides[rel]; !ok && !under(rel, list.skipped) {
			gone = append(gone, rel)
		}
	}
	return texts, gone, nil
}

// readText reads a text file of a listing, at most maxText bytes.
func (s *Service) readText(ctx context.Context, list listing, rel string) (string, error) {
	file, err := list.open(ctx, rel)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxText+1))
	switch {
	case err != nil:
		return "", err
	case len(data) > maxText:
		return "", errors.New("the file is too large")
	}
	return subtitles.DecodeText(data), nil
}

// storeLyrics keeps the text of a music folder's lyrics files new or
// changed, and forgets those gone.
func (s *Service) storeLyrics(ctx context.Context, id accounts.ID, list listing) error {
	stored, err := s.storedSides(ctx, "local_lyrics", id)
	if err != nil {
		return err
	}
	texts, gone, err := s.sideTexts(ctx, list, stored, lyricsFile)
	if err != nil {
		return err
	}
	for rel, text := range texts {
		f := list.sides[rel]
		if _, err := s.db.Exec(ctx, `INSERT INTO local_lyrics (addon_id, path, size, modified, text) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, text = $5`, id, rel, f.size, f.modified, text); err != nil {
			return err
		}
	}
	if len(gone) > 0 {
		_, err = s.db.Exec(ctx, "DELETE FROM local_lyrics WHERE addon_id = $1 AND path = ANY($2)", id, gone)
	}
	return err
}

// storePlaylists reads a music folder's playlists new or changed, and
// forgets those gone. roots are the folder's paths in the container, as
// configured and with its symbolic links resolved, none for a share.
func (s *Service) storePlaylists(ctx context.Context, id accounts.ID, list listing, roots []string) error {
	stored, err := s.storedSides(ctx, "local_playlists", id)
	if err != nil {
		return err
	}
	texts, gone, err := s.sideTexts(ctx, list, stored, m3uFile)
	if err != nil {
		return err
	}
	for rel, text := range texts {
		f := list.sides[rel]
		title, entries := readM3U(rel, text, roots)
		if _, err := s.db.Exec(ctx, `INSERT INTO local_playlists (addon_id, path, size, modified, title, entries) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, title = $5, entries = $6`,
			id, rel, f.size, f.modified, title, nonNil(entries)); err != nil {
			return err
		}
	}
	if len(gone) > 0 {
		_, err = s.db.Exec(ctx, "DELETE FROM local_playlists WHERE addon_id = $1 AND path = ANY($2)", id, gone)
	}
	return err
}

// readM3U reads an M3U playlist at rel in a folder: its title, that
// of its #PLAYLIST line, else its file's name; and the paths in the folder
// its lines name, in order. A line names a path relative to the playlist's
// folder, an absolute path (or a file: address) within one of roots, the
// folder's paths in the container, with slashes or backslashes; lines
// leading out of the folder, other addresses and comments (#EXTINF) are
// left out.
func readM3U(rel, text string, roots []string) (string, []string) {
	title := ""
	var entries []string
	dir := path.Dir(rel)
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if directive, ok := strings.CutPrefix(line, "#"); ok {
			if name, ok := cutPrefixFold(directive, "PLAYLIST:"); ok && title == "" {
				title = strings.TrimSpace(name)
			}
			continue
		}
		if entry := m3uEntry(dir, line, roots); entry != "" {
			entries = append(entries, entry)
		}
	}
	if title == "" {
		title = stem(rel)
	}
	return title, entries
}

// cutPrefixFold is strings.CutPrefix, in any letter case.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// m3uEntry is the path in the folder a playlist's line names (see
// readM3U), empty for none; dir is the playlist's folder.
func m3uEntry(dir, line string, roots []string) string {
	if rest, ok := cutPrefixFold(line, "file:"); ok {
		u, err := url.Parse("file:" + rest)
		if err != nil {
			return ""
		}
		line = u.Path
	} else if strings.Contains(line, "://") {
		return ""
	}
	line = strings.ReplaceAll(line, `\`, "/")
	if path.IsAbs(line) {
		line = path.Clean(line)
		for _, root := range roots {
			if rest, ok := strings.CutPrefix(line, strings.TrimSuffix(root, "/")+"/"); ok {
				return rest
			}
		}
		return ""
	}
	if len(line) > 1 && line[1] == ':' {
		// A Windows drive's path, out of the folder.
		return ""
	}
	entry := path.Join(dir, line)
	if entry == ".." || strings.HasPrefix(entry, "../") {
		return ""
	}
	return entry
}

// coverSource is where an album's artwork or an artist's picture comes
// from: a track embedding a cover, or an image, with its size and
// modification time.
type coverSource struct {
	path     string
	embedded bool
	file     found
}

// storeCovers chooses each album's artwork: the cover its first track
// embedding one embeds (by disc, number and path), else the first of
// folderImageNames beside its first track; and each artist's picture, the
// first of artistImageNames in their folder (see artistFolders). The cover
// of a source unchanged since is not read again; the artwork of albums and
// artists gone is forgotten.
func (s *Service) storeCovers(ctx context.Context, id accounts.ID, list listing, target probeTarget) error {
	rows, err := s.db.Query(ctx, `SELECT album_id, album_artist_ids, path, cover, size, modified FROM local_tracks WHERE addon_id = $1 AND reason = ''
		ORDER BY album_id, disc, track, path`, id)
	if err != nil {
		return err
	}
	type album struct {
		first    string
		artists  []string
		embedded *coverSource
	}
	albums := map[string]*album{}
	var albumID, rel string
	var artists []string
	var cover bool
	var f found
	if _, err := pgx.ForEachRow(rows, []any{&albumID, &artists, &rel, &cover, &f.size, &f.modified}, func() error {
		a := albums[albumID]
		if a == nil {
			a = &album{first: rel, artists: slices.Clone(artists)}
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
	// imageIn is the first image of names in dir, any letter case.
	imageIn := func(dir string, names []string) (coverSource, bool) {
		for _, name := range names {
			for _, ext := range imageExtensions {
				if image, ok := images[strings.ToLower(path.Join(dir, name+ext))]; ok {
					return coverSource{path: image, file: list.sides[image]}, true
				}
			}
		}
		return coverSource{}, false
	}
	desired := map[string]coverSource{}
	firsts := map[string][]string{}
	for albumID, a := range albums {
		firsts[a.first] = a.artists
		if a.embedded != nil {
			desired[albumID] = *a.embedded
		} else if source, ok := imageIn(path.Dir(a.first), folderImageNames); ok {
			desired[albumID] = source
		}
	}
	for dir, artist := range artistFolders(firsts) {
		if source, ok := imageIn(dir, artistImageNames); ok {
			desired[artist] = source
		}
	}
	rows, err = s.db.Query(ctx, "SELECT item_id, source, data IS NOT NULL, size, modified FROM local_covers WHERE addon_id = $1", id)
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
	for itemID, want := range desired {
		if old, ok := stored[itemID]; ok && old.path == want.path && old.embedded == want.embedded && old.file.size == want.file.size &&
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
				delete(desired, itemID)
				continue
			}
		} else {
			image = want.path
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO local_covers (addon_id, item_id, path, data, content_type, source, size, modified)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (addon_id, item_id) DO UPDATE SET path = $3, data = $4, content_type = $5, source = $6, size = $7, modified = $8`,
			id, itemID, image, data, contentType, want.path, want.file.size, want.file.modified); err != nil {
			return err
		}
	}
	var forgotten []string
	for itemID, old := range stored {
		if _, ok := desired[itemID]; !ok && !under(old.path, list.skipped) {
			forgotten = append(forgotten, itemID)
		}
	}
	if len(forgotten) > 0 {
		_, err = s.db.Exec(ctx, "DELETE FROM local_covers WHERE addon_id = $1 AND item_id = ANY($2)", id, forgotten)
	}
	return err
}

// artistFolders finds the folders of artists from the first track of each
// album, by path, with the identifiers of the album's artists: an album's
// folder is its first track's, the folder above for a disc's folder
// ("CD2"), and an artist's folder holds the folders of their albums, and
// of no album of another artist or of several; the folder's root holds
// the albums of its loose tracks too. It returns each artist's identifier
// by their folder.
func artistFolders(firsts map[string][]string) map[string]string {
	artists := map[string]string{}
	mixed := map[string]bool{}
	for first, ids := range firsts {
		dir := path.Dir(first)
		if discMarker.MatchString(path.Base(dir)) {
			dir = path.Dir(dir)
		}
		dir = path.Dir(dir)
		switch artist, ok := artists[dir]; {
		case len(ids) != 1 || ok && artist != ids[0]:
			mixed[dir] = true
		default:
			artists[dir] = ids[0]
		}
	}
	for dir := range mixed {
		delete(artists, dir)
	}
	return artists
}

// albumsQuery lists a music folder's albums, those whose tracks filter
// keeps: their identifier, title, artist and artists (their first
// track's album artist, as written and one by one, joined by the unit
// separator), year, track count, genres (joined by the unit separator),
// and artwork.
const albumsQuery = `WITH albums AS (
	SELECT album_id, (array_agg(album ORDER BY disc, track, path))[1] AS album,
		(array_agg(album_artist ORDER BY disc, track, path))[1] AS artist,
		(array_agg(array_to_string(album_artists, chr(31)) ORDER BY disc, track, path))[1] AS artists, max(year) AS year, count(*) AS tracks,
		coalesce(string_agg(array_to_string(genres, chr(31)), chr(31)), '') AS genres
	FROM local_tracks WHERE addon_id = $1 AND reason = '' %s GROUP BY album_id)
SELECT a.album_id, a.album, a.artist, a.artists, a.year, a.tracks, a.genres, c.id, coalesce(c.path, ''), coalesce(c.content_type, ''), c.modified
FROM albums a LEFT JOIN local_covers c ON c.addon_id = $1 AND c.item_id = a.album_id
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
		var artists, genres, image, contentType string
		var cover *accounts.ID
		var modified *time.Time
		if err := row.Scan(&a.ID, &a.Title, &a.Artist, &artists, &a.Year, &a.TrackCount, &genres, &cover, &image, &contentType,
			&modified); err != nil {
			return a, err
		}
		a.ArtworkURL = s.coverURL(cover, image, contentType, modified)
		a.Artists = splitJoined(artists)
		a.Genres = splitJoined(genres)
		return a, nil
	})
}

// splitJoined lists the names joined by the unit separator, each once.
func splitJoined(joined string) []string {
	var names []string
	for name := range strings.SplitSeq(joined, "\x1f") {
		if name != "" && !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) }) {
			names = append(names, name)
		}
	}
	return names
}

// lyricsOf matches the lyrics file l of a track t: the .lrc file of the
// same name beside it, in any letter case.
const lyricsOf = `l.addon_id = t.addon_id AND lower(regexp_replace(l.path, '\.[^./]*$', '')) = lower(regexp_replace(t.path, '\.[^./]*$', ''))`

// coverURL is the address an album's artwork or an artist's picture is
// served at by the loopback interface (see Open), changing with it; empty
// for none.
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

// tracksQuery lists a music folder's tracks with their artists and album
// artists, one by one, whether they have lyrics (a lyrics file or tag),
// and their album's artwork.
const tracksQuery = `SELECT t.id, t.title, t.artist, t.album, t.album_artist, t.artists, t.album_artists, t.album_id, t.track, t.disc, t.year,
	t.genres, t.duration, t.container, t.lyrics <> '' OR EXISTS (SELECT FROM local_lyrics l WHERE ` + lyricsOf + ` AND l.text <> ''),
	c.id, coalesce(c.path, ''), coalesce(c.content_type, ''), c.modified
FROM local_tracks t LEFT JOIN local_covers c ON c.addon_id = t.addon_id AND c.item_id = t.album_id
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
		if err := row.Scan(&track, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist, &t.Artists, &t.AlbumArtists, &t.AlbumID, &t.Number, &t.Disc,
			&t.Year, &t.Genres, &duration, &t.Format, &t.Lyrics, &cover, &image, &contentType, &modified); err != nil {
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

// MusicArtist returns an artist's page: the albums they are an album
// artist of, and their tracks, on their albums or others', at most
// artistTracks of them; their picture, else their first album's artwork;
// their genres are their tracks'.
func (s *Service) MusicArtist(ctx context.Context, source accounts.ID, id string) (eclipse.Artist, error) {
	artist := eclipse.Artist{ID: id}
	err := s.db.QueryRow(ctx, `SELECT n.name FROM local_tracks t,
			unnest(t.album_artists || t.artists, t.album_artist_ids || t.artist_ids) WITH ORDINALITY AS n(name, id, position)
		WHERE t.addon_id = $1 AND t.reason = '' AND n.id = $2 ORDER BY t.path, n.position LIMIT 1`, source, id).Scan(&artist.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return eclipse.Artist{}, stremio.ErrNotFound
	}
	if err != nil {
		return eclipse.Artist{}, err
	}
	if artist.Albums, err = s.albums(ctx, source, "AND $2 = ANY(album_artist_ids)", "", id); err != nil {
		return eclipse.Artist{}, err
	}
	if artist.TopTracks, err = s.tracks(ctx, source, `AND ($2 = ANY(t.artist_ids) OR $2 = ANY(t.album_artist_ids))
		ORDER BY lower(t.album), t.disc, t.track, t.path LIMIT $3`, id, artistTracks); err != nil {
		return eclipse.Artist{}, err
	}
	var cover *accounts.ID
	var image, contentType string
	var modified *time.Time
	err = s.db.QueryRow(ctx, "SELECT id, path, content_type, modified FROM local_covers WHERE addon_id = $1 AND item_id = $2", source, id).
		Scan(&cover, &image, &contentType, &modified)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return eclipse.Artist{}, err
	}
	artist.ArtworkURL = s.coverURL(cover, image, contentType, modified)
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

// MusicArtists lists a music folder's album artists, by name, each with
// their picture, else the artwork of their first album, as their page
// gives them.
func (s *Service) MusicArtists(ctx context.Context, source accounts.ID) ([]eclipse.Artist, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (lower(n.name)) n.name, n.id, c.id, coalesce(c.path, ''), coalesce(c.content_type, ''), c.modified
		FROM local_tracks t CROSS JOIN LATERAL unnest(t.album_artists, t.album_artist_ids) AS n(name, id)
		LEFT JOIN local_covers c ON c.addon_id = t.addon_id AND c.item_id = n.id
		WHERE t.addon_id = $1 AND t.reason = '' AND n.name <> '' ORDER BY lower(n.name), t.path`, source)
	if err != nil {
		return nil, err
	}
	artists, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (eclipse.Artist, error) {
		var a eclipse.Artist
		var image, contentType string
		var cover *accounts.ID
		var modified *time.Time
		err := row.Scan(&a.Name, &a.ID, &cover, &image, &contentType, &modified)
		a.ArtworkURL = s.coverURL(cover, image, contentType, modified)
		return a, err
	})
	if err != nil {
		return nil, err
	}
	albums, err := s.albums(ctx, source, "", "")
	if err != nil {
		return nil, err
	}
	artwork := map[string]string{}
	for _, album := range albums {
		for _, name := range album.Artists {
			if id := nameID("ar", name); artwork[id] == "" {
				artwork[id] = album.ArtworkURL
			}
		}
	}
	for i := range artists {
		if artists[i].ArtworkURL == "" {
			artists[i].ArtworkURL = artwork[artists[i].ID]
		}
	}
	return artists, nil
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
// artist and album), albums (by title and artist), artists (by name) and
// playlists (by title): every word of the term must be found, in any
// letter case.
func (s *Service) MusicSearch(ctx context.Context, source accounts.ID, term string) (eclipse.Results, error) {
	patterns := searchPatterns(term)
	if len(patterns) == 0 {
		return eclipse.Results{}, nil
	}
	var results eclipse.Results
	var err error
	if results.Tracks, err = s.tracks(ctx, source, `AND lower(t.title || ' ' || t.artist || ' ' || array_to_string(t.artists, ' ') || ' ' || t.album)
		LIKE ALL($2) ORDER BY lower(t.title), t.path LIMIT $3`, patterns, searchResults); err != nil {
		return results, err
	}
	if results.Albums, err = s.albums(ctx, source, `AND album_id IN (SELECT album_id FROM local_tracks WHERE addon_id = $1
		AND lower(album || ' ' || album_artist) LIKE ALL($2))`, " LIMIT $3", patterns, searchResults); err != nil {
		return results, err
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (lower(name)) name FROM (
			SELECT unnest(artists || album_artists) AS name FROM local_tracks WHERE addon_id = $1 AND reason = '') n
		WHERE name <> '' AND lower(name) LIKE ALL($2) ORDER BY lower(name), name LIMIT $3`, source, patterns, searchResults)
	if err != nil {
		return results, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return results, err
	}
	for _, name := range names {
		results.Artists = append(results.Artists, eclipse.Artist{ID: nameID("ar", name), Name: name})
	}
	results.Playlists, err = s.playlists(ctx, source, "AND lower(p.title) LIKE ALL($2)", " LIMIT $3", patterns, searchResults)
	return results, err
}

// entriesQuery lists the tracks of the folder a playlist p names, by its
// entries, in order (n): for each entry, the track of that path, the one
// of that exact path first among those differing in letter case alone.
const entriesQuery = `SELECT e.n, t.id, t.album_id FROM unnest(p.entries) WITH ORDINALITY AS e(path, n)
	JOIN LATERAL (SELECT id, album_id FROM local_tracks WHERE addon_id = p.addon_id AND reason = '' AND lower(path) = lower(e.path)
		ORDER BY path = e.path DESC, path LIMIT 1) t ON true`

// playlistsQuery lists a music folder's playlists that filter keeps,
// with how many tracks of the folder they name, and the artwork of the
// album of the first; those naming none are left out.
const playlistsQuery = `WITH playlists AS (
	SELECT p.id, p.title, p.path, count(m.id) AS tracks, (array_agg(m.album_id ORDER BY m.n))[1] AS album_id
	FROM local_playlists p LEFT JOIN LATERAL (` + entriesQuery + `) m ON true
	WHERE p.addon_id = $1 %s GROUP BY p.id)
SELECT p.id, p.title, p.tracks, c.id, coalesce(c.path, ''), coalesce(c.content_type, ''), c.modified
FROM playlists p LEFT JOIN local_covers c ON c.addon_id = $1 AND c.item_id = p.album_id
WHERE p.tracks > 0 ORDER BY lower(p.title), p.path`

// playlists lists the playlists playlistsQuery finds, filter narrowing
// them with args from $2 on, page appended.
func (s *Service) playlists(ctx context.Context, id accounts.ID, filter, page string, args ...any) ([]eclipse.Playlist, error) {
	rows, err := s.db.Query(ctx, strings.Replace(playlistsQuery, "%s", filter, 1)+page, append([]any{id}, args...)...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (eclipse.Playlist, error) {
		var p eclipse.Playlist
		var playlist accounts.ID
		var image, contentType string
		var cover *accounts.ID
		var modified *time.Time
		if err := row.Scan(&playlist, &p.Title, &p.TrackCount, &cover, &image, &contentType, &modified); err != nil {
			return p, err
		}
		p.ID = playlist.String()
		p.ArtworkURL = s.coverURL(cover, image, contentType, modified)
		return p, nil
	})
}

// MusicPlaylists lists a music folder's playlists, by title.
func (s *Service) MusicPlaylists(ctx context.Context, source accounts.ID) ([]eclipse.Playlist, error) {
	return s.playlists(ctx, source, "", "")
}

// MusicPlaylist returns a playlist's page: the playlist and the tracks of
// the folder it names, in its order.
func (s *Service) MusicPlaylist(ctx context.Context, source accounts.ID, id string) (eclipse.Playlist, error) {
	playlist, err := accounts.ParseID(id)
	if err != nil {
		return eclipse.Playlist{}, stremio.ErrNotFound
	}
	found, err := s.playlists(ctx, source, "AND p.id = $2", "", playlist)
	if err != nil {
		return eclipse.Playlist{}, err
	}
	if len(found) == 0 {
		return eclipse.Playlist{}, stremio.ErrNotFound
	}
	page := found[0]
	rows, err := s.db.Query(ctx, `SELECT m.id FROM local_playlists p, LATERAL (`+entriesQuery+`) m
		WHERE p.addon_id = $1 AND p.id = $2 ORDER BY m.n`, source, playlist)
	if err != nil {
		return eclipse.Playlist{}, err
	}
	order, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return eclipse.Playlist{}, err
	}
	tracks, err := s.tracks(ctx, source, "AND t.id = ANY($2)", order)
	if err != nil {
		return eclipse.Playlist{}, err
	}
	byID := make(map[string]eclipse.Track, len(tracks))
	for _, t := range tracks {
		byID[t.ID] = t
	}
	for _, id := range order {
		if t, ok := byID[id.String()]; ok {
			page.Tracks = append(page.Tracks, t)
		}
	}
	page.TrackCount = len(page.Tracks)
	return page, nil
}

// MusicLyrics returns a track's lyrics, LRC or plain text: those of its
// lyrics file, else of its lyrics tag; empty for none.
func (s *Service) MusicLyrics(ctx context.Context, source accounts.ID, id string) (string, error) {
	track, err := accounts.ParseID(id)
	if err != nil {
		return "", stremio.ErrNotFound
	}
	var text string
	err = s.db.QueryRow(ctx, `SELECT coalesce((SELECT l.text FROM local_lyrics l WHERE `+lyricsOf+` AND l.text <> '' ORDER BY l.path LIMIT 1),
		t.lyrics) FROM local_tracks t WHERE t.addon_id = $1 AND t.id = $2`, source, track).Scan(&text)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", stremio.ErrNotFound
	}
	return text, err
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
