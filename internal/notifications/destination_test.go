package notifications

import (
	"testing"
	"time"
)

func TestDestinationValidationPinsScopeAudienceAndTopics(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	cases := []Destination{
		{
			ID: "destination-slack", Means: MeansSlack, Label: "Operations Slack",
			Scope: ScopeInstallation, Audience: RecipientOperators,
			Topics: []Topic{TopicAcquisitionGaveUp, TopicChannelDegraded}, Enabled: true,
			Configuration: map[string]string{"channel": "alerts"}, Credentials: map[string]string{"token": "secret"},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "destination-approvers", Means: MeansDiscord, Label: "Review room",
			Scope: ScopeInstallation, Audience: RecipientApprovers,
			Topics: []Topic{TopicProposalSubmitted}, Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "destination-person-email", Means: MeansEmail, Label: "My verified email",
			Scope: ScopePerson, OwnerID: "user-1", Audience: RecipientPerson,
			Topics: []Topic{TopicProposalApproved, TopicChannelLive}, Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "destination-approver-email", Means: MeansEmail, Label: "My review email",
			Scope: ScopePerson, OwnerID: "admin-1", Audience: RecipientApprovers,
			Topics: []Topic{TopicProposalSubmitted}, Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, destination := range cases {
		if err := destination.Validate(); err != nil {
			t.Errorf("%s: %v", destination.ID, err)
		}
	}

	invalid := cases[0]
	invalid.Audience = RecipientPerson
	if err := invalid.Validate(); err == nil {
		t.Fatal("installation destination accepted a person audience")
	}
	invalid = cases[2]
	invalid.Means = MeansSlack
	if err := invalid.Validate(); err == nil {
		t.Fatal("person destination accepted an installation delivery means")
	}
	invalid = cases[2]
	invalid.OwnerID = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("person destination accepted no owner")
	}
	invalid = cases[1]
	invalid.Topics = []Topic{TopicChannelDegraded}
	if err := invalid.Validate(); err == nil {
		t.Fatal("approver destination accepted an operator topic")
	}
	invalid = cases[0]
	invalid.Topics = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("destination accepted no selected topics")
	}
	invalid = cases[0]
	invalid.Label = "alerts\nAuthorization: bearer"
	if err := invalid.Validate(); err == nil {
		t.Fatal("destination accepted control characters in its label")
	}
}
