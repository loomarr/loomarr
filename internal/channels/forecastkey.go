package channels

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/fnv"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// ForecastKey hashes every input the forecast of ch at `at` reads from the channel row and the
// install settings, so a cached forecast under the same key is the forecast CyclePreview would
// compute now. ok=false means an input could not be read or encoded; the caller computes instead.
//
// It answers a different question from decisionDigest. That digest leaves out library facts and
// the relaxations reconcile writes back, because they are not decisions; a cache key must include
// them, because the forecast reads them. So the lineup and policy are hashed whole, through the
// JSON the store round-trips: a field added to either changes the key and costs a miss, the safe
// direction.
//
// TestForecastKeyClassifiesEveryChannelField classifies every channel field as keyed, derived
// by assemble, or not a forecast input. Availability, the filler pool, airing history and
// natural-break measurements are not on the row; the guide's cache bounds their staleness by its
// TTL.
//
// `at` enters through schedule.ClockSignature (the active rule and holidays), and the rolling
// window index is the cache's to add. An internal channel's mid-roll splits pinned near `at` are
// read from the keyed accepted cycle, so two instants in one window share an entry whose pinned
// splits the TTL bounds too.
func (e *Engine) ForecastKey(ctx context.Context, ch channelstore.Channel, at time.Time) (uint64, bool) {
	s, err := e.settingsSnapshot(ctx)
	if err != nil {
		return 0, false
	}
	zone := ""
	if s.Calendar.Zone != nil {
		zone = s.Calendar.Zone.String()
	}
	blob, err := json.Marshal(struct {
		ID            string
		Lineup        []schedule.LineupEntry
		Policy        schedule.ChannelPolicy
		Strategy      schedule.Strategy
		Shuffle       schedule.ShuffleParams
		CreatedAt     time.Time
		PlayoutAnchor time.Time
		Desired       []schedule.Slot
		BreaksPerHour int
		BreakLength   time.Duration
		Window        time.Duration
		Pending       schedule.PendingPolicy
		Zone, Country string
		Holidays      any
		Backend       string
	}{
		ch.ID, ch.Lineup, ch.Policy, ch.Strategy, ch.Shuffle, ch.CreatedAt, ch.PlayoutAnchor, ch.Desired,
		s.BreaksPerHour, s.BreakLength, s.Window, s.Pending, zone, s.Calendar.Country, s.Calendar.Holidays, s.Backend,
	})
	if err != nil {
		return 0, false
	}
	h := fnv.New64a()
	_, _ = h.Write(blob)
	var num [8]byte
	binary.LittleEndian.PutUint64(num[:], schedule.ClockSignature(ch.Policy, at, s.Calendar))
	_, _ = h.Write(num[:])
	return h.Sum64(), true
}
