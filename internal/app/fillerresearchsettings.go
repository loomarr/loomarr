package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerresearch"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/settings"
)

type fillerResearchSettingsAdapter struct {
	store  fillerstore.FillerResearchStore
	set    resolved
	client *http.Client
	now    func() time.Time
}

func newFillerResearchSettingsAdapter(st fillerstore.FillerResearchStore, set resolved, recorder *metrics.Recorder) *fillerResearchSettingsAdapter {
	return &fillerResearchSettingsAdapter{store: st, set: set,
		client: httpx.NewNamedObserved(httpx.TargetFillerWebSearch, httpx.TimeoutReference, recorder), now: time.Now}
}

func (a *fillerResearchSettingsAdapter) currentConfig() fillerresearch.WebConfig {
	return fillerresearch.WebConfig{
		Provider:     fillerresearch.WebProvider(settings.FillerResearchWebProvider.Get(a.set)),
		BraveAPIKey:  settings.FillerResearchBraveAPIKey.Get(a.set),
		SearXNGURL:   settings.FillerResearchSearxngURL.Get(a.set),
		MonthlyLimit: settings.FillerResearchMonthlyLimit.Get(a.set),
	}
}

func (a *fillerResearchSettingsAdapter) Status(ctx context.Context) (fillerresearch.WebStatus, error) {
	config := a.currentConfig()
	now := a.now().UTC()
	usage, err := a.store.FillerResearchWebUsage(ctx, now.Format("2006-01"))
	if err != nil {
		return fillerresearch.WebStatus{}, err
	}
	return fillerresearch.Status(settings.FillerResearchEnabled.Get(a.set), config, usage, now), nil
}

func (a *fillerResearchSettingsAdapter) Test(ctx context.Context, proposed fillerresearch.WebConfig) (fillerresearch.WebStatus, bool, string, error) {
	current := a.currentConfig()
	if proposed.Provider == current.Provider {
		if proposed.Provider == fillerresearch.WebProviderBrave && strings.TrimSpace(proposed.BraveAPIKey) == "" {
			proposed.BraveAPIKey = current.BraveAPIKey
		}
		if proposed.Provider == fillerresearch.WebProviderSearXNG && strings.TrimSpace(proposed.SearXNGURL) == "" {
			proposed.SearXNGURL = current.SearXNGURL
		}
	}
	proposed.MonthlyLimit = settings.FillerResearchMonthlyLimit.Get(a.set)
	if err := proposed.Validate(); err != nil {
		return fillerresearch.WebStatus{}, false, "Enter the provider details, then try again.", nil
	}
	web := fillerresearch.NewWeb(func() fillerresearch.WebConfig { return proposed }, a.store,
		fillerresearch.WebOptions{Client: a.client, Now: a.now})
	_, err := web.Retrieve(ctx, fillerresearch.Lookup{Title: "Loomarr home media clip details"})
	status, statusErr := a.statusFor(ctx, proposed)
	if statusErr != nil {
		return fillerresearch.WebStatus{}, false, "", statusErr
	}
	if err == nil {
		return status, true, "Web search is ready.", nil
	}
	switch {
	case errors.Is(err, fillerresearch.ErrWebSearchLimit):
		return status, false, "This month's web-search limit has been reached.", nil
	case errors.Is(err, fillerresearch.ErrWebSearchUnavailable):
		return status, false, "The provider details are incomplete.", nil
	case strings.Contains(strings.ToLower(err.Error()), "401"), strings.Contains(strings.ToLower(err.Error()), "403"):
		return status, false, "The provider rejected those credentials.", nil
	default:
		return status, false, "Loomarr couldn't reach the search provider. Check the details and try again.", nil
	}
}

func (a *fillerResearchSettingsAdapter) statusFor(ctx context.Context, config fillerresearch.WebConfig) (fillerresearch.WebStatus, error) {
	now := a.now().UTC()
	usage, err := a.store.FillerResearchWebUsage(ctx, now.Format("2006-01"))
	if err != nil {
		return fillerresearch.WebStatus{}, err
	}
	return fillerresearch.Status(settings.FillerResearchEnabled.Get(a.set), config, usage, now), nil
}
