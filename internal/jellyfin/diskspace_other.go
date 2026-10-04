//go:build !linux && !darwin

package jellyfin

// diskSpace cannot measure disks here: folders are described unmeasured.
func diskSpace(string) (free, used int64, mount string, ok bool) {
	return 0, 0, "", false
}
