package server

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

var updateScript = flag.Bool("update", false, "write webclient.min.js again from webclient.js")

// webScriptBody is Polyfin's script as the web client loads it: minified.
// The tests of what it does run it.
var webScriptBody = func() []byte {
	data, err := fs.ReadFile(webScriptFS, webScriptFile)
	if err != nil {
		panic(err)
	}
	return data
}()

// webScriptSource is the script's source, webclient.js, for the tests that
// read its text rather than run it.
func webScriptSource(t *testing.T) []byte {
	t.Helper()
	source, err := os.ReadFile("webclient.js")
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// The web client loads webclient.min.js, which is webclient.js minified by
// esbuild, kept ES5 (see TestWebClientScriptParsesAsES5): the source keeps
// its comments, which are half of it, and its names. A change of the source
// writes it again with go generate ./internal/server, which runs this test
// with -update.
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
		t.Errorf("esbuild, webclient.js line %d: %s", line, message.Text)
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
		t.Fatalf("%s is not webclient.js minified: run go generate ./internal/server", webScriptFile)
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
