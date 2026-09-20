package app

import "testing"

func TestFillerResearchRunLimitHonorsMasterSourcesAndWebFallback(t *testing.T) {
	for _, tc := range []struct {
		name              string
		enabled           bool
		structuredEnabled bool
		webProvider       string
		pipelineLimit     int
		want              int
	}{
		{name: "master off", structuredEnabled: true, webProvider: "brave", pipelineLimit: 5, want: 0},
		{name: "structured source", enabled: true, structuredEnabled: true, pipelineLimit: 5, want: 1},
		{name: "web only", enabled: true, webProvider: "brave", pipelineLimit: 5, want: 1},
		{name: "nothing selected", enabled: true, pipelineLimit: 5, want: 0},
		{name: "explicit no web", enabled: true, webProvider: " none ", pipelineLimit: 5, want: 0},
		{name: "pipeline paused", enabled: true, structuredEnabled: true, pipelineLimit: 0, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fillerResearchRunLimit(tc.enabled, tc.structuredEnabled, tc.webProvider, tc.pipelineLimit); got != tc.want {
				t.Fatalf("limit = %d, want %d", got, tc.want)
			}
		})
	}
}
