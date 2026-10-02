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

// authenticated serves next only for a valid access token of an enabled
// user. Like Jellyfin, a refused request gets an empty 401.
func (h *Handler) authenticated(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := readCredentials(r, h.Accounts.Settings().LegacyAuthorization)
		if c.Token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		device, user, err := h.Accounts.DeviceByToken(r.Context(), c.Token, remoteAddress(r))
		if errors.Is(err, accounts.ErrNotFound) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), callerKey{}, caller{User: user, Device: device, Token: c.Token})
		next(w, r.WithContext(ctx))
	})
}

func remoteAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
