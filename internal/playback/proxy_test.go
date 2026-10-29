package playback

import (
	"context"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
)

type unusedOpener struct{}

func (unusedOpener) Open(context.Context, string, string, http.Header, bool) (*http.Response, error) {
	return nil, nil
}

// A server addon's stream is read through the source proxy when one is set;
// the other sources, and an empty setting, keep their opener.
func TestSourceProxyLocation(t *testing.T) {
	t.Parallel()
	var proxies []string
	s := &Service{
		settings: func() accounts.Settings { return accounts.Settings{SourceProxy: "socks5h://warp:1080"} },
		sourceProxy: func(proxy string) source.Opener {
			proxies = append(proxies, proxy)
			return unusedOpener{}
		},
	}
	proxied := s.locationOf(library.Version{URL: "https://example.org/movie.mkv", ProxySource: true})
	if proxied.Opener == nil || len(proxies) != 1 || proxies[0] != "socks5h://warp:1080" {
		t.Fatalf("proxied: %#v, %v", proxied, proxies)
	}
	direct := s.locationOf(library.Version{URL: "https://example.org/movie.mkv"})
	if direct.Opener != nil || len(proxies) != 1 {
		t.Errorf("direct: %#v, %v", direct, proxies)
	}
	s.settings = func() accounts.Settings { return accounts.Settings{} }
	empty := s.locationOf(library.Version{URL: "https://example.org/movie.mkv", ProxySource: true})
	if empty.Opener != nil || len(proxies) != 1 {
		t.Errorf("empty setting: %#v, %v", empty, proxies)
	}
}

// A server addon's stream is read through the source proxy only when the
// addon's own choice allows it.
func TestSourceProxyPerAddon(t *testing.T) {
	t.Parallel()
	proxies := 0
	s := &Service{
		settings: func() accounts.Settings { return accounts.Settings{SourceProxy: "socks5h://warp:1080"} },
		sourceProxy: func(string) source.Opener {
			proxies++
			return unusedOpener{}
		},
	}
	version := library.Version{ProxySource: true, Origin: library.Origin{Addon: accounts.ID{}}}
	s.sourceProxyForAddon = func(accounts.ID) bool { return false }
	if location := s.locationOf(version); location.Opener != nil || proxies != 0 {
		t.Fatalf("off: %#v, %d", location, proxies)
	}
	s.sourceProxyForAddon = func(accounts.ID) bool { return true }
	if location := s.locationOf(version); location.Opener == nil || proxies != 1 {
		t.Fatalf("on: %#v, %d", location, proxies)
	}
	s.settings = func() accounts.Settings { return accounts.Settings{} }
	if location := s.locationOf(version); location.Opener != nil || proxies != 1 {
		t.Fatalf("no proxy set: %#v, %d", location, proxies)
	}
}
