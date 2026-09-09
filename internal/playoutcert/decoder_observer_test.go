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
