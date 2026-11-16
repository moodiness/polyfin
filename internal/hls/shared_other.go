//go:build !windows

package hls

// heldElsewhere reports that no other program holds a file in the way:
// other systems open and rename over a file however others hold it.
func heldElsewhere(error) bool {
	return false
}
