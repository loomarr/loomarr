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
