// Package inventory owns Loomarr's durable, provider-neutral understanding of media (design §5,
// V66). Importers describe what they observed; consumers resolve stable items and sources without
// receiving Emby/Jellyfin response types or operational credentials.
package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type (
	ItemID      string
	SourceID    string
	AuthorityID string
	ItemKind    string
	SourceKind  string
	StreamKind  string
	Coverage    string
)

const (
	CoveragePresent Coverage = "present"
	CoverageEmpty   Coverage = "empty"

	SourceLibraryOriginal SourceKind = "library_original"
	SourceLocalFile       SourceKind = "local_file"

	StreamVideo    StreamKind = "video"
	StreamAudio    StreamKind = "audio"
	StreamSubtitle StreamKind = "subtitle"
)

var (
	ErrInvalid            = errors.New("inventory: invalid document")
	ErrIdentityConflict   = errors.New("inventory: identity conflict")
	ErrSourceRevisionGone = errors.New("inventory: source revision changed")
)

// OriginKey is the exact importer identity of an item. Authority identifies one configured server
// or scanner root; ExternalItemID is stable only within that authority.
type OriginKey struct {
	Authority      AuthorityID `json:"authority"`
	ExternalItemID string      `json:"externalItemId"`
}

// ExternalID is a grounded cross-provider identity asserted by one Origin.
type ExternalID struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
}

// Person is someone credited on a title (#2151 decision 49). Role is what they did, lower case
// (actor, director, writer, creator, or what the source calls it); Character is an actor's part.
// ID is their id in a catalog of people when the source knows one; one without is kept by name,
// unlinked.
type Person struct {
	Name      string     `json:"name"`
	Role      string     `json:"role,omitempty"`
	Character string     `json:"character,omitempty"`
	ID        ExternalID `json:"id,omitzero"`
	// Photo is an http(s) URL of their headshot.
	Photo string `json:"photo,omitempty"`
}

type Rating struct {
	System string  `json:"system"`
	Value  float64 `json:"value"`
}

type ArtworkRef struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	// Season is the season a season poster belongs to, 0 for the specials; zero for other kinds.
	Season int `json:"season,omitempty"`
}

type Relationship struct {
	Kind       string `json:"kind"`
	ExternalID string `json:"externalId"`
}

// ItemFacts intentionally retains more than today's consumers need. Typed fields are the stable
// projection Loomarr understands; Observation.Extension preserves bounded safe importer fields.
type ItemFacts struct {
	Name          string   `json:"name,omitempty"`
	OriginalTitle string   `json:"originalTitle,omitempty"`
	SortTitle     string   `json:"sortTitle,omitempty"`
	Overview      string   `json:"overview,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Studios       []string `json:"studios,omitempty"`
	Countries     []string `json:"countries,omitempty"`
	Tagline       string   `json:"tagline,omitempty"`
	// Trailer is a web page that plays the trailer.
	Trailer        string         `json:"trailer,omitempty"`
	People         []Person       `json:"people,omitempty"`
	ProductionYear int            `json:"productionYear,omitempty"`
	SeasonNumber   int            `json:"seasonNumber,omitempty"`
	EpisodeNumber  int            `json:"episodeNumber,omitempty"`
	EpisodeEnd     int            `json:"episodeEnd,omitempty"`
	PremiereDate   string         `json:"premiereDate,omitempty"`
	OfficialRating string         `json:"officialRating,omitempty"`
	CommunityScore float64        `json:"communityScore,omitempty"`
	RuntimeMillis  int64          `json:"runtimeMillis,omitempty"`
	Artwork        []ArtworkRef   `json:"artwork,omitempty"`
	Relationships  []Relationship `json:"relationships,omitempty"`
}

type Disposition struct {
	Default bool `json:"default,omitempty"`
	Forced  bool `json:"forced,omitempty"`
	// Comment is a commentary track; HearingImpaired one for the deaf and hard of hearing (SDH);
	// VisualImpaired one that describes the picture.
	Comment         bool `json:"comment,omitempty"`
	HearingImpaired bool `json:"hearingImpaired,omitempty"`
	VisualImpaired  bool `json:"visualImpaired,omitempty"`
}

// Stream is ordered by Index, the source-global stream index. Audio consumers derive ffmpeg's
// 0:a:N ordinal from the ordered audio subset rather than treating the global index as an ordinal.
type Stream struct {
	Index          int         `json:"index"`
	Kind           StreamKind  `json:"kind"`
	Codec          string      `json:"codec,omitempty"`
	Profile        string      `json:"profile,omitempty"`
	Level          string      `json:"level,omitempty"`
	Language       string      `json:"language,omitempty"`
	Title          string      `json:"title,omitempty"`
	Disposition    Disposition `json:"disposition,omitempty"`
	Channels       int         `json:"channels,omitempty"`
	ChannelLayout  string      `json:"channelLayout,omitempty"`
	SampleRate     int         `json:"sampleRate,omitempty"`
	// BitRate is the track's own rate in bit/s as the container states it (Matroska BPS tag) or
	// its sample table measures it (MP4); 0 when the container says nothing (#3228).
	BitRate int `json:"bitRate,omitempty"`
	// BitDepth is bits per sample: a video's luma depth, or a lossless or PCM audio track's
	// sample size; 0 when the stream has none to state (lossy audio) or it is unknown (#3228).
	BitDepth int `json:"bitDepth,omitempty"`
	Width          int         `json:"width,omitempty"`
	Height         int         `json:"height,omitempty"`
	FrameRate      string      `json:"frameRate,omitempty"`
	PixelFormat    string      `json:"pixelFormat,omitempty"`
	ColorSpace     string      `json:"colorSpace,omitempty"`
	ColorTransfer  string      `json:"colorTransfer,omitempty"`
	ColorPrimaries string      `json:"colorPrimaries,omitempty"`
	HDR            bool        `json:"hdr,omitempty"`
	// DolbyVision is a video stream's Dolby Vision profile, 0 for none.
	DolbyVision    int    `json:"dolbyVision,omitempty"`
	Interlaced     bool   `json:"interlaced,omitempty"`
	SubtitleFormat string `json:"subtitleFormat,omitempty"`
}

type SourceFacts struct {
	Protocol         string   `json:"protocol,omitempty"`
	Container        string   `json:"container,omitempty"`
	SizeBytes        int64    `json:"sizeBytes,omitempty"`
	Bitrate          int64    `json:"bitrate,omitempty"`
	DurationMillis   int64    `json:"durationMillis,omitempty"`
	ModifiedUnixNano int64    `json:"modifiedUnixNano,omitempty"`
	UnsafePreroll    bool     `json:"unsafePreroll,omitempty"`
	Streams          []Stream `json:"streams,omitempty"`
	// Chapters are the container's chapter marks, in order; nil when it has none or they were not
	// read.
	Chapters []Chapter `json:"chapters,omitempty"`
}

// Chapter is one chapter mark. EndMillis is -1 when the container does not say.
type Chapter struct {
	StartMillis int64  `json:"startMillis"`
	EndMillis   int64  `json:"endMillis"`
	Title       string `json:"title,omitempty"`
}

// Locator is protected operational data, never an authenticated URL. Library sources retain only
// stable external identity; Source Access reconstructs credentials at use time. Local paths are
// protected at export/log boundaries in the same way as other operational paths.
type Locator struct {
	Authority        AuthorityID `json:"authority,omitempty"`
	ExternalItemID   string      `json:"externalItemId,omitempty"`
	ExternalSourceID string      `json:"externalSourceId,omitempty"`
	Path             string      `json:"path,omitempty"`
}

// Observation records current imported facts. Coverage keys distinguish unknown (key absent) from
// explicitly observed empty (CoverageEmpty).
type Observation[T any] struct {
	SchemaVersion int                 `json:"schemaVersion"`
	ObservedAt    time.Time           `json:"observedAt"`
	Coverage      map[string]Coverage `json:"coverage,omitempty"`
	Facts         T                   `json:"facts"`
	Extension     json.RawMessage     `json:"extension,omitempty"`
}

type SourceSnapshot struct {
	ExternalSourceID string                   `json:"externalSourceId"`
	Kind             SourceKind               `json:"kind"`
	Revision         string                   `json:"revision"`
	Locator          Locator                  `json:"locator"`
	Observation      Observation[SourceFacts] `json:"observation"`
}

// Snapshot is one importer's atomic current observation of an item and its observed sources.
type Snapshot struct {
	Origin      OriginKey              `json:"origin"`
	Kind        ItemKind               `json:"kind"`
	Observation Observation[ItemFacts] `json:"observation"`
	ExternalIDs []ExternalID           `json:"externalIds,omitempty"`
	Sources     []SourceSnapshot       `json:"sources,omitempty"`
}

type ItemOrigin struct {
	Key         OriginKey              `json:"key"`
	Observation Observation[ItemFacts] `json:"observation"`
	LastSeenAt  time.Time              `json:"lastSeenAt"`
	MissingAt   time.Time              `json:"missingAt,omitempty"`
}

type SourceOrigin struct {
	Key              OriginKey                `json:"key"`
	ExternalSourceID string                   `json:"externalSourceId"`
	Locator          Locator                  `json:"locator"`
	Observation      Observation[SourceFacts] `json:"observation"`
	LastSeenAt       time.Time                `json:"lastSeenAt"`
	MissingAt        time.Time                `json:"missingAt,omitempty"`
}

type Measurement struct {
	SourceID    SourceID                 `json:"sourceId"`
	Revision    string                   `json:"revision"`
	Observation Observation[SourceFacts] `json:"observation"`
}

// NewFactsMeasurement is the measurement of one source revision's stream facts, observed at `at`.
// It is the one place that says what such a measurement covers: the streams were observed, and the
// audio streams are present or, when the file has none, explicitly empty, so a source with no audio
// is a known answer rather than an unmeasured one.
func NewFactsMeasurement(id SourceID, revision string, at time.Time, facts SourceFacts) Measurement {
	audio := CoverageEmpty
	for _, stream := range facts.Streams {
		if stream.Kind == StreamAudio {
			audio = CoveragePresent
			break
		}
	}
	return Measurement{
		SourceID: id, Revision: revision,
		Observation: Observation[SourceFacts]{
			SchemaVersion: 1, ObservedAt: at,
			Coverage: map[string]Coverage{"streams": CoveragePresent, "audioStreams": audio},
			Facts:    facts,
		},
	}
}

type Source struct {
	ID          SourceID       `json:"id"`
	ItemID      ItemID         `json:"itemId"`
	Kind        SourceKind     `json:"kind"`
	Revision    string         `json:"revision"`
	Origins     []SourceOrigin `json:"origins"`
	Measurement *Measurement   `json:"measurement,omitempty"`
}

type Item struct {
	ID          ItemID       `json:"id"`
	Kind        ItemKind     `json:"kind"`
	Origins     []ItemOrigin `json:"origins"`
	ExternalIDs []ExternalID `json:"externalIds,omitempty"`
	Sources     []Source     `json:"sources,omitempty"`
}

type ItemRef struct {
	ID     ItemID     `json:"id,omitempty"`
	Origin *OriginKey `json:"origin,omitempty"`
}

type SourceRequest struct {
	Item             ItemRef       `json:"item"`
	Kinds            []SourceKind  `json:"kinds,omitempty"`
	Now              time.Time     `json:"now"`
	MaxAge           time.Duration `json:"maxAge,omitempty"`
	RequiredCoverage []string      `json:"requiredCoverage,omitempty"`
}

// ResolvedSource is a playable source and its facts: the importer's observation, or a measurement
// of this exact revision when that is fresher. A measurement of an older revision never reaches it.
type ResolvedSource struct {
	ID          SourceID                 `json:"id"`
	ItemID      ItemID                   `json:"itemId"`
	Kind        SourceKind               `json:"kind"`
	Revision    string                   `json:"revision"`
	Locator     Locator                  `json:"-"`
	Observation Observation[SourceFacts] `json:"observation"`
}

// Service is the consumer-facing Media Inventory port from design §5 V66.
type Service interface {
	ApplySnapshot(context.Context, Snapshot) (ItemID, error)
	Item(context.Context, ItemRef) (Item, bool, error)
	ResolveSource(context.Context, SourceRequest) (ResolvedSource, bool, error)
	RecordMeasurement(context.Context, Measurement) error
	MarkUnseen(context.Context, AuthorityID, time.Time, []OriginKey) error
}

// Repository is the durable command/read seam implemented by store. It remains aggregate-shaped so
// callers cannot partially update the six-table representation.
type Repository interface {
	ApplyInventorySnapshot(context.Context, Snapshot) (ItemID, error)
	InventoryItem(context.Context, ItemRef) (Item, bool, error)
	RecordInventoryMeasurement(context.Context, Measurement) error
	MarkInventoryUnseen(context.Context, AuthorityID, time.Time, []OriginKey) error
}
