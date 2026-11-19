package process

import (
	"net"
	"os"
	"strings"
	"testing"
)

// A process other than the child that connects to an output first, here
// the tests' own, is closed unread: the output carries the child's bytes
// alone, and the child still connects to it.
func TestOutputsTakeTheChildsConnectionAlone(t *testing.T) {
	t.Parallel()
	cmd := helperCommand("write", "pipe:3")
	outputs, err := NewOutputs(cmd, 1)
	if err != nil {
		t.Fatal(err)
	}
	address, ok := strings.CutPrefix(cmd.Args[1], "tcp://")
	if !ok {
		t.Fatalf("arguments %q", cmd.Args)
	}
	// The listener is there from NewOutputs on: this process connects to
	// it before the child starts, and writes.
	other, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Write([]byte("not the child's\n")); err != nil {
		t.Fatal(err)
	}
	if err := other.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	outputs.Started(cmd, err)
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, outputs.Readers[0])
	_ = outputs.Readers[0].Close()
	// The helper ends with status 2 when it cannot connect to its output.
	if err := cmd.Wait(); err != nil {
		t.Errorf("the child: %v", err)
	}
	if want := "output 0\n"; got != want {
		t.Fatalf("the output carried %q, want the child's %q", got, want)
	}
	if n, err := other.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Errorf("the other connection read %d, %v: it was not closed", n, err)
	}
}

// connectedBy tells the process that made a connection on the loopback.
func TestConnectedByTellsTheConnectingProcess(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	dialed, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer dialed.Close()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if pid, ok := connectedBy(conn); !ok || pid != uint32(os.Getpid()) {
		t.Errorf("connectedBy: %d, %v, want %d", pid, ok, os.Getpid())
	}
}
