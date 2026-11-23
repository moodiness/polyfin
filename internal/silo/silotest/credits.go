package silotest

// EndCreditsStart is where the apps take a file's end credits to start,
// in seconds, and false when it has none: the latest start of its credits
// markers that falls in the file's last stretch, before its end. That
// stretch is the last 450 seconds or 40% of an episode, whichever is
// shorter, and the last 900 seconds or 25% of anything else. Reaching it
// finishes the title for the apps, as Silo's server judges a stop: Apple
// PlayerMarkerTimeline.endCreditsStart
// (Screens/Player/PlayerMarkerTimeline.swift), which moves a series' page
// on to the next episode once a stop reaches it, and Android
// MarkerTimeline.endCreditsStart (domain/player/MarkerTimeline.kt), which
// only its downloads read.
func EndCreditsStart(credits []float64, duration float64, episode bool) (float64, bool) {
	if duration <= 0 {
		return 0, false
	}
	tail := min(900, duration/4)
	if episode {
		tail = min(450, duration*0.4)
	}
	var start float64
	found := false
	for _, at := range credits {
		if at >= duration-tail && at < duration && (!found || at > start) {
			start, found = at, true
		}
	}
	return start, found
}
