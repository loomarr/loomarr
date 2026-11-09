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
	// Language is the language a catalog answered in ("en-US"); empty for a source that has
	// one language.
	Language string
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
	// Studios are a series' networks or a movie's studios, in the source's order.
	Studios []string
	// Status is a series' status as its source reports it, such as "continuing" or "ended".
	Status string
	// Runtime is the nominal running time, not a measurement of a file.
	Runtime time.Duration
	// Tagline is the one-line pitch printed on a poster.
	Tagline string
	// Countries are the production countries, as the source names them.
	Countries []string
	// Trailer is a web page that plays the trailer: an http(s) URL.
	Trailer string
	// Edition labels the version of the title this file is, such as "Director's Cut" (decision
	// 54); empty is the theatrical version or unknown.
	Edition string
	// People are the cast, in billing order, then the key crew.
	People []Person
	// Seasons is a series' layout in the source's own numbering, specials (season 0) included.
	// It says how many episodes each season holds, never which file is which: TMDB and TVDB number
	// some shows differently.
	Seasons []SeasonSize
	// Orderings counts the other episode orderings the catalog knows for a series (TMDB's episode
	// groups: DVD, absolute, story arc). Nil is unknown. Only a series with none is numbered by
	// season and episode (#2151 decision 53).
	Orderings *int
}

// FilmCredit is one title in a person's filmography (#2151 decision 50): a movie or series by its
// id in the catalog the filmography came from, what they did on it, and an actor's character.
type FilmCredit struct {
	Kind      Kind
	ID        ExternalID
	Title     string
	Year      int
	Role      PersonRole
	Character string
}

// SeasonSize is one season of a series and how many episodes it has.
type SeasonSize struct {
	Number   int
	Episodes int
}

// PersonRole is what a person did on a title.
type PersonRole string

const (
	PersonActor    PersonRole = "actor"
	PersonDirector PersonRole = "director"
	PersonWriter   PersonRole = "writer"
	// PersonCreator created a series.
	PersonCreator PersonRole = "creator"
)

// Person is someone credited on a title (decision 49). ID is their id in a catalog of people when
// the source knows one; a person without one is kept by name, unlinked.
type Person struct {
	Name      string
	Role      PersonRole
	Character string
	ID        ExternalID
	// Photo is an http(s) URL of their headshot.
	Photo string
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
	// Season is the season a season poster (KindSeasonPoster) belongs to, 0 for the specials;
	// zero for any other kind.
	Season int `json:",omitempty"`
}

// Artwork kinds, named as the arrs and TMDB name them so precedence picks one per kind. Every
// provider maps what it is sent onto these; library_art.kind's CHECK lists exactly this set.
const (
	KindPoster    = "poster"
	KindFanart    = "fanart"
	KindClearlogo = "clearlogo"
	KindBanner    = "banner"
	KindLandscape = "landscape"
	KindClearart  = "clearart"
	KindDiscart   = "discart"
	// KindThumb is an episode's still.
	KindThumb = "thumb"
	// KindSeasonPoster is a series' poster for one of its seasons (#2151 decisions 45, 61).
	KindSeasonPoster = "season-poster"
)
