package suggest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// ErrNotSubmitted reports an approval attempt on a proposal that is not awaiting one.
var ErrNotSubmitted = store.ErrProposalNotSubmitted

// ApprovalEdit is an approver's modification, applied AT the gate (§7, decision D-K).
//
// ⚠ THE EDIT IS A PARAMETER TO Approve, not something a caller applies first, and that is the
// whole design. Approving decides what gets acquired; if a handler pre-modified the proposal
// and passed the result, the decision would live outside the gate and the auto-approve path
// would not share it — the two would drift on what approving means, which is exactly what §8's
// one-implementation rule exists to prevent. Auto-approve passes nil and runs identical code.
type ApprovalEdit struct {
	// DropKeys are provisioning keys the approver removed. Applied to BOTH lineup and
	// acquisitions: a dropped title should not be acquired and should not be scheduled.
	DropKeys []provision.Key
	// Add are titles the approver added via search. They join the acquisitions list and go
	// through the same idempotent enqueue as anything the model proposed — an
	// admin-added title is not privileged, it is just another acquisition.
	Add []ProposalItem
	// Note is the approver's message to the requester, persisted for the audit trail.
	Note string
}

// isEmpty reports whether this edit changes nothing, so an unmodified approval records an
// empty ModSummary rather than a misleading "dropped 0, added 0".
func (e *ApprovalEdit) isEmpty() bool {
	return e == nil || (len(e.DropKeys) == 0 && len(e.Add) == 0)
}

// ApproveStore is the slice of the store approval needs.
type ApproveStore interface {
	// GetTitle is read-only quota input. The approval write itself is deliberately
	// exposed only as the atomic commit below.
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
	CommitProposalApproval(ctx context.Context, commit store.ProposalApproval) (int, error)
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
	edit *ApprovalEdit,
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

	// The edit is applied HERE, before anything is enqueued, so what gets acquired is decided
	// in one place for both callers. `applyEdit` also re-serialises the body back onto the
	// proposal: the stored record must show what was actually approved, not what the model
	// originally proposed — otherwise the audit trail describes a lineup that never existed.
	summary, editedJSON, aerr := applyEdit(&body, edit)
	if aerr != nil {
		return 0, aerr
	}
	if summary != "" {
		p.ModSummary = summary
	}
	if editedJSON != "" {
		p.ProposalJSON = editedJSON
	}
	if edit != nil && edit.Note != "" {
		p.Note = edit.Note
	}

	// Prepare every candidate before entering the store transaction. In-library picks become
	// `available` records so the scheduler can place them (§8:
	// "the approved lineup feeds the scheduler"). Without this, an in-library pick is
	// unresolvable and never becomes a program.
	titles := make([]provision.Record, 0, len(body.Lineup)+len(body.Acquisitions))
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
		titles = append(titles, provision.Record{
			Key: key, Title: title,
			State: provision.Available, LibraryID: l.LibraryItemID,
		})
	}

	decisionAt := now()
	for _, a := range body.Acquisitions {
		key, kerr := acquisitionKey(a)
		if kerr != nil {
			continue // a grounded proposal never has an unkeyable acquisition
		}
		title := provision.Title{
			MediaType: provision.MediaType(a.MediaType),
			TMDBID:    a.TMDBID, TVDBID: a.TVDBID, Name: a.Name, Year: a.Year, Seasons: a.Seasons,
		}
		// Deadline = now so the acquisition is DUE IMMEDIATELY: ClaimDueTitles only takes
		// rows with `deadline <= now AND deadline > 0`, so a zero-deadline wanted title is
		// never claimed and never submitted to Seerr. (Caught by the live smoke: approved
		// acquisitions sat in `wanted` forever because the deadline was unset.)
		titles = append(titles, provision.Record{Key: key, Title: title, State: provision.Wanted, Deadline: decisionAt})
	}

	p.Status = "approved"
	p.ApprovedBy = approvedBy
	// Stamped HERE, at the one chokepoint, for the same reason `approvedBy` is: every path that
	// approves — a human admin, the per-user auto-approve grant, auto-curate, and V27's bulk
	// approve — goes through this function, so none of them can record a decision without a
	// time. `now` is injected, so the stamp is deterministic under test rather than wall-clock.
	p.ApprovedAt = decisionAt
	p.UpdatedAt = decisionAt
	return st.CommitProposalApproval(ctx, store.ProposalApproval{Proposal: p, Titles: titles})
}

// applyEdit removes dropped titles, appends added ones, and re-serialises the result onto the
// proposal. Returns a server-generated summary of what changed.
//
// The summary is GENERATED, never taken from the caller: "dropped 2, added 1" written by the
// code is a record of what happened, while the same string typed by an approver is a claim
// about it. The audit trail is only worth keeping if it cannot be authored.
//
// Dropping applies to lineup AND acquisitions. A dropped title must not be acquired and must
// not be scheduled, and a proposal can carry the same title in either list depending on whether
// it was already in the library — so filtering one and not the other would drop it from the
// acquisition queue while leaving it in the channel, or the reverse.
// Returns the summary and the re-serialised proposal JSON; an empty JSON string means the
// stored bytes should be left exactly as they were.
func applyEdit(body *Proposal, edit *ApprovalEdit) (summary string, editedJSON string, err error) {
	if edit.isEmpty() {
		// An unmodified approval leaves ProposalJSON untouched, so its bytes stay
		// byte-identical to what the approver actually reviewed.
		return "", "", nil
	}

	drop := make(map[provision.Key]bool, len(edit.DropKeys))
	for _, k := range edit.DropKeys {
		drop[k] = true
	}

	keep := func(items []ProposalItem) ([]ProposalItem, int) {
		out := make([]ProposalItem, 0, len(items))
		dropped := 0
		for _, it := range items {
			k, err := acquisitionKey(it)
			// An unkeyable item cannot have been named in DropKeys, so it survives. Dropping
			// it "just in case" would silently remove something the approver never chose to.
			if err == nil && drop[k] {
				dropped++
				continue
			}
			out = append(out, it)
		}
		return out, dropped
	}

	var droppedTotal int
	var n int
	body.Lineup, n = keep(body.Lineup)
	droppedTotal += n
	body.Acquisitions, n = keep(body.Acquisitions)
	droppedTotal += n

	body.Acquisitions = append(body.Acquisitions, edit.Add...)

	raw, merr := json.Marshal(body)
	if merr != nil {
		return "", "", fmt.Errorf("approve: re-serialising the edited proposal: %w", merr)
	}

	parts := make([]string, 0, 2)
	if droppedTotal > 0 {
		parts = append(parts, fmt.Sprintf("dropped %d", droppedTotal))
	}
	if len(edit.Add) > 0 {
		parts = append(parts, fmt.Sprintf("added %d", len(edit.Add)))
	}
	return strings.Join(parts, ", "), string(raw), nil
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
		} else if gerr != nil {
			return 0, fmt.Errorf("quota: read title %s: %w", key, gerr)
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
