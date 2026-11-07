package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

// The script the web client loads, polyfin.js, parses as ES5 (ECMAScript
// 5.1): jellyfin-web 12.4 is built for engines as old as Chrome 27 and iOS
// 10, those of TV browsers among them, and an engine that cannot parse the
// script runs none of it. Both are parsed: the script as served, minified,
// and its source, webclient.js, where a failure is easier to find. They are
// parsed with the parser Node.js carries for its own use, acorn, which
// --expose-internals lets a script load: no other parser is needed. The
// parser must refuse a trailing comma in a call, as ES2017 brought them,
// else the check would prove nothing.
func TestWebClientScriptParsesAsES5(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	response := get(webHandler(setupState{}), "/web/"+webScript)
	if response.Code != http.StatusOK {
		t.Fatalf("/web/%s: %d", webScript, response.Code)
	}
	const harness = `
let acorn
try {
  acorn = require('internal/deps/acorn/acorn/dist/acorn')
} catch (error) {
  process.stdout.write(JSON.stringify({ missing: String(error) }))
  process.exit(0)
}
const scripts = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const refuses = (code) => {
  try {
    acorn.parse(code, { ecmaVersion: 5, sourceType: 'script' })
    return null
  } catch (error) {
    return { line: error.loc.line, column: error.loc.column, message: error.message }
  }
}
process.stdout.write(JSON.stringify({ probe: refuses('f(a,\n)') !== null, refused: scripts.map(refuses) }))
`
	scripts := []struct{ name, text string }{
		{"polyfin.js as served", response.Body.String()},
		{"webclient.js", string(webScriptSource(t))},
	}
	input, _ := json.Marshal([]string{scripts[0].text, scripts[1].text})
	command := exec.CommandContext(t.Context(), node, "--expose-internals", "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var result struct {
		Missing string
		Probe   bool
		Refused []*struct {
			Line, Column int
			Message      string
		}
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	if result.Missing != "" {
		t.Fatalf("Node.js %s carries no acorn where the test looks for it: %s", nodeVersion(t, node), result.Missing)
	}
	if !result.Probe {
		t.Fatal("the parser takes a trailing comma in a call as ES5")
	}
	if len(result.Refused) != len(scripts) {
		t.Fatalf("node parsed %d scripts, want %d", len(result.Refused), len(scripts))
	}
	for i, refused := range result.Refused {
		if refused == nil {
			continue
		}
		// The minified script is one long line: the error's surroundings.
		near := ""
		if lines := strings.Split(scripts[i].text, "\n"); refused.Line >= 1 && refused.Line <= len(lines) {
			line := []rune(lines[refused.Line-1])
			to := min(refused.Column+40, len(line))
			near = strings.TrimSpace(string(line[min(max(refused.Column-40, 0), to):to]))
		}
		t.Errorf("%s is not ES5 at line %d, column %d (%s): %q", scripts[i].name, refused.Line, refused.Column, refused.Message, near)
	}
}

// nodeVersion is the version node prints, for a failure's message.
func nodeVersion(t *testing.T, node string) string {
	output, err := exec.CommandContext(t.Context(), node, "--version").Output()
	if err != nil {
		return "(unknown version)"
	}
	return strings.TrimSpace(string(output))
}
