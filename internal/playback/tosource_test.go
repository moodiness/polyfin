package playback

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// An app is told where a version comes from only when its user owns the
// source: an administrator for the server's addons, a user for their own,
// and no one for an IPTV source, whose addresses hold its account. Send
// apps to the source tells every app, as before the setting existed.
func TestOnlyASourcesOwnerIsSentToIt(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileSource{data: make([]byte, 1000)}, "ffprobe", nil)
	store, err := accounts.Open(t.Context(), s.db)
	if err != nil {
		t.Fatal(err)
	}
	user := func(name string, administrator bool) accounts.User {
		t.Helper()
		created, err := store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsAdministrator: administrator})
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	administrator, member, other := user("administrator", true), user("member", false), user("other", false)
	position := 0
	source := func(kind string, owner *accounts.ID) library.Version {
		t.Helper()
		position++
		var id accounts.ID
		if err := s.db.QueryRow(t.Context(), `INSERT INTO addons (owner_id, kind, manifest_url, manifest, position)
			VALUES ($1, $2, $3, '{}', $4) RETURNING id`, owner, kind, "https://93.184.216.34/"+kind+"/"+string(rune('a'+position)), position).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return library.Version{ID: accounts.ID{byte(position)}, URL: "https://93.184.216.34/file.mkv", Origin: library.Origin{Addon: id}}
	}
	serverAddon, ownAddon := source(addons.KindStremio, nil), source(addons.KindStremio, &member.ID)
	musicAddon := source(addons.KindEclipse, nil)
	list, account, ownList := source(addons.KindM3U, nil), source(addons.KindXtream, nil), source(addons.KindXtream, &member.ID)
	recording := library.Version{ID: accounts.ID{90}, URL: "http://127.0.0.1:1/file/recording.ts"}
	removed := library.Version{ID: accounts.ID{91}, URL: "https://93.184.216.34/file.mkv", Origin: library.Origin{Addon: accounts.ID{9, 9}}}

	for _, c := range []struct {
		name    string
		version library.Version
		user    accounts.User
		want    bool
	}{
		{"a server addon, a member", serverAddon, member, false},
		{"a server addon, an administrator", serverAddon, administrator, true},
		{"a server music addon, a member", musicAddon, member, false},
		{"a user's own addon, its owner", ownAddon, member, true},
		{"a user's own addon, another user", ownAddon, other, false},
		{"a user's own addon, an administrator", ownAddon, administrator, false},
		{"a server M3U list, a member", list, member, false},
		{"a server M3U list, an administrator", list, administrator, false},
		{"a server Xtream account, a member", account, member, false},
		{"a server Xtream account, an administrator", account, administrator, false},
		{"a user's own Xtream account, its owner", ownList, member, false},
		{"a recording, a member", recording, member, true},
		{"a source removed since, an administrator", removed, administrator, false},
	} {
		if got := s.SendsToSource(t.Context(), c.user, c.version); got != c.want {
			t.Errorf("%s: sent to the source %v, want %v", c.name, got, c.want)
		}
	}

	// A database that does not answer keeps the source from the app.
	stopped, stop := context.WithCancel(t.Context())
	stop()
	if s.SendsToSource(stopped, administrator, serverAddon) {
		t.Error("an addon that could not be read sends its app to the source")
	}

	// With the setting, every version, an IPTV source's included.
	s.settings = func() accounts.Settings { return accounts.Settings{SendAppsToSource: true} }
	for name, version := range map[string]library.Version{"a server addon": serverAddon, "a server Xtream account": account, "a server M3U list": list,
		"another user's addon": ownAddon} {
		if !s.SendsToSource(t.Context(), other, version) {
			t.Errorf("with Send apps to the source, %s is kept from a member", name)
		}
	}
}

// headSource answers every address with body, announcing its size unless
// unsized is set, as an answer written at each request does not; with
// status when one is set, and the address refuses with a 403, as a link
// that expired is.
type headSource struct {
	body    string
	unsized bool
	fails   bool
	status  int
	refuses string
}

func (h headSource) Open(_ context.Context, _, target string, _ http.Header, _ bool) (*http.Response, error) {
	if h.fails {
		return nil, errors.New("the source does not answer")
	}
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(h.body)), ContentLength: -1}
	if !h.unsized {
		response.ContentLength = int64(len(h.body))
		response.Header.Set("Content-Length", strconv.Itoa(len(h.body)))
	}
	switch {
	case h.refuses != "" && target == h.refuses:
		response.StatusCode = http.StatusForbidden
	case h.status != 0:
		response.StatusCode = h.status
	}
	return response, nil
}

// A manifest relayed as it is hands an app the addresses its source wrote
// in it. An analysis tells one: a DASH manifest always, an HLS playlist
// unless the route names its files at Polyfin's addresses. Before the
// analysis, the first bytes do: text is taken for one, media is not,
// whether its source announces a size or not.
func TestAManifestRelayedAsItIsNamesItsSource(t *testing.T) {
	t.Parallel()
	const playlist = "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nhttps://93.184.216.34/key/seg1.ts\n#EXT-X-ENDLIST\n"
	const manifest = `<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><BaseURL>https://93.184.216.34/key/</BaseURL></MPD>`
	file := "\x1a\x45\xdf\xa3" + strings.Repeat("\x00\x01\x02\x03", 400)
	analyzed := func(s *Service, id byte, format string, duration time.Duration) library.Version {
		t.Helper()
		version := library.Version{ID: accounts.ID{id}, URL: "https://93.184.216.34/key/stream"}
		data, _ := json.Marshal(media.Analysis{Format: format, Duration: duration, Remote: true})
		if _, err := s.db.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
			t.Fatal(err)
		}
		return version
	}
	s := newService(t, headSource{body: file}, "ffprobe", nil)
	for _, c := range []struct {
		name    string
		version library.Version
		linked  bool
		want    bool
	}{
		{"an HLS playlist whose files the route names", analyzed(s, 1, "hls", time.Hour), true, false},
		{"an HLS playlist whose files the route does not name", analyzed(s, 2, "hls", time.Hour), false, true},
		{"a DASH manifest", analyzed(s, 3, "dash", time.Hour), true, true},
		{"a file", analyzed(s, 4, "matroska,webm", time.Hour), false, false},
		{"a torrent's file, not analyzed", library.Version{ID: accounts.ID{5}, Torrent: &library.Torrent{}}, false, false},
		{"a local folder's file, not analyzed", library.Version{ID: accounts.ID{6}, LocalFolder: true, URL: "http://127.0.0.1:1/file"}, false, false},
	} {
		if got, err := s.NamesSource(t.Context(), c.version, c.linked); err != nil || got != c.want {
			t.Errorf("%s: names its source %v %v, want %v", c.name, got, err, c.want)
		}
	}

	// Not analyzed yet: the first bytes tell.
	for _, c := range []struct {
		name   string
		source headSource
		want   bool
	}{
		{"a file", headSource{body: file}, false},
		{"an HLS playlist", headSource{body: playlist}, true},
		{"a DASH manifest", headSource{body: manifest}, true},
		{"an HLS playlist written at each request, without a size", headSource{body: playlist, unsized: true}, true},
		{"a file without a size", headSource{body: file, unsized: true}, false},
		// What gives no bytes has nothing to tell: relay answers for it.
		{"an empty answer", headSource{}, false},
		{"an answer that is no content", headSource{body: "<html>busy</html>", status: http.StatusBadGateway}, false},
	} {
		s := newService(t, c.source, "ffprobe", nil)
		version := library.Version{ID: accounts.ID{7}, URL: "https://93.184.216.34/key/stream"}
		// Whether the route names an HLS playlist's files changes nothing
		// before the analysis that tells a playlist is one.
		for _, linked := range []bool{false, true} {
			if got, err := s.NamesSource(t.Context(), version, linked); err != nil || got != c.want {
				t.Errorf("%s, not analyzed (linked %v): names its source %v %v, want %v", c.name, linked, got, err, c.want)
			}
		}
	}

	// A link that expired is renewed, as relay renews it, and the bytes
	// of the fresh one tell.
	const stale, fresh = "https://93.184.216.34/key/stale", "https://93.184.216.34/key/fresh"
	renewed := 0
	renewing := newService(t, headSource{body: playlist, refuses: stale}, "ffprobe", func(_ context.Context, version library.Version) (library.Version, error) {
		renewed++
		version.URL = fresh
		return version, nil
	})
	if got, err := renewing.NamesSource(t.Context(), library.Version{ID: accounts.ID{8}, URL: stale}, true); err != nil || !got || renewed != 1 {
		t.Errorf("a playlist behind a link that expired: names its source %v %v, after %d renewals", got, err, renewed)
	}

	// A source that cannot be reached, or a link that cannot be renewed,
	// tells nothing: the caller refuses.
	silent := newService(t, headSource{fails: true}, "ffprobe", nil)
	if _, err := silent.NamesSource(t.Context(), library.Version{ID: accounts.ID{9}, URL: fresh}, true); err == nil {
		t.Error("a source that does not answer is told as a file")
	}
	gone := newService(t, headSource{body: playlist, refuses: stale}, "ffprobe", func(context.Context, library.Version) (library.Version, error) {
		return library.Version{}, errors.New("the addon lists the stream no more")
	})
	if _, err := gone.NamesSource(t.Context(), library.Version{ID: accounts.ID{10}, URL: stale}, true); !errors.Is(err, ErrSourceUnavailable) {
		t.Errorf("a link that cannot be renewed: %v, want %v", err, ErrSourceUnavailable)
	}
}
