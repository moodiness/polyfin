package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Each file of the script lists what it relies on in jellyfin-web, its
// anchors, which webclient/check-anchors.mjs reads (see
// docs/development.md). The test has it read every list, so that a list it
// cannot read fails here. With POLYFIN_TEST_JELLYFIN_WEB naming a folder of
// jellyfin-web's built files, such as scripts/packaging/files.sh makes, it
// also finds each anchor there, as CI does with the files of the
// Dockerfile's jellyfin-web stage.
func TestWebClientFilesListTheirAnchors(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	args := []string{filepath.Join(webScriptFolder, "check-anchors.mjs")}
	if folder := os.Getenv("POLYFIN_TEST_JELLYFIN_WEB"); folder != "" {
		args = append(args, folder)
	}
	output, err := exec.CommandContext(t.Context(), node, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("check-anchors.mjs: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
