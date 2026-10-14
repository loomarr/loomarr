package kit

import "encoding/json"

// Trace is the part of a cycle preview's schedule trace (GET /v1/channels/{id}/cycle) the kit
// reads. The JSON tags match both the API DTO and schedule.ScheduleTrace.
type Trace struct {
	Ordering    string            `json:"ordering"`
	Seed        json.RawMessage   `json:"seed"` // a string in the API DTO, a number in the domain type
	Truncated   bool              `json:"truncated"`
	FactTotal   int               `json:"factTotal"`
	Relaxations []json.RawMessage `json:"relaxations"`
	Facts       []TraceFact       `json:"facts"`
}

// TraceFact is one recorded trace fact.
type TraceFact struct {
	Stage        string `json:"stage"`
	Outcome      string `json:"outcome"`
	DeckPosition *int   `json:"deckPosition"`
}

// Trace bounds, mirrored from internal/schedule/trace.go (ScheduleTraceMaxFacts and
// scheduleTracePlacementReserve): at most 256 placement facts and 768 others are recorded, while
// FactTotal counts every fact, recorded or not.
const (
	traceMaxFacts        = 1024
	tracePlacementFacts  = 256
	traceOtherFactsLimit = traceMaxFacts - tracePlacementFacts
)

// PoolFromTrace counts the episodes and films in a channel's full ordered deck, before the
// rolling-window slice. `breaks` is the number of commercial breaks in the arranged window.
//
// Every deck unit yields exactly one placement fact (placed or windowed out); each commercial
// break yields one more ("inserted", no deck position), which is not pool.
//
//   - Untruncated trace: count the placement facts that carry a deck position.
//   - Truncated (more than 256 placement facts): when every non-placement fact was recorded, all
//     unrecorded facts are placement facts, so pool = FactTotal − other facts − breaks.
//   - Otherwise the trace cannot answer: ok = false.
func PoolFromTrace(t Trace, breaks int) (pool int, ok bool) {
	withPos, other := 0, 0
	for _, f := range t.Facts {
		switch {
		case f.Stage != "placement":
			other++
		case f.DeckPosition != nil:
			withPos++
		}
	}
	if !t.Truncated {
		return withPos, withPos > 0
	}
	if other >= traceOtherFactsLimit {
		return 0, false
	}
	pool = t.FactTotal - other - breaks
	return pool, pool > 0
}
