// Package nuvio reads a Nuvio account, for an import into Polyfin (see
// package appimport).
//
// Nuvio keeps its accounts on a backend of its own, the official one or a
// self-hosted one, which serves its public configuration, its publishable
// key among it, at /.well-known/nuvio. Polyfin signs in there with the
// account's email and password, as Nuvio's apps do, asks once for the
// account's backup, a versioned document Nuvio's own scripts move accounts
// with, and signs out. The password and the session serve that only: they
// are never kept nor logged. The backup holds, for each of the account's
// profiles, the movies and episodes watched with their dates, the resume
// points, the library, and the addons.
package nuvio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/appimport"
)

// App names Nuvio among the apps imported from.
const App = "nuvio"

// DefaultBackend is the address of Nuvio's official backend.
const DefaultBackend = "https://api.nuvio.tv"

// The ways reading an account fails.
var (
	// ErrInvalidAddress reports a backend address that is not an http or
	// https address.
	ErrInvalidAddress = errors.New("not a Nuvio backend address")
	// ErrUnreachable reports a backend that does not answer, or fails.
	ErrUnreachable = errors.New("the Nuvio backend does not answer")
	// ErrNotNuvio reports an address where something other than a Nuvio
	// backend answers.
	ErrNotNuvio = errors.New("no Nuvio backend answers at this address")
	// ErrSignInRefused reports an email and password the backend refuses.
	ErrSignInRefused = errors.New("Nuvio refuses this email and password")
	// ErrRateLimited reports a backend asking to wait longer than a
	// sign-in waits.
	ErrRateLimited = errors.New("Nuvio asks to wait before trying again")
	// ErrUnsupportedBackup reports a backup of a format or version Polyfin
	// does not read (see BackupError).
	ErrUnsupportedBackup = errors.New("Nuvio answers a backup Polyfin does not read")
)

// The backup format Polyfin reads.
const (
	backupFormat  = "nuvio_account_backup"
	backupVersion = 1
)

// BackupError is a backup of a format or version Polyfin does not read.
type BackupError struct {
	Format  string
	Version int
}

func (e *BackupError) Error() string {
	return fmt.Sprintf("Nuvio answers a backup %q of version %d: Polyfin reads %q of version %d", e.Format, e.Version, backupFormat,
		backupVersion)
}

func (e *BackupError) Unwrap() error { return ErrUnsupportedBackup }

// maxBackup bounds the backup read: a profile's history is a few
// megabytes at most.
const maxBackup = 64 << 20

// maxReply bounds the other answers.
const maxReply = 1 << 20

// maxAddress bounds a backend address.
const maxAddress = 2048

// timing is how the reader paces its requests. Tests shorten it.
type timing struct {
	// request bounds each request; the backup takes the backend a while.
	request time.Duration
	// maxWait bounds how long a backend asking to wait is waited for, and
	// waits how many times.
	maxWait time.Duration
	waits   int
	// signOut bounds the wait for the session to end.
	signOut time.Duration
}

var defaultTiming = timing{request: time.Minute, maxWait: 30 * time.Second, waits: 2, signOut: 5 * time.Second}

// Options are the dependencies of the reader.
type Options struct {
	// Anime maps the Kitsu identifiers of anime to Polyfin's titles; nil
	// finds them as anime catalogs list them only.
	Anime *anime.Service
	// Version is Polyfin's, which the backend is told.
	Version string
}

// Reader reads Nuvio accounts.
type Reader struct {
	anime   *anime.Service
	version string
	client  *http.Client
	timing  timing
}

// New returns a reader with options.
func New(options Options) *Reader {
	return &Reader{anime: options.Anime, version: options.Version, client: &http.Client{}, timing: defaultTiming}
}

// ParseBackend reads the address of a Nuvio backend as a user types it:
// the official one when empty, https:// when it names none. A
// self-hosted backend may be on the local network.
func ParseBackend(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return DefaultBackend, nil
	}
	if len(text) > maxAddress {
		return "", ErrInvalidAddress
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", ErrInvalidAddress
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.EscapedPath(), "/"), nil
}

// Read signs in to the backend at address with email and password, reads
// the account's backup, signs out, and returns the account with each of
// its profiles. It fails with ErrInvalidAddress, ErrUnreachable,
// ErrNotNuvio, ErrSignInRefused, ErrRateLimited or a BackupError.
func (r *Reader) Read(ctx context.Context, address, email, password string) (appimport.Account, error) {
	backend, err := ParseBackend(address)
	if err != nil {
		return appimport.Account{}, err
	}
	s := &session{r: r, backend: backend}
	var configuration struct {
		PublishableKey string `json:"publishable_key"`
	}
	if err := s.send(ctx, http.MethodGet, "/.well-known/nuvio", nil, &configuration); err != nil {
		return appimport.Account{}, notFound(err)
	}
	if configuration.PublishableKey == "" {
		return appimport.Account{}, ErrNotNuvio
	}
	s.key = configuration.PublishableKey
	credentials, err := json.Marshal(struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{strings.TrimSpace(email), password})
	if err != nil {
		return appimport.Account{}, err
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	switch err := s.send(ctx, http.MethodPost, "/auth/v1/token?grant_type=password", credentials, &token); {
	case errors.Is(err, errRefused):
		return appimport.Account{}, ErrSignInRefused
	case err != nil:
		return appimport.Account{}, notFound(err)
	case token.AccessToken == "":
		return appimport.Account{}, ErrNotNuvio
	}
	s.token = token.AccessToken
	defer s.signOut()
	var b backup
	if err := s.send(ctx, http.MethodPost, "/rest/v1/rpc/sync_export_account_backup", []byte("{}"), &b); err != nil {
		if errors.Is(err, errRefused) {
			return appimport.Account{}, ErrSignInRefused
		}
		return appimport.Account{}, notFound(err)
	}
	if b.Format != backupFormat || b.Version.value != backupVersion {
		return appimport.Account{}, &BackupError{Format: b.Format, Version: int(b.Version.value)}
	}
	return r.account(b.Data), nil
}

// errRefused and errMissing are a request the backend answered refused,
// or not found: what that means depends on the request.
var (
	errRefused = errors.New("refused")
	errMissing = errors.New("not found")
)

// notFound reads errMissing as an address where no Nuvio backend answers.
func notFound(err error) error {
	if errors.Is(err, errMissing) || errors.Is(err, errRefused) {
		return ErrNotNuvio
	}
	return err
}

// session is a backend signed in to, or about to be.
type session struct {
	r       *Reader
	backend string
	// key is the backend's publishable key, token the session's.
	key, token string
}

// send sends a request, a GET or a POST of body as JSON, and reads the
// answer into into. It waits as long as the backend asks when it answers
// too many requests or that it is unavailable, a few times.
func (s *session) send(ctx context.Context, method, path string, body []byte, into any) error {
	waits := 0
	for {
		status, header, answer, err := s.do(ctx, method, path, body)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch {
		case err != nil:
			return ErrUnreachable
		case status == http.StatusOK:
			if json.Unmarshal(answer, into) != nil {
				return ErrNotNuvio
			}
			return nil
		case status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable:
			wait := retryAfter(header)
			if status == http.StatusServiceUnavailable && wait == 0 {
				return ErrUnreachable
			}
			wait = max(wait, time.Second)
			if waits++; waits > s.r.timing.waits || wait > s.r.timing.maxWait {
				return ErrRateLimited
			}
			if !sleep(ctx, wait) {
				return ctx.Err()
			}
		case status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden ||
			status == http.StatusUnprocessableEntity:
			return errRefused
		case status == http.StatusNotFound:
			return errMissing
		default:
			return ErrUnreachable
		}
	}
}

// do sends one request, with the publishable key and the session's token
// once known, in headers: never in the URL, which errors would hold.
func (s *session) do(ctx context.Context, method, path string, body []byte) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.r.timing.request)
	defer cancel()
	var content io.Reader
	if body != nil {
		content = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.backend+path, content)
	if err != nil {
		return 0, nil, nil, err
	}
	request.Header.Set("User-Agent", "Polyfin/"+s.r.version)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if s.key != "" {
		request.Header.Set("apikey", s.key)
	}
	if s.token != "" {
		request.Header.Set("Authorization", "Bearer "+s.token)
	}
	response, err := s.r.client.Do(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()
	limit := int64(maxReply)
	if strings.HasPrefix(path, "/rest/") {
		limit = maxBackup
	}
	answer, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return 0, nil, nil, err
	}
	return response.StatusCode, response.Header, answer, nil
}

// signOut ends the session, so that the account does not keep it.
func (s *session) signOut() {
	ctx, cancel := context.WithTimeout(context.Background(), s.r.timing.signOut)
	defer cancel()
	_, _, _, _ = s.do(ctx, http.MethodPost, "/auth/v1/logout", []byte("{}"))
	s.token = ""
}

// retryAfter reads a Retry-After header in seconds, zero when absent.
func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After")))
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// sleep waits for d, and reports false if ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// number is a number of the backup, which reads null, and anything that is
// not a number, as absent.
type number struct {
	value int64
	set   bool
}

func (n *number) UnmarshalJSON(data []byte) error {
	*n = number{}
	var f float64
	if !bytes.Equal(bytes.TrimSpace(data), []byte("null")) && json.Unmarshal(data, &f) == nil {
		*n = number{value: int64(f), set: true}
	}
	return nil
}

// backup is an account's backup, as the backend answers it.
type backup struct {
	Format  string `json:"format"`
	Version number `json:"version"`
	Data    data   `json:"data"`
}

// data is what a backup holds of each profile. A row names its profile by
// the profile's index.
type data struct {
	Profiles []struct {
		Index             number `json:"profile_index"`
		Name              string `json:"name"`
		UsesPrimaryAddons bool   `json:"uses_primary_addons"`
	} `json:"profiles"`
	Addons []struct {
		Profile number `json:"profile_id"`
		URL     string `json:"url"`
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
	} `json:"addons"`
	Library []struct {
		Profile     number `json:"profile_id"`
		ContentID   string `json:"content_id"`
		ContentType string `json:"content_type"`
		Name        string `json:"name"`
	} `json:"library_items"`
	Progress []struct {
		Profile     number `json:"profile_id"`
		ContentID   string `json:"content_id"`
		ContentType string `json:"content_type"`
		VideoID     string `json:"video_id"`
		Season      number `json:"season"`
		Episode     number `json:"episode"`
		Position    number `json:"position"`
		Duration    number `json:"duration"`
		LastWatched number `json:"last_watched"`
	} `json:"watch_progress"`
	Watched []struct {
		Profile     number `json:"profile_id"`
		ContentID   string `json:"content_id"`
		ContentType string `json:"content_type"`
		Title       string `json:"title"`
		Season      number `json:"season"`
		Episode     number `json:"episode"`
		WatchedAt   number `json:"watched_at"`
	} `json:"watched_items"`
}

// serverPrefix begins the identifiers of the titles of the Jellyfin and
// Emby servers Nuvio connects to, which are not addons' titles: their
// progress never leaves the device.
const serverPrefix = "srv1:"

// key names a title of a profile: a movie or series, or an episode of it,
// whose season and episode are -1 for the title itself.
type key struct {
	profile         int64
	content         string
	season, episode int64
}

// account reads the profiles of the backup, and what each holds.
func (r *Reader) account(d data) appimport.Account {
	account := appimport.Account{App: App}
	profiles := map[int64]int{}
	primary := map[int64]bool{}
	for _, p := range d.Profiles {
		if !p.Index.set {
			continue
		}
		profiles[p.Index.value] = len(account.Profiles)
		primary[p.Index.value] = p.UsesPrimaryAddons && p.Index.value > 1
		account.Profiles = append(account.Profiles, appimport.Profile{ID: strconv.FormatInt(p.Index.value, 10), Name: strings.TrimSpace(p.Name)})
	}
	// Every account has a first profile, which a backup lists unless it
	// was never named.
	if _, listed := profiles[1]; !listed {
		profiles[1] = len(account.Profiles)
		account.Profiles = append(account.Profiles, appimport.Profile{ID: "1"})
	}
	entries := map[key]*appimport.Entry{}
	var order []key
	entry := func(k key, kind, name string) *appimport.Entry {
		e := entries[k]
		if e == nil {
			e = &appimport.Entry{Label: appimport.Label{ID: k.content, Kind: kind}}
			if k.episode >= 0 {
				e.Label.Season, e.Label.Episode = int(k.season), int(k.episode)
			}
			e.Title, _ = appimport.TitleRef(r.anime, k.content, kind == "movie", int(max(k.season, 0)), int(max(k.episode, 0)))
			entries[k] = e
			order = append(order, k)
		}
		if e.Label.Name == "" {
			e.Label.Name = strings.TrimSpace(name)
		}
		return e
	}
	for _, w := range d.Watched {
		k, kind, ok := place(w.Profile, w.ContentID, w.ContentType, w.Season, w.Episode, "")
		// A series marked watched whole: Polyfin counts a series played
		// from its episodes.
		if !ok || kind == "series" {
			continue
		}
		e := entry(k, kind, w.Title)
		e.Played = true
		if at := watchedAt(w.WatchedAt.value); w.WatchedAt.set && at > 0 && (e.PlayedAt == nil || time.UnixMilli(at).After(*e.PlayedAt)) {
			e.PlayedAt = new(time.UnixMilli(at).UTC())
		}
	}
	for _, p := range d.Progress {
		k, kind, ok := place(p.Profile, p.ContentID, p.ContentType, p.Season, p.Episode, p.VideoID)
		if !ok || kind == "series" || strings.HasPrefix(p.ContentID, serverPrefix) || p.Position.value <= 0 ||
			p.LastWatched.value <= 0 {
			continue
		}
		if watched := entries[k]; watched != nil && watched.Played {
			continue
		}
		at := time.UnixMilli(p.LastWatched.value).UTC()
		e := entry(k, kind, "")
		if e.Resume == nil || at.After(e.Resume.At) {
			e.Resume = &appimport.Resume{Position: time.Duration(p.Position.value) * time.Millisecond,
				Runtime: time.Duration(max(p.Duration.value, 0)) * time.Millisecond, At: at}
		}
	}
	for _, l := range d.Library {
		k, kind, ok := place(l.Profile, l.ContentID, l.ContentType, number{}, number{}, "")
		if !ok {
			continue
		}
		entry(k, kind, l.Name).Favorite = true
	}
	for _, k := range order {
		if i, known := profiles[k.profile]; known {
			account.Profiles[i].Entries = append(account.Profiles[i].Entries, *entries[k])
		}
	}

	addons := map[int64][]appimport.Addon{}
	for _, a := range d.Addons {
		manifest, ok := manifestURL(a.URL)
		if !ok || a.Enabled != nil && !*a.Enabled || !a.Profile.set {
			continue
		}
		name := strings.TrimSpace(a.Name)
		if name == "" {
			if parsed, err := url.Parse(manifest); err == nil {
				name = parsed.Host
			}
		}
		if !slices.ContainsFunc(addons[a.Profile.value], func(o appimport.Addon) bool { return o.ManifestURL == manifest }) {
			addons[a.Profile.value] = append(addons[a.Profile.value], appimport.Addon{Name: name, ManifestURL: manifest})
		}
	}
	for index, i := range profiles {
		from := index
		// A profile using the first profile's addons has none of its own.
		if primary[index] {
			from = 1
		}
		account.Profiles[i].Addons = addons[from]
	}
	return account
}

// place names the title a row is of: an episode when it has a season and
// an episode, which the identifier of its video may give
// ("<content>:<season>:<episode>"), else a movie or series by its type. It
// reports false for a row without a profile or a title.
func place(profile number, content, contentType string, season, episode number, video string) (key, string, bool) {
	content = strings.TrimSpace(content)
	if !profile.set || content == "" {
		return key{}, "", false
	}
	if (!season.set || !episode.set) && video != "" {
		if rest, ok := strings.CutPrefix(video, content+":"); ok {
			if s, e, ok := strings.Cut(rest, ":"); ok {
				sn, errS := strconv.ParseInt(s, 10, 64)
				en, errE := strconv.ParseInt(e, 10, 64)
				if errS == nil && errE == nil {
					season, episode = number{value: sn, set: true}, number{value: en, set: true}
				}
			}
		}
	}
	if season.set && episode.set && season.value >= 0 && episode.value > 0 {
		return key{profile: profile.value, content: content, season: season.value, episode: episode.value}, "episode", true
	}
	kind := "series"
	if strings.EqualFold(strings.TrimSpace(contentType), "movie") {
		kind = "movie"
	}
	return key{profile: profile.value, content: content, season: -1, episode: -1}, kind, true
}

// The compact dates older Nuvio apps wrote of a watched title, as a number
// yyyyMMddHHmmss in UTC, between these: as epoch milliseconds they would
// be centuries ahead.
const (
	compactMin = 19000101000000
	compactMax = 29991231235959
)

// watchedAt reads the date a title was marked watched, in epoch
// milliseconds: as Nuvio stores it, or in the compact form older apps
// wrote.
func watchedAt(value int64) int64 {
	if value < compactMin || value > compactMax {
		return value
	}
	digits := fmt.Sprintf("%014d", value)
	at, err := time.Parse("20060102150405", digits)
	if err != nil {
		return value
	}
	return at.UnixMilli()
}

// manifestURL is the address of the manifest of the addon a profile lists:
// Nuvio lists some by the address their resources are under.
func manifestURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	if !strings.HasSuffix(parsed.Path, "/manifest.json") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/manifest.json"
		parsed.RawPath = ""
	}
	return parsed.String(), true
}
