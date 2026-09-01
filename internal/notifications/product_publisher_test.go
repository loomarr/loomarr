package notifications_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/notifications"
)

type commandPublisher struct {
	commands []notifications.PublishCommand
}

func (p *commandPublisher) Publish(
	_ context.Context,
	command notifications.PublishCommand,
) (notifications.Intent, bool, error) {
	p.commands = append(p.commands, command)
	return notifications.Intent{}, true, nil
}

func TestProductPublisherExpandsAcquisitionToRequesterAndOperatorsWithStableIdentity(t *testing.T) {
	sink := &commandPublisher{}
	publisher := notifications.NewProductPublisher(sink)
	result, err := publisher.Publish(t.Context(), notifications.ProductEvent{
		Identity: "title:movie:tmdb:603:available", Topic: notifications.TopicAcquisitionAvailable,
		ReferenceID: "movie:tmdb:603", SubjectName: "The Matrix", Summary: "Ready to schedule.",
		PersonIDs: []string{"user-2", "user-1", "user-1"},
	})
	if err != nil || result.Requested != 3 {
		t.Fatalf("publish acquisition = %+v, %v", result, err)
	}
	if len(sink.commands) != 3 {
		t.Fatalf("commands = %+v", sink.commands)
	}
	want := []struct {
		recipient notifications.RecipientKind
		id, key   string
	}{
		{notifications.RecipientPerson, "user-1", "notification:title:movie:tmdb:603:available:person:user-1"},
		{notifications.RecipientPerson, "user-2", "notification:title:movie:tmdb:603:available:person:user-2"},
		{notifications.RecipientOperators, "operators", "notification:title:movie:tmdb:603:available:operators:operators"},
	}
	for index, expected := range want {
		command := sink.commands[index]
		if command.RecipientKind != expected.recipient || command.RecipientID != expected.id ||
			command.IdempotencyKey != expected.key || command.ReferenceKind != notifications.ReferenceTitle ||
			command.ReferenceID != "movie:tmdb:603" || command.Template.SubjectName != "The Matrix" {
			t.Errorf("command %d = %+v", index, command)
		}
	}
}

func TestProductPublisherUsesGroupAndPersonAudiencesForProposalLifecycle(t *testing.T) {
	sink := &commandPublisher{}
	publisher := notifications.NewProductPublisher(sink)
	if _, err := publisher.Publish(t.Context(), notifications.ProductEvent{
		Identity: "proposal:proposal-1:submitted", Topic: notifications.TopicProposalSubmitted,
		ReferenceID: "proposal-1", SubjectName: "Saturday Cartoons",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(t.Context(), notifications.ProductEvent{
		Identity: "proposal:proposal-1:approved", Topic: notifications.TopicProposalApproved,
		ReferenceID: "proposal-1", SubjectName: "Saturday Cartoons", PersonIDs: []string{"user-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.commands) != 2 || sink.commands[0].RecipientKind != notifications.RecipientApprovers ||
		sink.commands[1].RecipientKind != notifications.RecipientPerson || sink.commands[1].RecipientID != "user-1" {
		t.Fatalf("proposal commands = %+v", sink.commands)
	}
}
