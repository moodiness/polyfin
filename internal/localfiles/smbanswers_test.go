package localfiles

import (
	"bytes"
	"context"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/cloudsoda/go-smb2"
)

// The SMB2 commands smbServer answers, and the one it does not.
const (
	smbNegotiate      = 0
	smbSessionSetup   = 1
	smbLogoff         = 2
	smbTreeConnect    = 3
	smbTreeDisconnect = 4
	smbCreate         = 5
	smbClose          = 6
	smbCancel         = 12
	smbChangeNotify   = 15
)

// smbServer is an SMB server a test serves: it signs anyone in as a
// guest, whose answers are not signed, opens any folder, and answers the
// watches of a folder with its answers, one each, then with nothing.
type smbServer struct {
	address string

	mu      sync.Mutex
	answers [][]byte
}

// serveSMB serves an SMB server on the loopback until the test ends.
func serveSMB(t *testing.T, answers ...[]byte) *smbServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smbServer{address: listener.Addr().String(), answers: answers}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var conns []net.Conn
	wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			wg.Go(func() { s.serve(conn) })
		}
	})
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		for _, conn := range conns {
			_ = conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return s
}

// url is the address of a folder of a share of the server.
func (s *smbServer) url() *url.URL {
	return &url.URL{Scheme: "smb", Host: s.address, Path: "/media/movies"}
}

// serve answers the requests of a connection until it ends.
func (s *smbServer) serve(conn net.Conn) {
	defer conn.Close()
	for {
		var size [4]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return
		}
		request := make([]byte, binary.BigEndian.Uint32(size[:]))
		if _, err := io.ReadFull(conn, request); err != nil || len(request) < 64 {
			return
		}
		status, body, ok := s.answer(request)
		if !ok {
			continue
		}
		// The header: the request's command and message, in the one
		// session and the one tree, with credits for the next requests.
		response := make([]byte, 4+64, 4+64+len(body))
		binary.BigEndian.PutUint32(response, uint32(64+len(body)))
		header := response[4:]
		copy(header, "\xfeSMB")
		binary.LittleEndian.PutUint16(header[4:], 64)
		binary.LittleEndian.PutUint16(header[6:], 1)
		binary.LittleEndian.PutUint32(header[8:], status)
		copy(header[12:14], request[12:14])
		binary.LittleEndian.PutUint16(header[14:], 16)
		binary.LittleEndian.PutUint32(header[16:], 1)
		copy(header[24:32], request[24:32])
		binary.LittleEndian.PutUint32(header[36:], 1)
		binary.LittleEndian.PutUint64(header[40:], 1)
		if _, err := conn.Write(append(response, body...)); err != nil {
			return
		}
	}
}

// answer is the status and the body the server answers a request with;
// not ok, it does not answer.
func (s *smbServer) answer(request []byte) (status uint32, body []byte, ok bool) {
	sized := func(size, length int) []byte {
		body := make([]byte, length)
		binary.LittleEndian.PutUint16(body, uint16(size))
		return body
	}
	switch binary.LittleEndian.Uint16(request[12:14]) {
	case smbNegotiate:
		// SMB 2.1, signing not required, no security buffer.
		body := sized(65, 65)
		binary.LittleEndian.PutUint16(body[2:], 1)
		binary.LittleEndian.PutUint16(body[4:], 0x0210)
		for _, at := range []int{28, 32, 36} {
			binary.LittleEndian.PutUint32(body[at:], 1<<20)
		}
		binary.LittleEndian.PutUint16(body[56:], 128)
		return 0, body, true
	case smbSessionSetup:
		// A guest's session, at once: the client still reads an NTLM
		// challenge, here with its own flags and no name.
		challenge := make([]byte, 60)
		copy(challenge, "NTLMSSP\x00")
		binary.LittleEndian.PutUint32(challenge[8:], 2)
		binary.LittleEndian.PutUint32(challenge[16:], 56)
		if at := bytes.Index(request, []byte("NTLMSSP\x00")); at >= 0 && len(request) >= at+16 {
			copy(challenge[20:24], request[at+12:at+16])
		}
		copy(challenge[24:32], "polyfin!")
		binary.LittleEndian.PutUint16(challenge[40:], 4)
		binary.LittleEndian.PutUint16(challenge[42:], 4)
		binary.LittleEndian.PutUint32(challenge[44:], 56)
		token, err := asn1.MarshalWithParams(struct {
			State     asn1.Enumerated       `asn1:"explicit,tag:0"`
			Mechanism asn1.ObjectIdentifier `asn1:"explicit,tag:1"`
			Token     []byte                `asn1:"explicit,tag:2"`
		}{Mechanism: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 2, 10}, Token: challenge}, "explicit,tag:1")
		if err != nil {
			return 0, nil, false
		}
		body := sized(9, 8)
		binary.LittleEndian.PutUint16(body[2:], 1)
		binary.LittleEndian.PutUint16(body[4:], 64+8)
		binary.LittleEndian.PutUint16(body[6:], uint16(len(token)))
		return 0, append(body, token...), true
	case smbTreeConnect:
		body := sized(16, 16)
		body[2] = 1
		return 0, body, true
	case smbCreate:
		body := sized(89, 89)
		copy(body[64:80], "a folder to open")
		return 0, body, true
	case smbClose:
		return 0, sized(60, 60), true
	case smbLogoff, smbTreeDisconnect:
		return 0, sized(4, 4), true
	case smbChangeNotify:
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.answers) == 0 {
			return 0, nil, false
		}
		body := s.answers[0]
		s.answers = s.answers[1:]
		return 0, body, true
	case smbCancel:
		return 0, nil, false
	}
	// STATUS_NOT_SUPPORTED.
	return 0xC00000BB, sized(9, 9), true
}

// notifyAnswer is the body of an answer to a folder's watch that says its
// records are at offset of the message, length long, before buffer.
func notifyAnswer(offset uint16, length uint32, buffer []byte) []byte {
	body := make([]byte, 8+len(buffer))
	binary.LittleEndian.PutUint16(body[0:2], 9)
	binary.LittleEndian.PutUint16(body[2:4], offset)
	binary.LittleEndian.PutUint32(body[4:8], length)
	copy(body[8:], buffer)
	return body
}

// notifyRecords is the body of an answer to a folder's watch holding
// records, as a server sends it.
func notifyRecords(records ...[]byte) []byte {
	buffer := slices.Concat(records...)
	return notifyAnswer(64+8, uint32(len(buffer)), buffer)
}

// notifyRecord is a record of an answer to a folder's watch: a file
// added, the next record said next further, the name said nameLength long.
func notifyRecord(next, nameLength uint32, name []byte) []byte {
	record := make([]byte, 12+len(name))
	binary.LittleEndian.PutUint32(record[0:4], next)
	binary.LittleEndian.PutUint32(record[4:8], smb2.ActionAdded)
	binary.LittleEndian.PutUint32(record[8:12], nameLength)
	copy(record[12:], name)
	return record
}

// notifyName is a name as a record holds it, padded with zeros to size
// bytes.
func notifyName(name string, size int) []byte {
	units := utf16.Encode([]rune(name))
	encoded := make([]byte, max(size, 2*len(units)))
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], unit)
	}
	return encoded
}

// watchSMB watches the server's share until the watch ends, and returns
// its error; changed gets the changes it tells of. The watch is ended by
// what the server answers, never from here while a request is being sent.
func watchSMB(t *testing.T, server *smbServer, changed func(paths []string)) error {
	t.Helper()
	share := newSMB(server.url(), "reader", "pass phrase")
	defer share.close()
	ctx, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	err := share.watch(ctx, notifyHow{armed: func() {}, changed: changed})
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("the watch did not end")
	}
	return err
}

// An answer to a folder's watch whose offsets and lengths do not hold in
// it, or in the buffer asked for, ends the watch as a broken answer does:
// nothing of it is read, and nothing is told.
func TestAnSMBWatchRefusesAnswersWhoseLengthsDoNotHold(t *testing.T) {
	t.Parallel()
	name := notifyName("a.mkv", 0)
	for _, c := range []struct {
		what   string
		answer []byte
		want   string
	}{
		{"records said before the message's own header", notifyAnswer(0, 4, make([]byte, 16)), "broken change notify response format"},
		{"records said inside the answer's own fields", notifyAnswer(64+4, 4, make([]byte, 16)), "broken change notify response format"},
		{"records said longer than 32 bits hold with their offset", notifyAnswer(64+8, 0xFFFFFFF0, make([]byte, 16)),
			"broken change notify response format"},
		{"records said longer than the answer", notifyAnswer(64+8, 17, make([]byte, 16)), "broken change notify response format"},
		{"a name said longer than 32 bits hold with its record", notifyRecords(notifyRecord(0, 0xFFFFFFF8, make([]byte, 4))),
			"broken file notify information format"},
		{"a name said longer than its record", notifyRecords(notifyRecord(0, 12, name)), "broken file notify information format"},
		{"a next record said inside this one's fields", notifyRecords(notifyRecord(8, uint32(len(name)), name)),
			"broken file notify information format"},
		// The name is itself a last record: read apart, it would be told too.
		{"a next record said inside this one's name", notifyRecords(notifyRecord(12, 12, notifyRecord(0, 0, nil))),
			"broken file notify information format"},
		{"a next record said past the records", notifyRecords(notifyRecord(64, uint32(len(name)), name)),
			"broken file notify information format"},
		{"a next record said where the records end", notifyRecords(notifyRecord(uint32(12+len(name)), uint32(len(name)), name)),
			"broken file notify information format"},
		{"more records than the buffer asked for holds", notifyRecords(notifyRecord(0, 32*1024, make([]byte, 32*1024))),
			"longer than the buffer asked for"},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			err := watchSMB(t, serveSMB(t, c.answer), func(paths []string) {
				t.Errorf("the watch told of %d changes", len(paths))
			})
			var ended *notifyPanic
			switch {
			case errors.As(err, &ended):
				t.Fatalf("reading the answer panicked: %s", ended.cause)
			case err == nil || !errors.Is(err, errShareUnreachable) || !strings.Contains(err.Error(), c.want):
				t.Fatalf("the watch ended with %v, want %q", err, c.want)
			}
		})
	}
}

// A well-formed answer tells of each of its records, the last being the
// one that says no next, whatever follows it; so does one that fills the
// buffer asked for. The watch goes on until an answer is refused.
func TestAnSMBWatchTellsOfTheChangesAnAnswerLists(t *testing.T) {
	t.Parallel()
	first, second := notifyName("a.mkv", 12), notifyName(`Shows\b.mkv`, 0)
	long := strings.Repeat("a", (32*1024-12)/2)
	server := serveSMB(t,
		notifyRecords(notifyRecord(uint32(12+len(first)), 10, first), notifyRecord(0, uint32(len(second)), second), make([]byte, 7)),
		notifyRecords(notifyRecord(0, uint32(2*len(long)), notifyName(long, 0))),
		notifyAnswer(64+8, 17, make([]byte, 16)))
	var told [][]string
	err := watchSMB(t, server, func(paths []string) { told = append(told, paths) })
	if err == nil || !strings.Contains(err.Error(), "broken change notify response format") {
		t.Fatalf("the watch ended with %v", err)
	}
	if len(told) != 2 || !slices.Equal(told[0], []string{"a.mkv", "Shows/b.mkv"}) || !slices.Equal(told[1], []string{long}) {
		t.Fatalf("told of %d answers, the first %q", len(told), told[:1])
	}
}

// panickingSMB is a share whose watch is an SMB share's, each answer to
// it read with a panic; ended gets the end of each watch.
type panickingSMB struct {
	remote
	smb   *smbShare
	ended chan error
}

func (p *panickingSMB) splittable() bool { return p.smb.splittable() }

func (p *panickingSMB) watch(ctx context.Context, how notifyHow) error {
	how.notify = func(context.Context, *smb2.File, smb2.NotifyOptions) ([]smb2.FileNotification, error) {
		panic("an answer read out of its bounds")
	}
	err := p.smb.watch(ctx, how)
	p.ended <- err
	return err
}

// logLines is a log kept for a test to read.
type logLines struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (l *logLines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

// with returns the lines holding each of the texts.
func (l *logLines) with(texts ...string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var lines []string
	for line := range strings.Lines(l.text.String()) {
		if !slices.ContainsFunc(texts, func(text string) bool { return !strings.Contains(line, text) }) {
			lines = append(lines, line)
		}
	}
	return lines
}

// A panic of the code reading a server's answer to a folder's watch ends
// the watch and no more: it is logged once with the share and its stack,
// the watch is set up again as after a failed session, and the share is
// polled meanwhile.
func TestAPanicBelowAnSMBWatchLeavesTheSharePolled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	share := newSMB(serveSMB(t).url(), "reader", "pass phrase")
	t.Cleanup(share.close)
	wt, shares, _ := newShareWatchTest(t, dir, func(string) func(context.Context) ([]watchFolder, error) { return one(id, testSMB) })
	watched := &panickingSMB{remote: shares.r, smb: share, ended: make(chan error, 8)}
	shares.use(watched)
	logs := &logLines{}
	wt.w.logger = slog.New(slog.NewTextHandler(logs, nil))
	wt.w.notifiedEvery, wt.w.rearm = time.Hour, time.Hour
	wt.start()

	// The watch that worked is set up again at once, and ends as the
	// first did: the next one waits rearm.
	for range 2 {
		select {
		case err := <-watched.ended:
			var ended *notifyPanic
			if !errors.As(err, &ended) || !errors.Is(err, errShareUnreachable) || !strings.Contains(ended.stack, "smbnotify.go") {
				t.Fatalf("the watch ended with %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("the watch did not end")
		}
	}
	// Set up again, the share was scanned; it is polled since.
	wt.scanned(id)
	wt.watchedAs(id, SharePolled, polledFailed)
	write(t, filepath.Join(dir, "movies", "Metropolis (1927)", "Metropolis (1927).mkv"), 10)
	wt.scanned(id)
	wt.watchedAs(id, SharePolled, polledFailed)

	stacks := logs.with("level=ERROR", "A network share's watch ended on a panic", "address="+testSMB,
		`panic="an answer read out of its bounds"`, "stack=")
	if len(stacks) != 1 || len(logs.with("level=ERROR")) != 1 {
		t.Errorf("%d panics logged with their stack, of %d errors logged, want one", len(stacks), len(logs.with("level=ERROR")))
	}
	if warned := logs.with("level=WARN", "cannot tell of changes for now", "the watch ended on a panic"); len(warned) != 1 {
		t.Errorf("%d warnings of the second watch's end, want one", len(warned))
	}
}
