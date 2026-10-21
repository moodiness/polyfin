//go:build !unix

package torrent

import "time"

// processCPU is not measured here: BenchmarkWireGuardRoute reports no
// processor time.
func processCPU() time.Duration { return 0 }
