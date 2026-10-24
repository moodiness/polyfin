package admin

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// waitForever stands for a request that never answers.
func waitForever(release <-chan struct{}) {
	<-release
}

// An administrator downloads what every goroutine is doing: a goroutine
// waiting is there, with what it waits for and its stack.
func TestAnAdministratorDownloadsTheServersGoroutines(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	release := make(chan struct{})
	defer close(release)
	go waitForever(release)

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
	for _, goroutine := range strings.Split(string(body), "\n\n") {
		if strings.Contains(goroutine, "[chan receive") && strings.Contains(goroutine, "admin.waitForever(") {
			return
		}
	}
	t.Fatalf("the waiting goroutine is not in the download:\n%.2000s", body)
}
