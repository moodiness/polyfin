// Package iptv imports IPTV channel lists, an M3U playlist or an Xtream
// Codes account, as sources that answer like an installed Stremio addon
// with one live TV catalog: the library lists their channels and plays
// their streams through the same paths as an addon's.
package iptv

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

var (
	// ErrInvalidName reports a source name that is not 1 to 64 printable
	// characters.
	ErrInvalidName = errors.New("source names are 1 to 64 printable characters")
	// ErrInvalidAddress reports a playlist or server address that is not an
	// http or https URL, or an Xtream account without a username or
	// password.
	ErrInvalidAddress = errors.New("invalid source address")
	// ErrInvalidList reports a download that is not a channel list.
	ErrInvalidList = errors.New("not an IPTV channel list")
	// ErrTooLarge reports a list over the size or channel limit.
	ErrTooLarge = errors.New("the channel list is too large")
	// ErrLoginRefused reports an Xtream account the server does not accept.
	ErrLoginRefused = errors.New("the IPTV server refused the account")
)

// catalogID is the identifier of a source's one live TV catalog.
const catalogID = "channels"

// Account is where a source's channels come from: a playlist address for
// an M3U source; a server address, a username and a password for an
// Xtream Codes account.
type Account struct {
	Kind     string
	URL      string
	Server   string
	Username string
	Password string
}

// maxAddress bounds an address, as the guide addresses are.
const maxAddress = 4096

// address checks an account and returns the address it is stored under,
// which embeds its credentials: the playlist address, or the Xtream
// server's player API address with the account.
func (a Account) address() (string, error) {
	switch a.Kind {
	case addons.KindM3U:
		address := strings.TrimSpace(a.URL)
		if !webAddress(address) {
			return "", ErrInvalidAddress
		}
		return address, nil
	case addons.KindXtream:
		server := strings.TrimRight(strings.TrimSpace(a.Server), "/")
		username, password := strings.TrimSpace(a.Username), strings.TrimSpace(a.Password)
		if !webAddress(server) || username == "" || password == "" {
			return "", ErrInvalidAddress
		}
		parsed, _ := url.Parse(server)
		if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return "", ErrInvalidAddress
		}
		address := server + "/player_api.php?" + url.Values{"username": {username}, "password": {password}}.Encode()
		if len(address) > maxAddress {
			return "", ErrInvalidAddress
		}
		return address, nil
	default:
		return "", ErrInvalidAddress
	}
}

func webAddress(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && len(address) <= maxAddress && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// accountOf reads back the account a source's address stores.
func accountOf(kind, address string) Account {
	if kind != addons.KindXtream {
		return Account{Kind: kind, URL: address}
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return Account{Kind: kind}
	}
	query := parsed.Query()
	parsed.RawQuery = ""
	return Account{Kind: kind, Server: strings.TrimSuffix(parsed.String(), "/player_api.php"),
		Username: query.Get("username"), Password: query.Get("password")}
}

// ProviderGuide returns the XMLTV guide an Xtream Codes server publishes
// for an account.
func ProviderGuide(a Account) string {
	return strings.TrimRight(strings.TrimSpace(a.Server), "/") + "/xmltv.php?" +
		url.Values{"username": {strings.TrimSpace(a.Username)}, "password": {strings.TrimSpace(a.Password)}}.Encode()
}

// streamURL is an Xtream channel's stream address: MPEG-TS, or HLS when
// the account only allows HLS.
func (a Account) streamURL(streamID string, hls bool) string {
	extension := ".ts"
	if hls {
		extension = ".m3u8"
	}
	return a.Server + "/live/" + url.PathEscape(a.Username) + "/" + url.PathEscape(a.Password) + "/" + url.PathEscape(streamID) + extension
}

// Redact shows a source's address without what may be a credential: its
// scheme and host only.
func Redact(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return "…"
	}
	return parsed.Scheme + "://" + parsed.Host + "/…"
}

// validName checks and trims a source name.
func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

// IDPrefix starts the Stremio identifiers of every IPTV source's channels,
// and prefix those of one source's.
const IDPrefix = "polyfin-iptv:"

func prefix(source accounts.ID) string { return IDPrefix + source.String() + ":" }

// manifest describes a source as an addon with one live TV catalog, whose
// channels' metas and streams it answers.
func manifest(source accounts.ID, kind, name string) stremio.Manifest {
	description := "M3U playlist"
	if kind == addons.KindXtream {
		description = "Xtream Codes account"
	}
	only := []string{prefix(source)}
	return stremio.Manifest{
		ID: "polyfin.iptv." + source.String(), Version: "1", Name: name, Description: description, Types: []string{"tv"},
		Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"tv"}, IDPrefixes: only},
			{Name: "stream", Types: []string{"tv"}, IDPrefixes: only}},
		Catalogs:   []stremio.Catalog{{Type: "tv", ID: catalogID, Name: name}},
		IDPrefixes: only,
	}
}
