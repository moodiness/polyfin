package admin

import (
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/config"
)

// dataFolderProblems are the problems of the code given that System ›
// Health lists for a server whose health sources change sets, with its
// data folder.
func dataFolderProblems(t *testing.T, code string, change func(*HealthSources)) (found []healthProblemJSON, dataDir string) {
	t.Helper()
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		change(&o.Health)
		dataDir = o.DataDir
	})
	var body struct {
		Problems []healthProblemJSON `json:"problems"`
	}
	if status := api.signedIn("administrator", true).raw(http.MethodGet, "/health/problems?language=en", "", &body); status != http.StatusOK {
		t.Fatalf("problems: %d", status)
	}
	for _, p := range body.Problems {
		if p.Code == code {
			found = append(found, p)
		}
	}
	return found, dataDir
}

// toldIn are the six languages a problem is told in.
var toldIn = []string{"en", "fr", "de", "es", "it", "pt-BR"}

// A data folder that is the default one of earlier versions, in the
// system's temporary folder, is a problem of System › Health, which names
// it and the folder its files are to be moved to; any other data folder
// is none.
func TestADataFolderInTheTemporaryFolderIsAHealthProblem(t *testing.T) {
	t.Parallel()
	if got, _ := dataFolderProblems(t, problemDataFolderTemporary, func(*HealthSources) {}); len(got) != 0 {
		t.Errorf("with a data folder of its own: %+v", got)
	}
	const moveTo = "/home/polyfin/.config/polyfin-server"
	got, dataDir := dataFolderProblems(t, problemDataFolderTemporary, func(h *HealthSources) { h.MoveDataDirTo = moveTo })
	if len(got) != 1 || got[0].Key != "data_folder_temporary" || got[0].Tone != "warning" || got[0].To != "/system/health" ||
		got[0].Path != dataDir || got[0].MoveTo != moveTo {
		t.Fatalf("problems = %+v; want the one of the data folder %s, to move to %s", got, dataDir, moveTo)
	}
	// Each language tells both folders, as the notification does.
	for _, language := range toldIn {
		if text := got[0].text(language); !strings.Contains(text, dataDir) || !strings.Contains(text, moveTo) || !strings.Contains(text, "POLYFIN_DATA_DIR") {
			t.Errorf("%s: %q", language, text)
		}
	}
}

// A data folder an administrator chose that another account of the system
// can change is a problem of System › Health, which names the folder,
// tells whether its owner or its mode is at fault, and gives the command
// that mends it; a folder without a fault is none.
func TestADataFolderAnotherAccountCanChangeIsAHealthProblem(t *testing.T) {
	t.Parallel()
	if got, _ := dataFolderProblems(t, problemDataFolderShared, func(*HealthSources) {}); len(got) != 0 {
		t.Errorf("with a data folder of its own: %+v", got)
	}
	const dir = "/srv/polyfin data"
	for _, test := range []struct {
		fault            config.DataFolderFault
		failure, command string
	}{
		{config.DataFolderFault{Dir: dir, Owner: 1000, User: 1000, Mode: 0o775}, "mode", `chmod go-w "/srv/polyfin data"`},
		{config.DataFolderFault{Dir: dir, Owner: 0, User: 1000, Mode: 0o755}, "owner", `chown 1000 "/srv/polyfin data"`},
	} {
		got, _ := dataFolderProblems(t, problemDataFolderShared, func(h *HealthSources) { h.DataFolderFault = &test.fault })
		if len(got) != 1 || got[0].Key != "data_folder_shared" || got[0].Tone != "warning" || got[0].To != "/system/health" ||
			got[0].Path != dir || got[0].Failure != test.failure || got[0].Command != test.command {
			t.Fatalf("problems = %+v; want the one of %s, for its %s, mended with %s", got, dir, test.failure, test.command)
		}
		texts := map[string]bool{}
		for _, language := range toldIn {
			text := got[0].text(language)
			if !strings.Contains(text, dir) || !strings.Contains(text, test.command) {
				t.Errorf("%s, %s: %q", test.failure, language, text)
			}
			texts[text] = true
		}
		if len(texts) != len(toldIn) {
			t.Errorf("%s: %d texts for %d languages", test.failure, len(texts), len(toldIn))
		}
	}
}
