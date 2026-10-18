package process

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// helperMode, in the environment, makes the test binary a child of the
// tests, which does what its value says (see helper).
const helperMode = "POLYFIN_PROCESS_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperMode); mode != "" {
		os.Exit(helper(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// helper is a child of the tests: "write" writes "output i" to each output
// its arguments name, pipe:N or tcp://, as FFmpeg would; "silent" ends
// without opening them; "keys" tells it is ready, waits for an interrupt
// (SIGINT, or q on its standard input on Windows), and tells it stopped;
// "parent" starts a "sleep" child, which sleeps, tells its process
// identifier and sleeps too.
func helper(mode string, args []string) int {
	switch mode {
	case "write":
		for i, name := range args {
			var w io.WriteCloser
			if fd, ok := strings.CutPrefix(name, "pipe:"); ok {
				n, _ := strconv.Atoi(fd)
				w = os.NewFile(uintptr(n), name)
			} else if address, ok := strings.CutPrefix(name, "tcp://"); ok {
				conn, err := net.Dial("tcp", address)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 2
				}
				w = conn
			} else {
				continue
			}
			fmt.Fprintf(w, "output %d\n", i)
			_ = w.Close()
		}
	case "silent":
	case "keys":
		stopped := make(chan struct{})
		if runtime.GOOS == "windows" {
			go func() {
				keys := bufio.NewReader(os.Stdin)
				for {
					key, err := keys.ReadByte()
					if err != nil || key == 'q' {
						close(stopped)
						return
					}
				}
			}()
		} else {
			interrupts := make(chan os.Signal, 1)
			signal.Notify(interrupts, os.Interrupt)
			go func() {
				<-interrupts
				close(stopped)
			}()
		}
		fmt.Println("ready")
		<-stopped
		fmt.Println("stopped")
	case "parent":
		if err := KillChildrenOnExit(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		child := helperCommand("sleep")
		if err := child.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Println(child.Process.Pid)
		time.Sleep(10 * time.Minute)
	case "sleep":
		time.Sleep(10 * time.Minute)
	}
	return 0
}

// helperCommand is the test binary run as a helper in mode.
func helperCommand(mode string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), helperMode+"="+mode)
	return cmd
}

// readAll reads r to its end, failing the test past a minute, a margin a
// busy runner never needs.
func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	read := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		read <- string(data)
	}()
	select {
	case data := <-read:
		return data
	case <-time.After(time.Minute):
		t.Fatal("the output never ended")
		return ""
	}
}

// A child writes each of its outputs, which it names pipe:3 on, and each
// reads what it wrote: on Windows, through the loopback addresses that
// replace those names in its arguments.
func TestOutputsCarryWhatTheChildWrites(t *testing.T) {
	cmd := helperCommand("write", "pipe:3", "pipe:4", "pipe:1")
	outputs, err := NewOutputs(cmd, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Start()
	outputs.Started(cmd, err)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if slices.Contains(cmd.Args, "pipe:3") || slices.Contains(cmd.Args, "pipe:4") || !strings.HasPrefix(cmd.Args[1], "tcp://127.0.0.1:") ||
			cmd.Args[3] != "pipe:1" {
			t.Errorf("arguments %q", cmd.Args)
		}
	} else if !slices.Equal(cmd.Args[1:], []string{"pipe:3", "pipe:4", "pipe:1"}) {
		t.Errorf("arguments %q", cmd.Args)
	}
	for i, r := range outputs.Readers {
		if got, want := readAll(t, r), fmt.Sprintf("output %d\n", i); got != want {
			t.Errorf("output %d: %q, want %q", i, got, want)
		}
		_ = r.Close()
	}
	if err := cmd.Wait(); err != nil {
		t.Error(err)
	}
}

// An output the child never opens ends with the child, as a pipe does, so
// that whoever reads it waits for the child no longer.
func TestOutputsEndWithAChildThatNeverOpensThem(t *testing.T) {
	cmd := helperCommand("silent", "pipe:3", "pipe:4")
	outputs, err := NewOutputs(cmd, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Start()
	outputs.Started(cmd, err)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range outputs.Readers {
		if got := readAll(t, r); got != "" {
			t.Errorf("output %d: %q", i, got)
		}
		_ = r.Close()
	}
	if err := cmd.Wait(); err != nil {
		t.Error(err)
	}
}

// A child that cannot start leaves no output open.
func TestOutputsOfAChildThatCannotStart(t *testing.T) {
	cmd := exec.Command(filepath.Join(t.TempDir(), "missing"), "pipe:3")
	outputs, err := NewOutputs(cmd, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Start()
	outputs.Started(cmd, err)
	if err == nil {
		t.Fatal("a missing program started")
	}
	if n, err := outputs.Readers[0].Read(make([]byte, 1)); n != 0 || err == nil {
		t.Errorf("read %d, %v from a closed output", n, err)
	}
}

// A child interrupted as Ctrl-C would stops on its own: SIGINT on Linux and
// macOS, q on its standard input on Windows, which takes -nostdin out of
// its arguments for FFmpeg to read it.
func TestInterruptStopsTheChild(t *testing.T) {
	cmd := helperCommand("keys", "-nostdin")
	interrupt, err := Interruptible(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && slices.Contains(cmd.Args, "-nostdin") {
		t.Errorf("arguments %q", cmd.Args)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lines := bufio.NewScanner(stdout)
	if !lines.Scan() || lines.Text() != "ready" {
		t.Fatalf("the child did not start: %q", lines.Text())
	}
	if err := interrupt(); err != nil {
		t.Fatal(err)
	}
	if !lines.Scan() || lines.Text() != "stopped" {
		t.Errorf("the child did not stop on its own: %q", lines.Text())
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("the child ended with %v", err)
	}
}
