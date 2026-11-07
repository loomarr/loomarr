package metadataprovider

import "time"

// Source is where a record comes from. Provider is empty for evidence Loomarr produces itself
// (a header read, a measurement, an id in a name, a confirmed link); Basis is always set.
type Source struct {
	Provider ID
	Basis    Basis
	// Catalog is the namespace whose catalog the record's descriptive text comes from, or empty
	// when it is not known.
	Catalog Namespace
}

// Record is what one source says about one item, grouped by field family. A nil family pointer
// means the source says nothing about that family, which is different from saying it is empty.
type Record struct {
	Source Source
	Kind   Kind
	// IDs are the item's own grounded ids: an episode's are its own episode ids, never its
	// series' (decision 15).
	IDs []ExternalID
	// Parent are the ids of the series an episode belongs to.
	Parent []ExternalID
	// Files are the media files the source says are this item, current file first.
	Files     []FileRef
	Numbering *Numbering

	Descriptive   *Descriptive
	Certification *Certification
	Technical     *Technical
	Artwork       []ArtworkRef

	// Children are a series' episodes when the source returned them with the series, or a
	// collection's movies, in its order.
	Children []Record
	// More are further items of the same kind that the same ref names: the other episodes of a
	// multi-episode file, each with its own ids (decision 15).
	More []Record

	// Page is the item's page in the source's own app, for a person to open: where "Wrong
	// title?" sends them when the source decides what the item is (decision 26). Empty when the
	// source has no such page.
	Page string
}

// Families returns the field families the record says something about.
func (r Record) Families() []Family {
	var out []Family
	if r.Technical != nil {
		out = append(out, FamilyTechnical)
	}
	if len(r.IDs) > 0 || len(r.Parent) > 0 || len(r.Files) > 0 || r.Numbering != nil {
		out = append(out, FamilyIdentity)
	}
	if r.Descriptive != nil {
		out = append(out, FamilyDescriptive)
	}
	if r.Certification != nil {
		out = append(out, FamilyCertification)
	}
	if len(r.Artwork) > 0 {
		out = append(out, FamilyArtwork)
	}
	return out
}

// CollectionRef names a franchise in a catalog. The ID's namespace is the catalog's collection
// namespace, not an item namespace.
type CollectionRef struct {
	ID   ExternalID
	Name string
}

// FileRef is a media file a source associates with an item, in the source's own path space.
type FileRef struct {
	Path      string
	SizeBytes int64
}

// Numbering places an episode in its series. Season 0 holds specials. The Airs fields are the
// broadcast position of a special between regular episodes (decision 38); zero means none.
type Numbering struct {
	Season     int
	Episode    int
	EpisodeEnd int // the last episode of a multi-episode file, or zero
	Absolute   int

	AirsAfterSeason   int
	AirsBeforeSeason  int
	AirsBeforeEpisode int
}

// Descriptive is guide text. Dates are YYYY-MM-DD.
type Descriptive struct {
	Title         string
	OriginalTitle string
	SortTitle     string
	Overview      string
	PremiereDate  string
	Year          int
	Genres        []string
	// Keywords are the catalog's topic tags for the item. They stand in for the tags a media
	// server keeps.
	Keywords []string
	// CommunityRating is the audience score on a 0-10 scale, or zero when the source has no votes.
	CommunityRating float64
	// Collection is the franchise a movie belongs to in the source's catalog, or nil.
	Collection *CollectionRef
	// Network is a series' network or a movie's studio.
	Network string
	// Status is a series' status as its source reports it, such as "continuing" or "ended".
	Status string
	// Runtime is the nominal running time, not a measurement of a file.
	Runtime time.Duration
}

// Certification is a content rating as one source states it. Country is the ISO 3166-1 alpha-2
// code of the rating system, upper case, or empty when the source does not say.
type Certification struct {
	Rating  string
	Country string
}

// Technical is what a source says the file is.
type Technical struct {
	Duration          time.Duration
	SizeBytes         int64
	Container         string
	VideoCodec        string
	Width             int
	Height            int
	HDR               bool
	Interlaced        bool
	AudioCodec        string
	AudioChannels     float64
	AudioLanguages    []string
	SubtitleLanguages []string
}

// ArtworkRef points at one image. Ref is a URL or a path; artwork bytes are never kept here.
type ArtworkRef struct {
	Kind string
	Ref  string
}
