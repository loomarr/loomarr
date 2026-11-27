package filler

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// ClipDTO is the API view of a filler clip (§10). Identity is the content HASH (V38c/V45a) — the
// store has keyed on hash since V38c, and V45a makes the wire identity the hash too, so a
// slash-bearing path never crosses the API (the source of the routing/proxy 404 class).
type ClipDTO struct {
	// Hash is the clip's IDENTITY (its sparse content hash, V38c) — what every clip-addressing route
	// takes (tags, split, and the byte routes). Hex, no slashes, so it is a clean URL/route id.
	Hash string `json:"hash"`
	// TunarrProgramID is informational: it exists for Tunarr filler-lists and is empty on an
	// install with no Tunarr. NOT an identity — clients must key on Hash.
	TunarrProgramID string `json:"tunarrProgramId,omitempty"`
	Name            string `json:"name"`
	Role            string `json:"role" enum:"unclassified,commercial,bumper,station_id,psa,trailer,interstitial,political,news"`
	Era             int    `json:"era,omitempty"`
	Audience        string `json:"audience,omitempty" enum:"kids,family,general,late_night,"`
	GeographicScope string `json:"geographicScope,omitempty" enum:"unknown,national,local"`
	Country         string `json:"country,omitempty" doc:"ISO 3166-1 alpha-2 country when grounded"`
	Network         string `json:"network,omitempty"`
	Station         string `json:"station,omitempty" doc:"Broadcast station directory id (fcc:<facility> or wd:<item>); a local clip airs near it"`
	StationCallSign string `json:"stationCallSign,omitempty" example:"WEWS" doc:"The station's call sign on the air date"`
	StationLabel    string `json:"stationLabel,omitempty" example:"WEWS · Cleveland, OH" doc:"The station's call sign on the air date and its community of license"`
	AirDate         string `json:"airDate,omitempty" doc:"Grounded broadcast date in YYYY-MM-DD form"`
	GeoEvidence     string `json:"geographyEvidence,omitempty" doc:"Literal source evidence or operator attribution"`
	// Category is the DERIVED product-leaf shadow of Tags (§10 V45a) — kept for readers that show a
	// single primary category; `Tags` is the full taxonomy set and the source of truth.
	Category string `json:"category,omitempty" doc:"Derived primary product-leaf tag; the full set is in tags (§10 V45a)"`
	// Tags is the clip's taxonomy tag set (§10 V45a) — the full leaf+rollup expansion, so a client can
	// show every tag and a curation rule can match any ancestor. Empty for a clip not yet tagged.
	Tags []string `json:"tags,omitempty" doc:"The clip's taxonomy tags (leaf + rolled-up ancestors); category is the derived primary product leaf (§10 V45a)"`
	// AssertedTags is what the classifier/operator actually chose. Editors round-trip this field;
	// Tags includes derived ancestors and must never be sent back as assertions.
	AssertedTags []string      `json:"assertedTags,omitempty" doc:"Directly asserted taxonomy tags; excludes derived rollups and is the set clip editors should round-trip"`
	DurationMs   int64         `json:"durationMs"`
	Source       string        `json:"source,omitempty"`
	SourceLabel  string        `json:"sourceLabel,omitempty" doc:"Human-readable registered source name; use this instead of the canonical source identity in ordinary UI"`
	Media        *ClipMediaDTO `json:"media,omitempty" doc:"Prepared playback file facts; included only for an exact single-clip detail read"`
	// Quality is the resolution label ("1080p", "480p"); "" for an audio-only clip or one
	// scanned before the column existed.
	// ⚠ The `doc` string is a CLIENT-VISIBLE contract (it ships in api/openapi.yaml and the
	// generated TS), so it must say that quality affects selection when the opt-in
	// `filler.min_quality` floor is set; a "display-only" claim would be repeated by the generated
	// client.
	Quality string `json:"quality,omitempty" doc:"Resolution label from the probed video height (1080p, 480p). Display-only by default; affects selection only when the filler.min_quality floor is set, which is off unless an operator turns it on."`
	// ThumbImage / HoverImage are the clip's still and hover loop, as image-service records
	// (§22) — content-addressed, with a width ladder, a ThumbHash and honest immutable caching.
	//
	// ⚠ **These are the only way a clip's artwork is addressed on the wire**: there are no
	// `thumbnail`/`preview` string fields and no per-clip artwork routes. One way to address an
	// asset is the point of §22.
	//
	// ⚠ **Absence is ORDINARY and the client must treat it that way** — it is not an error and not
	// a reason to draw a placeholder. A clip whose artwork the adoption job has not reached yet has
	// neither field, and so does one where ffmpeg never ran (the entire catalog on a Tunarr-backed
	// install). The card's no-frame layout is the designed answer for both.
	//
	// The clip ROW still stores `thumbnail`/`preview` — the render pipeline writes files under
	// FILLER_DIR and the adoption job converts them. Those columns are the seam between the two,
	// deliberately not exposed here: a path relative to a cache directory is not something a
	// client can do anything with.
	ThumbImage *httpcore.ImageDTO `json:"thumbImage,omitempty" doc:"Image-service record for the still; absent until the adoption job has run, or when no frame was extracted"`
	HoverImage *httpcore.ImageDTO `json:"hoverImage,omitempty" doc:"Image-service record for the hover loop; absent until adopted, or when no animation was rendered"`
	// PlayCount / LastPlayedAt count airings on INTERNAL playout only.
	//
	// ⚠ PlaysCounted is what stops this being a lie. A Tunarr-backed channel airs its filler
	// through Tunarr, which never reports back, so those clips sit at 0 forever — and "0
	// plays" and "we cannot see plays here" are different facts. The UI must render the
	// second as "not counted", never as a zero.
	PlayCount    int64  `json:"playCount"`
	LastPlayedAt string `json:"lastPlayedAt,omitempty" doc:"RFC3339; absent if never played (or not counted)"`
	PlaysCounted bool   `json:"playsCounted" doc:"False when this install cannot observe airings (Tunarr-backed playout) — render as 'not counted', not as 0"`
	Tagged       bool   `json:"tagged" doc:"Whether the clip has all match tags (era+audience+category)"`
	// NeedsDetails is the server's verdict, so the Library's caution chip and its "Untagged only"
	// filter cannot disagree: both come from clipcatalog.Clip.NeedsDetails. A client must not
	// re-derive it from kind + tagged.
	NeedsDetails bool `json:"needsDetails" doc:"A commercial missing a match tag (era, audience or category) — the same clips the untagged filter selects. Render the 'Needs details' exception from this field; do not re-derive it."`
	// IsComposite marks a recorded break — many adverts in one file — which is NOT airable (§10 V45).
	// The client renders it as a container (the "COMPOSITE · NOT AIRABLE" row that expands to its
	// segments), never as a playable clip. Its segments (ParentHash == this clip's identity) are the
	// airable clips.
	IsComposite bool `json:"isComposite,omitempty" doc:"A recorded break (many ads in one file), not airable — render as a container whose segments are the airable clips (§10 V45)"`
	// ParentHash is the identity of the composite this clip was split out of (§10 V45), or absent for
	// a clip with no parent. The lineage link: a segment card shows "from <break>", and a composite's
	// segments are fetched by this value.
	ParentHash string `json:"parentHash,omitempty" doc:"The composite this clip was split from; absent for a non-segment clip (§10 V45)"`
	// Brand is the advertiser the clip is FOR (§10 V44) — grounded, so absent is the common and
	// honest case, never a gap to fill in with a guess.
	Brand string `json:"brand,omitempty" doc:"The advertiser, when it appears literally in a text or visual signal (§10 V44). Absent means no GROUNDED brand — never inferred."`
	// Held marks a clip recorded but NOT yet in the playable catalog (§10 V38).
	//
	// ⚠ Travels with the `includeHeld` parameter: a client that can ASK for held clips and cannot
	// TELL which ones they are renders an unreviewed clip identically to a filed one.
	Held bool `json:"held,omitempty" doc:"Waiting for review in Incoming, not in the playable catalog (§10 V38). Only ever true when includeHeld or heldOnly was passed."`
	// Language has THREE states and a client must not collapse them (§10 V40):
	// absent = NOT YET CHECKED (the detection job has not reached this clip; never a fault),
	// "none" = CHECKED and there is no speech to judge (a wordless visual spot, always kept),
	// "en"/… = CHECKED, and this is what was heard.
	Language string `json:"language,omitempty" doc:"Absent = not yet checked; 'none' = checked and wordless; otherwise the detected language (§10 V40). The first two are different facts."`
	// License is the licence URL the SOURCE declared.
	//
	// ⚠ **"" means UNKNOWN, never "public domain".** About 92% of archive.org items declare no
	// licence at all, so an empty value is the COMMON case and carries no permission. This field
	// is a record of what a source claimed, not a rights determination, and it never gates
	// selection or playback.
	License string `json:"license,omitempty" doc:"The licence URL the source declared. ⚠ Absent means UNKNOWN, never public domain — most archive.org items declare none."`
	// HasTranscript reports whether a transcript exists, without shipping it.
	//
	// ⚠ Deliberately NOT the transcript itself: it is kilobytes per clip that no grid renders, so
	// at a 100-row page it would be roughly ten times the rest of the payload. The detail surface
	// fetches the text; the listing only needs to know there is some.
	HasTranscript     bool                      `json:"hasTranscript,omitempty" doc:"Whether a transcript exists (§10 V44). The text itself is a detail-surface read — kilobytes per clip that no grid renders."`
	SourceURL         string                    `json:"sourceUrl,omitempty" doc:"Original public item URL from hash-bound acquisition provenance. Available on exact single-hash reads; absent when unknown. Never inferred from a name or source ID."`
	Enrichment        *ClipEnrichmentDTO        `json:"enrichment,omitempty" doc:"Quiet server-owned detail state and provenance. Available on exact single-hash reads only; omitted from catalog pages."`
	ContextSuggestion *ClipContextSuggestionDTO `json:"contextSuggestion,omitempty" doc:"Cited likely era/geography context. It is not verified metadata and never affects scheduling. Available on exact single-hash reads only."`
}

// ClipMediaDTO is the small human-facing projection of the prepared playback lineage. The full
// lineage remains in the portable sidecar; the API does not re-probe media during a detail read.
type ClipMediaDTO struct {
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	FrameRate     string `json:"frameRate,omitempty" doc:"Exact ffprobe cadence, usually a rational such as 24000/1001"`
	Container     string `json:"container,omitempty"`
	VideoCodec    string `json:"videoCodec,omitempty"`
	AudioCodec    string `json:"audioCodec,omitempty"`
	AudioChannels int    `json:"audioChannels,omitempty"`
	AudioRateHz   int    `json:"audioRateHz,omitempty"`
	Bytes         int64  `json:"bytes,omitempty"`
}

type ClipEnrichmentDTO struct {
	State string                  `json:"state" enum:"adding_details,details_limited,complete"`
	Facts []ClipEnrichmentFactDTO `json:"facts,omitempty"`
}

type ClipEnrichmentFactDTO struct {
	Axis     string `json:"axis" enum:"role,era,audience,brand,geography,language,product,format,seasonal,audience-cue,presentation"`
	Evidence string `json:"evidence" enum:"inference,source_default,trusted_mapping,content_observation,item_metadata,operator"`
}

type ClipContextSuggestionDTO struct {
	Year        int                    `json:"year,omitempty"`
	Decade      int                    `json:"decade,omitempty"`
	CountryCode string                 `json:"countryCode,omitempty"`
	Country     string                 `json:"country,omitempty"`
	Confidence  int                    `json:"confidence"`
	Explanation string                 `json:"explanation,omitempty"`
	Sources     []ClipContextSourceDTO `json:"sources"`
}

type ClipContextSourceDTO struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func clipContextSuggestionDTO(report research.Report, verifiedEra int, verifiedCountry string) *ClipContextSuggestionDTO {
	suggestion := report.Suggestion
	if verifiedEra > 0 {
		suggestion.Year, suggestion.Decade = 0, 0
	}
	if verifiedCountry != "" {
		suggestion.CountryCode, suggestion.Country = "", ""
	}
	if suggestion.Year == 0 && suggestion.Decade == 0 && suggestion.Country == "" {
		return nil
	}
	// Sources is never null on the wire: an uncited observation has none, and the client reads
	// its length.
	detail := &ClipContextSuggestionDTO{Year: suggestion.Year, Decade: suggestion.Decade,
		CountryCode: suggestion.CountryCode, Country: suggestion.Country,
		Confidence: suggestion.Confidence, Explanation: suggestion.Explanation,
		Sources: []ClipContextSourceDTO{}}
	for _, citation := range report.Cited() {
		detail.Sources = append(detail.Sources, ClipContextSourceDTO{Title: citation.Title, URL: citation.URL})
	}
	return detail
}

func clipEnrichmentDTO(states []enrichment.State) *ClipEnrichmentDTO {
	projection := enrichment.ProjectDetails(states)
	detail := &ClipEnrichmentDTO{State: string(projection.State)}
	if len(projection.Facts) > 0 {
		detail.Facts = make([]ClipEnrichmentFactDTO, len(projection.Facts))
		for i, fact := range projection.Facts {
			detail.Facts[i] = ClipEnrichmentFactDTO{Axis: string(fact.Axis), Evidence: string(fact.Evidence)}
		}
	}
	return detail
}

// playsCounted reports whether THIS install can observe a filler clip airing.
//
// It is exactly "does internal playout run here": the resolver is what records a play (see
// playoutresolve.Resolver.airingFiller), and a Tunarr-backed install has none — its filler is aired by
// Tunarr, which never reports back. Deriving it from the same field the program route checks
// means the flag cannot drift from the behaviour it describes.
func (s *Handlers) playsCounted() bool { return s.playsCountedOn }

// clipPathByHash resolves a clip's content HASH (the wire identity, V45a) to its disk path (the disk
// LOCATION, server-internal). The media byte route takes the hash in the URL and needs
// the path to find the file. Returns ("", false) for a missing clip or a nil/unconfigured store — the
// caller renders that as an ordinary 404, never an error (a missing clip and a missing file are
// the same non-event to the client). ⚠ One store read per byte request: the tradeoff for a single
// wire identity (V45a) — the path never crosses the API, so no slash-in-URL encoding hazard remains.
func (s *Handlers) clipPathByHash(ctx context.Context, hash string) (string, bool) {
	if s.store == nil || hash == "" {
		return "", false
	}
	clip, err := s.store.GetClip(ctx, hash)
	if err != nil || clip.Path == "" {
		return "", false
	}
	return clip.Path, true
}

// clipArtworkResolver pre-resolves every artwork image for a page of clips in one pass.
//
// ⚠ **Called ONCE per page, before the render loop — never inside it.** The filler catalog is a
// grid, and each tile has a still AND a hover loop, so a per-row lookup is two queries per tile:
// a 200-clip page would be 400 round trips to render one screen. This is the same N+1 the channel
// list already avoids, one surface over, and it matters more here because the page is bigger.
func (s *Handlers) clipArtworkResolver(ctx context.Context, clips []clipstore.Clip) func(string) *httpcore.ImageDTO {
	if s.imageRecords == nil || len(clips) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(clips)*2)
	for _, c := range clips {
		hashes = append(hashes, c.ThumbImageHash, c.HoverImageHash)
	}
	byHash := s.imageDTOsByHash(ctx, hashes)
	if len(byHash) == 0 {
		return nil
	}
	return func(hash string) *httpcore.ImageDTO {
		if hash == "" {
			return nil
		}
		return byHash[hash]
	}
}

// clipToDTO renders a clip for the wire. `img`, when non-nil, resolves an image hash to its
// record — pre-resolved by the caller for the whole page, never looked up per clip (see
// imageDTOsByHash: the catalog is a grid, so a per-row lookup is two queries per tile).
// `playsCounted` comes from the caller because it is a property of the INSTALL (does internal
// playout run here?), not of the clip.
func clipToDTO(c clipstore.Clip, playsCounted bool, img func(string) *httpcore.ImageDTO) ClipDTO {
	d := ClipDTO{
		Hash: c.Hash, TunarrProgramID: c.TunarrProgramID, Name: c.Name, Role: string(c.Role),
		Era: c.Era, Audience: string(c.Audience), Category: c.Category, Tags: c.Tags, AssertedTags: c.AssertedTags,
		GeographicScope: string(c.GeographicScope), Country: c.Country,
		Network: c.Network, Station: c.Station, AirDate: c.AirDate, GeoEvidence: c.GeoEvidence,
		DurationMs: c.DurationMs, Source: c.Source, Quality: c.Quality,
		PlayCount: c.PlayCount, PlaysCounted: playsCounted,
		Tagged: c.Tagged(), NeedsDetails: c.NeedsDetails(),
		IsComposite: c.IsComposite, ParentHash: c.ParentHash,
		Brand: c.Brand, Held: c.Held, Language: c.Language,
		License: c.License,
		// ⚠ The PRESENCE of the transcript, not the transcript. See the field's own warning.
		HasTranscript: c.Transcript != "",
	}
	if !c.LastPlayedAt.IsZero() {
		d.LastPlayedAt = c.LastPlayedAt.UTC().Format(time.RFC3339)
	}
	if img != nil {
		d.ThumbImage = img(c.ThumbImageHash)
		d.HoverImage = img(c.HoverImageHash)
	}
	return d
}

func clipMediaDTO(tags asset.SidecarTags) *ClipMediaDTO {
	if tags.MediaAssets == nil || tags.MediaAssets.Playback == nil {
		return nil
	}
	playback := tags.MediaAssets.Playback
	return &ClipMediaDTO{
		Width: playback.OutputProbe.Width, Height: playback.OutputProbe.Height,
		FrameRate: playback.OutputProbe.Cadence, Bytes: playback.Asset.Bytes,
		Container: playback.Recipe.Container, VideoCodec: playback.Recipe.VideoCodec,
		AudioCodec: playback.Recipe.AudioCodec, AudioChannels: playback.Recipe.AudioChannels,
		AudioRateHz: playback.Recipe.AudioRateHz,
	}
}
