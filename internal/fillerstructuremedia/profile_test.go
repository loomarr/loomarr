package fillerstructuremedia

import (
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/encode"
)

// The digests are what an authority pins: a change to either recipe is a new profile that needs its
// own certification, never a silent edit of a certified one.
func TestProfileDigestsArePinned(t *testing.T) {
	for family, want := range map[encode.Family]string{
		encode.FamilySoftware: "356cc649eec90ae2bf85b50ba2f817e3abdefea34c7bde80af018e6d8c20d728",
		encode.FamilyNVENC:    "5b548b9597444b9d8f35e74bb428a8c413f157d6faa84a27c9a9a693efc96503",
	} {
		if profile, _ := ProfileFor(family); profile.SHA256 != want {
			t.Errorf("%s profile digest = %s", family, profile.SHA256)
		}
	}
}

func TestProfilePinsCompleteAssessmentRecipe(t *testing.T) {
	profile := SoftwareProfile()
	if profile.SHA256 != ProfileSHA256(profile) || profile.SHA256 == "" || profile.Family != encode.FamilySoftware ||
		profile.PartRecipeSHA256 == "" || profile.ConcatRecipeSHA256 == "" ||
		profile.MaximumVideoBytes != 64<<20 || profile.MaximumTimelineDriftMS != 1_000 {
		t.Fatalf("invalid software profile: %+v", profile)
	}
	if second := SoftwareProfile(); !reflect.DeepEqual(profile, second) {
		t.Fatalf("software profile drifted: first=%+v second=%+v", profile, second)
	}
	profile.Width++
	if profile.SHA256 == ProfileSHA256(profile) {
		t.Fatal("profile mutation retained its content identity")
	}
}

// #2121: each family with a measured cut level is its own profile, and only those resolve. A
// certification pins one digest, so a family that shared a digest with another would let one
// certification vouch for a picture nobody reviewed.
func TestEachCutFamilyIsItsOwnProfile(t *testing.T) {
	software := SoftwareProfile()
	nvenc, ok := ProfileFor(encode.FamilyNVENC)
	if !ok || nvenc.Family != encode.FamilyNVENC || nvenc.SHA256 == software.SHA256 ||
		nvenc.PartRecipeSHA256 == software.PartRecipeSHA256 || nvenc.ConcatRecipeSHA256 != software.ConcatRecipeSHA256 {
		t.Fatalf("software %+v, NVENC %+v", software, nvenc)
	}
	for _, want := range []Profile{software, nvenc} {
		if got, ok := ProfileBySHA256(want.SHA256); !ok || got != want || !Supported(got) {
			t.Fatalf("digest %s resolved to %+v, %v", want.SHA256, got, ok)
		}
	}
	for _, family := range []encode.Family{encode.FamilyVAAPI, encode.FamilyVideoToolbox, encode.FamilyGeneric, ""} {
		if _, ok := ProfileFor(family); ok {
			t.Errorf("family %q has a cut profile without a measured level", family)
		}
	}
	if _, ok := ProfileBySHA256(strings.Repeat("e", 64)); ok {
		t.Fatal("an unknown digest resolved")
	}
	forged := nvenc
	forged.Family = encode.FamilySoftware
	forged.SHA256 = ProfileSHA256(forged)
	if Supported(forged) {
		t.Fatal("the NVENC recipe passed as the software profile")
	}
}

func TestCutFamilyIsTheHostsOwnOnlyWhenMeasured(t *testing.T) {
	for _, tc := range []struct {
		host encode.Encoder
		want encode.Family
	}{
		{encode.EncoderNVENC, encode.FamilyNVENC},
		{encode.EncoderSoftware, encode.FamilySoftware},
		{encode.EncoderVAAPI, encode.FamilySoftware},
		{encode.EncoderVideoToolbox, encode.FamilySoftware},
	} {
		if got := CutFamily(encode.HostFor(tc.host, false, encode.GPUFilters{})); got != tc.want {
			t.Errorf("host %s cuts on %s, want %s", tc.host, got, tc.want)
		}
	}
}

func TestPartArgumentsPinJoinCompatibleProfile(t *testing.T) {
	for _, family := range cutFamilies {
		profile, _ := ProfileFor(family)
		arguments := strings.Join(PartArguments(profile, "source.mp4", 1_000, 2_000, "part.mp4"), " ")
		for _, required := range []string{
			"-ss 1.000 -i source.mp4 -t 2.000", "fps=30:start_time=0", "scale=w=960:h=720:force_original_aspect_ratio=decrease",
			"pad=960:720", "-pix_fmt yuv420p", "-ar 48000", "-ac 2", "-video_track_timescale 90000",
			"-threads 1", "-fflags +bitexact", "creation_time=", "encoder=",
		} {
			if !strings.Contains(arguments, required) {
				t.Fatalf("%s structure part arguments omit %q: %s", family, required, arguments)
			}
		}
	}
	software := strings.Join(PartArguments(SoftwareProfile(), "s.mp4", 0, 1_000, "p.mp4"), " ")
	nvenc, _ := ProfileFor(encode.FamilyNVENC)
	gpu := strings.Join(PartArguments(nvenc, "s.mp4", 0, 1_000, "p.mp4"), " ")
	if !strings.Contains(software, "-c:v libx264 -preset medium -profile:v high -crf 23") ||
		!strings.Contains(gpu, "-c:v h264_nvenc") || !strings.Contains(gpu, "-cq 29") {
		t.Fatalf("software cut %s\nNVENC cut %s", software, gpu)
	}
}

func TestConcatRecipePinsMetadataFreeCopy(t *testing.T) {
	arguments := strings.Join(concatArgumentTemplate(), " ")
	for _, required := range []string{"-safe 1", "-map_metadata -1", "-map_chapters -1", "-c copy", "-fflags +bitexact", "creation_time=", "encoder="} {
		if !strings.Contains(arguments, required) {
			t.Fatalf("concat arguments omit %q: %s", required, arguments)
		}
	}
}

func TestProfileDigestChangesWithRecipe(t *testing.T) {
	profile := SoftwareProfile()
	if profile.PartRecipeSHA256 == recipeSHA256(append(partArgumentTemplate(encode.FamilySoftware), "-an")) {
		t.Fatal("recipe mutation retained its content identity")
	}
}
