package suggest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// The production failure (#1402/#1410) persisted only "suggestion failed: provider failure":
// the provider's own explanation (a llama.cpp 500, a timeout budget) was dropped at the worker
// boundary, so a failed job could not be diagnosed from the UI or the row.
func TestWorker_LLMFailureCauseReachesLastError(t *testing.T) {
	const secretPrompt = "SECRET-PROMPT-TEXT"
	const secretKey = "sk-test-secret-key-123"

	cases := []struct {
		name     string
		durable  bool
		handler  http.HandlerFunc
		wantAll  []string
		wantNone []string
	}{
		{
			name: "500 JSON body, legacy job path",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"message":"slot 0 unavailable: context shift failed","type":"server_error"},"echo":"` + secretPrompt + `"}`))
			},
			wantAll:  []string{"suggestion failed: provider failure", "status 500", "slot 0 unavailable: context shift failed"},
			wantNone: []string{secretPrompt, secretKey},
		},
		{
			name:    "500 JSON body, durable workflow path",
			durable: true,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"message":"model not found; key ` + secretKey + ` rejected"}}`))
			},
			wantAll:  []string{"status 500", "model not found"},
			wantNone: []string{secretKey},
		},
		{
			name: "non-JSON body never leaks, status still shown",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("<html>" + secretPrompt + "</html>"))
			},
			wantAll:  []string{"status 502"},
			wantNone: []string{secretPrompt},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := httptest.NewServer(tc.handler)
			t.Cleanup(provider.Close)

			st := newStore(t)
			ms := testkit.NewMediaServer(t)
			lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
			tm := tmdb.NewWithBase(testkit.NewTMDB(t).URL, "key")
			sug := suggest.New(llm.NewOpenAI(provider.URL, "m", secretKey), catalog.New(lib, tm), tm, 10)
			svc := suggest.NewService(st, sug, suggest.Config{Workers: 1, Timeout: 5 * time.Second, CacheTTL: time.Hour},
				idGen(), time.Now, testkit.Logger())
			if tc.durable {
				svc = svc.WithDurableWorkflow(proposalworkflow.New(st, func() string { return "wf-1" }, time.Now))
			}
			jobID, err := svc.Submit(context.Background(), suggest.Intent{Description: "Classic Simpsons episodes"}, "alice")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go svc.Run(ctx)

			var job store.Job
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				job, _ = st.GetJob(context.Background(), jobID)
				if job.Status == "failed" {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if job.Status != "failed" {
				t.Fatalf("job status = %q, want failed", job.Status)
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(job.LastError, want) {
					t.Errorf("last_error %q missing %q", job.LastError, want)
				}
			}
			for _, bad := range tc.wantNone {
				if strings.Contains(job.LastError, bad) {
					t.Errorf("last_error %q leaks %q", job.LastError, bad)
				}
			}
			if len(job.LastError) > 400 {
				t.Errorf("last_error is %d bytes; want the provider cause bounded", len(job.LastError))
			}
		})
	}
}
