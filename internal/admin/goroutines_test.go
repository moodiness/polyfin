package admin

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// waitForever stands for a request that never answers.
func waitForever(release <-chan struct{}) {
	<-release
}

// An administrator downloads what every goroutine is doing: a goroutine
// waiting is there, with what it waits for and its stack. The goroutine
// may not have started, or not be waiting yet, when the first download is
// taken, on a busy machine: the download is taken again until it shows it,
// for as long as a slow runner may take.
func TestAnAdministratorDownloadsTheServersGoroutines(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	release := make(chan struct{})
	defer close(release)
	go waitForever(release)

	download := func() string {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.url+"/admin/api/health/goroutines", nil)
		response, err := admin.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Disposition"), "attachment") {
			t.Fatalf("download: %d %v", response.StatusCode, response.Header)
		}
		return string(body)
	}
	waiting := func(body string) bool {
		for _, goroutine := range strings.Split(body, "\n\n") {
			if strings.Contains(goroutine, "[chan receive") && strings.Contains(goroutine, "admin.waitForever(") {
				return true
			}
		}
		return false
	}
	var body string
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if body = download(); waiting(body) {
			return
		}
		if time.Now().After(deadline) {
			break
		}
	}
	t.Fatalf("the waiting goroutine is not in the download:\n%.2000s", body)
}
