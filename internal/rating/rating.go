// Package rating is the audience rating ladder (programming-design §4): one ordered scale that
// places the TV and film content-rating systems side by side, and the folding of a media server's
// raw rating string onto it.
//
// It is a leaf vocabulary. A channel's audience policy (the ceiling, and what to do with an
// unrated title) is schedule's; this package only says what a rating is and how two compare, so
// the metadata providers and the suggester can read a rating without importing the scheduler.
package rating

import "strings"

// Rating is a content rating from either the TV or film system. Comparison is by its rank on
// the ladder; an unmappable or empty value is *unrated* (no rank).
type Rating string

// ladderRank maps both rating systems onto one ordered ladder:
// TV-Y < TV-Y7 < TV-G/G < TV-PG/PG < TV-14/PG-13 < TV-MA/R/NC-17.
// A rating absent from this map is unrated (handled fail-closed by the caller's policy).
var ladderRank = map[Rating]int{
	"TV-Y":  0,
	"TV-Y7": 1,
	"TV-G":  2, "G": 2,
	"TV-PG": 3, "PG": 3,
	"TV-14": 4, "PG-13": 4,
	"TV-MA": 5, "R": 5, "NC-17": 5,
}

// KidsCeilingRank is the highest ladder rank still considered a "kids" ceiling (TV-PG). At or
// below it, a channel's default unrated policy is exclude, and an auto-raise must never cross
// it: a kids channel is never forced above TV-PG because a mis-rated pick slipped in (a TV-MA
// pick is dropped, not admitted).
const KidsCeilingRank = 3

// Normalize maps a raw media-server OfficialRating string onto the ladder. It upper-cases,
// trims, and folds the common Emby/Jellyfin variants ("TV_Y7", "tv-14", "Rated R" → …); anything
// not on the ladder returns "" (unrated), which enforcement handles fail-closed under a kids
// ceiling. Idempotent: a normalized rating normalizes to itself.
func Normalize(raw string) Rating {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimPrefix(s, "RATED")
	if _, ok := ladderRank[Rating(s)]; ok {
		return Rating(s)
	}
	return ""
}

// Mapped reports whether r is a known ladder rating (not unrated).
func (r Rating) Mapped() bool { _, ok := ladderRank[r]; return ok }

// Rank returns r's position on the ladder (TV-Y=0 … TV-MA/R=5) and whether r is a known ladder
// rating. The suggester uses it to guarantee its audience ceiling admits the titles it actually
// grounded — a ceiling stricter than a picked title would silently empty the channel.
func (r Rating) Rank() (int, bool) { n, ok := ladderRank[r]; return n, ok }

// AtOrBelow reports whether r is at or below ceiling on the ladder. Both must be mapped; an
// unrated item is never "at or below" — the caller's unrated policy decides it.
func (r Rating) AtOrBelow(ceiling Rating) bool {
	rr, rok := ladderRank[r]
	cc, cok := ladderRank[ceiling]
	return rok && cok && rr <= cc
}
