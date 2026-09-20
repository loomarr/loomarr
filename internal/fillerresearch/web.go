package fillerresearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const WebAdapterVersion = "web-v1"

var (
	ErrWebSearchUnavailable = errors.New("web search is not configured")
	ErrWebSearchLimit       = errors.New("monthly web-search limit reached")
	ErrWebSearchAttempted   = errors.New("web search already attempted for this clip revision")
)

type WebProvider string

const (
	WebProviderNone    WebProvider = "none"
	WebProviderBrave   WebProvider = "brave"
	WebProviderSearXNG WebProvider = "searxng"
)

type WebConfig struct {
	Provider     WebProvider
	BraveAPIKey  string
	SearXNGURL   string
	MonthlyLimit int
}

func (c WebConfig) Validate() error {
	if c.MonthlyLimit < 1 {
		return fmt.Errorf("%w: monthly limit must be positive", ErrInvalid)
	}
	switch c.Provider {
	case WebProviderBrave:
		if strings.TrimSpace(c.BraveAPIKey) == "" {
			return fmt.Errorf("%w: Brave Search API key is required", ErrWebSearchUnavailable)
		}
	case WebProviderSearXNG:
		u, err := url.Parse(strings.TrimSpace(c.SearXNGURL))
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("%w: SearXNG requires a credential-free HTTPS endpoint", ErrWebSearchUnavailable)
		}
	case WebProviderNone, "":
		return ErrWebSearchUnavailable
	default:
		return fmt.Errorf("%w: unknown web-search provider", ErrInvalid)
	}
	return nil
}

type WebUsage struct {
	Month         string
	RequestCount  int
	LastProvider  WebProvider
	LastSuccessAt time.Time
	LastFailureAt time.Time
}

// WebAttempt is the durable per-input-revision cost boundary. An empty attempt is permitted only
// for an administrator's explicit connection test, which still consumes the monthly allowance.
type WebAttempt struct {
	ClipHash       string
	InputRevision  int64
	AdapterVersion string
	ReservedAt     time.Time
}

func (a WebAttempt) Tracked() bool {
	return strings.TrimSpace(a.ClipHash) != "" && a.InputRevision > 0 &&
		strings.TrimSpace(a.AdapterVersion) != "" && !a.ReservedAt.IsZero()
}

type WebState string

const (
	WebStateOff          WebState = "off"
	WebStateUnconfigured WebState = "unconfigured"
	WebStateReady        WebState = "ready"
	WebStateDegraded     WebState = "degraded"
	WebStateLimitReached WebState = "limit_reached"
)

type WebStatus struct {
	StructuredEnabled bool
	Provider          WebProvider
	Configured        bool
	State             WebState
	Month             string
	RequestCount      int
	RequestLimit      int
	LastSuccessAt     time.Time
	LastFailureAt     time.Time
}

func Status(enabled bool, config WebConfig, usage WebUsage, now time.Time) WebStatus {
	status := WebStatus{StructuredEnabled: enabled, Provider: config.Provider, Month: now.UTC().Format("2006-01"),
		RequestCount: usage.RequestCount, RequestLimit: config.MonthlyLimit,
		LastSuccessAt: usage.LastSuccessAt, LastFailureAt: usage.LastFailureAt}
	status.Configured = config.Validate() == nil
	switch {
	case !enabled:
		status.State = WebStateOff
	case !status.Configured:
		status.State = WebStateUnconfigured
	case status.RequestCount >= status.RequestLimit:
		status.State = WebStateLimitReached
	case status.LastFailureAt.After(status.LastSuccessAt):
		status.State = WebStateDegraded
	default:
		status.State = WebStateReady
	}
	return status
}

type WebUsageLedger interface {
	ReserveFillerResearchWebRequest(context.Context, string, WebProvider, int, WebAttempt) (WebUsage, error)
	CompleteFillerResearchWebRequest(context.Context, string, bool, time.Time) error
	FillerResearchWebUsage(context.Context, string) (WebUsage, error)
}

type WebOptions struct {
	Client               *http.Client
	BraveEndpoint        string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

// Web resolves the currently configured general-search provider at each request. That keeps a
// saved provider/key hot without rebuilding the application generation while Identity still wakes
// clips when the selected provider or endpoint changes.
type Web struct {
	config func() WebConfig
	ledger WebUsageLedger
	client *http.Client
	brave  string
	test   bool
	now    func() time.Time
}

func NewWeb(config func() WebConfig, ledger WebUsageLedger, options WebOptions) *Web {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Web{config: config, ledger: ledger, client: options.Client, brave: options.BraveEndpoint,
		test: options.AllowInsecureTestURL, now: options.Now}
}

func (w *Web) Identity() (string, string) {
	if w == nil || w.config == nil {
		return "web", WebAdapterVersion + ":none"
	}
	config := w.config()
	identity := string(config.Provider)
	if config.Provider == WebProviderSearXNG {
		sum := sha256.Sum256([]byte(strings.TrimSpace(config.SearXNGURL)))
		identity += ":" + hex.EncodeToString(sum[:6])
	}
	if identity == "" {
		identity = "none"
	}
	return "web", WebAdapterVersion + ":" + identity
}

func (w *Web) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if w == nil || w.config == nil || w.ledger == nil {
		return Packet{}, ErrWebSearchUnavailable
	}
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	config := w.config()
	if err := config.Validate(); err != nil {
		return Packet{}, err
	}
	now := w.now().UTC()
	month := now.Format("2006-01")
	var attempt WebAttempt
	if strings.TrimSpace(lookup.ClipHash) != "" {
		_, adapterVersion := w.Identity()
		attempt = WebAttempt{ClipHash: lookup.ClipHash, InputRevision: lookup.InputRevision,
			AdapterVersion: adapterVersion, ReservedAt: now}
	}
	if _, err := w.ledger.ReserveFillerResearchWebRequest(ctx, month, config.Provider, config.MonthlyLimit, attempt); err != nil {
		return Packet{}, err
	}

	var (
		packet Packet
		err    error
	)
	switch config.Provider {
	case WebProviderBrave:
		var provider *Brave
		provider, err = NewBrave(BraveConfig{Client: w.client, APIKey: config.BraveAPIKey,
			Endpoint: w.brave, AllowInsecureTestURL: w.test, Now: w.now})
		if err == nil {
			packet, err = provider.Retrieve(ctx, lookup)
		}
	case WebProviderSearXNG:
		var provider *SearXNG
		provider, err = NewSearXNG(SearXNGConfig{Client: w.client, Endpoint: config.SearXNGURL,
			AllowInsecureTestURL: w.test, Now: w.now})
		if err == nil {
			packet, err = provider.Retrieve(ctx, lookup)
		}
	default:
		err = ErrWebSearchUnavailable
	}
	completeErr := w.ledger.CompleteFillerResearchWebRequest(ctx, month, err == nil, w.now().UTC())
	if err != nil {
		return Packet{}, errors.Join(err, completeErr)
	}
	if completeErr != nil {
		return Packet{}, completeErr
	}
	packet.Adapter = "web"
	_, packet.AdapterVersion = w.Identity()
	return packet, nil
}
