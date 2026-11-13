package suggest

import (
	"context"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/library/tmdb"
)

// bareProperNameNamesSet fails closed only when the request field itself is an
// otherwise ambiguous title-cased label. It intentionally does not scan spans:
// title examples, exclusions, and ordinary prose remain fuzzy discovery.
// A direct exact catalog title can still become user-supplied membership
// evidence during grounding; a merely thematic catalog hit cannot.
func bareProperNameNamesSet(text string) bool {
	name := strings.Trim(strings.TrimSpace(text), ".,;:!?()[]{}\"'")
	if name == "" || name == "The" || name == strings.ToUpper(name) || tmdb.IsKnownGenre(name) ||
		curatedKnownGenreInField(name) || themeLabel(name) ||
		bareProperNamePattern.FindString(name) != name {
		return false
	}
	return true
}

// themeLabelWords are the words a channel theme is typed with: formats, seasons and holidays,
// times of day and week, audiences and moods. A capitalised label made only of these and genres
// ("Holiday Films", "Saturday Morning Cartoons", "Space Westerns") is a theme someone typed in
// title case, not the proper name of a set, so it stays fuzzy discovery (#3363). One word outside
// the list ("Must See TV", "Studio Ghibli", "Classic Mudpuddles") keeps the named-set reading.
var themeLabelWords = map[string]bool{
	// Formats.
	"film": true, "films": true, "movie": true, "movies": true, "show": true, "shows": true,
	"series": true, "tv": true, "television": true, "cartoon": true, "cartoons": true,
	"sitcom": true, "sitcoms": true, "special": true, "specials": true, "episode": true,
	"episodes": true, "classic": true, "classics": true, "favorite": true, "favorites": true,
	"favourite": true, "favourites": true, "marathon": true, "marathons": true, "features": true,
	"shorts": true, "anthology": true, "miniseries": true, "flicks": true, "blockbusters": true,
	// Seasons and holidays.
	"holiday": true, "holidays": true, "christmas": true, "xmas": true, "halloween": true,
	"thanksgiving": true, "easter": true, "valentine's": true, "hanukkah": true, "new": true,
	"year's": true, "summer": true, "winter": true, "spring": true, "autumn": true, "fall": true,
	"festive": true, "spooky": true, "seasonal": true,
	// Times of day and week.
	"morning": true, "mornings": true, "afternoon": true, "afternoons": true, "evening": true,
	"evenings": true, "night": true, "nights": true, "late": true, "midnight": true,
	"weekend": true, "weekends": true, "saturday": true, "sunday": true, "friday": true,
	"daytime": true, "primetime": true,
	// Audiences, moods and settings. Words that also make up common titles ("Old School", "Feel
	// Good") stay out, so such a title still reads as one.
	"kids": true, "kid": true, "family": true, "teen": true, "cozy": true, "cosy": true,
	"retro": true, "vintage": true, "space": true, "feel-good": true, "funny": true, "scary": true,
	"&": true,
}

// themeLabel reports whether every word of a bare label is a theme word or a genre (in either
// number: "Westerns", "Comedies").
func themeLabel(name string) bool {
	words := strings.Fields(name)
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		w := strings.ToLower(strings.Trim(word, ".,;:!?()[]{}\"'"))
		if themeLabelWords[w] || tmdb.IsKnownGenre(w) || tmdb.IsKnownGenre(strings.TrimSuffix(w, "s")) ||
			(strings.HasSuffix(w, "ies") && tmdb.IsKnownGenre(strings.TrimSuffix(w, "ies")+"y")) {
			continue
		}
		return false
	}
	return true
}

// curatedTitleSubject extracts a proper-name title adjacent to the existing
// curated-episode cue vocabulary. Description and refinement are considered
// independently so a cue in one field cannot rewrite another field's subject.
func curatedTitleSubject(intent Intent) (string, bool) {
	for _, field := range []string{intent.Description, intent.RefineText} {
		if subject, found := curatedTitleSubjectInField(field); found {
			return subject, true
		}
	}
	return "", false
}

func curatedTitleSubjectInField(field string) (string, bool) {
	subject, found := curatedCueSubjectInField(field)
	if !found || subject == "The" || tmdb.IsKnownGenre(subject) || bareProperNamePattern.FindString(subject) != subject {
		return "", false
	}
	return subject, true
}

// curatedKnownGenreInField recognizes the field-local genre form of a curated
// cue so it remains ordinary discovery rather than a named set.
func curatedKnownGenreInField(field string) bool {
	subject, found := curatedCueSubjectInField(field)
	return found && tmdb.IsKnownGenre(subject)
}

func curatedCueSubjectInField(field string) (string, bool) {
	words := strings.Fields(strings.Trim(strings.TrimSpace(field), ".,;:!?()[]{}\"'"))
	if len(words) < 2 {
		return "", false
	}
	if last := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:!?()[]{}\"'")); last == "episode" || last == "episodes" {
		words = words[:len(words)-1]
	}
	if len(words) < 2 {
		return "", false
	}
	isCue := func(word string) bool {
		word = strings.ToLower(strings.Trim(word, ".,;:!?()[]{}\"'"))
		for _, cue := range curatedEpisodeCues {
			if word == cue {
				return true
			}
		}
		return false
	}
	var subjectWords []string
	if isCue(words[0]) {
		subjectWords = words[1:]
	} else if isCue(words[len(words)-1]) {
		subjectWords = words[:len(words)-1]
	} else {
		return "", false
	}
	subject := strings.Trim(strings.Join(subjectWords, " "), ".,;:!?()[]{}\"'")
	if subject == "" {
		return "", false
	}
	return subject, true
}

func sameCuratedTitleSubject(candidate, subject string) bool {
	if sameExactTitle(candidate, subject) {
		return true
	}
	trimThe := func(value string) string {
		fields := strings.Fields(value)
		if len(fields) > 1 && strings.EqualFold(fields[0], "the") {
			return strings.Join(fields[1:], " ")
		}
		return value
	}
	return sameExactTitle(trimThe(candidate), trimThe(subject))
}

func unambiguousCuratedTitleCandidate(candidates []titlesearch.Candidate, subject string) (titlesearch.Candidate, bool) {
	byKey := make(map[provision.Key]titlesearch.Candidate)
	for _, candidate := range candidates {
		if !sameCuratedTitleSubject(candidate.Name, subject) {
			continue
		}
		if key, err := candidate.Key(); err == nil {
			byKey[key] = candidate
		}
	}
	if len(byKey) != 1 {
		return titlesearch.Candidate{}, false
	}
	for _, candidate := range byKey {
		return candidate, true
	}
	return titlesearch.Candidate{}, false
}

func (s *Suggester) groundCuratedTitleSubject(ctx context.Context, intent *Intent) ([]titlesearch.Candidate, error) {
	subject, found := curatedTitleSubject(*intent)
	if !found {
		return nil, nil
	}
	intent.curatedTitleSet = true
	if intent.membershipSources.searches >= maxMembershipSourceQueries {
		return nil, nil
	}
	intent.membershipSources.searches++
	candidates, err := s.catalog.Search(ctx, subject, titlesearch.ScopeAll, catalogSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("search curated title subject %q: %w", subject, err)
	}
	candidate, found := unambiguousCuratedTitleCandidate(candidates, subject)
	if !found {
		return nil, nil
	}
	key, _ := candidate.Key()
	intent.curatedTitleKey = key
	intent.membershipKeys[key] = true
	return []titlesearch.Candidate{candidate}, nil
}
