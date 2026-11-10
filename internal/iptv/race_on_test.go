//go:build race

package iptv

// lineupScale divides the size of TestLargeLineupsStayFast's line-ups: the
// race detector, slowing their work tenfold, gets a tenth of them, 5,400
// entries in 42 groups and 540 in 4. Without the race detector, make check
// checks the time bounds at full size.
const lineupScale = 10
