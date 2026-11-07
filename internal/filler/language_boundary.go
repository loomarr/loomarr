package filler

// The commercial language boundary (§10, #2030 decisions 12–15). A channel airs clips in its
// commercial language plus wordless ones; when those run out the card airs, never a clip in
// another language. Pins skip it, as they skip every matching rule (pickPinned).
//
// A clip whose language is undetermined (the detector could not tell, or has not run) airs only
// where the channel's language is the household default: the intake gate kept it under that
// default, and unknown metadata never satisfies an explicit choice.

// commercialLanguage is the channel's commercial language: its own, else the household's.
func commercialLanguage(w Window, policy Policy) string {
	if lang := NormalizeLanguage(w.Language); lang != "" {
		return lang
	}
	return NormalizeLanguage(policy.Language)
}

// speaksTheChannelsLanguage reports whether c may air on the channel.
func speaksTheChannelsLanguage(c Clip, w Window, policy Policy) bool {
	want := commercialLanguage(w, policy)
	if want == "" {
		return true // no language anywhere: nothing to hold the clip to
	}
	switch got := NormalizeLanguage(c.Language); got {
	case LangNone:
		return true
	case LangUndetermined:
		return want == NormalizeLanguage(policy.Language)
	default:
		return got == want
	}
}

func filterLanguage(clips []Clip, w Window, policy Policy) []Clip {
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if speaksTheChannelsLanguage(c, w, policy) {
			out = append(out, c)
		}
	}
	return out
}
