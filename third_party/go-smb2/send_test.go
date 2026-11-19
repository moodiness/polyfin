package smb2

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/cloudsoda/go-smb2/internal/smb2"
)

// heldTransport holds each write until the test gives its result, and
// tells the packet's bytes as the write starts and as it ends.
type heldTransport struct {
	started, ended chan []byte
	results        chan error
}

func (t *heldTransport) Write(p []byte) (int, error) {
	t.started <- bytes.Clone(p)
	err := <-t.results
	t.ended <- bytes.Clone(p)
	return len(p), err
}

func (t *heldTransport) ReadSize() (int, error)   { select {} }
func (t *heldTransport) Read([]byte) (int, error) { select {} }
func (t *heldTransport) Close() error             { return nil }

// echo is a request taking one credit, so that each is numbered apart.
func echo() smb2.Packet {
	req := &smb2.EchoRequest{}
	req.CreditCharge = 1
	return req
}

// A request given up while its packet is being written leaves that
// packet as it is until written, and its write's result to no one: the
// next request is encoded apart, and gets the result of its own write.
func TestARequestGivenUpMidWriteKeepsItsPacket(t *testing.T) {
	t.Parallel()
	tr := &heldTransport{started: make(chan []byte, 4), ended: make(chan []byte, 4), results: make(chan error, 4)}
	c := &conn{t: tr, outstandingRequests: newOutstandingRequests(), account: openAccount(64), rdone: make(chan struct{}, 1),
		wdone: make(chan struct{}), write: make(chan outgoing, 1), werr: make(chan error, 1)}
	go c.runSender()
	t.Cleanup(func() { close(c.wdone) })

	ctx, cancel := context.WithCancel(t.Context())
	given := make(chan error, 1)
	go func() {
		_, err := c.send(ctx, echo())
		given <- err
	}()
	first := <-tr.started
	cancel()
	if err := <-given; !errors.Is(err, context.Canceled) {
		t.Fatalf("the request given up: %v, want it cancelled", err)
	}

	next := make(chan error, 1)
	go func() {
		_, err := c.send(t.Context(), echo())
		next <- err
	}()
	// The next request is encoded and waits for the sender, still
	// writing the first.
	for len(c.write) == 0 {
		runtime.Gosched()
	}
	tr.results <- errors.New("the first write failed")
	if written := <-tr.ended; !bytes.Equal(written, first) {
		t.Errorf("the first packet changed while written:\n%x\nwas\n%x", written, first)
	}
	tr.results <- nil
	second := <-tr.started
	<-tr.ended
	if err := <-next; err != nil {
		t.Errorf("the next request: %v, want its own write's result", err)
	}
	if bytes.Equal(second, first) {
		t.Error("the two requests have one packet")
	}
}
