package app

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

type languageChannels struct {
	channels []store.Channel
	err      error
}

func (l languageChannels) ListChannels(context.Context) ([]store.Channel, error) {
	return l.channels, l.err
}

func languageChannel(status schedule.ChannelStatus, language string) store.Channel {
	ch := store.Channel{Channel: schedule.Channel{Status: status}}
	if language != "" {
		ch.Policy.Filler = &schedule.FillerSelection{Language: language}
	}
	return ch
}

// #2030 decision 13: the intake gate reads the channels live, so adding a channel in a new language
// admits it with no settings change. A detached channel no longer airs and no longer counts; a
// paused one resumes with its language, so it still does.
func TestIntakeLanguages_ReadsTheChannelsLive(t *testing.T) {
	set := visionSet(t, map[string]string{"filler.language": "en"})
	lister := languageChannels{channels: []store.Channel{
		languageChannel(schedule.StatusLive, ""),
		languageChannel(schedule.StatusPaused, "es"),
		languageChannel(schedule.StatusDetached, "ja"),
	}}
	got, err := intakeLanguages(lister, set)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "en,es" {
		t.Fatalf("intake languages = %q, want en,es", got)
	}

	// Detaching the last Spanish channel closes intake to Spanish; with every channel detached the
	// household's language stands alone.
	lister.channels[1].Status = schedule.StatusDetached
	if got, err := intakeLanguages(lister, set)(context.Background()); err != nil || got != "en" {
		t.Fatalf("intake languages = %q, %v; want en once no channel airs Spanish", got, err)
	}
	lister.channels[0].Status = schedule.StatusDetached
	if got, err := intakeLanguages(lister, set)(context.Background()); err != nil || got != "en" {
		t.Fatalf("intake languages = %q, %v; want the household's en with every channel detached", got, err)
	}
}

// A failed read is surfaced, never mistaken for "no language" (which turns the gate off).
func TestIntakeLanguages_AFailedReadIsAnError(t *testing.T) {
	set := visionSet(t, map[string]string{"filler.language": "en"})
	if got, err := intakeLanguages(languageChannels{err: errors.New("db down")}, set)(context.Background()); err == nil {
		t.Fatalf("intake languages = %q with no error, want the read failure", got)
	}
}
