package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/programmer"
)

func TestResolvedTunarrConfigReadsLiveSettingsTogether(t *testing.T) {
	for _, env := range []string{
		"TUNARR_URL",
		"TUNARR_TRANSCODE_CONFIG_ID",
		"FILLER_WEIGHT",
		"FILLER_COOLDOWN_SECONDS",
	} {
		t.Setenv(env, "")
	}
	set := visionSet(t, map[string]string{
		"tunarr.url":                 "http://old-tunarr:8000",
		"tunarr.transcode_config_id": "old-transcode",
		"filler.weight":              "3",
		"filler.cooldown_seconds":    "90",
	})
	config := set.tunarrConfig()

	first := config()
	if first.BaseURL != "http://old-tunarr:8000" || first.TranscodeConfigID != "old-transcode" ||
		first.FillerWeight != 3 || first.FillerCooldownSeconds != 90 {
		t.Fatalf("initial config = %+v", first)
	}

	set.svc.SetDB(map[string]string{
		"tunarr.url":                 "http://new-tunarr:8000",
		"tunarr.transcode_config_id": "new-transcode",
		"filler.weight":              "7",
		"filler.cooldown_seconds":    "240",
	})
	second := config()
	if second.BaseURL != "http://new-tunarr:8000" || second.TranscodeConfigID != "new-transcode" ||
		second.FillerWeight != 7 || second.FillerCooldownSeconds != 240 {
		t.Fatalf("hot-applied config = %+v", second)
	}

	if first.BaseURL != "http://old-tunarr:8000" || first.FillerWeight != 3 {
		t.Fatalf("first operation's snapshot changed after hot-apply: %+v", first)
	}

	oldValues := map[string]string{
		"tunarr.url":                 "http://old-tunarr:8000",
		"tunarr.transcode_config_id": "old-transcode",
		"filler.weight":              "3",
		"filler.cooldown_seconds":    "90",
	}
	newValues := map[string]string{
		"tunarr.url":                 "http://new-tunarr:8000",
		"tunarr.transcode_config_id": "new-transcode",
		"filler.weight":              "7",
		"filler.cooldown_seconds":    "240",
	}
	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-stop:
				return
			default:
				set.svc.SetDB(oldValues)
				set.svc.SetDB(newValues)
			}
		}
	}()

	var mixed *programmer.Config
	for range 50_000 {
		snapshot := config()
		oldSnapshot := snapshot.BaseURL == "http://old-tunarr:8000" &&
			snapshot.TranscodeConfigID == "old-transcode" && snapshot.FillerWeight == 3 &&
			snapshot.FillerCooldownSeconds == 90
		newSnapshot := snapshot.BaseURL == "http://new-tunarr:8000" &&
			snapshot.TranscodeConfigID == "new-transcode" && snapshot.FillerWeight == 7 &&
			snapshot.FillerCooldownSeconds == 240
		if !oldSnapshot && !newSnapshot {
			mixed = &snapshot
			break
		}
	}
	close(stop)
	<-writerDone
	if mixed != nil {
		t.Fatalf("config mixed values across a concurrent hot-apply: %+v", *mixed)
	}
}
