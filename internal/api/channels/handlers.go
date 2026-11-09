// Package channels serves the channel resource under /v1/channels: list, get, create, edit, refine,
// reconcile and delete, its icon, logo and watermark art, and the viewer-facing channel routes
// (Home's channel ideas, favourite and recent channels, and household viewing).
package channels

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

// Store is the slice of the aggregate store these routes read and write.
type Store interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	GetChannel(ctx context.Context, id string) (channelstore.Channel, error)
	GetChannelByNumber(ctx context.Context, number int) (channelstore.Channel, error)
	SaveChannel(ctx context.Context, ch channelstore.Channel) (channelstore.Channel, error)
	ChannelRequesters(ctx context.Context) (map[string]string, error)
	ChannelHasUnfinishedJob(ctx context.Context, channelID string) (bool, error)
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
	// The per-person lists: favourites and recents, and hidden channel ideas.
	UserChannelLists(ctx context.Context, userID string) (channeldatastore.UserChannelLists, error)
	AddFavouriteChannel(ctx context.Context, userID, channelID string, at time.Time) error
	RemoveFavouriteChannel(ctx context.Context, userID, channelID string) error
	RecordChannelTune(ctx context.Context, userID, channelID string, at time.Time) error
	HiddenIdeas(ctx context.Context, userID string) (map[string]bool, error)
	HideIdea(ctx context.Context, userID, ideaID string, at time.Time) error
	UnhideIdea(ctx context.Context, userID, ideaID string) error
	// Channel ideas read the open proposals to mark an idea already requested; household viewing
	// names the people watching.
	ListProposalsByStatus(ctx context.Context, status string) ([]proposalstore.Proposal, error)
	ListUsers(ctx context.Context) ([]identitystore.User, error)
}

// ChannelService is the part of the channel-management surface these routes use (§9).
type ChannelService interface {
	// Reconcile converges one channel on its selected playout backend
	// (POST /v1/channels/{id}/reconcile).
	Reconcile(ctx context.Context, channelID string) error
	// Purge hard-deletes Loomarr's channel state plus any retained managed Tunarr projection, the
	// DELETE /v1/channels/{id}?purge=true path (§7).
	Purge(ctx context.Context, channelID string) error
}

// Suggester submits the jobs a channel starts: a refine of an existing channel (§7 POST
// /v1/channels/{id}/refine) and a built proposal that needed no generation (idea requests and
// hand-built channels).
type Suggester interface {
	// Refine submits a new job for the channel with a refine-flavored intent, so its proposal binds
	// back to the same channel and is listed as the refining admin's own request. Returns the job
	// id to poll.
	Refine(ctx context.Context, channelID string, intent proposals.Intent, requestedBy string) (string, error)
	// SubmitBuilt queues a proposal that needed no generation as the requester's own request, and
	// returns its job id. The model never runs.
	SubmitBuilt(ctx context.Context, intent proposals.Intent, proposal proposals.Proposal, createdBy string) (jobID string, err error)
}

// ManualLineupResolver re-grounds a hand-picked lineup against current Library presence, splitting
// it into owned titles and titles to acquire. It shares the one approval-addition resolver with the
// approval gate, so a manually submitted proposal gets the same "client presence is not authority"
// guarantee.
type ManualLineupResolver interface {
	ResolveManualLineup(ctx context.Context, items []proposals.ProposalItem) (lineup, acquisitions []proposals.ProposalItem, err error)
}

// Binder shares channel-number occupancy with manually typed channel numbers.
type Binder interface {
	// NumberInUse answers the SAME question the approve path's numbering asks, is this channel
	// number free in Loomarr's store AND in Tunarr, so an operator who types a number gets the
	// same answer the server gives itself when it picks one. Best-effort: an unreachable Tunarr
	// reports not-in-use rather than blocking a create.
	NumberInUse(ctx context.Context, number int) (bool, error)
}

// TunerRescanner is the operation-specific media-server freshness seam from §9. A guide refresh
// cannot remove a channel from an already-registered M3U tuner; the tuner itself must be re-read.
type TunerRescanner interface {
	RescanTuner(ctx context.Context) error
}

// Stopper disconnects a channel's live sessions.
type Stopper interface {
	StopChannel(channelID string)
}

// IconService backs GET /v1/channels/{id}/icon-suggestions (§icon P2): candidate poster URLs
// drawn from a channel's own lineup titles.
type IconService interface {
	// IconSuggestions returns candidate poster URLs for one channel, best-effort: a lookup failure
	// on one lineup title is skipped, never fails the whole call (§icon — one bad title must not
	// deny every other suggestion).
	IconSuggestions(ctx context.Context, channelID string) ([]IconSuggestion, error)
}

// IconSuggestion is one candidate channel icon: a lineup title's display name paired with the
// poster's URL on THIS instance's image service.
//
// ⚠ **`url` stopped being a TMDB URL in V52 phase 7.** It used to point at TMDB's CDN, which meant
// the icon picker — one of the three surfaces an operator looks at most — loaded a dozen
// third-party images in their browser. The poster is now adopted, fetched server-side, and served
// from our own disk (§22). The field keeps its name and its job: it is what the channel's `logo`
// is set to when the operator picks this suggestion.
type IconSuggestion struct {
	Title string `json:"title" example:"Star Trek: The Next Generation"`
	URL   string `json:"url" example:"https://loomarr.example/v1/images/9f2b1c4e/w500.jpg" doc:"Image-service URL for the poster; becomes the channel's logo when picked"`
	// ImageHash links the suggestion to its image record. Carried separately from `url` because
	// the handler resolves it to `image` in one deduplicated pass.
	ImageHash string `json:"imageHash,omitempty" doc:"Content hash of the poster's image record"`
	// Image is the full record the frontend's <Image> primitive needs: real width/height, the
	// ThumbHash, and both srcsets. Absent only if the record disappeared between listing and
	// enrichment, in which case the plain `url` still renders.
	Image *httpcore.ImageDTO `json:"image,omitempty" doc:"The poster's image record, for srcset and placeholder rendering"`
}

// Deps is what the channel routes need from the rest of the server. Every field is optional except
// Store: a nil service leaves its routes answering 501, as the API's nil-semantics describe.
type Deps struct {
	Log   *slog.Logger
	Store Store
	// LiveSettings reads the TMDB key (icon suggestions) and, without a transition checkpoint, the
	// applied playout backend.
	LiveSettings settings.Reader
	// BackendCheckpoint reads the durable backend checkpoint once per operation.
	BackendCheckpoint func(context.Context) (httpcore.BackendCheckpoint, error)
	// Settings gates refine on the suggestions feature; nil reads as ready.
	Settings       httpcore.FeatureGate
	Suggest        Suggester
	Channels       ChannelService
	Binder         Binder
	TunerRescanner TunerRescanner
	Icons          IconService
	// Playout stops a removed channel's live sessions.
	Playout  Stopper
	Activity *activity.Recorder
	// ImageRecords resolves image hashes to their records in one deduplicated pass; nil leaves
	// every logo and suggestion as a plain URL.
	ImageRecords func(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO
	// Images stores the icon and watermark uploads; nil answers those routes 503.
	Images ImageStore
	// WatermarkPreview draws the on-screen logo preview and its options; nil answers 501.
	WatermarkPreview WatermarkPreviewer
	// Now is the request clock (the watermark options, idea windows, viewing); nil is time.Now.
	Now func() time.Time
	// IdeaLibrary lists the library's movies and series for channel ideas; nil answers 501.
	IdeaLibrary IdeaLibrary
	// Approver re-grounds a hand-built lineup; nil answers 503 on that route.
	Approver ManualLineupResolver
	// Calendar is the engine's holiday calendar, so ideas agree with what airs; nil reads as the
	// default calendar.
	Calendar func() schedule.Calendar
	// Viewing is who is watching what, from the players' own playlist polls; nil reads as nobody.
	Viewing *viewing.Tracker
}

// Handlers serves the channel routes.
type Handlers struct {
	log            *slog.Logger
	store          Store
	liveSettings   settings.Reader
	backend        httpcore.Backend
	settings       httpcore.FeatureGate
	suggest        Suggester
	channels       ChannelService
	binder         Binder
	tunerRescanner TunerRescanner
	icons          IconService
	playout        Stopper
	activity       *activity.Recorder
	imageRecords   func(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO
	images         ImageStore
	watermark      WatermarkPreviewer
	now            func() time.Time
	ideaLibrary    IdeaLibrary
	approver       ManualLineupResolver
	calendar       func() schedule.Calendar
	viewing        *viewing.Tracker
}

// New returns the channel handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, liveSettings: d.LiveSettings,
		backend:  httpcore.Backend{Source: d.BackendCheckpoint, Settings: d.LiveSettings},
		settings: d.Settings, suggest: d.Suggest, channels: d.Channels, binder: d.Binder,
		tunerRescanner: d.TunerRescanner, icons: d.Icons, playout: d.Playout, activity: d.Activity,
		imageRecords: d.ImageRecords, images: d.Images, watermark: d.WatermarkPreview, now: d.Now,
		ideaLibrary: d.IdeaLibrary, approver: d.Approver, calendar: d.Calendar, viewing: d.Viewing,
	}
}

// Register mounts every channel route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerChannels(api)
	s.registerWatermarkPreview(api)
	s.registerWatermarkOptions(api)
	s.registerChannelIdeas(api)
	s.registerMyChannels(api)
	s.registerHouseholdViewing(api)
}

// holidayCalendar is the engine's holiday calendar, or the default (the US, UTC) without one.
func (s *Handlers) holidayCalendar() schedule.Calendar {
	if s.calendar == nil {
		return schedule.Calendar{}
	}
	return s.calendar()
}

// ImageStore is the image service the icon and watermark uploads write through (§22).
type ImageStore interface {
	Ingest(ctx context.Context, r io.Reader, req images.IngestRequest) (images.Image, error)
	URLFor(hash string, width int, f images.Format) string
}

// checkpoint reads the backend checkpoint every channel route judges playability by.
func (s *Handlers) checkpoint(ctx context.Context) (httpcore.BackendCheckpoint, error) {
	return s.backend.Checkpoint(ctx)
}

// unconfigured reports whether a required string setting is empty right now. Safe when
// liveSettings is unset (unit tests): reports false so a wired dependency still serves.
func (s *Handlers) unconfigured(keys ...settings.StringKey) bool {
	if s.liveSettings == nil {
		return false
	}
	for _, k := range keys {
		if k.Get(s.liveSettings) == "" {
			return true
		}
	}
	return false
}

// imageDTOsByHash resolves a set of image hashes in one pass, or nothing without an image service.
func (s *Handlers) imageDTOsByHash(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO {
	if s.imageRecords == nil || len(hashes) == 0 {
		return nil
	}
	return s.imageRecords(ctx, hashes)
}

// logoImageResolver pre-resolves the image records for a set of channels and returns a lookup by
// logo URL. A logo that is external, unresolvable, or points at a hash that no longer exists
// yields nil, and the frontend falls back to the plain URL.
//
// ⚠ **Pre-resolved rather than looked up per channel, and that is the whole reason this is a
// separate function.** `channelToDTO` is called once per channel in the LIST handler, so a lookup
// inside it is an N+1 — the exact shape a profile here has already caught once, and the reason
// `LineupEntryDTO.State` is documented as list-omitted. Distinct hashes are fetched ONCE each even
// when twenty channels share an icon.
//
// ⚠ A failed lookup is a nil entry, never an error. A channel whose image row was garbage-collected
// must still render — with its logo URL, degrading to the plain <img> path — rather than failing
// the whole list. §22's Durability section makes a missing upload an accepted state, so the read
// path has to survive it.
func (s *Handlers) logoImageResolver(ctx context.Context, logos []string) func(string) *httpcore.ImageDTO {
	if s.imageRecords == nil {
		return nil
	}
	hashes := make([]string, 0, len(logos))
	for _, logo := range logos {
		if h := httpcore.ImageHashFromLogo(logo); h != "" {
			hashes = append(hashes, h)
		}
	}
	byHash := s.imageDTOsByHash(ctx, hashes)
	if len(byHash) == 0 {
		return nil
	}
	return func(logo string) *httpcore.ImageDTO {
		hash := httpcore.ImageHashFromLogo(logo)
		if hash == "" {
			return nil
		}
		return byHash[hash]
	}
}

// writeProblem sends an RFC 7807 problem+json body from a plain mux handler.
func (s *Handlers) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}

// clock is the time the options route reads the calendar at.
func (s *Handlers) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
