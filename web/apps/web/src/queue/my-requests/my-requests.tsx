import { suggestionsApi, unwrap } from "@loomarr/api";
import { useQueries } from "@tanstack/react-query";
import { ErrorState, MyRequestCard } from "@/components/loomarr";

// MyRequests — the first tier of "My requests" (V26 / `A2`, §12): the REQUESTS a member
// submitted, above the tracked-titles table that has always been the second tier.
//
// ⚠ The two tiers scope differently, and the distinction is load-bearing rather than
// incidental. A `Proposal` carries `created_by`, so "the requests I submitted" is answerable
// and this list is genuinely per-member (`mine=true`, resolved server-side from the session).
// A `Title` carries no requester, so "the titles acquired for me" is NOT answerable — the table
// below stays the global list, exactly as §12 records. Scoping that half is a schema change, not
// a UI filter, and pretending otherwise would be a promise the data cannot keep.
//
// Three statuses are fetched because a member's question is "what happened to what I asked
// for?", and the two answers that matter most — it was declined, or it came back changed — live
// outside `submitted`. `GET /v1/suggestions` filters by ONE status per call, so this fans out
// and merges, the same aggregation the queue's title table already does.
const STATUSES = ["submitted", "approved", "denied"] as const;

const MyRequests = () => {
  const queries = useQueries({
    queries: STATUSES.map((status) => suggestionsApi.getListProposalsQueryOptions({ status, mine: true })),
  });

  const error = queries.find((q) => q.error)?.error;
  const proposals = queries.flatMap((q) => unwrap(q.data, (b) => b.proposals) ?? []);

  // A member with no requests sees nothing here rather than an empty-state card: the tracked
  // titles below are the page's real content, and an "attention, you have asked for nothing"
  // panel above them would be noise on the common path.
  if (!error && proposals.length === 0) return null;

  return (
    <section className="flex flex-col gap-2">
      <h2 className="font-semibold text-lg">My requests</h2>
      {error != null && <ErrorState error={error} />}
      <ul className="flex flex-col gap-2">
        {proposals.map((p) => (
          <li key={p.id}>
            <MyRequestCard proposal={p} />
          </li>
        ))}
      </ul>
    </section>
  );
};

export { MyRequests };
