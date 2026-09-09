package playout

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"
)

func TestStreamViewerByteBoundAndOrderedTail(t *testing.T) {
	for _, fragment := range []int{1, 376, transportReadSize} {
		t.Run(strconv.Itoa(fragment), func(t *testing.T) {
			viewer := newStreamViewer()
			want := make([]byte, 512<<10) // fixed contract, independent of the implementation constant
			for i := range want {
				want[i] = byte(i % 251)
			}
			for offset := 0; offset < len(want); offset += fragment {
				if !viewer.offer(want[offset:min(offset+fragment, len(want))]) {
					t.Fatalf("rejected %d bytes below the byte limit", offset)
				}
			}
			if viewer.offer([]byte{1}) {
				t.Fatal("accepted a byte beyond the memory bound")
			}
			viewer.close()
			var got []byte
			for {
				chunk, err := viewer.Next(t.Context())
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, chunk...)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("overflow or producer EOF lost/reordered accepted bytes")
			}
			if viewer.offer([]byte{1}) {
				t.Fatal("closed stream accepted more bytes")
			}
		})
	}
}

func TestStreamViewerWraparoundAndByteOwnership(t *testing.T) {
	viewer := newStreamViewer()
	defer viewer.discard()
	if !viewer.offer([]byte("unaligned")) {
		t.Fatal("prelude rejected")
	}
	if _, err := viewer.Next(t.Context()); err != nil {
		t.Fatal(err)
	}
	original := bytes.Repeat([]byte{17}, viewerBufferBytes)
	if !viewer.offer(original) {
		t.Fatal("initial burst rejected")
	}
	original[0] = 99
	first, err := viewer.Next(t.Context())
	if err != nil || first[0] != 17 {
		t.Fatalf("queued bytes alias producer: %v", err)
	}
	replacement := bytes.Repeat([]byte{42}, transportReadSize)
	if !viewer.offer(replacement) {
		t.Fatal("consumed bytes did not free capacity")
	}
	// Keep the returned chunk while the same ring positions are reused.
	if !bytes.Equal(first, bytes.Repeat([]byte{17}, transportReadSize)) {
		t.Fatal("returned bytes alias the ring")
	}
	viewer.close()
	var tail []byte
	for {
		chunk, err := viewer.Next(t.Context())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		tail = append(tail, chunk...)
	}
	want := append(bytes.Repeat([]byte{17}, viewerBufferBytes-transportReadSize), replacement...)
	if !bytes.Equal(tail, want) {
		t.Fatal("wrapped stream lost or reordered bytes")
	}
}

func TestStreamViewerCancellationDoesNotRetireStream(t *testing.T) {
	viewer := newStreamViewer()
	defer viewer.discard()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := viewer.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %v", err)
	}
	deadline, stop := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer stop()
	if _, err := viewer.Next(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting read deadline: %v", err)
	}
	if !viewer.offer([]byte("after cancellation")) {
		t.Fatal("read cancellation retired the stream")
	}
	got, err := viewer.Next(t.Context())
	if err != nil || string(got) != "after cancellation" {
		t.Fatalf("subsequent read = %q, %v", got, err)
	}
}

func TestStreamViewerReleaseWakesReadAndDiscardsTail(t *testing.T) {
	for _, queued := range []bool{false, true} {
		viewer := newStreamViewer()
		if queued && !viewer.offer([]byte("unread")) {
			t.Fatal("offer rejected")
		}
		if queued {
			viewer.close() // release must also clear an already retired producer's tail
			viewer.discard()
		}
		result := make(chan error, 1)
		go func() { _, err := viewer.Next(t.Context()); result <- err }()
		viewer.discard()
		select {
		case err := <-result:
			if !errors.Is(err, io.EOF) {
				t.Fatalf("released read: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("released reader remained blocked")
		}
		viewer.discard()
		if viewer.offer([]byte("late")) {
			t.Fatal("release reopened the stream")
		}
	}
}
