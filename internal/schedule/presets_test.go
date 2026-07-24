package schedule

import (
	"testing"
	"time"
)

func TestLowerWhen(t *testing.T) {
	// Reference clocks: 2026-07-25 Sat, 2026-07-23 Thu.
	sat9 := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	sat14 := time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)
	thu21 := time.Date(2026, 7, 23, 21, 0, 0, 0, time.UTC)
	thu1 := time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC)
	dec20 := time.Date(2026, 12, 20, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		token    string
		ok       bool
		priority int
		matchAt  time.Time
		matches  bool
	}{
		{"weekend", true, 20, sat9, true},
		{"weekday", true, 20, thu21, true},
		{"mornings", true, 30, sat9, true},
		{"mornings", true, 30, sat14, false},
		{"primetime", true, 40, thu21, true},
		{"late-night", true, 40, thu1, true}, // wraps 23→2
		{"overnight", true, 40, thu1, false}, // 2–6 excludes 1
		{"holiday:christmas", true, 60, dec20, true},
		{"holiday:christmas", true, 60, sat9, false},
		{"holiday:not-a-holiday", false, 0, sat9, false},
		// Composite: weekend AND mornings — matches Sat 9am, not Sat 2pm (hour fails) nor Thu 9am (day fails).
		{"weekend-mornings", true, 35, sat9, true},
		{"weekend-mornings", true, 35, sat14, false},
		{"garbage", false, 0, sat9, false},
	}
	for _, c := range cases {
		t.Run(c.token, func(t *testing.T) {
			w, p, ok := LowerWhen(c.token)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if p != c.priority {
				t.Errorf("priority = %d, want %d", p, c.priority)
			}
			if got := w.Matches(c.matchAt); got != c.matches {
				t.Errorf("Matches(%v) = %v, want %v", c.matchAt, got, c.matches)
			}
		})
	}
}

func TestLowerWhat(t *testing.T) {
	t.Run("all → nil scope, ok", func(t *testing.T) {
		s, _, ok := LowerWhat("all")
		if !ok || s != nil {
			t.Errorf("all: scope=%v ok=%v, want nil/true", s, ok)
		}
	})
	t.Run("kids → genres + stricter ceiling", func(t *testing.T) {
		s, ceil, ok := LowerWhat("kids")
		if !ok || s == nil || len(s.Genres.Include) == 0 {
			t.Fatalf("kids: scope=%v ok=%v", s, ok)
		}
		if ceil != NormalizeRating("TV-Y7") {
			t.Errorf("kids ceiling = %q, want TV-Y7", ceil)
		}
	})
	t.Run("series:<key> → series scope", func(t *testing.T) {
		s, _, ok := LowerWhat("series:series:tvdb:71470")
		if !ok || len(s.Series) != 1 || string(s.Series[0]) != "series:tvdb:71470" {
			t.Errorf("series scope wrong: %+v ok=%v", s, ok)
		}
	})
	t.Run("genre:Comedy → include", func(t *testing.T) {
		s, _, ok := LowerWhat("genre:Comedy")
		if !ok || len(s.Genres.Include) != 1 || s.Genres.Include[0] != "Comedy" {
			t.Errorf("genre include wrong: %+v", s)
		}
	})
	t.Run("era:1990-1999 → range", func(t *testing.T) {
		s, _, ok := LowerWhat("era:1990-1999")
		if !ok || s.Era == nil || s.Era.From != 1990 || s.Era.To != 1999 {
			t.Errorf("era wrong: %+v", s)
		}
	})
	t.Run("unknown → dropped", func(t *testing.T) {
		if _, _, ok := LowerWhat("nonsense"); ok {
			t.Error("unknown WHAT token should be dropped")
		}
	})
	t.Run("empty series key → dropped", func(t *testing.T) {
		if _, _, ok := LowerWhat("series:"); ok {
			t.Error("empty series key should be dropped")
		}
	})
}

func TestLowerHow(t *testing.T) {
	t.Run("marathon", func(t *testing.T) {
		how, win, ok := LowerHow("marathon")
		if !ok {
			t.Fatal("marathon should lower")
		}
		if how.Ordering != OrderSequential || !how.NoBreaks || how.Separation.BlockMax != -1 {
			t.Errorf("marathon how wrong: %+v", how)
		}
		if win != WindowFull {
			t.Errorf("marathon window = %v, want WindowFull", win)
		}
	})
	t.Run("syndication", func(t *testing.T) {
		how, _, ok := LowerHow("syndication")
		if !ok || how.Ordering != OrderSyndication {
			t.Errorf("syndication how wrong: %+v ok=%v", how, ok)
		}
	})
	t.Run("unknown → dropped", func(t *testing.T) {
		if _, _, ok := LowerHow("mystery"); ok {
			t.Error("unknown HOW token should be dropped")
		}
	})
}

// The marathon HOW actually produces an unbounded, break-free, sequential binge through
// the resolution chain — the end-to-end check that -1 BlockMax survives applyRuleHow and
// reads as unbounded in the resolved policy.
func TestMarathonHow_ResolvesToUnboundedNoBreaks(t *testing.T) {
	how, win, _ := LowerHow("marathon")
	rp := ChannelPolicy{}.Resolved(Sequential, false)
	rp = applyRuleHow(rp, how)
	if rp.Ordering != OrderSequential {
		t.Errorf("marathon ordering = %q, want sequential", rp.Ordering)
	}
	if rp.Sep.BlockMax > 0 {
		t.Errorf("marathon BlockMax = %d, want <= 0 (unbounded)", rp.Sep.BlockMax)
	}
	if win != WindowFull {
		t.Errorf("marathon window = %v, want WindowFull (no truncation)", win)
	}
}
