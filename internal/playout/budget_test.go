package playout

import (
	"context"
	"errors"
	"testing"
	"time"
)

// nvencFacts is the dev GeForce shape from phase 0: 0.034 cores and ~19x per 1080p SDR stream, a
// 12-session driver cap, and the 1-core GPU-host allowance.
func nvencFacts() BudgetFacts {
	return BudgetFacts{
		Hardware: true, SessionLimit: 12, CPUAllowance: 1,
		Rungs: []int{1080, 1080, 720, 720},
		Costs: map[CostKey]ClassCost{
			{Class: ClassSDR, Height: 1080}:   {Speed: 19, CPUCores: 0.034},
			{Class: ClassSDR, Height: 720}:    {Speed: 30, CPUCores: 0.025},
			{Class: ClassHDR4K, Height: 1080}: {Speed: 2.4, CPUCores: 0.12},
			{Class: ClassHDR4K, Height: 720}:  {Speed: 3.6, CPUCores: 0.1},
		},
	}
}

func admitN(t *testing.T, b *ResourceBudget, class StreamClass, n int) []*Lease {
	t.Helper()
	var out []*Lease
	for i := 0; i < n; i++ {
		l, err := b.Admit(context.Background(), AdmitRequest{Class: class})
		if err != nil {
			t.Fatalf("admission %d of %s: %v", i+1, class, err)
		}
		out = append(out, l)
	}
	return out
}

func TestResourceBudget_CapacityIsTheMinimumOfThreeLimits(t *testing.T) {
	cases := []struct {
		name  string
		facts func() BudgetFacts
		class StreamClass
		want  int
	}{
		// 19x / 1.2 = 15.8 GPU streams, 1 / 0.034 = 29 CPU streams, 12 sessions: the session cap binds.
		{"nvenc session cap binds", nvencFacts, ClassSDR, 12},
		// 1 / 0.0875 = 11.4 CPU streams (phase 0 Arc, before the fMP4-forward packager).
		{"cpu allowance binds", func() BudgetFacts {
			f := nvencFacts()
			f.SessionLimit = 0
			f.Costs[CostKey{Class: ClassSDR, Height: 1080}] = ClassCost{Speed: 19, CPUCores: 0.0875}
			return f
		}, ClassSDR, 11},
		// 2.4x / 1.2 = 2 GPU streams of 4K HDR.
		{"gpu throughput binds", nvencFacts, ClassHDR4K, 2},
		// Software: no GPU term; 3 cores / 1.35 = 2 streams at 1080p (maintainer: 2 at 1080p).
		{"software host", func() BudgetFacts {
			return BudgetFacts{CPUAllowance: 3, Rungs: []int{1080}, Costs: map[CostKey]ClassCost{
				{Class: ClassSDR, Height: 1080}: {Speed: 3.1, CPUCores: 1.35},
			}}
		}, ClassSDR, 2},
		{"operator cap lowers", func() BudgetFacts { f := nvencFacts(); f.OperatorCap = 3; return f }, ClassSDR, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewResourceBudget(tc.facts)
			if got := b.Snapshot().Classes[tc.class].Capacity; got != tc.want {
				t.Fatalf("reported capacity = %d, want %d", got, tc.want)
			}
			// Only the top rung at full quality: admission must refuse exactly at the reported capacity
			// (a software host degrades past it instead: TestResourceBudget_SoftwareHost*).
			f := tc.facts()
			f.Rungs = f.Rungs[:1]
			b = NewResourceBudget(func() BudgetFacts { return f })
			admitN(t, b, tc.class, tc.want)
			if _, err := b.Admit(context.Background(), AdmitRequest{Class: tc.class, NoRungDrop: true}); !errors.Is(err, ErrAtCapacity) {
				t.Fatalf("admission past capacity: err = %v, want ErrAtCapacity", err)
			}
		})
	}
}

func TestResourceBudget_DropsARungBeforeRefusing(t *testing.T) {
	facts := nvencFacts()
	facts.SessionLimit = 0
	b := NewResourceBudget(func() BudgetFacts { return facts })
	// Two 4K HDR streams at 1080p use 2 x 1.2/2.4 = the whole GPU.
	admitN(t, b, ClassHDR4K, 1)
	l, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K})
	if err != nil || l.Rung() != 0 {
		t.Fatalf("second HDR stream: rung %v err %v, want top rung", l, err)
	}
	// The third does not fit at 1080p; the 720p rung costs 1.2/3.6 and does not fit either. Free room
	// for exactly one 720p stream by releasing nothing: the GPU is full, so it is refused.
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("full GPU: err = %v, want ErrAtCapacity", err)
	}

	// SDR streams: 15 fit at 1080p (19/1.2), then the 720p rung (30/1.2 = 25) takes the remaining share.
	b = NewResourceBudget(func() BudgetFacts { return facts })
	top := admitN(t, b, ClassSDR, 15)
	for _, l := range top {
		if l.Rung() != 0 {
			t.Fatalf("stream under capacity dropped to rung %d", l.Rung())
		}
	}
	dropped, err := b.Admit(context.Background(), AdmitRequest{Class: ClassSDR})
	if err != nil {
		t.Fatalf("16th stream refused instead of dropping a rung: %v", err)
	}
	if got := facts.Rungs[dropped.Rung()]; got != 720 {
		t.Fatalf("16th stream rung height = %d, want 720", got)
	}
	// Its rung is pinned for the session's lifetime: freeing capacity does not move it.
	top[0].Release()
	if got := facts.Rungs[dropped.Rung()]; got != 720 {
		t.Fatalf("pinned rung moved to %d after a release", got)
	}
}

func TestResourceBudget_CopyIsFreeAndReleaseReturnsCapacity(t *testing.T) {
	f := nvencFacts()
	f.OperatorCap = 1
	b := NewResourceBudget(func() BudgetFacts { return f })
	held := admitN(t, b, ClassSDR, 1)
	admitN(t, b, ClassCopy, 5)
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassSDR}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("err = %v, want ErrAtCapacity", err)
	}
	held[0].Release()
	held[0].Release() // idempotent
	admitN(t, b, ClassSDR, 1)
	if s := b.Snapshot(); s.InUse.Transcodes != 1 || s.InUse.Sessions != 6 {
		t.Fatalf("in use = %+v, want 1 transcode of 6 sessions", s.InUse)
	}
}

func TestResourceBudget_ReclassFromCopyMustFitButATranscodeNeverStops(t *testing.T) {
	f := nvencFacts()
	f.OperatorCap = 1
	b := NewResourceBudget(func() BudgetFacts { return f })
	a := admitN(t, b, ClassSDR, 1)[0]
	c := admitN(t, b, ClassCopy, 1)[0]
	if c.Reclass(context.Background(), ClassSDR) {
		t.Fatal("a copy session became a transcode past the cap")
	}
	// A watched transcode crossing into an HDR item keeps playing even though it costs more.
	if !a.Reclass(context.Background(), ClassHDR4K) {
		t.Fatal("a live transcode was refused at an item boundary")
	}
	a.Release()
	if !c.Reclass(context.Background(), ClassSDR) {
		t.Fatal("copy → transcode refused with the cap free")
	}
}

// #1505: session admission and the hardware encode pool once counted separately, so a session could
// pass admission and then find the pool full (its programme dropped to software). The ledger is now
// the only count (#1562): a channel packager holds the lease it was admitted with, so the budget is
// full exactly when the packagers hold it, and a stopped packager returns its lease.
func TestPackagerAdmissionHoldsAndReturnsTheLedger(t *testing.T) {
	facts := nvencFacts()
	facts.OperatorCap = 2
	budget := NewResourceBudget(func() BudgetFacts { return facts })
	m := newTestPackagerHLS(t, slowItemSource{}, time.Hour)
	m.WithBudget(budget)

	for _, ch := range []string{"a", "b"} {
		c, release, err := m.acquire(ch, FormatBaseline)
		if err != nil || c == nil {
			t.Fatalf("acquire %s: %v", ch, err)
		}
		defer release()
	}
	if _, _, err := m.acquire("c", FormatBaseline); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("third transcode: err = %v, want ErrAtCapacity", err)
	}
	if budget.Fits(AdmitRequest{Class: ClassSDR}) {
		t.Fatal("the budget has room for a transcode it already gave to packagers")
	}
	m.StopChannel("a")
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if budget.Fits(AdmitRequest{Class: ClassSDR}) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a stopped packager did not return its lease")
		}
	}
}

func TestResourceBudget_LegacyUnitsWithoutMeasurements(t *testing.T) {
	b := NewResourceBudget(func() BudgetFacts { return BudgetFacts{MeasuredCapacity: 2} })
	admitN(t, b, ClassSDR, 2)
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("err = %v, want ErrAtCapacity", err)
	}
	unmeasured := NewResourceBudget(func() BudgetFacts { return BudgetFacts{} })
	admitN(t, unmeasured, ClassSDR, 50) // unmeasured never blocks playout
}

func TestResourceBudget_PlaybackHeadroom(t *testing.T) {
	b := NewResourceBudget(nvencFacts)
	if b.PlaybackNeedsHeadroom() {
		t.Fatal("idle budget reports playback needs headroom")
	}
	l := admitN(t, b, ClassSDR, 1)[0]
	if !b.PlaybackNeedsHeadroom() {
		t.Fatal("a live transcode must make background work yield")
	}
	l.Release()
	if b.PlaybackNeedsHeadroom() {
		t.Fatal("headroom still claimed after the last transcode ended")
	}
}

func TestClassOf(t *testing.T) {
	for _, tc := range []struct {
		f    MediaFormat
		want StreamClass
	}{
		{MediaFormat{VideoCodec: "h264", Width: 1920, Height: 1080, PixelFormat: "yuv420p"}, ClassSDR},
		{MediaFormat{}, ClassSDR},
		{MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 1080, PixelFormat: "yuv420p10le"}, ClassHEVC10},
		{MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084"}, ClassHDR4K},
		{MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, PixelFormat: "yuv420p"}, ClassHDR4K},
	} {
		if got := ClassOf(tc.f); got != tc.want {
			t.Errorf("ClassOf(%+v) = %s, want %s", tc.f, got, tc.want)
		}
	}
}

// Real content corrects the synthetic estimate (#1512): a live programme's CPU per media second
// moves its class's cost, bounded to half..double the probe's measurement so one odd file cannot
// swing admission, and a new probe measurement starts over from its own number.
func TestResourceBudget_LiveCPURefinesTheSyntheticCostWithinBounds(t *testing.T) {
	facts := nvencFacts()
	b := NewResourceBudget(func() BudgetFacts { return facts })
	lease := admitN(t, b, ClassSDR, 1)[0]
	cpu := func() float64 { return b.Snapshot().Classes[ClassSDR].CPUCores }

	lease.ObserveCPU(ClassSDR, 1*time.Second, 5*time.Second) // too short a sample
	if got := cpu(); got != 0.034 {
		t.Fatalf("a 5 s sample moved the cost to %v", got)
	}
	lease.ObserveCPU(ClassSDR, 6*time.Second, 60*time.Second) // 0.1 cores: clamped to 0.068
	if got := cpu(); got <= 0.034 || got > 0.068 {
		t.Fatalf("after one heavy sample CPU = %v, want within (0.034, 0.068]", got)
	}
	for range 50 {
		lease.ObserveCPU(ClassSDR, 6*time.Second, 60*time.Second)
	}
	if got := cpu(); got > 0.068+1e-9 {
		t.Fatalf("refined CPU = %v escaped the 2x bound", got)
	}

	facts.Costs[CostKey{Class: ClassSDR, Height: 1080}] = ClassCost{Speed: 19, CPUCores: 0.05} // re-measured
	if got := cpu(); got != 0.05 {
		t.Fatalf("a new synthetic measurement kept the old refinement: %v", got)
	}
}

// Background work (the boot capacity probe) yields the moment a live transcode is admitted: its
// context ends with ErrYielded inside Reserve, so a tune never waits on it. A copy uses no encoder
// and does not interrupt it; while a transcode runs, new background work starts already yielded.
func TestResourceBudget_BackgroundYieldsTheMomentATranscodeIsReserved(t *testing.T) {
	b := NewResourceBudget(nvencFacts)
	bg, cancel := b.BackgroundContext(t.Context())
	defer cancel()

	copyLease, err := b.Reserve(AdmitRequest{Class: ClassCopy})
	if err != nil {
		t.Fatal(err)
	}
	defer copyLease.Release()
	if bg.Err() != nil {
		t.Fatal("a copy interrupted background work; it holds no encoder")
	}

	live, err := b.Reserve(AdmitRequest{Class: ClassSDR})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(context.Cause(bg), ErrYielded) {
		t.Fatalf("background context after a live transcode was reserved: cause %v, want ErrYielded", context.Cause(bg))
	}
	again, cancelAgain := b.BackgroundContext(t.Context())
	defer cancelAgain()
	if !errors.Is(context.Cause(again), ErrYielded) {
		t.Fatal("background work started while a live transcode runs")
	}

	idle := make(chan error, 1)
	go func() { idle <- b.WaitIdle(t.Context()) }()
	select {
	case <-idle:
		t.Fatal("WaitIdle returned while a live transcode holds a lease")
	case <-time.After(100 * time.Millisecond):
	}
	live.Release()
	select {
	case err := <-idle:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitIdle did not return after the last transcode released")
	}
}

// HDR costs are keyed by tone curve (#1516's playout.tone_curve): choosing BT.2390, which takes a
// CPU hop on Intel, drops HDR capacity instead of overcommitting the CPU; a curve not yet measured
// borrows Hable's cell rather than refusing HDR; a cell stored before curves counts as Hable.
func TestBudgetFacts_HDRCostFollowsTheToneCurve(t *testing.T) {
	facts := BudgetFacts{
		Hardware: true, CPUAllowance: 1, Rungs: []int{720},
		Costs: map[CostKey]ClassCost{
			{Class: ClassSDR, Height: 720}:                    {Speed: 30, CPUCores: 0.02},
			{Class: ClassHDR4K, Height: 720}:                  {Speed: 6, CPUCores: 0.1}, // stored before curves: Hable
			{Class: ClassHDR4K, Height: 720, Curve: "bt2390"}: {Speed: 6, CPUCores: 0.33},
		},
	}
	if got := facts.capacity(ClassHDR4K, 720); got != 5 {
		t.Fatalf("Hable HDR capacity = %d, want 5 (GPU-bound: 6x / 1.2x)", got)
	}
	facts.ToneCurve = "bt2390"
	if got := facts.capacity(ClassHDR4K, 720); got != 3 {
		t.Fatalf("BT.2390 HDR capacity = %d, want 3 (CPU-bound: 1 core / 0.33)", got)
	}
	if got := facts.capacity(ClassSDR, 720); got != 25 {
		t.Errorf("the curve changed SDR capacity: %d", got)
	}
	facts.ToneCurve = "unmeasured-curve"
	if got := facts.capacity(ClassHDR4K, 720); got != 5 {
		t.Errorf("an unmeasured curve's HDR capacity = %d, want Hable's 5 until it is measured", got)
	}
}

// softwareHDRFacts is a CPU-only host whose boot probe measured 4K HDR at 3.6 cores and 0.3x at full
// quality (phase 0b's 4-CPU box), with an SDR cell beside it.
func softwareHDRFacts(allowance float64) BudgetFacts {
	return BudgetFacts{CPUAllowance: allowance, Rungs: []int{720}, Costs: map[CostKey]ClassCost{
		{Class: ClassSDR, Height: 720}:   {Speed: 3.1, CPUCores: 1.35},
		{Class: ClassHDR4K, Height: 720}: {Speed: 0.3, CPUCores: 3.6},
	}}
}

// On a software host admission picks the best ladder rung (#1517) whose measured CPU cost fits
// (#1520): 3.6 cores at full does not fit 3.5, rung 1's 0.95 share (3.42) does.
func TestResourceBudget_SoftwareHostPicksTheBestRungThatFits(t *testing.T) {
	for _, tc := range []struct {
		allowance float64
		want      SoftwareRung
	}{
		{4, RungFull},
		{3.5, RungLight},
		{3, RungKeyframes},
	} {
		b := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(tc.allowance) })
		l, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K})
		if err != nil {
			t.Fatalf("allowance %v: %v", tc.allowance, err)
		}
		if got := l.SoftwareRung(); got != tc.want {
			t.Errorf("allowance %v: rung %s, want %s", tc.allowance, got, tc.want)
		}
		if got, want := b.Snapshot().InUse.CPUCores, 3.6*heavyRungCosts[tc.want]; got < want-1e-9 || got > want+1e-9 {
			t.Errorf("allowance %v: ledger holds %v cores, want the rung's %v", tc.allowance, got, want)
		}
	}
}

// Speed below 1.2x never refuses a class on a software host: the ladder degrades it instead. The
// same cell on a GPU host is still refused, where speed is the GPU's throughput.
func TestResourceBudget_SoftwareHostNeverRefusesForSpeed(t *testing.T) {
	b := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(1) })
	l, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K})
	if err != nil {
		t.Fatalf("a software host refused 4K HDR measured at 0.3x: %v", err)
	}
	if l.SoftwareRung() != RungKeyframes {
		t.Errorf("rung %s, want keyframes-only", l.SoftwareRung())
	}
	gpu := softwareHDRFacts(1)
	gpu.Hardware = true
	if _, err := NewResourceBudget(func() BudgetFacts { return gpu }).Admit(context.Background(), AdmitRequest{Class: ClassHDR4K}); !errors.Is(err, ErrAtCapacity) {
		t.Errorf("a GPU host admitted a class measured at 0.3x: %v", err)
	}
}

// A software host refuses only when even keyframes-only does not fit: too many streams. 1 core
// holds two 4K HDR streams at keyframes-only (0.432 each), not three. Freeing one admits again, and
// an item boundary re-picks the lease's rung from the ledger and allowance as they are then.
func TestResourceBudget_SoftwareHostRefusesOnlyWhenKeyframesDoNotFit(t *testing.T) {
	allowance := 1.0
	b := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(allowance) })
	leases := admitN(t, b, ClassHDR4K, 2)
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("third keyframes-only stream on 1 core: err = %v, want ErrAtCapacity", err)
	}
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K, NoRungDrop: true}); !errors.Is(err, ErrAtCapacity) {
		t.Fatal("NoRungDrop admitted below full quality")
	}
	leases[1].Release()
	sdr, err := b.Admit(context.Background(), AdmitRequest{Class: ClassSDR})
	if err != nil || sdr.SoftwareRung() != RungKeyframes {
		t.Fatalf("SDR beside one HDR stream: rung %v err %v, want keyframes (0.568 cores left)", sdr.SoftwareRung(), err)
	}
	leases[0].Release()
	allowance = 2 // e.g. playout.app_reserve_millicores lowered
	if !sdr.ReclassItem(context.Background(), ClassSDR) || sdr.SoftwareRung() != RungFull {
		t.Fatalf("an item on an idle host stayed on rung %s, want full", sdr.SoftwareRung())
	}
}

// The ladder's live steps re-price the lease (#1520 follow-up). A step down releases the CPU the
// rung no longer needs. A step up must fit the ledger beside the other leases, else the encoder and
// the lease stay where they are; once it fits, the same headroom steps up, and the lease follows.
func TestLease_LiveLadderStepsRepriceTheLease(t *testing.T) {
	b := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(4.1) })
	cores := func() float64 { return b.Snapshot().InUse.CPUCores }
	near := func(got, want float64) bool { return got > want-1e-9 && got < want+1e-9 }

	full := admitN(t, b, ClassHDR4K, 1)[0] // 3.6 cores at full
	if full.SoftwareRung() != RungFull {
		t.Fatalf("first HDR stream on 4.1 cores: rung %s, want full", full.SoftwareRung())
	}
	// Sustained below realtime: the monitor steps full down to rung 1, and the lease follows.
	got := runScript(t, full.NewRungMonitor(RungMonitorConfig{CPUAllowance: 4}), phase{20 * time.Second, 0.85, 0})
	if len(got) == 0 || got[0].rung != RungLight || full.SoftwareRung() != RungLight || !near(cores(), 3.42) {
		t.Fatalf("step down: decisions %+v, lease %s, ledger %.3f cores; want rung 1 at 3.42", got, full.SoftwareRung(), cores())
	}

	small := admitN(t, b, ClassHDR4K, 1)[0] // 0.68 left: keyframes-only (0.432)
	if small.SoftwareRung() != RungKeyframes {
		t.Fatalf("second HDR stream: rung %s, want keyframes-only", small.SoftwareRung())
	}
	// The encoder has headroom for rung 1 (0.3 of its 4 cores) but the ledger does not (3.42 held).
	if got := runScript(t, small.NewRungMonitor(RungMonitorConfig{CPUAllowance: 4}), phase{3 * time.Minute, 1, 0.3}); len(got) != 0 {
		t.Fatalf("stepped up past the ledger: %+v", got)
	}
	if small.SoftwareRung() != RungKeyframes || !near(cores(), 3.42+0.432) {
		t.Fatalf("refused step up moved the lease: %s, %.3f cores", small.SoftwareRung(), cores())
	}
	full.Release()
	got = runScript(t, small.NewRungMonitor(RungMonitorConfig{CPUAllowance: 4}), phase{3 * time.Minute, 1, 0.3})
	if len(got) == 0 || got[0].rung != RungLight {
		t.Fatalf("step up with room: decisions %+v, want rung 1 first", got)
	}
	if last := got[len(got)-1].rung; small.SoftwareRung() != last || !near(cores(), 3.6*heavyRungCosts[last]) {
		t.Fatalf("lease %s at %.3f cores, want it to follow the last step (%s)", small.SoftwareRung(), cores(), last)
	}
}
