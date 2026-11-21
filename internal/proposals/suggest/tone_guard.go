package suggest

import (
	"regexp"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/library/titlesearch"
)

// Two code guards for what a weak model gets wrong at selection (#3365). Prompt wording alone
// doesn't hold: a model that lists the candidates it considered, rejections included, still has
// its picks taken as recommendations.

// rationaleRejectionPattern finds a pick's rationale arguing against the pick: "Holiday-set but
// far too violent", "doesn't fit a cozy channel", "unsuitable for kids". Phrases, not single
// words: "despite" or "dark" alone also appear in rationales that recommend a title.
var rationaleRejectionPattern = regexp.MustCompile(`(?i)\b(?:` +
	`(?:far |way |much )?too (?:violent|scary|gory|graphic|dark|disturbing|intense|mature|adult|explicit|grim|bleak)` +
	`|(?:does ?n[o']?t|doesn't|does not|wouldn't|would not|won't|will not|can't|cannot) (?:really )?(?:fit|suit|belong|match)` +
	`|not (?:a |an )?(?:good |great |real )?fit` + `|(?:poor|bad|weak) fit` +
	`|(?:un|not )suitable|inappropriate|not appropriate` +
	`|not (?:cozy|cosy|gentle|wholesome|family[- ]friendly|kid[- ]friendly|for kids)` +
	`|(?:rejected|excluded|skipped|left out|dropped)` +
	`)\b`)

// rationaleRejectsPick reports whether the model's own reason for a pick argues against it.
func rationaleRejectsPick(rationale string) bool {
	return rationaleRejectionPattern.MatchString(rationale)
}

// softToneCues are the words of a request that asks for a gentle channel. Horror cannot satisfy
// any of them, whatever its setting.
var softToneCues = regexp.MustCompile(`\b(?:cozy|cosy|gentle|wholesome|comfort|comforting|feel[- ]good|heartwarming|soothing|calm)\b`)

// contradictsSoftTone reports whether a grounded candidate is Horror while the request names a
// soft tone. A title the request names itself is the user's call, so it is never dropped here.
// A kids request alone is not a soft tone: kids' horror exists, and a rating cap ("tense movies
// rated PG or lower") can ask for threat. Ratings stay with the audience ceiling (over_ceiling).
func contradictsSoftTone(intent Intent, cand titlesearch.Candidate) bool {
	for _, wanted := range intent.MustInclude {
		if sameExactTitle(cand.Name, wanted) {
			return false
		}
	}
	if !softToneCues.MatchString(affirmativeIntentText(intent)) {
		return false
	}
	return slices.ContainsFunc(cand.Genres, func(g string) bool { return strings.EqualFold(strings.TrimSpace(g), "Horror") })
}
