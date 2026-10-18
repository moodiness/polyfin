package process

import (
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
)

// Interruptible prepares FFmpeg's cmd, not started yet, to be interrupted
// as Ctrl-C would. Windows sends no signal to a process: FFmpeg reads its
// keys on its standard input instead, -nostdin taken out of its arguments,
// and its first interrupt is q, on which it stops, writing what it holds.
// Its second, which on Linux ends a read waiting for its source, kills it:
// FFmpeg would write nothing more after it.
func Interruptible(cmd *exec.Cmd) (interrupt func() error, err error) {
	cmd.Args = slices.DeleteFunc(cmd.Args, func(arg string) bool { return arg == "-nostdin" })
	keys, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	asked := false
	return func() error {
		mu.Lock()
		defer mu.Unlock()
		if asked {
			return cmd.Process.Kill()
		}
		asked = true
		if _, err := io.WriteString(keys, "q"); err != nil {
			// FFmpeg no longer reads its keys: it is ending.
			return os.ErrProcessDone
		}
		return nil
	}, nil
}
