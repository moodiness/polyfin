//go:build !windows

package process

import (
	"io"
	"os"
	"os/exec"
)

// outputs are the write ends of the pipes, which the parent closes once the
// child holds them.
type outputs struct {
	writers []*os.File
}

func (o *Outputs) open(cmd *exec.Cmd, n int) error {
	for range n {
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		o.Readers = append(o.Readers, io.ReadCloser(r))
		o.writers = append(o.writers, w)
		cmd.ExtraFiles = append(cmd.ExtraFiles, w)
	}
	return nil
}

func (o *Outputs) started(*exec.Cmd, error) {
	for _, w := range o.writers {
		_ = w.Close()
	}
	o.writers = nil
}
