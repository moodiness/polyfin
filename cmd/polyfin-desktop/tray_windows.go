package main

import (
	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	postThreadMessage = user32.NewProc("PostThreadMessageW")
	// loopThread is the thread of the library's message loop.
	loopThread uint32
)

// wmQuit is WM_QUIT, which ends a thread's message loop.
const wmQuit = 0x0012

// trayThread records the thread systray.Run runs its loop on: the main
// one, which the library locks the main goroutine to.
func trayThread() {
	loopThread = windows.GetCurrentThreadId()
}

// endTrayLoop ends the library's message loop, which waits for messages
// of the thread too, when the icon's window is not there to close.
func endTrayLoop() {
	postThreadMessage.Call(uintptr(loopThread), wmQuit, 0, 0)
}
