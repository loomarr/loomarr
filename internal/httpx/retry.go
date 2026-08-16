package httpx

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/mantonx/loomarr/internal/metrics"
)

const (
	retryAttempts   = 4
	retryBaseDelay  = 200 * time.Millisecond
	retryMaxDelay   = 2 * time.Second
	retryDrainLimit = 4 << 10
)

// retryTransport owns the complete retry policy promised by design §6. It is a
// private implementation detail so adapters can neither weaken the GET-only
// rule nor grow service-specific retry knobs.
type retryTransport struct {
	next     http.RoundTripper
	attempts int
	base     time.Duration
	maxDelay time.Duration
	jitter   func(time.Duration) time.Duration
	wait     func(context.Context, time.Duration) error
	now      func() time.Time
	onRetry  func(metrics.OutboundRetryReason)
}

func newRetryTransport(
	next http.RoundTripper,
	onRetry func(metrics.OutboundRetryReason),
) *retryTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &retryTransport{
		next:     next,
		attempts: retryAttempts,
		base:     retryBaseDelay,
		maxDelay: retryMaxDelay,
		jitter:   fullJitter,
		wait:     waitForRetry,
		now:      time.Now,
		onRetry:  onRetry,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	attemptReq := req
	var pendingReason metrics.OutboundRetryReason

	for attempt := 0; attempt < t.attempts; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			attemptReq = req.Clone(req.Context())
			if req.Body != nil && req.Body != http.NoBody {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("replay GET body: %w", err)
				}
				attemptReq.Body = body
			}
			if t.onRetry != nil {
				t.onRetry(pendingReason)
			}
		}

		resp, err := t.next.RoundTrip(attemptReq)
		if err != nil && resp != nil {
			// RoundTripper forbids returning both, but do not leak a body if a
			// broken implementation does so.
			discardAndClose(resp.Body)
			resp = nil
		}

		if attempt == t.attempts-1 || req.Method != http.MethodGet || !replayable {
			return resp, err
		}
		reason, retry := retryReason(req.Context(), resp, err)
		if !retry {
			return resp, err
		}

		delay, fits := t.delay(req.Context(), resp, attempt)
		if !fits {
			return resp, err
		}
		if resp != nil {
			discardAndClose(resp.Body)
		}
		if err := t.wait(req.Context(), delay); err != nil {
			return nil, err
		}
		pendingReason = reason
	}

	panic("unreachable")
}

func retryReason(
	ctx context.Context,
	resp *http.Response,
	err error,
) (metrics.OutboundRetryReason, bool) {
	if ctx.Err() != nil {
		return 0, false
	}
	if err != nil {
		return metrics.OutboundRetryTransport, true
	}
	if resp == nil {
		return 0, false
	}
	switch resp.StatusCode {
	case http.StatusRequestTimeout:
		return metrics.OutboundRetryStatus408, true
	case http.StatusTooManyRequests:
		return metrics.OutboundRetryStatus429, true
	case http.StatusInternalServerError:
		return metrics.OutboundRetryStatus500, true
	case http.StatusBadGateway:
		return metrics.OutboundRetryStatus502, true
	case http.StatusServiceUnavailable:
		return metrics.OutboundRetryStatus503, true
	case http.StatusGatewayTimeout:
		return metrics.OutboundRetryStatus504, true
	default:
		return 0, false
	}
}

func (t *retryTransport) delay(ctx context.Context, resp *http.Response, retry int) (time.Duration, bool) {
	ceiling := t.base
	for range retry {
		if ceiling >= t.maxDelay/2 {
			ceiling = t.maxDelay
			break
		}
		ceiling *= 2
	}
	if ceiling > t.maxDelay {
		ceiling = t.maxDelay
	}
	delay := t.jitter(ceiling)

	if resp != nil {
		if after, ok := parseRetryAfter(resp.Header.Get("Retry-After"), t.now()); ok {
			if after > t.maxDelay {
				return 0, false
			}
			if after > delay {
				delay = after
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return 0, false
	}
	if deadline, ok := ctx.Deadline(); ok && delay >= deadline.Sub(t.now()) {
		return 0, false
	}
	return delay, true
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > int64(retryMaxDelay/time.Second) {
			// Preserve the over-cap signal without risking duration overflow.
			return retryMaxDelay + time.Nanosecond, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	date, err := http.ParseTime(value)
	if err != nil || !date.After(now) {
		return 0, false
	}
	return date.Sub(now), true
}

func fullJitter(ceiling time.Duration) time.Duration {
	if ceiling <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(ceiling) + 1))
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func discardAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, body, retryDrainLimit)
	_ = body.Close()
}
