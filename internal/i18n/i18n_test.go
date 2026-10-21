package i18n

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]string{
		"en": "en", "en-US": "en", "fr": "fr", "fr_CA.UTF-8": "fr", "FR-be": "fr", "de-AT": "de",
		"de_CH.UTF-8": "de", "es-419": "es", "es_MX": "es", "it-IT": "it", "pt": "pt-BR", "pt-BR": "pt-BR",
		"pt_PT.UTF-8": "pt-BR", "nl-NL": "", "": "", "C": "", "zh-Hans": "",
	} {
		if got := Match(tag); got != want {
			t.Errorf("Match(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestIn(t *testing.T) {
	t.Parallel()
	text := Text{"en", "fr", "de", "es", "it", "pt"}
	for i, code := range Languages {
		if got := text.In(code); got != text[i] {
			t.Errorf("In(%q) = %q, want %q", code, got, text[i])
		}
	}
	if got := text.In("nl"); got != "en" {
		t.Errorf(`In("nl") = %q, want English`, got)
	}
	if len(Languages) != len(Text{}) {
		t.Fatalf("Text holds %d languages, Languages lists %d", len(Text{}), len(Languages))
	}
}

// TestTexts reads every Go file of the module but tests, and checks each
// text Polyfin writes in several languages: every i18n.Text literal, and
// every map keyed by language (a map literal with a "fr" key), such as the
// tray texts of Polyfin Desktop. Each must hold every language, none
// empty, with the placeholders and the variables of the English.
func TestTexts(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	texts, maps := 0, 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); path != root && (strings.HasPrefix(name, ".") || name == "node_modules" ||
				name == "testdata" || name == "web") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(files, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			where := files.Position(literal.Pos()).String()
			switch {
			case isText(literal.Type):
				texts++
				checkText(t, files, where, literal)
			case elementIsText(literal.Type):
				for _, element := range literal.Elts {
					if pair, ok := element.(*ast.KeyValueExpr); ok {
						element = pair.Value
					}
					if inner, ok := element.(*ast.CompositeLit); ok && inner.Type == nil {
						texts++
						checkText(t, files, files.Position(inner.Pos()).String(), inner)
					}
				}
			case byLanguage(literal):
				maps++
				checkLanguages(t, files, where, literal)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Guards the walk itself: a wrong root would check nothing.
	if texts < 100 || maps < 3 {
		t.Fatalf("found %d texts and %d maps by language: the walk misses the module", texts, maps)
	}
}

// isText reports whether a type expression is i18n.Text.
func isText(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "i18n" && selector.Sel.Name == "Text"
}

// elementIsText reports whether a type expression is a map, slice or array
// of i18n.Text, whose literals may leave out the type of their elements.
func elementIsText(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.MapType:
		return isText(typ.Value)
	case *ast.ArrayType:
		return isText(typ.Elt)
	}
	return false
}

// byLanguage reports whether a literal is a map keyed by language: its keys
// include "en" and "fr", and are all among Languages (a map that also has
// "nl" serves another purpose).
func byLanguage(literal *ast.CompositeLit) bool {
	if _, ok := literal.Type.(*ast.MapType); !ok {
		return false
	}
	var keys []string
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key := stringValue(pair.Key)
		if !slices.Contains(Languages, key) {
			return false
		}
		keys = append(keys, key)
	}
	return slices.Contains(keys, "en") && slices.Contains(keys, "fr")
}

func checkText(t *testing.T, files *token.FileSet, where string, literal *ast.CompositeLit) {
	t.Helper()
	if len(literal.Elts) != len(Languages) {
		t.Errorf("%s: %d languages, want %d (%s)", where, len(literal.Elts), len(Languages), strings.Join(Languages, ", "))
		return
	}
	english := shape(files, literal.Elts[0])
	for i, element := range literal.Elts {
		if _, ok := element.(*ast.KeyValueExpr); ok {
			t.Errorf("%s: an i18n.Text lists its languages in order, without keys", where)
			return
		}
		got := shape(files, element)
		if got.empty {
			t.Errorf("%s: empty in %s", where, Languages[i])
		}
		if !slices.Equal(got.parts, english.parts) {
			t.Errorf("%s: %s has %q, English %q", where, Languages[i], got.parts, english.parts)
		}
	}
}

func checkLanguages(t *testing.T, files *token.FileSet, where string, literal *ast.CompositeLit) {
	t.Helper()
	values := map[string]ast.Expr{}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		values[stringValue(pair.Key)] = pair.Value
	}
	if len(values) != len(Languages) {
		t.Errorf("%s: %d languages, want %d (%s)", where, len(values), len(Languages), strings.Join(Languages, ", "))
	}
	english := fields(files, values["en"])
	for _, code := range Languages {
		value, ok := values[code]
		if !ok {
			t.Errorf("%s: no %s", where, code)
			continue
		}
		got := fields(files, value)
		for name, want := range english {
			have, ok := got[name]
			switch {
			case !ok:
				t.Errorf("%s: %s lacks %s", where, code, name)
			case have.empty:
				t.Errorf("%s: %s is empty in %s", where, name, code)
			case !slices.Equal(have.parts, want.parts):
				t.Errorf("%s: %s has %q in %s, English %q", where, name, have.parts, code, want.parts)
			}
		}
		if len(got) != len(english) {
			t.Errorf("%s: %s has %d texts, English %d", where, code, len(got), len(english))
		}
	}
}

// fields are the texts of one language's value in a map by language: the
// value itself, or each field or entry of a literal, by key ("Name",
// `"resume"`), those of a nested literal by their path (`refusals."off"`).
func fields(files *token.FileSet, value ast.Expr) map[string]textShape {
	result := map[string]textShape{}
	var walk func(path string, value ast.Expr)
	walk = func(path string, value ast.Expr) {
		literal, ok := value.(*ast.CompositeLit)
		if !ok {
			result[path] = shape(files, value)
			return
		}
		for i, element := range literal.Elts {
			key := strconv.Itoa(i)
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				key, element = source(files, pair.Key), pair.Value
			}
			if path != "" {
				key = path + "." + key
			}
			walk(key, element)
		}
	}
	walk("", value)
	return result
}

// textShape is what the languages of a text must share: its placeholders
// and the variables it is built with, sorted; and whether it is empty.
type textShape struct {
	parts []string
	empty bool
}

var placeholder = regexp.MustCompile(`%(?:\[\d+\])?[-+# 0]*\d*(?:\.\d+)?([a-zA-Z%])`)

// shape reads a text: string literals, joined with + to variables, calls
// and the like, which must appear in each language.
func shape(files *token.FileSet, expr ast.Expr) textShape {
	var parts []string
	empty := true
	var walk func(ast.Expr)
	walk = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			walk(e.X)
		case *ast.BinaryExpr:
			if e.Op == token.ADD {
				walk(e.X)
				walk(e.Y)
				return
			}
			parts = append(parts, source(files, e))
			empty = false
		case *ast.BasicLit:
			text := stringValue(e)
			if strings.TrimSpace(text) != "" {
				empty = false
			}
			if e.Kind != token.STRING {
				parts = append(parts, e.Value)
				return
			}
			for _, match := range placeholder.FindAllStringSubmatch(text, -1) {
				if match[1] != "%" {
					parts = append(parts, "%"+match[1])
				}
			}
		default:
			parts = append(parts, source(files, e))
			empty = false
		}
	}
	walk(expr)
	slices.Sort(parts)
	return textShape{parts, empty}
}

func stringValue(expr ast.Expr) string {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return ""
	}
	return value
}

func source(files *token.FileSet, node ast.Node) string {
	var b strings.Builder
	_ = printer.Fprint(&b, files, node)
	return b.String()
}
