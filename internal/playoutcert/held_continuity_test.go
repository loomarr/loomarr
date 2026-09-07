package playoutcert

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestRunDoesNotTreatUnreadPreOverloadBacklogAsSustainedHeldMedia(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	fixture.HeldBufferedBytes = 64 << 10
	fixture.StallHeldAfterOverload = true
	config := fixtureConfig(fixture, fixtureChannels(100))
	config.Client = fixture.BackloggedHeldClient()

	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	phase := report.PhaseMust("overload")
	if report.Certified || phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("finite pre-overload backlog certified as sustained media: %+v", phase)
	}
	encoded, err := json.Marshal(phase)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		HeldContinuity []struct {
			Outcome string `json:"outcome"`
		} `json:"heldContinuity"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.HeldContinuity) != 4 {
		t.Fatalf("held continuity observations = %d, want 4", len(document.HeldContinuity))
	}
	for _, observation := range document.HeldContinuity {
		if observation.Outcome != "stalled" {
			t.Fatalf("held continuity outcome = %q, want stalled", observation.Outcome)
		}
	}
	if active := fixture.BacklogReadsActive(); active != 0 {
		t.Fatalf("held body reads still active after Run: %d", active)
	}
}
