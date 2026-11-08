package i18n

import (
	"flag"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var update = flag.Bool("update", false, "set the French texts apart with narrow no-break spaces, in place")

// wide finds a space a French text holds where a narrow no-break space
// belongs: a plain or no-break one before : ; ? ! and », or after «. A
// sign a letter or a digit follows is not punctuation but syntax, as in
// "Ajoutez :port", and keeps its space.
var wide = regexp.MustCompile(`[ \x{a0}](?:[:;?!](?:[^\pL\pN]|$)|»)|«[ \x{a0}]`)

// TestFrenchSpacing reads the French texts of the module's Go files but
// tests (the French of each i18n.Text literal, and the "fr" value of each
// map by language), and fails on one with a plain or no-break space before
// : ; ? ! or », or after «: French sets them apart with a narrow no-break
// space (U+202F), so that a line never breaks before them. With -update,
// as go generate runs it and web/scripts/french-spacing.mjs runs go
// generate, it writes these spaces \u202f in place instead.
func TestFrenchSpacing(t *testing.T) {
	t.Parallel()
	texts := 0
	moduleFiles(t, func(files *token.FileSet, path string, file *ast.File) {
		type edit struct {
			start, end int
			text       string
		}
		var edits []edit
		for _, literal := range frenchStrings(file) {
			texts++
			value := stringValue(literal)
			narrowed, changed := narrow(literal.Value)
			if changed == 0 {
				continue
			}
			where := files.Position(literal.Pos())
			switch {
			case !*update:
				if wide.MatchString(value) {
					t.Errorf("%s: a French text sets a sign apart with a wide space, %q", where, value)
				}
			case strings.HasPrefix(literal.Value, "`"):
				t.Errorf("%s: a raw string cannot write \\u202f, set it apart by hand: %q", where, value)
			default:
				edits = append(edits, edit{where.Offset, files.Position(literal.End()).Offset, narrowed})
			}
		}
		if len(edits) == 0 {
			return
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range slices.Backward(edits) {
			source = slices.Concat(source[:e.start], []byte(e.text), source[e.end:])
		}
		if source, err = format.Source(source); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d French texts set apart", path, len(edits))
	})
	// Guards the walk itself: a wrong root would check nothing.
	if texts < 100 {
		t.Fatalf("found %d French texts: the walk misses the module", texts)
	}
}

// frenchStrings are the string literals of the French texts of a file, in
// the order of the source: the second of each i18n.Text literal, which
// holds Languages in order, and those of the "fr" value of each map by
// language.
func frenchStrings(file *ast.File) []*ast.BasicLit {
	var result []*ast.BasicLit
	add := func(expr ast.Expr) {
		ast.Inspect(expr, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				result = append(result, literal)
			}
			return true
		})
	}
	french := func(literal *ast.CompositeLit) {
		if len(literal.Elts) == len(Languages) {
			add(literal.Elts[slices.Index(Languages, "fr")])
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch {
		case isText(literal.Type):
			french(literal)
		case elementIsText(literal.Type):
			for _, element := range literal.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					element = pair.Value
				}
				if inner, ok := element.(*ast.CompositeLit); ok && inner.Type == nil {
					french(inner)
				}
			}
		case byLanguage(literal):
			for _, element := range literal.Elts {
				if pair := element.(*ast.KeyValueExpr); stringValue(pair.Key) == "fr" {
					add(pair.Value)
				}
			}
		}
		return true
	})
	slices.SortFunc(result, func(a, b *ast.BasicLit) int { return int(a.Pos() - b.Pos()) })
	return slices.CompactFunc(result, func(a, b *ast.BasicLit) bool { return a == b })
}

// narrow rewrites the source of an interpreted string literal: a plain,
// no-break or narrow no-break space before : ; ? ! or », or after «, is
// written \u202f, as wide finds them. It counts the spaces it wrote anew.
func narrow(source string) (string, int) {
	if !strings.HasPrefix(source, `"`) {
		// A raw string: only counted, for its caller to say.
		if wide.MatchString(source) {
			return source, 1
		}
		return source, 0
	}
	// The units of the source, each written as an escape or a character,
	// and what they stand for.
	type unit struct {
		source string
		r      rune
	}
	var units []unit
	for rest := source[1 : len(source)-1]; rest != ""; {
		r, _, tail, err := strconv.UnquoteChar(rest, '"')
		if err != nil {
			return source, 0
		}
		units = append(units, unit{rest[:len(rest)-len(tail)], r})
		rest = tail
	}
	isSpace := func(r rune) bool { return r == ' ' || r == '\u00a0' || r == '\u202f' }
	isSign := func(i int) bool {
		switch {
		case i >= len(units):
			return false
		case units[i].r == '»':
			return true
		case !strings.ContainsRune(":;?!", units[i].r):
			return false
		}
		return i+1 == len(units) || !unicode.IsLetter(units[i+1].r) && !unicode.IsDigit(units[i+1].r)
	}
	var b strings.Builder
	b.WriteByte('"')
	count := 0
	for i, u := range units {
		set := isSpace(u.r) && (isSign(i+1) || i > 0 && units[i-1].r == '«')
		if set && u.source != `\u202f` {
			b.WriteString(`\u202f`)
			count++
			continue
		}
		b.WriteString(u.source)
	}
	b.WriteByte('"')
	return b.String(), count
}
