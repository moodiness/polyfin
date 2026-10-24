package themecopy

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// place is where a reference to a file is, which tells what it may load:
// a stylesheet for @import, a font or an image for a CSS url(), and any of
// the four kinds for an address a script names.
type place int

const (
	inImport place = iota
	inURL
	inScript
)

// reference is an address a text names, at text[start:end]: raw as
// written, and resolved. builtAtRunTime tells a script address the code
// completes when it runs, which no copy can follow.
type reference struct {
	start, end     int
	raw            string
	address        string
	place          place
	builtAtRunTime bool
}

var (
	// cssImport matches @import with its address, in url() or a string.
	cssImport = regexp.MustCompile(`(?i)@import\s+(?:url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s'"]*))\s*\)|"([^"]*)"|'([^']*)')`)
	// cssURL matches a url() with its address, quoted or not.
	cssURL = regexp.MustCompile(`(?i)\burl\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s'"]*))\s*\)`)
	// cssComment matches a CSS comment, whose addresses load nothing.
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	// scriptAddress matches an http or https address in a script: the
	// characters that cannot be part of one end it.
	scriptAddress = regexp.MustCompile("https?://[^\\s'\"`<>()\\\\{}$|^]+")
)

// cssReferences returns the addresses a stylesheet loads, resolved against
// base, its own address, or nil for a text in the page: the page's own
// relative addresses are Polyfin's, not to be saved.
func cssReferences(text string, base *url.URL) []reference {
	comments := cssComment.FindAllStringIndex(text, -1)
	inComment := func(at int) bool {
		for _, span := range comments {
			if at >= span[0] && at < span[1] {
				return true
			}
		}
		return false
	}
	var refs []reference
	var imports [][]int
	add := func(match []int, at place) {
		for group := 2; group < len(match); group += 2 {
			if match[group] < 0 {
				continue
			}
			raw := text[match[group]:match[group+1]]
			if address, ok := resolve(strings.TrimSpace(raw), base); ok {
				refs = append(refs, reference{start: match[group], end: match[group+1], raw: raw, address: address, place: at})
			}
			return
		}
	}
	for _, match := range cssImport.FindAllStringSubmatchIndex(text, -1) {
		if inComment(match[0]) {
			continue
		}
		imports = append(imports, match[:2])
		add(match, inImport)
	}
	for _, match := range cssURL.FindAllStringSubmatchIndex(text, -1) {
		if inComment(match[0]) {
			continue
		}
		inside := false
		for _, span := range imports {
			if match[0] >= span[0] && match[1] <= span[1] {
				inside = true
			}
		}
		if !inside {
			add(match, inURL)
		}
	}
	slices.SortFunc(refs, func(a, b reference) int { return a.start - b.start })
	return refs
}

// scriptReferences returns the http and https addresses a script names.
// An address the code completes when it runs, followed by a template's
// ${ or by a + after its string, or ending with a slash, is told as such:
// it is no file to save.
func scriptReferences(text string) []reference {
	var refs []reference
	for _, match := range scriptAddress.FindAllStringIndex(text, -1) {
		raw := text[match[0]:match[1]]
		address, ok := resolve(raw, nil)
		if !ok {
			continue
		}
		ref := reference{start: match[0], end: match[1], raw: raw, address: address, place: inScript}
		rest := text[match[1]:]
		after := strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(rest, "'"), `"`), "`"), " \t\r\n")
		ref.builtAtRunTime = strings.HasPrefix(rest, "${") || strings.HasPrefix(after, "+") || strings.HasSuffix(raw, "/")
		refs = append(refs, ref)
	}
	return refs
}

// resolve returns the absolute http or https address raw names, against
// base when it is relative, and false for an address that loads nothing
// from elsewhere: data:, a fragment, or a relative one without base.
func resolve(raw string, base *url.URL) (string, bool) {
	if raw == "" || strings.HasPrefix(raw, "#") {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if base != nil {
		parsed = base.ResolveReference(parsed)
	} else if strings.HasPrefix(raw, "//") {
		parsed.Scheme = "https"
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	parsed.Fragment = ""
	return parsed.String(), true
}

// fragment returns the fragment raw ends with, such as #home for an SVG
// image's symbol, which the address of the file saved keeps.
func fragment(raw string) string {
	if at := strings.IndexByte(raw, '#'); at >= 0 {
		return raw[at:]
	}
	return ""
}

// rewrite returns text with each reference replaced by what by gives for
// it; a reference by gives "" for is left as written.
func rewrite(text string, refs []reference, by func(reference) string) string {
	var out strings.Builder
	at := 0
	for _, ref := range refs {
		replacement := by(ref)
		if replacement == "" || ref.start < at {
			continue
		}
		out.WriteString(text[at:ref.start])
		out.WriteString(replacement)
		at = ref.end
	}
	out.WriteString(text[at:])
	return out.String()
}
