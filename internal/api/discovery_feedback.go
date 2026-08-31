package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
)

func (s *Server) registerDiscoveryFeedback(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "record-discovery-feedback", Method: http.MethodPost, Path: "/v1/discovery/feedback",
		Summary: "Record explicit household discovery feedback (admin)", Tags: []string{"discovery"},
	}, RoleAdmin), s.recordDiscoveryFeedback)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-discovery-feedback", Method: http.MethodGet, Path: "/v1/discovery/feedback",
		Summary: "List effective explicit discovery feedback", Tags: []string{"discovery"},
	}, RoleMember), s.listDiscoveryFeedback)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "clear-discovery-feedback", Method: http.MethodPost, Path: "/v1/discovery/feedback/clear",
		Summary: "Clear explicit household discovery feedback (admin)", Tags: []string{"discovery"},
	}, RoleAdmin), s.clearDiscoveryFeedback)
}

type discoveryFeedbackBody struct {
	Scope     store.FeedbackScope  `json:"scope" enum:"household,channel"`
	ScopeID   string               `json:"scopeId,omitempty"`
	TargetKey provision.Key        `json:"targetKey"`
	Action    store.FeedbackAction `json:"action" enum:"keep,less,never,surprise"`
	Reason    string               `json:"reason,omitempty" maxLength:"280"`
}

type discoveryFeedbackInput struct{ Body discoveryFeedbackBody }
type clearDiscoveryFeedbackInput struct {
	Body struct {
		Scope     store.FeedbackScope `json:"scope" enum:"household,channel"`
		ScopeID   string              `json:"scopeId,omitempty"`
		TargetKey provision.Key       `json:"targetKey"`
	}
}
type listDiscoveryFeedbackInput struct {
	Scope   store.FeedbackScope `query:"scope" enum:"household,channel"`
	ScopeID string              `query:"scopeId"`
}
type discoveryFeedbackOutput struct{ Body discoveryFeedbackDTO }
type listDiscoveryFeedbackOutput struct{ Body []discoveryFeedbackDTO }

type discoveryFeedbackDTO struct {
	ID        string               `json:"id"`
	ActorID   string               `json:"actorId"`
	Scope     store.FeedbackScope  `json:"scope"`
	ScopeID   string               `json:"scopeId,omitempty"`
	TargetKey provision.Key        `json:"targetKey"`
	Action    store.FeedbackAction `json:"action"`
	Reason    string               `json:"reason,omitempty"`
	CreatedAt time.Time            `json:"createdAt"`
}

func (s *Server) recordDiscoveryFeedback(ctx context.Context, in *discoveryFeedbackInput) (*discoveryFeedbackOutput, error) {
	return s.appendDiscoveryFeedback(ctx, in.Body.Scope, in.Body.ScopeID, in.Body.TargetKey, in.Body.Action, in.Body.Reason)
}

func (s *Server) clearDiscoveryFeedback(ctx context.Context, in *clearDiscoveryFeedbackInput) (*discoveryFeedbackOutput, error) {
	return s.appendDiscoveryFeedback(ctx, in.Body.Scope, in.Body.ScopeID, in.Body.TargetKey, store.FeedbackClear, "")
}

func (s *Server) appendDiscoveryFeedback(ctx context.Context, scope store.FeedbackScope, scopeID string,
	target provision.Key, action store.FeedbackAction, reason string,
) (*discoveryFeedbackOutput, error) {
	if s.store == nil {
		return nil, errFeatureNotConfigured("Discovery feedback unavailable", "The persistence service is not configured.")
	}
	if !validFeedbackRequest(scope, scopeID, target) {
		return nil, errBadRequest("Invalid discovery feedback", "Choose household scope or a specific channel and a grounded movie or series.")
	}
	actor := userIDFromHuma(ctx)
	if actor == "" {
		actor = "api-token"
	}
	feedback := store.DiscoveryFeedback{ID: "feedback_" + strings.TrimPrefix(newRequestID(), "req_"),
		ActorID: actor, Scope: scope, ScopeID: scopeID, Target: target, Action: action,
		Reason: strings.TrimSpace(reason), CreatedAt: time.Now().UTC()}
	if err := s.store.AppendDiscoveryFeedback(ctx, feedback); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
		}
		return nil, err
	}
	return &discoveryFeedbackOutput{Body: feedbackDTO(feedback)}, nil
}

func (s *Server) listDiscoveryFeedback(ctx context.Context, in *listDiscoveryFeedbackInput) (*listDiscoveryFeedbackOutput, error) {
	if s.store == nil || !validFeedbackRequest(in.Scope, in.ScopeID, "movie:tmdb:1") {
		return nil, errBadRequest("Invalid discovery feedback scope", "Choose household scope or a specific channel.")
	}
	events, err := s.store.ListDiscoveryFeedback(ctx, store.FeedbackFilter{Scope: in.Scope, ScopeID: in.ScopeID})
	if err != nil {
		return nil, err
	}
	seen := make(map[provision.Key]bool, len(events))
	out := &listDiscoveryFeedbackOutput{Body: make([]discoveryFeedbackDTO, 0, len(events))}
	for _, event := range events {
		if seen[event.Target] {
			continue
		}
		seen[event.Target] = true
		if event.Action != store.FeedbackClear {
			out.Body = append(out.Body, feedbackDTO(event))
		}
	}
	return out, nil
}

func validFeedbackRequest(scope store.FeedbackScope, scopeID string, target provision.Key) bool {
	validScope := (scope == store.FeedbackHousehold && scopeID == "") || (scope == store.FeedbackChannel && scopeID != "")
	validTarget := strings.HasPrefix(string(target), "movie:") || strings.HasPrefix(string(target), "series:")
	return validScope && validTarget
}

func feedbackDTO(feedback store.DiscoveryFeedback) discoveryFeedbackDTO {
	return discoveryFeedbackDTO{ID: feedback.ID, ActorID: feedback.ActorID, Scope: feedback.Scope,
		ScopeID: feedback.ScopeID, TargetKey: feedback.Target, Action: feedback.Action,
		Reason: feedback.Reason, CreatedAt: feedback.CreatedAt}
}
