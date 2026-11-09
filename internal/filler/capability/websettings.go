package capability

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// WebSettings reads the web-search settings live and answers the Settings page's status and test
// requests for them.
type WebSettings struct {
	store  records.FillerResearchStore
	set    settings.Reader
	client *http.Client
	now    func() time.Time
}

// NewWebSettings returns the web-search settings adapter over st and the live settings.
func NewWebSettings(st records.FillerResearchStore, set settings.Reader, recorder *metrics.Recorder) *WebSettings {
	return &WebSettings{store: st, set: set,
		client: httpx.NewNamedObserved(httpx.TargetFillerWebSearch, httpx.TimeoutReference, recorder), now: time.Now}
}

func (a *WebSettings) currentConfig() research.WebConfig {
	return research.WebConfig{
		Provider:     research.WebProvider(settings.FillerResearchWebProvider.Get(a.set)),
		BraveAPIKey:  settings.FillerResearchBraveAPIKey.Get(a.set),
		SearXNGURL:   settings.FillerResearchSearxngURL.Get(a.set),
		MonthlyLimit: settings.FillerResearchMonthlyLimit.Get(a.set),
	}
}

func (a *WebSettings) Status(ctx context.Context) (research.WebStatus, error) {
	config := a.currentConfig()
	now := a.now().UTC()
	usage, err := a.store.FillerResearchWebUsage(ctx, now.Format("2006-01"))
	if err != nil {
		return research.WebStatus{}, err
	}
	return research.Status(settings.FillerResearchEnabled.Get(a.set), config, usage, now), nil
}

func (a *WebSettings) Test(ctx context.Context, proposed research.WebConfig) (research.WebStatus, bool, string, error) {
	current := a.currentConfig()
	if proposed.Provider == current.Provider {
		if proposed.Provider == research.WebProviderBrave && strings.TrimSpace(proposed.BraveAPIKey) == "" {
			proposed.BraveAPIKey = current.BraveAPIKey
		}
		if proposed.Provider == research.WebProviderSearXNG && strings.TrimSpace(proposed.SearXNGURL) == "" {
			proposed.SearXNGURL = current.SearXNGURL
		}
	}
	proposed.MonthlyLimit = settings.FillerResearchMonthlyLimit.Get(a.set)
	if err := proposed.Validate(); err != nil {
		return research.WebStatus{}, false, "Enter the provider details, then try again.", nil
	}
	web := research.NewWeb(func() research.WebConfig { return proposed }, a.store,
		research.WebOptions{Client: a.client, Now: a.now})
	_, err := web.Retrieve(ctx, research.Lookup{Title: "Loomarr home media clip details"})
	status, statusErr := a.statusFor(ctx, proposed)
	if statusErr != nil {
		return research.WebStatus{}, false, "", statusErr
	}
	if err == nil {
		return status, true, "Web search is ready.", nil
	}
	switch {
	case errors.Is(err, research.ErrWebSearchLimit):
		return status, false, "This month's web-search limit has been reached.", nil
	case errors.Is(err, research.ErrWebSearchUnavailable):
		return status, false, "The provider details are incomplete.", nil
	case strings.Contains(strings.ToLower(err.Error()), "401"), strings.Contains(strings.ToLower(err.Error()), "403"):
		return status, false, "The provider rejected those credentials.", nil
	default:
		return status, false, "Loomarr couldn't reach the search provider. Check the details and try again.", nil
	}
}

func (a *WebSettings) statusFor(ctx context.Context, config research.WebConfig) (research.WebStatus, error) {
	now := a.now().UTC()
	usage, err := a.store.FillerResearchWebUsage(ctx, now.Format("2006-01"))
	if err != nil {
		return research.WebStatus{}, err
	}
	return research.Status(settings.FillerResearchEnabled.Get(a.set), config, usage, now), nil
}
