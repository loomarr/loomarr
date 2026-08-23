package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

func TestTrackReplicaSettingsRefresh_SQLiteStartsNoReader(t *testing.T) {
	var reads atomic.Int64
	loader := settings.StoreLoader{List: func(context.Context) ([]settings.SettingRow, error) {
		reads.Add(1)
		return nil, nil
	}}
	svc, err := settings.New(context.Background(), settings.NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := newGenerationLifecycle(context.Background())
	t.Cleanup(func() {
		if err := owner.shutdown(context.Background()); err != nil {
			t.Errorf("shutdown settings lifecycle: %v", err)
		}
	})
	if trackReplicaSettingsRefresh(owner, store.DialectSQLite, svc, time.Nanosecond, nil) {
		t.Fatal("SQLite started a replica settings refresher")
	}
	time.Sleep(time.Millisecond)
	if got := reads.Load(); got != 1 {
		t.Fatalf("SQLite settings reads = %d, want only the boot read", got)
	}
}
