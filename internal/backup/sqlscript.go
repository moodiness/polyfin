package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// pg_restore writes a backup as an SQL script, which Polyfin runs itself
// in one transaction: its statements, and the data of each table as a COPY
// block, its rows in COPY's text format up to a line `\.`.

// copyHeader is the line starting a COPY block: the table, schema
// qualified, and its columns.
var copyHeader = regexp.MustCompile(`^COPY ([^ ]+) \((.*)\) FROM stdin;$`)

// errScript reports a script pg_restore would not write.
var errScript = errors.New("pg_restore wrote a script Polyfin does not read")

// script reads a script: statements between COPY blocks, then each block.
type script struct {
	r    *bufio.Reader
	done bool
}

func newScript(r io.Reader) *script {
	return &script{r: bufio.NewReaderSize(r, 1<<20)}
}

// statements returns the statements up to the next COPY block or the end,
// and the block's header, empty at the end. psql's \restrict and
// \unrestrict lines, which guard psql against a doctored script, are left
// out: no psql reads it.
func (s *script) statements() (sql, header string, err error) {
	var b strings.Builder
	for !s.done {
		line, err := s.r.ReadString('\n')
		if errors.Is(err, io.EOF) {
			s.done = true
			if line == "" {
				break
			}
		} else if err != nil {
			return "", "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if copyHeader.MatchString(trimmed) {
			return b.String(), trimmed, nil
		}
		if strings.HasPrefix(trimmed, `\restrict `) || strings.HasPrefix(trimmed, `\unrestrict `) {
			continue
		}
		if strings.HasPrefix(trimmed, `\`) {
			return "", "", fmt.Errorf("%w: %.40s", errScript, trimmed)
		}
		b.WriteString(line)
	}
	return b.String(), "", nil
}

// copyTarget reads a COPY block's header: its table, without schema, and
// its columns, quoted or not.
func copyTarget(header string) (table string, columns []string) {
	match := copyHeader.FindStringSubmatch(header)
	if _, name, ok := strings.Cut(match[1], "."); ok {
		table = name
	} else {
		table = match[1]
	}
	for column := range strings.SplitSeq(match[2], ", ") {
		if unquoted, ok := strings.CutPrefix(column, `"`); ok {
			column = strings.ReplaceAll(strings.TrimSuffix(unquoted, `"`), `""`, `"`)
		}
		columns = append(columns, column)
	}
	return strings.Trim(table, `"`), columns
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

// drain reads the rest of the block, as when its data is not wanted.
func (c *copyData) drain() error {
	_, err := io.Copy(io.Discard, c)
	return err
}

// run runs the script on conn, a transaction being open on it.
func (s *script) run(ctx context.Context, conn *pgconn.PgConn) error {
	for {
		sql, header, err := s.statements()
		if err != nil {
			return err
		}
		if strings.TrimSpace(sql) != "" {
			if _, err := conn.Exec(ctx, sql).ReadAll(); err != nil {
				return err
			}
		}
		if header == "" {
			return nil
		}
		data := &copyData{s: s}
		if _, err := conn.CopyFrom(ctx, data, strings.TrimSuffix(header, ";")); err != nil {
			table, _ := copyTarget(header)
			return fmt.Errorf("restore %s: %w", table, err)
		}
		if !data.end {
			if err := data.drain(); err != nil {
				return err
			}
		}
	}
}

// eachRow calls row with each row of every COPY block of the script, its
// table without schema, and its values by column: NULL is absent.
func (s *script) eachRow(row func(table string, values map[string]string) error) error {
	for {
		_, header, err := s.statements()
		if err != nil || header == "" {
			return err
		}
		name, columns := copyTarget(header)
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
