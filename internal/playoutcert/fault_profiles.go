package playoutcert

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// FaultProfile is the fixed, redaction-safe vocabulary for opt-in destructive drills.
type FaultProfile string

const (
	FaultChildFailure  FaultProfile = "child_failure"
	FaultParentFailure FaultProfile = "parent_failure"
	FaultShutdown      FaultProfile = "shutdown"
)

var knownFaultProfiles = []FaultProfile{FaultChildFailure, FaultParentFailure, FaultShutdown}

// FaultController binds destructive authority to one explicitly named target scope.
// Implementations may expose richer drill operations to Run, but every controller must
// provide this identity so shutdown authority cannot be represented by a boolean.
type FaultController interface {
	Scope() string
}

// ParentFaultController can stop only a parent process which it owns for the
// current isolated target. The base URL binding prevents a scope string from
// becoming authority over a different endpoint.
type ParentFaultController interface {
	FaultController
	CurrentParent(context.Context, ParentFaultRequest) (uint64, error)
	FailParent(context.Context, ParentFaultRequest) (ParentFaultReceipt, error)
}

type ParentFaultRequest struct {
	BaseURL    string
	ChannelID  string
	Generation uint64
}

// ParentFaultReceipt contains only causal, non-secret evidence.
type ParentFaultReceipt struct {
	ChannelID  string
	Generation uint64
	Exited     bool
}

// ChildFaultController can stop only a currently registered program encoder
// belonging to this target's current owned parent. It deliberately exchanges
// opaque generations instead of diagnostics handles or process IDs.
type ChildFaultController interface {
	FaultController
	CurrentChild(context.Context, ChildFaultRequest) (ChildFaultTarget, error)
	FailChild(context.Context, ChildFaultRequest) (ChildFaultReceipt, error)
}

type ChildFaultRequest struct {
	BaseURL          string
	ChannelID        string
	ParentGeneration uint64
	ChildGeneration  uint64
}

type ChildFaultTarget struct {
	ParentGeneration uint64
	ChildGeneration  uint64
}

// ChildFaultReceipt contains only causal, non-secret evidence.
type ChildFaultReceipt struct {
	ChannelID        string
	ParentGeneration uint64
	ChildGeneration  uint64
	Exited           bool
}

type FaultQualification struct {
	Profile            FaultProfile    `json:"profile"`
	Status             string          `json:"status"`
	Outcome            string          `json:"outcome"`
	Baseline           *ResourceSample `json:"baseline,omitempty"`
	PhasePeak          *ResourceSample `json:"phasePeak,omitempty"`
	Final              *ResourceSample `json:"final,omitempty"`
	ReceiptOutcome     string          `json:"receiptOutcome,omitempty"`
	SelectedContinuity string          `json:"selectedContinuity,omitempty"`
	PeerContinuity     string          `json:"peerContinuity,omitempty"`
	Recovery           string          `json:"recovery,omitempty"`
}

func ParseFaultProfiles(values []string) ([]FaultProfile, error) {
	profiles := make([]FaultProfile, 0, len(values))
	for _, value := range values {
		profile := FaultProfile(strings.TrimSpace(value))
		if !slices.Contains(knownFaultProfiles, profile) {
			return nil, fmt.Errorf("unknown fault profile %q", value)
		}
		if slices.Contains(profiles, profile) {
			return nil, fmt.Errorf("duplicate fault profile %q", value)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func ValidateFaultSelection(profiles []FaultProfile, controllerScope, disposableTarget string) error {
	c := Config{FaultProfiles: profiles, DisposableTarget: disposableTarget}
	if controllerScope != "" {
		c.FaultController = namedFaultScope(controllerScope)
	}
	return c.validateFaultProfiles()
}

type namedFaultScope string

func (s namedFaultScope) Scope() string { return string(s) }

func (c Config) validateFaultProfiles() error {
	seen := make(map[FaultProfile]struct{}, len(c.FaultProfiles))
	for _, profile := range c.FaultProfiles {
		if !slices.Contains(knownFaultProfiles, profile) {
			return fmt.Errorf("unknown fault profile %q", profile)
		}
		if _, ok := seen[profile]; ok {
			return fmt.Errorf("duplicate fault profile %q", profile)
		}
		seen[profile] = struct{}{}
	}
	if _, shutdown := seen[FaultShutdown]; shutdown {
		if len(seen) != 1 {
			return errors.New("shutdown must be selected alone because it is terminal")
		}
		ack := strings.TrimSpace(c.DisposableTarget)
		if ack == "" {
			return errors.New("shutdown requires an explicit named disposable target acknowledgement")
		}
		if c.FaultController == nil || ack != c.FaultController.Scope() {
			return errors.New("disposable target acknowledgement must match the fault controller scope")
		}
	}
	return nil
}

func faultQualifications(selected []FaultProfile, controller FaultController) ([]FaultQualification, []string) {
	rows := make([]FaultQualification, 0, len(knownFaultProfiles))
	failures := []string{}
	for _, profile := range knownFaultProfiles {
		row := FaultQualification{Profile: profile, Status: "unqualified", Outcome: "not_selected"}
		if slices.Contains(selected, profile) {
			row.Status, row.Outcome = "unavailable", "controller_not_exercised"
			if controller == nil {
				row.Outcome = "controller_unavailable"
			}
			failures = append(failures, string(profile)+"_unavailable")
		}
		rows = append(rows, row)
	}
	return rows, failures
}
