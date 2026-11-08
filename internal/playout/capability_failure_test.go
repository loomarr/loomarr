package playout

import "testing"

func TestTrialFailureNamesTheDriverAnOldHostNeeds(t *testing.T) {
	// Verbatim from an A16 with driver 580 and ffmpeg n9.0.2 (2026-11-08).
	stderr := "[h264_nvenc @ 0x64090f882c40] Cannot load libnvidia-encode.so.1\n" +
		"[h264_nvenc @ 0x64090f882c40] The minimum required Nvidia driver for nvenc is 610.00 or newer\n" +
		"[h264_nvenc @ 0x64090f882c40] Nvenc unloaded\n"

	got := trialFailure(stderr)

	want := "[h264_nvenc @ 0x64090f882c40] The minimum required Nvidia driver for nvenc is 610.00 or newer"
	if got != want {
		t.Fatalf("trialFailure = %q, want %q", got, want)
	}
}

func TestTrialFailureIsOtherwiseTheFirstLine(t *testing.T) {
	got := trialFailure("  No VA display found for device /dev/dri/renderD128.\nlater noise\n")

	if got != "No VA display found for device /dev/dri/renderD128." {
		t.Fatalf("trialFailure = %q", got)
	}
}
