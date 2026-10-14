package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/proposaloutlook"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit/outlookfixture"
)

func TestProposalOutlookIsAuthenticatedReadOnlyAndCannotApprove(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/outlook.db")
	t.Cleanup(func() { _ = st.Close() })
	seedProposal(t, st, "outlook")
	log := slog.New(slog.DiscardHandler)
	planner := binder.New(st, nil, nil, log)
	engine := channels.New(st, nil, nil, nil, channels.Config{}, time.Now, log)
	media := &outlookfixture.OutlookLibrary{}
	observer := proposaloutlook.New(proposaloutlook.Config{Titles: st, Library: media, Episodes: media.ResolveEpisodes, Planner: planner, Preview: engine})
	handler := api.Router(log, api.Options{Store: st, Auth: testAuthorizer{}, Log: log, ProposalOutlook: observer, Approver: suggest.NewApprover(st, planner, time.Now)})
	request := func(token, route string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, route, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	for _, tc := range []struct {
		name, token string
		status      int
	}{{"anonymous", "", http.StatusUnauthorized}, {"member", memberToken, http.StatusOK}, {"admin", adminToken, http.StatusOK}} {
		t.Run(tc.name, func(t *testing.T) {
			res := request(tc.token, "/v1/proposals/outlook/outlook")
			if res.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
			if res.Code == http.StatusOK {
				var payload struct {
					Relaxations []schedule.AppliedRelaxation `json:"relaxations"`
				}
				if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Relaxations == nil {
					t.Fatal("outlook must encode an empty relaxation list, not null, for the generated client")
				}
			}
		})
	}
	if res := request(memberToken, "/v1/proposals/outlook/approve"); res.Code != http.StatusForbidden {
		t.Fatalf("member preview must never grant approval: %d", res.Code)
	}
	proposal, err := st.GetProposal(context.Background(), "outlook")
	if err != nil || proposal.Status != "submitted" {
		t.Fatalf("observation changed proposal decision: %+v, %v", proposal, err)
	}
	wanted, err := st.ListTitlesByState(context.Background(), provision.Wanted)
	if err != nil || len(wanted) != 0 {
		t.Fatalf("observation acquired media: %+v, %v", wanted, err)
	}
	localChannels, err := st.ListChannels(context.Background())
	if err != nil || len(localChannels) != 0 {
		t.Fatalf("observation persisted a channel: %+v, %v", localChannels, err)
	}
	if res := request(adminToken, "/v1/proposals/outlook/approve"); res.Code != http.StatusOK {
		t.Fatalf("normal admin approval failed: %d: %s", res.Code, res.Body.String())
	}
	if res := request(memberToken, "/v1/proposals/outlook/outlook"); res.Code != http.StatusConflict {
		t.Fatalf("handled proposal must not produce a stale pre-approval outlook: %d", res.Code)
	}
}
