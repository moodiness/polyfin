package localfiles

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// The SFTP packets a listing sends and reads, in the protocol's version
// 3, the one servers speak.
const (
	sftpInit    = 1
	sftpVersion = 2
	sftpClose   = 4
	sftpOpendir = 11
	sftpReaddir = 12
	sftpStatus  = 101
	sftpHandle  = 102
	sftpName    = 104
)

// The codes of the status a server answers with: the end of a listing,
// a path it does not know, one it refuses.
const (
	sftpEOF        = 1
	sftpNoSuchFile = 2
	sftpDenied     = 3
)

// The attributes of an entry its flags say it holds, and the bits of its
// permissions telling its type.
const (
	sftpAttrSize     = 0x1
	sftpAttrOwner    = 0x2
	sftpAttrMode     = 0x4
	sftpAttrTimes    = 0x8
	sftpAttrExtended = 0x80000000

	sftpTypeMask = 0o170000
	sftpTypeDir  = 0o040000
	sftpTypeFile = 0o100000
	sftpTypeLink = 0o120000
)

// sftpMaxPacket bounds one packet a listing reads, as SFTP clients and
// servers bound theirs.
const sftpMaxPacket = 256 << 10

// sftpLister lists the folders of an SFTP server over a channel of the
// connection of its own, beside the one files are read over. The client
// reading files gives a folder's entries only once it read all of them;
// here they are read an answer at a time, each entry counted against the
// bounds of a listing as it comes, and the answers' bytes too.
type sftpLister struct {
	// client is the connection's client the lister was opened beside.
	client  *sftp.Client
	session *ssh.Session
	in      io.Writer
	out     io.Reader
	id      uint32
	// packet holds the packet read last; read counts the bytes of the
	// answers of the listing under way.
	packet []byte
	read   int64
}

// sftpListEntry is an entry of a folder as a server lists it: its type
// bits (see sftpTypeMask), none when it does not tell them.
type sftpListEntry struct {
	name     string
	kind     uint32
	size     int64
	modified time.Time
}

// openSFTPLister opens a channel of conn for listings, beside client's.
func openSFTPLister(conn *ssh.Client, client *sftp.Client) (*sftpLister, error) {
	session, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	l := &sftpLister{client: client, session: session}
	if l.in, err = session.StdinPipe(); err == nil {
		if l.out, err = session.StdoutPipe(); err == nil {
			err = session.RequestSubsystem("sftp")
		}
	}
	if err == nil {
		err = l.send(binary.BigEndian.AppendUint32([]byte{sftpInit}, 3))
	}
	if err == nil {
		var kind byte
		kind, _, err = l.receive(listBounds{answer: 4 + sftpMaxPacket})
		if err == nil && kind != sftpVersion {
			err = fmt.Errorf("the server answered the SFTP version with a packet of type %d", kind)
		}
	}
	if err != nil {
		session.Close()
		return nil, err
	}
	return l, nil
}

func (l *sftpLister) close() { l.session.Close() }

// send sends a packet, body being its type and what follows.
func (l *sftpLister) send(body []byte) error {
	_, err := l.in.Write(append(binary.BigEndian.AppendUint32(nil, uint32(len(body))), body...))
	return err
}

// request sends a request of kind about the text s, under an id of its
// own, which it returns.
func (l *sftpLister) request(kind byte, s string) (uint32, error) {
	l.id++
	body := binary.BigEndian.AppendUint32([]byte{kind}, l.id)
	body = binary.BigEndian.AppendUint32(body, uint32(len(s)))
	return l.id, l.send(append(body, s...))
}

// receive reads a packet within the bytes left of the answer listing a
// folder: its type, and what follows, valid until the next one. A channel
// that ends is io.ErrUnexpectedEOF, never the end of a listing.
func (l *sftpLister) receive(b listBounds) (byte, []byte, error) {
	var head [4]byte
	if _, err := io.ReadFull(l.out, head[:]); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return 0, nil, err
	}
	size := int64(binary.BigEndian.Uint32(head[:]))
	switch {
	case size == 0 || size > sftpMaxPacket:
		return 0, nil, fmt.Errorf("an SFTP packet of %d bytes", size)
	case l.read+4+size > b.answer:
		return 0, nil, &shareBound{failure: errBoundListing, detail: fmt.Sprintf("an answer of more than %d bytes", b.answer)}
	}
	l.read += 4 + size
	if int64(cap(l.packet)) < size {
		l.packet = make([]byte, size)
	}
	l.packet = l.packet[:size]
	if _, err := io.ReadFull(l.out, l.packet); err != nil {
		return 0, nil, err
	}
	return l.packet[0], l.packet[1:], nil
}

// answer reads the answer to the request id: its type and what follows
// the id.
func (l *sftpLister) answer(id uint32, b listBounds) (byte, []byte, error) {
	kind, body, err := l.receive(b)
	if err != nil {
		return 0, nil, err
	}
	answered, body, ok := sftpUint32(body)
	if !ok || answered != id {
		return 0, nil, fmt.Errorf("the SFTP server answered request %d with a packet for another one", id)
	}
	if kind == sftpStatus {
		return kind, nil, sftpStatusError(body)
	}
	return kind, body, nil
}

// list lists the folder at dir, giving each entry to each in turn, the
// listing read within b. A failure other than a status the server
// answered leaves the lister's channel unfit for another listing.
func (l *sftpLister) list(dir string, b listBounds, each func(entry sftpListEntry) error) error {
	l.read = 0
	id, err := l.request(sftpOpendir, dir)
	if err != nil {
		return err
	}
	kind, body, err := l.answer(id, b)
	if err != nil {
		return err
	}
	handle, _, ok := sftpString(body)
	if kind != sftpHandle || !ok {
		return errors.New("the SFTP server answered the opening of a folder without a handle")
	}
	// The handle is copied out of the packet, read again for each answer.
	h := string(handle)
	for {
		id, err := l.request(sftpReaddir, h)
		if err != nil {
			return err
		}
		kind, body, err := l.answer(id, b)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if kind != sftpName {
			return fmt.Errorf("the SFTP server answered a listing with a packet of type %d", kind)
		}
		if err := sftpEntries(body, each); err != nil {
			return err
		}
	}
	id, err = l.request(sftpClose, h)
	if err != nil {
		return err
	}
	_, _, err = l.answer(id, b)
	return err
}

// sftpEntries gives each the entries of an answer to a listing.
func sftpEntries(body []byte, each func(entry sftpListEntry) error) error {
	broken := errors.New("a broken SFTP listing")
	count, body, ok := sftpUint32(body)
	if !ok {
		return broken
	}
	for range count {
		var name []byte
		var entry sftpListEntry
		if name, body, ok = sftpString(body); !ok {
			return broken
		}
		// The long name, as ls writes the entry, is not read.
		if _, body, ok = sftpString(body); !ok {
			return broken
		}
		if body, ok = entry.attributes(body); !ok {
			return broken
		}
		if i := bytes.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		entry.name = string(name)
		if err := each(entry); err != nil {
			return err
		}
	}
	return nil
}

// attributes reads the attributes of an entry, from b on, and returns
// what follows them.
func (e *sftpListEntry) attributes(b []byte) ([]byte, bool) {
	flags, b, ok := sftpUint32(b)
	if ok && flags&sftpAttrSize != 0 {
		var size uint64
		if size, b, ok = sftpUint64(b); ok {
			e.size = int64(size)
		}
	}
	if ok && flags&sftpAttrOwner != 0 {
		_, b, ok = sftpUint64(b)
	}
	if ok && flags&sftpAttrMode != 0 {
		var mode uint32
		if mode, b, ok = sftpUint32(b); ok {
			e.kind = mode & sftpTypeMask
		}
	}
	if ok && flags&sftpAttrTimes != 0 {
		var times uint64
		if times, b, ok = sftpUint64(b); ok {
			e.modified = time.Unix(int64(uint32(times)), 0)
		}
	}
	if ok && flags&sftpAttrExtended != 0 {
		var count uint32
		count, b, ok = sftpUint32(b)
		for i := uint32(0); ok && i < 2*count; i++ {
			_, b, ok = sftpString(b)
		}
	}
	return b, ok
}

// sftpStatusError is the error a status answers, from its code on: nil
// for none, io.EOF at the end of a listing.
func sftpStatusError(body []byte) error {
	code, _, ok := sftpUint32(body)
	switch {
	case !ok:
		return errors.New("a broken SFTP status")
	case code == 0:
		return nil
	case code == sftpEOF:
		return io.EOF
	case code == sftpNoSuchFile:
		return fs.ErrNotExist
	case code == sftpDenied:
		return fs.ErrPermission
	}
	return &sftp.StatusError{Code: code}
}

func sftpUint32(b []byte) (uint32, []byte, bool) {
	if len(b) < 4 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint32(b), b[4:], true
}

func sftpUint64(b []byte) (uint64, []byte, bool) {
	if len(b) < 8 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint64(b), b[8:], true
}

func sftpString(b []byte) ([]byte, []byte, bool) {
	size, b, ok := sftpUint32(b)
	if !ok || uint64(len(b)) < uint64(size) {
		return nil, nil, false
	}
	return b[:size], b[size:], true
}
