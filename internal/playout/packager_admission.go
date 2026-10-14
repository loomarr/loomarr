package playout

import (
	"context"
	"errors"
	"time"
)

// Admission on a full host (#1780). Measured on the real Shield on rc.5: 38 of 120 channel surfs
// got 503 while neighbour warms and sessions lingering in their grace, none of them watched, held
// every slot of the ledger. Warming may only use spare room (#1512 G3), so a viewer's tune gives up
// idle work before it accepts a lower rung or is refused; a speculative tune evicts nothing but a
// premium warm, and a watched session is never evicted.
//
// Premium warms (#1037). On rc.6 every Shield tune of a 4K channel started its premium cold: a
// neighbour warm warmed only the baseline, and a premium dies watchedWithin after its last fetch.
// A warm for a viewer whose client takes the premium warms the premium too, but it ranks below every
// baseline: it takes only spare room, and any baseline, even a warm's, may reclaim it.

// admissionMsg is the one message every admission decision (admit, refuse, evict) is logged under,
// so a household's log can be audited for them.
const admissionMsg = "packager hls: admission"

// watchedWithin is how recently a viewer must have fetched a packager's playlist for it to count as
// watched. An HLS viewer counts only while a playlist request is in flight, so a playing client
// reads 0 viewers between its polls (one per target duration, a few seconds); a longer silence
// means it has left. It is also the grace of a premium variant no warm holds (graceFor).
const watchedWithin = 10 * time.Second

// evictWait bounds how long a viewer's tune waits for an evicted packager's encoder to exit. After
// it the lease is returned anyway: the process is already cancelled and only tearing down.
const evictWait = 2 * time.Second

// Victim ranks, in the order a full host gives idle work up.
const (
	victimPremiumWarm = iota + 1 // a premium a neighbour warm last held (#1037)
	victimWarm                   // a neighbour warm no viewer joined
	victimGrace                  // a session in its grace, not fetched within watchedWithin
	victimPremium                // a premium nobody is fetching now, whose channel's baseline still runs
)

// victimReasons names each rank in the admission log.
var victimReasons = [...]string{
	victimPremiumWarm: "premium-warm", victimWarm: "warm", victimGrace: "grace", victimPremium: "premium-unwatched",
}

// victimRank is how readily a packager is given up for a tune, or 0 when it never is. A premium
// yields only while its channel's baseline runs, so its viewer's player has the baseline variant to
// fall back to.
func victimRank(channels map[packagedKey]*packagedChannel, key packagedKey, c *packagedChannel, now time.Time) int {
	switch {
	case c.viewers > 0:
		return 0
	case key.format.isPremium() && c.warmHeld:
		return victimPremiumWarm
	case c.speculative:
		return victimWarm
	case !c.idleSince.IsZero() && now.Sub(c.idleSince) >= watchedWithin:
		return victimGrace
	case key.format.isPremium() && channels[packagedKey{channel: key.channel, format: FormatBaseline}] != nil:
		return victimPremium
	default:
		return 0
	}
}

// pickIdleVictim is the packager a tune evicts next, if any, among those ranked at most maxRank:
// the lowest rank first, and within a rank the oldest idle first.
func pickIdleVictim(channels map[packagedKey]*packagedChannel, now time.Time, maxRank int) (packagedKey, *packagedChannel, bool) {
	var (
		bestKey  packagedKey
		best     *packagedChannel
		bestRank int
	)
	for key, c := range channels {
		rank := victimRank(channels, key, c, now)
		if rank == 0 || rank > maxRank {
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
// while a premium viewer is fetching it, or while a neighbour warm holds it; a baseline keeps the
// full grace that absorbs surfing. Called with m.mu held.
func (m *PackagerHLS) graceFor(key packagedKey, c *packagedChannel) time.Duration {
	if key.format.isPremium() && !c.warmHeld {
		return min(m.grace, watchedWithin)
	}
	return m.grace
}

// admitReason names who a new packager is for in the admission log.
func admitReason(key packagedKey, speculative bool) string {
	switch {
	case !speculative:
		return "viewer"
	case key.format.isPremium():
		return "premium-warm"
	default:
		return "warm"
	}
}

// admit books a new packager's lease (#1520) and logs the decision. A viewer's tune on a full host
// evicts idle work, one packager at a time in pickIdleVictim's order, while that keeps it at the
// first rung; with nothing idle left it may drop a rung, or is refused. A warm is admitted only into
// spare room, except that a baseline warm may reclaim a premium warm's (#1037).
func (m *PackagerHLS) admit(ctx context.Context, key packagedKey, req AdmitRequest, speculative bool) (*Lease, error) {
	if m.budget == nil {
		return nil, nil
	}
	lease, err := m.admitEvicting(ctx, key, req, speculative)
	decision := "admit"
	if err != nil {
		decision = "refuse"
	}
	args := []any{"decision", decision, "channel", key.channel, "format", string(key.format),
		"reason", admitReason(key, speculative)}
	if lease != nil {
		args = append(args, "rung", lease.Rung())
	}
	m.log.Info(admissionMsg, append(args, "budget", m.budget.Usage())...)
	return lease, err
}

func (m *PackagerHLS) admitEvicting(ctx context.Context, key packagedKey, req AdmitRequest, speculative bool) (*Lease, error) {
	top, evictable := req, victimPremium
	switch {
	case !speculative:
		top.NoRungDrop = true
	case key.format.isPremium():
		evictable = 0
	default:
		evictable = victimPremiumWarm
	}
	for {
		lease, err := m.budget.Admit(ctx, top)
		if !errors.Is(err, ErrAtCapacity) {
			return lease, err
		}
		if evictable == 0 || !m.evictIdle(ctx, key, evictable) {
			break
		}
	}
	if speculative {
		return nil, ErrAtCapacity
	}
	return m.budget.Admit(ctx, req)
}

// evictIdle stops the next idle packager ranked at most maxRank and returns its lease, reporting
// whether there was one. forKey is the packager the room is made for.
func (m *PackagerHLS) evictIdle(ctx context.Context, forKey packagedKey, maxRank int) bool {
	now := time.Now()
	m.mu.Lock()
	key, c, ok := pickIdleVictim(m.channels, now, maxRank)
	var reason string
	var idleFor time.Duration
	if ok {
		reason, idleFor = victimReasons[victimRank(m.channels, key, c, now)], now.Sub(c.idleSince)
		m.removeLocked(key, c)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	wait, cancel := context.WithTimeout(ctx, evictWait)
	select {
	case <-c.done:
	case <-wait.Done():
	}
	cancel()
	c.lease.Release() // idempotent: the run returns it too once its encoder exits
	m.log.Info(admissionMsg, "decision", "evict", "channel", key.channel, "format", string(key.format), "reason", reason,
		"for_channel", forKey.channel, "for_format", string(forKey.format), "idle_ms", idleFor.Milliseconds(),
		"budget", m.budget.Usage())
	return true
}
