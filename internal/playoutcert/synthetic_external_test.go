package playoutcert_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestSyntheticCloseExecutesShutdownButDoesNotQualifyDrill(t *testing.T) {
	target := &app.PlayoutCertificationTarget{BaseURL: "http://127.0.0.1:9999"}
	if err := target.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if receipt, err := target.Shutdown(context.Background(), playoutcert.ShutdownRequest{BaseURL: target.BaseURL}); err != nil || !receipt.ServingStopped || !receipt.ProcessesExited {
		t.Fatalf("Close did not preserve the terminal stop: receipt=%+v err=%v", receipt, err)
	}
	rows, failures := playoutcert.FaultQualificationsForTest([]playoutcert.FaultProfile{playoutcert.FaultShutdown}, target)
	if len(failures) != 1 || rows[2].Status != "unavailable" || rows[2].Outcome != "controller_not_exercised" {
		t.Fatalf("ordinary Close qualified shutdown: rows=%+v failures=%v", rows, failures)
	}
}
