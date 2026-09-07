package playoutcert

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestFaultProfileSelectionFailsClosed(t *testing.T) {
	valid := func() Config {
		return Config{
			BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin", DeviceToken: "device",
			Channels: fixtureChannels(100), Certify: true,
			FaultProfiles: []FaultProfile{FaultChildFailure}, FaultController: playoutcertfixture.ScopedFaultController{ScopeName: "run-a"},
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"unknown", func(c *Config) { c.FaultProfiles = []FaultProfile{"other"} }, "unknown"},
		{"duplicate", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultChildFailure, FaultChildFailure} }, "duplicate"},
		{"conflicting terminal faults", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultParentFailure, FaultShutdown} }, "shutdown must be selected alone"},
		{"shutdown missing acknowledgement", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultShutdown} }, "disposable"},
		{"shutdown wrong target", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultShutdown}; c.DisposableTarget = "run-b" }, "match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := valid()
			tc.mutate(&config)
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}
