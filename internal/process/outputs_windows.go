package process

import (
	"io"
	"net"
	"os/exec"
	"sync"

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

func (o *Outputs) started(cmd *exec.Cmd, err error) {
	if err != nil || len(o.loopbacks) == 0 {
		return
	}
	for _, l := range o.loopbacks {
		go l.accept()
	}
	// An output the child never connected to ends with the child, as a
	// pipe would.
	child, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	go func() {
		if openErr == nil {
			_, _ = windows.WaitForSingleObject(child, windows.INFINITE)
			_ = windows.CloseHandle(child)
		}
		for _, l := range o.loopbacks {
			_ = l.listener.Close()
		}
	}()
}

// loopback is an output of a child on the loopback: it reads the first
// connection its listener accepts, and ends without one once the listener
// is closed.
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

// accept takes the child's connection, the only one.
func (l *loopback) accept() {
	defer l.end()
	conn, err := l.listener.Accept()
	_ = l.listener.Close()
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		_ = conn.Close()
		return
	}
	l.conn = conn
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
