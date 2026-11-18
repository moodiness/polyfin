//go:build !windows

package config

import (
	"os"
	"syscall"
)

// writableByAnother tells the fault of the folder dir, whose info is
// given, when it is not the user self's alone to write into: another
// user owns it, or its group or others can write into it. Nil tells it
// has none.
func writableByAnother(dir string, info os.FileInfo, self int) *DataFolderFault {
	fault := &DataFolderFault{Dir: dir, Owner: self, User: self, Mode: info.Mode().Perm()}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		fault.Owner = int(stat.Uid)
	}
	if fault.OtherOwner() || fault.Mode&0o022 != 0 {
		return fault
	}
	return nil
}
