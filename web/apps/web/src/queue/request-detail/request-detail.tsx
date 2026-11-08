import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { requestAcquisitions, requestFixLabel, requestStatus } from "@loomarr/core/requests";
import { Action, Badge, Card } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useAuth } from "@/auth/use-auth";
import { StateBadge } from "@/components/loomarr/channels/state-badge";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { PageHeader } from "@/components/loomarr/shell/page-header";
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
    <h2 className="text-lg font-semibold">{title}</h2>
    {children}
  </section>
);

const when = (iso?: string): string | undefined => (iso ? new Date(iso).toLocaleString() : undefined);

const RequestDetail = ({ jobId }: { jobId: string }) => {
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const query = proposalJobsApi.useGetProposalJob(jobId, {
    query: {
      // A generating request streams its progress; the SSE frame refetches sooner, this is the floor.
      refetchInterval: (q) =>
        q.state.data?.status === 200 && q.state.data.data.milestone === "generating" ? 2_000 : false,
    },
  });
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
  if (!journey) return <p className="text-muted-foreground p-6 text-sm">Loading…</p>;

  const status = requestStatus(journey, titles);
  // Only what is still outstanding: a title that has landed is part of the channel, so listing it
  // as "being added" (or badging it AVAILABLE) contradicts a live channel.
  const asked = requestAcquisitions(journey, titles);
  const requested = asked.length;
  const acquisitions = asked.filter(({ title }) => title?.state !== "available");
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
            <Action accessibilityRole="link" icon="back" render={<Link to="/requests" />} variant="ghost">
              Requests
            </Action>
            {journey.channel && (
              <Action
                accessibilityRole="link"
                render={<Link to="/channels/$id" params={{ id: journey.channel.id }} />}
                variant="outline"
              >
                Open channel
              </Action>
            )}
            {fix && (
              <Action
                accessibilityRole="link"
                render={<Link to="/requests/new" search={{ job: journey.jobId }} />}
                variant="outline"
              >
                {fix}
              </Action>
            )}
          </>
        }
      />
      <div className="max-w-reading mx-auto flex w-full flex-1 flex-col gap-6 overflow-auto p-6">
        <div className="flex flex-wrap items-center gap-2">
          <Badge shape="square" tone={status.tone}>
            {status.line}
          </Badge>
        </div>

        {/* The same live surface the Guide shows while generating: stage line, titles as they are
            chosen, count and time. It hands off to the failure card above or the decision and
            lineup below as soon as the Journey moves on. */}
        {journey.milestone === "generating" && (
          <GenerationProgress phase="reasoning" progress={journey.progress} />
        )}

        {journey.failure && (
          <Card gap={4} padding={16}>
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
                {lineup.map((item, index) => (
                  // A title can repeat within a proposal, so the id alone is not a unique key.
                  // biome-ignore lint/suspicious/noArrayIndexKey: the lineup is a fixed, ordered snapshot
                  <li key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}>
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
                ? "All titles are on the channel."
                : requested > 0
                  ? "All titles have arrived."
                  : "Titles appear once the channel starts building."}
            </p>
          </Section>
        )}

        {acquisitions.length > 0 && (
          <Section title="Titles being added">
            <ul className="flex flex-col gap-2">
              {acquisitions.map(({ item, title }, index) => (
                <li
                  // biome-ignore lint/suspicious/noArrayIndexKey: a title can repeat within a proposal
                  key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                  className="border-border bg-card flex items-center gap-3 rounded-md border px-3 py-2.5"
                >
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {item.name}
                    {item.year ? <span className="text-muted-foreground"> ({item.year})</span> : null}
                    {/* WHY it was given up (e.g. "deadline exceeded"); otherwise the row only says it failed. */}
                    {title?.state === "unavailable" && title.lastError ? (
                      <span className="text-muted-foreground block truncate text-xs">{title.lastError}</span>
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
                    <Action
                      disabled={retryTitle.isPending}
                      icon="refresh"
                      variant="outline"
                      onPress={() =>
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
                      Try again
                    </Action>
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
