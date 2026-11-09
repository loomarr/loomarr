// Package metadataprovider is the contract every metadata provider implements: a system that
// already knows what a media file is and what to say about it, such as Sonarr and Radarr, a Kodi
// NFO beside the file, or a reference catalog like TMDB (#2151, decision 0046).
//
// A provider declares what it can do as capabilities: the item kinds it describes, the external-id
// namespaces it can fetch by, the field families it supplies, the kind of evidence it is (its
// basis), and how hard it may be called. It fetches one item by id, and may list everything it
// holds or report what changed. Which provider wins a field is decided in one place, the
// precedence package, over those capabilities; no code compares provider ids as strings.
//
// The package imports nothing from Loomarr, so the importer, the API and every provider package
// can all name it without importing each other.
package metadataprovider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ID names a provider. It is stable: it is recorded as the provenance of every field a provider
// supplies and is published on the wire, so a provider never changes its ID. The set is closed:
// a provider is added here and in IDs together, and the registry refuses any other.
type ID string

const (
	IDSonarr   ID = "sonarr"
	IDRadarr   ID = "radarr"
	IDNFO      ID = "nfo"
	IDTMDB     ID = "tmdb"
	IDLocalArt ID = "localart"
)

// IDs returns every provider ID, in a stable order. It is the closed enum the API publishes.
func IDs() []ID { return []ID{IDSonarr, IDRadarr, IDNFO, IDTMDB, IDLocalArt} }

// ParseID returns the provider ID s names, or an error when it names none.
func ParseID(s string) (ID, error) {
	for _, id := range IDs() {
		if string(id) == s {
			return id, nil
		}
	}
	return "", fmt.Errorf("metadataprovider: unknown provider %q", s)
}

// Kind is the kind of item a record describes.
type Kind string

const (
	KindMovie   Kind = "movie"
	KindSeries  Kind = "series"
	KindEpisode Kind = "episode"
	// KindCollection is a catalog's set of movies, such as a TMDB collection (a franchise). Its
	// record's Children are the set's movies in the catalog's order.
	KindCollection Kind = "collection"
	// KindSeason is one season of a series in a catalog's numbering. Its record's Children are the
	// season's episodes, numbered; it is fetched by "<series id>/<season>".
	KindSeason Kind = "season"
	// KindEpisodeOrder is one of a series' other episode orders (an EpisodeOrder), fetched by its
	// id. Its record's Children are the order's episodes numbered as the order numbers them, and
	// its Descriptive.Seasons are the order's seasons and their sizes.
	KindEpisodeOrder Kind = "episode_order"
)

// Family is a group of fields that share one precedence rule.
type Family string

const (
	// FamilyTechnical is what the file is: duration, codecs, dimensions, languages.
	FamilyTechnical Family = "technical"
	// FamilyIdentity is which title the file is: external ids, the current file, and numbering.
	FamilyIdentity Family = "identity"
	// FamilyDescriptive is the guide text: titles, overview, dates, genres, network.
	FamilyDescriptive Family = "descriptive"
	// FamilyCertification is the content rating.
	FamilyCertification Family = "certification"
	// FamilyArtwork is references to posters, fanart and logos, never their bytes.
	FamilyArtwork Family = "artwork"
)

// Families returns every field family, in a stable order.
func Families() []Family {
	return []Family{FamilyTechnical, FamilyIdentity, FamilyDescriptive, FamilyCertification, FamilyArtwork}
}

// Basis is the kind of evidence a record is. Precedence ranks bases, never providers, so a new
// provider takes its place in every rule by declaring its basis.
type Basis string

const (
	// BasisMeasured is a measurement of the whole file by Loomarr.
	BasisMeasured Basis = "measured"
	// BasisHeader is Loomarr's read of the container header.
	BasisHeader Basis = "header"
	// BasisManager is the download manager that placed the file and tracks it (Sonarr, Radarr).
	BasisManager Basis = "manager"
	// BasisEmbeddedID is an id written in a folder or file name, such as {tmdb-123}.
	BasisEmbeddedID Basis = "embedded_id"
	// BasisSidecar is a file beside the media, such as a Kodi NFO.
	BasisSidecar Basis = "sidecar"
	// BasisLink is a link the maintainer confirmed.
	BasisLink Basis = "link"
	// BasisSearch is a catalog search the scanner trusted: exactly one result agreed on the
	// title's name, year and length (decision 56). It ranks last: any id beats it.
	BasisSearch Basis = "search"
	// BasisCatalog is a reference catalog queried by id, such as TMDB or TVDB.
	BasisCatalog Basis = "catalog"
)

// Namespace is an external-id namespace. It is open-ended: a provider may report ids in any
// namespace it knows, and the ones Loomarr bridges by are named here.
type Namespace string

const (
	NamespaceTMDB Namespace = "tmdb"
	NamespaceTVDB Namespace = "tvdb"
	NamespaceIMDB Namespace = "imdb"
	// NamespaceTMDBCollection is a TMDB movie collection (a franchise). Its ids are not TMDB
	// movie ids.
	NamespaceTMDBCollection Namespace = "tmdb_collection"
	// NamespaceFile is a local media path. A provider that reads files beside the media, rather
	// than querying a service, resolves it.
	NamespaceFile Namespace = "file"
	// NamespaceSonarr is Sonarr's own id for an episode, as Sonarr writes it into the NFO beside
	// the file. Loomarr grounds nothing by it; two catalogs that read the same NFO share it.
	NamespaceSonarr Namespace = "sonarr"
)

// Grounds reports whether an id in n says which title an item is, in a namespace catalogs share
// (TMDB, TVDB, IMDb): what grounds a title and what two catalogs can agree on. A collection's id,
// a file path or an arr's own episode id does not. A namespace Loomarr bridges titles by is added
// here, not where ids are read.
func (n Namespace) Grounds() bool {
	switch n {
	case NamespaceTMDB, NamespaceTVDB, NamespaceIMDB:
		return true
	}
	return false
}

// ExternalID is one grounded identity of an item in a namespace.
type ExternalID struct {
	Namespace Namespace
	Value     string
}

// Ref asks a provider for one item.
type Ref struct {
	Kind Kind
	ID   ExternalID
}

// Resolvable is one (kind, namespace) pair a provider can fetch by.
type Resolvable struct {
	Kind      Kind
	Namespace Namespace
}

// RateHint is how a caller should pace a provider. A zero field declares no limit.
type RateHint struct {
	// MaxConcurrent is the most fetches to have in flight at once.
	MaxConcurrent int
	// MinInterval is the least time between two requests.
	MinInterval time.Duration
	// PreferList is true when listing everything costs less than fetching items one by one.
	PreferList bool
}

// Capabilities is what a provider declares about itself. Every record it returns stays inside
// them: its kinds, its families, its basis and, when declared, its catalog.
type Capabilities struct {
	Basis Basis
	// Catalog is the namespace whose catalog the provider's descriptive text comes from, such as
	// TVDB for Sonarr. Empty means it varies by record, as it does for an NFO, and each record says.
	Catalog  Namespace
	Kinds    []Kind
	Resolves []Resolvable
	Families []Family
	Rate     RateHint
}

// CanResolve reports whether a provider with these capabilities can fetch ref.
func (c Capabilities) CanResolve(ref Ref) bool {
	for _, r := range c.Resolves {
		if r.Kind == ref.Kind && r.Namespace == ref.ID.Namespace {
			return true
		}
	}
	return false
}

// Supplies reports whether a provider with these capabilities supplies family.
func (c Capabilities) Supplies(family Family) bool {
	for _, f := range c.Families {
		if f == family {
			return true
		}
	}
	return false
}

// Provider is one system that knows about media. Its optional capabilities are the interfaces
// below it implements (Lister, ChangeFeed); a caller asks with a type assertion.
type Provider interface {
	ID() ID
	Capabilities() Capabilities
	// Fetch returns the item ref names. A series comes with its episodes as Children when the
	// provider holds them. It returns ErrNotFound when the provider holds no such item,
	// ErrUnsupported when ref is outside its capabilities, and ErrNotConfigured when its
	// connection is not set.
	Fetch(ctx context.Context, ref Ref) (Record, error)
}

// Lister returns every item a provider holds, for a full pull. A pull that read some items and
// failed on others returns both the items and a non-nil error; the caller decides what a partial
// pull may change.
type Lister interface {
	List(ctx context.Context) ([]Record, error)
}

// ChangeFeed reports which items changed since a time. A cursor older than Window cannot be
// answered from the feed, and the caller refetches everything instead.
type ChangeFeed interface {
	Window() time.Duration
	Changes(ctx context.Context, since time.Time) ([]Ref, error)
}

var (
	ErrNotFound      = errors.New("metadataprovider: item not found")
	ErrUnsupported   = errors.New("metadataprovider: request outside the provider's capabilities")
	ErrNotConfigured = errors.New("metadataprovider: provider not configured")
)
