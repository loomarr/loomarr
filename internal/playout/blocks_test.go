package playout

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

type writeCloser struct{ bytes.Buffer }

func (*writeCloser) Close() error { return nil }

func TestPumpBlocksReportsOnlyAuthoritativeAiringTransitions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identities := []AiringIdentity{
		{StartedAt: time.Unix(1, 0).UTC(), Kind: schedule.SlotProgram, ContentID: "episode"},
		{StartedAt: time.Unix(2, 0).UTC(), Kind: schedule.SlotFiller, ContentID: "commercial"},
	}
	next := 0
	source := BlockSource(func(context.Context, string, EncodePlan) (Block, error) {
		if next == len(identities) {
			cancel()
			return Block{}, context.Canceled
		}
		block := Block{
			Content:  io.NopCloser(strings.NewReader(string(rune('a' + next)))),
			Identity: identities[next],
		}
		next++
		return block, nil
	})
	var output writeCloser
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	pumpBlocks(ctx, &output, source, "channel", PlanBaseline, log)

	if got := output.String(); got != "ab" {
		t.Fatalf("mux input = %q, want both finite blocks", got)
	}
	line := logs.String()
	for _, want := range []string{
		"msg=\"playout: block transition\"", "from_kind=program", "from_content=episode",
		"to_kind=filler", "to_content=commercial",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("transition log missing %q:\n%s", want, line)
		}
	}
}
