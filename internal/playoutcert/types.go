// Package playoutcert drives Loomarr's public playout transports through a bounded,
// credential-redacted production-path certification run.
package playoutcert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const SchemaVersion = 1

type Channel struct {
	ID    string   `json:"id"`
	Roles []string `json:"roles,omitempty"`
}

type MediaShape struct {
	VideoStreams int    `json:"videoStreams"`
	AudioStreams int    `json:"audioStreams"`
	VideoCodec   string `json:"videoCodec,omitempty"`
	AudioCodec   string `json:"audioCodec,omitempty"`
}

type Validator interface {
	Validate(context.Context, []byte) (MediaShape, error)
}

type Decoder interface {
	// Decode owns input until it returns. It reports the cumulative number of
	// decoded video frames while preserving one decoder lifecycle for the whole
	// admitted response body.
	Decode(context.Context, io.ReadCloser, func(int64)) error
}

// ProgrammeBoundaryWitness binds a certification observation to the actual
// finite-block source feeding one admitted parent stream. Production targets do
// not currently expose this causal seam; the isolated synthetic target does.
type ProgrammeBoundaryWitness interface {
	Subscribe(channelID string) (ProgrammeBoundarySubscription, error)
}

type ProgrammeBoundarySubscription interface {
	WaitInitial(context.Context) error
	WaitTransition(context.Context) error
	Close()
}

type Config struct {
	BaseURL                          string
	AdminBearer                      string
	DeviceToken                      string
	Channels                         []Channel
	Certify                          bool
	RemoteAcknowledged               bool
	Concurrency                      int
	SurfRounds                       int
	FanInViewers                     int
	RequestTimeout                   time.Duration
	CleanupTimeout                   time.Duration
	CleanupPoll                      time.Duration
	WarmGrace                        time.Duration
	RawCaptureBytes                  int
	PreparedP95                      time.Duration
	PreparedRawP95                   time.Duration
	ProgrammeBoundaryTimeout         time.Duration
	ProgrammeBoundaryLateObservation time.Duration
	ProgrammeBoundaryWitness         ProgrammeBoundaryWitness
	Client                           *http.Client
	Validator                        Validator
	Decoder                          Decoder
	Now                              func() time.Time
}

func (c Config) Validate() error {
	boundaryTimeout := c.ProgrammeBoundaryTimeout
	if boundaryTimeout == 0 {
		boundaryTimeout = 20 * time.Minute
	}
	lateObservation := c.ProgrammeBoundaryLateObservation
	if lateObservation == 0 {
		lateObservation = 3 * time.Second
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("base URL must be an absolute HTTP origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("base URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("base URL must not contain credentials, query, or fragment")
	}
	if !c.RemoteAcknowledged && !isLoopbackHost(parsed.Hostname()) {
		return errors.New("remote origin requires explicit remote acknowledgement")
	}
	if strings.TrimSpace(c.AdminBearer) == "" {
		return errors.New("administrator bearer is required")
	}
	if strings.TrimSpace(c.DeviceToken) == "" {
		return errors.New("device playout token is required")
	}
	if boundaryTimeout < 2*time.Second || boundaryTimeout > 25*time.Minute {
		return errors.New("programme boundary timeout must be within 2s..25m")
	}
	if lateObservation < 250*time.Millisecond || lateObservation > 30*time.Second || lateObservation >= boundaryTimeout {
		return errors.New("programme boundary late observation must be within 250ms..30s and shorter than the boundary timeout")
	}
	if c.Certify && len(c.Channels) < 100 {
		return fmt.Errorf("certification requires at least 100 configured Channels, got %d", len(c.Channels))
	}
	if len(c.Channels) == 0 || len(c.Channels) > 1000 {
		return fmt.Errorf("channel count must be within 1..1000")
	}
	seen := make(map[string]struct{}, len(c.Channels))
	for _, channel := range c.Channels {
		id := strings.TrimSpace(channel.ID)
		if id == "" || len(id) > 256 || strings.ContainsAny(id, "/?#") {
			return errors.New("channel manifest contains an invalid id")
		}
		if _, ok := seen[id]; ok {
			return errors.New("channel manifest contains a duplicate id")
		}
		seen[id] = struct{}{}
		for _, role := range channel.Roles {
			if !slices.Contains(knownRoles, role) {
				return fmt.Errorf("channel manifest contains unknown role %q", role)
			}
		}
	}
	return nil
}

func preparedChannelIndexes(channels []Channel) []int {
	marked := []int{}
	for index, channel := range channels {
		if slices.Contains(channel.Roles, "prepared") {
			marked = append(marked, index)
		}
	}
	if len(marked) > 0 {
		return marked
	}
	indexes := make([]int, len(channels))
	for index := range channels {
		indexes[index] = index
	}
	return indexes
}

// strictTranscodeChannelIndexes deliberately has no fallback: capacity and
// overload evidence is meaningless when the manifest did not provide the lane.
func strictTranscodeChannelIndexes(channels []Channel) []int {
	indexes := []int{}
	for index, channel := range channels {
		if slices.Contains(channel.Roles, "transcode_h264") || slices.Contains(channel.Roles, "transcode_hevc") {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

var knownRoles = []string{
	"prepared", "copy", "transcode_h264", "transcode_hevc",
	"audio_aac", "audio_ac3", "audio_eac3", "expected_failure", "remote_input",
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) normalized() Config {
	if c.Concurrency <= 0 {
		c.Concurrency = 12
	}
	if c.Concurrency > 64 {
		c.Concurrency = 64
	}
	if c.SurfRounds <= 0 {
		c.SurfRounds = 1
	}
	if c.FanInViewers <= 0 {
		c.FanInViewers = 4
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 15 * time.Second
	}
	if c.CleanupTimeout <= 0 {
		c.CleanupTimeout = 45 * time.Second
	}
	if c.CleanupPoll <= 0 {
		c.CleanupPoll = 250 * time.Millisecond
	}
	if c.WarmGrace <= 0 {
		c.WarmGrace = 30 * time.Second
	}
	if c.RawCaptureBytes <= 0 {
		c.RawCaptureBytes = 2 << 20
	}
	if c.RawCaptureBytes < 188 {
		c.RawCaptureBytes = 188
	}
	if c.RawCaptureBytes > 16<<20 {
		c.RawCaptureBytes = 16 << 20
	}
	if c.PreparedP95 <= 0 {
		c.PreparedP95 = 100 * time.Millisecond
	}
	if c.PreparedRawP95 <= 0 {
		c.PreparedRawP95 = 500 * time.Millisecond
	}
	if c.ProgrammeBoundaryTimeout == 0 {
		c.ProgrammeBoundaryTimeout = 20 * time.Minute
	}
	if c.ProgrammeBoundaryLateObservation == 0 {
		c.ProgrammeBoundaryLateObservation = 3 * time.Second
	}
	if c.Client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 128
		transport.MaxIdleConnsPerHost = 64
		c.Client = &http.Client{Transport: transport}
	}
	if c.Validator == nil {
		c.Validator = FFprobeValidator{}
	}
	if c.Decoder == nil {
		c.Decoder = FFmpegDecoder{}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

type Target struct {
	Version            string `json:"version"`
	Revision           string `json:"revision"`
	ManifestSHA256     string `json:"manifestSha256"`
	ConfiguredChannels int    `json:"configuredChannels"`
	Capacity           int    `json:"capacity"`
}

type LatencySummary struct {
	Attempts  int     `json:"attempts"`
	Successes int     `json:"successes"`
	Failures  int     `json:"failures"`
	P50MS     float64 `json:"p50Ms"`
	P95MS     float64 `json:"p95Ms"`
	P99MS     float64 `json:"p99Ms"`
}

type Phase struct {
	Name string `json:"name"`
	LatencySummary
	FirstByte           LatencySummary                 `json:"firstByte"`
	PreparedHits        int                            `json:"preparedHits,omitempty"`
	HTTPClasses         map[string]int                 `json:"httpClasses,omitempty"`
	Media               []MediaShape                   `json:"media,omitempty"`
	HeldContinuity      []HeldContinuityObservation    `json:"heldContinuity,omitempty"`
	ProgrammeBoundaries []ProgrammeBoundaryObservation `json:"programmeBoundaries,omitempty"`
	Resources           PhaseResources                 `json:"resources"`
}

// ProgrammeBoundaryObservation contains only bounded, run-local evidence. The
// scheduler identities used to prove the transition are deliberately omitted.
type ProgrammeBoundaryObservation struct {
	Lane              string     `json:"lane"`
	Outcome           string     `json:"outcome"`
	Transitions       int        `json:"transitions"`
	ObservationMS     float64    `json:"observationMs"`
	DecodedFrameDelta int64      `json:"decodedFrameDelta"`
	ReadDelta         int        `json:"readDelta"`
	BytesDelta        int        `json:"bytesDelta"`
	Media             MediaShape `json:"media"`
}

// HeldContinuityObservation is bounded viewer-side evidence collected after
// overload admission. It makes no producer-liveness claim behind buffers.
type HeldContinuityObservation struct {
	Outcome        string     `json:"outcome"`
	ObservationMS  float64    `json:"observationMs"`
	AdvancingReads int        `json:"advancingReads"`
	BytesObserved  int        `json:"bytesObserved"`
	DecodedFrame   bool       `json:"decodedFrame"`
	Media          MediaShape `json:"media"`
}

// PhaseResources records bounded periodic observations. Maxima are maxima of
// these samples, not a claim of continuous hardware telemetry.
type PhaseResources struct {
	Samples         int            `json:"samples"`
	SampleFailures  int            `json:"sampleFailures"`
	IntervalMS      float64        `json:"intervalMs"`
	CPUSecondsDelta float64        `json:"cpuSecondsDelta"`
	Maximum         ResourceSample `json:"maximum"`
}

type ResourceSample struct {
	Point            string  `json:"point"`
	RSSBytes         float64 `json:"rssBytes"`
	CPUSeconds       float64 `json:"cpuSeconds"`
	OpenFDs          float64 `json:"openFds"`
	Goroutines       float64 `json:"goroutines"`
	HTTPInFlight     float64 `json:"httpInFlight"`
	SessionsActive   int     `json:"sessionsActive"`
	ViewerActive     int     `json:"viewerActive"`
	GraceIdle        int     `json:"graceIdle"`
	TranscodeCost    int     `json:"transcodeCost"`
	Capacity         int     `json:"capacity"`
	FFmpegRunning    int     `json:"ffmpegRunning"`
	PreparedChannels int     `json:"preparedChannels"`
	ReadyChannels    int     `json:"readyChannels"`
	ChannelHealth    int     `json:"channelHealth"`
	StalledChannels  int     `json:"stalledChannels"`
	GPUVRAMGiB       float64 `json:"gpuVramGiB"`
	LLMVRAMGiB       float64 `json:"llmVramGiB"`
	GPUContended     bool    `json:"gpuContended"`
}

type Report struct {
	SchemaVersion int              `json:"schemaVersion"`
	StartedAt     time.Time        `json:"startedAt"`
	CompletedAt   time.Time        `json:"completedAt"`
	Target        Target           `json:"target"`
	Phases        []Phase          `json:"phases"`
	Resources     []ResourceSample `json:"resources"`
	Failures      []string         `json:"failures"`
	Certified     bool             `json:"certified"`
}

func (r Report) Phase(name string) (Phase, bool) {
	for _, phase := range r.Phases {
		if phase.Name == name {
			return phase, true
		}
	}
	return Phase{}, false
}

func (r Report) PhaseMust(name string) Phase {
	phase, _ := r.Phase(name)
	return phase
}

func manifestDigest(channels []Channel) string {
	h := sha256.New()
	for _, channel := range channels {
		_, _ = h.Write([]byte(channel.ID))
		_, _ = h.Write([]byte{0})
		for _, role := range channel.Roles {
			_, _ = h.Write([]byte(role))
			_, _ = h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
