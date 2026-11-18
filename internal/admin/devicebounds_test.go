package admin

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The web player's trade of a provider's ticket names its device within
// the bounds of a sign-in: past one it is a malformed request, as one that
// names no device is, answered before the ticket is looked at.
func TestTheProviderTradeNamesItsDeviceWithinBounds(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	b := api.browser()
	device := func(client, name, id, version string) string {
		return fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="%s"`, client, name, id, version)
	}
	long := func(bytes int) string { return strings.Repeat("N", bytes) }
	for name, header := range map[string]string{
		"a device name of 1 KB":      device("Jellyfin Web", long(1<<10), "browser-id", "12.4.0"),
		"an app name past a bound":   device(long(accounts.MaxDeviceTextBytes+1), "Firefox", "browser-id", "12.4.0"),
		"a version past a bound":     device("Jellyfin Web", "Firefox", "browser-id", long(accounts.MaxDeviceTextBytes+1)),
		"an identifier past a bound": device("Jellyfin Web", "Firefox", long(accounts.MaxDeviceIDBytes+1), "12.4.0"),
	} {
		if status, body, _ := b.call(http.MethodPost, "/provider/web", nil, "Authorization", header); status != http.StatusBadRequest ||
			body["error"] != "invalid_request" {
			t.Errorf("a trade with %s: %d %v", name, status, body["error"])
		}
	}
	// At the bounds the device is read, and the ticket, which this
	// browser does not hold, looked at.
	atBounds := device(long(accounts.MaxDeviceTextBytes), long(accounts.MaxDeviceTextBytes), long(accounts.MaxDeviceIDBytes),
		long(accounts.MaxDeviceTextBytes))
	if status, body, _ := b.call(http.MethodPost, "/provider/web", nil, "Authorization", atBounds); status != http.StatusUnauthorized ||
		body["error"] != "invalid_credentials" {
		t.Errorf("a trade at the bounds, without a ticket: %d %v", status, body["error"])
	}
}
