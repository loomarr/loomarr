import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import { unwrap } from "@loomarr/api/unwrap";
import { MyRequestCard } from "@/components/loomarr/ai/my-request-card";
import { ErrorState } from "@/components/loomarr/feedback/error-state";

// MyRequests — the caller-owned Proposal Job history above the tracked-titles table (§7/§12).
//
// ⚠ The two tiers scope differently, and the distinction is load-bearing rather than
// incidental. A Proposal Job carries `created_by`, so "the requests I submitted" is answerable
// and this list is genuinely per-member (`mine=true`, resolved server-side from the session).
// A `Title` carries no requester, so "the titles acquired for me" is NOT answerable — the table
// below stays the global list, exactly as §12 records. Scoping that half is a schema change, not
// a UI filter, and pretending otherwise would be a promise the data cannot keep.
//
// The Job read is authoritative for execution. A Proposal appears only after grounded generation
// succeeds, so a Proposal-only list necessarily erases queued, running, and failed requests.

const MyRequests = () => {
  const query = proposalJobsApi.useListProposalJobs(
    { mine: true },
    {
      query: {
        // Poll only while work is active. The list stays truthful if an SSE frame is missed, while
        // terminal history remains a single bounded read.
        refetchInterval: ({ state }) => {
          const jobs = unwrap(state.data, (body) => body.proposalJobs) ?? [];
          return jobs.some((job) => job.status === "queued" || job.status === "running") ? 2_000 : false;
        },
      },
    },
  );

  const jobs = unwrap(query.data, (body) => body.proposalJobs) ?? [];

  // A member with no requests sees nothing here rather than an empty-state card: the tracked
  // titles below are the page's real content, and an "attention, you have asked for nothing"
  // panel above them would be noise on the common path.
  if (query.error == null && jobs.length === 0) return null;

  return (
    <section className="flex flex-col gap-2">
      <h2 className="font-semibold text-lg">My requests</h2>
      {query.error != null && <ErrorState error={query.error} />}
      <ul className="flex flex-col gap-2">
        {jobs.map((job) => (
          <li key={job.jobId}>
            <MyRequestCard job={job} />
          </li>
        ))}
      </ul>
    </section>
  );
};

export { MyRequests };
