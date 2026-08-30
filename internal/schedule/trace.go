package schedule

import "github.com/loomarr/loomarr/internal/provision"

// ScheduleTrace is bounded current-cycle evidence emitted by ComputeDesiredAt (§8.4).
// Unlike a Proposal's immutable DecisionTrace, this describes one scheduling computation over
// the approved lineup, live availability, resolved policy, and supplied wall-clock.
type ScheduleTrace struct {
	Version       int                 `json:"version"`
	Ordering      OrderingMode        `json:"ordering"`
	Seed          int64               `json:"seed"`
	WindowMs      int64               `json:"windowMs"`
	WindowIndex   int64               `json:"windowIndex"`
	Relaxations   []AppliedRelaxation `json:"relaxations"`
	FactTotal     int                 `json:"factTotal"`
	RecordedTotal int                 `json:"recordedTotal"`
	Truncated     bool                `json:"truncated"`
	Facts         []ScheduleFact      `json:"facts"`
}

// ScheduleFact is one fact captured at the seam that made the decision. Positions are
// zero-based and optional: deckPosition is before rolling-window selection; cyclePosition is
// the final slot position after break insertion.
type ScheduleFact struct {
	Stage         ScheduleStage   `json:"stage"`
	Outcome       ScheduleOutcome `json:"outcome"`
	Reason        ScheduleReason  `json:"reason"`
	Key           provision.Key   `json:"key,omitempty"`
	Title         string          `json:"title,omitempty"`
	Season        int             `json:"season,omitempty"`
	Episode       int             `json:"episode,omitempty"`
	DeckPosition  *int            `json:"deckPosition,omitempty"`
	CyclePosition *int            `json:"cyclePosition,omitempty"`
}

type ScheduleStage string

const (
	StageHardFilter       ScheduleStage = "hard_filter"
	StageAvailability     ScheduleStage = "availability"
	StageEpisodeSelection ScheduleStage = "episode_selection"
	StagePlacement        ScheduleStage = "placement"
)

type ScheduleOutcome string

const (
	OutcomeKept        ScheduleOutcome = "kept"
	OutcomeExcluded    ScheduleOutcome = "excluded"
	OutcomeAvailable   ScheduleOutcome = "available"
	OutcomePending     ScheduleOutcome = "pending"
	OutcomeSelected    ScheduleOutcome = "selected"
	OutcomeOmitted     ScheduleOutcome = "omitted"
	OutcomePlaced      ScheduleOutcome = "placed"
	OutcomeWindowedOut ScheduleOutcome = "windowed_out"
	OutcomeInserted    ScheduleOutcome = "inserted"
)

type ScheduleReason string

const (
	ReasonEligible              ScheduleReason = "eligible"
	ReasonOverCeiling           ScheduleReason = "over_ceiling"
	ReasonUnrated               ScheduleReason = "unrated"
	ReasonOutOfScope            ScheduleReason = "out_of_scope"
	ReasonOutOfRuleScope        ScheduleReason = "out_of_rule_scope"
	ReasonOutOfSeason           ScheduleReason = "out_of_season"
	ReasonAvailable             ScheduleReason = "available"
	ReasonUnavailablePodFill    ScheduleReason = "unavailable_pod_fill"
	ReasonUnavailableComingSoon ScheduleReason = "unavailable_coming_soon"
	ReasonCompleteSelected      ScheduleReason = "complete_selected"
	ReasonHighlightsSelected    ScheduleReason = "highlights_selected"
	ReasonHighlightsOmitted     ScheduleReason = "highlights_omitted"
	ReasonHolidaySelected       ScheduleReason = "holiday_selected"
	ReasonHolidayOmitted        ScheduleReason = "holiday_omitted"
	ReasonFullRunFallback       ScheduleReason = "full_run_fallback"
	ReasonOutOfSeasonRange      ScheduleReason = "out_of_season_range"
	ReasonSequential            ScheduleReason = "sequential"
	ReasonShuffle               ScheduleReason = "shuffle"
	ReasonSyndication           ScheduleReason = "syndication"
	ReasonWindowedOut           ScheduleReason = "windowed_out"
	ReasonCommercialBreak       ScheduleReason = "commercial_break"
)

const (
	ScheduleTraceVersion          = 1
	ScheduleTraceMaxFacts         = 1024
	scheduleTracePlacementReserve = 256
)

type scheduleTraceBuilder struct {
	trace                ScheduleTrace
	nonPlacementRecorded int
	placementRecorded    int
}

func newScheduleTrace(ordering OrderingMode, seed int64, windowMs, windowIndex int64, factHint int) *scheduleTraceBuilder {
	factHint = min(max(factHint, 64), ScheduleTraceMaxFacts)
	return &scheduleTraceBuilder{trace: ScheduleTrace{
		Version: ScheduleTraceVersion, Ordering: ordering, Seed: seed,
		WindowMs: windowMs, WindowIndex: windowIndex,
		Facts: make([]ScheduleFact, 0, factHint),
	}}
}

func (b *scheduleTraceBuilder) add(fact ScheduleFact) {
	if b == nil {
		return
	}
	b.trace.FactTotal++
	// A long series can emit hundreds of episode-selection facts before placement. Reserve a
	// quarter of the bound for placement so truncation never erases the entire answer to "why
	// does this air here?" while preserving stable pipeline order within the retained stream.
	limit := ScheduleTraceMaxFacts - scheduleTracePlacementReserve
	recorded := b.nonPlacementRecorded
	if fact.Stage == StagePlacement {
		limit = scheduleTracePlacementReserve
		recorded = b.placementRecorded
	}
	if recorded < limit {
		b.trace.Facts = append(b.trace.Facts, fact)
		b.trace.RecordedTotal++
		if fact.Stage == StagePlacement {
			b.placementRecorded++
		} else {
			b.nonPlacementRecorded++
		}
		return
	}
	b.trace.Truncated = true
}

func (b *scheduleTraceBuilder) finish(relaxations []AppliedRelaxation) ScheduleTrace {
	b.trace.Relaxations = append([]AppliedRelaxation(nil), relaxations...)
	b.trace.Truncated = b.trace.Truncated || b.trace.FactTotal > b.trace.RecordedTotal
	return b.trace
}

func intRef(v int) *int { return &v }

func tracePlacement(trace *scheduleTraceBuilder, fullDeck, selected, final []Slot, ordering OrderingMode) {
	selectedCounts := make(map[slotTraceID]int, len(selected))
	deckPositions := make(map[slotTraceID][]int, len(fullDeck))
	for i, slot := range fullDeck {
		identity := slotIdentity(slot)
		deckPositions[identity] = append(deckPositions[identity], i)
	}
	for _, slot := range selected {
		selectedCounts[slotIdentity(slot)]++
	}
	for i, slot := range fullDeck {
		identity := slotIdentity(slot)
		if selectedCounts[identity] > 0 {
			selectedCounts[identity]--
			continue
		}
		trace.add(ScheduleFact{Stage: StagePlacement, Outcome: OutcomeWindowedOut, Reason: ReasonWindowedOut,
			Key: slot.Key, Title: slot.Title, Season: slot.Season, Episode: slot.Episode, DeckPosition: intRef(i)})
	}

	reason := ReasonSequential
	switch ordering {
	case OrderShuffle:
		reason = ReasonShuffle
	case OrderSyndication:
		reason = ReasonSyndication
	}
	usedPositions := make(map[slotTraceID]int, len(deckPositions))
	for i, slot := range final {
		if slot.Kind == SlotFiller && slot.Key == "" {
			trace.add(ScheduleFact{Stage: StagePlacement, Outcome: OutcomeInserted, Reason: ReasonCommercialBreak,
				CyclePosition: intRef(i)})
			continue
		}
		identity := slotIdentity(slot)
		positions := deckPositions[identity]
		used := usedPositions[identity]
		var deckPosition *int
		if used < len(positions) {
			deckPosition = intRef(positions[used])
			usedPositions[identity] = used + 1
		}
		trace.add(ScheduleFact{Stage: StagePlacement, Outcome: OutcomePlaced, Reason: reason,
			Key: slot.Key, Title: slot.Title, Season: slot.Season, Episode: slot.Episode,
			DeckPosition: deckPosition, CyclePosition: intRef(i)})
	}
}

type slotTraceID struct {
	kind          SlotKind
	key           provision.Key
	libraryItemID string
	title         string
	partGroup     string
	season        int
	episode       int
	partIndex     int
}

func slotIdentity(slot Slot) slotTraceID {
	return slotTraceID{
		kind: slot.Kind, key: slot.Key, libraryItemID: slot.LibraryItemID, title: slot.Title,
		partGroup: slot.PartGroup, season: slot.Season, episode: slot.Episode, partIndex: slot.PartIndex,
	}
}
