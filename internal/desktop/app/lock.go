package app

import (
	"errors"
	"os"
	"sync"
)

// ErrRunning is a data folder another app runs on.
var ErrRunning = errors.New("Polyfin Desktop already runs on this data folder")

// Lock keeps a second app from running on the data folder of path: it
// returns ErrRunning while another holds it, and a release otherwise.
func Lock(path string) (release func(), err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		file.Close()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { file.Close() }) }, nil
}
