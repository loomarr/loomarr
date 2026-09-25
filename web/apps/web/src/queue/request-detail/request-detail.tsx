import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, RotateCw } from "lucide-react";
import type { ReactNode } from "react";
import { useAuth } from "@/auth/use-auth";
import { StateBadge } from "@/components/loomarr/channels/state-badge";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { requestAcquisitions, requestFixLabel, requestStatus } from "@/queue/request-status";
import { useRequests } from "@/queue/use-requests";

// One request, end to end (#1405) — the Proposal Job journey (GET /v1/proposal-jobs/{jobId}):
// what was asked, what was proposed, who decided and when, how each title is coming along, the
// channel it became, and the actions that make sense from here. Clicking a request on the Requests
// page used to do nothing; this is what it opens.
//
// Members can open only their own request and admins any (the endpoint enforces it), so a 403/404
// is shown as the API's own problem message rather than an empty page.

const Section = ({ title, children }: { title: string; children: ReactNode }) => (
  <section className="flex flex-col gap-2">
    <h2 className="font-semibold text-lg">{title}</h2>
    {children}
  </section>
);

const when = (iso?: string): string | undefined => (iso ? new Date(iso).toLocaleString() : undefined);

const RequestDetail = ({ jobId }: { jobId: string }) => {
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const query = proposalJobsApi.useGetProposalJob(jobId);
  const { titles } = useRequests();
  const retryTitle = titlesApi.useEnqueueTitle({
    mutation: {
      // A re-enqueue moves the title back to `wanted`, and the reconciler may advance it before we
      // refetch, so every state's list is stale — the same invalidation the old In flight tab used.
      onSuccess: () =>
        Promise.all(
          Object.values(TitleDTOState).map((state) =>
            queryClient.invalidateQueries({ queryKey: titlesApi.getListTitlesQueryKey({ state }) }),
          ),
        ),
    },
  });

  const journey = unwrap(query.data, (body) => body);
  if (query.error != null) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (!journey) return <p className="p-6 text-muted-foreground text-sm">Loading…</p>;

  const status = requestStatus(journey, titles);
  const acquisitions = requestAcquisitions(journey, titles);
  const proposal = journey.proposal;
  const fix = requestFixLabel(journey);
  const lineup = proposal?.proposal.lineup ?? [];
  const approvedAt = when(proposal?.approvedAt);

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={journey.intent.description}
        description={`Requested ${when(journey.createdAt)}`}
        actions={
          <>
            <Link to="/requests" className={buttonVariants({ variant: "ghost", size: "sm" })}>
              <ArrowLeft aria-hidden />
              Requests
            </Link>
            {journey.channel && (
              <Link
                to="/channels/$id"
                params={{ id: journey.channel.id }}
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                Open channel
              </Link>
            )}
            {fix && (
              <Link
                to="/guide"
                search={{ job: journey.jobId }}
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                {fix}
              </Link>
            )}
          </>
        }
      />
      <div className="flex max-w-3xl flex-1 flex-col gap-6 overflow-auto p-6">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={status.tone}>{status.line}</Badge>
        </div>

        {journey.failure && (
          <Card className="flex flex-col gap-1 p-4">
            <p className="font-medium">{journey.failure.message}</p>
            <p className="text-muted-foreground text-sm">{journey.failure.guidance}</p>
          </Card>
        )}

        {proposal && (
          <Section title="Decision">
            {proposal.status === "submitted" && (
              <p className="text-muted-foreground text-sm">Waiting for an admin to approve this request.</p>
            )}
            {proposal.status === "approved" && (
              <p className="text-sm">
                {proposal.approvedByName ? `Approved by ${proposal.approvedByName}` : "Approved"}
                {approvedAt ? ` on ${approvedAt}` : ""}
                {proposal.modSummary ? `, with changes: ${proposal.modSummary}` : ""}
              </p>
            )}
            {proposal.status === "denied" && (
              <p className="text-onair-300 text-sm">
                Not approved{proposal.denyReason ? `: ${proposal.denyReason}` : ". No reason was given."}
              </p>
            )}
            {proposal.note && <p className="text-sm">{proposal.note}</p>}
          </Section>
        )}

        {proposal && (
          <Section title="Proposed lineup">
            {lineup.length === 0 ? (
              <p className="text-muted-foreground text-sm">Nothing from your library was proposed.</p>
            ) : (
              <ul className="flex flex-col gap-1 text-sm">
                {lineup.map((item) => (
                  <li key={`${item.mediaType}:${item.tmdbId ?? item.name}`}>
                    {item.name}
                    {item.year ? <span className="text-muted-foreground"> ({item.year})</span> : null}
                  </li>
                ))}
              </ul>
            )}
          </Section>
        )}

        {/* An approval that has not produced its titles yet still gets the section, saying so — an
            approved request must never show a silently missing "Titles being added". */}
        {proposal?.status === "approved" && acquisitions.length === 0 && (
          <Section title="Titles being added">
            <p className="text-muted-foreground text-sm">
              {journey.milestone === "live"
                ? "Everything in this channel was already in your library."
                : "Titles appear once the channel starts building."}
            </p>
          </Section>
        )}

        {acquisitions.length > 0 && (
          <Section title="Titles being added">
            <ul className="flex flex-col gap-2">
              {acquisitions.map(({ item, title }) => (
                <li
                  key={`${item.mediaType}:${item.tmdbId ?? item.name}`}
                  className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2.5"
                >
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {item.name}
                    {item.year ? <span className="text-muted-foreground"> ({item.year})</span> : null}
                    {/* WHY it was given up (e.g. "deadline exceeded"); otherwise the row only says it failed. */}
                    {title?.state === "unavailable" && title.lastError ? (
                      <span className="block truncate text-muted-foreground text-xs">{title.lastError}</span>
                    ) : null}
                  </span>
                  {title ? (
                    <StateBadge state={title.state} />
                  ) : (
                    <span className="text-muted-foreground text-xs">Not requested yet</span>
                  )}
                  {/* Re-enqueue puts a given-up title back in the queue (§4). Admin-only, as is
                      POST /v1/titles — a member would only be offered a button that 403s. */}
                  {isAdmin && title?.state === "unavailable" && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={retryTitle.isPending}
                      onClick={() =>
                        retryTitle.mutate({
                          data: {
                            mediaType: title.mediaType,
                            tmdbId: title.tmdbId,
                            tvdbId: title.tvdbId,
                            name: title.name,
                            year: title.year,
                          },
                        })
                      }
                    >
                      <RotateCw aria-hidden />
                      Try again
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            {retryTitle.error != null && <ErrorState error={retryTitle.error} />}
          </Section>
        )}
      </div>
    </div>
  );
};

export { RequestDetail };
