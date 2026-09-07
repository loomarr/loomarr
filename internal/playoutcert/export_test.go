package playoutcert

import (
	"context"
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
	result := observeProgrammeBoundary(ctx, endpoint, config, boundaryLane{name: laneName, channelIndex: channelIndex})
	return ProgrammeBoundaryResultForTest{Class: result.observation.class, Evidence: result.evidence}
}

type RawObservationForTest struct{ Class string }

func RawBurstForTest(ctx context.Context, endpoint *EndpointForTest, config Config, indexes []int) ([]RawObservationForTest, ResourceSample) {
	observations, sample := rawBurst(ctx, endpoint, config, indexes)
	projected := make([]RawObservationForTest, len(observations))
	for index, observation := range observations {
		projected[index] = RawObservationForTest{Class: observation.class}
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
		projected[index] = RawObservationForTest{Class: observation.class}
	}
	return projected
}

func (b *HeldBurstForTest) Verify(ctx context.Context) { b.burst.verify(ctx) }
func (b *HeldBurstForTest) Release()                   { b.burst.release() }

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
