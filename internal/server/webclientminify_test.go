package server

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

var updateScript = flag.Bool("update", false, "write webclient.min.js again from the files of webclient/")

// webScriptFolder holds the script's source: a file for each part and the
// core the parts share (see docs/development.md).
const webScriptFolder = "webclient"

// webScriptGroups are the files of webScriptFolder in the order they run,
// which is the order the parts ran in when the script was one file. Each
// group runs in a function of its own, ;(function () { … })(), and its
// files share that function's scope: the parts of the third group call
// the helpers of core/shared.js and each other's functions, and
// core/start.js, last, follows the routes for them all. A file holds the
// statements of its group's function, indented as in it.
var webScriptGroups = [][]string{
	{"core/page-changes.js"},
	{"19-web-themes.js"},
	{
		"core/shared.js",
		"04-menu-names.js",
		"01-admin-pages.js",
		"02-versions.js",
		"03-play-buttons.js",
		"07-hidden-libraries.js",
		"08-admin-link.js",
		"05-chosen-screen.js",
		"06-duplicate-errors.js",
		"09-scores.js",
		"10-item-menus.js",
		"12-share.js",
		"core/start.js",
	},
	{"11-home-rows.js"},
	{"13-reminders.js"},
	{"14-kept-answers.js"},
	{"15-recent-searches.js"},
	{"16-poster-ratings.js"},
	{"17-torrent-start.js"},
	{"18-provider-sign-in.js"},
	{"20-volume-boost.js"},
	{"21-version-badges.js"},
	{"22-scroll-back.js"},
	{"23-trailers.js"},
	{"24-collection-types.js"},
}

// webScriptPart is where a file of webScriptFolder starts in the script
// assembled: its first line there, counted from 1.
type webScriptPart struct {
	file string
	line int
}

// assembledWebScript is the script's source as webScriptGroups assemble
// it, with where each file starts in it.
var assembledWebScript = sync.OnceValues(func() (assembled struct {
	text  []byte
	parts []webScriptPart
}, err error) {
	var text bytes.Buffer
	line := 1
	for _, group := range webScriptGroups {
		text.WriteString(";(function () {\n")
		line++
		for _, file := range group {
			data, err := os.ReadFile(filepath.Join(webScriptFolder, filepath.FromSlash(file)))
			if err != nil {
				return assembled, err
			}
			if !bytes.HasSuffix(data, []byte("\n")) {
				return assembled, fmt.Errorf("%s does not end with a line end", file)
			}
			assembled.parts = append(assembled.parts, webScriptPart{file, line})
			text.Write(data)
			line += bytes.Count(data, []byte("\n"))
		}
		text.WriteString("})()\n")
		line++
	}
	assembled.text = text.Bytes()
	return assembled, nil
})

// webScriptBody is Polyfin's script as the web client loads it: minified.
// The tests of what it does run it.
var webScriptBody = func() []byte {
	data, err := fs.ReadFile(webScriptFS, webScriptFile)
	if err != nil {
		panic(err)
	}
	return data
}()

// webScriptSource is the script's source, its files assembled, for the
// tests that read its text rather than run it.
func webScriptSource(t *testing.T) []byte {
	t.Helper()
	assembled, err := assembledWebScript()
	if err != nil {
		t.Fatal(err)
	}
	return assembled.text
}

// webScriptPlace names the file and the line of a line of the script
// assembled, such as "webclient/02-versions.js line 120", for a failure's
// message.
func webScriptPlace(t *testing.T, line int) string {
	t.Helper()
	assembled, err := assembledWebScript()
	if err != nil {
		t.Fatal(err)
	}
	place := fmt.Sprintf("line %d of the script assembled", line)
	for _, part := range assembled.parts {
		if part.line <= line {
			place = fmt.Sprintf("%s line %d", path.Join(webScriptFolder, part.file), line-part.line+1)
		}
	}
	return place
}

// The web client loads webclient.min.js, which is the script's files
// assembled, then minified by esbuild, kept ES5 (see
// TestWebClientScriptParsesAsES5): the source keeps its comments, which
// are half of it, and its names. A change of the source writes it again
// with go generate ./internal/server, which runs this test with -update.
func TestTheScriptIsServedMinified(t *testing.T) {
	t.Parallel()
	result := api.Transform(string(webScriptSource(t)), api.TransformOptions{
		Loader:            api.LoaderJS,
		Target:            api.ES5,
		Charset:           api.CharsetUTF8,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
	})
	for _, message := range append(result.Errors, result.Warnings...) {
		line := 0
		if message.Location != nil {
			line = message.Location.Line
		}
		t.Errorf("esbuild, %s: %s", webScriptPlace(t, line), message.Text)
	}
	if t.Failed() {
		return
	}
	if *updateScript {
		if err := os.WriteFile(webScriptFile, result.Code, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if !bytes.Equal(result.Code, webScriptBody) {
		t.Fatalf("%s is not the script's files minified: run go generate ./internal/server", webScriptFile)
	}
	response := get(webHandler(setupState{}), "/web/"+webScript)
	if !bytes.Equal(response.Body.Bytes(), webScriptBody) {
		t.Errorf("/web/%s is not %s", webScript, webScriptFile)
	}
	// Minified, the script is less than half its source: a change of the
	// minifier's options that lost that would show here.
	if source := len(webScriptSource(t)); len(webScriptBody)*2 > source {
		t.Errorf("%s is %d bytes, for a source of %d", webScriptFile, len(webScriptBody), source)
	}
}

// Every file of the script's folder runs, once: a part added to the folder
// but not to webScriptGroups would be left out of the script.
func TestTheScriptAssemblesEveryFileOnce(t *testing.T) {
	t.Parallel()
	var listed []string
	for _, group := range webScriptGroups {
		listed = append(listed, group...)
	}
	var found []string
	err := filepath.WalkDir(webScriptFolder, func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(name, ".js") {
			relative, _ := filepath.Rel(webScriptFolder, name)
			found = append(found, filepath.ToSlash(relative))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(listed)
	slices.Sort(found)
	if !slices.Equal(listed, found) {
		t.Errorf("webScriptGroups lists %v, the folder holds %v", listed, found)
	}
}

// The page links the script with its version, its ETag without quotes,
// at which the script is cached for good: a new Polyfin's page links its
// new version. At any other address it is checked again (see
// TestOwnFilesAreCachedForGoodOrRevalidated).
func TestThePageLinksTheScriptAtItsVersion(t *testing.T) {
	t.Parallel()
	h := webHandler(setupState{})
	version := strings.Trim(get(h, "/web/"+webScript).Header().Get("ETag"), `"`)
	if want := `<script src="polyfin.js?v=` + version + `"></script>`; version == "" || !strings.Contains(get(h, "/web/").Body.String(), want) {
		t.Fatalf("the page does not link %s", want)
	}
	response := get(h, "/web/"+webScript+"?v="+version)
	header := response.Header()
	if header.Get("Cache-Control") != immutableCache || header.Get("Content-Type") != "text/javascript; charset=utf-8" || !bytes.Equal(response.Body.Bytes(), webScriptBody) {
		t.Errorf("at its version: Cache-Control %q, Content-Type %q, %d bytes", header.Get("Cache-Control"), header.Get("Content-Type"), response.Body.Len())
	}
}
