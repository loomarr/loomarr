package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

// newID generates a random 128-bit hex id for jobs/proposals.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "id-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// instanceDeviceID returns a stable per-install id (§11) used as the media
// server DeviceId, so Loomarr appears as one device. Generated + stored on first
// call; read thereafter. Best-effort — falls back to a constant if the store errs.
func instanceDeviceID(ctx context.Context, st store.Store) string {
	const key = "instance_id"
	if v, err := st.GetSetting(ctx, key); err == nil && v != "" {
		return v
	}
	id := "loomarr-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := st.SetSetting(ctx, key, id); err != nil {
		return "loomarr"
	}
	return id
}
