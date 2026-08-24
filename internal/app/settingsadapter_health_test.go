package app

import "testing"

func TestIsHealthSetting(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"library.url", "seerr.api_key", "sonarr.url", "radarr.url", "requester.provider",
		"tunarr.url", "llm.model", "tmdb.api_key", "job.system_health.schedule",
	} {
		if !isHealthSetting(key) {
			t.Errorf("isHealthSetting(%q) = false", key)
		}
	}
	for _, key := range []string{"filler.dir", "job.reconcile.schedule", "session.ttl"} {
		if isHealthSetting(key) {
			t.Errorf("isHealthSetting(%q) = true", key)
		}
	}
}
