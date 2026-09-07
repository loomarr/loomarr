package playoutcert

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

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

func TestSyntheticBoundaryWitnessRefusesMalformedInitialAndNextIdentity(t *testing.T) {
	witness := newSyntheticBoundaryWitness()
	subscription, err := witness.Subscribe("channel-a")
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()

	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: playout.AiringIdentity{}})
	if err := subscription.WaitInitial(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid initial") {
		t.Fatalf("malformed initial error = %v", err)
	}

	first := testAiring(time.Unix(100, 0), "one")
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: first})
	if err := subscription.WaitInitial(context.Background()); err != nil {
		t.Fatal(err)
	}
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: playout.AiringIdentity{}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := subscription.WaitTransition(ctx); err == nil || !strings.Contains(err.Error(), "invalid next") {
		t.Fatalf("malformed next error = %v", err)
	}
}

func TestSyntheticBoundaryWitnessRejectsMalformedNextBeforeLaterValidForwardEvent(t *testing.T) {
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
	// A malformed event from another parent remains irrelevant. A malformed
	// event from the admitted parent poisons this witness, even when a valid
	// forward event follows it in the same queued burst.
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 8, identity: playout.AiringIdentity{}})
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: playout.AiringIdentity{}})
	witness.publish("channel-a", syntheticBoundaryEvent{sourceID: 7, identity: testAiring(first.EndsAt, "two")})
	if err := subscription.WaitTransition(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid next") {
		t.Fatalf("malformed same-parent event must reject the witness before later good event: %v", err)
	}
}

func TestObserveProgrammeBoundaryPostValidationTerminalBodiesJoinLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal error
	}{
		{name: "EOF", terminal: io.EOF},
		{name: "read error", terminal: errors.New("controlled post-validation read error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 100)
			fixture.RawOpened = make(chan struct{}, 1)
			fixture.HeldBufferedBytes = 188
			decoder := &playoutcertfixture.Decoder{}
			witness := newSyntheticBoundaryWitness()
			config := fixtureConfig(fixture, fixtureChannels(100)).normalized()
			config.Decoder, config.ProgrammeBoundaryWitness = decoder, witness
			config.ProgrammeBoundaryLateObservation = 20 * time.Millisecond
			var rawClosed atomic.Int32
			gate := make(chan struct{})
			base := fixture.Server.Client().Transport
			config.Client = &http.Client{Transport: httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				response, err := base.RoundTrip(request)
				if err == nil && strings.HasPrefix(request.URL.Path, "/v1/playout/stream/") && response.StatusCode == http.StatusOK {
					response.Body = playoutcertfixture.NewGatedTerminalBody(response.Body, gate, config.RawCaptureBytes, tc.terminal, &rawClosed)
				}
				return response, err
			})}
			endpoint, err := newEndpoint(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			resultCh := make(chan boundaryLaneResult, 1)
			go func() {
				resultCh <- observeProgrammeBoundary(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
			}()
			waitForRawOpened(t, fixture.RawOpened, ctx)
			first := testAiring(time.Now().UTC(), "one")
			witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: first})
			waitForWitnessDrain(t, witness, config.Channels[0].ID, ctx)
			witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: testAiring(first.EndsAt, "two")})
			waitForWitnessDrain(t, witness, config.Channels[0].ID, ctx)
			close(gate)
			result := <-resultCh
			if result.observation.class != "post_boundary_decode_failed" {
				t.Fatalf("outcome = %q, want post_boundary_decode_failed", result.observation.class)
			}
			assertBoundaryLifecycle(t, witness, decoder, rawClosed.Load())
		})
	}
}

func TestObserveProgrammeBoundaryExternalCancellationAfterInitialWitnessJoinsLifecycle(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	fixture.RawOpened = make(chan struct{}, 1)
	fixture.HeldBufferedBytes = 188
	decoder := &playoutcertfixture.Decoder{}
	witness := newSyntheticBoundaryWitness()
	config := fixtureConfig(fixture, fixtureChannels(100)).normalized()
	config.Decoder, config.ProgrammeBoundaryWitness = decoder, witness
	var rawClosed atomic.Int32
	gate := make(chan struct{})
	base := fixture.Server.Client().Transport
	config.Client = &http.Client{Transport: httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(request)
		if err == nil && strings.HasPrefix(request.URL.Path, "/v1/playout/stream/") && response.StatusCode == http.StatusOK {
			response.Body = playoutcertfixture.NewGatedTerminalBody(response.Body, gate, config.RawCaptureBytes, io.EOF, &rawClosed)
		}
		return response, err
	})}
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resultCh := make(chan boundaryLaneResult, 1)
	go func() {
		resultCh <- observeProgrammeBoundary(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
	}()
	waitForRawOpened(t, fixture.RawOpened, ctx)
	first := testAiring(time.Now().UTC(), "one")
	witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: first})
	waitForWitnessDrain(t, witness, config.Channels[0].ID, ctx)
	cancel()
	result := <-resultCh
	if result.observation.class != "transition_cancelled" {
		t.Fatalf("outcome = %q, want transition_cancelled", result.observation.class)
	}
	assertBoundaryLifecycle(t, witness, decoder, rawClosed.Load())
}

func waitForWitnessDrain(t *testing.T, witness *syntheticBoundaryWitness, channelID string, ctx context.Context) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		witness.mu.Lock()
		drained := false
		for subscription := range witness.subscribers[channelID] {
			drained = len(subscription.events) == 0
		}
		witness.mu.Unlock()
		if drained {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("witness event did not drain: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForRawOpened(t *testing.T, opened <-chan struct{}, ctx context.Context) {
	t.Helper()
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatalf("raw stream did not open: %v", ctx.Err())
	}
}

func assertBoundaryLifecycle(t *testing.T, witness *syntheticBoundaryWitness, decoder *playoutcertfixture.Decoder, rawClosed int32) {
	t.Helper()
	if subscriptions := boundarySubscriberCount(witness); subscriptions != 0 || rawClosed != 1 {
		t.Fatalf("lifecycle subscribers=%d raw closes=%d", subscriptions, rawClosed)
	}
	active, started, stopped := decoder.Counts()
	if active != 0 || started != 1 || stopped != 1 {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
}

func TestObserveProgrammeBoundaryTimeoutsJoinRawBodyDecoderAndSubscription(t *testing.T) {
	for _, tc := range []struct {
		name string
		send bool
		want string
	}{
		{name: "initial", want: "initial_witness_timeout"},
		{name: "transition", send: true, want: "transition_timeout"},
		{name: "post transition", send: true, want: "post_boundary_stalled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 100)
			fixture.RawOpened = make(chan struct{}, 1)
			decoder := &playoutcertfixture.Decoder{}
			witness := newSyntheticBoundaryWitness()
			config := fixtureConfig(fixture, fixtureChannels(100)).normalized()
			config.Decoder = decoder
			config.ProgrammeBoundaryWitness = witness
			config.ProgrammeBoundaryLateObservation = 25 * time.Millisecond
			var rawClosed atomic.Int32
			base := fixture.Server.Client().Transport
			config.Client = &http.Client{Transport: httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				response, err := base.RoundTrip(request)
				if err == nil && strings.HasPrefix(request.URL.Path, "/v1/playout/stream/") {
					response.Body = &playoutcertfixture.CountingReadCloser{ReadCloser: response.Body, Closed: &rawClosed}
				}
				return response, err
			})}
			endpoint, err := newEndpoint(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			if tc.send {
				signalDone := make(chan struct{})
				go func() {
					defer close(signalDone)
					select {
					case <-fixture.RawOpened:
					case <-ctx.Done():
						return
					}
					first := testAiring(time.Now().UTC(), "one")
					witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: first})
					if tc.name == "post transition" {
						witness.publish(config.Channels[0].ID, syntheticBoundaryEvent{sourceID: 1, identity: testAiring(first.EndsAt, "two")})
					}
				}()
				t.Cleanup(func() {
					cancel()
					<-signalDone
				})
			} else {
				t.Cleanup(cancel)
			}
			result := observeProgrammeBoundary(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
			if result.observation.class != tc.want {
				t.Fatalf("outcome = %q, want %q", result.observation.class, tc.want)
			}
			if subscriptions := boundarySubscriberCount(witness); subscriptions != 0 || rawClosed.Load() != 1 {
				t.Fatalf("lifecycle subscribers=%d raw closes=%d", subscriptions, rawClosed.Load())
			}
			active, started, stopped := decoder.Counts()
			if active != 0 || started != 1 || stopped != 1 {
				t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
			}
		})
	}
}

func boundarySubscriberCount(witness *syntheticBoundaryWitness) int {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	count := 0
	for _, subscribers := range witness.subscribers {
		count += len(subscribers)
	}
	return count
}

func testAiring(start time.Time, id string) playout.AiringIdentity {
	return playout.AiringIdentity{StartedAt: start, EndsAt: start.Add(6 * time.Second), Kind: schedule.SlotProgram, ContentID: id, ScheduleBlockID: "block-" + id}
}
