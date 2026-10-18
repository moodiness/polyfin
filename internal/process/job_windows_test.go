package process

import (
	"bufio"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A Polyfin killed outright on Windows takes its children with it: the
// child of a helper that joined its job ends when the helper is killed.
func TestChildrenEndWithTheParent(t *testing.T) {
	parent := helperCommand("parent")
	stdout, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Process.Kill(); _ = parent.Wait() }()
	lines := bufio.NewScanner(stdout)
	if !lines.Scan() {
		t.Fatal("the helper started no child")
	}
	pid, err := strconv.Atoi(lines.Text())
	if err != nil {
		t.Fatalf("the child's identifier: %q", lines.Text())
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	if event, _ := windows.WaitForSingleObject(child, 0); event == windows.WAIT_OBJECT_0 {
		t.Fatal("the child ended before its parent")
	}
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// A minute is a margin a busy runner never needs.
	if event, _ := windows.WaitForSingleObject(child, uint32(time.Minute/time.Millisecond)); event != windows.WAIT_OBJECT_0 {
		_ = windows.TerminateProcess(child, 1)
		t.Error("the child outlived its parent")
	}
}
