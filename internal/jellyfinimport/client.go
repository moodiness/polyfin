package jellyfinimport

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The ways reading a server fails.
var (
	// ErrInvalidAddress reports an address that is not an http or https
	// URL, or that holds a user name or password.
	ErrInvalidAddress = errors.New("invalid server address")
	// ErrKeyRefused reports an API key or token the server refused.
	ErrKeyRefused = errors.New("the server refused the API key")
	// ErrForbidden reports a key the server knows but does not let read
	// what was asked: its users, or another user's data.
	ErrForbidden = errors.New("the server does not let the key read this")
	// ErrUnreachable reports a server that did not answer, or kept failing.
	ErrUnreachable = errors.New("server unreachable")
	// ErrNotJellyfin reports an address that answered, but not as the kind
	// of server asked does.
	ErrNotJellyfin = errors.New("not a server of the kind asked")
	// ErrSignInRefused reports a user name or password the server refused.
	ErrSignInRefused = errors.New("the server refused the name or password")
	// ErrSignInForbidden reports an account the server does not let sign
	// in: disabled, or outside its allowed hours.
	ErrSignInForbidden = errors.New("the server does not let the account sign in")
	// ErrOtherUser reports a sign-in as a user that opened another user's
	// session.
	ErrOtherUser = errors.New("the server signed in another user")
	// ErrUserKey reports an Emby key that is not one of the server's API
	// keys: a user's, which Emby does not tell the owner of.
	ErrUserKey = errors.New("the Emby key is not one of the server's API keys")
)

// maxAddress bounds the length of an address.
const maxAddress = 2048

// maxReply bounds the bytes read of an answer: a page of items without
// their images takes well under a megabyte.
const maxReply = 32 << 20

// embyPath is the path Emby serves its API under, as well as at its root.
const embyPath = "/emby"

// ParseAddress reads the address of a server as an administrator types it:
// an http or https URL, possibly with the path the server is served under,
// or a bare host and port, which means http. It returns it without a
// trailing slash. Local network addresses are allowed: the server to move
// from usually runs next to Polyfin.
func ParseAddress(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > maxAddress {
		return "", ErrInvalidAddress
	}
	if !strings.Contains(text, "://") {
		text = "http://" + text
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", ErrInvalidAddress
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.EscapedPath(), "/"), nil
}

// client reads one server with a key, one request at a time, spaced by the
// service's gap: an API key of the server's dashboard, a user's own key,
// the token of a session the client signed in, or Plex's owner's token.
type client struct {
	s            *Service
	kind         Kind
	address, key string
	// session tells that key is the token of a session the client signed
	// in, which signOut ends.
	session bool
	// retries is how many times more a request that failed is tried: none
	// while an administrator waits on the answer, a few during an import.
	retries int
	last    time.Time
	// plex is what was read of a Plex server's libraries, once.
	plex *plexLibrary
}

// get reads path, with query, into into.
func (c *client) get(ctx context.Context, path string, query url.Values, into any) error {
	return c.send(ctx, http.MethodGet, path, query, nil, into)
}

// send sends a request, a GET or a POST of body as JSON, and reads the
// answer into into. It waits as long as the server asks when it answers
// too many requests, and tries again after failures.
func (c *client) send(ctx context.Context, method, path string, query url.Values, body []byte, into any) error {
	failures, waits := 0, 0
	for {
		if !sleep(ctx, time.Until(c.last.Add(c.s.timing.gap))) {
			return ctx.Err()
		}
		status, header, answer, err := c.do(ctx, method, path, query, body)
		c.last = time.Now()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var wait time.Duration
		switch {
		case err == nil && status == http.StatusOK:
			if json.Unmarshal(answer, into) != nil {
				return ErrNotJellyfin
			}
			return nil
		case err == nil && status == http.StatusUnauthorized:
			return ErrKeyRefused
		case err == nil && status == http.StatusForbidden:
			return ErrForbidden
		case err == nil && status == http.StatusTooManyRequests:
			wait = max(retryAfter(header), time.Second)
			if waits++; waits > c.retries || wait > c.s.timing.maxWait {
				return ErrUnreachable
			}
		case err != nil || status >= 500:
			if failures++; failures > c.retries {
				return ErrUnreachable
			}
			wait = c.s.timing.retryFirst << (failures - 1)
		default:
			// The server answers every route an import reads.
			return ErrNotJellyfin
		}
		if !sleep(ctx, wait) {
			return ctx.Err()
		}
	}
}

// do sends one request, a POST of body as JSON when there is one. The key
// goes in the header the server reads keys from, never in the URL, which
// errors would hold: Authorization for Jellyfin, X-Emby-Token for Emby, and
// X-Plex-Token for Plex, which is also told Polyfin's device. Without a
// key, the header names Polyfin as the app and device signing in, as
// Jellyfin's and Emby's apps do: a server missing the device may sign the
// user out of every other device.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body []byte) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.s.timing.request)
	defer cancel()
	target := c.address + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var content io.Reader
	if body != nil {
		content = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, content)
	if err != nil {
		return 0, nil, nil, err
	}
	signIn := `MediaBrowser Client="Polyfin", Device="Polyfin", DeviceId="` + newID() + `", Version="` + c.s.version + `"`
	switch {
	case c.kind == Plex:
		if c.key != "" {
			request.Header.Set("X-Plex-Token", c.key)
		}
		request.Header.Set("X-Plex-Client-Identifier", c.s.device)
		request.Header.Set("X-Plex-Product", "Polyfin")
		request.Header.Set("X-Plex-Version", c.s.version)
	case c.kind == Emby && c.key != "":
		request.Header.Set("X-Emby-Token", c.key)
	case c.kind == Emby:
		request.Header.Set("X-Emby-Authorization", signIn)
	case c.key != "":
		request.Header.Set("Authorization", `MediaBrowser Token="`+c.key+`"`)
	default:
		request.Header.Set("Authorization", signIn)
	}
	request.Header.Set("User-Agent", "Polyfin/"+c.s.version)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.s.client.Do(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxReply))
	if err != nil {
		return 0, nil, nil, err
	}
	return response.StatusCode, response.Header, answer, nil
}

// signIn signs in as the user named name with password, as Jellyfin's apps
// do, and reads with the session's token from then on. It returns the
// identifier of the user signed in.
func (c *client) signIn(ctx context.Context, name, password string) (string, error) {
	body, err := json.Marshal(struct{ Username, Pw string }{name, password})
	if err != nil {
		return "", err
	}
	var signed struct {
		User struct {
			ID string `json:"Id"`
		}
		AccessToken string
	}
	err = c.send(ctx, http.MethodPost, "/Users/AuthenticateByName", nil, body, &signed)
	switch {
	case errors.Is(err, ErrKeyRefused):
		return "", ErrSignInRefused
	case errors.Is(err, ErrForbidden):
		return "", ErrSignInForbidden
	case err != nil:
		return "", err
	case signed.User.ID == "" || signed.AccessToken == "":
		return "", ErrNotJellyfin
	}
	c.key, c.session = signed.AccessToken, true
	return signed.User.ID, nil
}

// signOutWait bounds the wait for a session to end: the import does not
// depend on it.
const signOutWait = 5 * time.Second

// signOut ends the session signIn opened, if any, so that the server does
// not keep it among the user's devices.
func (c *client) signOut() {
	if !c.session {
		return
	}
	c.session = false
	ctx, cancel := context.WithTimeout(context.Background(), signOutWait)
	defer cancel()
	_, _, _, _ = c.do(ctx, http.MethodPost, "/Sessions/Logout", nil, nil)
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
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Server is the server an address leads to.
type Server struct {
	Kind          Kind
	Name, Version string
	// Address is the server's address as ParseAddress gives it, with the
	// path Emby's API answers under.
	Address string
	// KeyOwner is the user the import reads as: the owner of a user's own
	// key or access token, or the user signed in with a name and password;
	// empty for an API key of the server's dashboard, or Plex's owner's
	// token, which reads every user's played history. A user's key reads
	// that user's watch data only: some servers answer it with that user's
	// data whatever user is asked.
	KeyOwner string
}

// User is a user of a server.
type User struct {
	ID, Name                        string
	Administrator, Disabled, Hidden bool
	// LastActivity is when the user last used the server, nil when never
	// or when the server does not tell.
	LastActivity *time.Time
}

// serverInfoJSON is what Jellyfin and Emby tell of themselves without a
// key. Jellyfin names itself in ProductName, "Jellyfin Server"; Emby leaves
// it out.
type serverInfoJSON struct {
	ID          string `json:"Id"`
	ServerName  string
	Version     string
	ProductName string
}

// is tells whether the server that answered info is of kind, Jellyfin or
// Emby. Each answers much of the other's API, but not the same way: Emby
// has no /Users/Me, which tells whose a key is on Jellyfin, and reads keys
// from its own header.
func (info serverInfoJSON) is(kind Kind) bool {
	return strings.HasPrefix(info.ProductName, "Jellyfin") == (kind == Jellyfin)
}

type userJSON struct {
	ID               string `json:"Id"`
	Name             string
	LastActivityDate string
	Policy           struct {
		IsAdministrator, IsDisabled, IsHidden bool
	}
}

// connect reads which server the address leads to, signs in when
// credentials name a user, then reads the server's users, which only a
// server's key or an administrator lists, and whose key it reads with.
func (c *client) connect(ctx context.Context, credentials Credentials) (Server, []User, error) {
	if c.kind == Plex {
		return c.plexConnect(ctx, credentials.Key)
	}
	server, err := c.open(ctx, credentials)
	if err != nil {
		return Server{}, nil, err
	}
	var listed []userJSON
	if err := c.get(ctx, "/Users", nil, &listed); err != nil {
		return Server{}, nil, err
	}
	users := make([]User, 0, len(listed))
	for _, u := range listed {
		if u.ID == "" {
			continue
		}
		users = append(users, User{ID: u.ID, Name: u.Name, Administrator: u.Policy.IsAdministrator, Disabled: u.Policy.IsDisabled,
			Hidden: u.Policy.IsHidden, LastActivity: date(u.LastActivityDate)})
	}
	// A session is its user's whatever /Users/Me answers: a server that does
	// not answer it would pass the session for a server's key.
	if credentials.Name == "" {
		if server.KeyOwner, err = c.keyOwner(ctx); err != nil {
			return Server{}, nil, err
		}
	}
	return server, users, nil
}

// open reads which Jellyfin or Emby server the address leads to, then
// signs in when credentials name a user, or else reads with their key. It
// returns the server, whose KeyOwner is the user signed in, if any.
func (c *client) open(ctx context.Context, credentials Credentials) (Server, error) {
	info, err := c.locate(ctx)
	if err != nil {
		return Server{}, err
	}
	server := Server{Kind: c.kind, Name: info.ServerName, Version: info.Version, Address: c.address}
	// A password goes only to a server that answered as the kind asked
	// does.
	if credentials.Name == "" {
		c.key = credentials.Key
		return server, nil
	}
	if server.KeyOwner, err = c.signIn(ctx, credentials.Name, credentials.Password); err != nil {
		return Server{}, err
	}
	return server, nil
}

// locate reads, without a key, which server the address leads to, and
// fails with ErrNotJellyfin when it is not of c's kind: a key or password
// goes only to a server of the kind asked. Emby serves its API under /emby,
// and at its root too when reached directly: an Emby address without that
// path tries it first, so that a proxy passing only that path on works too.
// c reads at the address that answered from then on.
func (c *client) locate(ctx context.Context) (serverInfoJSON, error) {
	addresses := []string{c.address}
	if c.kind == Emby && !strings.HasSuffix(c.address, embyPath) {
		addresses = []string{c.address + embyPath, c.address}
	}
	var err error
	for _, address := range addresses {
		c.address = address
		var info serverInfoJSON
		err = c.get(ctx, "/System/Info/Public", nil, &info)
		if errors.Is(err, ErrKeyRefused) || errors.Is(err, ErrForbidden) || err == nil && (info.ID == "" || !info.is(c.kind)) {
			// Jellyfin and Emby answer it to anyone, and tell which they
			// are.
			err = ErrNotJellyfin
		}
		if !errors.Is(err, ErrNotJellyfin) {
			return info, err
		}
	}
	return serverInfoJSON{}, err
}

// keyOwner reads whose key c holds: /Users/Me answers a user's own key or
// access token with that user, and an API key of the server's dashboard,
// which belongs to no user, with an error (400 on Jellyfin). It returns the
// user's identifier, empty for a server's key. Emby has no /Users/Me: see
// embyKey.
func (c *client) keyOwner(ctx context.Context) (string, error) {
	if c.kind == Emby {
		return "", c.embyKey(ctx)
	}
	var me userJSON
	err := c.get(ctx, "/Users/Me", nil, &me)
	switch {
	case err == nil:
		return me.ID, nil
	case errors.Is(err, ErrNotJellyfin), errors.Is(err, ErrKeyRefused), errors.Is(err, ErrForbidden):
		// The key read the users: it is the server's, which no user is.
		return "", nil
	}
	return "", err
}

// embyKey checks that c's key is one of the Emby server's API keys, which
// /Auth/Keys lists, and fails with ErrUserKey otherwise. Emby has no way
// to tell whose a user's key is, and answers another user's items to some
// keys: a user's key, whose owner the import could not know, is not taken.
// The keys listed are only compared, never kept.
func (c *client) embyKey(ctx context.Context) error {
	var keys struct {
		Items []struct{ AccessToken string }
	}
	err := c.get(ctx, "/Auth/Keys", nil, &keys)
	if errors.Is(err, ErrUnreachable) || ctx.Err() != nil {
		return err
	}
	if err == nil {
		for _, key := range keys.Items {
			if subtle.ConstantTimeCompare([]byte(key.AccessToken), []byte(c.key)) == 1 {
				return nil
			}
		}
	}
	return ErrUserKey
}

// date reads a date as Jellyfin writes them, nil when there is none.
// Jellyfin writes a missing date as the zero year.
func date(text string) *time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, text); err == nil {
			if t.Year() <= 1 {
				return nil
			}
			return new(t.UTC())
		}
	}
	return nil
}

// itemJSON is a movie, series or episode with the user's data of it.
type itemJSON struct {
	ID             string `json:"Id"`
	Name           string
	Type           string
	ProductionYear int
	ProviderIDs    map[string]string `json:"ProviderIds"`
	// SeriesID names an episode's series, ParentIndexNumber its season,
	// IndexNumber its number, and IndexNumberEnd the last number of a file
	// holding several episodes.
	SeriesID          string `json:"SeriesId"`
	SeriesName        string
	ParentIndexNumber *int
	IndexNumber       *int
	IndexNumberEnd    *int
	RunTimeTicks      int64
	UserData          struct {
		Played                bool
		PlayCount             int
		PlaybackPositionTicks int64
		PlayedPercentage      float64
		IsFavorite            bool
		LastPlayedDate        string
	}
}

type pageJSON struct {
	Items            []itemJSON
	TotalRecordCount int
}

// items reads, page by page, the items of user that types and filter
// select, handing each to found. Emby leaves the play count and the date
// last played out of a list's user data unless its fields name them.
func (c *client) items(ctx context.Context, user, types, filter string, found func(itemJSON)) error {
	path := "/Users/" + url.PathEscape(user) + "/Items"
	fields := "ProviderIds"
	if c.kind == Emby {
		fields += ",UserDataPlayCount,UserDataLastPlayedDate"
	}
	for start := 0; ; {
		query := url.Values{
			"Recursive":              {"true"},
			"IncludeItemTypes":       {types},
			"Filters":                {filter},
			"Fields":                 {fields},
			"EnableUserData":         {"true"},
			"EnableImages":           {"false"},
			"EnableTotalRecordCount": {"true"},
			"SortBy":                 {"SortName"},
			"SortOrder":              {"Ascending"},
			"StartIndex":             {strconv.Itoa(start)},
			"Limit":                  {strconv.Itoa(c.s.timing.pageSize)},
		}
		var page pageJSON
		if err := c.get(ctx, path, query, &page); err != nil {
			return err
		}
		for _, item := range page.Items {
			found(item)
		}
		start += len(page.Items)
		if len(page.Items) == 0 || start >= page.TotalRecordCount {
			return nil
		}
	}
}

// providers reads the identifiers of the items ids name, as user sees
// them: the series of episodes, whose own identifiers are the episodes'.
func (c *client) providers(ctx context.Context, user string, ids []string) (map[string]map[string]string, error) {
	path := "/Users/" + url.PathEscape(user) + "/Items"
	result := make(map[string]map[string]string, len(ids))
	for start := 0; start < len(ids); start += idsPerRequest {
		batch := ids[start:min(start+idsPerRequest, len(ids))]
		query := url.Values{
			"Ids":            {strings.Join(batch, ",")},
			"Fields":         {"ProviderIds"},
			"EnableUserData": {"false"},
			"EnableImages":   {"false"},
		}
		var page pageJSON
		if err := c.get(ctx, path, query, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			result[item.ID] = item.ProviderIDs
		}
	}
	return result, nil
}

// idsPerRequest bounds the items a request names, which keeps its URL
// short.
const idsPerRequest = 50
