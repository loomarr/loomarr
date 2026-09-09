package playoutcert

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestPreparedHLSRejectsChangedCoordinatesForReplayedSegment(t *testing.T) {
	for _, mode := range []playoutcertfixture.PreparedHLSMode{playoutcertfixture.PreparedHLSChangedTime, playoutcertfixture.PreparedHLSChangedDuration} {
		fixture := playoutcertfixture.NewPreparedHLS(t, mode)
		config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, RequestTimeout: time.Second}
		e, err := newEndpoint(config.normalized())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		signed, _, class := e.mint(ctx, "prepared")
		if class != "ok" {
			cancel()
			t.Fatal(class)
		}
		reader := newPreparedHLSReader(ctx, e, signed)
		firstErr := reader.refresh()
		replayErr := reader.refresh()
		closeErr := reader.Close()
		cancel()
		if firstErr != nil || replayErr == nil || replayErr.Error() != "inconsistent_hls_replay" || closeErr != nil {
			t.Fatalf("mode=%d initial=%v replay=%v close=%v", mode, firstErr, replayErr, closeErr)
		}
	}
}
