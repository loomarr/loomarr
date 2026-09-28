package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestCompilationGate_Defers(t *testing.T) {
	gate := func(take bool) filler.CompilationGate {
		return filler.CompilationGate{
			Take: func() bool { return take },
			Over: func() time.Duration { return 2 * time.Minute },
		}
	}
	reel, spot := int64(30*60*1000), int64(45*1000)
	for _, tc := range []struct {
		name     string
		gate     filler.CompilationGate
		kind     string
		duration int64
		want     bool
	}{
		{"off holds a reel back", gate(false), "archive", reel, true},
		{"on takes a reel", gate(true), "archive", reel, false},
		{"a single spot is never held", gate(false), "archive", spot, false},
		{"exactly the ceiling is a single clip", gate(false), "archive", (2 * time.Minute).Milliseconds(), false},
		{"unknown length can't be judged before download", gate(false), "youtube", 0, false},
		{"the drop folder is the operator's own media", gate(false), "folder", reel, false},
		{"the zero gate takes everything", filler.CompilationGate{}, "archive", reel, false},
	} {
		if got := tc.gate.Defers(tc.kind, tc.duration); got != tc.want {
			t.Errorf("%s: Defers(%q, %d) = %v, want %v", tc.name, tc.kind, tc.duration, got, tc.want)
		}
	}
}
