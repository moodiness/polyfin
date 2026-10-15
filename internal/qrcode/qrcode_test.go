package qrcode

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// polyfinMask2 is "Polyfin" in byte mode at level M with mask 2, as
// Project Nayuki's and node-qrcode's generators draw it.
var polyfinMask2 = []string{
	"111111100011101111111",
	"100000100000001000001",
	"101110101110101011101",
	"101110101110101011101",
	"101110101010101011101",
	"100000101101001000001",
	"111111101010101111111",
	"000000001110000000000",
	"101111100101001111100",
	"110101000111111101001",
	"011010110100101101010",
	"010111001101111111100",
	"110010111100100010000",
	"000000001110100111010",
	"111111100101010000110",
	"100000101110000001111",
	"101110101011010111010",
	"101110101011111110000",
	"101110101100101001100",
	"100000100011111111100",
	"111111101110100000010",
}

func TestEncodeMatchesAReferenceCode(t *testing.T) {
	if got := encode([]byte("Polyfin"), 1, 2).Rows(); strings.Join(got, "\n") != strings.Join(polyfinMask2, "\n") {
		t.Errorf("got\n%s", strings.Join(got, "\n"))
	}
}

// An otpauth address takes the smallest version that holds it, with its
// three finder patterns.
func TestEncodeChoosesTheSmallestVersion(t *testing.T) {
	address := "otpauth://totp/Polyfin:alice?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=Polyfin"
	code, err := Encode([]byte(address))
	if err != nil || code.Size != 5*4+17 {
		t.Fatalf("size %d, %v", code.Size, err)
	}
	finder := []string{"1111111", "1000001", "1011101", "1011101", "1011101", "1000001", "1111111"}
	for _, corner := range [][2]int{{0, 0}, {code.Size - 7, 0}, {0, code.Size - 7}} {
		for dy, row := range finder {
			for dx, module := range row {
				if code.Dark(corner[0]+dx, corner[1]+dy) != (module == '1') {
					t.Fatalf("finder at %v differs at %d,%d", corner, dx, dy)
				}
			}
		}
	}
	// Version 40 at level M holds 2331 bytes.
	if code, err := Encode(bytes.Repeat([]byte{'a'}, 2331)); err != nil || code.Size != 177 {
		t.Errorf("2331 bytes: size %d, %v", code.Size, err)
	}
	if _, err := Encode(bytes.Repeat([]byte{'a'}, 2332)); !errors.Is(err, ErrTooLong) {
		t.Errorf("2332 bytes: %v", err)
	}
}
