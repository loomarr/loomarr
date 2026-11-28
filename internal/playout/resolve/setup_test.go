package resolve

// setSubtitles gives a hand-built resolver the subtitle readers New takes from Config.
func setSubtitles(r *Resolver, tracks LibrarySubtitleReader, cues SubtitleCues) {
	r.librarySubtitles, r.subtitleCues = tracks, cues
}
