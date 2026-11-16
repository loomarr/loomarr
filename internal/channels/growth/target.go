package growth

import (
	"math"
	"time"
)

// FilmRest is how long a film rests between airings where the line allows (decision 4).
const FilmRest = 30 * 24 * time.Hour

// Need is what a channel's size depends on: how much it airs and what it airs.
type Need struct {
	// DailyAirtime is the programme time a day carries (breaks excluded).
	DailyAirtime time.Duration
	// FilmShare is the share of that airtime films take: 0 on a series channel, 1 on a film channel.
	FilmShare float64
	// MeanEpisode and MeanFilm are the typical runtimes of what the channel airs.
	MeanEpisode, MeanFilm time.Duration
}

// Target is how big a channel should be (decision 2): enough episodes that none repeats within a
// week, enough shows that none dominates a day, and enough films that each rests a month.
type Target struct {
	Episodes, Shows, Films int
}

// TargetFor sizes a channel. maxShowShare is the largest share of a day one show may take, which
// sets the show count; the simulator sets it (#4140).
func TargetFor(n Need, maxShowShare float64) Target {
	var t Target
	seriesTime := time.Duration(float64(n.DailyAirtime) * (1 - n.FilmShare))
	filmTime := time.Duration(float64(n.DailyAirtime) * n.FilmShare)
	if seriesTime > 0 && n.MeanEpisode > 0 {
		t.Episodes = ceilDiv(7*seriesTime, n.MeanEpisode)
		if maxShowShare > 0 {
			t.Shows = int(math.Ceil(1 / maxShowShare))
		}
	}
	if filmTime > 0 && n.MeanFilm > 0 {
		t.Films = ceilDiv(time.Duration(float64(FilmRest/(24*time.Hour))*float64(filmTime)), n.MeanFilm)
	}
	return t
}

func ceilDiv(a, b time.Duration) int { return int((a + b - 1) / b) }
