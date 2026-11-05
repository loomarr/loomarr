package fillerscreening

// Airworthiness is deterministic audience-policy evaluation over closed, authority-bound filler
// suitability evidence: the three safety axes' certified observations reduce to one decision that
// the four-axis aggregate carries.

const (
	AirworthinessEvidenceSchemaVersion   = 1
	AirworthinessEvidenceContractVersion = "filler-airworthiness-evidence-v1"
	AirworthinessDecisionSchemaVersion   = 1
	AirworthinessDecisionContractVersion = "filler-airworthiness-decision-v1"
	AirworthinessPolicyVersion           = "filler-airworthiness-policy-v1"
	AirworthinessVocabularyVersion       = "filler-suitability-vocabulary-v1"
	maximumEvidenceAxes                  = 3
	maximumObservations                  = 256
	// AirworthinessMaximumRenderedDurationMS bounds an Airworthiness evidence document.
	AirworthinessMaximumRenderedDurationMS = int64(24 * 60 * 60 * 1000)
	maximumRenderedDurationMS              = AirworthinessMaximumRenderedDurationMS
)

type AirworthinessProfile string

const (
	ProfileAllAges           AirworthinessProfile = "all_ages"
	ProfileGeneralAudience   AirworthinessProfile = "general_audience"
	ProfileRestrictedArchive AirworthinessProfile = "restricted_archive"
)

type AirworthinessAxis string

const (
	AxisVisual  AirworthinessAxis = "visual"
	AxisSpoken  AirworthinessAxis = "spoken"
	AxisWritten AirworthinessAxis = "written"
)

type AirworthinessCoverage string

const (
	CoverageComplete   AirworthinessCoverage = "complete"
	CoverageIncomplete AirworthinessCoverage = "incomplete"
	CoverageFailed     AirworthinessCoverage = "failed"
	CoverageConflict   AirworthinessCoverage = "conflict"
)

type AirworthinessFlag string

const (
	FlagAdultNudity                  AirworthinessFlag = "adult_nudity"
	FlagAgeAmbiguous                 AirworthinessFlag = "age_ambiguous"
	FlagAlcohol                      AirworthinessFlag = "alcohol"
	FlagAnimalHarmOrDeath            AirworthinessFlag = "animal_harm_or_death"
	FlagCommercialOrBrand            AirworthinessFlag = "commercial_or_brand"
	FlagDrug                         AirworthinessFlag = "drug"
	FlagExplicitSexualLanguage       AirworthinessFlag = "explicit_sexual_language"
	FlagFrighteningOrDisturbing      AirworthinessFlag = "frightening_or_disturbing"
	FlagGambling                     AirworthinessFlag = "gambling"
	FlagGraphicViolenceOrGore        AirworthinessFlag = "graphic_violence_or_gore"
	FlagHateOrExtremistSymbol        AirworthinessFlag = "hate_or_extremist_symbol"
	FlagHatefulTargeting             AirworthinessFlag = "hateful_targeting"
	FlagHumanDeathOrCorpse           AirworthinessFlag = "human_death_or_corpse"
	FlagMinorNudityOrSexualRisk      AirworthinessFlag = "minor_nudity_or_sexual_risk"
	FlagMinorPresent                 AirworthinessFlag = "minor_present"
	FlagNonGraphicViolence           AirworthinessFlag = "non_graphic_violence"
	FlagProfanity                    AirworthinessFlag = "profanity"
	FlagRegulatedProductPromotion    AirworthinessFlag = "regulated_product_promotion"
	FlagReligiousSuffering           AirworthinessFlag = "religious_suffering"
	FlagSelfHarmOrSuicide            AirworthinessFlag = "self_harm_or_suicide"
	FlagSevereInjuryOrMedical        AirworthinessFlag = "severe_injury_or_medical"
	FlagSexualActivityOrPresentation AirworthinessFlag = "sexual_activity_or_presentation"
	FlagSlurOrDegradingLanguage      AirworthinessFlag = "slur_or_degrading_language"
	FlagThreat                       AirworthinessFlag = "threat"
	FlagTobaccoOrNicotine            AirworthinessFlag = "tobacco_or_nicotine"
	FlagWarOrMilitary                AirworthinessFlag = "war_or_military"
	FlagWeaponDepiction              AirworthinessFlag = "weapon_depiction"
)

type AirworthinessSeverity string

const (
	SeverityLow      AirworthinessSeverity = "low"
	SeverityModerate AirworthinessSeverity = "moderate"
	SeverityHigh     AirworthinessSeverity = "high"
)

type AirworthinessContext string

const (
	ContextPresence    AirworthinessContext = "presence"
	ContextDepiction   AirworthinessContext = "depiction"
	ContextPromotion   AirworthinessContext = "promotion"
	ContextInstruction AirworthinessContext = "instruction"
)

type AirworthinessAxisProfile struct {
	Axis                 AirworthinessAxis   `json:"axis"`
	EvidenceContract     string              `json:"evidenceContract"`
	PolicySHA256         string              `json:"policySha256"`
	CertificationSHA256  string              `json:"certificationSha256"`
	ImplementationSHA256 string              `json:"implementationSha256"`
	CertifiedFlags       []AirworthinessFlag `json:"certifiedFlags"`
}

// AirworthinessDocument contains already-reduced axis evidence. It carries positive
// observations and complete-coverage state, never raw model votes or text.
type AirworthinessDocument struct {
	SchemaVersion   int                         `json:"schemaVersion"`
	ContractVersion string                      `json:"contractVersion"`
	SubjectSHA256   string                      `json:"subjectSha256"`
	DurationMS      int64                       `json:"durationMs"`
	Axes            []AirworthinessAxisEvidence `json:"axes"`
}

type AirworthinessAxisEvidence struct {
	SubjectSHA256  string                     `json:"subjectSha256"`
	Profile        AirworthinessAxisProfile   `json:"profile"`
	Coverage       AirworthinessCoverage      `json:"coverage"`
	EvidenceSHA256 string                     `json:"evidenceSha256"`
	Observations   []AirworthinessObservation `json:"observations"`
}

type AirworthinessObservation struct {
	ID       string                `json:"id"`
	Flag     AirworthinessFlag     `json:"flag"`
	Severity AirworthinessSeverity `json:"severity"`
	Context  AirworthinessContext  `json:"context"`
	StartMS  int64                 `json:"startMs"`
	EndMS    int64                 `json:"endMs"`
}

type AirworthinessVerdict string

const (
	VerdictPass   AirworthinessVerdict = "pass"
	VerdictReject AirworthinessVerdict = "reject"
	VerdictHold   AirworthinessVerdict = "hold"
)

type AirworthinessReason string

const (
	ReasonEvidenceSatisfied         AirworthinessReason = "airworthiness_evidence_satisfied"
	ReasonProhibitedObservation     AirworthinessReason = "airworthiness_prohibited_observation"
	ReasonObservationRequiresReview AirworthinessReason = "airworthiness_observation_requires_review"
	ReasonCoverageIncomplete        AirworthinessReason = "airworthiness_coverage_incomplete"
	ReasonCertificationIncomplete   AirworthinessReason = "airworthiness_certification_incomplete"
	ReasonEvidenceInvalid           AirworthinessReason = "airworthiness_evidence_invalid"
	ReasonRestrictedArchive         AirworthinessReason = "airworthiness_restricted_archive_not_playout"
)

type AirworthinessEffect string

const (
	EffectReject AirworthinessEffect = "reject"
	EffectReview AirworthinessEffect = "review"
)

type AirworthinessTrigger struct {
	ObservationID  string                `json:"observationId"`
	Axis           AirworthinessAxis     `json:"axis"`
	Flag           AirworthinessFlag     `json:"flag"`
	Severity       AirworthinessSeverity `json:"severity"`
	Context        AirworthinessContext  `json:"context"`
	StartMS        int64                 `json:"startMs"`
	EndMS          int64                 `json:"endMs"`
	EvidenceSHA256 string                `json:"evidenceSha256"`
	Effect         AirworthinessEffect   `json:"effect"`
}

// AirworthinessDecision is the path-free, public result. It contains only closed values
// and evidence digests; restricted text and model prose cannot enter it.
type AirworthinessDecision struct {
	SchemaVersion     int                    `json:"schemaVersion"`
	ContractVersion   string                 `json:"contractVersion"`
	SubjectSHA256     string                 `json:"subjectSha256"`
	Profile           AirworthinessProfile   `json:"profile"`
	PolicyVersion     string                 `json:"policyVersion"`
	VocabularyVersion string                 `json:"vocabularyVersion"`
	AuthoritySHA256   string                 `json:"authoritySha256"`
	Verdict           AirworthinessVerdict   `json:"verdict"`
	ReasonCodes       []AirworthinessReason  `json:"reasonCodes"`
	ObservedFlags     []AirworthinessFlag    `json:"observedFlags"`
	Triggers          []AirworthinessTrigger `json:"triggers"`
	HeldAxes          []AirworthinessAxis    `json:"heldAxes"`
	EvidenceSHA256s   []string               `json:"evidenceSha256s"`
	SHA256            string                 `json:"sha256"`
}
