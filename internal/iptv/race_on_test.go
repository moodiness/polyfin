//go:build race

package iptv

// raceSlowdown scales time bounds while the race detector slows code down.
const raceSlowdown = 10

// lineupScale divides the size of TestLargeLineupsStayFast's line-up: the
// race detector, slowing its work tenfold, gets a tenth of it, 5,400
// entries in 42 groups. Without the race detector, make check checks the
// time bounds at full size.
const lineupScale = 10
