package torrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A peer that refused the first connection is dialed again once its wait
// passed, and sends the torrent, which has no other peer.
func TestAPeerThatRefusedIsDialedAgain(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(300 << 10)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	proxy := torrenttest.StartProxy(t, true, "", "")
	proxy.RefuseFirst(seeder.Address())
	e := newEngineWith(t, proxied(proxy), func(c *Config) { c.StartTimeout = 30 * time.Second })
	e.redialWait = 200 * time.Millisecond
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole")
	}
	asked := 0
	for _, target := range proxy.Targets() {
		if target == "tcp "+seeder.Address().String() {
			asked++
		}
	}
	if asked < 2 {
		t.Fatalf("the seeder was asked for %d times", asked)
	}
}

// A peer whose connection ended is dialed again once its wait passed, and
// the read waiting for its blocks goes on.
func TestAPeerThatDroppedIsDialedAgain(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(300 << 10)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	seeder.Hold()
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
	e.redialWait = 200 * time.Millisecond
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	read := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(io.NewSectionReader(r, 0, r.Size()))
		read <- data
	}()
	waitFor(t, "the seeder to be asked", func() bool { return seeder.Requests() > 0 })
	seeder.Disconnect()
	seeder.Release()
	if got := <-read; !bytes.Equal(got, tor.Data) {
		t.Fatalf("read %d bytes, not the file's", len(got))
	}
	if len(seeder.Remotes()) < 2 {
		t.Fatalf("the seeder saw %d connections", len(seeder.Remotes()))
	}
}

// How a dial ended is read from its error, whichever route made it.
func TestOutcomeOfADial(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]outcome{
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}: outcomeRefused,
		&socksReplyError{code: socksRefused}:                                               outcomeRefused,
		&socksReplyError{code: socksHostUnreachable}:                                       outcomeUnreachable,
		&socksReplyError{code: socksTTLExpired}:                                            outcomeTimedOut,
		&socksReplyError{code: 1}:                                                          outcomeProxy,
		&RouteError{Kind: ProxyUnreachable}:                                                outcomeProxy,
		fmt.Errorf("dial: %w", context.DeadlineExceeded):                                   outcomeTimedOut,
		os.ErrDeadlineExceeded:                                                             outcomeTimedOut,
		io.ErrUnexpectedEOF:                                                                outcomeClosed,
		&net.OpError{Op: "connect", Err: errors.New("connection was refused")}:             outcomeRefused,
		&net.OpError{Op: "connect", Err: errors.New("no route to host")}:                   outcomeUnreachable,
		errHandshake: outcomeOther,
	} {
		if got := outcomeOf(err); got != want {
			t.Errorf("%v: %d, want %d", err, got, want)
		}
	}
}
