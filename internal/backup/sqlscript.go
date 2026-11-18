package backup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// pg_restore writes the data of a backup as an SQL script: settings, the
// rows of each table as a COPY block, in COPY's text format up to a line
// `\.`, and the value of each sequence. Polyfin reads the script, and
// sends none of its lines to the database: it tells what each line asks
// for, and a restore then issues statements of its own (see load). A
// line that is none of those pg_restore writes for Polyfin's tables
// fails the script.

// scriptName is a name as pg_dump writes it: as it is when it is made of
// lower-case letters, digits and underscores, quoted otherwise.
const scriptName = `(?:[a-z_][a-z0-9_]*|"(?:[^"']|"")+")`

var (
	// copyHeader is the line starting a COPY block: the table, in public,
	// and its columns.
	copyHeader = regexp.MustCompile(`^COPY public\.(` + scriptName + `) \((.+)\) FROM stdin;$`)
	firstName  = regexp.MustCompile(`^` + scriptName)
	// sequenceValue is the line setting a sequence of public: its value,
	// and whether a row took it already.
	sequenceValue = regexp.MustCompile(`^SELECT pg_catalog\.setval\('public\.(` + scriptName + `)', (-?[0-9]{1,19}), (true|false)\);$`)
	// setting is a line setting a parameter of the session the script is
	// written for.
	setting = regexp.MustCompile(`^SET ([a-z_]+) = ('[^']*'|[A-Za-z0-9_]+);$`)
)

// emptySearchPath is the line emptying the search path of the session
// the script is written for.
const emptySearchPath = `SELECT pg_catalog.set_config('search_path', '', false);`

// errScript reports a script pg_restore would not write.
var errScript = errors.New("pg_restore wrote a script Polyfin does not read")

// script reads a script: what each of its lines asks for, then the rows
// of each COPY block.
type script struct {
	r    *bufio.Reader
	done bool
}

func newScript(r io.Reader) *script {
	return &script{r: bufio.NewReaderSize(r, 1<<20)}
}

// The kinds of entry: a COPY block, whose rows follow; the value of a
// sequence; the encoding of the rows.
const (
	scriptEnd = iota
	scriptCopy
	scriptSequence
	scriptEncoding
)

// entry is what a script asks for next: nothing more at its end.
type entry struct {
	kind int
	// name is the table of a COPY block or the sequence, without schema
	// nor quotes, or the encoding.
	name string
	// columns are those of a COPY block.
	columns []string
	// value and called are the value of a sequence.
	value  int64
	called bool
}

// next returns the next entry of the script. Comments, the other
// settings, the search path and psql's \restrict and \unrestrict lines,
// which guard psql against a doctored script, are passed over: Polyfin
// runs none of them.
func (s *script) next() (entry, error) {
	for !s.done {
		line, err := s.r.ReadString('\n')
		if errors.Is(err, io.EOF) {
			s.done = true
		} else if err != nil {
			return entry{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, "--") || line == emptySearchPath ||
			strings.HasPrefix(line, `\restrict `) || strings.HasPrefix(line, `\unrestrict `) {
			continue
		}
		if match := setting.FindStringSubmatch(line); match != nil {
			if match[1] == "client_encoding" {
				return entry{kind: scriptEncoding, name: strings.Trim(match[2], "'")}, nil
			}
			continue
		}
		if match := copyHeader.FindStringSubmatch(line); match != nil {
			if columns, ok := scriptNames(match[2]); ok {
				return entry{kind: scriptCopy, name: unquoteName(match[1]), columns: columns}, nil
			}
		}
		if match := sequenceValue.FindStringSubmatch(line); match != nil {
			if value, err := strconv.ParseInt(match[2], 10, 64); err == nil {
				return entry{kind: scriptSequence, name: unquoteName(match[1]), value: value, called: match[3] == "true"}, nil
			}
		}
		return entry{}, fmt.Errorf("%w: %.40q", errScript, line)
	}
	return entry{}, nil
}

// scriptNames reads a list of names, as a COPY block's header separates
// its columns.
func scriptNames(list string) ([]string, bool) {
	var names []string
	for {
		name := firstName.FindString(list)
		if name == "" {
			return nil, false
		}
		names = append(names, unquoteName(name))
		if list = list[len(name):]; list == "" {
			return names, true
		}
		var more bool
		if list, more = strings.CutPrefix(list, ", "); !more {
			return nil, false
		}
	}
}

// unquoteName reads a name scriptName matched.
func unquoteName(name string) string {
	if unquoted, ok := strings.CutPrefix(name, `"`); ok {
		return strings.ReplaceAll(strings.TrimSuffix(unquoted, `"`), `""`, `"`)
	}
	return name
}

// copyData is the data of the COPY block being read, as COPY reads it: its
// lines up to `\.`, which it leaves out.
type copyData struct {
	s    *script
	line []byte
	end  bool
}

func (c *copyData) Read(p []byte) (int, error) {
	for len(c.line) == 0 {
		if c.end {
			return 0, io.EOF
		}
		line, err := c.s.r.ReadBytes('\n')
		if err != nil {
			return 0, fmt.Errorf("%w: a COPY block ends early", errScript)
		}
		if string(line) == "\\.\n" || string(line) == "\\.\r\n" {
			c.end = true
			return 0, io.EOF
		}
		c.line = line
	}
	n := copy(p, c.line)
	c.line = c.line[n:]
	return n, nil
}

// eachRow calls row with each row of every COPY block of the script, its
// table without schema, and its values by column: NULL is absent.
func (s *script) eachRow(row func(table string, values map[string]string) error) error {
	for {
		block, err := s.next()
		if err != nil || block.kind == scriptEnd {
			return err
		}
		if block.kind != scriptCopy {
			continue
		}
		name, columns := block.name, block.columns
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				return fmt.Errorf("%w: a COPY block ends early", errScript)
			}
			line = strings.TrimRight(line, "\r\n")
			if line == `\.` {
				break
			}
			fields := strings.Split(line, "\t")
			if len(fields) != len(columns) {
				return fmt.Errorf("%w: a row of %s has %d values for %d columns", errScript, name, len(fields), len(columns))
			}
			values := make(map[string]string, len(columns))
			for i, field := range fields {
				if field == `\N` {
					continue
				}
				values[columns[i]] = unescapeCopy(field)
			}
			if err := row(name, values); err != nil {
				return err
			}
		}
	}
}

// unescapeCopy reads a value of COPY's text format.
func unescapeCopy(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		c := field[i]
		if c != '\\' || i+1 == len(field) {
			b.WriteByte(c)
			continue
		}
		i++
		switch next := field[i]; next {
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case 'x':
			value, n := 0, 0
			for n < 2 && i+1 < len(field) && isHex(field[i+1]) {
				i++
				value = value*16 + hexValue(field[i])
				n++
			}
			if n == 0 {
				b.WriteByte('x')
			} else {
				b.WriteByte(byte(value))
			}
		default:
			if '0' <= next && next <= '7' {
				value := int(next - '0')
				for n := 1; n < 3 && i+1 < len(field) && '0' <= field[i+1] && field[i+1] <= '7'; n++ {
					i++
					value = value*8 + int(field[i]-'0')
				}
				b.WriteByte(byte(value))
			} else {
				b.WriteByte(next)
			}
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func hexValue(c byte) int {
	switch {
	case c <= '9':
		return int(c - '0')
	case c >= 'a':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}
