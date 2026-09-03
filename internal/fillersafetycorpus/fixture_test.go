package fillersafetycorpus

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillersafety"
)

type vctkFixture struct {
	root, authorityPath, seedPath, output string
	authority                             VCTKReleaseAuthority
	config                                PrepareVCTKConfig
	wrapper                               *fakeWrapper
}

type fakeWrapper struct {
	calls         int
	failAt        int
	identityCalls int
	driftIdentity bool
}

func (wrapper *fakeWrapper) Identity(context.Context) (fillersafety.ToolIdentity, fillersafety.ToolIdentity, string, error) {
	wrapper.identityCalls++
	ffmpegHash := fixtureSHA(800)
	if wrapper.driftIdentity && wrapper.identityCalls > 1 {
		ffmpegHash = fixtureSHA(801)
	}
	return fillersafety.ToolIdentity{Version: "ffmpeg version fixture", BinarySHA256: ffmpegHash},
		fillersafety.ToolIdentity{Version: "ffprobe version fixture", BinarySHA256: fixtureSHA(802)},
		hashBytes([]byte(VCTKNeutralVideoRecipe)), nil
}

func (wrapper *fakeWrapper) Wrap(_ context.Context, input, output string) (wrappedMedia, error) {
	wrapper.calls++
	if wrapper.failAt == wrapper.calls {
		return wrappedMedia{}, fmt.Errorf("fixture failure")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		return wrappedMedia{}, err
	}
	raw = append([]byte("wrapped-audiovisual:"), raw...)
	if err := os.WriteFile(output, raw, 0o600); err != nil {
		return wrappedMedia{}, err
	}
	return wrappedMedia{SHA256: bytesSHA(raw), Bytes: int64(len(raw)), DurationMS: 2_000}, nil
}

func newVCTKFixture(t *testing.T) *vctkFixture {
	t.Helper()
	root := t.TempDir()
	preparedAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	license := writeAuthorityFile(t, root, "LICENSE.txt", []byte("CC BY 4.0 fixture"))
	readme := writeAuthorityFile(t, root, "README.txt", []byte("VCTK 0.92 fixture"))
	rights := writeAuthorityFile(t, root, "rights-review.json", []byte("private rights review fixture"))
	authority := VCTKReleaseAuthority{
		SchemaVersion: VCTKReleaseSchemaVersion, ContractVersion: VCTKReleaseContractVersion,
		ReleaseID: VCTKReleaseID, ReleaseRecordURL: VCTKReleaseRecordURL,
		ArchiveSHA256: fixtureSHA(1), ArchiveBytes: 10_000, LicenseID: VCTKLicenseID,
		License: license, Readme: readme, RightsReviewEvidence: rights,
		RightsReviewerID: "rights-reviewer", RightsReviewedAt: preparedAt.Add(-2 * time.Hour),
		RightsContract: fixtureRightsContract(preparedAt),
	}
	for index := 0; index < 100; index++ {
		speaker := fmt.Sprintf("p%03d", 200+index)
		authority.Members = append(authority.Members, writeVCTKMember(t, root, speaker, speaker+"_001", "mic1", index))
	}
	// A second microphone for one utterance shares its transcript, as the real
	// release does. It must not inflate either the family or verified-byte count.
	alternate := writeVCTKMember(t, root, "p299", "p299_001", "mic2", 100)
	alternate.Transcript = authority.Members[len(authority.Members)-1].Transcript
	authority.Members = append(authority.Members, alternate)
	slices.SortFunc(authority.Members, func(a, b VCTKMember) int { return strings.Compare(vctkMemberKey(a), vctkMemberKey(b)) })
	authorityPath := filepath.Join(root, "release-authority.json")
	writePrivateJSONFixture(t, authorityPath, authority)
	seedPath := filepath.Join(root, "seed.bin")
	if err := os.WriteFile(seedPath, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := &vctkFixture{
		root: root, authorityPath: authorityPath, seedPath: seedPath, output: filepath.Join(root, "prepared"),
		authority: authority, wrapper: &fakeWrapper{},
	}
	fixture.config = PrepareVCTKConfig{
		ReleaseAuthorityPath: authorityPath, ReleaseRoot: root, SeedPath: seedPath,
		FFmpegPath: "fixture-ffmpeg", FFprobePath: "fixture-ffprobe", PolicySHA256: fixtureSHA(900),
		Implementation: "spoken-safety-evaluator-v1", PreparedAt: preparedAt, ExpectedSpeakers: 100,
		MaximumInputBytes: 1 << 20, MaximumOutputBytes: 1 << 20, MaximumWallTime: time.Minute,
		OutputDirectory: fixture.output,
	}
	return fixture
}

func writeVCTKMember(t *testing.T, root, speaker, utterance, microphone string, seed int) VCTKMember {
	t.Helper()
	prefix := filepath.ToSlash(filepath.Join("members", speaker, utterance+"_"+microphone))
	return VCTKMember{
		SpeakerID: speaker, UtteranceID: utterance, Microphone: microphone, Locale: "en-GB",
		Audio:             writeAuthorityFile(t, root, prefix+".flac", []byte(fmt.Sprintf("real speech fixture %03d", seed))),
		Transcript:        writeAuthorityFile(t, root, prefix+".txt", []byte(fmt.Sprintf("harmless transcript fixture %03d\n", seed))),
		ScreeningEvidence: writeAuthorityFile(t, root, prefix+".screen", []byte(fmt.Sprintf("screened fixture %03d", seed))),
	}
}

func writeAuthorityFile(t *testing.T, root, relative string, raw []byte) FileAuthority {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return FileAuthority{Path: relative, SHA256: bytesSHA(raw), Bytes: int64(len(raw))}
}

func writePrivateJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureRightsContract(_ time.Time) fillercorpus.HoldoutRightsContract {
	return fillercorpus.HoldoutRightsContract{
		SchemaVersion: fillercorpus.HoldoutRightsContractSchemaVersion,
		AgreementID:   "vctk-cc-by-review", AgreementSHA256: fixtureSHA(10),
		ScheduleID: "vctk-preparation", ScheduleSHA256: fixtureSHA(11),
		SignerAuthorityStatus: fillercorpus.RightsStatusCleared, SignerAuthorityEvidenceSHA256: fixtureSHA(12),
		ProcessorID: "openrouter-pinned-routes", ProcessorTermsSHA256: fixtureSHA(13),
		Grants: fillercorpus.HoldoutRightsGrants{CommercialEvaluation: true, CopyAndStorage: true, TechnicalModification: true, EvidenceExtraction: true, ProviderTransfer: true},
		EmbeddedRights: fillercorpus.EmbeddedRightsStatus{
			Music: fillercorpus.RightsStatusNotPresent, PerformersAndVoices: fillercorpus.RightsStatusCleared,
			StockAndArtwork: fillercorpus.RightsStatusNotPresent, Trademarks: fillercorpus.RightsStatusNotPresent,
			PrivacyAndPublicity: fillercorpus.RightsStatusCleared, Locations: fillercorpus.RightsStatusNotPresent,
		},
		EmbeddedRightsEvidenceSHA256: fixtureSHA(14), RedistributionScope: fillercorpus.RedistributionExternalOnly,
		Territory: fillercorpus.RightsTerritoryWorldwide, Term: fillercorpus.RightsTermPerpetualIrrevocable,
		Withdrawal: fillercorpus.RightsWithdrawalDefectRetirement,
	}
}

func bytesSHA(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }
func fixtureSHA(seed int) string { return fmt.Sprintf("%064x", seed) }
