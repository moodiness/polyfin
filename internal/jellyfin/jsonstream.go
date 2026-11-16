package jellyfin

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// jsonChunk is how much of a streamed answer is encoded before it is
// written: the response's compression (see server.compress) takes it as
// it comes, instead of the whole answer once encoded.
const jsonChunk = 32 << 10

// jsonChunks are the buffers of streamed answers, reused between them.
var jsonChunks = sync.Pool{New: func() any {
	buffer := make([]byte, 0, 2*jsonChunk)
	return &buffer
}}

// jsonStream writes a JSON answer as it is encoded, a chunk at a time: an
// answer of thousands of items is never held whole, encoded nor compressed.
// Its encoders append to buf what encoding/json would write, byte for byte
// (see the tests), and spill writes it once a chunk is full.
type jsonStream struct {
	w      http.ResponseWriter
	buffer *[]byte
	buf    []byte
	err    error
}

// newJSONStream starts a JSON answer of status on w, with the headers
// writeJSON sends.
func newJSONStream(w http.ResponseWriter, status int) *jsonStream {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	buffer := jsonChunks.Get().(*[]byte)
	return &jsonStream{w: w, buffer: buffer, buf: (*buffer)[:0]}
}

// spill writes what was encoded once it fills a chunk; it reports false
// once a write failed, as when the app stopped waiting: the rest need not
// be encoded.
func (s *jsonStream) spill() bool {
	if len(s.buf) >= jsonChunk {
		s.write()
	}
	return s.err == nil
}

func (s *jsonStream) write() {
	if s.err == nil && len(s.buf) > 0 {
		_, s.err = s.w.Write(s.buf)
	}
	s.buf = s.buf[:0]
}

// close ends the answer with the newline json.Encoder ends writeJSON's
// with, writes the rest, and gives the buffer back.
func (s *jsonStream) close() {
	s.buf = append(s.buf, '\n')
	s.write()
	*s.buffer = s.buf[:0]
	jsonChunks.Put(s.buffer)
	s.buffer, s.buf = nil, nil
}

// appendJSONKey appends a member's name, after a comma unless first, and
// its colon.
func appendJSONKey(b []byte, key string, first bool) []byte {
	if !first {
		b = append(b, ',')
	}
	b = append(b, '"')
	b = append(b, key...)
	return append(b, '"', ':')
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as encoding/json writes a string: HTML's
// <, > and & escaped, invalid UTF-8 replaced by U+FFFD, and U+2028 and
// U+2029 escaped.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '\\', '"':
				b = append(b, '\\', c)
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, "\uFFFD"...)
			i += size
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', '2', '0', '2', hexDigits[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}

// appendJSONFloat appends f as encoding/json writes a float64.
func appendJSONFloat(b []byte, f float64) []byte {
	format := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	b = strconv.AppendFloat(b, f, format, -1, 64)
	if format == 'e' {
		// Like encoding/json, e-09 becomes e-9.
		if n := len(b); n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b
}

// appendJSONTime appends t as Time's MarshalJSON writes it.
func appendJSONTime(b []byte, t time.Time) []byte {
	t = t.UTC()
	b = append(b, '"')
	start := len(b)
	b = t.AppendFormat(b, "2006-01-02T15:04:05.0000000")
	if t.Nanosecond()/100 != 0 {
		for len(b) > start && b[len(b)-1] == '0' {
			b = b[:len(b)-1]
		}
	}
	return append(b, 'Z', '"')
}

// appendJSONValue appends v as encoding/json writes it, for the members
// apps rarely ask for, whose cost does not matter.
func appendJSONValue(b []byte, v any) []byte {
	encoded, err := json.Marshal(v)
	if err != nil {
		return append(b, "null"...)
	}
	return append(b, encoded...)
}
