package jellyfinimport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The ways reading a Jellyfin server fails.
var (
	// ErrInvalidAddress reports an address that is not an http or https
	// URL, or that holds a user name or password.
	ErrInvalidAddress = errors.New("invalid Jellyfin address")
	// ErrKeyRefused reports an API key the server refused.
	ErrKeyRefused = errors.New("the Jellyfin server refused the API key")
	// ErrForbidden reports a key the server knows but does not let read
	// what was asked: its users, or another user's data.
	ErrForbidden = errors.New("the Jellyfin server does not let the key read this")
	// ErrUnreachable reports a server that did not answer, or kept failing.
	ErrUnreachable = errors.New("Jellyfin server unreachable")
	// ErrNotJellyfin reports an address that answered, but not as a
	// Jellyfin server does.
	ErrNotJellyfin = errors.New("not a Jellyfin server")
)

// maxAddress bounds the length of an address.
const maxAddress = 2048

// maxReply bounds the bytes read of an answer: a page of items without
// their images takes well under a megabyte.
const maxReply = 32 << 20

// ParseAddress reads the address of a Jellyfin server as an administrator
// types it: an http or https URL, possibly with the path Jellyfin is served
// under, or a bare host and port, which means http. It returns it without
// a trailing slash. Local network addresses are allowed: the server to
// move from usually runs next to Polyfin.
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

// client reads one Jellyfin server with an API key, one request at a time,
// spaced by the service's gap.
type client struct {
	s            *Service
	address, key string
	// retries is how many times more a request that failed is tried: none
	// while an administrator waits on the answer, a few during an import.
	retries int
	last    time.Time
}

// get reads path, with query, into into. It waits as long as the server
// asks when it answers too many requests, and tries again after failures.
func (c *client) get(ctx context.Context, path string, query url.Values, into any) error {
	failures, waits := 0, 0
	for {
		if !sleep(ctx, time.Until(c.last.Add(c.s.timing.gap))) {
			return ctx.Err()
		}
		status, header, body, err := c.do(ctx, path, query)
		c.last = time.Now()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var wait time.Duration
		switch {
		case err == nil && status == http.StatusOK:
			if json.Unmarshal(body, into) != nil {
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
			// Jellyfin answers every route an import reads.
			return ErrNotJellyfin
		}
		if !sleep(ctx, wait) {
			return ctx.Err()
		}
	}
}

// do sends one request. The API key goes in the header Jellyfin reads API
// keys from, never in the URL, which errors would hold.
func (c *client) do(ctx context.Context, path string, query url.Values) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.s.timing.request)
	defer cancel()
	target := c.address + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	request.Header.Set("Authorization", `MediaBrowser Token="`+c.key+`"`)
	request.Header.Set("User-Agent", "Polyfin/"+c.s.version)
	request.Header.Set("Accept", "application/json")
	response, err := c.s.client.Do(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxReply))
	if err != nil {
		return 0, nil, nil, err
	}
	return response.StatusCode, response.Header, body, nil
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

// Server is the Jellyfin server an address leads to.
type Server struct {
	Name, Version string
	// Address is the server's address as ParseAddress gives it.
	Address string
	// KeyOwner is the Jellyfin user the key belongs to when it is a user's
	// own key or access token, empty for an API key of the server's
	// dashboard. A user's key reads that user's watch data only: some
	// servers answer it with that user's data whatever user is asked.
	KeyOwner string
}

// User is a user of a Jellyfin server.
type User struct {
	ID, Name                        string
	Administrator, Disabled, Hidden bool
	// LastActivity is when the user last used the server, nil when never.
	LastActivity *time.Time
}

// serverInfoJSON is what Jellyfin tells of itself without a key.
type serverInfoJSON struct {
	ID         string `json:"Id"`
	ServerName string
	Version    string
}

type userJSON struct {
	ID               string `json:"Id"`
	Name             string
	LastActivityDate string
	Policy           struct {
		IsAdministrator, IsDisabled, IsHidden bool
	}
}

// server reads which server the address leads to, then its users, which
// only an API key reads, and whose key it is.
func (c *client) server(ctx context.Context) (Server, []User, error) {
	var info serverInfoJSON
	err := c.get(ctx, "/System/Info/Public", nil, &info)
	if errors.Is(err, ErrKeyRefused) || errors.Is(err, ErrForbidden) {
		// Jellyfin answers it to anyone.
		err = ErrNotJellyfin
	}
	if err != nil {
		return Server{}, nil, err
	}
	if info.ID == "" {
		return Server{}, nil, ErrNotJellyfin
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
	owner, err := c.keyOwner(ctx)
	if err != nil {
		return Server{}, nil, err
	}
	return Server{Name: info.ServerName, Version: info.Version, Address: c.address, KeyOwner: owner}, users, nil
}

// keyOwner reads whose key c holds: /Users/Me answers a user's own key or
// access token with that user, and an API key of the server's dashboard,
// which belongs to no user, with an error (400 on Jellyfin). It returns the
// user's identifier, empty for a server's key.
func (c *client) keyOwner(ctx context.Context) (string, error) {
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
// select, handing each to found.
func (c *client) items(ctx context.Context, user, types, filter string, found func(itemJSON)) error {
	path := "/Users/" + url.PathEscape(user) + "/Items"
	for start := 0; ; {
		query := url.Values{
			"Recursive":              {"true"},
			"IncludeItemTypes":       {types},
			"Filters":                {filter},
			"Fields":                 {"ProviderIds"},
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
