package playoutcert

import (
	"context"
	"time"
)

// phaseSampler has one owner goroutine, so boundary and periodic samples are
// serialized and end cannot freeze a phase with a sample still in flight.
type phaseSampler struct {
	endpoint *endpoint
	ctx      context.Context
	interval time.Duration
	ops      chan samplerOp
	done     chan struct{}
}

type samplerOp struct {
	kind  byte
	name  string
	reply chan PhaseResources
}

type phaseAggregate struct {
	samples, failures int
	maximum           ResourceSample
	firstCPU, lastCPU float64
	cpuReset          bool
}

func newPhaseSampler(ctx context.Context, endpoint *endpoint, interval time.Duration) *phaseSampler {
	if interval <= 0 {
		interval = 25 * time.Millisecond
	}
	s := &phaseSampler{endpoint: endpoint, ctx: ctx, interval: interval, ops: make(chan samplerOp), done: make(chan struct{})}
	go s.run()
	return s
}

func (s *phaseSampler) run() {
	ticker := time.NewTicker(s.interval)
	defer func() { ticker.Stop(); close(s.done) }()
	active := ""
	aggregates := map[string]*phaseAggregate{}
	sample := func(name string) {
		if name == "" {
			return
		}
		agg := aggregates[name]
		if agg == nil {
			agg = &phaseAggregate{}
			aggregates[name] = agg
		}
		if s.ctx.Err() != nil {
			agg.failures++
			return
		}
		value, err := s.endpoint.sample(s.ctx, name+"_sample")
		if err != nil {
			agg.failures++
			return
		}
		if agg.samples == 0 {
			agg.maximum = value
			agg.firstCPU = value.CPUSeconds
		} else {
			agg.maximum = maximumSample(agg.maximum, value)
			if value.CPUSeconds < agg.lastCPU {
				agg.cpuReset = true
				agg.failures++
			}
		}
		agg.lastCPU = value.CPUSeconds
		agg.samples++
	}
	for {
		select {
		case <-ticker.C:
			sample(active)
		case op := <-s.ops:
			switch op.kind {
			case 'b':
				active = op.name
				aggregates[op.name] = &phaseAggregate{}
				sample(op.name)
				close(op.reply)
			case 'e':
				if active == op.name {
					sample(op.name)
					active = ""
				}
				agg := aggregates[op.name]
				result := PhaseResources{IntervalMS: float64(s.interval) / float64(time.Millisecond)}
				if agg != nil {
					result.Samples, result.SampleFailures, result.Maximum = agg.samples, agg.failures, agg.maximum
					if agg.samples > 1 {
						result.CPUSecondsDelta = agg.lastCPU - agg.firstCPU
						if agg.cpuReset || result.CPUSecondsDelta < 0 {
							result.CPUSecondsDelta = 0
							if !agg.cpuReset {
								result.SampleFailures++
							}
						}
					}
					delete(aggregates, op.name)
				}
				op.reply <- result
				close(op.reply)
			case 'c':
				close(op.reply)
				return
			}
		}
	}
}

func (s *phaseSampler) begin(name string) {
	reply := make(chan PhaseResources)
	s.ops <- samplerOp{kind: 'b', name: name, reply: reply}
	<-reply
}

func (s *phaseSampler) end(name string) PhaseResources {
	reply := make(chan PhaseResources, 1)
	s.ops <- samplerOp{kind: 'e', name: name, reply: reply}
	return <-reply
}

func (s *phaseSampler) close() {
	reply := make(chan PhaseResources)
	s.ops <- samplerOp{kind: 'c', reply: reply}
	<-reply
	<-s.done
}

func maximumSample(a, b ResourceSample) ResourceSample {
	if b.RSSBytes > a.RSSBytes {
		a.RSSBytes = b.RSSBytes
	}
	if b.CPUSeconds > a.CPUSeconds {
		a.CPUSeconds = b.CPUSeconds
	}
	if b.OpenFDs > a.OpenFDs {
		a.OpenFDs = b.OpenFDs
	}
	if b.Goroutines > a.Goroutines {
		a.Goroutines = b.Goroutines
	}
	if b.HTTPInFlight > a.HTTPInFlight {
		a.HTTPInFlight = b.HTTPInFlight
	}
	if b.SessionsActive > a.SessionsActive {
		a.SessionsActive = b.SessionsActive
	}
	if b.ViewerActive > a.ViewerActive {
		a.ViewerActive = b.ViewerActive
	}
	if b.GraceIdle > a.GraceIdle {
		a.GraceIdle = b.GraceIdle
	}
	if b.TranscodeCost > a.TranscodeCost {
		a.TranscodeCost = b.TranscodeCost
	}
	if b.FFmpegRunning > a.FFmpegRunning {
		a.FFmpegRunning = b.FFmpegRunning
	}
	if b.Capacity > a.Capacity {
		a.Capacity = b.Capacity
	}
	if b.PreparedChannels > a.PreparedChannels {
		a.PreparedChannels = b.PreparedChannels
	}
	if b.ReadyChannels > a.ReadyChannels {
		a.ReadyChannels = b.ReadyChannels
	}
	if b.ChannelHealth > a.ChannelHealth {
		a.ChannelHealth = b.ChannelHealth
	}
	if b.StalledChannels > a.StalledChannels {
		a.StalledChannels = b.StalledChannels
	}
	if b.GPUVRAMGiB > a.GPUVRAMGiB {
		a.GPUVRAMGiB = b.GPUVRAMGiB
	}
	if b.LLMVRAMGiB > a.LLMVRAMGiB {
		a.LLMVRAMGiB = b.LLMVRAMGiB
	}
	a.GPUContended = a.GPUContended || b.GPUContended
	return a
}
