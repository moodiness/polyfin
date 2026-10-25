package source

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// RandomFile is a file read at offsets, as a file of an SFTP server is
// over its SSH connection.
type RandomFile interface {
	io.ReaderAt
	io.Closer
}

// ProgressFile is a RandomFile whose reads tell, before they return, that
// the bytes they wait for are coming, as a torrent's blocks do while a
// piece is fetched: the cache counts a connection stalled when no byte
// arrives for its stall timeout, rather than when a whole block of it did
// not.
type ProgressFile interface {
	RandomFile
	ReadAtProgress(p []byte, off int64, progress func()) (int, error)
}

// FileOpener reads the sources whose files are not served over HTTP, such
// as those of an SFTP server, for the cache: it answers its requests as a
// server serving ranges does, from the file open opens at the address
// target, with its size. A range is answered 206 with its Content-Range,
// several ranges 200 with the whole file, which the cache then asks for
// one by one, and a range past the end 416. A file that cannot be opened
// answers 404 when it does not exist and 403 when it is refused (see
// fs.ErrNotExist and fs.ErrPermission); open's other errors are the
// request's, as a host not reached.
type FileOpener func(ctx context.Context, target string) (RandomFile, int64, error)

// fileBlock is how much one read of a file asks for: a block, which the
// file's protocol may fetch over several requests at once.
const fileBlock = blockSize

func (o FileOpener) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, size, err := o(ctx, target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return answer(http.StatusNotFound, http.Header{}, 0, http.NoBody), nil
	case errors.Is(err, fs.ErrPermission):
		return answer(http.StatusForbidden, http.Header{}, 0, http.NoBody), nil
	case err != nil:
		return nil, err
	}
	answered := http.Header{}
	contentType := ""
	if u, err := url.Parse(target); err == nil {
		contentType = mime.TypeByExtension(path.Ext(u.Path))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	answered.Set("Content-Type", contentType)
	answered.Set("Accept-Ranges", "bytes")
	start, end, ranged := fileRange(header.Get("Range"), size)
	status := http.StatusOK
	switch {
	case ranged && start >= size:
		file.Close()
		answered.Del("Content-Type")
		answered.Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
		return answer(http.StatusRequestedRangeNotSatisfiable, answered, 0, http.NoBody), nil
	case ranged:
		status = http.StatusPartialContent
		answered.Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end-1, 10)+"/"+strconv.FormatInt(size, 10))
	}
	if method == http.MethodHead {
		file.Close()
		return answer(status, answered, end-start, http.NoBody), nil
	}
	return answer(status, answered, end-start, &fileBody{ctx: ctx, file: file, offset: start, end: end}), nil
}

// fileRange reads the one range a Range header asks of a file of size:
// its start and its end, excluded, and whether it asks for one. Without
// one, or asking for several, it is the whole file.
func fileRange(value string, size int64) (start, end int64, ranged bool) {
	spec, ok := strings.CutPrefix(strings.TrimSpace(value), "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return 0, size, false
	}
	first, last, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, size, false
	}
	if first == "" {
		// The last bytes of the file.
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n <= 0 {
			return 0, size, false
		}
		return max(0, size-n), size, true
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil || start < 0 {
		return 0, size, false
	}
	end = size
	if last != "" {
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n < start {
			return 0, size, false
		}
		end = min(size, n+1)
	}
	return start, max(start, end), true
}

func answer(status int, header http.Header, length int64, body io.ReadCloser) *http.Response {
	return &http.Response{Status: strconv.Itoa(status) + " " + http.StatusText(status), StatusCode: status, Proto: "HTTP/1.1", ProtoMajor: 1,
		ProtoMinor: 1, Header: header, ContentLength: length, Body: body}
}

// fileBody is a file's bytes from offset to end, read a block at a time.
type fileBody struct {
	ctx    context.Context
	file   RandomFile
	offset int64
	end    int64
	buffer []byte
	// pending is what the last read gave and Read did not yet.
	pending []byte
	// progress, when set, is called as a ProgressFile's read is fed.
	progress func()
}

// onProgress has progress called as the reads of a ProgressFile are fed
// (see watched), nil for none.
func (b *fileBody) onProgress(progress func()) {
	b.progress = progress
}

func (b *fileBody) Read(p []byte) (int, error) {
	if len(b.pending) == 0 {
		if b.offset >= b.end {
			return 0, io.EOF
		}
		if err := b.ctx.Err(); err != nil {
			return 0, err
		}
		if b.buffer == nil {
			b.buffer = make([]byte, fileBlock)
		}
		want := b.buffer[:min(int64(len(b.buffer)), b.end-b.offset)]
		var n int
		var err error
		if file, ok := b.file.(ProgressFile); ok && b.progress != nil {
			n, err = file.ReadAtProgress(want, b.offset, b.progress)
		} else {
			n, err = b.file.ReadAt(want, b.offset)
		}
		b.offset += int64(n)
		b.pending = want[:n]
		if n < len(want) {
			if err == nil || errors.Is(err, io.EOF) {
				// The file is shorter than it was when opened.
				err = io.ErrUnexpectedEOF
			}
			if n == 0 {
				return 0, err
			}
			b.end = b.offset
		}
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

func (b *fileBody) Close() error {
	return b.file.Close()
}
