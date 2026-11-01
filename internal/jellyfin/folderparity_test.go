package jellyfin

import (
	"net/http"
	"testing"
)

// A folder of videos is an item that plays nothing itself, as Jellyfin
// 12.4's home videos folders: PlaybackInfo, its stream and its download
// answer 400, as for any item without media sources (MediaSourceManager.cs,
// GetStaticMediaSources), without asking the addon for streams of the
// folder's own identifier. The series routes do not find it, as it is no
// series (TvShowsController.cs).
func TestFoldersPlayNothingThemselves(t *testing.T) {
	t.Parallel()
	l := newStoreLibrary(t)
	var item BaseItemDto
	if status := l.get(t, "/Items/"+l.folder, l.token, &item); status != http.StatusOK || item.Type != "Folder" {
		t.Errorf("the folder's details: %d %+v", status, item)
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/Items/" + l.folder + "/PlaybackInfo"},
		{http.MethodPost, "/Items/" + l.folder + "/PlaybackInfo"},
		{http.MethodGet, "/Videos/" + l.folder + "/stream?static=true"},
		{http.MethodGet, "/Videos/" + l.folder + "/stream.mp4?static=true"},
		{http.MethodGet, "/Items/" + l.folder + "/Download"},
	} {
		var body any
		if request.method == http.MethodPost {
			body = map[string]any{}
		}
		if status, data := l.call(request.method, request.path, app("tv", l.token), body); status != http.StatusBadRequest ||
			string(data) != "Error processing request." {
			t.Errorf("%s %s: %d %s", request.method, request.path, status, data)
		}
	}
	if asked := l.store.streamAsks(storeDownload); asked != 0 {
		t.Errorf("the addon was asked %d times for the folder's own streams", asked)
	}
	// An item that does not exist is still not found.
	if status, _ := l.call(http.MethodPost, "/Items/"+testServerID+"/PlaybackInfo", app("tv", l.token), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("PlaybackInfo of an unknown item: %d", status)
	}
	// The title without videos is a video: PlaybackInfo answers it (the
	// fake file plays nowhere, which the other tests of folders cover).
	if status, data := l.call(http.MethodPost, "/Items/"+l.single+"/PlaybackInfo", app("tv", l.token), map[string]any{}); status != http.StatusOK {
		t.Errorf("PlaybackInfo of the title without videos: %d %s", status, data)
	}
	// It is no series.
	for _, path := range []string{"/Shows/" + l.folder + "/Seasons", "/Shows/" + l.folder + "/Episodes"} {
		if status, data := l.call(http.MethodGet, path, app("tv", l.token), nil); status != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, status, data)
		}
	}
}
