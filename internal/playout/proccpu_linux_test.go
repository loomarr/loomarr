package playout

import (
	"testing"
	"time"
)

func TestParseProcStatCPU_CountsFieldsAfterTheCommandName(t *testing.T) {
	// A command name with spaces and a parenthesis must not shift the fields.
	stat := "4242 (ff (x) mpeg) S 1 4242 4242 0 -1 4194304 100 0 0 0 250 50 0 0 20 0 9 0 1 1 1"
	got, ok := parseProcStatCPU(stat)
	if !ok || got != 3*time.Second {
		t.Fatalf("parseProcStatCPU = %v, %v; want 3s (250+50 ticks)", got, ok)
	}
	if _, ok := parseProcStatCPU("garbage"); ok {
		t.Fatal("malformed stat parsed")
	}
}
