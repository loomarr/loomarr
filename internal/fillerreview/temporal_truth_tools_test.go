package fillerreview

import "testing"

func TestTemporalTruthReviewVideoPinsInputAndOutputThreads(t *testing.T) {
	arguments := temporalTruthReviewVideoArguments("source.ogv", 0, 1_000, "review.mp4")
	input := argumentIndex(arguments, "-i")
	if input < 0 || !hasArgumentPair(arguments[:input], "-threads", "1") {
		t.Fatalf("input decoder is not single-threaded: %v", arguments)
	}
	if !hasArgumentPair(arguments[input+1:], "-threads", "1") {
		t.Fatalf("output encoder is not single-threaded: %v", arguments)
	}
}

func argumentIndex(arguments []string, wanted string) int {
	for index, argument := range arguments {
		if argument == wanted {
			return index
		}
	}
	return -1
}

func hasArgumentPair(arguments []string, first, second string) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == first && arguments[index+1] == second {
			return true
		}
	}
	return false
}
