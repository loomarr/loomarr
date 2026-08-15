package media

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEncodePoolForegroundUsesEverySlot(t *testing.T) {
	p := NewEncodePool(func() int { return 2 })

	r1, ok1 := p.AcquireForeground(t.Context())
	r2, ok2 := p.AcquireForeground(t.Context())
	if !ok1 || !ok2 {
		t.Fatalf("first two foreground leases = %v, %v; want both admitted", ok1, ok2)
	}
	if _, ok := p.AcquireForeground(t.Context()); ok {
		t.Fatal("third foreground lease admitted past capacity")
	}
	r1()
	r2()
}

func TestEncodePoolBackgroundKeepsOneSlotForForeground(t *testing.T) {
	p := NewEncodePool(func() int { return 2 })

	workCtx, release, ok := p.AcquireBackground(t.Context())
	if !ok {
		t.Fatal("background lease refused with one spare slot")
	}
	defer release()
	if err := workCtx.Err(); err != nil {
		t.Fatalf("background context already cancelled: %v", err)
	}
	if _, _, ok := p.AcquireBackground(t.Context()); ok {
		t.Fatal("a second background encode was admitted")
	}
	foregroundRelease, ok := p.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("background work consumed the foreground reserve")
	}
	foregroundRelease()
}

func TestEncodePoolForegroundPreemptsBackground(t *testing.T) {
	p := NewEncodePool(func() int { return 2 })

	backgroundCtx, backgroundRelease, ok := p.AcquireBackground(t.Context())
	if !ok {
		t.Fatal("background lease refused")
	}
	foregroundRelease, ok := p.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("first foreground lease refused")
	}

	released := make(chan struct{})
	go func() {
		<-backgroundCtx.Done()
		backgroundRelease()
		close(released)
	}()

	secondRelease, ok := p.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("second foreground lease did not replace cancelled background work")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("preempted background worker did not observe cancellation")
	}
	foregroundRelease()
	secondRelease()
}

func TestEncodePoolBackgroundNeedsMeasuredSpareCapacity(t *testing.T) {
	for name, capacity := range map[string]func() int{
		"software only": func() int { return 0 },
		"one slot":      func() int { return 1 },
		"unknown":       nil,
	} {
		t.Run(name, func(t *testing.T) {
			p := NewEncodePool(capacity)
			if _, _, ok := p.AcquireBackground(t.Context()); ok {
				t.Fatal("background work admitted without capacity beyond the live reserve")
			}
		})
	}
}

func TestEncodePoolReleaseIsIdempotentAndRaceSafe(t *testing.T) {
	const slots = 4
	p := NewEncodePool(func() int { return slots })
	var wg sync.WaitGroup
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if release, ok := p.AcquireForeground(context.Background()); ok {
				release()
				release()
			}
		}()
	}
	wg.Wait()

	releases := make([]func(), 0, slots)
	for range slots {
		release, ok := p.AcquireForeground(t.Context())
		if !ok {
			t.Fatal("an idempotent release leaked a slot")
		}
		releases = append(releases, release)
	}
	if _, ok := p.AcquireForeground(t.Context()); ok {
		t.Fatal("pool exceeded capacity after concurrent use")
	}
	for _, release := range releases {
		release()
	}
}
