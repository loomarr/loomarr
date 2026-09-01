package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

type productNotificationCoordinator struct {
	publisher *notifications.ProductPublisher
	source    productNotificationSource
	log       *slog.Logger
}

type productNotificationSource interface {
	ListNotificationReferenceRecipients(context.Context, notifications.ReferenceKind, string) ([]string, error)
	GetChannel(context.Context, string) (store.Channel, error)
}

func (c *productNotificationCoordinator) ProposalSubmitted(ctx context.Context, proposal store.Proposal) {
	c.publishProposal(ctx, proposal, notifications.TopicProposalSubmitted, "")
}

func (c *productNotificationCoordinator) ProposalApproved(ctx context.Context, proposal store.Proposal, _ string) {
	c.publishProposal(ctx, proposal, notifications.TopicProposalApproved, proposal.ModSummary)
}

func (c *productNotificationCoordinator) ProposalDeclined(ctx context.Context, proposal store.Proposal) {
	c.publishProposal(ctx, proposal, notifications.TopicProposalDeclined, proposal.DenyReason)
}

func (c *productNotificationCoordinator) Provisioned(ctx context.Context, event provision.DomainEvent) {
	if c == nil || c.publisher == nil || c.source == nil {
		return
	}
	topic := notifications.TopicAcquisitionGaveUp
	if event.State == provision.Available {
		topic = notifications.TopicAcquisitionAvailable
	} else if event.State != provision.Unavailable {
		return
	}
	people, err := c.source.ListNotificationReferenceRecipients(ctx, notifications.ReferenceTitle, string(event.Key))
	if err == nil {
		_, err = c.publisher.Publish(ctx, notifications.ProductEvent{
			Identity: "title:" + string(event.Key) + ":" + string(event.State), Topic: topic,
			ReferenceID: string(event.Key), SubjectName: event.Title.Name, PersonIDs: people,
		})
	}
	c.logPublicationFailure(err, "title", string(event.Key), topic)
}

func (c *productNotificationCoordinator) ChannelChanged(channelID, previousStatus, status string) {
	if c == nil || c.publisher == nil || c.source == nil {
		return
	}
	previousTopic, previousNotifiable := channelNotificationTopic(previousStatus)
	topic, notifiable := channelNotificationTopic(status)
	if !notifiable || (previousNotifiable && previousTopic == topic) {
		return
	}
	ctx := context.Background()
	channel, err := c.source.GetChannel(ctx, channelID)
	if err != nil {
		c.logPublicationFailure(err, "channel", channelID, "")
		return
	}
	people, err := c.source.ListNotificationReferenceRecipients(ctx, notifications.ReferenceChannel, channelID)
	if err == nil {
		_, err = c.publisher.Publish(ctx, notifications.ProductEvent{
			Identity: "channel:" + channelID + ":revision:" + fmt.Sprint(channel.Revision) + ":" + status,
			Topic:    topic, ReferenceID: channelID, SubjectName: channel.Name, PersonIDs: people,
		})
	}
	c.logPublicationFailure(err, "channel", channelID, topic)
}

func channelNotificationTopic(status string) (notifications.Topic, bool) {
	switch schedule.ChannelStatus(status) {
	case schedule.StatusLive:
		return notifications.TopicChannelLive, true
	case schedule.StatusDrifted, schedule.StatusEmpty:
		return notifications.TopicChannelDegraded, true
	default:
		return "", false
	}
}

func (c *productNotificationCoordinator) publishProposal(
	ctx context.Context,
	proposal store.Proposal,
	topic notifications.Topic,
	summary string,
) {
	if c == nil || c.publisher == nil {
		return
	}
	people := []string(nil)
	if topic != notifications.TopicProposalSubmitted {
		people = []string{proposal.CreatedBy}
	}
	_, err := c.publisher.Publish(ctx, notifications.ProductEvent{
		Identity: "proposal:" + proposal.ID + ":" + proposalEventIdentity(topic),
		Topic:    topic, ReferenceID: proposal.ID, SubjectName: proposalSubject(proposal),
		Summary: summary, PersonIDs: people,
	})
	if err != nil && c.log != nil {
		c.log.Warn("product notification could not be published", "topic", topic, "proposal", proposal.ID, "err", err)
	}
}

func (c *productNotificationCoordinator) logPublicationFailure(err error, referenceKind, referenceID string, topic notifications.Topic) {
	if err != nil && c.log != nil {
		c.log.Warn("product notification could not be published", "topic", topic,
			"reference_kind", referenceKind, "reference_id", referenceID, "err", err)
	}
}

func proposalSubject(proposal store.Proposal) string {
	var body suggest.Proposal
	if json.Unmarshal([]byte(proposal.ProposalJSON), &body) == nil {
		if name := strings.TrimSpace(body.ChannelName); name != "" {
			return name
		}
		if description := strings.TrimSpace(body.Intent.Description); description != "" {
			if len(description) > 200 {
				end := 200
				for end > 0 && !utf8.RuneStart(description[end]) {
					end--
				}
				return description[:end]
			}
			return description
		}
	}
	return "Channel proposal"
}

func proposalEventIdentity(topic notifications.Topic) string {
	switch topic {
	case notifications.TopicProposalSubmitted:
		return "submitted"
	case notifications.TopicProposalApproved:
		return "approved"
	case notifications.TopicProposalDeclined:
		return "declined"
	default:
		return "changed"
	}
}
