package process

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillChildrenOnExit makes the programs Polyfin starts, and those they
// start, end with it, however it ends, killed included: on Windows, which
// has no process groups, Polyfin joins a job object of its own that kills
// its processes once its handle closes, as it does when Polyfin ends. The
// children join the job as they start. Its handle is kept for Polyfin's
// life.
func KillChildrenOnExit() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	// A process already in a job, such as the desktop app's, joins this
	// one within it.
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	return nil
}
