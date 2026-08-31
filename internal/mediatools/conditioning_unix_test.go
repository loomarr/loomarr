//go:build !windows

package mediatools_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestMeasureConditioningBindsEveryInvocationToValidatedArtifacts(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "artifact.mp4")
	parent := filepath.Join(dir, "parent.mp4")
	if err := os.WriteFile(artifact, []byte("artifact-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("parent-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "replaced")
	script := fmt.Sprintf(`artifact=%s
parent=%s
state=%s
input=""
interval=""
previous=""
last=""
packets=false
probe=false
frames=false
for arg do
  if [ "$previous" = "-i" ]; then input="$arg"; fi
  if [ "$previous" = "-read_intervals" ]; then interval="$arg"; fi
  if [ "$arg" = "-show_packets" ]; then packets=true; fi
  if [ "$arg" = "-show_frames" ]; then frames=true; fi
  if [ "$arg" = "-show_entries" ]; then probe=true; fi
  previous="$arg"
  last="$arg"
done
if [ -z "$input" ]; then input="$last"; fi
if [ ! -f "$input" ]; then exit 90; fi
contents=$(cat -- "$input") || exit 91
case "$interval" in
  1.000%%7.000|5.000%%11.000) want=parent-original ;;
  *) want=artifact-original ;;
esac
if [ "$contents" != "$want" ]; then exit 92; fi
if [ ! -e "$state" ]; then
  : > "$state"
  rm -f -- "$artifact"
  mkfifo -- "$artifact"
  rm -f -- "$parent"
  ln -s /dev/null "$parent"
fi
if $packets; then
  case "$interval" in
    0.000%%3.000) packet='{"stream_index":0,"pts_time":"0.000","duration_time":"0.100","data_hash":"start"}' ;;
    7.000%%10.000) packet='{"stream_index":0,"pts_time":"9.000","duration_time":"0.100","data_hash":"end"}' ;;
    1.000%%7.000) packet='{"stream_index":0,"pts_time":"4.100","duration_time":"0.100","data_hash":"start"}' ;;
    5.000%%11.000) packet='{"stream_index":0,"pts_time":"8.000","duration_time":"0.100","data_hash":"end"}' ;;
    *) exit 93 ;;
  esac
  printf '{"streams":[{"index":0,"codec_type":"video"}],"packets":[%%s],"format":{"duration":"10"}}\n' "$packet"
elif $frames; then
  printf '%%s\n' '{"frames":[{"stream_index":0,"best_effort_timestamp_time":"0","duration_time":"10"}]}'
elif $probe; then
  printf '%%s\n' '{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"10","avg_frame_rate":"25/1"}],"format":{"duration":"10"}}'
fi
`, strconv.Quote(artifact), strconv.Quote(parent), strconv.Quote(state))
	tool := testkit.POSIXExecutable(t, "conditioning-tool", script)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(ctx, mediatools.ConditioningRequest{
		Path: artifact, ParentPath: parent,
		IntendedCuts: []mediatools.Interval{{StartMs: 4_000, EndMs: 8_000}},
	})
	if err != nil {
		t.Fatalf("replacement changed or blocked measurement: %v", err)
	}
	if len(got.Cuts) != 1 || len(got.Cuts[0].Streams) != 1 ||
		!got.Cuts[0].Streams[0].StartError.Available || !got.Cuts[0].Streams[0].EndError.Available {
		t.Fatalf("bound cut evidence = %+v", got.Cuts)
	}
}

func TestMeasureConditioningRejectsDevicesAndFIFOsBeforeTools(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "invoked")
	tool := testkit.POSIXExecutable(t, "conditioning-tool", "touch \""+sentinel+"\"\nexit 99")
	fifo := filepath.Join(t.TempDir(), "media.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"device": "/dev/null", "FIFO": fifo} {
		t.Run(name, func(t *testing.T) {
			_ = os.Remove(sentinel)
			_, err := mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: path})
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("error = %v, want invalid output", err)
			}
			if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
				t.Fatalf("media tool ran for %s", name)
			}
		})
	}
}
