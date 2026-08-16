package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
)

func TestStartReplicaSettingsRefresh_SQLiteStartsNoReader(t *testing.T) {
	var reads atomic.Int64
	loader := settings.StoreLoader{List: func(context.Context) ([]settings.SettingRow, error) {
		reads.Add(1)
		return nil, nil
	}}
	svc, err := settings.New(context.Background(), settings.NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if startReplicaSettingsRefresh(ctx, store.DialectSQLite, svc, time.Nanosecond, nil) {
		t.Fatal("SQLite started a replica settings refresher")
	}
	time.Sleep(time.Millisecond)
	if got := reads.Load(); got != 1 {
		t.Fatalf("SQLite settings reads = %d, want only the boot read", got)
	}
}
