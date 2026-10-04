package jellyfin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// credentials are what a request carries in its Authorization header (or
// legacy equivalents): the calling app and, once signed in, its token.
type credentials struct {
	Client   string
	Device   string
	DeviceID string
	Version  string
	Token    string
}

// complete reports whether the app identified itself fully, which Jellyfin
// requires before signing a device in.
func (c credentials) complete() bool {
	return c.Client != "" && c.Device != "" && c.DeviceID != "" && c.Version != ""
}

// readCredentials extracts credentials the way Jellyfin 12.1 does. The
// official forms are the "MediaBrowser" Authorization scheme and the ApiKey
// query parameter. Legacy forms (the "Emby" scheme, X-Emby-Authorization,
// X-Emby-Token, X-MediaBrowser-Token and api_key) count only when legacy is
// true.
func readCredentials(r *http.Request, legacy bool) credentials {
	var c credentials
	header := r.Header.Get("Authorization")
	scheme, params, _ := strings.Cut(strings.TrimSpace(header), " ")
	switch {
	case strings.EqualFold(scheme, "MediaBrowser"), legacy && strings.EqualFold(scheme, "Emby"):
		c = parseParameters(params)
	case legacy && r.Header.Get("X-Emby-Authorization") != "":
		scheme, params, _ = strings.Cut(strings.TrimSpace(r.Header.Get("X-Emby-Authorization")), " ")
		if strings.EqualFold(scheme, "MediaBrowser") || strings.EqualFold(scheme, "Emby") {
			c = parseParameters(params)
		}
	}
	if c.Token == "" {
		c.Token = query(r, "ApiKey")
	}
	if c.Token == "" && legacy {
		for _, value := range []string{
			r.Header.Get("X-Emby-Token"),
			r.Header.Get("X-MediaBrowser-Token"),
			query(r, "api_key"),
		} {
			if value != "" {
				c.Token = value
				break
			}
		}
	}
	return c
}

// parseParameters reads comma-separated key=value pairs whose keys match
// without regard to case and whose values may be quoted. Values are
// form-decoded, as apps escape spaces and punctuation in device names.
func parseParameters(input string) credentials {
	var c credentials
	for input != "" {
		input = strings.TrimLeft(input, " ,")
		key, rest, found := strings.Cut(input, "=")
		if !found {
			break
		}
		key = strings.TrimSpace(key)
		rest = strings.TrimLeft(rest, " ")
		var value string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				value, input = rest[1:], ""
			} else {
				value, input = rest[1:end+1], rest[end+2:]
			}
		} else {
			value, input, _ = strings.Cut(rest, ",")
			value = strings.TrimSpace(value)
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		switch strings.ToLower(key) {
		case "client":
			c.Client = value
		case "device":
			c.Device = value
		case "deviceid":
			c.DeviceID = value
		case "version":
			c.Version = value
		case "token":
			c.Token = value
		}
	}
	return c
}

// caller is the signed-in user and device of an authenticated request.
type caller struct {
	User   accounts.User
	Device accounts.Device
	Token  string
}

type callerKey struct{}

func callerFrom(ctx context.Context) caller {
	value, _ := ctx.Value(callerKey{}).(caller)
	return value
}

// signedInCaller is the user and device of the access token r carries. ok
// is false when r carries none, or none of an enabled user.
func (h *Handler) signedInCaller(r *http.Request) (c caller, ok bool, err error) {
	credentials := readCredentials(r, h.Accounts.Settings().LegacyAuthorization)
	if credentials.Token == "" {
		return caller{}, false, nil
	}
	device, user, err := h.Accounts.DeviceByToken(r.Context(), credentials.Token, remoteAddress(r))
	if errors.Is(err, accounts.ErrNotFound) {
		return caller{}, false, nil
	}
	if err != nil {
		return caller{}, false, err
	}
	return caller{User: user, Device: device, Token: credentials.Token}, true, nil
}

// authenticated serves next only for a valid access token of an enabled
// user, within their allowed hours. Like Jellyfin, a request without a
// valid token gets an empty 401, and one outside the hours an empty 403.
func (h *Handler) authenticated(next http.HandlerFunc) http.Handler {
	return h.authorized(next, true)
}

// authenticatedAnyHour is authenticated for the endpoints Jellyfin serves
// whatever the user's allowed hours (its IgnoreParentalControl policy).
func (h *Handler) authenticatedAnyHour(next http.HandlerFunc) http.Handler {
	return h.authorized(next, false)
}

func (h *Handler) authorized(next http.HandlerFunc, hours bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok, err := h.signedInCaller(r)
		switch {
		case err != nil:
			h.internalError(w, r, err)
		case !ok:
			w.WriteHeader(http.StatusUnauthorized)
		case hours && h.outsideHours(c.User):
			w.WriteHeader(http.StatusForbidden)
		default:
			next(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, c)))
		}
	})
}

// outsideHours reports whether a user's requests are refused for now
// being outside their allowed hours. Like Jellyfin, administrators are
// served at any hour once signed in; signing in is refused to everyone
// (see authenticateByName).
func (h *Handler) outsideHours(user accounts.User) bool {
	return !user.IsAdministrator && !user.AllowedAt(h.now())
}

func remoteAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
