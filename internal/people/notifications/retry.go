package notifications

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

const MaxAttempts = 5

var retryBase = [...]time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// RetryDelay is fixed product policy: attempts 2..5 follow the §11 ladder with deterministic
// bounded jitter. Determinism makes restart and cross-replica behavior agree without persisted RNG.
func RetryDelay(intentID string, nextAttempt int) (time.Duration, bool) {
	if nextAttempt < 2 || nextAttempt > MaxAttempts {
		return 0, false
	}
	base := retryBase[nextAttempt-2]
	sum := sha256.Sum256([]byte(intentID + "\x00" + string(rune(nextAttempt))))
	// Scale the first 16 bits into [-10%, +10%], inclusive.
	point := int64(binary.BigEndian.Uint16(sum[:2]))
	partsPerTenThousand := point*2000/65535 - 1000
	jitter := time.Duration(int64(base) * partsPerTenThousand / 10_000)
	return (base + jitter).Round(time.Second), true
}
