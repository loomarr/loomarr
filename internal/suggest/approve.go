package suggest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// ErrNotSubmitted reports an approval attempt on a proposal that is not awaiting one.
var ErrNotSubmitted = errors.New("proposal is not in the submitted state")

// ApproveStore is the slice of the store approval needs.
type ApproveStore interface {
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
	UpsertTitle(ctx context.Context, rec provision.Record) error
	UpdateProposal(ctx context.Context, p store.Proposal) error
}

// Approve turns a submitted proposal into real state: in-library picks become `available`
// records so the scheduler can place them, and missing titles become `wanted` ones the
// provisioner will acquire. Returns how many acquisitions were newly enqueued.
//
// THE APPROVAL GATE HAS ONE IMPLEMENTATION. Both the admin's manual approval (§7) and
// the auto-approve grant (§8, §11) call this, so the two paths cannot drift on what
// approving means — a second copy would be a second place for the gate to be wrong.
//
// `approvedBy` is the audit record (§11): an admin's user id, or "auto" for the grant,
// so the trail distinguishes a machine decision from a human one.
func Approve(
	ctx context.Context,
	st ApproveStore,
	p store.Proposal,
	approvedBy string,
	now func() time.Time,
) (enqueued int, err error) {
	if p.Status != "submitted" {
		return 0, ErrNotSubmitted
	}
	if now == nil {
		now = time.Now
	}

	var body Proposal
	if err := json.Unmarshal([]byte(p.ProposalJSON), &body); err != nil {
		return 0, fmt.Errorf("approve: stored proposal is malformed: %w", err)
	}

	// In-library picks become `available` records so the scheduler can place them (§8:
	// "the approved lineup feeds the scheduler"). Without this, an in-library pick is
	// unresolvable and never becomes a program.
	for _, l := range body.Lineup {
		if !l.InLibrary || l.LibraryItemID == "" {
			continue // a not-in-library lineup item is covered by acquisitions
		}
		title := provision.Title{
			MediaType: provision.MediaType(l.MediaType),
			TMDBID:    l.TMDBID, TVDBID: l.TVDBID, Name: l.Name, Year: l.Year, Seasons: l.Seasons,
		}
		key, kerr := title.Key()
		if kerr != nil {
			continue // grounded proposals are always keyable; skip defensively
		}
		// Idempotent: never clobber an existing record — it may already be tracked
		// through the provisioner.
		if _, gerr := st.GetTitle(ctx, key); gerr == nil {
			continue
		} else if !errors.Is(gerr, store.ErrNotFound) {
			return 0, gerr
		}
		rec := provision.Record{
			Key: key, Title: title,
			State: provision.Available, LibraryID: l.LibraryItemID,
		}
		if err := st.UpsertTitle(ctx, rec); err != nil {
			return 0, err
		}
	}

	for _, a := range body.Acquisitions {
		key, kerr := acquisitionKey(a)
		if kerr != nil {
			continue // a grounded proposal never has an unkeyable acquisition
		}
		// Idempotent enqueue (§4 inv. 3) — only create if absent.
		if _, gerr := st.GetTitle(ctx, key); gerr == nil {
			continue
		} else if !errors.Is(gerr, store.ErrNotFound) {
			return 0, gerr
		}
		title := provision.Title{
			MediaType: provision.MediaType(a.MediaType),
			TMDBID:    a.TMDBID, TVDBID: a.TVDBID, Name: a.Name, Year: a.Year, Seasons: a.Seasons,
		}
		// Deadline = now so the acquisition is DUE IMMEDIATELY: ClaimDueTitles only takes
		// rows with `deadline <= now AND deadline > 0`, so a zero-deadline wanted title is
		// never claimed and never submitted to Seerr. (Caught by the live smoke: approved
		// acquisitions sat in `wanted` forever because the deadline was unset.)
		rec := provision.Record{Key: key, Title: title, State: provision.Wanted, Deadline: now()}
		if err := st.UpsertTitle(ctx, rec); err != nil {
			return 0, err
		}
		enqueued++
	}

	p.Status = "approved"
	p.ApprovedBy = approvedBy
	if err := st.UpdateProposal(ctx, p); err != nil {
		return 0, err
	}
	return enqueued, nil
}

// NewAcquisitions counts the acquisitions in a proposal that do NOT already exist as
// titles — the number that would actually be spent by approving it. Used by the quota
// gate, which must not charge a user for a title someone else already caused.
func NewAcquisitions(ctx context.Context, st ApproveStore, p store.Proposal) (int, error) {
	var body Proposal
	if err := json.Unmarshal([]byte(p.ProposalJSON), &body); err != nil {
		return 0, fmt.Errorf("quota: stored proposal is malformed: %w", err)
	}
	count := 0
	seen := map[string]bool{}
	for _, a := range body.Acquisitions {
		key, kerr := acquisitionKey(a)
		if kerr != nil || seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		if _, gerr := st.GetTitle(ctx, key); errors.Is(gerr, store.ErrNotFound) {
			count++
		}
	}
	return count, nil
}

// The real store must satisfy both slices. Asserted here so a signature change fails the
// BUILD rather than at the one call site that happens to wire it.
var (
	_ ApproveStore = (store.Store)(nil)
	_ QuotaStore   = (store.Store)(nil)
)
