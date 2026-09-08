package playoutcert

import (
	"context"
	"fmt"
	"io"
	"testing"
)

func FaultQualificationsForTest(profiles []FaultProfile, controller FaultController) ([]FaultQualification, []string) {
	return faultQualifications(profiles, controller)
}

func NormalizeConfigForTest(config Config) Config { return config.normalized() }

type EndpointForTest = endpoint

func NewEndpointForTest(config Config) (*EndpointForTest, error) { return newEndpoint(config) }

type ProgrammeBoundaryResultForTest struct {
	Class    string
	Evidence ProgrammeBoundaryObservation
}

func ObserveProgrammeBoundaryForTest(ctx context.Context, endpoint *EndpointForTest, config Config, laneName string, channelIndex int) ProgrammeBoundaryResultForTest {
	result := observeProgrammeSignals(ctx, endpoint, config, boundaryLane{name: laneName, channelIndex: channelIndex})
	return ProgrammeBoundaryResultForTest{Class: result.observation.class, Evidence: result.evidence}
}

type RawObservationForTest struct {
	Class string
	Media MediaShape
}

func RawBurstForTest(ctx context.Context, endpoint *EndpointForTest, config Config, indexes []int) ([]RawObservationForTest, ResourceSample) {
	observations, sample := rawBurst(ctx, endpoint, config, indexes)
	projected := make([]RawObservationForTest, len(observations))
	for index, observation := range observations {
		projected[index] = RawObservationForTest{Class: observation.class, Media: observation.media}
	}
	return projected, sample
}

type HeldBurstForTest struct{ burst *heldBurst }

func StartHeldBurstForTest(ctx context.Context, endpoint *EndpointForTest, config Config, indexes []int) *HeldBurstForTest {
	return &HeldBurstForTest{burst: startHeldBurst(ctx, endpoint, config, indexes)}
}

func (b *HeldBurstForTest) Results() []RawObservationForTest {
	projected := make([]RawObservationForTest, len(b.burst.results))
	for index, observation := range b.burst.results {
		projected[index] = RawObservationForTest{Class: observation.class, Media: observation.media}
	}
	return projected
}

func (b *HeldBurstForTest) Verify(ctx context.Context) { b.burst.verify(ctx) }
func (b *HeldBurstForTest) Release()                   { b.burst.release() }

func WaitForConvergenceForTest(ctx context.Context, endpoint *EndpointForTest, config Config, baseline ResourceSample) (string, ResourceSample) {
	observation, sample := waitForConvergence(ctx, endpoint, config, baseline, config.CleanupTimeout)
	return observation.class, sample
}

type ChildFaultDrillForTest struct {
	Phase    Phase
	Receipt  string
	Selected string
	Peer     string
	Recovery string
}

func ChildFailureDrillForTest(ctx context.Context, endpoint *EndpointForTest, config Config, indexes []int, capacity int) ChildFaultDrillForTest {
	drill := childFailureDrill(ctx, endpoint, config, indexes, capacity, nil)
	return ChildFaultDrillForTest{Phase: drill.phase, Receipt: drill.receipt, Selected: drill.selected, Peer: drill.peer, Recovery: drill.recovery}
}

func RequireCertifiedPublicationForTest(t *testing.T, report Report) {
	requireCertifiedPublication(t, report)
}
func RequireUncertifiedPublicationForTest(t *testing.T, report Report) {
	requireUncertifiedPublication(t, report)
}

// ColdHLSStreamForTest requires a prepared miss before opening ordinary signed HLS.
func ColdHLSStreamForTest(ctx context.Context, config Config, channel string) (io.ReadCloser, error) {
	e, err := newEndpoint(config.normalized())
	if err != nil {
		return nil, err
	}
	signed, _, class := e.mint(ctx, channel)
	if class != "ok" {
		return nil, fmt.Errorf("mint: %s", class)
	}
	_, hit, class := e.prepared(ctx, signed)
	if hit || class != "prepared_miss" {
		return nil, fmt.Errorf("expected cold cohort: %s", class)
	}
	return newLiveHLSReader(ctx, e, signed), nil
}

func ObserveProgrammeSignalsForTest(ctx context.Context, config Config, laneName string, channelIndex int) (ProgrammeBoundaryResultForTest, error) {
	config = config.normalized()
	e, err := newEndpoint(config)
	if err != nil {
		return ProgrammeBoundaryResultForTest{}, err
	}
	result := observeProgrammeSignals(ctx, e, config, boundaryLane{name: laneName, channelIndex: channelIndex})
	return ProgrammeBoundaryResultForTest{Class: result.observation.class, Evidence: result.evidence}, nil
}
