package process

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// A program written while others start runs at once: none of them holds
// it open for writing, which would fail its run, "text file busy".
// Written without the lock, it failed so in each of 30 tries, by its
// 280th run at the latest.
func TestAProgramWrittenWhileOthersStartRuns(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("this system runs a file open for writing")
	}
	stop := make(chan struct{})
	var starting sync.WaitGroup
	for range 2 {
		starting.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = exec.Command("true").Run()
			}
		})
	}
	defer func() {
		close(stop)
		starting.Wait()
	}()
	dir := t.TempDir()
	for i := range 500 {
		path := filepath.Join(dir, strconv.Itoa(i))
		if err := WriteExecutable(path, strings.NewReader("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command(path).Run(); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}
