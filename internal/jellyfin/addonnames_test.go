package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// An addon given a name of its own is shown under it in apps: its
// versions' short names end with it, and its libraries are told apart
// from those of another install of the same addon by it.
func TestARenamedAddonNamesItsVersionsAndLibraries(t *testing.T) {
	t.Parallel()
	p := playing(t)
	installed, err := p.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	if _, err := p.addons.SetName(t.Context(), addons.Shared(), installed[0].ID, "Mine"); err != nil {
		t.Fatal(err)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionNames, s.NameHowVersionsPlay = "short", false })
	var dto BaseItemDto
	if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &dto); status != http.StatusOK || dto.MediaSources == nil {
		t.Fatalf("details: %d", status)
	}
	var names []string
	for _, source := range *dto.MediaSources {
		names = append(names, source.Name)
	}
	if want := []string{"4K · HEVC · 1 MB · Mine", "SD · H.264 · Mono · EN · 1 MB · Mine"}; !slices.Equal(names, want) {
		t.Errorf("version names: %q, want %q", names, want)
	}

	// A second install of the same addon: the libraries are named after
	// the addons once their catalogs' names collide.
	if _, err := p.addons.Install(t.Context(), addons.Shared(), streamingAddon(t), false); err != nil {
		t.Fatal(err)
	}
	views := func() []string {
		t.Helper()
		var result QueryResult
		p.get(t, "/UserViews", p.token, &result)
		var list []string
		for _, item := range result.Items {
			list = append(list, item.Name)
		}
		return list
	}
	if got := views(); !slices.Contains(got, "Top (Movies, Mine)") || !slices.Contains(got, "Top (Movies, Streams)") {
		t.Errorf("libraries: %q", got)
	}
	if _, err := p.addons.SetName(t.Context(), addons.Shared(), installed[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := views(); !slices.Contains(got, "Top (Movies, Streams)") || !slices.Contains(got, "Top (Movies, Streams) (2)") {
		t.Errorf("libraries once the name is the manifest's again: %q", got)
	}
}
