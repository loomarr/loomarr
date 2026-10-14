package app

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The §9.1 V50 decision (Q2): a channel's broadcast codec is the MAJORITY of its content's codecs,
// with everything non-HEVC counting as h264 and an even split (including the empty sample) breaking
// to h264 — the maximally-compatible choice. This pins that vote so a refactor can't quietly flip
// a tie toward HEVC and force fMP4 on a channel that isn't clearly HEVC-dominant.
func TestMajorityBroadcastCodec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		codecs []string
		want   string
	}{
		{"empty sample defaults h264", nil, store.BroadcastCodecH264},
		{"all h264", []string{"h264", "h264", "h264"}, store.BroadcastCodecH264},
		{"all hevc", []string{"hevc", "hevc"}, store.BroadcastCodecHEVC},
		{"h265 alias counts as hevc", []string{"h265", "h265"}, store.BroadcastCodecHEVC},
		{"hevc majority wins", []string{"hevc", "hevc", "h264"}, store.BroadcastCodecHEVC},
		{"h264 majority wins", []string{"h264", "h264", "hevc"}, store.BroadcastCodecH264},
		{"even split breaks to h264", []string{"hevc", "h264"}, store.BroadcastCodecH264},
		// Non-h264/non-hevc sources (vp9, mpeg2, theora) all normalize DOWN to h264, so they count
		// against HEVC — a channel of mixed legacy codecs broadcasts h264, never HEVC.
		{"legacy codecs count as not-hevc", []string{"vp9", "mpeg2video", "hevc"}, store.BroadcastCodecH264},
		{"hevc beats a lone legacy codec", []string{"hevc", "hevc", "vp9"}, store.BroadcastCodecHEVC},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := majorityBroadcastCodec(tc.codecs); got != tc.want {
				t.Errorf("majorityBroadcastCodec(%v) = %q, want %q", tc.codecs, got, tc.want)
			}
		})
	}
}

func TestComputeChannelCodecRetriesAChangingChannel(t *testing.T) {
	t.Parallel()
	access := &testkit.ChannelCodecStore{ReadRevisions: []int64{1, 2, 2, 2}}
	r := &playoutResolver{channels: access, codecs: access}

	got, err := r.ComputeChannelCodec(context.Background(), "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if got != store.BroadcastCodecH264 {
		t.Fatalf("codec = %q, want h264 fallback", got)
	}
	if len(access.Writes) != 1 || access.Writes[0].ExpectedRevision != 2 {
		t.Fatalf("writes = %+v, want one write against stable revision 2", access.Writes)
	}
}

func TestComputeChannelCodecRetriesAStaleTargetedWrite(t *testing.T) {
	t.Parallel()
	access := &testkit.ChannelCodecStore{
		ReadRevisions: []int64{1, 1, 2, 2},
		WriteErrors:   []error{store.ErrChannelStale, nil},
	}
	r := &playoutResolver{channels: access, codecs: access}

	if _, err := r.ComputeChannelCodec(context.Background(), "ch1"); err != nil {
		t.Fatal(err)
	}
	if len(access.Writes) != 2 || access.Writes[0].ExpectedRevision != 1 || access.Writes[1].ExpectedRevision != 2 {
		t.Fatalf("writes = %+v, want retries against revisions 1 then 2", access.Writes)
	}
}

func TestComputeChannelCodecDoesNotRetryNonStaleWriteError(t *testing.T) {
	t.Parallel()
	boom := errors.New("database unavailable")
	access := &testkit.ChannelCodecStore{ReadRevisions: []int64{1, 1}, WriteErrors: []error{boom}}
	r := &playoutResolver{channels: access, codecs: access}

	_, err := r.ComputeChannelCodec(context.Background(), "ch1")
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want database failure", err)
	}
}

func TestMeasureChannelCodecPinsOneLibraryConnectionAcrossBatch(t *testing.T) {
	t.Parallel()
	providerCalls := 0
	lib := library.NewDynamic(func() library.Connection {
		providerCalls++
		if providerCalls == 1 {
			return library.Connection{
				Flavor: library.Emby, BaseURL: "https://emby-a.invalid", Token: "token-a",
			}
		}
		return library.Connection{
			Flavor: library.Jellyfin, BaseURL: "https://jellyfin-b.invalid", Token: "token-b",
		}
	}, "device-1")
	var inputs []string
	r := &playoutResolver{
		engine: stubCycle{slots: []schedule.Slot{
			{Kind: schedule.SlotProgram, LibraryItemID: "item-1"},
			{Kind: schedule.SlotProgram, LibraryItemID: "item-2"},
		}},
		lib: lib,
		now: time.Now,
		probeFormat: func(_ context.Context, input string) (playout.MediaFormat, error) {
			inputs = append(inputs, input)
			return playout.MediaFormat{VideoCodec: "hevc"}, nil
		},
	}

	if got := r.measureChannelCodec(context.Background(), "channel-1"); got != store.BroadcastCodecHEVC {
		t.Fatalf("codec = %q, want hevc", got)
	}
	if providerCalls != 1 {
		t.Fatalf("connection provider calls = %d, want one for the full batch", providerCalls)
	}
	if len(inputs) != 2 {
		t.Fatalf("probe inputs = %d, want 2", len(inputs))
	}
	for _, input := range inputs {
		parsed, err := url.Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Host != "emby-a.invalid" || parsed.Query().Get("api_key") != "token-a" {
			t.Fatalf("probe input = %q, want pinned first connection", input)
		}
	}
}
