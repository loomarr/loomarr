package screening

import "slices"

var vocabulary = []AirworthinessFlag{
	FlagAdultNudity,
	FlagAgeAmbiguous,
	FlagAlcohol,
	FlagAnimalHarmOrDeath,
	FlagCommercialOrBrand,
	FlagDrug,
	FlagExplicitSexualLanguage,
	FlagFrighteningOrDisturbing,
	FlagGambling,
	FlagGraphicViolenceOrGore,
	FlagHateOrExtremistSymbol,
	FlagHatefulTargeting,
	FlagHumanDeathOrCorpse,
	FlagMinorNudityOrSexualRisk,
	FlagMinorPresent,
	FlagNonGraphicViolence,
	FlagProfanity,
	FlagRegulatedProductPromotion,
	FlagReligiousSuffering,
	FlagSelfHarmOrSuicide,
	FlagSevereInjuryOrMedical,
	FlagSexualActivityOrPresentation,
	FlagSlurOrDegradingLanguage,
	FlagThreat,
	FlagTobaccoOrNicotine,
	FlagWarOrMilitary,
	FlagWeaponDepiction,
}

var policyFlags = []AirworthinessFlag{
	FlagAdultNudity,
	FlagAlcohol,
	FlagAnimalHarmOrDeath,
	FlagDrug,
	FlagExplicitSexualLanguage,
	FlagFrighteningOrDisturbing,
	FlagGambling,
	FlagGraphicViolenceOrGore,
	FlagHateOrExtremistSymbol,
	FlagHatefulTargeting,
	FlagHumanDeathOrCorpse,
	FlagMinorNudityOrSexualRisk,
	FlagNonGraphicViolence,
	FlagProfanity,
	FlagRegulatedProductPromotion,
	FlagSelfHarmOrSuicide,
	FlagSevereInjuryOrMedical,
	FlagSexualActivityOrPresentation,
	FlagSlurOrDegradingLanguage,
	FlagThreat,
	FlagTobaccoOrNicotine,
	FlagWeaponDepiction,
}

var axisOrder = []AirworthinessAxis{AxisVisual, AxisSpoken, AxisWritten}

// airworthinessVocabulary returns a copy of the complete closed version-one suitability
// vocabulary for the tests that enumerate every flag.
func airworthinessVocabulary() []AirworthinessFlag { return slices.Clone(vocabulary) }

func validFlag(value AirworthinessFlag) bool { return slices.Contains(vocabulary, value) }

func validAxis(value AirworthinessAxis) bool { return slices.Contains(axisOrder, value) }

func validProfile(value AirworthinessProfile) bool {
	return value == ProfileAllAges || value == ProfileGeneralAudience || value == ProfileRestrictedArchive
}

func validSeverity(value AirworthinessSeverity) bool {
	return value == SeverityLow || value == SeverityModerate || value == SeverityHigh
}

func validContext(value AirworthinessContext) bool {
	return value == ContextPresence || value == ContextDepiction || value == ContextPromotion || value == ContextInstruction
}

func flagOwners(flag AirworthinessFlag) []AirworthinessAxis {
	switch flag {
	case FlagAdultNudity, FlagAgeAmbiguous, FlagAnimalHarmOrDeath, FlagFrighteningOrDisturbing,
		FlagGraphicViolenceOrGore, FlagHumanDeathOrCorpse, FlagMinorNudityOrSexualRisk,
		FlagMinorPresent, FlagNonGraphicViolence, FlagSevereInjuryOrMedical,
		FlagSexualActivityOrPresentation, FlagWeaponDepiction:
		return []AirworthinessAxis{AxisVisual}
	case FlagExplicitSexualLanguage, FlagProfanity, FlagSlurOrDegradingLanguage:
		return []AirworthinessAxis{AxisSpoken, AxisWritten}
	case FlagHateOrExtremistSymbol:
		return []AirworthinessAxis{AxisVisual, AxisWritten}
	case FlagAlcohol, FlagCommercialOrBrand, FlagDrug, FlagGambling, FlagHatefulTargeting,
		FlagRegulatedProductPromotion, FlagReligiousSuffering, FlagSelfHarmOrSuicide,
		FlagThreat, FlagTobaccoOrNicotine, FlagWarOrMilitary:
		return []AirworthinessAxis{AxisVisual, AxisSpoken, AxisWritten}
	default:
		return nil
	}
}

// airworthinessAxesForFlag returns the safety modalities whose certified coverage can
// support one closed suitability flag, for the tests that build per-axis fixtures.
func airworthinessAxesForFlag(flag AirworthinessFlag) []AirworthinessAxis {
	return slices.Clone(flagOwners(flag))
}

func axisOwnsFlag(axis AirworthinessAxis, flag AirworthinessFlag) bool {
	return slices.Contains(flagOwners(flag), axis)
}
