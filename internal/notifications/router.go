package notifications

import (
	"context"
	"fmt"
	"sort"
)

type DestinationSource interface {
	ListNotificationDestinations(context.Context) ([]Destination, error)
}

type AudienceEligibility interface {
	Eligible(context.Context, RecipientKind, string) (bool, error)
}

// DestinationRouter owns configurable product routing. Domain callers publish one audience-bound
// intent; this module alone selects enabled destinations and delivery means.
type DestinationRouter struct {
	source      DestinationSource
	eligibility AudienceEligibility
}

func NewDestinationRouter(source DestinationSource, eligibility AudienceEligibility) *DestinationRouter {
	return &DestinationRouter{source: source, eligibility: eligibility}
}

func (r *DestinationRouter) Routes(ctx context.Context, intent Intent) ([]Route, error) {
	if intent.Policy != PolicyConfigurable {
		return nil, fmt.Errorf("destination router requires a configurable notification intent")
	}
	if r == nil || r.source == nil {
		return nil, nil
	}
	destinations, err := r.source.ListNotificationDestinations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notification destinations: %w", err)
	}
	sort.Slice(destinations, func(i, j int) bool { return destinations[i].ID < destinations[j].ID })
	routes := make([]Route, 0, len(destinations))
	for _, destination := range destinations {
		if err := destination.Validate(); err != nil {
			return nil, fmt.Errorf("validate notification destination %q: %w", destination.ID, err)
		}
		if !destination.Enabled || !destinationHasTopic(destination, intent.Topic) {
			continue
		}
		if destination.Scope == ScopeInstallation {
			if !installationDestinationMatches(destination.Means, intent.Topic, intent.RecipientKind) {
				continue
			}
		} else if destination.Means == MeansWebPush {
			matches, matchErr := r.personalWebPushMatches(ctx, destination, intent)
			if matchErr != nil {
				return nil, matchErr
			}
			if !matches {
				continue
			}
		} else {
			if destination.Audience != intent.RecipientKind {
				continue
			}
			if intent.RecipientKind == RecipientPerson {
				if destination.OwnerID != intent.RecipientID {
					continue
				}
				if r.eligibility != nil {
					eligible, eligibilityErr := r.eligibility.Eligible(ctx, intent.RecipientKind, destination.OwnerID)
					if eligibilityErr != nil {
						return nil, fmt.Errorf("resolve person eligibility for %q: %w", destination.OwnerID, eligibilityErr)
					}
					if !eligible {
						continue
					}
				}
			} else {
				if r.eligibility == nil {
					continue
				}
				eligible, eligibilityErr := r.eligibility.Eligible(ctx, intent.RecipientKind, destination.OwnerID)
				if eligibilityErr != nil {
					return nil, fmt.Errorf("resolve %s eligibility for %q: %w", intent.RecipientKind, destination.OwnerID, eligibilityErr)
				}
				if !eligible {
					continue
				}
			}
		}
		routes = append(routes, Route{
			Means: destination.Means, DestinationRef: destination.ID,
			DestinationRedacted: destination.Label,
		})
	}
	return routes, nil
}

func (r *DestinationRouter) personalWebPushMatches(
	ctx context.Context,
	destination Destination,
	intent Intent,
) (bool, error) {
	if intent.RecipientKind == RecipientPerson {
		if destination.OwnerID != intent.RecipientID {
			return false, nil
		}
	} else if intent.RecipientKind != RecipientApprovers && intent.RecipientKind != RecipientOperators {
		return false, nil
	}
	if r.eligibility == nil {
		return intent.RecipientKind == RecipientPerson, nil
	}
	eligible, err := r.eligibility.Eligible(ctx, intent.RecipientKind, destination.OwnerID)
	if err != nil {
		return false, fmt.Errorf("resolve %s eligibility for %q: %w", intent.RecipientKind, destination.OwnerID, err)
	}
	return eligible, nil
}

func installationDestinationMatches(means Means, topic Topic, recipient RecipientKind) bool {
	// SMTP is one installation provider but remains recipient-aware: the adapter resolves the
	// intent's verified contact after claim. Shared endpoints receive only the canonical group
	// intent for an event, avoiding one duplicate delivery per affected requester.
	if means == MeansEmail {
		return true
	}
	switch topic {
	case TopicProposalSubmitted:
		return recipient == RecipientApprovers
	case TopicAcquisitionAvailable, TopicAcquisitionGaveUp, TopicChannelLive, TopicChannelDegraded:
		return recipient == RecipientOperators
	default:
		return false
	}
}

func destinationHasTopic(destination Destination, topic Topic) bool {
	for _, selected := range destination.Topics {
		if selected == topic {
			return true
		}
	}
	return false
}
