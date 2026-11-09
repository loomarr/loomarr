package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

// visionSet builds a live settings service over the DECLARED registry, so these tests resolve
// exactly as boot does — including defaults, kinds and validation. A hand-rolled map would let a
// key that the registry rejects pass here and fail on the operator's box.
func visionSet(t *testing.T, rows map[string]string) resolved {
	t.Helper()
	loader := settings.StoreLoader{List: func(context.Context) ([]settings.SettingRow, error) {
		out := make([]settings.SettingRow, 0, len(rows))
		for k, v := range rows {
			out = append(out, settings.SettingRow{Key: k, Value: v, UpdatedBy: "test"})
		}
		return out, nil
	}}
	svc, err := settings.New(context.Background(), settings.NewRegistry(), loader, nil)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return resolved{svc: svc}
}
