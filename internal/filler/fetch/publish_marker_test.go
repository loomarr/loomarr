package fetch

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// failingMarkerWriter fails every manifest write, as a locked database does.
type failingMarkerWriter struct{ err error }

func (w failingMarkerWriter) UpsertAcquisitionArtifacts(context.Context, []acquisition.Artifact) error {
	return w.err
}

// When publication fails, the artifact is meant to be recorded as repair so recovery finds it. If
// that record cannot be written the artifact still reads as staged, so the failure must say both
// things: the publish error and that the repair marker was not saved. The cases are every place a
// publish can fail after staging.
func TestPublish_ReportsAFailedRepairMarkerWrite(t *testing.T) {
	boom := errors.New("database is locked")
	for name, tc := range map[string]struct {
		setup func(t *testing.T, root string, artifact acquisition.Artifact)
		want  string // the publish failure, which must still be reported
	}{
		"a conflicting sidecar": {
			setup: func(t *testing.T, root string, _ acquisition.Artifact) { foreignSidecar(t, root) },
			want:  "occupied by another item",
		},
		"media that cannot be moved": {
			setup: func(t *testing.T, root string, artifact acquisition.Artifact) {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(artifact.StagingPath))); err != nil {
					t.Fatal(err)
				}
			},
			want: "no such file",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			artifact := stagedRecoveryArtifact(t, root)
			ing, outputs := publishFixture(t, root, artifact)
			tc.setup(t, root, artifact)
			var logs strings.Builder
			ing.log = slog.New(slog.NewTextHandler(&logs, nil))
			ing.writer = failingMarkerWriter{boom}
			ing.now = time.Now

			got, err := ing.publish(t.Context(), []acquisition.Artifact{artifact}, outputs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("publish err = %v, want the publish failure (%q) kept", err, tc.want)
			}
			if !errors.Is(err, boom) {
				t.Errorf("publish err = %v, want the failed repair-marker write reported too", err)
			}
			if got[0].State != acquisition.ArtifactRepair {
				t.Errorf("state = %s, want repair in the returned manifest", got[0].State)
			}
			if !strings.Contains(logs.String(), "repair marker") {
				t.Errorf("a failed repair-marker write left no log line:\n%s", logs.String())
			}
		})
	}
}
