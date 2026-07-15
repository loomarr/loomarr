package requester

import (
	"encoding/json"
	"testing"

	"github.com/mantonx/loomarr/internal/testkit"
)

// FORWARD-LOOKING CONTRACT PIN.
//
// The Seerr adapter (seerr.go) is deliberately status-only today: Request treats
// any 2xx (incl. the observed 201-with-existing-media) and 409 as success and
// never reads the response body (§6 idempotency; Phase-0 findings in
// fixtures/seerr/FINDINGS.md). This test does NOT change that — it PINS the shape
// of the 201 body Seerr actually returns, so that if/when Loomarr starts reading
// the body (e.g. to short-circuit on media.status "available" without re-queuing,
// or to surface the created request id), the fields it would depend on are known
// to parse against the real capture rather than remembered field names.
//
// The pinned capture is fixtures/seerr/request_available_201.json — a live 201 for
// an already-available movie (tmdbId 1241983, media.status 5). FINDINGS.md calls
// out the two fields that matter for the idempotency decision: the top-level
// request `type` ("movie") and `media.status` (5 = Available), where
// media.downloadStatus == [] confirms no new acquisition was queued (safe).
type seerrRequestResponse struct {
	// Top-level request record fields.
	Type   string `json:"type"`   // "movie" | "tv"
	Status int    `json:"status"` // Seerr request status
	Is4K   bool   `json:"is4k"`

	// media is the linked media record; its status is what tells Loomarr the title
	// is already available (5) vs still processing.
	Media struct {
		ID             int               `json:"id"`
		MediaType      string            `json:"mediaType"`
		TMDBID         int               `json:"tmdbId"`
		Status         int               `json:"status"`         // 5 == Available
		DownloadStatus []json.RawMessage `json:"downloadStatus"` // [] == nothing queued (safe)
	} `json:"media"`
}

// Seerr media status codes (from the FINDINGS.md capture): 5 == Available.
const seerrStatusAvailable = 5

func TestSeerr201Body_ContractPin(t *testing.T) {
	raw := testkit.Fixture(t, "seerr/request_available_201.json")

	var resp seerrRequestResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("pinned Seerr 201 body must decode into the forward-looking response struct: %v", err)
	}

	// The fields Loomarr would need if it read the body:
	if resp.Type != "movie" {
		t.Errorf("request type = %q, want \"movie\" (top-level discriminator per FINDINGS.md)", resp.Type)
	}
	if resp.Media.Status != seerrStatusAvailable {
		t.Errorf("media.status = %d, want %d (Available) — the field the idempotency short-circuit would read", resp.Media.Status, seerrStatusAvailable)
	}
	if resp.Media.TMDBID != 1241983 {
		t.Errorf("media.tmdbId = %d, want 1241983 (the pinned already-available movie)", resp.Media.TMDBID)
	}
	if resp.Media.ID != 2728 {
		t.Errorf("media.id = %d, want 2728 (the existing media record returned on the 201)", resp.Media.ID)
	}
	// downloadStatus == [] is the "no new acquisition queued" safety signal.
	if len(resp.Media.DownloadStatus) != 0 {
		t.Errorf("media.downloadStatus should be empty (nothing queued) on an already-available 201, got %d entries", len(resp.Media.DownloadStatus))
	}
}
