package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

var (
	// ErrInvalidSource means preparation was given an incomplete durable source identity or an
	// invalid selected-track identity.
	ErrInvalidSource = errors.New("prepared: invalid source")
	// ErrSourceChanged means Source Access could no longer validate the selected Inventory revision.
	ErrSourceChanged = errors.New("prepared: source revision changed")
	// ErrPackagerUnavailable means preparation has no media packager or Source Access port wired.
	ErrPackagerUnavailable = errors.New("prepared: packager unavailable")
	// ErrTransientInput means code attempted to serialize an operational FFmpeg input.
	ErrTransientInput = errors.New("prepared: transient input cannot be serialized")
)

// Source is the durable, provider-neutral identity of the exact Inventory source and selected
// track preparation reads. It deliberately contains no path, URL, credential, or importer type.
type Source struct {
	ItemID     string `json:"itemId"`
	SourceID   string `json:"sourceId"`
	Revision   string `json:"revision"`
	AudioTrack int    `json:"audioTrack"`
	// HDR is a content fact read from the resolved Inventory source (PQ or HLG transfer). It is not
	// part of the fingerprint: a source whose HDR-ness changes has a new Revision, and what the
	// build does about it is recorded in RenditionContract.ToneMap.
	HDR bool `json:"hdr,omitempty"`
}

// Input is a transient FFmpeg input opened from Source immediately before background packaging.
// It must never be serialized or placed in logs/diagnostics without redaction.
type Input struct {
	url  string
	http bool
}

// LocalInput constructs a transient local FFmpeg input. Its location remains unexported so generic
// serializers cannot accidentally place an operational path into durable state.
func LocalInput(path string) Input { return Input{url: path} }

// HTTPInput constructs a transient authenticated FFmpeg input. Its URL remains unexported so
// generic serializers cannot accidentally place credentials into durable state.
func HTTPInput(url string) Input { return Input{url: url, http: true} }

// IsHTTP reports which FFmpeg protocol options apply without exposing the operational input.
func (i Input) IsHTTP() bool { return i.http }

// MarshalJSON fails closed so an authenticated URL or protected path cannot enter generic durable
// documents if Input is accidentally embedded in one later.
func (Input) MarshalJSON() ([]byte, error) { return nil, ErrTransientInput }

// SourceAccess validates a durable source revision and opens its current operational input. A
// Library implementation may mint a fresh authenticated URL on every call; a local implementation
// may return its protected path only after validating size and mtime.
type SourceAccess interface {
	OpenInput(context.Context, Source) (Input, error)
}

// Request describes one reusable prepared rendition without naming a Channel or client platform.
type Request struct {
	Source    Source            `json:"source"`
	Rendition RenditionContract `json:"rendition"`
	// DurationMS is the authoritative scheduled programme duration used only for capacity
	// admission. It is not publication identity: the source revision and rendition already own
	// the immutable bytes, while different channels may schedule the same programme length.
	DurationMS int64 `json:"durationMs,omitempty"`
}

// Packager writes every immutable media file for a request into workspace and declares the
// complete output. Library owns validation and the atomic commit after this returns.
type Packager interface {
	Package(context.Context, string, Input, int, RenditionContract) (Output, error)
}

// Preparer is the control-plane entry point for prepared media. Prepare opens and packages a
// source; Lookup derives immutable identity and checks Library only, making it safe for tune time.
type Preparer struct {
	library  *Library
	packager Packager
	access   SourceAccess
	storage  *storagegovernor.Governor
}

type PreparerDependencies struct {
	Library  *Library
	Packager Packager
	Access   SourceAccess
	Storage  *storagegovernor.Governor
}

func NewPreparer(deps PreparerDependencies) *Preparer {
	return &Preparer{library: deps.Library, packager: deps.Packager, access: deps.Access, storage: deps.Storage}
}

// Lookup reports a complete publication without opening the source, reading Inventory, or calling
// Packager. Source revisions are refreshed and rebound only by the readiness control plane.
func (p *Preparer) Lookup(request Request) (Specification, bool, error) {
	if p == nil || p.library == nil {
		return Specification{}, false, nil
	}
	spec, err := specificationFor(request)
	if err != nil {
		return Specification{}, false, err
	}
	_, ready, err := p.library.Peek(spec)
	if err != nil || !ready {
		return Specification{}, false, err
	}
	return spec, true, nil
}

// Prepare validates and opens the selected revision only in the background, then atomically
// publishes its rendition. Concurrent requests for the same source/rendition share Library's one
// build; a second Source Access check prevents publishing bytes after a local revision changed.
func (p *Preparer) Prepare(ctx context.Context, request Request) (Publication, error) {
	if p == nil || p.library == nil || p.packager == nil || p.access == nil {
		return Publication{}, ErrPackagerUnavailable
	}
	spec, err := specificationFor(request)
	if err != nil {
		return Publication{}, err
	}
	if publication, ready, lookupErr := p.library.Peek(spec); lookupErr != nil || ready {
		return publication, lookupErr
	}
	var lease *storagegovernor.Lease
	if p.storage != nil {
		videoKbps := request.Rendition.VideoBitrateKbps
		if ceiling, ok := p.packager.(interface{ VideoRateCeilingKbps(RenditionContract) int }); ok {
			// Reserve for what the encoder may spend, not its nominal rung: a quality-targeted
			// encode measured 8-10 Mbps against a 5.16 Mbps estimate and was paused mid-publication.
			videoKbps = max(videoKbps, ceiling.VideoRateCeilingKbps(request.Rendition))
		}
		reservation, ok := storagegovernor.EstimatePrepared(
			request.DurationMS, videoKbps, request.Rendition.AudioBitrateKbps,
		)
		if !ok {
			return Publication{}, fmt.Errorf("prepared storage paused (%s)", storagegovernor.ReasonEstimateUnknown)
		}
		var decision storagegovernor.Decision
		lease, decision = p.storage.Reserve(ctx, storagegovernor.Request{
			Path: p.library.root, Domain: storagegovernor.DomainPrepared,
			EstimatedBytes: reservation, Mode: storagegovernor.Automatic,
		})
		if lease == nil {
			return Publication{}, preparedStorageError(decision)
		}
		defer lease.Release()
		if decision = lease.Revalidate(ctx, 0); !decision.Allowed {
			return Publication{}, preparedStorageError(decision)
		}
	}
	return p.library.Publish(ctx, spec, func(ctx context.Context, workspace string) (Output, error) {
		input, err := p.access.OpenInput(ctx, request.Source)
		if err != nil {
			return Output{}, err
		}
		if strings.TrimSpace(input.url) == "" {
			return Output{}, ErrInvalidSource
		}
		packageCtx := ctx
		finishStorage := func() error { return nil }
		if lease != nil {
			packageCtx, finishStorage = storagegovernor.MonitorGrowingPath(ctx, lease, workspace, 0)
		}
		output, err := p.packager.Package(packageCtx, workspace, input, request.Source.AudioTrack, request.Rendition)
		storageErr := finishStorage()
		if storageErr != nil {
			return Output{}, storageErr
		}
		if err != nil {
			return Output{}, err
		}
		if _, err := p.access.OpenInput(ctx, request.Source); err != nil {
			return Output{}, err
		}
		return output, nil
	})
}

func preparedStorageError(decision storagegovernor.Decision) error {
	if decision.Err != nil {
		return fmt.Errorf("prepared storage paused (%s): %w", decision.Snapshot.Reason, decision.Err)
	}
	return fmt.Errorf("prepared storage paused (%s)", decision.Snapshot.Reason)
}

func specificationFor(request Request) (Specification, error) {
	source := request.Source
	if strings.TrimSpace(source.ItemID) == "" || strings.TrimSpace(source.SourceID) == "" ||
		strings.TrimSpace(source.Revision) == "" || source.AudioTrack < 0 {
		return Specification{}, ErrInvalidSource
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		source.ItemID, source.SourceID, source.Revision, strconv.Itoa(source.AudioTrack),
	}, "\x00")))
	spec := Specification{
		SourceFingerprint: "inventory-sha256:" + hex.EncodeToString(digest[:]),
		Rendition:         request.Rendition,
	}
	if _, err := keyFor(spec); err != nil {
		return Specification{}, err
	}
	return spec, nil
}
