package library

// ItemMetadata is the display metadata for one Library item: what the guide's listing (§9.1), a
// lineup's durations and a proposal's outlook read. Since #2151 step 16 it is filled from the
// Library's tables (LibraryItemFacts), never from the media server.
type ItemMetadata struct {
	// Overview is the plot description. XMLTV's `<desc>`, and the field that makes a guide
	// entry worth selecting.
	Overview string
	Genres   []string
	Year     int
	// OfficialRating is the content rating ("TV-PG", "R"). Already carried on a LineupEntry for
	// the SERIES, but an episode carries its own where the Library has one.
	OfficialRating string
	// RuntimeMs is the item's own runtime.
	//
	// Distinct from a slot's DurationMs, which is how long the slot OCCUPIES the timeline.
	// They normally agree, and where they do not the difference is the interesting part — a
	// guide entry claiming 22m in a 30m slot is how you see padding.
	RuntimeMs int64
}
