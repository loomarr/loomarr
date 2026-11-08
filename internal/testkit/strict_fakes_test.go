package testkit

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
)

// A shared fake refuses what the real thing refuses; one that accepts anything hides the bugs
// the tests above it exist to catch. Each test here pins one refusal of the real contract.

// Emby answers 401 to a library read without a valid API key, so a client that drops or
// mangles its token must fail against the double too.
func TestMediaServerRefusesAReadWithoutItsToken(t *testing.T) {
	ms := NewMediaServer(t)
	defer ms.Close()
	for _, path := range []string{"/Items?SearchTerm=matrix", "/Users", "/Library/VirtualFolders"} {
		for name, token := range map[string]string{"none": "", "wrong": "not-the-admin-token"} {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ms.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if token != "" {
				req.Header.Set("X-Emby-Token", token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("GET %s with %s token = %d, want 401", path, name, resp.StatusCode)
			}
		}
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, ms.URL+path, nil)
		req.Header.Set("X-Emby-Token", ms.AdminToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s with the admin token = %d, want 200", path, resp.StatusCode)
		}
	}
}

func publishedRecovery(t *testing.T) (*NotificationRepository, notifications.Intent) {
	t.Helper()
	repository := NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	n := 0
	ids := func() string { n++; return "id-" + string(rune('0'+n)) }
	router := NotificationRouter{RoutesResult: []notifications.Route{{
		Means: notifications.MeansEmail, DestinationRef: "contact-1", DestinationRedacted: "a***@example.com",
	}}}
	service := notifications.NewService(repository, router, nil, ids, func() time.Time { return now })
	intent, created, err := service.Publish(t.Context(), notifications.PublishCommand{
		Topic: notifications.TopicLocalPasswordRecovery, RecipientKind: notifications.RecipientPerson,
		RecipientID: "user-1", ReferenceKind: notifications.ReferenceRecovery, ReferenceID: "recovery-1",
		Policy: notifications.PolicyMandatoryAccount, IdempotencyKey: "recovery-1:created",
	})
	if err != nil || !created {
		t.Fatalf("publish = %+v, %t, %v", intent, created, err)
	}
	return repository, intent
}

// The store validates an intent and its initial attempts before writing either.
func TestNotificationRepositoryRefusesAnInvalidIntent(t *testing.T) {
	repository := NewNotificationRepository()
	if _, _, err := repository.CreateNotificationIntent(t.Context(), notifications.Intent{}, nil); err == nil {
		t.Fatal("an empty intent was stored")
	}
	if len(repository.Intents) != 0 {
		t.Fatal("a refused intent left a row behind")
	}
}

// Replaying an idempotency key with a different request is a conflict, not a silent replay.
func TestNotificationRepositoryRefusesAnIdempotencyKeyReusedForADifferentRequest(t *testing.T) {
	repository, intent := publishedRecovery(t)
	attempts, err := repository.ListNotificationAttempts(t.Context(), intent.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %+v, %v", attempts, err)
	}
	other := intent
	other.ID = "other-intent"
	other.ReferenceID = "recovery-2"
	otherAttempt := attempts[0]
	otherAttempt.ID, otherAttempt.IntentID = "other-attempt", other.ID
	_, created, err := repository.CreateNotificationIntent(t.Context(), other, []notifications.Attempt{otherAttempt})
	if !errors.Is(err, notifications.ErrConflict) || created {
		t.Fatalf("reused key = created %t, err %v; want ErrConflict", created, err)
	}
	if _, created, err := repository.CreateNotificationIntent(t.Context(), intent, attempts); err != nil || created {
		t.Fatalf("identical replay = created %t, err %v; want the existing intent", created, err)
	}
}

// Completing an attempt that does not exist is not-found; completing one with an invalid
// outcome is refused before anything changes.
func TestNotificationRepositoryRefusesAnInvalidCompletion(t *testing.T) {
	repository, _ := publishedRecovery(t)
	now := time.Unix(1_900_000_000, 0)
	if err := repository.CompleteNotificationAttempt(t.Context(), notifications.Completion{
		AttemptID: "missing", LeaseOwner: "worker-1", Status: notifications.StatusDelivered, FinishedAt: now,
	}); !errors.Is(err, notifications.ErrNotFound) {
		t.Fatalf("complete missing attempt = %v, want ErrNotFound", err)
	}
	claimed, err := repository.ClaimDueNotificationAttempt(t.Context(), "worker-1", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteNotificationAttempt(t.Context(), notifications.Completion{
		AttemptID: claimed.ID, LeaseOwner: "worker-1", Status: notifications.StatusDelivered,
	}); err == nil {
		t.Fatal("a completion without a finished time was accepted")
	}
	if got := repository.Attempts[claimed.ID]; got.Status != notifications.StatusSending {
		t.Fatalf("refused completion moved the attempt to %q", got.Status)
	}
}

func approvalCommit(t *testing.T) store.ProposalApproval {
	t.Helper()
	p := store.Proposal{ID: "proposal-1", JobID: "job-1", Status: "approved"}
	ch, err := (&ApprovalChannels{}).PlanApprovedChannel(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return store.ProposalApproval{Proposal: p, Channel: ch}
}

// The approval commit is a compare-and-swap from submitted: a proposal approves once.
func TestApprovalStoreRefusesASecondApprovalOfOneProposal(t *testing.T) {
	s := &ApprovalStore{}
	commit := approvalCommit(t)
	if _, err := s.CommitProposalApproval(t.Context(), commit); err != nil {
		t.Fatalf("first approval: %v", err)
	}
	if _, err := s.CommitProposalApproval(t.Context(), commit); !errors.Is(err, store.ErrProposalNotSubmitted) {
		t.Fatalf("second approval = %v, want ErrProposalNotSubmitted", err)
	}
}

// The store refuses a commit whose proposal is not approved, whose channel does not validate,
// or whose title key does not match its title.
func TestApprovalStoreRefusesAnInvalidCommit(t *testing.T) {
	cases := map[string]func(*store.ProposalApproval){
		"not approved":    func(c *store.ProposalApproval) { c.Proposal.Status = "submitted" },
		"no intent ref":   func(c *store.ProposalApproval) { c.Channel.IntentRef = "" },
		"invalid channel": func(c *store.ProposalApproval) { c.Channel.Name = "" },
		"title key mismatch": func(c *store.ProposalApproval) {
			c.Titles = []provision.Record{{
				Key:   "movie:tmdb:90000001",
				Title: provision.Title{Name: "Two", MediaType: provision.Movie, TMDBID: 90000002},
				State: provision.Wanted, Deadline: time.Unix(1_900_000_000, 0),
			}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := &ApprovalStore{}
			commit := approvalCommit(t)
			mutate(&commit)
			if _, err := s.CommitProposalApproval(t.Context(), commit); err == nil {
				t.Fatal("an invalid approval committed")
			}
		})
	}
}

// Neither Seerr nor the arrs can request a title without a usable provider id.
func TestRequesterRefusesATitleWithoutAProviderID(t *testing.T) {
	r := &Requester{}
	title := provision.Title{Name: "Nameless", MediaType: provision.Movie}
	if err := r.Request(context.Background(), title); err == nil {
		t.Fatal("a title without a provider id was requested")
	}
}
