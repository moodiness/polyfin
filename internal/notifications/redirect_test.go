package notifications

import (
	"net/http"
	"strings"
	"testing"
)

// A Gotify target whose address redirects to another host, here the fake
// targets named localhost rather than 127.0.0.1, gets its message there
// without its key. A Pushover message, which holds the user's key and the
// app's token, is not sent again to another host on a 307: the test says
// why it was not delivered.
func TestTargetsSecretsGoToNoOtherHost(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, gotify, pushover := h.chatServices(t)
	elsewhere := strings.Replace(h.targets.url, "127.0.0.1", "localhost", 1)
	h.targets.always("/gotify/message", status(http.StatusTemporaryRedirect, "Location", elsewhere+"/elsewhere/gotify"))
	h.targets.always(pushoverPath, status(http.StatusTemporaryRedirect, "Location", elsewhere+"/elsewhere/pushover"))

	if result, err := h.Test(t.Context(), nil, gotify.ID); err != nil || !result.Delivered {
		t.Fatalf("Gotify through a redirect: %+v, %v", result, err)
	}
	landed := h.targets.at("/elsewhere/gotify")
	if len(landed) != 1 || landed[0].body["message"] == nil {
		t.Fatalf("the other host received %+v, want the message once", landed)
	}
	if key := landed[0].header.Values("X-Gotify-Key"); len(key) != 0 {
		t.Errorf("the other host was sent the key %q", key)
	}

	result, err := h.Test(t.Context(), nil, pushover.ID)
	if err != nil || result.Delivered || result.Answer != "redirected to another host" {
		t.Errorf("Pushover through a 307 to another host: %+v, %v", result, err)
	}
	if landed := h.targets.at("/elsewhere/pushover"); len(landed) != 0 {
		t.Errorf("the other host received %+v", landed)
	}
}
