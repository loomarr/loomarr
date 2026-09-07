package playoutcert

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestSyntheticProgrammeScheduleHasSuccessiveCurrentNextAndDiscontinuity(t *testing.T) {
	epoch := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	programmeSchedule := syntheticProgrammeSchedule{epoch: epoch, duration: 6 * time.Second}
	current, next, offset := programmeSchedule.airings(epoch.Add(18*time.Second), "channel-a")
	if current.StartedAt != epoch.Add(18*time.Second) || current.EndsAt != epoch.Add(24*time.Second) || next.StartedAt != current.EndsAt || next.EndsAt != epoch.Add(30*time.Second) || offset != 0 {
		t.Fatalf("exact boundary airings current=%+v next=%+v offset=%s", current, next, offset)
	}
	if current.ScheduleBlockID != "synthetic-block-3" || next.ScheduleBlockID != "synthetic-block-4" || current.ContentID == next.ContentID || current.Kind != schedule.SlotProgram {
		t.Fatalf("successive identity/discontinuity = current=%+v next=%+v", current, next)
	}
	current, next, offset = programmeSchedule.airings(epoch.Add(23*time.Second), "channel-a")
	if current.StartedAt != epoch.Add(18*time.Second) || next.StartedAt != epoch.Add(24*time.Second) || offset != 5*time.Second {
		t.Fatalf("mid-epoch current/next/offset = current=%+v next=%+v offset=%s", current, next, offset)
	}
}

func TestObserveProgrammeBoundaryRejectsEarlyBurstThenOpenStall(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	fixture.BoundaryEarlyBurstDelay = 25 * time.Millisecond
	config := fixtureConfig(fixture, fixtureChannels(100)).normalized()
	config.ProgrammeBoundaryTimeout = time.Second
	config.ProgrammeBoundaryLateObservation = 100 * time.Millisecond
	witness := newSyntheticBoundaryWitness()
	config.ProgrammeBoundaryWitness = witness
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}

	first := testAiring(time.Now().UTC(), "one")
	go func() {
		time.Sleep(10 * time.Millisecond)
		witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: first})
		time.Sleep(10 * time.Millisecond)
		witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: testAiring(first.EndsAt, "two")})
	}()
	result := observeProgrammeBoundary(context.Background(), endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
	if result.evidence.Outcome != "post_boundary_stalled" || result.observation.class != "post_boundary_stalled" {
		t.Fatalf("early burst followed by open stall = evidence=%+v observation=%+v", result.evidence, result.observation)
	}
}

func TestSyntheticBoundaryWitnessRequiresSameParentAndSuccessiveIdentity(t *testing.T) {
	witness := newSyntheticBoundaryWitness()
	subscription, err := witness.Subscribe("channel-a")
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	first := testAiring(time.Unix(100, 0), "one")
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: first})
	if err := subscription.WaitInitial(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Neither another parent nor duplicate, stale, or backwards
	// identities establish the next programme for this admitted source.
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 8, identity: testAiring(first.EndsAt, "two")})
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: first})
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: testAiring(first.StartedAt.Add(-6*time.Second), "old")})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := subscription.WaitTransition(ctx); err == nil {
		t.Fatal("non-successive witness event established a transition")
	}
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: testAiring(first.EndsAt, "two")})
	if err := subscription.WaitTransition(context.Background()); err != nil {
		t.Fatalf("successive same-parent transition = %v", err)
	}
}

func testAiring(start time.Time, id string) playout.AiringIdentity {
	return playout.AiringIdentity{StartedAt: start, EndsAt: start.Add(6 * time.Second), Kind: schedule.SlotProgram, ContentID: id, ScheduleBlockID: "block-" + id}
}
