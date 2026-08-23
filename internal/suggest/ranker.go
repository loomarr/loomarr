package suggest

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// FeedbackSignal is the ranker's store-independent view of one effective
// explicit household signal. Passive playback never enters this interface.
type FeedbackSignal struct {
	Target provision.Key
	Action FeedbackAction
}

type FeedbackAction string

const (
	FeedbackKeep     FeedbackAction = "keep"
	FeedbackLess     FeedbackAction = "less"
	FeedbackNever    FeedbackAction = "never"
	FeedbackSurprise FeedbackAction = "surprise"
)

type rankedCandidate struct {
	candidate  catalog.Candidate
	key        provision.Key
	relevance  int
	preference int
	novelty    int
}

// RankGroundedCandidates is the one pure ordering seam for discovery. Relevance
// is the primary sort key, so a taste signal cannot promote an unrelated title
// above a relevant one. Feedback and novelty only order candidates within the
// same relevance band; identity is the final deterministic tie-break.
func RankGroundedCandidates(intent string, candidates []catalog.Candidate, signals []FeedbackSignal) []catalog.Candidate {
	effective := make(map[provision.Key]FeedbackAction, len(signals))
	lessGenres := map[string]bool{}
	byKey := make(map[provision.Key]catalog.Candidate, len(candidates))
	for _, candidate := range candidates {
		if key, err := candidate.Key(); err == nil {
			byKey[key] = candidate
		}
	}
	for _, signal := range signals {
		effective[signal.Target] = signal.Action
		if signal.Action == FeedbackLess {
			for _, genre := range byKey[signal.Target].Genres {
				lessGenres[strings.ToLower(genre)] = true
			}
		}
	}
	query := wordSet(intent)
	ranked := make([]rankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key, err := candidate.Key()
		if err != nil || effective[key] == FeedbackNever {
			continue
		}
		preference := 0
		switch effective[key] {
		case FeedbackKeep:
			preference += 3
		case FeedbackLess:
			preference -= 4
		case FeedbackSurprise:
			preference += 3
		}
		for _, genre := range candidate.Genres {
			if lessGenres[strings.ToLower(genre)] && effective[key] != FeedbackSurprise {
				preference--
				break
			}
		}
		novelty := 0
		if !candidate.InLibrary {
			novelty = 1
		}
		ranked = append(ranked, rankedCandidate{candidate: candidate, key: key,
			relevance:  overlap(query, wordSet(candidate.Name+" "+candidate.Overview+" "+strings.Join(candidate.Genres, " "))),
			preference: preference, novelty: novelty})
	}
	slices.SortFunc(ranked, func(a, b rankedCandidate) int {
		if order := cmp.Compare(b.relevance, a.relevance); order != 0 {
			return order
		}
		if order := cmp.Compare(b.preference, a.preference); order != 0 {
			return order
		}
		if order := cmp.Compare(b.novelty, a.novelty); order != 0 {
			return order
		}
		return cmp.Compare(string(a.key), string(b.key))
	})
	out := make([]catalog.Candidate, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.candidate)
	}
	return out
}

func wordSet(text string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := make(map[string]bool, len(words))
	for _, word := range words {
		if strings.HasSuffix(word, "ies") && len(word) > 4 {
			word = strings.TrimSuffix(word, "ies") + "y"
		} else if strings.HasSuffix(word, "s") && len(word) > 3 {
			word = strings.TrimSuffix(word, "s")
		}
		out[word] = true
	}
	return out
}

func overlap(a, b map[string]bool) int {
	n := 0
	for word := range a {
		if b[word] {
			n++
		}
	}
	return n
}
