package playoutcertfixture

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShapeValidatorWaitRespectsCancellation(t *testing.T) {
	blocked := make(chan struct{})
	validator := &ShapeValidator[int]{Shapes: []int{1}, WaitFor: blocked}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := validator.Validate(ctx, nil)
		result <- err
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Validate error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Validate remained blocked after context cancellation")
	}
}
