package torrent

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// errBencode reports bytes that are not bencoded as the protocol needs.
var errBencode = errors.New("malformed bencoded data")

// maxDepth bounds the nesting of what a peer, a tracker or a DHT node
// sends.
const maxDepth = 32

// decode reads one bencoded value at the start of data: an int64, a
// string, a []any or a map[string]any. It returns the bytes left.
func decode(data []byte) (any, []byte, error) {
	return decodeDepth(data, 0)
}

func decodeDepth(data []byte, depth int) (any, []byte, error) {
	if len(data) == 0 || depth > maxDepth {
		return nil, nil, errBencode
	}
	switch c := data[0]; {
	case c == 'i':
		end := bytes.IndexByte(data, 'e')
		if end < 2 {
			return nil, nil, errBencode
		}
		n, err := strconv.ParseInt(string(data[1:end]), 10, 64)
		if err != nil {
			return nil, nil, errBencode
		}
		return n, data[end+1:], nil
	case c >= '0' && c <= '9':
		colon := bytes.IndexByte(data, ':')
		if colon < 1 {
			return nil, nil, errBencode
		}
		n, err := strconv.Atoi(string(data[:colon]))
		if err != nil || n < 0 || n > len(data)-colon-1 {
			return nil, nil, errBencode
		}
		return string(data[colon+1 : colon+1+n]), data[colon+1+n:], nil
	case c == 'l':
		var list []any
		rest := data[1:]
		for {
			if len(rest) == 0 {
				return nil, nil, errBencode
			}
			if rest[0] == 'e' {
				return list, rest[1:], nil
			}
			var item any
			var err error
			if item, rest, err = decodeDepth(rest, depth+1); err != nil {
				return nil, nil, err
			}
			list = append(list, item)
		}
	case c == 'd':
		dict := map[string]any{}
		rest := data[1:]
		for {
			if len(rest) == 0 {
				return nil, nil, errBencode
			}
			if rest[0] == 'e' {
				return dict, rest[1:], nil
			}
			key, after, err := decodeDepth(rest, depth+1)
			if err != nil {
				return nil, nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, nil, errBencode
			}
			var value any
			if value, rest, err = decodeDepth(after, depth+1); err != nil {
				return nil, nil, err
			}
			dict[name] = value
		}
	}
	return nil, nil, errBencode
}

// decodeDict reads data as one bencoded dictionary, and returns the bytes
// that follow it.
func decodeDict(data []byte) (map[string]any, []byte, error) {
	value, rest, err := decode(data)
	if err != nil {
		return nil, nil, err
	}
	dict, ok := value.(map[string]any)
	if !ok {
		return nil, nil, errBencode
	}
	return dict, rest, nil
}

// encode bencodes a value made of integers, strings, byte slices, lists
// and dictionaries with string keys, sorted as the format requires.
func encode(value any) []byte {
	var b bytes.Buffer
	encodeTo(&b, value)
	return b.Bytes()
}

func encodeTo(b *bytes.Buffer, value any) {
	switch v := value.(type) {
	case int:
		fmt.Fprintf(b, "i%de", v)
	case int64:
		fmt.Fprintf(b, "i%de", v)
	case string:
		fmt.Fprintf(b, "%d:%s", len(v), v)
	case []byte:
		fmt.Fprintf(b, "%d:", len(v))
		b.Write(v)
	case []any:
		b.WriteByte('l')
		for _, item := range v {
			encodeTo(b, item)
		}
		b.WriteByte('e')
	case []string:
		b.WriteByte('l')
		for _, item := range v {
			encodeTo(b, item)
		}
		b.WriteByte('e')
	case map[string]any:
		b.WriteByte('d')
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			encodeTo(b, key)
			encodeTo(b, v[key])
		}
		b.WriteByte('e')
	default:
		panic(fmt.Sprintf("torrent: cannot bencode %T", value))
	}
}

// str, num, list and dict read a dictionary's entry as the type expected,
// the zero value when it is missing or of another type.
func str(d map[string]any, key string) string {
	s, _ := d[key].(string)
	return s
}

func num(d map[string]any, key string) (int64, bool) {
	n, ok := d[key].(int64)
	return n, ok
}

func list(d map[string]any, key string) []any {
	l, _ := d[key].([]any)
	return l
}

func dict(d map[string]any, key string) map[string]any {
	m, _ := d[key].(map[string]any)
	return m
}
