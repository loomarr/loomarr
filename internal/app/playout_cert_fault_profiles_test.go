package app

import (
	"context"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestSyntheticParentFaultRefusesWrongTargetAndStaleParent(t *testing.T) {
	target := &PlayoutCertificationTarget{BaseURL: "http://127.0.0.1:9999", parents: map[string]*syntheticParent{}}
	if _, err := target.CurrentParent(context.Background(), playoutcert.ParentFaultRequest{BaseURL: "http://127.0.0.1:9998", ChannelID: "channel"}); err == nil {
		t.Fatal("CurrentParent accepted a mismatched target")
	}
	if _, err := target.FailParent(context.Background(), playoutcert.ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: "channel", Generation: 1}); err == nil {
		t.Fatal("FailParent accepted a stale parent")
	}
}

func TestSyntheticShutdownRefusesMismatchAndSharesOneTerminalReceipt(t *testing.T) {
	target := &PlayoutCertificationTarget{BaseURL: "http://127.0.0.1:9999", scope: "isolated"}
	if _, err := target.Shutdown(context.Background(), playoutcert.ShutdownRequest{BaseURL: "http://127.0.0.1:9998"}); err == nil {
		t.Fatal("Shutdown accepted a mismatched target")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := target.Shutdown(cancelled, playoutcert.ShutdownRequest{BaseURL: target.BaseURL}); err == nil {
		t.Fatal("Shutdown reported success for a cancelled caller")
	}
	var wg sync.WaitGroup
	receipts := make([]playoutcert.ShutdownReceipt, 2)
	errs := make([]error, 2)
	for index := range receipts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipts[index], errs[index] = target.Shutdown(context.Background(), playoutcert.ShutdownRequest{BaseURL: target.BaseURL})
		}()
	}
	wg.Wait()
	for index := range receipts {
		if errs[index] != nil || receipts[index].Scope != target.scope || !receipts[index].ServingStopped || !receipts[index].ProcessesExited {
			t.Fatalf("shutdown receipt %d = %+v, %v", index, receipts[index], errs[index])
		}
	}
	if _, err := target.SampleStopped(context.Background(), "final"); err == nil {
		t.Fatal("SampleStopped fabricated a sample without an owned retained projection")
	}
}
