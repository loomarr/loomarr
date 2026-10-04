package storagegovernor

// SizeConfidence says how far a byte count shown to a human can be trusted (#1817). It is one
// vocabulary for every approval surface that states what approving will occupy: a request's
// owned titles and a filler pull's clips read the same shape, so the UI renders one way.
//
// There is no "unknown" value. A size nobody can state is an ABSENT field, never a zero byte
// count and never a third confidence: "0 B" reads as an empty file, and an "unknown" enum
// beside a meaningless number invites a renderer to show the number anyway.
type SizeConfidence string

const (
	// SizeExact is a byte count the media server or provider declared for the file itself.
	SizeExact SizeConfidence = "exact"
	// SizeEstimated is derived (approximate provider metadata, or duration × bitrate) and may
	// differ from what lands on disk.
	SizeEstimated SizeConfidence = "estimated"
)

// SizeConfidences is the closed set, in declaration order, for the wire schema.
func SizeConfidences() []SizeConfidence { return []SizeConfidence{SizeExact, SizeEstimated} }
