package playout

import (
	"context"
	"errors"
	"time"
)

// Admission on a full host (#1780). Measured on the real Shield on rc.5: 38 of 120 channel surfs
// got 503 while neighbour warms and sessions lingering in their grace, none of them watched, held
// every slot of the ledger. Warming may only use spare room (#1512 G3), so a viewer's tune gives up
// idle work before it accepts a lower rung or is refused; a speculative tune never evicts anything,
// and a watched session is never evicted.

// watchedWithin is how recently a viewer must have fetched a packager's playlist for it to count as
// watched. An HLS viewer counts only while a playlist request is in flight, so a playing client
// reads 0 viewers between its polls (one per target duration, a few seconds); a longer silence
// means it has left. It is also a premium variant's whole grace (graceFor).
const watchedWithin = 10 * time.Second

// evictWait bounds how long a viewer's tune waits for an evicted packager's encoder to exit. After
// it the lease is returned anyway: the process is already cancelled and only tearing down.
const evictWait = 2 * time.Second

// Victim ranks, in the order a full host gives idle work up.
const (
	victimWarm    = iota + 1 // a neighbour warm no viewer joined
	victimGrace              // a session in its grace, not fetched within watchedWithin
	victimPremium            // a premium nobody is fetching now, whose channel's baseline still runs
)

// pickIdleVictim is the packager a viewer's tune evicts next, if any: warms first, then grace
// sessions and then premiums, each oldest idle first. A premium yields only while its channel's
// baseline runs, so its viewer's player has the baseline variant to fall back to.
func pickIdleVictim(channels map[packagedKey]*packagedChannel, now time.Time) (packagedKey, *packagedChannel, bool) {
	var (
		bestKey  packagedKey
		best     *packagedChannel
		bestRank int
	)
	for key, c := range channels {
		if c.viewers > 0 {
			continue
		}
		var rank int
		switch {
		case c.speculative:
			rank = victimWarm
		case !c.idleSince.IsZero() && now.Sub(c.idleSince) >= watchedWithin:
			rank = victimGrace
		case key.format.isPremium() && channels[packagedKey{channel: key.channel, format: FormatBaseline}] != nil:
			rank = victimPremium
		default:
			continue
		}
		if best == nil || rank < bestRank || rank == bestRank && olderIdle(key, c, bestKey, best) {
			bestKey, best, bestRank = key, c, rank
		}
	}
	return bestKey, best, best != nil
}

// olderIdle orders two candidates of one rank: the longer idle first, then by key so the choice
// never depends on map order.
func olderIdle(key packagedKey, c *packagedChannel, otherKey packagedKey, other *packagedChannel) bool {
	if !c.idleSince.Equal(other.idleSince) {
		return c.idleSince.Before(other.idleSince)
	}
	if key.channel != otherKey.channel {
		return key.channel < otherKey.channel
	}
	return key.format < otherKey.format
}

// graceFor is how long a packager survives its last viewer. A premium variant keeps running only
// while a premium viewer is fetching it; a baseline keeps the full grace that absorbs surfing.
func (m *PackagerHLS) graceFor(key packagedKey) time.Duration {
	if key.format.isPremium() {
		return min(m.grace, watchedWithin)
	}
	return m.grace
}

// admit books a new packager's lease (#1520). A viewer's tune on a full host evicts idle work, one
// packager at a time in pickIdleVictim's order, while that keeps it at the first rung; with nothing
// idle left it may drop a rung, or is refused. A speculative tune is admitted only into spare room.
func (m *PackagerHLS) admit(ctx context.Context, req AdmitRequest, speculative bool) (*Lease, error) {
	if m.budget == nil {
		return nil, nil
	}
	if speculative {
		return m.budget.Admit(ctx, req)
	}
	top := req
	top.NoRungDrop = true
	for {
		lease, err := m.budget.Admit(ctx, top)
		if !errors.Is(err, ErrAtCapacity) {
			return lease, err
		}
		if !m.evictIdle(ctx) {
			return m.budget.Admit(ctx, req)
		}
	}
}

// evictIdle stops the next idle packager and returns its lease, reporting whether there was one.
func (m *PackagerHLS) evictIdle(ctx context.Context) bool {
	m.mu.Lock()
	key, c, ok := pickIdleVictim(m.channels, time.Now())
	var speculative bool
	var idleFor time.Duration
	if ok {
		speculative, idleFor = c.speculative, time.Since(c.idleSince)
		m.removeLocked(key, c)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	m.log.Info("packager hls: evicted idle work for a viewer", "channel", key.channel, "format", string(key.format),
		"speculative", speculative, "idle_ms", idleFor.Milliseconds())
	wait, cancel := context.WithTimeout(ctx, evictWait)
	select {
	case <-c.done:
	case <-wait.Done():
	}
	cancel()
	c.lease.Release() // idempotent: the run returns it too once its encoder exits
	return true
}
