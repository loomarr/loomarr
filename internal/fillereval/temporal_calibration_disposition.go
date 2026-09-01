package fillereval

import (
	"slices"
	"sort"
)

const temporalCalibrationMinimumAgreementControls = 3

type TemporalCalibrationNextAction string

const (
	TemporalCalibrationRepairSelection  TemporalCalibrationNextAction = "repair_calibration_selection"
	TemporalCalibrationRepeatHosted     TemporalCalibrationNextAction = "repeat_bounded_hosted_calibration"
	TemporalCalibrationRepairEvidence   TemporalCalibrationNextAction = "repair_evidence_or_prompt"
	TemporalCalibrationRepeatDiagnostic TemporalCalibrationNextAction = "revise_assessors_and_repeat_32_case_diagnostic"
)

type TemporalCalibrationAxisOutcome string

const (
	TemporalCalibrationAxisNotTested     TemporalCalibrationAxisOutcome = "not_tested"
	TemporalCalibrationAxisMatchedFirst  TemporalCalibrationAxisOutcome = "unanimously_matched_first"
	TemporalCalibrationAxisMatchedSecond TemporalCalibrationAxisOutcome = "unanimously_matched_second"
	TemporalCalibrationAxisMixedSupport  TemporalCalibrationAxisOutcome = "mixed_local_support"
	TemporalCalibrationAxisNovelClaim    TemporalCalibrationAxisOutcome = "novel_or_not_comparable"
	TemporalCalibrationAxisIncomplete    TemporalCalibrationAxisOutcome = "incomplete"
)

type TemporalCalibrationDisposition struct {
	PolicyVersion            string                               `json:"policyVersion"`
	NextAction               TemporalCalibrationNextAction        `json:"nextAction"`
	FullCorpusRelabelAllowed bool                                 `json:"fullCorpusRelabelAllowed"`
	Reasons                  []string                             `json:"reasons"`
	Axes                     []TemporalCalibrationAxisDisposition `json:"axes"`
}

type TemporalCalibrationAxisDisposition struct {
	Axis               string                         `json:"axis"`
	Disputes           int                            `json:"disputes"`
	MatchedFirst       int                            `json:"matchedFirst"`
	MatchedSecond      int                            `json:"matchedSecond"`
	MatchedNeither     int                            `json:"matchedNeither"`
	NotComparable      int                            `json:"notComparable"`
	OperationalFailure int                            `json:"operationalFailure"`
	Outcome            TemporalCalibrationAxisOutcome `json:"outcome"`
}

const temporalCalibrationDispositionPolicyVersion = "filler-temporal-calibration-disposition-v1"

// temporalCalibrationDisposition prescribes only the next diagnostic
// experiment. A third model is not truth, so this policy never authorizes the
// full-corpus relabel; that still requires a passing fresh 32-case comparison.
func temporalCalibrationDisposition(report TemporalCalibrationReport) TemporalCalibrationDisposition {
	unit := temporalCalibrationAxis(report.CaseResults, "unit", "unit_disagreement")
	role := temporalCalibrationAxis(report.CaseResults, "role", "role_disagreement")
	disposition := TemporalCalibrationDisposition{
		PolicyVersion: temporalCalibrationDispositionPolicyVersion,
		NextAction:    TemporalCalibrationRepeatDiagnostic,
		Axes:          []TemporalCalibrationAxisDisposition{unit, role},
	}

	if report.AgreementControls < temporalCalibrationMinimumAgreementControls || unit.Disputes == 0 || role.Disputes == 0 {
		disposition.NextAction = TemporalCalibrationRepairSelection
		if report.AgreementControls < temporalCalibrationMinimumAgreementControls {
			disposition.Reasons = append(disposition.Reasons, "fewer_than_three_agreement_controls")
		}
		if unit.Disputes == 0 {
			disposition.Reasons = append(disposition.Reasons, "no_unit_disputes")
		}
		if role.Disputes == 0 {
			disposition.Reasons = append(disposition.Reasons, "no_role_disputes")
		}
		return canonicalTemporalCalibrationDisposition(disposition)
	}

	if report.OperationalFailures > 0 {
		disposition.NextAction = TemporalCalibrationRepeatHosted
		disposition.Reasons = append(disposition.Reasons, "hosted_operational_failure")
		return canonicalTemporalCalibrationDisposition(disposition)
	}

	if report.ThirdUnitUnclear > 0 || report.ThirdRoleUnclear > 0 {
		disposition.NextAction = TemporalCalibrationRepairEvidence
		disposition.Reasons = append(disposition.Reasons, "hosted_semantic_abstention")
	}
	if report.AgreementControlsBroken > 0 || report.AgreementControlsPreserved != report.AgreementControls {
		disposition.NextAction = TemporalCalibrationRepairEvidence
		disposition.Reasons = append(disposition.Reasons, "agreement_control_broken")
	}
	for _, axis := range disposition.Axes {
		switch axis.Outcome {
		case TemporalCalibrationAxisNovelClaim:
			disposition.NextAction = TemporalCalibrationRepairEvidence
			disposition.Reasons = append(disposition.Reasons, axis.Axis+"_novel_or_not_comparable")
		case TemporalCalibrationAxisMixedSupport:
			disposition.NextAction = TemporalCalibrationRepairEvidence
			disposition.Reasons = append(disposition.Reasons, axis.Axis+"_mixed_local_support")
		case TemporalCalibrationAxisIncomplete:
			disposition.NextAction = TemporalCalibrationRepeatHosted
			disposition.Reasons = append(disposition.Reasons, axis.Axis+"_incomplete")
		}
	}
	if len(disposition.Reasons) == 0 {
		disposition.Reasons = []string{"clean_controls_and_unanimous_axis_support"}
	}
	return canonicalTemporalCalibrationDisposition(disposition)
}

func temporalCalibrationAxis(results []TemporalCalibrationCaseResult, axis, reason string) TemporalCalibrationAxisDisposition {
	disposition := TemporalCalibrationAxisDisposition{Axis: axis}
	for _, result := range results {
		if !slices.Contains(result.Reasons, reason) {
			continue
		}
		disposition.Disputes++
		relation := result.UnitRelation
		if axis == "role" {
			relation = result.RoleRelation
		}
		switch relation {
		case TemporalCalibrationMatchedFirst:
			disposition.MatchedFirst++
		case TemporalCalibrationMatchedSecond:
			disposition.MatchedSecond++
		case TemporalCalibrationMatchedNeither:
			disposition.MatchedNeither++
		case TemporalCalibrationNotApplicable, TemporalCalibrationNotComparable, TemporalCalibrationAgreementBroken, TemporalCalibrationAgreementPreserved:
			disposition.NotComparable++
		case TemporalCalibrationOperationalFailure:
			disposition.OperationalFailure++
		default:
			disposition.NotComparable++
		}
	}
	disposition.Outcome = temporalCalibrationAxisOutcome(disposition)
	return disposition
}

func temporalCalibrationAxisOutcome(axis TemporalCalibrationAxisDisposition) TemporalCalibrationAxisOutcome {
	if axis.Disputes == 0 {
		return TemporalCalibrationAxisNotTested
	}
	if axis.OperationalFailure > 0 {
		return TemporalCalibrationAxisIncomplete
	}
	if axis.MatchedNeither > 0 || axis.NotComparable > 0 {
		return TemporalCalibrationAxisNovelClaim
	}
	if axis.MatchedFirst == axis.Disputes {
		return TemporalCalibrationAxisMatchedFirst
	}
	if axis.MatchedSecond == axis.Disputes {
		return TemporalCalibrationAxisMatchedSecond
	}
	return TemporalCalibrationAxisMixedSupport
}

func canonicalTemporalCalibrationDisposition(disposition TemporalCalibrationDisposition) TemporalCalibrationDisposition {
	sort.Strings(disposition.Reasons)
	return disposition
}
