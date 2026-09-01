package fillerreference

import (
	"testing"
	"time"
)

func TestCompareDuplicateSequencesToleratesLeaderAndTrailer(t *testing.T) {
	unit := []uint64{
		0x0f0f0f0f0f0f0f0f, 0x1f0f0f0f0f0f0f0f, 0x2f0f0f0f0f0f0f0f,
		0x3f0f0f0f0f0f0f0f, 0x4f0f0f0f0f0f0f0f, 0x5f0f0f0f0f0f0f0f,
		0x6f0f0f0f0f0f0f0f, 0x7f0f0f0f0f0f0f0f, 0x8f0f0f0f0f0f0f0f,
		0x9f0f0f0f0f0f0f0f, 0xaf0f0f0f0f0f0f0f, 0xbf0f0f0f0f0f0f0f,
		0xcf0f0f0f0f0f0f0f, 0xdf0f0f0f0f0f0f0f, 0xef0f0f0f0f0f0f0f,
		0xff0f0f0f0f0f0f0f,
	}
	a := append([]uint64{0x3333333333333333, 0x5555555555555555}, unit...)
	b := append(append([]uint64{}, unit...), 0x7777777777777777, 0xaaaaaaaaaaaaaaaa)

	got := CompareDuplicateSequences(a, b)
	if !got.Related || got.MatchedFrames < len(unit) || got.Coverage < 0.85 {
		t.Fatalf("comparison = %+v, want related sustained sequence", got)
	}
}

func TestCompareDuplicateSequencesRejectsSparseOrFlatAgreement(t *testing.T) {
	flat := make([]uint64, 30)
	if got := CompareDuplicateSequences(flat, flat); got.Related {
		t.Fatalf("flat comparison = %+v, want unrelated", got)
	}

	a := make([]uint64, 30)
	b := make([]uint64, 30)
	for i := range a {
		a[i] = uint64(i+8) * 0x0101010101010101
		b[i] = ^a[i]
	}
	copy(b[:5], a[:5])
	if got := CompareDuplicateSequences(a, b); got.Related {
		t.Fatalf("sparse comparison = %+v, want unrelated", got)
	}
}

func TestCompareDuplicateSequencesRequiresEnoughEvidence(t *testing.T) {
	a := []uint64{0x1111111111111111, 0x2222222222222222, 0x3333333333333333}
	if got := CompareDuplicateSequences(a, a); got.Related {
		t.Fatalf("short comparison = %+v, want unrelated", got)
	}
}

func TestBuildFamilyAuditReportsTransitiveNonCliqueWithoutChoosingRendition(t *testing.T) {
	base := make([]uint64, 20)
	for i := range base {
		base[i] = uint64(i+5) * 0x0102040810204081
	}
	a := append([]uint64{}, base...)
	b := append([]uint64{}, base...)
	c := append([]uint64{}, base...)
	for i := 0; i < 5; i++ {
		a[i] ^= 0xffffffffffffffff
		c[len(c)-1-i] ^= 0xffffffffffffffff
	}
	when := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	got := BuildFamilyAudit("audit-sha", []FamilyFingerprint{
		{CaseID: "c", FrameHashes: c}, {CaseID: "a", FrameHashes: a}, {CaseID: "b", FrameHashes: b},
	}, when)
	if got.Summary.DuplicateFamilies != 1 || got.Summary.NonCliqueFamilies != 1 || len(got.Families) != 1 {
		t.Fatalf("summary=%+v families=%+v", got.Summary, got.Families)
	}
	if got.Families[0].CompleteClique || got.Families[0].PreferredCase != "" {
		t.Fatalf("family=%+v, want unresolved non-clique", got.Families[0])
	}
}

func TestCompareAudioEnvelopesFindsShiftedLevelScaledCopy(t *testing.T) {
	a := make([]uint32, 120)
	for i := range a {
		a[i] = uint32((i*37)%101 + 20)
	}
	b := make([]uint32, 0, 125)
	b = append(b, 4, 8, 12, 16, 20)
	for _, value := range a {
		b = append(b, value*3+7)
	}
	got := CompareAudioEnvelopes(a, b)
	if !got.Related || got.Correlation < 0.999 || got.OffsetBins != 5 {
		t.Fatalf("audio comparison=%+v, want shifted related track", got)
	}
}

func TestCompareAudioEnvelopesRejectsSilence(t *testing.T) {
	silence := make([]uint32, 100)
	if got := CompareAudioEnvelopes(silence, silence); got.Related {
		t.Fatalf("silence comparison=%+v, want unrelated", got)
	}
}
