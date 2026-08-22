import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { Link } from "@tanstack/react-router";
import { MyRequestCard } from "@/components/loomarr/ai/my-request-card";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

const MILESTONE = {
  generating: { label: "Generating", variant: "suggest" as const },
  awaiting_approval: { label: "Waiting for approval", variant: "suggest" as const },
  denied: { label: "Not approved", variant: "onair" as const },
  building: { label: "Building", variant: "caution" as const },
  live: { label: "Live", variant: "lock" as const },
  failed: { label: "Needs attention", variant: "onair" as const },
};

const proposalFrom = (journey: ProposalJourneyDTO): ProposalDTO | undefined => {
  if (!journey.proposal) return undefined;
  return { ...journey.proposal, jobId: journey.jobId };
};

const RequestWithoutProposal = ({ journey }: { journey: ProposalJourneyDTO }) => {
  const status = MILESTONE[journey.milestone];
  return (
    <Card className="flex flex-col gap-2 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1 font-medium">{journey.intent.description}</p>
        <Badge variant={status.variant}>{status.label}</Badge>
      </div>
      {journey.failure && <p className="text-muted-foreground text-sm">{journey.failure.message}</p>}
      {journey.actions.includes("edit") && (
        <Link
          to="/guide"
          search={{ intent: journey.intent.description }}
          className={buttonVariants({ variant: "link", size: "sm", className: "w-fit px-0" })}
        >
          Edit and try again
        </Link>
      )}
    </Card>
  );
};

// Proposal Jobs, rather than Proposal artifacts, are the recovery surface: this
// includes queued, running, and failed requests that have no Proposal yet.
const MyRequests = () => {
  const query = proposalJobsApi.useListProposalJobs(
    { mine: true },
    {
      query: {
        refetchInterval: (state) => {
          const journeys = unwrap(state.state.data, (body) => body.journeys) ?? [];
          return journeys.some((journey) => journey.milestone === "generating") ? 2_000 : false;
        },
      },
    },
  );
  const journeys = unwrap(query.data, (body) => body.journeys) ?? [];

  if (!query.error && journeys.length === 0) return null;

  return (
    <section className="flex flex-col gap-2">
      <h2 className="font-semibold text-lg">My requests</h2>
      {query.error != null && <ErrorState error={query.error} />}
      <ul className="flex flex-col gap-2">
        {journeys.map((journey) => {
          const proposal = proposalFrom(journey);
          return (
            <li key={journey.jobId}>
              {proposal ? <MyRequestCard proposal={proposal} /> : <RequestWithoutProposal journey={journey} />}
            </li>
          );
        })}
      </ul>
    </section>
  );
};

export { MyRequests };
