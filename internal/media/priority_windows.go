package media

import "golang.org/x/sys/windows"

// LowerPriority gives a process the lowest CPU priority, so that playback
// and the server's requests go first: FFmpeg reading in the background
// runs so.
func LowerPriority(pid int) {
	process, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, uint32(pid))
	if err != nil {
		return
	}
	_ = windows.SetPriorityClass(process, windows.IDLE_PRIORITY_CLASS)
	_ = windows.CloseHandle(process)
}
