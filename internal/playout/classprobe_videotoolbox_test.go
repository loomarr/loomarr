//go:build !windows

package playout

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

func TestVideoToolboxFallbackProbe_NormalizesEncoderOverride(t *testing.T) {
	clip, ok := fallbackProbeClip(ClassProbeConfig{Encoder: Encoder("hevc_videotoolbox")}, probeClips[0])
	if !ok || !clip.source().Interlaced {
		t.Fatal("HEVC VideoToolbox override omitted the CPU-deinterlace measurement")
	}
	pipe, err := Build(HostFor(Encoder("hevc_videotoolbox"), true, GPUFilters{}), clip.source(), testOutput)
	if err != nil || !strings.HasPrefix(pipe.VideoFilter, "bwdif=mode=send_frame,scale=") {
		t.Fatalf("override probe does not exercise the live CPU-deinterlace graph: %+v %v", pipe, err)
	}
}

func TestProbeClassCosts_VideoToolboxFallbackPricesAdmission(t *testing.T) {
	bin := execfixture.POSIX(t, "ffmpeg", `
step=1000000
case "$*" in *bwdif*) step=100000 ;; esac
i=0
while :; do
  i=$((i+step))
  echo out_time_us=$i
  echo progress=continue
  sleep 0.05
done`)
	res := probeVideoToolboxSDR(t, bin)
	key := CostKey{Class: ClassSDR, Height: 1080}
	cost, ok := res.Costs[key]
	if !ok || cost.Speed >= 3 || !cost.ConservativeCPU {
		t.Fatalf("fallback was not measured in the class envelope: %+v", res)
	}
	facts := BudgetFacts{Hardware: true, CPUAllowance: 1, Rungs: []int{1080}, Costs: res.Costs}
	budget := NewResourceBudget(func() BudgetFacts { return facts })
	lease, err := budget.Reserve(AdmitRequest{Class: ClassSDR})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if _, err := budget.Reserve(AdmitRequest{Class: ClassSDR}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("second stream borrowed progressive-only throughput: %v", err)
	}
}

func TestProbeClassCosts_VideoToolboxFallbackFailureLeavesNoCheapCell(t *testing.T) {
	bin := execfixture.POSIX(t, "ffmpeg", `
case "$*" in *bwdif*) echo 'fallback failed' >&2; exit 1 ;; esac
i=0
while :; do
  i=$((i+1000000))
  echo out_time_us=$i
  echo progress=continue
  sleep 0.05
done`)
	res := probeVideoToolboxSDR(t, bin)
	if len(res.Costs) != 0 || !strings.Contains(strings.Join(res.Failures, ";"), "interlaced fallback: fallback failed") {
		t.Fatalf("failed fallback retained cheap progressive measurements: %+v", res)
	}
}

func probeVideoToolboxSDR(t *testing.T, bin string) ClassProbeResult {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, probeClips[0].fileName()), []byte("owned fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ProbeClassCosts(t.Context(), ClassProbeConfig{
		FFmpeg: bin, ClipDir: dir, Encoder: EncoderVideoToolbox,
		Outputs: []Profile{{Width: 1920, Height: 1080, Framerate: 25}},
		Classes: []StreamClass{ClassSDR},
	})
}
