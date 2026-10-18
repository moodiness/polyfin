//go:build !linux

package torrent

// bindsDevices tells whether sockets can be bound to a network interface:
// only Linux binds them (SO_BINDTODEVICE).
const bindsDevices = false

func bindToDevice(uintptr, string) error {
	return ErrInterfaceUnsupported
}
