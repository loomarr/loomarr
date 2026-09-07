package playoutcertfixture

import (
	"context"
	"time"
)

// ScopedFaultController is a cycle-free test double for interfaces whose only
// contract is a stable, explicitly named disposable-target scope.
type ScopedFaultController struct {
	ScopeName string
}

func (c ScopedFaultController) Scope() string { return c.ScopeName }

// ParentFaultTarget supplies the neutral mechanics for a public-stream parent
// fault test. Package users map their own request and receipt types at their
// boundary, avoiding an import cycle with the certification package.
type ParentFaultTarget struct {
	Fixture             *Fixture
	Peer                string
	WaitForExpiry       bool
	RetainAfterRecovery bool
}

// ShutdownTarget supplies the neutral mechanics for a shutdown fault test.
// Certification packages adapt their own request, receipt, and resource types
// at the boundary, keeping this shared fixture free of domain imports.
type ShutdownTarget struct {
	Fixture       *Fixture
	Channels      []string
	WaitForExpiry bool
	SampleErr     error
	Sample        *StoppedResource
	Samples       []StoppedResource
	SampleIndex   *int
}

// StoppedResource is the fixture's measured shutdown state. It intentionally
// has no certification-package dependency.
type StoppedResource struct {
	Point                                                                          string
	RSSBytes, CPUSeconds, OpenFDs, Goroutines, HTTPInFlight                        float64
	SessionsActive, ViewerActive, GraceIdle, TranscodeCost, Capacity               int
	FFmpegRunning, PreparedChannels, ReadyChannels, ChannelHealth, StalledChannels int
}

func (c ShutdownTarget) Stop(ctx context.Context) error {
	if c.WaitForExpiry {
		<-ctx.Done()
	}
	for _, channel := range c.Channels {
		c.Fixture.FailSession(channel)
	}
	return nil
}

func (c ShutdownTarget) SampleStopped(_ context.Context, point string) (StoppedResource, error) {
	if c.SampleErr != nil {
		return StoppedResource{}, c.SampleErr
	}
	if c.Sample != nil {
		return *c.Sample, nil
	}
	if len(c.Samples) > 0 {
		index := 0
		if c.SampleIndex != nil {
			index = *c.SampleIndex
			if index < len(c.Samples)-1 {
				*c.SampleIndex++
			}
		}
		return c.Samples[index], nil
	}
	c.Fixture.mu.Lock()
	defer c.Fixture.mu.Unlock()
	c.Fixture.expireLocked(time.Now())
	active, viewers, grace := len(c.Fixture.sessions), 0, 0
	for _, session := range c.Fixture.sessions {
		if session.viewers > 0 {
			viewers++
		} else {
			grace++
		}
	}
	return StoppedResource{
		Point: point, RSSBytes: 104857600, CPUSeconds: 2, OpenFDs: 12, Goroutines: 18, HTTPInFlight: 1,
		SessionsActive: active, ViewerActive: viewers, GraceIdle: grace, TranscodeCost: min(active, 4),
		Capacity: 4, FFmpegRunning: c.Fixture.activeRaw, PreparedChannels: 100, ReadyChannels: 100, ChannelHealth: 100,
	}, nil
}

func (c ParentFaultTarget) Current(ctx context.Context) (uint64, error) {
	return 1, ctx.Err()
}

func (c ParentFaultTarget) Fail(ctx context.Context, channelID string) error {
	_ = c.Fixture.FailSession(channelID)
	_ = c.Fixture.ContinueSession(c.Peer)
	if c.RetainAfterRecovery {
		c.Fixture.RetainAfterRecovery(channelID)
	}
	if c.WaitForExpiry {
		<-ctx.Done()
	}
	return nil
}

// CleanupFailureTarget is a bounded isolated-target port for command cleanup
// tests. It makes no process or network calls.
type CleanupFailureTarget struct {
	Err error
}

func (c CleanupFailureTarget) Close(context.Context) error { return c.Err }
