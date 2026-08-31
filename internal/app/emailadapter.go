package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/notifications"
)

type emailTestAdapter struct {
	adapter *notifications.EmailAdapter
}

func (a emailTestAdapter) SendTest(ctx context.Context, destination string) api.EmailTestResult {
	if a.adapter == nil {
		return api.EmailTestResult{Outcome: string(notifications.OutcomeMeansUnavailable)}
	}
	result := a.adapter.SendTest(ctx, destination)
	return api.EmailTestResult{
		OK: result.Status == notifications.StatusDelivered, Outcome: string(result.OutcomeCode),
	}
}
