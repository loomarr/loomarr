package window

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

func TestParseDirectVideoResponseProjectsWindowLocalTimelineOntoSource(t *testing.T) {
	plan, err := NewPlan(sourceFixture(300_000), softwareMedia)
	if err != nil {
		t.Fatal(err)
	}
	set := mediaSetForPlan(t, plan)
	window := set.Plan.Windows[1]
	duration := window.MediaEndMS - window.MediaStartMS
	raw := `{"segments":[{"endMs":15000,"role":"commercial","decisiveAtMs":[1000],"reason":"offer"},{"endMs":` + integerString(duration) + `,"role":"promo","decisiveAtMs":[20000],"reason":"promotion"}]}`
	segments, err := ParseDirectVideoResponse(set, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []structure.Segment{
		{StartMS: window.MediaStartMS, EndMS: window.MediaStartMS + 15_000, Role: structure.RoleCommercial},
		{StartMS: window.MediaStartMS + 15_000, EndMS: window.MediaEndMS, Role: structure.RolePromo},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("segments=%+v, want %+v", segments, want)
	}
}

func TestParseDirectVideoResponseRejectsIncompleteWindowTimeline(t *testing.T) {
	plan, err := NewPlan(sourceFixture(300_000), softwareMedia)
	if err != nil {
		t.Fatal(err)
	}
	set := mediaSetForPlan(t, plan)
	if _, err := ParseDirectVideoResponse(set, 1, `{"segments":[{"endMs":1000,"role":"commercial","decisiveAtMs":[500],"reason":"offer"}]}`); err == nil {
		t.Fatal("incomplete window timeline was accepted")
	}
}

func TestDirectVideoPromptAndSchemaIdentityAreStableAndDurationBound(t *testing.T) {
	t.Parallel()
	wantDigest := regexp.MustCompile(`^[0-9a-f]{64}$`)
	prompt := DirectVideoPromptSHA256(1_000)
	schema := DirectVideoSchemaSHA256(1_000)
	// The certified authority pins this digest; a schema edit is a new certification.
	if got := DirectVideoSchemaSHA256(10_000); got != "7842e47ce17bc2335928d67e661ca1cbccf6b02e60018200f09e0ab68b9d4174" {
		t.Fatalf("schema digest drifted: %s", got)
	}
	if !wantDigest.MatchString(prompt) || !wantDigest.MatchString(schema) ||
		prompt != DirectVideoPromptSHA256(1_000) || schema != DirectVideoSchemaSHA256(1_000) ||
		prompt == DirectVideoPromptSHA256(2_000) || schema == DirectVideoSchemaSHA256(2_000) {
		t.Fatalf("prompt=%q schema=%q", prompt, schema)
	}
}

func TestDecodeDirectVideoResponseNormalizesAndCoalescesProgramme(t *testing.T) {
	t.Parallel()
	raw := `{"segments":[{"endMs":4500,"role":"programme_fragment","decisiveAtMs":[4000,1000],"reason":"title"},{"endMs":20000,"role":"programme_fragment","decisiveAtMs":[15000],"reason":"opening"},{"endMs":80000,"role":"commercial","decisiveAtMs":[70000,25000],"reason":"offer"},{"endMs":99555,"role":"programme_fragment","decisiveAtMs":[98000],"reason":"resumes"}]}`
	got, err := decodeDirectVideoResponse(raw, 99_555)
	if err != nil {
		t.Fatal(err)
	}
	want := []structure.Segment{
		{StartMS: 0, EndMS: 20_000, Role: structure.RoleProgrammeFragment},
		{StartMS: 20_000, EndMS: 80_000, Role: structure.RoleCommercial},
		{StartMS: 80_000, EndMS: 99_555, Role: structure.RoleProgrammeFragment},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments=%+v, want %+v", got, want)
	}
}

func TestDecodeDirectVideoResponsePreservesAdjacentFiller(t *testing.T) {
	t.Parallel()
	raw := `{"segments":[{"endMs":500,"role":"commercial","decisiveAtMs":[200],"reason":"first"},{"endMs":1000,"role":"commercial","decisiveAtMs":[700],"reason":"second"}]}`
	got, err := decodeDirectVideoResponse(raw, 1_000)
	if err != nil || len(got) != 2 {
		t.Fatalf("segments=%+v err=%v", got, err)
	}
}

func TestDecodeDirectVideoResponseRejectsIncompleteOrUnresolvedEvidence(t *testing.T) {
	t.Parallel()
	tests := []string{
		`{"segments":[{"endMs":999,"role":"commercial","decisiveAtMs":[200],"reason":"offer"}]}`,
		`{"segments":[{"endMs":1000,"role":"commercial","decisiveAtMs":[],"reason":"offer"}]}`,
		`{"segments":[{"endMs":1000,"role":"commercial","decisiveAtMs":[1001],"reason":"offer"}]}`,
		`{"segments":[{"endMs":1000,"role":"commercial","decisiveAtMs":[200],"reason":"offer","extra":true}]}`,
		`{"segments":[{"endMs":1000,"role":"commercial","decisiveAtMs":[200],"reason":"offer"}]} {}`,
		`{"segments":[{"endMs":1000,"role":"unknown","decisiveAtMs":[200],"reason":"offer"}]}`,
		`{"segments":[{"endMs":1000,"role":"commercial","decisiveAtMs":[200],"reason":"  "}]}`,
	}
	for _, raw := range tests {
		if _, err := decodeDirectVideoResponse(raw, 1_000); err == nil {
			t.Fatalf("invalid response accepted: %s", raw)
		}
	}
}

func TestDecodeDirectVideoResponseEnforcesLocalSegmentCeiling(t *testing.T) {
	t.Parallel()
	response := directVideoResponse{Segments: make([]directVideoResponseSegment, DirectVideoMaximumSegments+1)}
	for index := range response.Segments {
		response.Segments[index] = directVideoResponseSegment{
			EndMS: int64(index + 1), Role: string(structure.RoleCommercial),
			DecisiveAtMS: []int64{int64(index)}, Reason: "bounded item",
		}
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeDirectVideoResponse(string(raw), int64(len(response.Segments))); err == nil {
		t.Fatal("response accepted more than the local segment ceiling")
	}
}

func integerString(value int64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
