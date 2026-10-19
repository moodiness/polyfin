//go:build !windows && (!darwin || cgo)

package main

// trayThread needs nothing but on Windows.
func trayThread() {}

// endTrayLoop needs nothing but on Windows: systray.Quit ends the loop,
// with the icon or without.
func endTrayLoop() {}
