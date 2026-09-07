package playoutcert

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestConfigInputBoundsRejectBeforeRunObservation(t *testing.T) {
	valid := func() Config {
		return Config{BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin", DeviceToken: "device", Channels: []Channel{{ID: "channel"}}}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "negative concurrency", mutate: func(c *Config) { c.Concurrency = -1 }},
		{name: "concurrency above ceiling", mutate: func(c *Config) { c.Concurrency = 65 }},
		{name: "surf rounds above ceiling", mutate: func(c *Config) { c.SurfRounds = 101 }},
		{name: "fan in above ceiling", mutate: func(c *Config) { c.FanInViewers = 65 }},
		{name: "raw capture below packet", mutate: func(c *Config) { c.RawCaptureBytes = 187 }},
		{name: "warm grace above ceiling", mutate: func(c *Config) { c.WarmGrace = time.Minute + time.Nanosecond }},
		{name: "request timeout above ceiling", mutate: func(c *Config) { c.RequestTimeout = 30*time.Minute + time.Nanosecond }},
		{name: "cleanup poll below floor", mutate: func(c *Config) { c.CleanupPoll = time.Nanosecond }},
		{name: "prepared HLS threshold above ceiling", mutate: func(c *Config) { c.PreparedP95 = 100*time.Millisecond + time.Nanosecond }},
		{name: "prepared raw threshold above ceiling", mutate: func(c *Config) { c.PreparedRawP95 = 500*time.Millisecond + time.Nanosecond }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := valid()
			tc.mutate(&config)
			var requests atomic.Int32
			config.Client = &http.Client{Transport: httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return nil, errors.New("request must not be issued")
			})}
			if _, err := Run(context.Background(), config); err == nil {
				t.Fatal("Run accepted invalid input")
			}
			if requests.Load() != 0 {
				t.Fatalf("requests = %d, want 0", requests.Load())
			}
		})
	}
}

func TestConfigInputBoundsAllowDefaultsAndEdges(t *testing.T) {
	config := Config{BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin", DeviceToken: "device", Channels: []Channel{{ID: "channel"}}}
	if err := config.Validate(); err != nil {
		t.Fatalf("zero defaults rejected: %v", err)
	}
	normalized := config.normalized()
	if normalized.Concurrency != 12 || normalized.SurfRounds != 1 || normalized.FanInViewers != 4 || normalized.RawCaptureBytes != 2<<20 {
		t.Fatalf("defaults = %+v", normalized)
	}
	config.Concurrency, config.SurfRounds, config.FanInViewers = 64, 100, 64
	config.RawCaptureBytes, config.WarmGrace = 188, time.Minute
	config.RequestTimeout, config.CleanupTimeout, config.CleanupPoll = 30*time.Minute, 30*time.Minute, time.Millisecond
	config.PreparedP95, config.PreparedRawP95 = 100*time.Millisecond, 500*time.Millisecond
	if err := config.Validate(); err != nil {
		t.Fatalf("valid bounds rejected: %v", err)
	}
}
