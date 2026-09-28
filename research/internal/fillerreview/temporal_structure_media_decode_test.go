package fillerreview

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/bgexec"
)

func TestRunTemporalStructureMediaCommandRejectsSuccessfulProcessWithErrorOutput(t *testing.T) {
	t.Parallel()
	if os.Getenv("LOOMARR_TEMPORAL_MEDIA_ERROR_HELPER") == "1" {
		_, _ = fmt.Fprintln(os.Stderr, "damaged input packet")
		os.Exit(0)
	}
	command := bgexec.Tool(context.Background(), os.Args[0], "-test.run=TestRunTemporalStructureMediaCommandRejectsSuccessfulProcessWithErrorOutput")
	command.Env = append(os.Environ(), "LOOMARR_TEMPORAL_MEDIA_ERROR_HELPER=1")
	output := &boundedTemporalStructureMediaOutput{}
	command.Stdout, command.Stderr = output, output
	err := runTemporalStructureMediaCommand(context.Background(), command, output)
	if err == nil || !strings.Contains(err.Error(), "errors despite a successful exit") || !strings.Contains(err.Error(), "damaged input packet") {
		t.Fatalf("error = %v", err)
	}
}
