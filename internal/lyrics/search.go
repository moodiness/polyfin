package lyrics

import (
	"cmp"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Apps' lyrics editors search a track's lyrics and let administrators
// choose among the results, as Jellyfin's lyrics providers serve them:
// Search lists LRCLIB's lyrics for the track, Choose makes those of a
// result the track's, and Remove leaves the track without any.

const (
	// ProviderName names LRCLIB in search results.
	ProviderName = "LRCLIB"
	// maxSearchAnswer bounds the bytes read of a search answer: LRCLIB
	// lists 20 tracks at most, each with its lyrics plain and synced.
	maxSearchAnswer = 4 << 20
	// maxResults bounds the tracks of a search answer listed, those of the
	// closest length first.
	maxResults = 20
	// maxResultID bounds the length of a result's identifier read.
	maxResultID = 64
	// maxLength bounds the length of a track LRCLIB lists: a longer or
	// negative one is taken as unknown.
	maxLength = 24 * time.Hour
)

// providerID starts the identifiers of LRCLIB's results, as Jellyfin
// starts those of a provider's with the MD5 of its lowercase name.
var providerID = func() string {
	sum := md5.Sum([]byte(strings.ToLower(ProviderName)))
	return hex.EncodeToString(sum[:])
}()

// ErrNotFound reports lyrics that cannot be chosen: an identifier that is
// none of LRCLIB's results, a result LRCLIB no longer has or could not
// give, or lyrics turned off.
var ErrNotFound = errors.New("lyrics not found")

// Result is lyrics a search found: those of a track LRCLIB lists, synced
// or plain, and the track's artist, album, title and length (0 when
// unknown), as LRCLIB names them.
type Result struct {
	ID       string
	Artist   string
	Album    string
	Title    string
	Duration time.Duration
	Lyrics   Lyrics
}

// listed is a track as LRCLIB lists it in a search, or answers it by its
// identifier: its record, and what it is.
type listed struct {
	record
	ID         int64   `json:"id"`
	TrackName  string  `json:"trackName"`
	ArtistName string  `json:"artistName"`
	AlbumName  string  `json:"albumName"`
	Duration   float64 `json:"duration"`
}

// length is the track's length, 0 when unknown or out of bounds.
func (l listed) length() time.Duration {
	if l.Duration <= 0 || l.Duration > maxLength.Seconds() {
		return 0
	}
	return time.Duration(l.Duration * float64(time.Second))
}

// Search lists LRCLIB's lyrics for track, which it is asked for by its
// title, artist and album: the tracks of the closest length first, each
// with its synced lyrics before its plain ones. It lists none while the
// settings turn lyrics off, for a track without a title, while LRCLIB is
// left alone after a failure or asked maxAsking requests already, or when
// LRCLIB fails: a failure leaves it alone for a while, as look-ups do.
func (s *Service) Search(ctx context.Context, track Track) []Result {
	if !s.on() || strings.TrimSpace(track.Title) == "" || !s.begin() {
		return nil
	}
	query := url.Values{"track_name": {track.Title}}
	if strings.TrimSpace(track.Artist) != "" {
		query.Set("artist_name", track.Artist)
	}
	if strings.TrimSpace(track.Album) != "" {
		query.Set("album_name", track.Album)
	}
	asking, cancel := context.WithTimeout(ctx, askTimeout)
	body, err := s.get(asking, "/api/search?"+query.Encode(), maxSearchAnswer)
	cancel()
	var results []Result
	if err == nil {
		results, err = parseSearch(body, track.Duration)
	}
	s.end(ctx, err)
	if err != nil {
		return nil
	}
	return results
}

// parseSearch reads LRCLIB's search answer into results, ranked by how
// close the length of their track is to length (unranked when 0), at most
// maxResults tracks. Tracks without an identifier or lyrics are left out.
func parseSearch(body []byte, length time.Duration) ([]Result, error) {
	var tracks []listed
	if err := json.Unmarshal(body, &tracks); err != nil {
		return nil, fmt.Errorf("read the answer: %w", err)
	}
	tracks = slices.DeleteFunc(tracks, func(l listed) bool { return l.ID <= 0 })
	if length > 0 {
		distance := func(l listed) time.Duration {
			if l.length() == 0 {
				return math.MaxInt64
			}
			return (l.length() - length).Abs()
		}
		slices.SortStableFunc(tracks, func(a, b listed) int { return cmp.Compare(distance(a), distance(b)) })
	}
	var results []Result
	for _, track := range tracks[:min(len(tracks), maxResults)] {
		result := Result{Artist: track.ArtistName, Album: track.AlbumName, Title: track.TrackName, Duration: track.length()}
		if lyrics, ok := track.lyrics(true); ok {
			result.ID, result.Lyrics = resultID(track.ID, true), lyrics
			results = append(results, result)
		}
		if lyrics, ok := track.lyrics(false); ok {
			result.ID, result.Lyrics = resultID(track.ID, false), lyrics
			results = append(results, result)
		}
	}
	return results, nil
}

// kept is what is kept of a track's lyrics, synced or plain, and whether
// it has them.
func (l listed) kept(synced bool) (answer, bool) {
	if synced {
		return answer{kind: kindSynced, lyrics: l.SyncedLyrics}, len(parseLRC(l.SyncedLyrics)) > 0
	}
	return answer{kind: kindPlain, lyrics: l.PlainLyrics}, strings.TrimSpace(l.PlainLyrics) != ""
}

// lyrics are a track's lyrics, synced or plain, and whether it has them.
func (l listed) lyrics(synced bool) (Lyrics, bool) {
	kept, ok := l.kept(synced)
	if !ok {
		return Lyrics{}, false
	}
	return kept.parse()
}

// resultID identifies the synced or plain lyrics of LRCLIB's track id, as
// Jellyfin's LRCLIB provider does, after providerID.
func resultID(id int64, synced bool) string {
	kind := "plain"
	if synced {
		kind = "synced"
	}
	return providerID + "_" + strconv.FormatInt(id, 10) + "_" + kind
}

// parseResultID reads the LRCLIB track and the kind of lyrics a resultID
// names.
func parseResultID(raw string) (id int64, synced bool, ok bool) {
	if len(raw) > maxResultID {
		return 0, false, false
	}
	prefix, rest, ok := strings.Cut(raw, "_")
	if !ok || prefix != providerID {
		return 0, false, false
	}
	number, kind, ok := strings.Cut(rest, "_")
	if !ok || kind != "synced" && kind != "plain" {
		return 0, false, false
	}
	id, err := strconv.ParseInt(number, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != number {
		return 0, false, false
	}
	return id, kind == "synced", true
}

// Choose makes the lyrics of the search result id the track's: they are
// asked for to LRCLIB, as a search's are, and kept as LRCLIB's answers
// are, marked as chosen so that no look-up replaces them. It returns
// them, or ErrNotFound while the settings turn lyrics off, for an
// identifier that is none of LRCLIB's results, when LRCLIB no longer has
// the lyrics, or when it cannot be asked for them, as a search cannot.
func (s *Service) Choose(ctx context.Context, track accounts.ID, id string) (Lyrics, error) {
	number, synced, ok := parseResultID(id)
	if !s.on() || !ok || !s.begin() {
		return Lyrics{}, ErrNotFound
	}
	asking, cancel := context.WithTimeout(ctx, askTimeout)
	body, err := s.get(asking, "/api/get/"+strconv.FormatInt(number, 10), maxAnswer)
	cancel()
	var found listed
	if err == nil {
		if err = json.Unmarshal(body, &found); err != nil {
			err = fmt.Errorf("read the answer: %w", err)
		}
	}
	s.end(ctx, err)
	if err != nil || found.ID != number {
		return Lyrics{}, ErrNotFound
	}
	kept, ok := found.kept(synced)
	if !ok {
		return Lyrics{}, ErrNotFound
	}
	lyrics, _ := kept.parse()
	_, err = s.db.Exec(ctx, `INSERT INTO track_lyrics (item_id, kind, lyrics, looked_up_at, chosen) VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (item_id) DO UPDATE SET kind = excluded.kind, lyrics = excluded.lyrics, looked_up_at = excluded.looked_up_at, chosen = true`,
		track, kept.kind, kept.lyrics, s.now())
	if err != nil {
		return Lyrics{}, fmt.Errorf("save the lyrics chosen: %w", err)
	}
	s.logger.Info("Lyrics were chosen for a track", "track", track, "synced", synced)
	return lyrics, nil
}

// Chosen returns the lyrics an administrator chose for a track, while the
// settings turn lyrics on. It never asks LRCLIB.
func (s *Service) Chosen(ctx context.Context, track accounts.ID) (Lyrics, bool) {
	if !s.on() {
		return Lyrics{}, false
	}
	kept, ok := s.load(ctx, track)
	if !ok || !kept.chosen {
		return Lyrics{}, false
	}
	return kept.parse()
}

// Remove forgets the lyrics kept for a track, chosen or found, and keeps
// their removal as an administrator's choice: no look-up gives the track
// lyrics again, until lyrics are chosen for it (see Choose). Its own
// lyrics, those of a local music folder, are not Polyfin's to remove. It
// never asks LRCLIB, and removes whether lyrics are turned on or not.
func (s *Service) Remove(ctx context.Context, track accounts.ID) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `INSERT INTO track_lyrics (item_id, kind, lyrics, looked_up_at, chosen) VALUES ($1, $2, '', $3, true)
		ON CONFLICT (item_id) DO UPDATE SET kind = excluded.kind, lyrics = '', looked_up_at = excluded.looked_up_at, chosen = true`,
		track, kindRemoved, s.now())
	if err != nil {
		return fmt.Errorf("remove a track's lyrics: %w", err)
	}
	s.logger.Info("Lyrics were removed from a track", "track", track)
	return nil
}

// begin counts a search or a choice among the requests to LRCLIB under
// way, unless LRCLIB is left alone after a failure or asked maxAsking
// requests already.
func (s *Service) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.now().Before(s.pausedUntil) || len(s.asking)+s.others >= maxAsking {
		return false
	}
	s.others++
	return true
}

// end counts a search or a choice done. One that failed, unless LRCLIB
// did not know what it was asked or the app stopped waiting (ctx is
// done), leaves LRCLIB alone for a while.
func (s *Service) end(ctx context.Context, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.others--
	if err != nil && !errors.Is(err, errUnknown) && ctx.Err() == nil {
		s.pause(err)
	}
}
