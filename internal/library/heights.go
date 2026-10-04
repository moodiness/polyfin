package library

import (
	"strconv"
	"strings"
	"unicode"
)

// labelHeights are the video heights addons' labels name as words, in
// lines.
var labelHeights = map[string]int{
	"8k": 4320, "4k": 2160, "uhd": 2160, "ultrahd": 2160, "2k": 1440, "qhd": 1440,
	"fhd": 1080, "fullhd": 1080, "hd": 720, "sd": 480,
}

// scannedHeights are the heights a number of lines followed by p or i
// names: those of usual video, so that numbers that only look like one,
// such as 1200p, are not taken for one.
var scannedHeights = map[int]bool{240: true, 360: true, 480: true, 540: true, 576: true, 720: true, 1080: true, 1440: true, 2160: true, 4320: true}

// LabelHeight is the video height the labels of a stream name, from an
// addon's name, title, description or file name: 2160p, 4K or UHD, 1440p,
// 2K or QHD, 1080p or FHD, 720p or HD, 576p, 480p or SD, 1920x1080 and the
// like; 0 when they name none. Numbers of lines win over words, which
// releases use loosely ("4K remaster 1080p"); labels that disagree name
// none, as a wrong guess would leave out a version that fits.
func LabelHeight(labels ...string) int {
	var numbered, worded int
	numberedClash, wordedClash := false, false
	note := func(height int, found *int, clash *bool) {
		if *found != 0 && *found != height {
			*clash = true
		}
		*found = height
	}
	for _, label := range labels {
		words := strings.FieldsFunc(strings.ToLower(label), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		for i, word := range words {
			if height := scannedHeight(word); height > 0 {
				note(height, &numbered, &numberedClash)
				continue
			}
			height, ok := labelHeights[word]
			if !ok {
				continue
			}
			if word == "hd" {
				previous, next := "", ""
				if i > 0 {
					previous = words[i-1]
				}
				if i+1 < len(words) {
					next = words[i+1]
				}
				switch {
				case previous == "full":
					height = 1080
				case previous == "ultra":
					height = 2160
				case previous == "dts" || previous == "true" || next == "audio":
					// DTS-HD, True HD and HD audio are about the sound.
					continue
				}
			}
			note(height, &worded, &wordedClash)
		}
	}
	switch {
	case numbered != 0:
		if numberedClash {
			return 0
		}
		return numbered
	case wordedClash:
		return 0
	}
	return worded
}

// scannedHeight reads a number of lines followed by p or i, 1080p, or
// after a width, 1920x1080; 0 for any other word.
func scannedHeight(word string) int {
	var digits string
	if width, height, ok := strings.Cut(word, "x"); ok {
		if _, err := strconv.Atoi(width); err != nil || len(width) < 3 {
			return 0
		}
		digits = height
	} else if n := len(word); n > 1 && (word[n-1] == 'p' || word[n-1] == 'i') {
		digits = word[:n-1]
	}
	height, err := strconv.Atoi(digits)
	if err != nil || !scannedHeights[height] {
		return 0
	}
	return height
}
