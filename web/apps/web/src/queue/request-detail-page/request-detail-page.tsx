import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { requestFixLabel, requestStatus } from "@loomarr/core/requests";
import { Action, Stack, Surface } from "@loomarr/design-system";
import { RequestDetailBody } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { StateBadge } from "@/components/loomarr/channels/state-badge";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useRequests } from "@/queue/use-requests";

// One request, end to end (#1405) — the Proposal Job journey (GET /v1/proposal-jobs/{jobId}). It
// is what clicking a request on the Requests page opens. This page fetches, decides and navigates;
// RequestDetailBody draws the request.
//
// Members can open only their own request and admins any (the endpoint enforces it), so a 403/404
// is shown as the API's own problem message rather than an empty page.

const RequestDetailPage = ({ jobId }: { jobId: string }) => {
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  // A generating request follows its suggestion frames: the bus refetches /v1/proposal-jobs on each.
  const query = proposalJobsApi.useGetProposalJob(jobId);
  const { titles } = useRequests();
  const retryTitle = titlesApi.useEnqueueTitle({
    mutation: {
      // A re-enqueue moves the title back to `wanted`, and the reconciler may advance it before we
      // refetch, so every state's list is stale and all of them are invalidated.
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

  const entry = { journey, status: requestStatus(journey, titles) };
  const proposal = journey.proposal;
  const fix = requestFixLabel(journey);
  const decisionError = approve.error ?? deny.error;

  return (
    <Stack height="100%">
      <PageHeader
        title={journey.intent.description}
        // Dates read relative here as they do in the lists ("25d ago"), never as a locale timestamp (#2738).
        description={`Requested ${formatRelative(journey.createdAt)}`}
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
      <Stack
        flexBasis="0%"
        flexGrow={1}
        flexShrink={1}
        gap="6"
        marginHorizontal="auto"
        maxWidth={768}
        overflow="auto"
        padding={24}
        width="100%"
      >
        <RequestDetailBody
          decisionActions={
            <>
              {/* Deciding is admin-only (§11); a member only sees the wait. Approving creates the
                  channel, so it ends on that channel, as it does from the approval queue. */}
              {proposal?.status === "submitted" && isAdmin && (
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
              {decisionError != null && <ErrorState error={decisionError} />}
            </>
          }
          entry={entry}
          headingLevel="h2"
          // The same live surface the Guide shows while generating: stage line, titles as they are
          // chosen, count and time.
          progress={<GenerationProgress phase="reasoning" progress={journey.progress} />}
          // Re-enqueue puts a given-up title back in the queue (§4). Admin-only, as is
          // POST /v1/titles — a member would only be offered a button that 403s.
          titleAction={(title) =>
            isAdmin && title.state === "unavailable" ? (
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
            ) : null
          }
          titleBadge={(title) => <StateBadge state={title.state} />}
          titles={titles}
          titlesError={retryTitle.error != null && <ErrorState error={retryTitle.error} />}
          whenText={formatRelative}
        />
      </Stack>
    </Stack>
  );
};

export { RequestDetailPage };
