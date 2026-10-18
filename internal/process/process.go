// Package process controls the programs Polyfin runs, FFmpeg above all, the
// same way on each system: the outputs they write besides their standard
// output, how they are interrupted as Ctrl-C would, and how they end with
// Polyfin.
//
// Linux and macOS give a child file descriptors past its standard ones, and
// signals. Windows does neither: a child's further outputs are connections
// on the loopback there, FFmpeg is interrupted through its standard input,
// and the children are kept in a job object that ends them with Polyfin.
package process

import (
	"io"
	"os/exec"
	"strconv"
)

// Outputs are the outputs a child writes besides its standard output and
// error, which it names pipe:3, pipe:4 and so on in its arguments, as
// FFmpeg does: Readers reads each in turn. Each read ends, io.EOF, once the
// child closed its output or ended, even when it never opened it.
type Outputs struct {
	Readers []io.ReadCloser
	outputs
}

// NewOutputs gives cmd, not started yet, n outputs: on Linux and macOS,
// pipes it inherits as file descriptors 3 to 3+n-1; on Windows, which passes
// none past the standard ones, connections on the loopback, whose
// addresses replace pipe:3 to pipe:3+n-1 in cmd's arguments. Once cmd is
// started, Started must be called.
func NewOutputs(cmd *exec.Cmd, n int) (*Outputs, error) {
	o := &Outputs{}
	if err := o.open(cmd, n); err != nil {
		o.Started(cmd, err)
		return nil, err
	}
	return o, nil
}

// Started is called once cmd.Start returned err: the ends the child holds
// are closed here, and on failure the readers too.
func (o *Outputs) Started(cmd *exec.Cmd, err error) {
	o.started(cmd, err)
	if err != nil {
		o.close()
	}
}

// close closes every reader.
func (o *Outputs) close() {
	for _, r := range o.Readers {
		_ = r.Close()
	}
}

// pipeName is how a child names its output i, from 0: pipe:3 on.
func pipeName(i int) string {
	return "pipe:" + strconv.Itoa(3+i)
}
