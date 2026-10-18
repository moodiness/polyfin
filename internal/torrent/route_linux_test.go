//go:build linux

package torrent

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// boundDevice reads the interface a socket is bound to (SO_BINDTODEVICE).
func boundDevice(t *testing.T, conn syscall.Conn) string {
	t.Helper()
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var name [syscall.IFNAMSIZ]byte
	size := uint32(len(name))
	var errno syscall.Errno
	if err := raw.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE,
			uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&size)), 0)
	}); err != nil {
		t.Fatal(err)
	}
	if errno != 0 {
		t.Fatal(errno)
	}
	return string(bytes.TrimRight(name[:size], "\x00"))
}

func TestInterfaceRouteBindsEverySocket(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	route := InterfaceRoute("lo")
	ctx := context.Background()
	conn, err := route.Dial(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := boundDevice(t, conn.(*net.TCPConn)); got != "lo" {
		t.Fatalf("a TCP socket is bound to %q", got)
	}
	packets, err := route.ListenPacket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer packets.Close()
	if got := boundDevice(t, packets.(*interfacePacketConn).conn); got != "lo" {
		t.Fatalf("a UDP socket is bound to %q", got)
	}
	// Names are looked up over the interface, never by a name server on
	// the loopback, which would send the query another way.
	if _, err := route.(*interfaceRoute).resolver.Dial(ctx, "udp", "127.0.0.53:53"); err == nil {
		t.Fatal("a name server on the loopback was asked")
	}
}

func TestInterfaceRoutePlays(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	udpTracker := torrenttest.StartUDPTracker(t, seeder.Address())
	e := newEngine(t, &testSettings{s: Settings{Enabled: true, Interface: "lo"}}, 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL(), "tracker:" + udpTracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatal("the largest file was not read whole")
	}
	waitFor(t, "the UDP tracker's announce", func() bool { return udpTracker.Announces() > 0 })
}

func TestMissingInterfaceStartsNothing(t *testing.T) {
	t.Parallel()
	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	e := newEngine(t, &testSettings{s: Settings{Enabled: true, Interface: "pfmissing0"}}, 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	e.Play(spec)()
	if _, err := e.Open(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got %v, want the interface missing", err)
	}
	status := e.Status(context.Background())
	if status.RouteError == nil || status.Failure == nil || status.Playing != 0 {
		t.Fatalf("Health does not tell the interface is missing: %+v", status)
	}
	// Nothing started: neither the tracker nor the seeder was reached.
	if len(tracker.Remotes()) != 0 || len(seeder.Remotes()) != 0 {
		t.Fatal("the torrent was reached without its interface")
	}
}

// TestInterfaceGoingDown needs a dummy interface with an address, named
// by POLYFIN_TEST_TORRENT_INTERFACE, and the right to set it down and up
// (NET_ADMIN): a throwaway container provides both.
func TestInterfaceGoingDown(t *testing.T) {
	name := os.Getenv("POLYFIN_TEST_TORRENT_INTERFACE")
	if name == "" {
		t.Skip("POLYFIN_TEST_TORRENT_INTERFACE names no interface to set down")
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		t.Fatal(err)
	}
	addrs, err := iface.Addrs()
	if err != nil || len(addrs) == 0 {
		t.Fatalf("the interface %s has no address", name)
	}
	host := addrs[0].(*net.IPNet).IP.String()
	ip := func(args ...string) {
		if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
			t.Fatalf("ip %v: %v: %s", args, err, out)
		}
	}
	defer ip("link", "set", "dev", name, "up")

	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, host)
	tracker := torrenttest.HTTPTracker(t, host, seeder.Address())
	settings := &testSettings{s: Settings{Enabled: true, Interface: name, DownloadMbps: 1}}
	e := newEngine(t, settings, 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadAt(make([]byte, 1000), 0); err != nil {
		t.Fatal(err)
	}
	for _, remote := range seeder.Remotes() {
		if !strings.HasPrefix(remote, host+":") {
			t.Fatalf("the seeder saw a connection from %s, not from %s", remote, name)
		}
	}
	ip("link", "set", "dev", name, "down")
	buffer := make([]byte, r.Size())
	if _, err := r.ReadAt(buffer, 0); err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("a read with the interface down gave %v", err)
	}
	before := len(seeder.Remotes())
	e.Play(spec)()
	if _, err := e.Open(ctx, spec); err == nil {
		t.Fatal("the torrent opened with the interface down")
	}
	if status := e.Status(ctx); status.RouteError == nil || !strings.Contains(status.RouteError.Error(), "down") {
		t.Fatalf("Health does not tell the interface is down: %+v", status)
	}
	if got := len(seeder.Remotes()); got != before {
		t.Fatalf("the seeder saw %d new connections with the interface down", got-before)
	}
}
