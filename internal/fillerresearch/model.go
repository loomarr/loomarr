// Package fillerresearch owns bounded external context lookup for publicly sourced filler.
//
// It deliberately returns suggestions rather than fillerenrichment.State. That separation is the
// authority boundary: campaign context can help a person understand a clip, but it cannot become a
// verified scheduling fact merely because a model found a plausible page.
package fillerresearch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	MaxAdapterResults = 3
	MaxCitations      = 5
	MaxExtractBytes   = 5_000
	MaxPacketBytes    = 20_000
	MaxExplanationLen = 600
)

var ErrInvalid = errors.New("invalid filler context research")

// Input contains only metadata already published by a registered remote source. Callers must not
// put a local filename, private library title, transcript, or household data in this envelope.
type Input struct {
	ClipHash      string
	InputRevision int64
	Title         string
	Description   string
	SourceKind    string
	SourceID      string
	SourceURL     string
	KnownEra      int
	KnownCountry  string
}

func (in Input) Validate() error {
	if strings.TrimSpace(in.ClipHash) == "" || strings.TrimSpace(in.Title) == "" ||
		strings.TrimSpace(in.SourceID) == "" {
		return fmt.Errorf("%w: clip, public title, and source id are required", ErrInvalid)
	}
	switch strings.ToLower(strings.TrimSpace(in.SourceKind)) {
	case "archive", "archive.org", "youtube":
		if strings.TrimSpace(in.SourceURL) == "" {
			return nil
		}
		if !validPublicSourceURL(in.SourceKind, in.SourceURL) {
			return fmt.Errorf("%w: source URL is not canonical for %q", ErrInvalid, in.SourceKind)
		}
		return nil
	default:
		return fmt.Errorf("%w: source %q is not a public lookup source", ErrInvalid, in.SourceKind)
	}
}

func validPublicSourceURL(kind, raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "archive", "archive.org":
		return (host == "archive.org" || host == "www.archive.org") && strings.HasPrefix(u.EscapedPath(), "/details/")
	case "youtube":
		return host == "youtube.com" || host == "www.youtube.com" || host == "m.youtube.com" || host == "youtu.be"
	default:
		return false
	}
}

// Citation is one page Loomarr retrieved. ID is packet-local and is the only value an interpreter
// may return; the model never returns a URL for Loomarr to fetch or store.
type Citation struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Extract string `json:"extract"`
}

type Packet struct {
	Query          string     `json:"query"`
	Adapter        string     `json:"adapter"`
	AdapterVersion string     `json:"adapterVersion"`
	RetrievedAt    time.Time  `json:"retrievedAt"`
	Citations      []Citation `json:"citations"`
}

func (p Packet) Validate() error {
	if strings.TrimSpace(p.Query) == "" || strings.TrimSpace(p.Adapter) == "" ||
		strings.TrimSpace(p.AdapterVersion) == "" || p.RetrievedAt.IsZero() {
		return fmt.Errorf("%w: packet identity is required", ErrInvalid)
	}
	if len(p.Citations) == 0 || len(p.Citations) > MaxCitations {
		return fmt.Errorf("%w: packet has %d citations", ErrInvalid, len(p.Citations))
	}
	total := 0
	seen := make(map[int]bool, len(p.Citations))
	for _, citation := range p.Citations {
		u, err := url.Parse(citation.URL)
		if citation.ID < 1 || seen[citation.ID] || strings.TrimSpace(citation.Title) == "" ||
			err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("%w: malformed citation", ErrInvalid)
		}
		seen[citation.ID] = true
		if len(citation.Extract) > MaxExtractBytes {
			return fmt.Errorf("%w: citation extract is too large", ErrInvalid)
		}
		total += len(citation.Extract)
	}
	if total > MaxPacketBytes {
		return fmt.Errorf("%w: evidence packet is too large", ErrInvalid)
	}
	return nil
}

type Suggestion struct {
	Year        int    `json:"year,omitempty"`
	Decade      int    `json:"decade,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
	Country     string `json:"country,omitempty"`
	Confidence  int    `json:"confidence,omitempty"`
	Explanation string `json:"explanation,omitempty"`
	CitationIDs []int  `json:"citationIds,omitempty"`
}

// Report is the durable, non-authorizing result. ProducerVersion binds the interpreting model and
// prompt while Packet binds the independently owned retrieval adapter.
type Report struct {
	ClipHash        string     `json:"clipHash"`
	InputRevision   int64      `json:"inputRevision"`
	Producer        string     `json:"producer"`
	ProducerVersion string     `json:"producerVersion"`
	CompletedAt     time.Time  `json:"completedAt"`
	Suggestion      Suggestion `json:"suggestion"`
	Packet          Packet     `json:"packet"`
}

func (r Report) Validate() error {
	if strings.TrimSpace(r.ClipHash) == "" || r.InputRevision < 1 || strings.TrimSpace(r.Producer) == "" ||
		strings.TrimSpace(r.ProducerVersion) == "" || r.CompletedAt.IsZero() {
		return fmt.Errorf("%w: report identity is required", ErrInvalid)
	}
	if err := r.Packet.Validate(); err != nil {
		return err
	}
	s := r.Suggestion
	if s.Year != 0 && (s.Year < 1880 || s.Year > 2100) {
		return fmt.Errorf("%w: suggested year is outside the supported range", ErrInvalid)
	}
	if s.Decade != 0 && (s.Decade < 1880 || s.Decade > 2100 || s.Decade%10 != 0) {
		return fmt.Errorf("%w: suggested decade is invalid", ErrInvalid)
	}
	if s.Year != 0 && s.Decade != 0 && s.Year/10*10 != s.Decade {
		return fmt.Errorf("%w: suggested year and decade disagree", ErrInvalid)
	}
	if s.CountryCode != "" && (len(s.CountryCode) != 2 || s.CountryCode != strings.ToUpper(s.CountryCode)) {
		return fmt.Errorf("%w: country code must be uppercase ISO alpha-2", ErrInvalid)
	}
	if s.Confidence < 0 || s.Confidence > 80 || len(s.Explanation) > MaxExplanationLen {
		return fmt.Errorf("%w: suggestion confidence or explanation is invalid", ErrInvalid)
	}
	known := make(map[int]bool, len(r.Packet.Citations))
	for _, citation := range r.Packet.Citations {
		known[citation.ID] = true
	}
	seen := make(map[int]bool, len(s.CitationIDs))
	for _, id := range s.CitationIDs {
		if !known[id] || seen[id] {
			return fmt.Errorf("%w: suggestion cites evidence outside its packet", ErrInvalid)
		}
		seen[id] = true
	}
	if (s.Year != 0 || s.Decade != 0 || s.CountryCode != "" || s.Country != "") && len(s.CitationIDs) == 0 {
		return fmt.Errorf("%w: a contextual claim requires a citation", ErrInvalid)
	}
	return nil
}

func (r Report) Cited() []Citation {
	wanted := make(map[int]bool, len(r.Suggestion.CitationIDs))
	for _, id := range r.Suggestion.CitationIDs {
		wanted[id] = true
	}
	out := make([]Citation, 0, len(wanted))
	for _, citation := range r.Packet.Citations {
		if wanted[citation.ID] {
			out = append(out, citation)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func boundCitations(citations []Citation) []Citation {
	out := make([]Citation, 0, min(MaxCitations, len(citations)))
	remaining := MaxPacketBytes
	for _, citation := range citations {
		if len(out) == MaxCitations || remaining <= 0 {
			break
		}
		if len(citation.Extract) > MaxExtractBytes {
			citation.Extract = citation.Extract[:MaxExtractBytes]
		}
		if len(citation.Extract) > remaining {
			citation.Extract = citation.Extract[:remaining]
		}
		citation.ID = len(out) + 1
		remaining -= len(citation.Extract)
		out = append(out, citation)
	}
	return out
}

// Lookup is the bounded public search envelope. Adapters may derive provider-specific queries from
// it, but cannot receive local paths, transcripts, or arbitrary fetch targets.
type Lookup struct {
	Title       string
	Description string
}

func (l Lookup) Validate() error {
	if strings.TrimSpace(l.Title) == "" || len(l.Title) > 240 || len(l.Description) > 2_000 {
		return fmt.Errorf("%w: lookup title or description is invalid", ErrInvalid)
	}
	return nil
}

func (l Lookup) CanonicalTitle() string { return strings.Join(strings.Fields(l.Title), " ") }

// Terms is the complete deterministic search plan. The literal provider title preserves exact
// matches while Subject removes presentation noise that can bury the advertiser or product.
func (l Lookup) Terms() []string {
	literal, subject := l.CanonicalTitle(), l.Subject()
	if subject == "" || strings.EqualFold(literal, subject) {
		return []string{literal}
	}
	return []string{literal, subject}
}

// Subject returns a conservative subject query by removing presentation words that commonly bury
// the actual brand/product in remote-source titles. It never adds a model-authored term or URL.
func (l Lookup) Subject() string {
	generic := map[string]bool{
		"ad": true, "ads": true, "advert": true, "adverts": true, "advertisement": true,
		"advertisements": true, "classic": true, "commercial": true, "commercials": true,
		"retro": true, "television": true, "tv": true, "vintage": true,
	}
	words := strings.Fields(l.CanonicalTitle())
	kept := make([]string, 0, len(words))
	for _, word := range words {
		trimmed := strings.Trim(word, ".,:;!?()[]{}\"'")
		if trimmed == "" || generic[strings.ToLower(trimmed)] {
			continue
		}
		kept = append(kept, trimmed)
		if len(kept) == 12 {
			break
		}
	}
	if len(kept) == 0 {
		return l.CanonicalTitle()
	}
	return strings.Join(kept, " ")
}

type Retriever interface {
	Identity() (adapter, version string)
	Retrieve(ctx context.Context, lookup Lookup) (Packet, error)
}
