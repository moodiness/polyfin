package accounts

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A WireGuard configuration for torrents is checked as it is saved, and
// stored as a secret: sealed with the key, its private and preshared keys
// with it, sealed once a key is set when it was stored without one, and
// replaced by another route when the key cannot open it.
func TestWireGuardConfigIsAStoredSecret(t *testing.T) {
	ctx := t.Context()
	pool := migrated.New(t)
	box, _ := secrets.New(bytes.Repeat([]byte{1}, secrets.KeySize))
	other, _ := secrets.New(bytes.Repeat([]byte{2}, secrets.KeySize))
	plain, err := Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Open(ctx, pool, Sealing(box))
	if err != nil {
		t.Fatal(err)
	}
	stored := func() string {
		var value string
		if err := pool.QueryRow(ctx, "SELECT torrent_wireguard FROM settings").Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	private, shared := torrenttest.NewPrivateKey(), torrenttest.NewPrivateKey()
	config := "# From a provider\n[Interface]\nPrivateKey = " + private + "\nAddress = 10.2.0.2/32\nDNS = 10.2.0.1\nPostUp = never run\n\n" +
		"[Peer]\nPublicKey = " + torrenttest.NewPrivateKey() + "\nPresharedKey = " + shared + "\n" +
		"Endpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n"

	// Refused with why, never with a value.
	settings := sealed.Settings()
	settings.TorrentStreams, settings.TorrentWireGuard = true, strings.Replace(config, "AllowedIPs = 0.0.0.0/0", "AllowedIPs = 10.0.0.0/8", 1)
	_, err = sealed.UpdateSettings(ctx, settings)
	var configErr *torrent.WireGuardConfigError
	if !errors.Is(err, ErrInvalidWireGuardConfig) || !errors.As(err, &configErr) || configErr.Key != "AllowedIPs" ||
		configErr.Reason != torrent.WireGuardNotAll || strings.Contains(err.Error(), private) {
		t.Fatalf("a narrow AllowedIPs: %v", err)
	}
	settings.TorrentWireGuard, settings.TorrentProxy = config, "127.0.0.1:1080"
	if _, err := sealed.UpdateSettings(ctx, settings); !errors.Is(err, ErrInvalidTorrentRoute) {
		t.Fatalf("a proxy and a tunnel: %v", err)
	}

	// Saved as it is written again, without what the host would run, and
	// sealed.
	settings.TorrentProxy = ""
	saved, err := sealed.UpdateSettings(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saved.TorrentWireGuard, private) || strings.Contains(saved.TorrentWireGuard, "PostUp") ||
		sealed.Settings().TorrentWireGuard != saved.TorrentWireGuard {
		t.Fatalf("saved %q", saved.TorrentWireGuard)
	}
	if value := stored(); !secrets.Sealed(value) || strings.Contains(value, private) || strings.Contains(value, shared) ||
		strings.Contains(value, "10.2.0.2") {
		t.Fatalf("stored %q", value)
	}
	if report, err := other.Inspect(ctx, pool); err != nil || !slices.Contains(report.Unreadable, secrets.Unreadable{Setting: "torrentWireGuard"}) {
		t.Fatalf("another key: %+v, %v", report, err)
	}

	// The key cannot open it: it reads as none, and another route
	// replaces it.
	restored, err := Open(ctx, pool, Sealing(other))
	if err != nil {
		t.Fatal(err)
	}
	if restored.Settings().TorrentWireGuard != "" {
		t.Fatal("a configuration the key cannot open reads as set")
	}
	settings = restored.Settings()
	settings.TorrentProxy = "127.0.0.1:1080"
	if _, err := restored.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if value := stored(); value != "" {
		t.Fatalf("the configuration stays beside the proxy: %q", value)
	}

	// Stored without a key, it is sealed once one is set.
	settings = plain.Settings()
	settings.TorrentProxy, settings.TorrentWireGuard = "", config
	if _, err := plain.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if value := stored(); secrets.Sealed(value) || !strings.Contains(value, private) {
		t.Fatalf("stored without a key: %q", value)
	}
	if count, err := box.SealStored(ctx, pool); err != nil || count != 1 {
		t.Fatalf("sealed %d, %v", count, err)
	}
	if value := stored(); !secrets.Sealed(value) || strings.Contains(value, private) {
		t.Fatalf("stored once a key is set: %q", value)
	}
}
