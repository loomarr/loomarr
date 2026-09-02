package fillercorpus

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHoldoutRightsHoldReasonsFailClosedByIndependentAxis(t *testing.T) {
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	valid := func() *HoldoutRightsContract {
		return &HoldoutRightsContract{
			SchemaVersion: HoldoutRightsContractSchemaVersion,
			AgreementID:   "agreement-v1", AgreementSHA256: strings.Repeat("a", 64), ScheduleID: "schedule-v1", ScheduleSHA256: strings.Repeat("b", 64),
			SignerAuthorityStatus: RightsStatusCleared, SignerAuthorityEvidenceSHA256: strings.Repeat("c", 64), ProcessorID: "processor-v1", ProcessorTermsSHA256: strings.Repeat("d", 64),
			Grants:                       HoldoutRightsGrants{CommercialEvaluation: true, CopyAndStorage: true, TechnicalModification: true, EvidenceExtraction: true, ProviderTransfer: true},
			EmbeddedRights:               EmbeddedRightsStatus{Music: RightsStatusNotPresent, PerformersAndVoices: RightsStatusCleared, StockAndArtwork: RightsStatusNotPresent, Trademarks: RightsStatusCleared, PrivacyAndPublicity: RightsStatusCleared, Locations: RightsStatusNotPresent},
			EmbeddedRightsEvidenceSHA256: strings.Repeat("e", 64), RedistributionScope: RedistributionExternalOnly, Territory: RightsTerritoryWorldwide, Term: RightsTermPerpetualIrrevocable, Withdrawal: RightsWithdrawalDefectRetirement,
		}
	}
	if reasons := HoldoutRightsHoldReasons(valid(), at); len(reasons) != 0 {
		t.Fatalf("valid contract held by %v", reasons)
	}
	tests := map[string]struct {
		mutate func(*HoldoutRightsContract)
		want   string
	}{
		"schedule digest":   {func(v *HoldoutRightsContract) { v.ScheduleSHA256 = "bad" }, "schedule_identity_invalid"},
		"signer unknown":    {func(v *HoldoutRightsContract) { v.SignerAuthorityStatus = RightsStatusUnknown }, "signer_authority_unconfirmed"},
		"provider grant":    {func(v *HoldoutRightsContract) { v.Grants.ProviderTransfer = false }, "grant_provider_transfer_missing"},
		"embedded conflict": {func(v *HoldoutRightsContract) { v.EmbeddedRights.Music = RightsStatusConflicting }, "embedded_rights_music_unresolved"},
		"territory":         {func(v *HoldoutRightsContract) { v.Territory = "us_only" }, "territory_not_worldwide"},
		"expired": {func(v *HoldoutRightsContract) {
			expired := at.Add(-time.Minute)
			v.Term, v.ExpiresAt = RightsTermExpires, &expired
		}, "term_expired"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			contract := valid()
			test.mutate(contract)
			if reasons := HoldoutRightsHoldReasons(contract, at); !slices.Contains(reasons, test.want) {
				t.Fatalf("reasons = %v; want %q", reasons, test.want)
			}
		})
	}
}
