package suggest

import (
	"encoding/json"
	"fmt"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/provision"
)

// pick is one entry the model returns in its final JSON. It is UNTRUSTED — the
// ids are re-grounded against surfaced candidates before use (§8).
type pick struct {
	MediaType  string  `json:"mediaType"`
	TMDBID     int     `json:"tmdbId"`
	TVDBID     int     `json:"tvdbId"`
	Name       string  `json:"name"`
	Rationale  string  `json:"rationale"`
	Confidence float64 `json:"confidence"`
}

// key derives the provisioning key a pick claims. Empty when the pick has no
// usable id (which makes it ungrounded → dropped).
func (p pick) key() string {
	mt := provision.MediaType(p.MediaType)
	if !mt.Valid() {
		return ""
	}
	t := provision.Title{MediaType: mt, TMDBID: p.TMDBID, TVDBID: p.TVDBID, Name: p.Name}
	k, err := t.Key()
	if err != nil {
		return ""
	}
	return string(k)
}

// finalOutput is the model's final JSON shape (§8 output contract, simplified to
// picks + rationale; the suggester classifies picks into lineup/acquisitions by
// their in_library flag, so the model needn't).
type finalOutput struct {
	Rationale string `json:"rationale"`
	Picks     []pick `json:"picks"`
}

// parsePicks parses the model's final JSON. A malformed final output is an error
// (the job fails cleanly rather than emitting a garbage proposal).
func parsePicks(content string) ([]pick, string, error) {
	var out finalOutput
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, "", fmt.Errorf("suggester: model final output is not valid JSON: %w", err)
	}
	return out.Picks, out.Rationale, nil
}

// toolResult is the JSON the catalog tool returns to the model — a trimmed
// candidate view with real ids and the in_library flag. Untrusted text from the
// library/TMDB (names) rides here but can never steer tools or reach secrets
// (§8): it is data the model selects from, not instructions.
type toolCandidate struct {
	MediaType string `json:"mediaType"`
	TMDBID    int    `json:"tmdbId,omitempty"`
	TVDBID    int    `json:"tvdbId,omitempty"`
	Name      string `json:"name"`
	Year      int    `json:"year,omitempty"`
	InLibrary bool   `json:"inLibrary"`
}

func toolResult(cands []catalog.Candidate) []toolCandidate {
	out := make([]toolCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, toolCandidate{
			MediaType: string(c.MediaType), TMDBID: c.TMDBID, TVDBID: c.TVDBID,
			Name: c.Name, Year: c.Year, InLibrary: c.InLibrary,
		})
	}
	return out
}
