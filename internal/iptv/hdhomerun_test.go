package iptv

import (
	"errors"
	"testing"
)

// A tuner's address keeps its path, as proxies that act as a tuner serve
// it under one; its documents' addresses and a final "/" name the same
// tuner. Addresses that are no tuner's are still refused.
func TestTunerAddressesKeepTheirPath(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"192.168.1.20":                              "http://192.168.1.20",
		"http://192.168.1.20/":                      "http://192.168.1.20",
		"http://192.168.1.20/discover.json":         "http://192.168.1.20",
		"http://192.168.1.20/lineup.json":           "http://192.168.1.20",
		"http://nas.lan:9191/hdhr":                  "http://nas.lan:9191/hdhr",
		"nas.lan:9191/hdhr/":                        "http://nas.lan:9191/hdhr",
		"https://nas.lan/hdhr/discover.json":        "https://nas.lan/hdhr",
		"http://nas.lan:9191/hdhr/news/lineup.json": "http://nas.lan:9191/hdhr/news",
		"http://nas.lan/tv%20tuner":                 "http://nas.lan/tv%20tuner",
	} {
		if got, err := tunerAddress(raw); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "ftp://nas.lan/hdhr", "http://", "http://user:secret@nas.lan/hdhr", "http://nas.lan/hdhr?profile=1",
		"http://nas.lan/hdhr?", "http://nas.lan/hdhr#top", "mailto:tuner@nas.lan"} {
		if got, err := tunerAddress(raw); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("%q: %q %v, want refused", raw, got, err)
		}
	}
}
