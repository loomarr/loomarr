package playoutcert

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestPreparedHLSReplacementEpochRejectsMissingAudio(t *testing.T) {
	fixture := playoutcertfixture.NewPreparedHLS(t, playoutcertfixture.PreparedHLSReplacement)
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}, {VideoStreams: 1}}}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Channels: []Channel{{ID: "prepared", Roles: []string{"prepared"}}}, RequestTimeout: time.Second, RawCaptureBytes: 188, Validator: validator, Decoder: &playoutcertfixture.Decoder{}}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := observePreparedProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	if result.observation.class != "invalid_media" || validator.Calls() != 2 {
		t.Fatalf("replacement epoch=%+v validation calls=%d", result, validator.Calls())
	}
}

func TestPreparedHLSBlockedRefreshCancelsBeforeObserverJoin(t *testing.T) {
	fixture := playoutcertfixture.NewPreparedHLS(t, playoutcertfixture.PreparedHLSBlockedRefresh)
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1}}, WaitFor: fixture.Blocked}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Channels: []Channel{{ID: "prepared", Roles: []string{"prepared"}}}, RequestTimeout: 5 * time.Second, RawCaptureBytes: 188, Validator: validator, Decoder: &playoutcertfixture.Decoder{}}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	resultCh := make(chan boundaryLaneResult, 1)
	go func() {
		resultCh <- observePreparedProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	}()
	select {
	case <-fixture.Blocked:
	case <-time.After(time.Second):
		t.Fatal("prepared refresh did not block")
	}
	select {
	case result := <-resultCh:
		if result.observation.class != "invalid_media" {
			t.Fatalf("early validation failure after blocked refresh result=%+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("early validation failure did not cancel blocked refresh before observer join")
	}
	select {
	case <-fixture.Canceled:
	case <-time.After(time.Second):
		t.Fatal("early validation failure did not cancel the live blocked refresh")
	}
}

func TestPreparedHLSConsumesQueuedTransitionAfterObservationExpiry(t *testing.T) {
	fixture := playoutcertfixture.NewPreparedHLS(t, playoutcertfixture.PreparedHLSQueuedTransition)
	started := make(chan int, 3)
	completed := make(chan int, 3)
	releaseValidation := make(chan struct{})
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{
		Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, WaitFor: releaseValidation, WaitForCall: 2, CallStarted: started,
	}
	decoder := &playoutcertfixture.Decoder{CallCompleted: completed}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Channels: []Channel{{ID: "prepared", Roles: []string{"prepared"}}}, RequestTimeout: time.Second, RawCaptureBytes: 188, ProgrammeBoundaryLateObservation: 250 * time.Millisecond, Validator: validator, Decoder: decoder}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resultCh := make(chan boundaryLaneResult, 1)
	go func() {
		resultCh <- observePreparedProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	}()
	for want := 1; want <= 2; want++ {
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("validation call = %d, want %d", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	select {
	case got := <-completed:
		if got != 1 {
			t.Fatalf("decoder completion = %d, want initial epoch", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The second epoch is live until the legal observation window ends. Its
	// final bytes then make the reader refresh, queue the next discontinuity,
	// and return EOF to decoder call two. Validation remains blocked until that
	// completion, so the observer sees fresh progress and EOF together.
	timer := time.NewTimer(config.ProgrammeBoundaryLateObservation)
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		t.Fatal(ctx.Err())
	}
	fixture.ReleaseNext <- struct{}{}
	select {
	case <-fixture.TransitionQueued:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case got := <-completed:
		if got != 2 {
			t.Fatalf("decoder completion = %d, want second epoch", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	close(releaseValidation)
	select {
	case result := <-resultCh:
		if result.observation.class != "ok" || result.evidence.Transitions < 2 || result.evidence.ReadDelta <= 0 {
			t.Fatalf("queued post-window transition = %+v", result)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestPreparedHLSCancellationAfterQualifiedEpochJoinsReadersAndDecoders(t *testing.T) {
	fixture := playoutcertfixture.NewPreparedHLS(t, playoutcertfixture.PreparedHLSBlockedAfterQualified)
	started := make(chan int, 2)
	decoder := &playoutcertfixture.Decoder{}
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, CallStarted: started}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Channels: []Channel{{ID: "prepared", Roles: []string{"prepared"}}}, RequestTimeout: time.Second, RawCaptureBytes: 188, ProgrammeBoundaryLateObservation: 250 * time.Millisecond, Validator: validator, Decoder: decoder}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan boundaryLaneResult, 1)
	go func() {
		resultCh <- observePreparedProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	}()
	for want := 1; want <= 2; want++ {
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("validation call = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("qualified epoch did not validate")
		}
	}
	select {
	case <-fixture.Blocked:
	case <-time.After(time.Second):
		t.Fatal("post-qualified refresh did not block")
	}
	cancel()
	select {
	case result := <-resultCh:
		if result.observation.class == "ok" {
			t.Fatalf("cancellation qualified unexpectedly: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not join")
	}
	select {
	case <-fixture.Canceled:
	case <-time.After(time.Second):
		t.Fatal("blocked refresh was not canceled")
	}
	if active, started, stopped := decoder.Counts(); active != 0 || started != stopped {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
}

func TestPreparedHLSTerminalEpochWithoutNextTransitionCannotQualify(t *testing.T) {
	fixture := playoutcertfixture.NewPreparedHLS(t, playoutcertfixture.PreparedHLSBlockedAfterQualified)
	started := make(chan int, 2)
	decoder := &playoutcertfixture.Decoder{}
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, CallStarted: started}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Channels: []Channel{{ID: "prepared", Roles: []string{"prepared"}}}, RequestTimeout: time.Second, RawCaptureBytes: 188, ProgrammeBoundaryLateObservation: 250 * time.Millisecond, Validator: validator, Decoder: decoder}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	resultCh := make(chan boundaryLaneResult, 1)
	go func() {
		resultCh <- observePreparedProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	}()
	for want := 1; want <= 2; want++ {
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("validation call = %d, want %d", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	select {
	case <-fixture.Blocked:
	case <-ctx.Done():
		t.Fatal("terminal epoch did not reach its blocked refresh")
	}
	result := <-resultCh
	if result.observation.class == "ok" || result.evidence.Transitions != 0 {
		t.Fatalf("terminal epoch qualified from buffered bytes: %+v", result)
	}
	select {
	case <-fixture.Canceled:
	case <-time.After(time.Second):
		t.Fatal("terminal blocked refresh was not canceled")
	}
	if active, started, stopped := decoder.Counts(); active != 0 || started != stopped {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
}
