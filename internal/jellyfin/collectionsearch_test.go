package jellyfin

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// The admin app's collection pages find what jellyfin-web's search finds:
// an IPTV source's movies and a local folder's titles as an addon's, which
// then go in a collection like any title.
func TestCollectionSearchFindsWhatJellyfinWebSearchFinds(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	ctx := t.Context()
	provider := newVODProvider(t, "", 0)
	on, off := true, false
	if _, err := p.iptv.Add(ctx, addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream, Server: provider.url,
		Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}}, false); err != nil {
		t.Fatal(err)
	}
	id, _ := accounts.ParseID(p.user)
	user, err := p.store.User(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	p.limit(t, user, func(c *accounts.UserChanges) { c.CollectionManagement = new(true) })

	found := func(term string) library.Item {
		t.Helper()
		var web QueryResult
		p.get(t, "/Items?searchTerm="+url.QueryEscape(term)+"&Recursive=true", p.token, &web)
		items, err := p.handler.CollectionSearch(ctx, user, term, 30)
		if err != nil {
			t.Fatal(err)
		}
		var ids, webIDs []string
		for _, item := range items {
			ids = append(ids, item.ID.String())
		}
		for _, item := range web.Items {
			webIDs = append(webIDs, item.Id)
		}
		if len(items) != 1 || !slices.Contains(webIDs, ids[0]) {
			t.Fatalf("%s: %v, jellyfin-web finds %v", term, itemNames(mustNames(items)), itemNames(web.Items))
		}
		return items[0]
	}
	movie := found("orb tale")
	local := found("harbor lights")
	if movie.Kind != library.KindMovie || local.Kind != library.KindMovie {
		t.Errorf("kinds: %s %s", movie.Kind, local.Kind)
	}

	picks := p.createCollection(t, p.token, "name=Picks&ids="+movie.ID.String()+","+local.ID.String())
	var listed QueryResult
	if status := p.get(t, "/Items?ParentId="+picks, p.token, &listed); status != http.StatusOK || len(listed.Items) != 2 {
		t.Errorf("Picks: %d %v", status, itemNames(listed.Items))
	}
}

// mustNames describes items by their names alone, for failures.
func mustNames(items []library.Item) []BaseItemDto {
	result := make([]BaseItemDto, 0, len(items))
	for _, item := range items {
		result = append(result, BaseItemDto{Name: item.Name})
	}
	return result
}
