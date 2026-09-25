package media

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func memoryPool(capacity int, host *fakeHost, reserve int64, clock *time.Time, mu *sync.Mutex) *EncodePool {
	p := NewEncodePool(func() int { return capacity }).WithMemoryGate(MemoryGate{
		Available: host.available,
		Reserve:   func() int64 { return reserve },
		PerEncode: func() int64 { return host.cost },
	})
	p.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return *clock }
	return p
}

func TestEncodePoolMemoryGateStopsBackgroundAtReserve(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	// Eight measured slots, but 6 GiB available with a 2 GiB reserve fits exactly four 1 GiB encodes.
	host := &fakeHost{base: 6 * gib, cost: gib, known: true}
	p := memoryPool(8, host, 2*gib, &clock, &mu)

	var releases []func()
	for i := range 4 {
		_, release, ok := p.AcquireBackground(t.Context(), time.Unix(int64(i), 0))
		if !ok {
			t.Fatalf("background lease %d refused with memory above the reserve", i)
		}
		releases = append(releases, release)
	}
	// The burst has not shown in the reading yet (running is still 0): the ramp window must count it.
	if _, _, ok := p.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("fifth background lease admitted into the memory reserve")
	}
	for _, release := range releases {
		release()
	}
}

func TestEncodePoolMemoryRampWindowHandsOverToTheReading(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	host := &fakeHost{base: 6 * gib, cost: gib, known: true}
	p := memoryPool(8, host, 2*gib, &clock, &mu)

	var releases []func()
	for range 4 {
		_, release, ok := p.AcquireBackground(t.Context(), time.Time{})
		if !ok {
			t.Fatal("setup lease refused")
		}
		releases = append(releases, release)
	}
	// The encoders ramp up: after the window the reading itself reflects all four; still no room.
	host.running.Store(4)
	mu.Lock()
	clock = clock.Add(memoryRampWindow + time.Second)
	mu.Unlock()
	if _, _, ok := p.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("background lease admitted once the reading showed the reserve was reached")
	}
	// One encode exits and its memory returns: exactly one more fits.
	releases[0]()
	host.running.Add(-1)
	_, release, ok := p.AcquireBackground(t.Context(), time.Time{})
	if !ok {
		t.Fatal("background lease refused after memory was released")
	}
	release()
	for _, r := range releases[1:] {
		r()
	}
}

func TestEncodePoolLiveGetsSlotUnderPreparationMemoryLoad(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	host := &fakeHost{base: 5 * gib, cost: gib, known: true}
	p := memoryPool(8, host, 2*gib, &clock, &mu)

	var workers sync.WaitGroup
	for {
		workCtx, release, ok := p.AcquireBackground(t.Context(), time.Time{})
		if !ok {
			break
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-workCtx.Done()
			host.running.Add(-1) // the encoder exits and its memory returns before the lease is released
			release()
		}()
	}
	if n := len(p.backgrounds); n != 3 {
		t.Fatalf("preparation filled %d encodes, want the 3 that fit above the reserve", n)
	}
	// The encoders ramp up and the reading now reflects every one of them.
	host.running.Store(3)
	mu.Lock()
	clock = clock.Add(memoryRampWindow + time.Second)
	mu.Unlock()

	// Memory is at the reserve because of preparation alone: live must preempt it, not fall back.
	release, ok := p.AcquireForeground(t.Context())
	if !ok {
		workers.Wait()
		t.Fatal("live playback refused because preparation held the memory")
	}
	workers.Wait()
	if host.running.Load() != 0 {
		t.Fatalf("preparation encodes still running after live preemption: %d", host.running.Load())
	}
	if _, _, ok := p.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("preparation readmitted while live playback holds a lease")
	}
	release()
}

func TestEncodePoolLiveFallsBackOnlyWhenNoPreparationCanYield(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	// The host is already short for reasons outside the pool; there is nothing to preempt.
	host := &fakeHost{base: 2*gib + gib/2, cost: gib, known: true}
	p := memoryPool(8, host, 2*gib, &clock, &mu)

	if _, ok := p.AcquireForeground(t.Context()); ok {
		t.Fatal("live hardware encode admitted into the memory reserve with no preparation to yield")
	}
	if _, _, ok := p.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("preparation admitted into the memory reserve")
	}
}

func TestEncodePoolUnknownMemoryLeavesGateOpen(t *testing.T) {
	var mu sync.Mutex
	clock := time.Unix(1_000, 0)
	host := &fakeHost{base: 0, cost: gib, known: false}
	p := memoryPool(4, host, 2*gib, &clock, &mu)

	release, ok := p.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("unknown host memory refused a live encode")
	}
	release()
	_, bgRelease, ok := p.AcquireBackground(t.Context(), time.Time{})
	if !ok {
		t.Fatal("unknown host memory refused preparation within slot capacity")
	}
	bgRelease()
}

func TestParseMemAvailable(t *testing.T) {
	const meminfo = "MemTotal:       32768000 kB\nMemFree:         1600000 kB\nMemAvailable:    6082560 kB\nBuffers:           10000 kB\n"
	got, ok := parseMemAvailable(strings.NewReader(meminfo))
	if !ok || got != 6082560*1024 {
		t.Fatalf("parseMemAvailable = %d, %v; want %d, true", got, ok, int64(6082560*1024))
	}
	for name, content := range map[string]string{
		"absent":    "MemTotal: 1 kB\n",
		"negative":  "MemAvailable: -5 kB\n",
		"garbage":   "MemAvailable: lots kB\n",
		"wrongUnit": "MemAvailable: 5 MB\n",
		"empty":     "",
	} {
		if _, ok := parseMemAvailable(strings.NewReader(content)); ok {
			t.Errorf("%s: parseMemAvailable accepted %q", name, content)
		}
	}
}
