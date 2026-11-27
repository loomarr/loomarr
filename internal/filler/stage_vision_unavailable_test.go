package filler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/llm"
)

// A model that is down, busy or too slow is the backend's trouble, not the clip's: the rung reports
// ErrAIUnavailable so the pipeline waits instead of counting a failure. An answer the model gave
// and the clip cannot use, or a refusal that a retry cannot cure, stays an ordinary failure.
func TestVisionStage_UnavailableModelIsNotAClipFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		providerErr error
		answer      string
		unavailable bool
	}{
		{"server error", &httpx.StatusError{Method: "POST", Path: "/v1/chat", Status: 503}, "", true},
		{"rate limited", &httpx.StatusError{Method: "POST", Path: "/v1/chat", Status: 429}, "", true},
		{"first token timeout", llm.ErrFirstTokenTimeout, "", true},
		{"rejected request", &httpx.StatusError{Method: "POST", Path: "/v1/chat", Status: 400}, "", false},
		{"not json", nil, "no json here", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov := &scriptedProvider{answer: tc.answer, err: tc.providerErr}
			s := newVisionStage(newFakeVisionStore(), &scriptedVision{frames: oneFrame}, prov)
			_, err := s.Run(context.Background(), wordless(visionClip("a.mp4")))
			if err == nil {
				t.Fatal("want an error: nothing was read")
			}
			if got := errors.Is(err, filler.ErrAIUnavailable); got != tc.unavailable {
				t.Fatalf("errors.Is(err, ErrAIUnavailable) = %v, want %v (err: %v)", got, tc.unavailable, err)
			}
		})
	}
}
