//go:build !unix

package iptv

import "time"

// processCPU is not measured here: TestLargeLineupsStayFast compares no
// processor times.
func processCPU() time.Duration { return 0 }
