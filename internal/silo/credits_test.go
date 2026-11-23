package silo

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/silo/silotest"
	"github.com/moodiness/polyfin/internal/userdata"
)

// An episode the apps take as finished, stopped at its end credits by
// their rule (see silotest.EndCreditsStart), is played on Polyfin too,
// whatever resume threshold the settings choose: Polyfin counts the first
// credits in the second half of an episode, which the apps' last stretch
// always is, so the Apple app moves a series' page on to the episode
// Polyfin also moves Next Up on to. Movies are left to the percentage
// rule, as their scenes after the credits are part of them: the apps read
// their end credits only for downloads, which Polyfin does not serve.
func TestEpisodesTheAppsFinishAtTheirCreditsArePlayed(t *testing.T) {
	t.Parallel()
	seconds := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
	for _, minutes := range []float64{11, 22, 45, 62} {
		duration := minutes * 60
		for _, credits := range [][]float64{{duration - 90}, {duration * 0.62}, {60, duration - 300}, {duration - 400, duration - 120},
			{duration * 0.7, duration * 0.95}} {
			start, ends := silotest.EndCreditsStart(credits, duration, true)
			if !ends {
				continue
			}
			for _, resume := range []int{accounts.MinResumePercent, accounts.DefaultResumePercent, accounts.MaxResumePercent} {
				thresholds := userdata.Thresholds{Resume: resume, Played: accounts.DefaultPlayedPercent}
				var first time.Duration
				for _, at := range credits {
					if thresholds.EndCredits(seconds(at), seconds(duration)) && (first == 0 || seconds(at) < first) {
						first = seconds(at)
					}
				}
				if !thresholds.ReachesCredits(seconds(start), seconds(duration), first) {
					t.Errorf("%v minutes, credits at %v, resume %d%%: a stop at %vs is not played", minutes, credits, resume, start)
				}
			}
		}
	}
}
