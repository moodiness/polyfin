package jellyfin

import (
	"net/http"
	"testing"
)

// A channel gives its parent, the Live TV view, in a listing that asks for
// ParentId, as in its details and as Jellyfin's channels do; not unasked.
func TestListedChannelsGiveTheirParentWhenAsked(t *testing.T) {
	t.Parallel()
	s, token, _ := tuned(t, newTVAddon(t, false, ""))
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) == 0 || channels.Items[0].ParentId != "" {
		t.Fatalf("channels listed unasked: %+v", channels.Items)
	}
	channel := channels.Items[0].Id
	for _, path := range []string{"/Items?ids=" + channel + "&Fields=ParentId", "/LiveTv/Channels?Fields=ParentId"} {
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK || len(page.Items) == 0 || page.Items[0].ParentId != liveTvViewID.String() {
			t.Errorf("%s: %d %+v", path, status, page.Items)
		}
	}
	var details BaseItemDto
	if s.get(t, "/Items/"+channel, token, &details); details.ParentId != liveTvViewID.String() {
		t.Errorf("details: parent %q", details.ParentId)
	}
}
