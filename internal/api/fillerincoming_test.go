package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

type incomingBody struct {
	Asks []struct {
		Path         string `json:"path"`
		SuggestedEra int    `json:"suggestedEra"`
		Reason       string `json:"reason"`
	} `json:"asks"`
	Reels []struct {
		ProposalID     string `json:"proposalId"`
		ClipHash       string `json:"clipHash"`
		ClipName       string `json:"clipName"`
		Segments       int    `json:"segments"`
		NeedsAttention int    `json:"needsAttention"`
	} `json:"reels"`
	Total int `json:"total"`
}

func getIncoming(t *testing.T, url, token string) (*http.Response, incomingBody) {
	t.Helper()
	res := sourceReq(t, http.MethodGet, url, "", token)
	var b incomingBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
			t.Fatal(err)
		}
	}
	return res, b
}

func putClip(t *testing.T, st store.Store, c filler.Clip) {
	t.Helper()
	// ⚠ Identity is the HASH since V38c (§10). Fixtures across these tests name clips by a
	// readable path, so default the id from it rather than making every literal carry both —
	// a `filler.Clip{Path: …}` with no Hash has an EMPTY id, which makes every lookup miss and
	// every pin silently fail to match.
	if c.Hash == "" {
		c.Hash = c.Path
	}
	if err := st.UpsertClip(context.Background(), store.Clip{Clip: c, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

// The two asks are different QUESTIONS, and the queue must not collapse them: an ungrounded era
// has a proposed answer to confirm, while an untagged commercial has nothing to confirm.
func TestFillerIncoming_QueuesTheTwoAskKindsSeparately(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	// ⚠ HELD, because V38 made waiting a STATE rather than something inferred from missing tags.
	// A clip that is not held is in the catalog and is nobody's decision, however it is tagged.
	putClip(t, st, filler.Clip{
		Path: "guess.mp4", Name: "guess.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 0, Audience: filler.Kids, Category: "toys", SuggestedEra: 1988, Held: true,
	})
	putClip(t, st, filler.Clip{
		Path: "mystery.mp4", Name: "mystery.mp4", Kind: filler.Commercial, DurationMs: 25_000,
		Held: true,
	})
	// Filed and fully tagged: not an ask.
	putClip(t, st, filler.Clip{
		Path: "known.mp4", Name: "known.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 1992, Audience: filler.Kids, Category: "cereal",
	})

	res, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(body.Asks) != 2 {
		t.Fatalf("asks = %d, want 2 — a fully tagged clip is not waiting on anyone", len(body.Asks))
	}
	// An ungrounded era sorts first: it is one click, where an untagged clip needs everything.
	if body.Asks[0].Path != "guess.mp4" || body.Asks[0].SuggestedEra != 1988 {
		t.Errorf("asks[0] = %+v, want the ungrounded-era clip first", body.Asks[0])
	}
	if body.Asks[0].Reason == body.Asks[1].Reason {
		t.Error("both asks carry the same reason — the two questions have been collapsed into one")
	}
}

// ⚠ Bumpers and station IDs do their bookend job with no era/audience/category, so queueing them
// would fill the review with work that changes nothing. Same rule the AI-tagging job applies.
func TestFillerIncoming_DoesNotQueueBumpers(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "bump.mp4", Name: "bump.mp4", Kind: filler.Bumper, DurationMs: 5_000})
	putClip(t, st, filler.Clip{Path: "ident.mp4", Name: "ident.mp4", Kind: filler.StationID, DurationMs: 4_000})

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Asks) != 0 {
		t.Errorf("queued %d bookend clips: %+v", len(body.Asks), body.Asks)
	}
}

// A reel of twelve clean segments and a reel with three problems are different amounts of work,
// and the queue should say which is which before it is opened.
func TestFillerIncoming_CountsSegmentsNeedingAttention(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_1", ClipHash: "hash-of-comps/1987.mp4", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{
			{Index: 0, StartMs: 0, EndMs: 30_000, Name: "clean"},
			{Index: 1, StartMs: 30_000, EndMs: 61_000, Name: "dup", DupOf: "old/ad.mp4"},
			{Index: 2, StartMs: 61_000, EndMs: 149_000, Name: "stuck", Unsplittable: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Reels) != 1 {
		t.Fatalf("reels = %d, want 1", len(body.Reels))
	}
	if body.Reels[0].Segments != 3 {
		t.Errorf("segments = %d, want 3", body.Reels[0].Segments)
	}
	if body.Reels[0].NeedsAttention != 2 {
		t.Errorf("needsAttention = %d, want 2 (a duplicate and an unsplittable stretch)", body.Reels[0].NeedsAttention)
	}
	// ⚠ No clip backs this proposal, so the name falls back to the identity rather than rendering
	// blank. A reel whose compilation has been deleted is a real state an operator must still be
	// able to see and dismiss.
	if body.Reels[0].ClipName != "hash-of-comps/1987.mp4" {
		t.Errorf("clipName = %q, want the hash as the fallback for a missing clip", body.Reels[0].ClipName)
	}
}

// The reel row is titled with the compilation's NAME. ⚠ Its identity is a 64-character hash, which
// is not a row title — and the field it replaced (`clipPath`) only looked acceptable because the
// fixtures used friendly filenames no real catalog contains: a filed clip's path is
// `a3/f9/<hash>.mp4`.
func TestFillerIncoming_ReelCarriesTheCompilationName(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Hash: "a3f9deadbeef", Path: "a3/f9/a3f9deadbeef.mp4",
		Name: "1987 Saturday morning block", Kind: filler.Commercial, DurationMs: 149_000,
	})
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_1", ClipHash: "a3f9deadbeef", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000, Name: "clean"}},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Reels) != 1 {
		t.Fatalf("reels = %d, want 1", len(body.Reels))
	}
	if body.Reels[0].ClipName != "1987 Saturday morning block" {
		t.Errorf("clipName = %q, want the catalog name", body.Reels[0].ClipName)
	}
	if body.Reels[0].ClipHash != "a3f9deadbeef" {
		t.Errorf("clipHash = %q — the identity must still be carried alongside the name",
			body.Reels[0].ClipHash)
	}
}

// Both halves in ONE read. A client fetching them separately renders a half-empty queue whenever
// one call is slower — which is exactly when the queue matters.
func TestFillerIncoming_TotalCoversBothHalves(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "mystery.mp4", Name: "mystery.mp4", Kind: filler.Commercial, DurationMs: 25_000, Held: true})
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_1", ClipHash: "hash-of-comps/a.mp4", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000, Name: "a"}},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if body.Total != 2 {
		t.Errorf("total = %d, want 2 — it must cover asks AND reels", body.Total)
	}
}

// Empty is an array, never null: a null makes every consumer guard before iterating, and "nothing
// needs you" is a real answer the tab renders as its all-clear state.
func TestFillerIncoming_EmptyHalvesAreArrays(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/incoming", "", adminToken)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"asks", "reels"} {
		if got := string(raw[k]); got != "[]" {
			t.Errorf("%s = %s, want []", k, got)
		}
	}
}

// §19 negative: the queue names filesystem paths and drives what gets filed.
func TestFillerIncoming_RequiresAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	if res, _ := getIncoming(t, srv.URL+"/v1/filler/incoming", ""); res.StatusCode == http.StatusOK {
		t.Error("served the incoming queue with no credential")
	}
}
