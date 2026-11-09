package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// registerChannelExtras mounts the /v1/channels routes whose handlers still live in this package
// (now/next, upcoming and timeline with the guide; the icon and watermark uploads; the pod, cycle
// and coverage previews). The channel resource itself is api/channels. Each file that moves out
// takes its registration with it, and this function disappears with the last one.
func (s *Server) registerChannelExtras(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channels-now-next", Method: http.MethodGet, Path: "/v1/channels/now-next",
		Summary: "What is airing now and next", Description: "Per channel, the program currently airing and the one after it, read from the backend that streams that channel. Internal channels share Loomarr's playout timeline; Tunarr-backed channels use Tunarr's generated guide.",
		Tags: []string{"channels"},
	}, httpcore.RoleMember), s.channelsNowNext)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channel-upcoming", Method: http.MethodGet, Path: "/v1/channels/{id}/upcoming",
		Summary:     "What's on this channel now and next",
		Description: "The program airing now (first) then the next few, in airtime order, from the backend that streams this channel (gaps skipped). Powers the Overview 'what's on later' strip. Read-only — any authenticated user (§8.1 viewer-facing).",
		Tags:        []string{"channels"},
	}, httpcore.RoleMember), s.channelUpcoming)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channel-timeline", Method: http.MethodGet, Path: "/v1/channels/{id}/timeline",
		Summary: "The Watch player's schedule strip for one channel",
		Description: "The current programme, the next few, and the commercial breaks between them, from " +
			"internal playout's guide (§9.1 V47) — each with episode/series detail and a TMDB preview image. " +
			"Feeds the Watch player's mini-guide scrubber. Read-only — any authenticated user (viewer-facing).",
		Tags: []string{"channels"},
	}, httpcore.RoleMember), s.channelTimeline)

	// ⚠ **`channel-icon` (GET /v1/channels/{id}/icon) was RETIRED in V52 phase 8**, with the
	// `channel_icons` table behind it. A channel's logo is a /v1/images URL now, which Tunarr and retired-ok
	// the media server fetch unauthenticated exactly as they fetched this — the public-with-no-
	// credential property moved to the image serve route, where §22 makes visibility a property of
	// the IMAGE rather than of the route.
	//
	// The upload half below survives; only the serving moved.

	// The upload — admin, multipart. MaxBodyBytes fences a hostile upload at the framework
	// edge, before the handler's own read limit; the handler still checks, because the multipart
	// header's declared size is a claim, not a measurement.
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "upload-channel-icon", Method: http.MethodPost, Path: "/v1/channels/{id}/icon",
		Summary: "Upload a channel icon",
		Description: "Admin only. A raster image under 2 MB in the `file` field (PNG, JPEG, WebP, GIF — " +
			"SVG is refused because the serve half is public and an SVG can carry script). Ingests the " +
			"bytes into the image service (§22), points the channel's logo at the content-addressed URL, " +
			"and reconciles so Tunarr picks it up. Re-uploading the same image yields the same URL; a " +
			"different image yields a different one, so no cache-bust is needed.",
		Tags:         []string{"channels"},
		MaxBodyBytes: maxIconBytes + 1024,
	}, httpcore.RoleAdmin), s.uploadChannelIcon)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "upload-channel-watermark", Method: http.MethodPost, Path: "/v1/channels/{id}/watermark",
		Summary: "Upload a custom channel watermark",
		Description: "Admin only. A PNG or WebP with transparency, under 2 MB, in the `file` field. The image's " +
			"alpha is the bug's shape. Stores it in the image service (role watermark, original kept) and sets " +
			"the channel's policy.watermark.image, which wins over the automatic image. Clear it by saving the " +
			"policy without watermark.image.",
		Tags:         []string{"channels"},
		MaxBodyBytes: maxIconBytes + 1024,
	}, httpcore.RoleAdmin), s.uploadChannelWatermark)
	s.registerWatermarkPreview(api)
	s.registerWatermarkOptions(api)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "preview-channel-pods", Method: http.MethodGet, Path: "/v1/channels/{id}/pods",
		Summary:     "Preview the commercial pool this channel would get",
		Description: "Assembles the channel's SAVED filler pool WITHOUT touching Tunarr (§10, §12). Same code path and same seed as reconcile, so what you see is what the channel gets. Read-only, so any authenticated user may call it.",
		Tags:        []string{"channels", "filler"},
	}, httpcore.RoleMember), s.previewChannelPods)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channel-filler-coverage", Method: http.MethodGet, Path: "/v1/channels/{id}/filler/coverage",
		Summary:     "How well the catalog covers this channel's breaks",
		Description: "Which rung of the §10 fallback ladder this channel's breaks would draw from, and how much material each rung holds. Computed from the SAME ladder reconcile uses (internal/filler), through the same per-channel selection as the pod preview — so the meter and the pods cannot disagree. Read-only, so any authenticated user may call it. A rung the channel's policy skips is ABSENT rather than zero, and `total` is the widest rung rather than a sum (the rungs nest).",
		Tags:        []string{"channels", "filler"},
	}, httpcore.RoleMember), s.channelFillerCoverage)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "preview-channel-cycle", Method: http.MethodGet, Path: "/v1/channels/{id}/cycle",
		Summary:     "Preview what airs at a chosen time (curation rules)",
		Description: "The time-travel cycle preview (§8.1): what this channel would air at `at` (default now), and WHICH curation rule is active then. Runs the SAME pure lineup builder as reconcile — preview and reality cannot disagree — WITHOUT touching Tunarr or the store beyond a read. Makes first-match-by-priority rule resolution legible (\"at Saturday 9am, the Weekend TNG marathon rule is active\"). Read-only, so any authenticated user may call it; `at` may be past or future.",
		Tags:        []string{"channels"},
	}, httpcore.RoleMember), s.previewChannelCycle)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "preview-draft-channel-pods", Method: http.MethodPost, Path: "/v1/channels/{id}/pods/preview",
		Summary:     "Preview a draft filler selection without saving it",
		Description: "Assembles the pool a DRAFT filler selection would produce, without persisting it (§10, §12) — the live sandbox on the channel's Filler section. Same assembler + seed as the saved preview and reconcile, so the sandbox shows exactly what will air once applied. Admin-only (an authoring tool); applying is a normal PATCH of policy.filler.",
		Tags:        []string{"channels", "filler"},
	}, httpcore.RoleAdmin), s.previewDraftChannelPods)
}
