package playoutcert

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestDecoderObserverCancellationJoinsReaderAndDecoder(t *testing.T) {
	decoder := &playoutcertfixture.Decoder{}
	reader, writer := io.Pipe()
	observer := startDecoderObserver(context.Background(), decoder, reader, 188)
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.Write(make([]byte, 188))
		writeDone <- err
	}()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := observer.wait(waitCtx, func(snapshot decoderSnapshot) bool { return snapshot.frames > 0 }); err != nil {
		t.Fatal(err)
	}
	if err := observer.close(); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("input writer did not join")
	}
	active, started, stopped := decoder.Counts()
	if active != 0 || started != 1 || stopped != 1 {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
}

func TestPostBoundaryProgressRequiresMediaInLateObservation(t *testing.T) {
	decoder := &playoutcertfixture.Decoder{}
	reader, writer := io.Pipe()
	observer := startDecoderObserver(context.Background(), decoder, reader, 188)
	t.Cleanup(func() {
		_ = observer.close()
		_ = writer.Close()
	})

	write := func() {
		t.Helper()
		if _, err := writer.Write(make([]byte, 188)); err != nil {
			t.Fatal(err)
		}
	}
	write()
	readyCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := observer.wait(readyCtx, func(snapshot decoderSnapshot) bool { return snapshot.frames > 0 }); err != nil {
		t.Fatal(err)
	}
	atBoundary := observer.snapshot()
	write() // A real early post-transition read/frame burst.
	if _, err := observer.wait(readyCtx, func(snapshot decoderSnapshot) bool { return snapshot.frames > atBoundary.frames }); err != nil {
		t.Fatal(err)
	}
	late := time.Now().Add(100 * time.Millisecond)
	time.Sleep(125 * time.Millisecond) // Reader and decoder remain open, but stalled.
	after := observer.snapshot()
	if postBoundaryProgressed(atBoundary, after, late) {
		t.Fatalf("early burst while open passed late continuity: before=%+v after=%+v", atBoundary, after)
	}
	write()
	if _, err := observer.wait(readyCtx, func(snapshot decoderSnapshot) bool {
		return !snapshot.lastRead.Before(late) && !snapshot.lastFrame.Before(late)
	}); err != nil {
		t.Fatal(err)
	}
	if !postBoundaryProgressed(atBoundary, observer.snapshot(), late) {
		t.Fatal("late read and decoded frame did not satisfy continuity")
	}
}
