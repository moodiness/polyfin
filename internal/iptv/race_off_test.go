//go:build !race

package iptv

// raceSlowdown scales time bounds while the race detector slows code down.
const raceSlowdown = 1
