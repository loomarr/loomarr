package proposaloutlook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// Planner derives the same channel that approval would commit, without writes.
type Planner interface {
	PlanSubmittedChannel(context.Context, store.Proposal) (store.Channel, error)
}

// Preview evaluates the normal channel scheduling inputs using fresh observations.
type Preview interface {
	PreviewPlannedChannel(context.Context, store.Channel, time.Time, schedule.Availability) (channels.CycleResult, error)
}

// Config supplies existing read-only adapters and the approval/scheduling authorities.
type Config struct {
	Titles   Titles
	Library  Library
	Episodes channels.EpisodeResolver
	Planner  Planner
	Preview  Preview
	Now      func() time.Time
}

// Service owns one bounded observation and its explanation; it cannot commit approval.
type Service struct{ config Config }

func New(config Config) *Service {
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{config: config}
}

type Mix struct {
	Core      int `json:"core"`
	Adjacent  int `json:"adjacent"`
	Discovery int `json:"discovery"`
	Unknown   int `json:"unknown"`
}

// Assessment is evidence for this exact proposal/edit snapshot, never a release or
// transport-readiness certificate. Durations count program media, excluding breaks.
type Assessment struct {
	Fingerprint         string                       `json:"fingerprint"`
	ObservedAt          time.Time                    `json:"observedAt"`
	State               string                       `json:"state" enum:"ready,waiting,uncertain,empty"`
	Titles              int                          `json:"titles"`
	ScheduledTitles     int                          `json:"scheduledTitles"`
	MissingAcquisitions int                          `json:"missingAcquisitions"`
	MissingLibrary      int                          `json:"missingLibrary"`
	UnknownTitles       int                          `json:"unknownTitles"`
	Programs            int                          `json:"programs"`
	Seasons             int                          `json:"seasons"`
	UniqueRuntimeMs     int64                        `json:"uniqueRuntimeMs"`
	FirstRepeatMs       *int64                       `json:"firstRepeatMs"`
	WindowMs            int64                        `json:"windowMs"`
	WindowLimited       bool                         `json:"windowLimited"`
	Thin                bool                         `json:"thin"`
	Ordering            schedule.OrderingMode        `json:"ordering"`
	Relaxations         []schedule.AppliedRelaxation `json:"relaxations"`
	Mix                 Mix                          `json:"mix"`
}

// Assess shares preparation with approval, but has no store-writing capability.
func (s *Service) Assess(ctx context.Context, proposal store.Proposal, edit *suggest.ApprovalEdit) (Assessment, error) {
	if s == nil || s.config.Planner == nil || s.config.Preview == nil {
		return Assessment{}, errors.New("proposal outlook is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	prepared, body, err := suggest.PrepareApproval(proposal, edit)
	if err != nil {
		return Assessment{}, err
	}
	channel, err := s.config.Planner.PlanSubmittedChannel(ctx, prepared)
	if err != nil {
		return Assessment{}, err
	}
	observedAt := s.config.Now().UTC()
	observed := observe(ctx, s.config.Titles, s.config.Library, s.config.Episodes, body, channel.Lineup)
	cycle, err := s.config.Preview.PreviewPlannedChannel(ctx, channel, observedAt, observed)
	if err != nil {
		return Assessment{}, err
	}
	result := summarize(cycle)
	result.ObservedAt = observedAt
	result.Titles = len(channel.Lineup)
	acquisitions := map[provision.Key]bool{}
	for _, item := range body.Acquisitions {
		if key, err := item.Key(); err == nil {
			acquisitions[key] = true
		}
	}
	for _, entry := range channel.Lineup {
		if observed.unknown[entry.Key] {
			result.UnknownTitles++
		} else if observed.missing[entry.Key] {
			if acquisitions[entry.Key] {
				result.MissingAcquisitions++
			} else {
				result.MissingLibrary++
			}
		}
	}
	result.Mix = editorialMix(body, channel.Lineup)
	switch {
	case result.Programs > 0:
		result.State = "ready"
	case result.UnknownTitles > 0:
		result.State = "uncertain"
	case result.MissingAcquisitions > 0:
		result.State = "waiting"
	default:
		result.State = "empty"
	}
	// Include effective planning state as well as edited content. An operator policy
	// change must invalidate an older estimate even when the proposal JSON is unchanged.
	channel.ReconcileDeadline = time.Time{} // operational due-now time is not proposal content
	snapshot, err := json.Marshal(struct {
		Proposal string
		Channel  store.Channel
	}{prepared.ProposalJSON, channel})
	if err != nil {
		return Assessment{}, err
	}
	digest := sha256.Sum256(snapshot)
	result.Fingerprint = hex.EncodeToString(digest[:])
	return result, nil
}

func summarize(cycle channels.CycleResult) Assessment {
	result := Assessment{WindowMs: cycle.Window.Milliseconds(), Ordering: cycle.Trace.Ordering, Relaxations: append([]schedule.AppliedRelaxation{}, cycle.Trace.Relaxations...)}
	programs := map[string]bool{}
	titles := map[provision.Key]bool{}
	seasons := map[struct {
		key    provision.Key
		season int
	}]bool{}
	var programTime, cycleTime int64
	for _, slot := range cycle.Slots {
		cycleTime += slot.DurationMs
		if !slot.IsProgram() || slot.LibraryItemID == "" || slot.DurationMs <= 0 {
			continue
		}
		if programs[slot.LibraryItemID] && result.FirstRepeatMs == nil {
			repeatAt := programTime
			result.FirstRepeatMs = &repeatAt
		}
		programTime += slot.DurationMs
		if programs[slot.LibraryItemID] {
			continue
		}
		programs[slot.LibraryItemID] = true
		titles[slot.Key] = true
		result.UniqueRuntimeMs += slot.DurationMs
		if slot.Key.IsSeries() && slot.Season > 0 {
			seasons[struct {
				key    provision.Key
				season int
			}{slot.Key, slot.Season}] = true
		}
	}
	result.Programs, result.ScheduledTitles, result.Seasons = len(programs), len(titles), len(seasons)
	result.WindowLimited = cycle.Window > 0 && cycleTime >= cycle.Window.Milliseconds()
	if result.Programs > 0 && result.FirstRepeatMs == nil && !result.WindowLimited {
		result.FirstRepeatMs = &programTime // the next program is the start of this complete cycle
	}
	result.Thin = result.Programs > 0 && (result.Programs < 3 || (result.FirstRepeatMs != nil && *result.FirstRepeatMs < int64((2*time.Hour)/time.Millisecond)))
	return result
}

func editorialMix(proposal suggest.Proposal, lineup []schedule.LineupEntry) Mix {
	// Historical and edited additions without run-local role evidence stay unknown.
	roles := map[provision.Key]suggest.EditorialRole{}
	for _, items := range [][]suggest.ProposalItem{proposal.Lineup, proposal.Acquisitions} {
		for _, item := range items {
			if key, err := item.Key(); err == nil {
				if _, present := roles[key]; !present {
					roles[key] = item.EditorialRole
				}
			}
		}
	}
	var mix Mix
	for _, entry := range lineup {
		switch roles[entry.Key] {
		case suggest.EditorialCore:
			mix.Core++
		case suggest.EditorialAdjacent:
			mix.Adjacent++
		case suggest.EditorialDiscovery:
			mix.Discovery++
		default:
			mix.Unknown++
		}
	}
	return mix
}
