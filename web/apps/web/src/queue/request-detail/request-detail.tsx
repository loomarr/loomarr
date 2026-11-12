import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { requestAcquisitions, requestFixLabel, requestStatus } from "@loomarr/core/requests";
import { Action, Badge, Card, Stack, Surface, Text } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
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
  <Stack gap="inline" render="section">
    <Text as="h2" strong textRole="headline">
      {title}
    </Text>
    {children}
  </Stack>
);

// Dates read relative here as they do in the lists ("25d ago"), never as a locale timestamp (#2738).
const when = (iso?: string): string | undefined => (iso ? formatRelative(iso) : undefined);

const RequestDetail = ({ jobId }: { jobId: string }) => {
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
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

  // The decision moves the request on: its journey, the approval list and the Requests tabs refetch.
  const decided = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: proposalJobsApi.getGetProposalJobQueryKey(jobId) }),
      queryClient.invalidateQueries({ queryKey: proposalJobsApi.getListProposalJobsQueryKey() }),
      queryClient.invalidateQueries({ queryKey: proposalsApi.getListProposalsQueryKey() }),
    ]);
  const approve = proposalsApi.useApproveProposal({
    mutation: {
      onSuccess: async (res) => {
        await decided();
        if (res.status === 200) void navigate({ to: "/channels/$id", params: { id: res.data.channelId } });
      },
    },
  });
  const deny = proposalsApi.useDenyProposal({ mutation: { onSuccess: decided } });
  const deciding = approve.isPending || deny.isPending;

  const journey = unwrap(query.data, (body) => body);
  if (query.error != null) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  // The route's loader ensured the request, so it is here before this page mounts.
  if (!journey) return null;

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
    <Stack className="h-full">
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
        <Stack align="center" direction="row" gap="inline" wrap>
          <Badge shape="square" tone={status.tone}>
            {status.line}
          </Badge>
        </Stack>

        {/* The same live surface the Guide shows while generating: stage line, titles as they are
            chosen, count and time. It hands off to the failure card above or the decision and
            lineup below as soon as the Journey moves on. */}
        {journey.milestone === "generating" && (
          <GenerationProgress phase="reasoning" progress={journey.progress} />
        )}

        {journey.failure && (
          <Card gap={4} padding={16}>
            <Text as="p" textRole="cardLabel">
              {journey.failure.message}
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              {journey.failure.guidance}
            </Text>
          </Card>
        )}

        {proposal && (
          <Section title="Decision">
            {proposal.status === "submitted" && (
              <Text as="p" textRole="compact" tone="secondary">
                Waiting for an admin to approve this request.
              </Text>
            )}
            {/* Deciding is admin-only (§11); a member only sees the wait. Approving creates the
                channel, so it ends on that channel, as it does from the approval queue. */}
            {proposal.status === "submitted" && isAdmin && (
              <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap={8}>
                <Action
                  disabled={deciding}
                  onPress={() => approve.mutate({ id: proposal.id, data: {} })}
                  tone="primary"
                >
                  {approve.isPending ? "Approving…" : "Approve"}
                </Action>
                <Action
                  disabled={deciding}
                  onPress={() => deny.mutate({ id: proposal.id, data: {} })}
                  tone="secondary"
                >
                  Deny
                </Action>
              </Surface>
            )}
            {(approve.error ?? deny.error) != null && <ErrorState error={approve.error ?? deny.error} />}
            {proposal.status === "approved" && (
              <Text as="p" textRole="compact">
                {proposal.approvedByName ? `Approved by ${proposal.approvedByName}` : "Approved"}
                {approvedAt ? ` ${approvedAt}` : ""}
                {proposal.modSummary ? `, with changes: ${proposal.modSummary}` : ""}
              </Text>
            )}
            {proposal.status === "denied" && (
              <Text as="p" textRole="compact" tone="danger">
                Not approved{proposal.denyReason ? `: ${proposal.denyReason}` : ". No reason was given."}
              </Text>
            )}
            {proposal.note && (
              <Text as="p" textRole="compact">
                {proposal.note}
              </Text>
            )}
          </Section>
        )}

        {proposal && (
          <Section title="Proposed lineup">
            {lineup.length === 0 ? (
              <Text as="p" textRole="compact" tone="secondary">
                Nothing from your library was proposed.
              </Text>
            ) : (
              <Stack gap="1" render="ul">
                {lineup.map((item, index) => (
                  // A title can repeat within a proposal, so the id alone is not a unique key.
                  <Text
                    as="li"
                    // biome-ignore lint/suspicious/noArrayIndexKey: the lineup is a fixed, ordered snapshot
                    key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                    textRole="compact"
                  >
                    {item.name}
                    {item.year ? (
                      <Text as="span" textRole="compact" tone="secondary">
                        {" "}
                        ({item.year})
                      </Text>
                    ) : null}
                  </Text>
                ))}
              </Stack>
            )}
          </Section>
        )}

        {/* An approval that has not produced its titles yet still gets the section, saying so — an
            approved request must never show a silently missing "Titles being added". */}
        {proposal?.status === "approved" && acquisitions.length === 0 && (
          <Section title="Titles being added">
            <Text as="p" textRole="compact" tone="secondary">
              {journey.milestone === "live"
                ? "All titles are on the channel."
                : requested > 0
                  ? "All titles have arrived."
                  : "Titles appear once the channel starts building."}
            </Text>
          </Section>
        )}

        {acquisitions.length > 0 && (
          <Section title="Titles being added">
            <Stack gap="inline" render="ul">
              {acquisitions.map(({ item, title }, index) => (
                <Stack
                  align="center"
                  // biome-ignore lint/suspicious/noArrayIndexKey: a title can repeat within a proposal
                  key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                  className="border-border bg-card rounded-md border px-3 py-2.5"
                  direction="row"
                  gap="control"
                  render="li"
                >
                  <Text as="span" className="min-w-0 flex-1 truncate" textRole="compact">
                    {item.name}
                    {item.year ? (
                      <Text as="span" textRole="compact" tone="secondary">
                        {" "}
                        ({item.year})
                      </Text>
                    ) : null}
                    {/* WHY it was given up (e.g. "deadline exceeded"); otherwise the row only says it failed. */}
                    {title?.state === "unavailable" && title.lastError ? (
                      <Text as="span" className="block truncate" textRole="hint" tone="secondary">
                        {title.lastError}
                      </Text>
                    ) : null}
                  </Text>
                  {title ? (
                    <StateBadge state={title.state} />
                  ) : (
                    <Text as="span" textRole="hint" tone="secondary">
                      Not requested yet
                    </Text>
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
                </Stack>
              ))}
            </Stack>
            {retryTitle.error != null && <ErrorState error={retryTitle.error} />}
          </Section>
        )}
      </div>
    </Stack>
  );
};

export { RequestDetail };
