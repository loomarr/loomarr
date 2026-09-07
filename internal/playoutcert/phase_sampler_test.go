package playoutcert

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func newSamplerForTest(t *testing.T, ctx context.Context, tr *httpfixture.PlayoutSamplerTransport, interval time.Duration) *phaseSampler {
	t.Helper()
	base, err := url.Parse("http://sampler.test")
	if err != nil {
		t.Fatal(err)
	}
	return newPhaseSampler(ctx, &endpoint{base: base, client: &http.Client{Transport: tr}, timeout: time.Second}, interval)
}

func TestPhaseSamplerRetainsTransientPeakAcrossAllResourceFields(t *testing.T) {
	tr := httpfixture.NewPlayoutSamplerTransport(1, 99, 2)
	tr.SetCPUValues(10, 11, 12)
	s := newSamplerForTest(t, context.Background(), tr, time.Millisecond)
	defer s.close()
	s.begin("phase")
	for deadline := time.Now().Add(time.Second); tr.MetricCalls() < 2; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("periodic peak sample did not occur")
		}
	}
	r := s.end("phase")
	m := r.Maximum
	if r.Samples < 3 || r.SampleFailures != 0 || r.CPUSecondsDelta != 2 || m.RSSBytes != 99 || m.CPUSeconds != 12 || m.OpenFDs != 99 || m.Goroutines != 99 || m.HTTPInFlight != 99 || m.SessionsActive != 99 || m.ViewerActive != 99 || m.GraceIdle != 99 || m.TranscodeCost != 99 || m.Capacity != 99 || m.FFmpegRunning != 99 || m.PreparedChannels != 99 || m.ReadyChannels != 99 || m.ChannelHealth != 99 || m.StalledChannels != 99 || m.GPUVRAMGiB != 99 || m.LLMVRAMGiB != 99 || !m.GPUContended {
		t.Fatalf("peak was not retained: %#v", r)
	}
}

func TestPhaseSamplerEndOwnsBlockedPeriodicFailure(t *testing.T) {
	tr := httpfixture.NewPlayoutSamplerTransport(1)
	tr.BlockMetricCall, tr.BlockError = 2, errors.New("controlled sample failure")
	s := newSamplerForTest(t, context.Background(), tr, time.Millisecond)
	defer s.close()
	s.begin("A")
	select {
	case <-tr.Started:
	case <-time.After(time.Second):
		t.Fatal("periodic sample did not start")
	}
	ended := make(chan PhaseResources, 1)
	go func() { ended <- s.end("A") }()
	select {
	case <-ended:
		t.Fatal("end did not wait for periodic sample")
	case <-time.After(10 * time.Millisecond):
	}
	close(tr.Release)
	if r := <-ended; r.SampleFailures != 1 {
		t.Fatalf("A failure lost: %#v", r)
	}
	s.begin("B")
	if r := s.end("B"); r.SampleFailures != 0 {
		t.Fatalf("B inherited A failure: %#v", r)
	}
}

func TestPhaseSamplerCancellationJoinsWithoutFurtherReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := httpfixture.NewPlayoutSamplerTransport(1)
	tr.BlockMetricCall = 2
	s := newSamplerForTest(t, ctx, tr, time.Millisecond)
	var closeOnce sync.Once
	closeSampler := func() { closeOnce.Do(s.close) }
	defer closeSampler()
	s.begin("A")
	select {
	case <-tr.Started:
	case <-time.After(time.Second):
		t.Fatal("periodic sample did not start")
	}
	cancel()
	done := make(chan struct{})
	go func() { closeSampler(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close did not join cancelled sampler")
	}
	reads := tr.Reads()
	time.Sleep(5 * time.Millisecond)
	if tr.Reads() != reads {
		t.Fatalf("reads continued after close: %d then %d", reads, tr.Reads())
	}
}

func TestPhaseSamplerCPUCounterResetFailsIncludingMidPhaseReset(t *testing.T) {
	tr := httpfixture.NewPlayoutSamplerTransport(5, 1, 6)
	tr.SetCPUValues(5, 1, 6)
	s := newSamplerForTest(t, context.Background(), tr, time.Millisecond)
	defer s.close()
	s.begin("phase")
	for deadline := time.Now().Add(time.Second); tr.MetricCalls() < 3; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("did not collect reset and recovery samples")
		}
	}
	r := s.end("phase")
	if r.SampleFailures != 1 || r.CPUSecondsDelta != 0 {
		t.Fatalf("CPU reset was accepted: %#v", r)
	}
}

func TestPhaseSamplerMissingRequiredMetricFails(t *testing.T) {
	tr := httpfixture.NewPlayoutSamplerTransport(1)
	tr.OmitGoroutines = true
	s := newSamplerForTest(t, context.Background(), tr, time.Hour)
	s.begin("phase")
	r := s.end("phase")
	s.close()
	if r.Samples != 0 || r.SampleFailures != 2 {
		t.Fatalf("missing metric produced a successful sample: %#v", r)
	}
}

func TestPhaseSamplerSerialBeginEnd(t *testing.T) {
	tr := httpfixture.NewPlayoutSamplerTransport(1)
	s := newSamplerForTest(t, context.Background(), tr, time.Hour)
	defer s.close()
	for i := 0; i < 100; i++ {
		s.begin("phase-" + strconv.Itoa(i))
		if r := s.end("phase-" + strconv.Itoa(i)); r.Samples != 2 || r.SampleFailures != 0 {
			t.Fatalf("run %d: %#v", i, r)
		}
	}
}
