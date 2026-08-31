package mediatools

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestConditioningScalarParsingFailsMalformedAndOverflowClosed(t *testing.T) {
	for _, raw := range []string{"nope", "NaN", "+Inf", "1e40"} {
		if _, err := parseConditioningMilliseconds(raw); !errors.Is(err, ErrConditioningOutput) {
			t.Errorf("parse %q error = %v, want invalid output", raw, err)
		}
	}
	for _, raw := range []string{"", "N/A"} {
		got, err := parseConditioningMilliseconds(raw)
		if err != nil || got.Available {
			t.Errorf("parse absent %q = %+v, %v", raw, got, err)
		}
	}
	if _, err := checkedConditioningAdd(maxInt64ForConditioningTest, 1); !errors.Is(err, ErrConditioningOutput) {
		t.Fatalf("addition overflow error = %v", err)
	}
	if _, err := checkedConditioningSub(minInt64ForConditioningTest, 1); !errors.Is(err, ErrConditioningOutput) {
		t.Fatalf("subtraction overflow error = %v", err)
	}
}

func TestCopyConditioningSnapshotCapsActualBytesAfterStat(t *testing.T) {
	err := copyConditioningSnapshot(context.Background(), io.Discard, endlessConditioningReader{})
	if !errors.Is(err, ErrConditioningResourceLimit) {
		t.Fatalf("growing-object copy error = %v, want resource limit", err)
	}
}

func TestCopyConditioningSnapshotObservesCancellationDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := cancelingConditioningReader{cancel: cancel}
	err := copyConditioningSnapshot(ctx, io.Discard, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("controlled copy cancellation error = %v, want context cancellation", err)
	}
}

type endlessConditioningReader struct{}

func (endlessConditioningReader) Read(buffer []byte) (int, error) {
	return len(buffer), nil
}

type cancelingConditioningReader struct {
	cancel context.CancelFunc
}

func (r cancelingConditioningReader) Read(buffer []byte) (int, error) {
	r.cancel()
	return len(buffer), nil
}

const (
	maxInt64ForConditioningTest = int64(^uint64(0) >> 1)
	minInt64ForConditioningTest = -maxInt64ForConditioningTest - 1
)
