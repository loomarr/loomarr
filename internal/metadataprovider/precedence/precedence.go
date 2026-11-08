// Package precedence decides which source's value Loomarr uses for each field of a media item
// (#2151 decision 7, ADR 0046). It is the one place precedence lives. Every rule ranks the basis a
// record declares (measured, header, manager, sidecar, catalog …) and, for guide text, the catalog
// it comes from; no rule names a provider.
//
//   - Technical facts: measured, then header, then the manager's summary.
//   - Identity and numbering: manager, then ids embedded in names, then sidecar, then a confirmed
//     link.
//   - Descriptive text: a service (catalog or manager) whose text comes from TMDB for movies and
//     series, or from TVDB for episodes; then any other service; then sidecars. Within each tier,
//     catalog before manager.
//   - Certification: the strictest grounded value on the US ladder. An episode with no rating of
//     its own inherits its series' strictest; one a grounded source rates itself keeps that
//     rating, stricter or not (decision 17).
//   - Artwork: sidecar, then catalog, then manager, per artwork kind (decision 19).
//
// A field is filled from the highest-ranked source that has a value for it, so a lower source
// fills gaps but never overrides. Numbering and the file list move as a whole from one source.
package precedence

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/rating"
)

// ErrCountryUnsupported refuses a certification country with no rating ladder (decision 21).
var ErrCountryUnsupported = errors.New("precedence: no rating ladder for this country")

// ladderCountry is the one country whose ladder exists: MPAA film ratings and the TV Parental
// Guidelines, as schedule ranks them for enforcement.
const ladderCountry = "US"

var (
	technicalOrder     = []mp.Basis{mp.BasisMeasured, mp.BasisHeader, mp.BasisManager}
	identityOrder      = []mp.Basis{mp.BasisManager, mp.BasisEmbeddedID, mp.BasisSidecar, mp.BasisLink}
	descriptiveOrder   = []mp.Basis{mp.BasisCatalog, mp.BasisManager, mp.BasisSidecar}
	certificationOrder = []mp.Basis{mp.BasisCatalog, mp.BasisManager, mp.BasisSidecar}
	artworkOrder       = []mp.Basis{mp.BasisSidecar, mp.BasisCatalog, mp.BasisManager}
)

// Resolver applies the precedence rules for one installation's certification country.
type Resolver struct {
	country string
}

// New returns a resolver for country (ISO 3166-1 alpha-2). Only the US has a ladder; any other
// country is refused rather than rated against the wrong ladder.
func New(country string) (*Resolver, error) {
	c := strings.ToUpper(strings.TrimSpace(country))
	if c != ladderCountry {
		return nil, fmt.Errorf("%w: %q", ErrCountryUnsupported, country)
	}
	return &Resolver{country: c}, nil
}

// Input is everything known about one item. Records of another kind are ignored. Series is used
// only for an episode: it is everything known about the episode's series.
type Input struct {
	Kind    mp.Kind
	Records []mp.Record
	Series  []mp.Record
}

// Picked is a resolved value and the source it came from. A zero From means nothing supplied it.
type Picked[T any] struct {
	Value T
	From  mp.Source
}

// Known reports whether any source supplied the value.
func (p Picked[T]) Known() bool { return p.From.Basis != "" }

// Certification is a resolved rating on the ladder. Inherited is true when an episode took it
// from its series.
type Certification struct {
	Rating    string
	Inherited bool
}

// Resolved is one item's values after precedence, each with its provenance.
type Resolved struct {
	Kind mp.Kind

	IDs       []Picked[mp.ExternalID]
	Parent    []Picked[mp.ExternalID]
	Files     Picked[[]mp.FileRef]
	Numbering Picked[mp.Numbering]

	Title         Picked[string]
	OriginalTitle Picked[string]
	SortTitle     Picked[string]
	Overview      Picked[string]
	PremiereDate  Picked[string]
	Year          Picked[int]
	Genres        Picked[[]string]
	Keywords      Picked[[]string]
	Network       Picked[string]
	Status        Picked[string]
	Runtime       Picked[time.Duration]
	Tagline       Picked[string]
	Countries     Picked[[]string]
	Trailer       Picked[string]
	Edition       Picked[string]
	People        Picked[[]mp.Person]

	CommunityRating Picked[float64]
	Collection      Picked[mp.CollectionRef]

	Certification Picked[Certification]

	Technical Technical

	Artwork []Picked[mp.ArtworkRef]
}

// Technical is the resolved technical facts. HDR and Interlaced come from the highest-ranked
// technical record, since a false there is an answer, not a gap.
type Technical struct {
	Duration          Picked[time.Duration]
	SizeBytes         Picked[int64]
	Container         Picked[string]
	VideoCodec        Picked[string]
	Width             Picked[int]
	Height            Picked[int]
	HDR               Picked[bool]
	Interlaced        Picked[bool]
	AudioCodec        Picked[string]
	AudioChannels     Picked[float64]
	AudioLanguages    Picked[[]string]
	SubtitleLanguages Picked[[]string]
}

// Resolve applies every rule to in.
func (r *Resolver) Resolve(in Input) Resolved {
	records := ofKind(in.Records, in.Kind)
	out := Resolved{Kind: in.Kind}
	resolveIdentity(&out, ranked(records, identityOrder, hasIdentity))
	resolveDescriptive(&out, descriptiveRanked(in.Kind, records))
	resolveTechnical(&out, ranked(records, technicalOrder, func(rec mp.Record) bool { return rec.Technical != nil }))
	out.Artwork = resolveArtwork(ranked(records, artworkOrder, func(rec mp.Record) bool { return len(rec.Artwork) > 0 }))

	if cert, ok := r.strictest(records); ok {
		out.Certification = cert
	} else if in.Kind == mp.KindEpisode {
		if cert, ok := r.strictest(ofKind(in.Series, mp.KindSeries)); ok {
			cert.Value.Inherited = true
			out.Certification = cert
		}
	}
	return out
}

func ofKind(records []mp.Record, kind mp.Kind) []mp.Record {
	var out []mp.Record
	for _, rec := range records {
		if rec.Kind == kind {
			out = append(out, rec)
		}
	}
	return out
}

// ranked keeps the records whose basis is in order and that pass has, sorted by their basis's
// position in order. Records of one basis keep their input order.
func ranked(records []mp.Record, order []mp.Basis, has func(mp.Record) bool) []mp.Record {
	var out []mp.Record
	for _, rec := range records {
		if slices.Contains(order, rec.Source.Basis) && has(rec) {
			out = append(out, rec)
		}
	}
	slices.SortStableFunc(out, func(a, b mp.Record) int {
		return slices.Index(order, a.Source.Basis) - slices.Index(order, b.Source.Basis)
	})
	return out
}

// descriptiveRanked orders guide text in three tiers: a service (catalog or manager) whose text
// comes from the kind's preferred catalog, then any other service, then sidecars. Within a tier
// it ranks by basis, then by catalog. A sidecar's catalog is only the scraper that wrote it, so it
// orders sidecars among themselves but never lifts one above a service.
func descriptiveRanked(kind mp.Kind, records []mp.Record) []mp.Record {
	preferred := mp.NamespaceTMDB
	if kind == mp.KindEpisode {
		preferred = mp.NamespaceTVDB
	}
	out := ranked(records, descriptiveOrder, func(rec mp.Record) bool { return rec.Descriptive != nil })
	slices.SortStableFunc(out, func(a, b mp.Record) int {
		if d := descriptiveTier(a, preferred) - descriptiveTier(b, preferred); d != 0 {
			return d
		}
		if d := slices.Index(descriptiveOrder, a.Source.Basis) - slices.Index(descriptiveOrder, b.Source.Basis); d != 0 {
			return d
		}
		return catalogRank(a, preferred) - catalogRank(b, preferred)
	})
	return out
}

func descriptiveTier(rec mp.Record, preferred mp.Namespace) int {
	if rec.Source.Basis == mp.BasisSidecar {
		return 2
	}
	return catalogRank(rec, preferred)
}

func catalogRank(rec mp.Record, preferred mp.Namespace) int {
	if rec.Source.Catalog == preferred {
		return 0
	}
	return 1
}

func hasIdentity(rec mp.Record) bool {
	return len(rec.IDs) > 0 || len(rec.Parent) > 0 || len(rec.Files) > 0 || rec.Numbering != nil
}

func resolveIdentity(out *Resolved, records []mp.Record) {
	out.IDs = pickIDs(records, func(rec mp.Record) []mp.ExternalID { return rec.IDs })
	out.Parent = pickIDs(records, func(rec mp.Record) []mp.ExternalID { return rec.Parent })
	for _, rec := range records {
		if len(rec.Files) > 0 {
			out.Files = Picked[[]mp.FileRef]{Value: rec.Files, From: rec.Source}
			break
		}
	}
	for _, rec := range records {
		if rec.Numbering != nil {
			out.Numbering = Picked[mp.Numbering]{Value: *rec.Numbering, From: rec.Source}
			break
		}
	}
}

// pickIDs takes, per namespace, the value from the highest-ranked record that has one.
func pickIDs(records []mp.Record, get func(mp.Record) []mp.ExternalID) []Picked[mp.ExternalID] {
	var out []Picked[mp.ExternalID]
	seen := map[mp.Namespace]bool{}
	for _, rec := range records {
		for _, id := range get(rec) {
			if id.Value == "" || seen[id.Namespace] {
				continue
			}
			seen[id.Namespace] = true
			out = append(out, Picked[mp.ExternalID]{Value: id, From: rec.Source})
		}
	}
	return out
}

func resolveDescriptive(out *Resolved, records []mp.Record) {
	d := func(get func(*mp.Descriptive) string) Picked[string] {
		return first(records, func(rec mp.Record) string { return get(rec.Descriptive) })
	}
	out.Title = d(func(x *mp.Descriptive) string { return x.Title })
	out.OriginalTitle = d(func(x *mp.Descriptive) string { return x.OriginalTitle })
	out.SortTitle = d(func(x *mp.Descriptive) string { return x.SortTitle })
	out.Overview = d(func(x *mp.Descriptive) string { return x.Overview })
	out.PremiereDate = d(func(x *mp.Descriptive) string { return x.PremiereDate })
	out.Network = d(func(x *mp.Descriptive) string { return x.Network })
	out.Status = d(func(x *mp.Descriptive) string { return x.Status })
	out.Tagline = d(func(x *mp.Descriptive) string { return x.Tagline })
	out.Trailer = d(func(x *mp.Descriptive) string { return x.Trailer })
	out.Edition = d(func(x *mp.Descriptive) string { return x.Edition })
	out.Countries = firstSlice(records, func(rec mp.Record) []string { return rec.Descriptive.Countries })
	out.People = firstSlice(records, func(rec mp.Record) []mp.Person { return rec.Descriptive.People })
	out.Year = first(records, func(rec mp.Record) int { return rec.Descriptive.Year })
	out.Runtime = first(records, func(rec mp.Record) time.Duration { return rec.Descriptive.Runtime })
	out.Genres = firstSlice(records, func(rec mp.Record) []string { return rec.Descriptive.Genres })
	out.Keywords = firstSlice(records, func(rec mp.Record) []string { return rec.Descriptive.Keywords })
	out.CommunityRating = first(records, func(rec mp.Record) float64 { return rec.Descriptive.CommunityRating })
	for _, rec := range records {
		if c := rec.Descriptive.Collection; c != nil && c.ID.Value != "" {
			out.Collection = Picked[mp.CollectionRef]{Value: *c, From: rec.Source}
			break
		}
	}
}

func resolveTechnical(out *Resolved, records []mp.Record) {
	t := &out.Technical
	t.Duration = first(records, func(rec mp.Record) time.Duration { return rec.Technical.Duration })
	t.SizeBytes = first(records, func(rec mp.Record) int64 { return rec.Technical.SizeBytes })
	t.Container = first(records, func(rec mp.Record) string { return rec.Technical.Container })
	t.VideoCodec = first(records, func(rec mp.Record) string { return rec.Technical.VideoCodec })
	for _, rec := range records {
		// Dimensions are one fact: a width from one source and a height from another can
		// describe a picture no file has.
		if rec.Technical.Width > 0 && rec.Technical.Height > 0 {
			t.Width = Picked[int]{Value: rec.Technical.Width, From: rec.Source}
			t.Height = Picked[int]{Value: rec.Technical.Height, From: rec.Source}
			break
		}
	}
	t.AudioCodec = first(records, func(rec mp.Record) string { return rec.Technical.AudioCodec })
	t.AudioChannels = first(records, func(rec mp.Record) float64 { return rec.Technical.AudioChannels })
	t.AudioLanguages = firstSlice(records, func(rec mp.Record) []string { return rec.Technical.AudioLanguages })
	t.SubtitleLanguages = firstSlice(records, func(rec mp.Record) []string { return rec.Technical.SubtitleLanguages })
	if len(records) > 0 {
		top := records[0]
		t.HDR = Picked[bool]{Value: top.Technical.HDR, From: top.Source}
		t.Interlaced = Picked[bool]{Value: top.Technical.Interlaced, From: top.Source}
	}
}

func resolveArtwork(records []mp.Record) []Picked[mp.ArtworkRef] {
	var out []Picked[mp.ArtworkRef]
	seen := map[string]bool{}
	for _, rec := range records {
		for _, art := range rec.Artwork {
			if art.Ref == "" || seen[art.Kind] {
				continue
			}
			seen[art.Kind] = true
			out = append(out, Picked[mp.ArtworkRef]{Value: art, From: rec.Source})
		}
	}
	return out
}

// strictest returns the highest rating on the ladder among grounded records. A rating in another
// country's system, or one the ladder does not know (such as "NR"), grounds nothing. A rating whose
// source names no country is read as US when the ladder knows it, so an unprefixed NFO "PG" from
// another system reads as MPAA PG; that is the cost of NFOs that omit the country. Ties keep the
// higher-ranked source.
func (r *Resolver) strictest(records []mp.Record) (Picked[Certification], bool) {
	var best Picked[Certification]
	bestRank, found := -1, false
	for _, rec := range ranked(records, certificationOrder, func(rec mp.Record) bool { return rec.Certification != nil }) {
		country := strings.ToUpper(strings.TrimSpace(rec.Certification.Country))
		if country != "" && country != r.country {
			continue
		}
		normalized := rating.Normalize(rec.Certification.Rating)
		rank, ok := normalized.Rank()
		if !ok || rank <= bestRank {
			continue
		}
		best = Picked[Certification]{Value: Certification{Rating: string(normalized)}, From: rec.Source}
		bestRank, found = rank, true
	}
	return best, found
}

func first[T comparable](records []mp.Record, get func(mp.Record) T) Picked[T] {
	var zero T
	for _, rec := range records {
		if v := get(rec); v != zero {
			return Picked[T]{Value: v, From: rec.Source}
		}
	}
	return Picked[T]{}
}

func firstSlice[T any](records []mp.Record, get func(mp.Record) []T) Picked[[]T] {
	for _, rec := range records {
		if v := get(rec); len(v) > 0 {
			return Picked[[]T]{Value: v, From: rec.Source}
		}
	}
	return Picked[[]T]{}
}
