package library

import (
	"context"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// namingIPTV is an IPTV source whose movies and series the other addons
// complete and name, recording the names it learns by identifier.
type namingIPTV struct {
	IPTV
	mu      sync.Mutex
	learned map[string]string
}

func (*namingIPTV) Enrichment(context.Context, accounts.ID) (enriched, named bool) { return true, true }

func (n *namingIPTV) LearnName(_ context.Context, _ accounts.ID, id, name string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.learned[id] = name
	return nil
}

func (n *namingIPTV) name(id string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.learned[id]
}

// An IPTV source's titles, whose names every user of the source sees, are
// named by an addon without a language: for a server's source, one of
// the server's that everyone uses; for a user's own source, any of the
// user's.
func TestWhoNamesAnIPTVSourcesTitles(t *testing.T) {
	t.Parallel()
	server := installed{shared: true}
	own := installed{}
	limited := installed{shared: true, addon: addons.Addon{UsedBy: addons.UsedBy{Listed: true}}}
	translating := installed{shared: true, addon: addons.Addon{Language: "fr"}}
	for _, c := range []struct {
		name              string
		source, candidate installed
		want              bool
	}{
		{"the server's addon, a server's source", server, server, true},
		{"a member's own addon, a server's source", server, own, false},
		{"an addon limited by Used by, a server's source", server, limited, false},
		{"a translator, a server's source", server, translating, false},
		{"a member's own addon, their own source", own, own, true},
		{"the server's addon, a member's own source", own, server, true},
		{"an addon limited by Used by, a member's own source", own, limited, true},
		{"a translator, a member's own source", own, translating, false},
	} {
		if got := namesFor(c.source, c.candidate); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// A server's IPTV movie that names its titles after the metadata addons
// takes the name of the server's metadata addon that everyone uses, and
// the source learns it; one that Used by limits describes it without
// naming it.
func TestIPTVTitlesTakeTheServersMetadataNames(t *testing.T) {
	t.Parallel()
	described := stremio.Meta{ID: "tt100", Type: "movie", Name: "Canonical Movie", Logo: "https://images.example/logo.png"}
	manifest := stremio.Manifest{ID: "metadata", Name: "Metadata", Version: "1", Types: []string{"movie"},
		Resources: []stremio.Resource{{Name: "meta", Types: []string{"movie"}, IDPrefixes: []string{"tt"}}}}
	source := installed{addon: addons.Addon{ID: accounts.ID{9}, Kind: addons.KindXtream}, shared: true}
	own := stremio.Meta{ID: "polyfin-iptv:box:vod:1", Type: "movie", Name: "BRAND | Movie", ImdbID: "tt100"}
	for _, limited := range []bool{false, true} {
		e := newEnv(t)
		e.install(addons.Shared(), &fakeAddon{manifest: manifest, metas: map[string]stremio.Meta{"movie/tt100": described}})
		if limited {
			usedBy := addons.UsedBy{Listed: true, Users: []accounts.ID{e.member.ID}}
			if _, err := e.addons.SetUsedBy(t.Context(), e.installedID(addons.Shared(), "metadata"), usedBy); err != nil {
				t.Fatal(err)
			}
		}
		naming := &namingIPTV{learned: map[string]string{}}
		e.service.iptv = naming
		v, err := e.service.view(t.Context(), e.member)
		if err != nil {
			t.Fatal(err)
		}
		meta, ok := e.service.enrich(t.Context(), v, source, own, false, fromAddons)
		wantName, wantLearned := "Canonical Movie", "Canonical Movie"
		if limited {
			wantName, wantLearned = own.Name, ""
		}
		if !ok || meta.Logo == "" || meta.Name != wantName || naming.name(own.ID) != wantLearned {
			t.Errorf("limited by Used by %v: %q %v, learned %q", limited, meta.Name, ok, naming.name(own.ID))
		}
	}
}
