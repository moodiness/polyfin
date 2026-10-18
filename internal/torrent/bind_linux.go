package torrent

import "syscall"

// bindsDevices tells whether sockets can be bound to a network interface.
const bindsDevices = true

// bindToDevice binds a socket not yet bound to the network interface
// name (SO_BINDTODEVICE): since Linux 5.7 it needs no capability.
func bindToDevice(fd uintptr, name string) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, name)
}
