package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /metrics lists the bytes sent to apps by delivery, every delivery from
// the start, and counts what each adds.
func TestBytesSentAreListedByDelivery(t *testing.T) {
	read := func() string {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		request.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		Handler(func() (bool, string) { return true, "token" }).ServeHTTP(response, request)
		body, _ := io.ReadAll(response.Body)
		return string(body)
	}
	body := read()
	for _, delivery := range []string{DeliveryDirectPlay, DeliveryRemux, DeliveryConversion} {
		if !strings.Contains(body, `polyfin_bytes_sent_total{delivery="`+delivery+`"}`) {
			t.Errorf("%s is not listed: %s", delivery, body)
		}
	}
	before := Value("polyfin_bytes_sent_total", map[string]string{"delivery": DeliveryRemux})
	sent := BytesSent(DeliveryRemux)
	sent.Add(1 << 20)
	sent.Add(512)
	if got := Value("polyfin_bytes_sent_total", map[string]string{"delivery": DeliveryRemux}); got-before != 1<<20+512 {
		t.Errorf("remux counted %v bytes", got-before)
	}
}
