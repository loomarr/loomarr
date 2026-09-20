package fillerresearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type memoryWebLedger struct {
	usage    WebUsage
	limit    bool
	attempts map[string]bool
}

func (l *memoryWebLedger) ReserveFillerResearchWebRequest(_ context.Context, month string, provider WebProvider,
	limit int, attempt WebAttempt) (WebUsage, error) {
	if l.limit || l.usage.RequestCount >= limit {
		return WebUsage{}, ErrWebSearchLimit
	}
	if attempt.Tracked() {
		if l.attempts == nil {
			l.attempts = make(map[string]bool)
		}
		key := attempt.ClipHash + ":" + fmt.Sprint(attempt.InputRevision) + ":" + attempt.AdapterVersion
		if l.attempts[key] {
			return WebUsage{}, ErrWebSearchAttempted
		}
		l.attempts[key] = true
	}
	l.usage.Month, l.usage.LastProvider = month, provider
	l.usage.RequestCount++
	return l.usage, nil
}
func (l *memoryWebLedger) CompleteFillerResearchWebRequest(_ context.Context, _ string, success bool, at time.Time) error {
	if success {
		l.usage.LastSuccessAt = at
	} else {
		l.usage.LastFailureAt = at
	}
	return nil
}
func (l *memoryWebLedger) FillerResearchWebUsage(context.Context, string) (WebUsage, error) {
	return l.usage, nil
}

func TestWebEnforcesMonthlyLimitBeforeProviderRequest(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"results":[{"title":"Result","url":"https://example.org/result","content":"Useful clip context."}]}`))
	}))
	defer server.Close()
	ledger := &memoryWebLedger{limit: true}
	web := NewWeb(func() WebConfig {
		return WebConfig{Provider: WebProviderSearXNG, SearXNGURL: server.URL, MonthlyLimit: 1}
	}, ledger, WebOptions{Client: server.Client()})
	_, err := web.Retrieve(t.Context(), Lookup{Title: "clip"})
	if !errors.Is(err, ErrWebSearchLimit) || requests != 0 {
		t.Fatalf("err=%v provider requests=%d", err, requests)
	}
}

func TestWebRecordsOneSuccessfulBoundedRequest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"Result","url":"https://example.org/result","content":"Useful clip context."}]}`))
	}))
	defer server.Close()
	at := time.Unix(300, 0).UTC()
	ledger := &memoryWebLedger{}
	web := NewWeb(func() WebConfig {
		return WebConfig{Provider: WebProviderSearXNG, SearXNGURL: server.URL, MonthlyLimit: 2}
	}, ledger, WebOptions{Client: server.Client(), Now: func() time.Time { return at }})
	packet, err := web.Retrieve(t.Context(), Lookup{Title: "clip"})
	if err != nil {
		t.Fatal(err)
	}
	if ledger.usage.RequestCount != 1 || ledger.usage.LastSuccessAt != at || packet.Adapter != "web" {
		t.Fatalf("usage=%+v packet=%+v", ledger.usage, packet)
	}
}

func TestWebAttemptsOneProviderRequestPerClipRevisionEvenAfterFailure(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	at := time.Unix(300, 0).UTC()
	ledger := &memoryWebLedger{}
	web := NewWeb(func() WebConfig {
		return WebConfig{Provider: WebProviderSearXNG, SearXNGURL: server.URL, MonthlyLimit: 5}
	}, ledger, WebOptions{Client: server.Client(), Now: func() time.Time { return at }})
	lookup := Lookup{Title: "clip", ClipHash: "clip-hash", InputRevision: 4}
	if _, err := web.Retrieve(t.Context(), lookup); err == nil {
		t.Fatal("failed provider request unexpectedly succeeded")
	}
	if _, err := web.Retrieve(t.Context(), lookup); !errors.Is(err, ErrWebSearchAttempted) {
		t.Fatalf("second request error = %v, want already attempted", err)
	}
	if requests != 1 || ledger.usage.RequestCount != 1 {
		t.Fatalf("provider requests=%d usage=%+v", requests, ledger.usage)
	}
}
