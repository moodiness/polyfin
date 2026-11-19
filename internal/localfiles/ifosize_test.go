package localfiles

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// A DVD's IFO file on a share is read whole up to 4 MiB, several times
// the largest real one; a larger one is not a DVD's, and is read no
// further than the bound.
func TestShareIFOFilesAreBounded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		size int
		read bool
	}{{size: 300 << 10, read: true}, {size: 4 << 20, read: true}, {size: 4<<20 + 1}, {size: 16 << 20}} {
		data := content(c.size)
		var served int64
		list := listing{open: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(&countingReader{r: bytes.NewReader(data), n: &served}), nil
		}}
		at, err := readIFO(t.Context(), list, "Movie/VIDEO_TS/VTS_01_0.IFO")
		if !c.read {
			if !errors.Is(err, errNotDVD) {
				t.Errorf("an IFO file of %d bytes: %v, want it refused", c.size, err)
			}
			if served > 4<<20+1 {
				t.Errorf("an IFO file of %d bytes: %d bytes read, want at most the bound and one", c.size, served)
			}
			continue
		}
		if err != nil {
			t.Errorf("an IFO file of %d bytes: %v", c.size, err)
			continue
		}
		tail := make([]byte, 16)
		if n, err := at.ReadAt(tail, int64(c.size-len(tail))); err != nil && err != io.EOF || n != len(tail) ||
			!bytes.Equal(tail, data[c.size-len(tail):]) {
			t.Errorf("an IFO file of %d bytes reads %d bytes at its end, %v", c.size, n, err)
		}
	}
}

// countingReader counts the bytes read of r in n.
type countingReader struct {
	r io.Reader
	n *int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}
