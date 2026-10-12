//go:build !race

package iptv

// raceSlowdown scales time bounds while the race detector slows code down.
const raceSlowdown = 1

// lineupScale divides the size of TestLargeLineupsStayFast's line-up.
const lineupScale = 1
