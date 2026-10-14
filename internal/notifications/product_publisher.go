package notifications

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

type IntentPublisher interface {
	Publish(context.Context, PublishCommand) (Intent, bool, error)
}

// ProductEvent is the provider-neutral fact presented by composition after an authoritative domain
// transition. Identity names that exact transition and must remain stable across caller retries.
type ProductEvent struct {
	Identity    string
	Topic       Topic
	ReferenceID string
	SubjectName string
	Summary     string
	PersonIDs   []string
}

type PublicationResult struct {
	Requested int
	Created   int
}

// ProductPublisher owns audience expansion and idempotency. Domain transition code supplies the
// fact and affected people but never chooses destinations or delivery means.
type ProductPublisher struct {
	sink IntentPublisher
}

func NewProductPublisher(sink IntentPublisher) *ProductPublisher {
	return &ProductPublisher{sink: sink}
}

func (p *ProductPublisher) Publish(ctx context.Context, event ProductEvent) (PublicationResult, error) {
	if p == nil || p.sink == nil {
		return PublicationResult{}, fmt.Errorf("product notification publisher is unavailable")
	}
	if err := identifier("product event identity", event.Identity); err != nil {
		return PublicationResult{}, err
	}
	if err := identifier("product event reference", event.ReferenceID); err != nil {
		return PublicationResult{}, err
	}
	people, err := uniqueSortedIDs(event.PersonIDs)
	if err != nil {
		return PublicationResult{}, err
	}
	reference, recipients, err := productRecipients(event.Topic, people)
	if err != nil {
		return PublicationResult{}, err
	}
	result := PublicationResult{Requested: len(recipients)}
	var publicationErrors []error
	for _, recipient := range recipients {
		_, created, publishErr := p.sink.Publish(ctx, PublishCommand{
			Topic: event.Topic, RecipientKind: recipient.kind, RecipientID: recipient.id,
			ReferenceKind: reference, ReferenceID: event.ReferenceID, Policy: PolicyConfigurable,
			Template:       TemplateData{SubjectName: event.SubjectName, Summary: event.Summary},
			IdempotencyKey: fmt.Sprintf("notification:%s:%s:%s", event.Identity, recipient.kind, recipient.id),
		})
		if publishErr != nil {
			publicationErrors = append(publicationErrors, publishErr)
			continue
		}
		if created {
			result.Created++
		}
	}
	return result, errors.Join(publicationErrors...)
}

type productRecipient struct {
	kind RecipientKind
	id   string
}

func productRecipients(topic Topic, people []string) (ReferenceKind, []productRecipient, error) {
	switch topic {
	case TopicProposalSubmitted:
		return ReferenceProposal, []productRecipient{{kind: RecipientApprovers, id: string(RecipientApprovers)}}, nil
	case TopicProposalApproved, TopicProposalDeclined:
		if len(people) != 1 {
			return "", nil, fmt.Errorf("proposal decision notification requires exactly one requester")
		}
		return ReferenceProposal, []productRecipient{{kind: RecipientPerson, id: people[0]}}, nil
	case TopicAcquisitionAvailable, TopicAcquisitionGaveUp:
		return ReferenceTitle, peopleAndOperators(people), nil
	case TopicChannelLive, TopicChannelDegraded:
		return ReferenceChannel, peopleAndOperators(people), nil
	default:
		return "", nil, fmt.Errorf("topic %q is not a product event", topic)
	}
}

func peopleAndOperators(people []string) []productRecipient {
	recipients := make([]productRecipient, 0, len(people)+1)
	for _, personID := range people {
		recipients = append(recipients, productRecipient{kind: RecipientPerson, id: personID})
	}
	return append(recipients, productRecipient{kind: RecipientOperators, id: string(RecipientOperators)})
}

func uniqueSortedIDs(ids []string) ([]string, error) {
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if err := identifier("product event person", id); err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	sort.Strings(unique)
	return unique, nil
}
