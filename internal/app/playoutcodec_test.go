package app

import (
	"context"
	"errors"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// The §9.1 V50 decision (Q2): a channel's broadcast codec is the MAJORITY of its content's codecs,
// with everything non-HEVC counting as h264 and an even split (including the empty sample) breaking
// to h264 — the maximally-compatible choice. This pins that vote so a refactor can't quietly flip
// a tie toward HEVC and force fMP4 on a channel that isn't clearly HEVC-dominant.
func TestMajorityBroadcastCodec(t *testing.T) {
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
	boom := errors.New("database unavailable")
	access := &testkit.ChannelCodecStore{ReadRevisions: []int64{1, 1}, WriteErrors: []error{boom}}
	r := &playoutResolver{channels: access, codecs: access}

	_, err := r.ComputeChannelCodec(context.Background(), "ch1")
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want database failure", err)
	}
}
