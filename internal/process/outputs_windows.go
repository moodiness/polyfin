package process

import (
	"io"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// outputs are the listeners on the loopback the child connects to, one an
// output, in order.
type outputs struct {
	loopbacks []*loopback
}

func (o *Outputs) open(cmd *exec.Cmd, n int) error {
	for i := range n {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		l := &loopback{listener: listener, accepted: make(chan struct{})}
		o.loopbacks = append(o.loopbacks, l)
		o.Readers = append(o.Readers, l)
		// FFmpeg writes to tcp://, as it would to pipe:N.
		address := "tcp://" + listener.Addr().String()
		for j, arg := range cmd.Args {
			if j > 0 && arg == pipeName(i) {
				cmd.Args[j] = address
			}
		}
	}
	return nil
}

// pipeName is how a child names its output i, from 0: pipe:3 on.
func pipeName(i int) string {
	return "pipe:" + strconv.Itoa(3+i)
}

func (o *Outputs) started(cmd *exec.Cmd, err error) {
	if err != nil || len(o.loopbacks) == 0 {
		return
	}
	// The handle keeps the child's identifier from being given to another
	// process until every output has taken its connection or ended.
	pid := uint32(cmd.Process.Pid)
	child, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	var accepting sync.WaitGroup
	for _, l := range o.loopbacks {
		accepting.Go(func() { l.accept(pid) })
	}
	// An output the child never connected to ends with the child, as a
	// pipe would.
	go func() {
		if openErr == nil {
			_, _ = windows.WaitForSingleObject(child, windows.INFINITE)
		}
		for _, l := range o.loopbacks {
			_ = l.listener.Close()
		}
		accepting.Wait()
		if openErr == nil {
			_ = windows.CloseHandle(child)
		}
	}()
}

// loopback is an output of a child on the loopback: it reads the first
// connection its listener accepts from the child, and ends without one
// once the listener is closed.
type loopback struct {
	listener net.Listener
	// accepted is closed once the listener accepted its connection, or
	// will accept none.
	accepted chan struct{}
	ended    sync.Once

	mu     sync.Mutex
	conn   net.Conn
	closed bool
}

// accept takes the child's connection, the only one: a connection made by
// any other process is closed unread, and the listener waits on.
func (l *loopback) accept(child uint32) {
	defer l.end()
	defer l.listener.Close()
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			return
		}
		if pid, ok := connectedBy(conn); ok && pid == child {
			l.take(conn)
			return
		}
		_ = conn.Close()
	}
}

// take keeps conn as the output, unless the output was closed meanwhile.
func (l *loopback) take(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		_ = conn.Close()
		return
	}
	l.conn = conn
}

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	getExtendedTCPTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

// tcpTableOwnerPIDConnections is TCP_TABLE_OWNER_PID_CONNECTIONS: the
// IPv4 connections, without the listeners, each with the process that
// made it.
const tcpTableOwnerPIDConnections = 4

// connectedBy tells the process that made conn, a connection accepted on
// the loopback, from the system's table of connections: the row whose
// local end is conn's remote one and the other way round. False when no
// row is found.
func connectedBy(conn net.Conn) (uint32, bool) {
	local, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		return 0, false
	}
	remote, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return 0, false
	}
	ours, theirs := local.AddrPort(), remote.AddrPort()
	ours = netip.AddrPortFrom(ours.Addr().Unmap(), ours.Port())
	theirs = netip.AddrPortFrom(theirs.Addr().Unmap(), theirs.Port())
	// MIB_TCPTABLE_OWNER_PID: the number of rows, then the rows, six
	// DWORDs each (MIB_TCPROW_OWNER_PID): state, local address, local
	// port, remote address, remote port, process.
	table := make([]uint32, 1024)
	for {
		size := uint32(len(table) * 4)
		r, _, _ := getExtendedTCPTable.Call(uintptr(unsafe.Pointer(&table[0])), uintptr(unsafe.Pointer(&size)), 0,
			windows.AF_INET, tcpTableOwnerPIDConnections, 0)
		if r == 0 {
			break
		}
		if windows.Errno(r) != windows.ERROR_INSUFFICIENT_BUFFER {
			return 0, false
		}
		// Room for the connections made since.
		table = make([]uint32, (size+size/4)/4+1)
	}
	n := int(table[0])
	if n > (len(table)-1)/6 {
		return 0, false
	}
	for i := range n {
		row := table[1+6*i : 7+6*i]
		if tableEnd(row[1], row[2]) == theirs && tableEnd(row[3], row[4]) == ours {
			return row[5], true
		}
	}
	return 0, false
}

// tableEnd is an end of a connection as the table gives it: the IPv4
// address and the port in network byte order, in the order they are in
// memory, the port in the first two bytes of its DWORD.
func tableEnd(address, port uint32) netip.AddrPort {
	ip := netip.AddrFrom4([4]byte{byte(address), byte(address >> 8), byte(address >> 16), byte(address >> 24)})
	return netip.AddrPortFrom(ip, uint16(port)<<8|uint16(port>>8)&0xff)
}

// Read reads the child's output: any end of its connection, the child
// closing it or ending, is io.EOF, as with a pipe.
func (l *loopback) Read(p []byte) (int, error) {
	<-l.accepted
	l.mu.Lock()
	conn := l.conn
	l.mu.Unlock()
	if conn == nil {
		return 0, io.EOF
	}
	n, err := conn.Read(p)
	if err != nil {
		err = io.EOF
	}
	return n, err
}

func (l *loopback) Close() error {
	l.mu.Lock()
	l.closed = true
	conn := l.conn
	l.mu.Unlock()
	_ = l.listener.Close()
	if conn != nil {
		_ = conn.Close()
	}
	// A child that never started leaves no connection to wait for.
	l.end()
	return nil
}

// end tells Read the connection is accepted, or will not be.
func (l *loopback) end() {
	l.ended.Do(func() { close(l.accepted) })
}
