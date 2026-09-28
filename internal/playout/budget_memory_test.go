package playout

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media"
)

const gib = int64(1) << 30

// fakeHost models host memory: every running encode holds cost bytes of the base.
type fakeHost struct {
	base    int64
	cost    int64
	running atomic.Int64
	known   bool
}

func (h *fakeHost) available() (int64, bool) {
	return h.base - h.running.Load()*h.cost, h.known
}

func (h *fakeHost) gate(reserve int64) media.MemoryGate {
	return media.MemoryGate{
		Available: h.available,
		Reserve:   func() int64 { return reserve },
		PerEncode: func() int64 { return h.cost },
	}
}

func testMemory(host *fakeHost, reserve int64, clock *time.Time, mu *sync.Mutex) *hostMemory {
	return &hostMemory{gate: host.gate(reserve), now: func() time.Time { mu.Lock(); defer mu.Unlock(); return *clock }}
}

func TestHostMemory_RampWindowHandsOverToTheReading(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	// 6 GiB available, 2 GiB reserve, 1 GiB per encode: four fit.
	host := &fakeHost{base: 6 * gib, cost: gib, known: true}
	m := testMemory(host, 2*gib, &clock, &mu)

	var releases []func()
	for range 4 {
		release, ok := m.admit()
		if !ok {
			t.Fatal("an encode that fits was refused")
		}
		releases = append(releases, release)
	}
	if _, ok := m.admit(); ok {
		t.Fatal("a fifth encode was admitted into the reserve inside the ramp window")
	}
	// The encoders ramp up: after the window the reading itself reflects all four; still no room.
	host.running.Store(4)
	mu.Lock()
	clock = clock.Add(memoryRampWindow + time.Second)
	mu.Unlock()
	if _, ok := m.admit(); ok {
		t.Fatal("an encode was admitted once the reading showed the reserve was reached")
	}
	// One encode exits and its memory returns: exactly one more fits.
	releases[0]()
	host.running.Add(-1)
	release, ok := m.admit()
	if !ok {
		t.Fatal("an encode was refused after memory was released")
	}
	release()
	for _, r := range releases[1:] {
		r()
	}
}

func TestHostMemory_ReleasedEncodeStopsCountingInsideTheRampWindow(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	// 4 GiB available, 2 GiB reserve, 1 GiB per encode: two encodes fit.
	host := &fakeHost{base: 4 * gib, cost: gib, known: true}
	m := testMemory(host, 2*gib, &clock, &mu)

	first, ok1 := m.admit()
	second, ok2 := m.admit()
	if !ok1 || !ok2 {
		t.Fatal("two encodes that fit were refused")
	}
	if _, ok := m.admit(); ok {
		t.Fatal("third encode admitted into the reserve")
	}
	mu.Lock()
	clock = clock.Add(time.Second) // still inside the ramp window
	mu.Unlock()
	first() // exits early; its memory never showed in the reading
	first() // a repeated release must not remove another encode's entry
	third, ok := m.admit()
	if !ok {
		t.Fatal("a released encode still counted against the next admission inside the ramp window")
	}
	if _, ok := m.admit(); ok {
		t.Fatal("double release freed a second ramp entry")
	}
	second()
	third()
}

func TestHostMemory_UnknownMemoryLeavesTheGateOpen(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	m := testMemory(&fakeHost{cost: gib, known: false}, 2*gib, &clock, &mu)
	for range 8 {
		if _, ok := m.admit(); !ok {
			t.Fatal("unknown host memory refused an encode")
		}
	}
}

// A hardware transcode the host has no memory for is refused, and the refusal returns its ledger
// booking; a copy and a software transcode hold no hardware encoder and are not gated.
func TestResourceBudget_MemoryGateRefusesAHardwareTranscodeIntoTheReserve(t *testing.T) {
	host := &fakeHost{base: 2*gib + gib/2, cost: gib, known: true} // short before anything runs
	b := NewResourceBudget(nvencFacts).WithMemoryGate(host.gate(2 * gib))
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassSDR}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("hardware transcode into the memory reserve: err = %v, want ErrAtCapacity", err)
	}
	b.mu.Lock()
	booked := b.useLocked(nil).Transcodes
	b.mu.Unlock()
	if booked != 0 {
		t.Fatalf("a refused admission left %d transcode(s) booked", booked)
	}
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassCopy}); err != nil {
		t.Fatalf("a copy holds no encoder and must not be gated: %v", err)
	}
	software := NewResourceBudget(func() BudgetFacts { f := nvencFacts(); f.Hardware = false; return f }).
		WithMemoryGate(host.gate(2 * gib))
	if _, err := software.Admit(context.Background(), AdmitRequest{Class: ClassSDR}); err != nil {
		t.Fatalf("a software transcode holds no hardware encoder and must not be gated: %v", err)
	}
}

// #1562 / #1505: the ledger is the only count. The retired encode pool counted hardware slots as
// the SDR streams that fit rung 0, so on a host whose sessions start lower (FirstRung) it refused
// a stream the ledger admitted at the dropped rung.
func TestResourceBudget_HardwareAdmissionFollowsTheLedgerBelowRungZero(t *testing.T) {
	facts := BudgetFacts{
		Hardware: true, SessionLimit: 12, CPUAllowance: 1,
		Rungs: []int{1080, 720}, FirstRung: 1,
		Costs: map[CostKey]ClassCost{
			{Class: ClassSDR, Height: 1080}: {Speed: 2.4, CPUCores: 0.03}, // two fit at rung 0
			{Class: ClassSDR, Height: 720}:  {Speed: 3.6, CPUCores: 0.02}, // three fit at 720p
		},
	}
	host := &fakeHost{base: 64 * gib, cost: gib, known: true}
	b := NewResourceBudget(func() BudgetFacts { return facts }).WithMemoryGate(host.gate(2 * gib))
	admitN(t, b, ClassSDR, 3)
	if _, err := b.Admit(context.Background(), AdmitRequest{Class: ClassSDR}); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("a fourth stream past the ledger: err = %v, want ErrAtCapacity", err)
	}
}
