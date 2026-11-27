package operator

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func capturingLog() (*slog.Logger, *strings.Builder) {
	var logs strings.Builder
	return slog.New(slog.NewTextHandler(&logs, nil)), &logs
}

// Registering a source is best-effort, so a store failure never stops the download, but it is
// reported: an operator who added a collection and finds it missing from the Sources tab can see why.
func TestRememberSourcesLogsAFailedRegistration(t *testing.T) {
	t.Parallel()
	log, logs := capturingLog()
	rs := &recordingSources{err: errors.New("disk full")}
	New(Config{Sources: rs, Log: log, Now: time.Now}).rememberSources(context.Background(),
		[]string{"https://archive.org/details/classic_tv_commercials", "https://archive.org/details/second_item"})

	for _, id := range []string{"classic_tv_commercials", "second_item"} {
		if !strings.Contains(logs.String(), id) {
			t.Errorf("no log line names the source %q whose registration failed:\n%s", id, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "disk full") || !strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("the failure was not logged as a warning with its cause:\n%s", logs.String())
	}
}

// A store failure with no logger wired is still safe.
func TestRememberSourcesWithoutALoggerSurvivesAFailure(t *testing.T) {
	t.Parallel()
	New(Config{Sources: &recordingSources{err: errors.New("disk full")}}).rememberSources(context.Background(),
		[]string{"https://archive.org/details/x"})
}

// fakeRequeuer is the pipeline's Requeue.
type fakeRequeuer struct {
	moved bool
	err   error
	calls []string
}

func (f *fakeRequeuer) Requeue(_ context.Context, hash string) (bool, error) {
	f.calls = append(f.calls, hash)
	return f.moved, f.err
}

// A re-detect writes a fresh proposal, which stays valid whether or not the reel could be un-parked,
// so a failed un-park does not fail the detection. It is logged: the reel then sits at review until
// someone moves it by hand, and nothing else says so.
func TestRequeueAfterDetectLogsAFailureAndStaysQuietOtherwise(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		pipe    *fakeRequeuer
		wantLog string
	}{
		"the reel is un-parked":                {&fakeRequeuer{moved: true}, ""},
		"the reel was never parked":            {&fakeRequeuer{}, ""},
		"the pipeline could not be written to": {&fakeRequeuer{err: errors.New("database is locked")}, "database is locked"},
	} {
		t.Run(name, func(t *testing.T) {
			log, logs := capturingLog()
			New(Config{Log: log}).requeueAfterDetect(context.Background(), tc.pipe, "clip-1")
			if len(tc.pipe.calls) != 1 || tc.pipe.calls[0] != "clip-1" {
				t.Fatalf("Requeue calls = %v, want one for clip-1", tc.pipe.calls)
			}
			if tc.wantLog == "" && logs.Len() != 0 {
				t.Errorf("a clean requeue logged:\n%s", logs.String())
			}
			if tc.wantLog != "" && (!strings.Contains(logs.String(), tc.wantLog) || !strings.Contains(logs.String(), "clip-1")) {
				t.Errorf("a failed requeue did not log its cause and clip:\n%s", logs.String())
			}
		})
	}
}
