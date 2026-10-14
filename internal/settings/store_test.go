package settings

import (
	"context"
	"testing"
)

func TestStoreLoader_LoadSnapshotReadsValuesAndOwnershipOnce(t *testing.T) {
	reads := 0
	loader := StoreLoader{List: func(context.Context) ([]SettingRow, error) {
		reads++
		return []SettingRow{
			{Key: "library.url", Value: "http://emby:8096"},
			{Key: "library.token", Value: "secret", EnvOverride: true},
		}, nil
	}}
	snapshot, err := loader.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("List calls = %d, want one coherent table read", reads)
	}
	if snapshot.Values["library.url"] != "http://emby:8096" ||
		snapshot.Values["library.token"] != "secret" ||
		!snapshot.EnvOverrides["library.token"] {
		t.Fatalf("snapshot = %+v, want values and ownership from the same rows", snapshot)
	}
}
