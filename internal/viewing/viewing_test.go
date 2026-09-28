package viewing

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

var t0 = time.Unix(1_700_000_000, 0).UTC()

func poll(tr *Tracker, v Viewer, channel string, at time.Duration) {
	tr.Observe(v, channel, t0.Add(at))
}

func describe(ws []Watch) string {
	out := ""
	for _, w := range ws {
		out += fmt.Sprintf("[%s/%s %s since %s]", w.UserID, w.DeviceKey, w.ChannelID, w.Since.Sub(t0))
	}
	return out
}

var (
	adaTV  = Viewer{UserID: "u-ada", DeviceKey: "tv", DeviceLabel: "Living room TV"}
	adaWeb = Viewer{UserID: "u-ada", DeviceKey: "web", DeviceLabel: "Firefox on Linux"}
	boTV   = Viewer{UserID: "u-bo", DeviceKey: "tv2", DeviceLabel: "Den TV"}
)

func TestAPlayerPollingIsWatching(t *testing.T) {
	tr := New()
	poll(tr, adaTV, "ch-news", 0)
	poll(tr, adaTV, "ch-news", 4*time.Second)
	got := tr.Watching(t0.Add(5 * time.Second))
	if describe(got) != "[u-ada/tv ch-news since 0s]" || got[0].DeviceLabel != "Living room TV" {
		t.Fatalf("watching = %s %+v", describe(got), got)
	}
}

// The channel warmer fetches a neighbour's playlist once; that is not a viewer.
func TestASingleFetchIsNotWatching(t *testing.T) {
	tr := New()
	poll(tr, adaTV, "ch-news", 0)
	poll(tr, adaTV, "ch-news", 4*time.Second)
	poll(tr, adaTV, "ch-films", 5*time.Second) // warm of the next channel
	if got := describe(tr.Watching(t0.Add(6 * time.Second))); got != "[u-ada/tv ch-news since 0s]" {
		t.Fatalf("watching after a neighbour warm = %s, want only ch-news", got)
	}
}

func TestAViewerWhoStopsPollingLeaves(t *testing.T) {
	tr := New()
	poll(tr, adaTV, "ch-news", 0)
	poll(tr, adaTV, "ch-news", 4*time.Second)
	if got := tr.Watching(t0.Add(4*time.Second + TTL + time.Millisecond)); len(got) != 0 {
		t.Fatalf("watching after TTL = %s, want nobody", describe(got))
	}
}

// Changing channel moves the device at once; the old channel doesn't linger until its TTL.
func TestADeviceWatchesOneChannel(t *testing.T) {
	tr := New()
	poll(tr, adaTV, "ch-news", 0)
	poll(tr, adaTV, "ch-news", 4*time.Second)
	poll(tr, adaTV, "ch-films", 6*time.Second)
	poll(tr, adaTV, "ch-films", 8*time.Second)
	if got := describe(tr.Watching(t0.Add(9 * time.Second))); got != "[u-ada/tv ch-films since 6s]" {
		t.Fatalf("watching after a channel change = %s", got)
	}
}

// A gap longer than the TTL is a new viewing: it restarts "since" and needs two polls again.
func TestAGapStartsANewViewing(t *testing.T) {
	tr := New()
	poll(tr, adaTV, "ch-news", 0)
	poll(tr, adaTV, "ch-news", 4*time.Second)
	back := 4*time.Second + TTL + time.Second
	poll(tr, adaTV, "ch-news", back)
	if got := tr.Watching(t0.Add(back)); len(got) != 0 {
		t.Fatalf("one poll after a gap = %s, want not yet watching", describe(got))
	}
	poll(tr, adaTV, "ch-news", back+2*time.Second)
	if got := describe(tr.Watching(t0.Add(back + 2*time.Second))); got != fmt.Sprintf("[u-ada/tv ch-news since %s]", back) {
		t.Fatalf("after the gap = %s", got)
	}
}

func TestEachPersonAndDeviceIsSeparate(t *testing.T) {
	tr := New()
	for _, at := range []time.Duration{0, 3 * time.Second} {
		poll(tr, adaTV, "ch-news", at)
		poll(tr, adaWeb, "ch-films", at+time.Second)
		poll(tr, boTV, "ch-news", at+2*time.Second)
	}
	got := describe(tr.Watching(t0.Add(6 * time.Second)))
	want := "[u-ada/tv ch-news since 0s][u-ada/web ch-films since 1s][u-bo/tv2 ch-news since 2s]"
	if got != want {
		t.Fatalf("watching = %s, want %s", got, want)
	}
}

// Property: for any interleaving of polls, Watching reports at most one channel per device, only
// viewings with at least MinPolls polls, none older than the TTL, oldest viewing first; and the
// tracker forgets everything once it has been quiet for the TTL.
func TestWatchingInvariantsProperty(t *testing.T) {
	viewers := []Viewer{adaTV, adaWeb, boTV}
	channels := []string{"ch-a", "ch-b", "ch-c"}
	for seed := range uint64(300) {
		rng := rand.New(rand.NewPCG(seed, seed*7+1))
		tr := New()
		type k struct{ user, device, channel string }
		type hist struct {
			polls []time.Duration
		}
		seen := map[k]*hist{}
		at := time.Duration(0)
		for range 60 {
			at += time.Duration(rng.IntN(12_000)) * time.Millisecond
			v := viewers[rng.IntN(len(viewers))]
			ch := channels[rng.IntN(len(channels))]
			poll(tr, v, ch, at)
			key := k{v.UserID, v.DeviceKey, ch}
			if seen[key] == nil {
				seen[key] = &hist{}
			}
			seen[key].polls = append(seen[key].polls, at)

			now := t0.Add(at)
			got := tr.Watching(now)
			devices := map[string]bool{}
			for i, w := range got {
				if devices[w.UserID+"/"+w.DeviceKey] {
					t.Fatalf("seed %d: device reported twice: %s", seed, describe(got))
				}
				devices[w.UserID+"/"+w.DeviceKey] = true
				if now.Sub(w.LastSeen) > TTL {
					t.Fatalf("seed %d: stale viewing reported: %s", seed, describe(got))
				}
				if i > 0 && w.Since.Before(got[i-1].Since) {
					t.Fatalf("seed %d: not oldest-first: %s", seed, describe(got))
				}
				// The reported viewing's own history: every poll from Since to LastSeen is within
				// the TTL of the one before, and there are at least MinPolls of them.
				h := seen[k{w.UserID, w.DeviceKey, w.ChannelID}]
				n := 0
				for _, p := range h.polls {
					if !t0.Add(p).Before(w.Since) {
						n++
					}
				}
				if n < MinPolls {
					t.Fatalf("seed %d: reported with %d polls since %s: %s", seed, n, w.Since.Sub(t0), describe(got))
				}
			}
		}
		if got := tr.Watching(t0.Add(at + TTL + time.Millisecond)); len(got) != 0 {
			t.Fatalf("seed %d: still watching after a quiet TTL: %s", seed, describe(got))
		}
	}
}
