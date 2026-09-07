package httpfixture

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// PlayoutSamplerTransport is a no-network fixture for the resource endpoints
// sampled by playout certification. Each metric request advances its sample.
type PlayoutSamplerTransport struct {
	mu                 sync.Mutex
	values             []float64
	cpuValues          []float64
	metricCalls, reads int
	BlockMetricCall    int // one-based; zero does not block
	Started, Release   chan struct{}
	BlockError         error
	OmitGoroutines     bool
}

func NewPlayoutSamplerTransport(values ...float64) *PlayoutSamplerTransport {
	return &PlayoutSamplerTransport{values: append([]float64(nil), values...), Started: make(chan struct{}), Release: make(chan struct{})}
}

// SetCPUValues configures the independent process CPU counter before sampling starts.
func (s *PlayoutSamplerTransport) SetCPUValues(values ...float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metricCalls != 0 {
		panic("httpfixture: CPU values must be configured before first use")
	}
	s.cpuValues = append([]float64(nil), values...)
}

func (s *PlayoutSamplerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.reads++
	path := req.URL.Path
	if path == "/metrics" {
		i := s.metricCalls
		s.metricCalls++
		block := s.BlockMetricCall == i+1
		if block {
			s.BlockMetricCall = 0
			close(s.Started)
		}
		value := s.value(i)
		cpu := s.cpuValue(i)
		omit := s.OmitGoroutines
		release, blockErr := s.Release, s.BlockError
		s.mu.Unlock()
		if block {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-release:
				if blockErr != nil {
					return nil, blockErr
				}
			}
		}
		body := metricsBody(value, cpu)
		if omit {
			body = strings.Replace(body, fmt.Sprintf("go_goroutines %g\n", value), "", 1)
		}
		return playoutSamplerResponse(req, body), nil
	}
	value := s.value(s.metricCalls - 1)
	s.mu.Unlock()
	n := int(value)
	switch path {
	case "/v1/playout/sessions":
		return playoutSamplerResponse(req, fmt.Sprintf(`{"running":true,"capacity":%d,"active":%d,"viewerActiveSessions":%d,"graceIdleSessions":%d,"transcodeCost":%d}`, n, n, n, n, n)), nil
	case "/v1/diagnostics/processes":
		return playoutSamplerResponse(req, `{"items":[`+strings.TrimSuffix(strings.Repeat(`{"executable":"ffmpeg","status":"running"},`, n), ",")+`]}`), nil
	case "/v1/playout/status":
		return playoutSamplerResponse(req, fmt.Sprintf(`{"running":true,"gpu":{"vramGiB":%g,"llmVramGiB":%g,"contended":true},"channels":[%s],"prepared":{"channels":%d,"readyChannels":%d}}`, value, value, strings.TrimSuffix(strings.Repeat(`{"health":"stalled"},`, n), ","), n, n)), nil
	default:
		return nil, errors.New("httpfixture: unexpected playout sampler path " + path)
	}
}

func (s *PlayoutSamplerTransport) value(index int) float64 {
	if len(s.values) == 0 {
		return 0
	}
	if index < 0 {
		index = 0
	}
	if index >= len(s.values) {
		index = len(s.values) - 1
	}
	return s.values[index]
}

func (s *PlayoutSamplerTransport) cpuValue(index int) float64 {
	if len(s.cpuValues) == 0 {
		return float64(index + 1)
	}
	if index >= len(s.cpuValues) {
		index = len(s.cpuValues) - 1
	}
	return s.cpuValues[index]
}

func (s *PlayoutSamplerTransport) MetricCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metricCalls
}
func (s *PlayoutSamplerTransport) Reads() int { s.mu.Lock(); defer s.mu.Unlock(); return s.reads }

func metricsBody(v, cpu float64) string {
	return fmt.Sprintf("process_resident_memory_bytes %g\nprocess_cpu_seconds_total %g\nprocess_open_fds %g\ngo_goroutines %g\nloomarr_http_requests_in_flight %g\nloomarr_playout_sessions_active %g\n", v, cpu, v, v, v, v)
}
func playoutSamplerResponse(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}
}

var _ http.RoundTripper = (*PlayoutSamplerTransport)(nil)
