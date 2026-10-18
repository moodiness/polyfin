package app

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// prepareServer gives the server a process group of its own, which a
// console control event reaches alone, and a hidden console.
func prepareServer(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW
	cmd.SysProcAttr.HideWindow = true
}

var (
	jobOnce sync.Once
	job     windows.Handle
	jobErr  error
)

// adopt puts the server in a job that ends with the app, even when the app
// crashes: Windows closes the job's handle, which kills what it holds.
func adopt(process *os.Process) error {
	jobOnce.Do(func() {
		job, jobErr = windows.CreateJobObject(nil, nil)
		if jobErr != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		_, jobErr = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	})
	if jobErr != nil {
		return jobErr
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.AssignProcessToJobObject(job, handle)
}

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	attachConsole         = kernel32.NewProc("AttachConsole")
	freeConsole           = kernel32.NewProc("FreeConsole")
	setConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
	consoleMu             sync.Mutex
)

// attachParentProcess is ATTACH_PARENT_PROCESS, the console of the
// process that started the app.
const attachParentProcess = uintptr(^uint32(0))

// interrupt sends Ctrl-Break to the server's console, which the server
// takes as a request to stop. The app attaches to that console for the
// time of it, ignoring the event itself. A process has one console at
// most: the app's own, when it runs from a terminal with -headless, is
// left meanwhile, then attached again.
func interrupt(process *os.Process) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	if had, _, _ := freeConsole.Call(); had != 0 {
		defer attachConsole.Call(attachParentProcess)
	}
	if r, _, err := attachConsole.Call(uintptr(process.Pid)); r == 0 {
		return err
	}
	defer freeConsole.Call()
	setConsoleCtrlHandler.Call(0, 1)
	defer setConsoleCtrlHandler.Call(0, 0)
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid))
}

// kill ends the server at once; the job the server keeps its own
// children in ends them with it.
func kill(process *os.Process) error {
	err := process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

// alive is not needed on Windows, where no server outlives the app.
func alive(int) bool { return false }

func startTime(int) (int64, bool) { return 0, false }
