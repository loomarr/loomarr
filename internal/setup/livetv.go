// Package setup owns the operator connection flows (§7, §13): the Live TV wiring
// and setup-status checklist. It composes the media-server LiveTV capability (§6);
// either Loomarr or Tunarr can publish the playlist and guide that the media server consumes.
package setup

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mantonx/loomarr/internal/library"
)

// LiveTVURLs is a Live TV M3U playlist + XMLTV guide pair.
// The pair can point at either Loomarr's internal publisher or Tunarr (§6).
type LiveTVURLs struct {
	M3U   string
	XMLTV string
}

// TunarrURLsFrom derives the tuner + guide URLs from the Tunarr base URL.
func TunarrURLsFrom(tunarrBaseURL string) LiveTVURLs {
	base := strings.TrimRight(tunarrBaseURL, "/")
	return LiveTVURLs{
		M3U:   base + "/api/channels.m3u",
		XMLTV: base + "/api/xmltv.xml",
	}
}

// InternalPlayoutURLs derives the tuner + guide URLs Loomarr serves ITSELF (§9.1).
//
// ⚠ WHICH BACKEND STREAMS DECIDES WHICH URLs THE MEDIA SERVER GETS. `playout.backend`
// defaults to `internal`, meaning Loomarr encodes the channels — but the Live TV wiring
// registered Tunarr's `/api/channels.m3u` unconditionally, so the media server was pointed at
// a backend that was not serving those channels. Reported symptom: the channels appear in
// Emby's guide and will not play, and a `livetv-reconnect` "fixes" it by re-registering the
// same wrong URLs. design.md §9.1 item 3 called for exactly this and the code never did it.
//
// Both URLs carry the device token as a query parameter, because the media server fetches them
// unauthenticated from a background job — the same reason /playout/* takes `?token=` at all
// (§11: this authenticates a DEVICE, not a person).
//
// An empty publicURL yields empty URLs, and the caller MUST treat that as "not wireable" rather
// than registering a relative path: the media server resolves the URL from its own host, so a
// blank or relative base silently points it at itself. That is the `server.public_url` failure
// mode the playout handlers already guard.
func InternalPlayoutURLs(publicURL, deviceToken string) LiveTVURLs {
	base := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if base == "" {
		return LiveTVURLs{}
	}
	q := ""
	if deviceToken != "" {
		q = "?token=" + url.QueryEscape(deviceToken)
	}
	return LiveTVURLs{
		M3U:   base + "/v1/playout/tuner.m3u" + q,
		XMLTV: base + "/v1/playout/guide.xml" + q,
	}
}

// LiveTVURLsFor picks the URLs for the backend that will actually stream (§9.1).
//
// `internal` (the default) ⇒ Loomarr's own endpoints; `tunarr` ⇒ Tunarr's. Anything else falls
// back to Tunarr, which is the pre-§9.1 behaviour and the safer default for an unrecognised
// value: an install that has not opted into internal playout keeps working exactly as it did.
func LiveTVURLsFor(backend, tunarrBaseURL, publicURL, deviceToken string) LiveTVURLs {
	// The literal rather than schedule.PlayoutBackendInternal: `setup` does not otherwise
	// depend on `schedule`, and importing the scheduler for one enum string would be a worse
	// trade than this comment. The value is pinned by TestLiveTVURLsFor_* below.
	if strings.TrimSpace(backend) == "internal" {
		return InternalPlayoutURLs(publicURL, deviceToken)
	}
	return TunarrURLsFrom(tunarrBaseURL)
}

// LiveTVConnector performs idempotent publication and forced-repair effects against the
// media-server Live TV capability (§6). Transition-coordinated production calls pass an explicit
// URL pair; the fixed pair exists for the single-operation compatibility helpers and tests.
type LiveTVConnector struct {
	library      LiveTVSource
	fallbackURLs LiveTVURLs
}

// LiveTVSource starts one immutable media-server operation. Production returns
// library.Client.Snapshot; fixed tests and adapters return the same value each time.
type LiveTVSource func() library.LiveTV

// NewLiveTVConnector wires a live library source. Each public connector operation
// resolves the source exactly once and carries that bound capability through all
// enumerate, mutate, verify, and refresh calls.
func NewLiveTVConnector(source LiveTVSource, urls LiveTVURLs) *LiveTVConnector {
	return &LiveTVConnector{library: source, fallbackURLs: urls}
}

// NewLiveTVConnectorFixed wires a connector to a fixed URL pair — for tests and any caller with
// no live settings to read. Production transition and status paths use the explicit Target methods.
func NewLiveTVConnectorFixed(lib library.LiveTV, urls LiveTVURLs) *LiveTVConnector {
	return NewLiveTVConnector(func() library.LiveTV { return lib }, urls)
}

// Snapshot binds the current media-server connection for a compound workflow whose
// phases are invoked separately. The returned connector can safely be retained from
// Prepare through RefreshTarget and RetireStale without re-reading live credentials.
func (c *LiveTVConnector) Snapshot() *LiveTVConnector {
	if c == nil {
		return nil
	}
	return NewLiveTVConnectorFixed(c.library(), c.fallbackURLs)
}

// ConnectResult reports what the connect did — so the API/UI can distinguish
// "just wired it" from "already wired" (§6 second-call-no-op) without guessing.
type ConnectResult struct {
	TunerAdded     bool  // an m3u tuner host was registered this call
	ListingAdded   bool  // an xmltv listing provider was registered this call
	TunerRemoved   int   // stale Loomarr tuner hosts retired this call (URL change)
	ListingRemoved int   // stale Loomarr listing providers retired this call
	Poked          bool  // a rescan+refresh was attempted (something changed)
	PokeErr        error // best-effort poke failure; the connect still succeeded
}

// AlreadyWired reports whether nothing needed to change — the idempotent no-op
// case the Phase-10 gate asserts on the second call. A URL-change reconcile that
// retired a stale tuner is NOT a no-op, so the removals count too.
func (r ConnectResult) AlreadyWired() bool {
	return !r.TunerAdded && !r.ListingAdded && r.TunerRemoved == 0 && r.ListingRemoved == 0
}

// Connect composes the connector's three transition-safe operations: prepare the
// target pair, retire stale Loomarr-owned registrations, then refresh the media
// server. Preparation is add-first, so an add failure cannot delete the working
// pair. Backend cutovers use the operations separately because the target feed must
// be published between Prepare and RefreshTarget, and durable activation belongs
// between RefreshTarget and RetireStale (§6).
func (c *LiveTVConnector) Connect(ctx context.Context) (ConnectResult, error) {
	lib := c.library()
	urls := c.fallbackURLs
	res, err := prepare(ctx, lib, urls)
	if err != nil {
		return res, err
	}
	retired, err := retireStale(ctx, lib, urls)
	res.merge(retired)
	if err != nil {
		return res, err
	}
	if res.AlreadyWired() {
		return res, nil
	}

	res.Poked = true
	res.PokeErr = refreshTarget(ctx, lib, urls)
	return res, nil
}

// Prepare adds and verifies the target tuner/listing pair without enumerating or
// removing stale registrations. A half-prepared target is deliberately retained:
// retrying fills the missing half, while the previous pair keeps serving viewers.
// The explicit target makes a multi-step cutover immune to a live setting changing
// between its durable phases.
func (c *LiveTVConnector) Prepare(ctx context.Context, urls LiveTVURLs) (ConnectResult, error) {
	return prepare(ctx, c.library(), urls)
}

func prepare(ctx context.Context, lib library.LiveTV, urls LiveTVURLs) (ConnectResult, error) {
	var res ConnectResult
	if err := validateLiveTVURLs(urls); err != nil {
		return res, err
	}

	tunerThere, err := lib.TunerRegistered(ctx, urls.M3U)
	if err != nil {
		return res, fmt.Errorf("enumerate tuner hosts: %w", err)
	}
	if !tunerThere {
		if err := lib.AddTuner(ctx, urls.M3U); err != nil {
			return res, fmt.Errorf("register tuner: %w", err)
		}
		res.TunerAdded = true
	}
	if tunerThere, err = lib.TunerRegistered(ctx, urls.M3U); err != nil {
		return res, fmt.Errorf("verify tuner host: %w", err)
	} else if !tunerThere {
		return res, fmt.Errorf("verify tuner host: target registration is missing")
	}

	listingThere, err := lib.ListingRegistered(ctx, urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("enumerate listing providers: %w", err)
	}
	if !listingThere {
		if err := lib.AddListingProvider(ctx, urls.XMLTV); err != nil {
			return res, fmt.Errorf("register listing provider: %w", err)
		}
		res.ListingAdded = true
	}
	if listingThere, err = lib.ListingRegistered(ctx, urls.XMLTV); err != nil {
		return res, fmt.Errorf("verify listing provider: %w", err)
	} else if !listingThere {
		return res, fmt.Errorf("verify listing provider: target registration is missing")
	}

	return res, nil
}

// RetireStale removes Loomarr-owned registrations other than the target. It first
// verifies BOTH target halves, so even an accidental early call cannot delete the
// working pair before its replacement exists. A hand-added tuner is never returned
// by the library ownership queries and therefore never touched.
func (c *LiveTVConnector) RetireStale(ctx context.Context, urls LiveTVURLs) (ConnectResult, error) {
	return retireStale(ctx, c.library(), urls)
}

func retireStale(ctx context.Context, lib library.LiveTV, urls LiveTVURLs) (ConnectResult, error) {
	var res ConnectResult
	if err := validateLiveTVURLs(urls); err != nil {
		return res, err
	}
	tunerThere, err := lib.TunerRegistered(ctx, urls.M3U)
	if err != nil {
		return res, fmt.Errorf("verify target tuner before retirement: %w", err)
	}
	listingThere, err := lib.ListingRegistered(ctx, urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("verify target listing before retirement: %w", err)
	}
	if !tunerThere || !listingThere {
		return res, fmt.Errorf("retire stale live tv registrations: target tuner and listing must both exist")
	}

	staleTuners, err := lib.StaleLoomarrTuners(ctx, urls.M3U)
	if err != nil {
		return res, fmt.Errorf("enumerate stale tuner hosts: %w", err)
	}
	for _, id := range staleTuners {
		if err := lib.RemoveTuner(ctx, id); err != nil {
			return res, fmt.Errorf("remove stale tuner %s: %w", id, err)
		}
		res.TunerRemoved++
	}

	staleListings, err := lib.StaleLoomarrListings(ctx, urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("enumerate stale listing providers: %w", err)
	}
	for _, id := range staleListings {
		if err := lib.RemoveListingProvider(ctx, id); err != nil {
			return res, fmt.Errorf("remove stale listing provider %s: %w", id, err)
		}
		res.ListingRemoved++
	}
	return res, nil
}

// RefreshTarget makes the media server read a prepared target's channel list and
// guide. It is separate from Prepare so an internal M3U can be durably published
// before the fetch, and separate from RetireStale so activation can commit before
// old registrations are removed. Both pokes are attempted; the first error wins.
func (c *LiveTVConnector) RefreshTarget(ctx context.Context, urls LiveTVURLs) error {
	return refreshTarget(ctx, c.library(), urls)
}

func refreshTarget(ctx context.Context, lib library.LiveTV, urls LiveTVURLs) error {
	if err := validateLiveTVURLs(urls); err != nil {
		return err
	}
	var first error
	if err := lib.RescanTuner(ctx, urls.M3U); err != nil {
		first = fmt.Errorf("rescan tuner: %w", err)
	}
	if err := lib.RefreshGuide(ctx); err != nil && first == nil {
		first = fmt.Errorf("refresh guide: %w", err)
	}
	return first
}

func validateLiveTVURLs(urls LiveTVURLs) error {
	if urls.M3U == "" || urls.XMLTV == "" {
		// Nothing wireable — internal playout with no server.public_url set. Registering a
		// relative path here would point the media server at ITSELF (it resolves the URL from
		// its own host), which looks wired and never plays.
		return fmt.Errorf("live tv: no reachable playout URLs — set Loomarr's public address in Settings")
	}
	return nil
}

func (r *ConnectResult) merge(other ConnectResult) {
	r.TunerAdded = r.TunerAdded || other.TunerAdded
	r.ListingAdded = r.ListingAdded || other.ListingAdded
	r.TunerRemoved += other.TunerRemoved
	r.ListingRemoved += other.ListingRemoved
}

// Reconnect FORCE-re-wires both halves of the Live TV publication: it removes every
// Loomarr-owned tuner and listing provider (even at the current URLs, which Connect
// leaves alone) and re-adds the target pair, then pokes a re-scan + guide refresh.
// This repairs stale channel→stream and guide bindings that an idempotent Connect
// cannot detect. The wiring operations themselves are strict; only the final pokes
// remain best-effort, like Connect.
func (c *LiveTVConnector) Reconnect(ctx context.Context) (ConnectResult, error) {
	return c.ReconnectTarget(ctx, c.fallbackURLs)
}

// ReconnectTarget force-rewires one explicit tuner/listing pair. Backend transitions use the
// explicit form while holding their durable workflow lock, so a live resolver cannot select a
// stale process-local backend or change targets between removal and re-addition.
func (c *LiveTVConnector) ReconnectTarget(ctx context.Context, urls LiveTVURLs) (ConnectResult, error) {
	return reconnectTarget(ctx, c.library(), urls)
}

func reconnectTarget(ctx context.Context, lib library.LiveTV, urls LiveTVURLs) (ConnectResult, error) {
	var res ConnectResult
	if err := validateLiveTVURLs(urls); err != nil {
		return res, err
	}

	tuners, err := lib.LoomarrTuners(ctx)
	if err != nil {
		return res, fmt.Errorf("enumerate loomarr tuners: %w", err)
	}
	// Passing an empty desired URL asks the existing ownership query for every
	// Loomarr-shaped provider. Reconnect deliberately includes the provider already
	// at the target URL; unlike RetireStale, it must force a fresh binding.
	listings, err := lib.StaleLoomarrListings(ctx, "")
	if err != nil {
		return res, fmt.Errorf("enumerate loomarr listing providers: %w", err)
	}

	for _, id := range tuners {
		if err := lib.RemoveTuner(ctx, id); err != nil {
			return res, fmt.Errorf("remove tuner %s: %w", id, err)
		}
		res.TunerRemoved++
	}
	if err := lib.AddTuner(ctx, urls.M3U); err != nil {
		return res, fmt.Errorf("re-add tuner: %w", err)
	}
	res.TunerAdded = true

	for _, id := range listings {
		if err := lib.RemoveListingProvider(ctx, id); err != nil {
			return res, fmt.Errorf("remove listing provider %s: %w", id, err)
		}
		res.ListingRemoved++
	}
	if err := lib.AddListingProvider(ctx, urls.XMLTV); err != nil {
		return res, fmt.Errorf("re-add listing provider: %w", err)
	}
	res.ListingAdded = true

	// Poke so the re-added tuner's channels are re-read now (the whole point).
	res.Poked = true
	if err := lib.RescanTuner(ctx, urls.M3U); err != nil {
		res.PokeErr = fmt.Errorf("rescan tuner: %w", err)
	}
	if err := lib.RefreshGuide(ctx); err != nil && res.PokeErr == nil {
		res.PokeErr = fmt.Errorf("refresh guide: %w", err)
	}
	return res, nil
}

// Wired reports whether the connector's fixed pair is registered as both tuner and guide.
func (c *LiveTVConnector) Wired(ctx context.Context) (bool, error) {
	return c.WiredTarget(ctx, c.fallbackURLs)
}

// WiredTarget reports whether one explicit tuner/listing pair is registered. Request-path status
// adapters use this form with a durable checkpoint read so Postgres replicas never report against
// a stale process-local backend.
func (c *LiveTVConnector) WiredTarget(ctx context.Context, urls LiveTVURLs) (bool, error) {
	return wiredTarget(ctx, c.library(), urls)
}

func wiredTarget(ctx context.Context, lib library.LiveTV, urls LiveTVURLs) (bool, error) {
	if urls.M3U == "" || urls.XMLTV == "" {
		return false, nil // nothing wireable ⇒ not wired; the checklist reports it as such
	}
	tuner, err := lib.TunerRegistered(ctx, urls.M3U)
	if err != nil {
		return false, err
	}
	listing, err := lib.ListingRegistered(ctx, urls.XMLTV)
	if err != nil {
		return false, err
	}
	return tuner && listing, nil
}

// PokeGuideRefresh implements channels.GuidePoker (§9): trigger the media
// server's guide-refresh task after an existing channel's lineup changed.
// Best-effort.
func (c *LiveTVConnector) PokeGuideRefresh(ctx context.Context) error {
	return c.library().RefreshGuide(ctx)
}

// RescanTuner implements channels.GuidePoker (§9): make the media server re-read
// the tuner channel list so a newly-created channel is discovered. Best-effort.
func (c *LiveTVConnector) RescanTuner(ctx context.Context) error {
	return c.RescanTarget(ctx, c.fallbackURLs)
}

// RescanTarget makes the media server re-read one explicit tuner target. Backend
// transitions use this operation instead of RescanTuner: the transport checkpoint
// may have published Loomarr's prepared internal feed while the ordinary connector
// still resolves the previously applied Tunarr feed. Keeping the URL explicit makes
// a lifecycle removal refresh the same catalog that admitted the channel.
func (c *LiveTVConnector) RescanTarget(ctx context.Context, urls LiveTVURLs) error {
	if urls.M3U == "" {
		return nil // nothing to rescan; the poke is best-effort by contract (§9)
	}
	return c.library().RescanTuner(ctx, urls.M3U)
}
