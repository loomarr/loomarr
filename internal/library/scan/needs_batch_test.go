package scan

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// cutOffAfterCommit saves a batch, then drops the caller's request, and fails the first title's
// rewrite.
type cutOffAfterCommit struct {
	librarystore.LibraryNeedsStore
	cancel    context.CancelFunc
	rewritten []string
}

func (s *cutOffAfterCommit) IgnoreLibraryNeeds(context.Context, []string, time.Time) ([]librarystore.LibraryAnswered, error) {
	s.cancel()
	return []librarystore.LibraryAnswered{{ID: "first"}, {ID: "second"}}, nil
}

func (s *cutOffAfterCommit) RestoreLibraryIgnored(ctx context.Context, ids []string) ([]librarystore.LibraryAnswered, error) {
	return s.IgnoreLibraryNeeds(ctx, ids, time.Time{})
}

func (s *cutOffAfterCommit) RewriteLibraryItemFiles(ctx context.Context, id string) ([]librarystore.LibraryFilePath, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return nil, errors.New("the rewrite has no deadline")
	}
	s.rewritten = append(s.rewritten, id)
	if id == "first" {
		return nil, errors.New("disk is busy")
	}
	return nil, nil
}

// Once a batch is saved it has happened: a dropped request or a failed rewrite is logged, not
// returned, so the caller keeps the ids it needs to undo it.
func TestBatchAnswersSucceedOnceSaved(t *testing.T) {
	for name, run := range map[string]func(*Library, context.Context, []string) error{
		"ignore":  (*Library).IgnoreAll,
		"restore": (*Library).RestoreAll,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			st := &cutOffAfterCommit{cancel: cancel}
			var logged bytes.Buffer
			lib := &Library{NeedStore: st, Log: slog.New(slog.NewTextHandler(&logged, nil))}
			if err := run(lib, ctx, []string{"first", "second"}); err != nil {
				t.Fatalf("a saved batch = %v, want success", err)
			}
			if strings.Join(st.rewritten, ",") != "first,second" {
				t.Errorf("rewritten = %q, want both titles on a live context", st.rewritten)
			}
			if !strings.Contains(logged.String(), "disk is busy") {
				t.Errorf("the failed rewrite wasn't logged: %q", logged.String())
			}
		})
	}
}
