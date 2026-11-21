package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// composeFile is the part of a Compose file this test reads.
type composeFile struct {
	Services map[string]struct {
		Image           string `yaml:"image"`
		Build           any    `yaml:"build"`
		StopGracePeriod string `yaml:"stop_grace_period"`
	} `yaml:"services"`
}

// A container is SIGKILLed when its stop_grace_period runs out, which would cut the generation's
// teardown (and the store close that ends it) mid-write. Every Compose file that defines the app
// container sets a period that covers the whole teardown, and one that sets it never sets less.
func TestComposeStopGracePeriodCoversTheTeardown(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "docker", "compose*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no Compose files found: %v", err)
	}
	defining := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var compose composeFile
		if err := yaml.Unmarshal(raw, &compose); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		app, ok := compose.Services["loomarr"]
		if !ok {
			continue
		}
		name := filepath.Base(path)
		// An overlay merges onto a file that defines the container, so only a file that names the
		// image or the build must carry the period itself.
		standalone := app.Image != "" || app.Build != nil
		if app.StopGracePeriod == "" {
			if standalone {
				defining++
				t.Errorf("%s defines the loomarr container without stop_grace_period; Docker's 10s default is shorter than the %v teardown", name, teardownBudget)
			}
			continue
		}
		if standalone {
			defining++
		}
		grace, err := time.ParseDuration(app.StopGracePeriod)
		if err != nil {
			t.Errorf("%s: stop_grace_period %q: %v", name, app.StopGracePeriod, err)
			continue
		}
		if grace < teardownBudget {
			t.Errorf("%s: stop_grace_period %v is shorter than the %v the generation's teardown can take", name, grace, teardownBudget)
		}
	}
	if defining == 0 {
		t.Error("no Compose file defines the loomarr container, so the check read nothing")
	}
}
