package playout

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestOpenBlockBeforeKeepsProgrammeAliveAfterStartupDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var lifetime context.Context
		deadline := time.Now().Add(time.Second)
		block, err := openBlockBefore(t.Context(), deadline, func(ctx context.Context) (Block, error) {
			lifetime = ctx
			return Block{Content: io.NopCloser(strings.NewReader(strings.Repeat("abc", 200)))}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		if lifetime.Err() != nil {
			t.Fatal("startup deadline cancelled the programme lifetime")
		}
		body, err := io.ReadAll(block.Content)
		if err != nil || string(body) != strings.Repeat("abc", 200) {
			t.Fatalf("body=%q err=%v", body, err)
		}
		if err := block.Content.Close(); err != nil {
			t.Fatal(err)
		}
		if lifetime.Err() == nil {
			t.Fatal("release did not cancel lifetime")
		}
	})
}

func TestOpenBlockBeforeRejectsLateFirstByteAndClosesReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		defer writer.Close()
		deadline := time.Now().Add(time.Second)
		_, err := openBlockBefore(t.Context(), deadline, func(context.Context) (Block, error) {
			return Block{Content: reader}, nil
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err=%v", err)
		}
		if _, err := writer.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write=%v", err)
		}
		if !time.Now().Equal(deadline) {
			t.Fatalf("ended at %s, want %s", time.Now(), deadline)
		}
	})
}

func TestOpenBlockBeforeRejectsLateLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deadline := time.Now().Add(time.Second)
		_, err := openBlockBefore(t.Context(), deadline, func(ctx context.Context) (Block, error) {
			<-ctx.Done()
			return Block{}, ctx.Err()
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err=%v", err)
		}
		if !time.Now().Equal(deadline) {
			t.Fatalf("ended at %s, want %s", time.Now(), deadline)
		}
	})
}

func TestOpenBlockBeforeDoesNotOpenAfterDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, err := openBlockBefore(t.Context(), time.Now(), func(context.Context) (Block, error) {
			t.Fatal("expired open was invoked")
			return Block{}, nil
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	})
}

func TestPumpBlocksOpensSuccessorBeforeBoundaryWithoutExpiringItsLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		now := time.Now()
		boundary := now.Add(2 * time.Second)
		reader, writer := io.Pipe()
		var requests []time.Time
		source := BlockSource(func(ctx context.Context, request BlockRequest) (Block, error) {
			requests = append(requests, request.AiringAt)
			switch len(requests) {
			case 1:
				return Block{Content: io.NopCloser(strings.NewReader("a")), Identity: AiringIdentity{
					StartedAt: now.Add(-time.Second), EndsAt: boundary, Kind: "program", ContentID: "a", ScheduleBlockID: "a",
				}}, nil
			case 2:
				if !time.Now().Before(boundary) {
					t.Fatal("successor opening waited until its boundary")
				}
				go func() {
					defer writer.Close()
					if _, err := writer.Write([]byte("b")); err != nil {
						t.Error(err)
						return
					}
					time.Sleep(3 * time.Second)
					if ctx.Err() != nil {
						t.Error("successor expired at its start boundary")
						return
					}
					if _, err := writer.Write([]byte("c")); err != nil {
						t.Error(err)
					}
				}()
				return Block{Content: reader, Identity: AiringIdentity{
					StartedAt: boundary, EndsAt: boundary.Add(2 * time.Second), Kind: "program", ContentID: "b", ScheduleBlockID: "b",
				}}, nil
			default:
				cancel()
				return Block{}, ctx.Err()
			}
		})
		var output writeCloser
		pumpBlocks(ctx, &output, source, "channel", PlanBaseline, nil)
		if output.String() != "abc" {
			t.Fatalf("output = %q", output.String())
		}
		if len(requests) != 3 || !requests[0].IsZero() || !requests[1].Equal(boundary) || !requests[2].Equal(boundary.Add(2*time.Second)) {
			t.Fatalf("requested schedule instants = %v", requests)
		}
	})
}

func TestPumpBlocksDiscardsLookaheadWithChangedBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		now := time.Now()
		boundary := now.Add(time.Second)
		calls := 0
		source := BlockSource(func(ctx context.Context, request BlockRequest) (Block, error) {
			calls++
			switch calls {
			case 1:
				return Block{Content: io.NopCloser(strings.NewReader("a")), Identity: AiringIdentity{
					StartedAt: now.Add(-time.Second), EndsAt: boundary, Kind: "program", ContentID: "a", ScheduleBlockID: "a",
				}}, nil
			case 2:
				if !request.AiringAt.Equal(boundary) {
					t.Fatal("missing lookahead instant")
				}
				return Block{Content: io.NopCloser(strings.NewReader("wrong")), Identity: AiringIdentity{
					StartedAt: boundary.Add(-time.Second), EndsAt: boundary.Add(time.Second), Kind: "program", ContentID: "changed", ScheduleBlockID: "changed",
				}}, nil
			case 3:
				if !request.AiringAt.IsZero() || !time.Now().Equal(boundary) {
					t.Fatal("fallback did not resolve current time at boundary")
				}
				cancel()
				return Block{Content: io.NopCloser(strings.NewReader("current")), Identity: AiringIdentity{
					StartedAt: boundary, EndsAt: boundary.Add(time.Second), Kind: "program", ContentID: "current", ScheduleBlockID: "current",
				}}, nil
			default:
				t.Fatal("unexpected extra resolve")
				return Block{}, context.Canceled
			}
		})
		var output writeCloser
		pumpBlocks(ctx, &output, source, "channel", PlanBaseline, nil)
		if calls != 3 || output.String() != "acurrent" {
			t.Fatalf("calls=%d output=%q", calls, output.String())
		}
	})
}
