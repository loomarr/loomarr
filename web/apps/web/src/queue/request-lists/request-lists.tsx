import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { requestFailureHint, requestFixLabel } from "@loomarr/core/requests";
import { Text } from "@loomarr/design-system";
import { CardAction, ListGroup } from "@loomarr/ui";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { RequestRow } from "@/queue/request-row";
import { type RequestEntry, useRequests } from "@/queue/use-requests";
import { ApprovalQueue } from "@/suggest/approval-queue";

// The three Requests tabs, one list each. They read `useRequests()` independently — React Query
// shares the cache with the layout, so this is one fetch, not three.

// Every empty tab ends in the same single next action: ask for a channel. The page-wide "you
// haven't requested a channel yet" state lives in the layout; this is one tab having nothing.
const TabEmpty = ({ title, description }: { title: string; description: string }) => {
  const navigate = useNavigate();
  return (
    <EmptyState
      title={title}
      description={description}
      action={{
        label: "Request a channel",
        onClick: () => void navigate({ to: "/requests/new" }),
      }}
    />
  );
};

// The date reads relative, like the rest of the app; the full date shows on hover and the `<time>`
// carries it machine-readable.
const When = ({ at, verb }: { at: string; verb: string }) => (
  <Text textRole="cardMeta" tone="secondary">
    <time dateTime={at} title={new Date(at).toLocaleString()}>
      {verb} {formatRelative(at)}
    </time>
  </Text>
);

// The title as a link to the request's detail, for a row whose one action goes somewhere else.
const DetailLink = ({ jobId, title }: { jobId: string; title: string }) => (
  <Link
    to="/requests/$jobId"
    params={{ jobId }}
    className="focus-visible:ring-ring rounded-sm hover:underline focus-visible:ring-2 focus-visible:outline-none"
  >
    {title}
  </Link>
);

// Requests › In progress and Done: each request is a row you scan (#1817 Q11, #2765), the same row
// card as Home's Your requests. The metadata line reads status · date · the sentence behind the status.
//
// Every row reaches the request's detail. A finished request that became a channel has a second
// destination, the channel itself, so its title is the detail link and the one action goes to the
// channel. ⚠ The actions name their row ("Open Westerns", "Open channel for Westerns"): a list of
// identical "Open" links is a list a screen reader cannot tell apart.
const rowFor = ({ journey, status }: RequestEntry) => {
  const title = journey.intent.description;
  const channel = status.tab === "done" ? journey.channel : undefined;
  return (
    <RequestRow
      key={journey.jobId}
      status={status}
      title={channel ? <DetailLink jobId={journey.jobId} title={title} /> : title}
      meta={[
        <When key="when" at={journey.createdAt} verb="Requested" />,
        ...(status.detail ? [status.detail] : []),
      ]}
      action={
        channel ? (
          <CardAction
            accessibilityLabel={`Open channel for ${title}`}
            accessibilityRole="link"
            render={<Link to="/channels/$id" params={{ id: channel.id }} />}
          >
            Open channel
          </CardAction>
        ) : (
          <CardAction
            accessibilityLabel={`Open ${title}`}
            accessibilityRole="link"
            render={<Link to="/requests/$jobId" params={{ jobId: journey.jobId }} />}
          >
            Open
          </CardAction>
        )
      }
    />
  );
};

// A failed request in Needs you stays a decision row: it carries its reason and the one way to
// fix it, and the viewer has to act on it (#1817 Q11). Its title is the link to the detail.
const failedRowFor = (
  { journey, status }: RequestEntry,
  hint: string | undefined,
  fix: string | undefined,
) => (
  <RequestRow
    key={journey.jobId}
    status={status}
    title={<DetailLink jobId={journey.jobId} title={journey.intent.description} />}
    meta={[<When key="when" at={journey.createdAt} verb="Requested" />, ...(hint ? [hint] : [])]}
    action={
      fix ? (
        <CardAction
          accessibilityRole="link"
          attention
          render={<Link to="/requests/new" search={{ job: journey.jobId }} />}
        >
          {fix}
        </CardAction>
      ) : undefined
    }
  />
);

// Decided filler pulls, for the Done tab (#1430): the server KEEPS a pull once an admin approves
// or dismisses it — exactly so History can answer what was agreed to — but no screen ever asked
// for anything but the pending ones. There is no pull detail route (a pull id is not a job id), so
// its row has no link.
const pullDecisionLine = (status: PullDTO["status"]): { line: string; tone: "lock" | "onair" } =>
  status === "approved" ? { line: "Downloaded", tone: "lock" } : { line: "Declined", tone: "onair" };

const pullRowFor = (pull: PullDTO) => {
  const { line, tone } = pullDecisionLine(pull.status);
  return (
    <RequestRow
      key={pull.id}
      status={{ line, tone }}
      title={pull.title}
      meta={[
        ...(pull.decidedAt ? [<When key="when" at={pull.decidedAt} verb="Decided" />] : []),
        // Pulls of one kind share a title; what each was for is what tells the rows apart (#2738).
        ...(pull.reason ? [pull.reason] : []),
        ...(pull.note ? [pull.note] : []),
      ]}
    />
  );
};

const RequestList = ({ tab }: { tab: "in-progress" | "done" }) => {
  const { isAdmin } = useAuth();
  const { inTab, error, refetch } = useRequests();
  const entries = inTab(tab);

  // Filler pulls are an admin-only list server-side (§10 V35), so a member's Done tab asks for
  // nothing here — the same `enabled` gate `usePendingApprovals` uses for the same reason.
  const pullsQuery = fillerApi.useListFillerPulls(
    {},
    { query: { enabled: tab === "done" && isAdmin, retry: false } },
  );
  const decidedPulls =
    tab === "done"
      ? ((unwrap(pullsQuery.data, (b) => b.pulls) ?? []).filter((p) => p.status !== "pending") as PullDTO[])
      : [];

  if (error != null) return <ErrorState error={error} onRetry={refetch} />;
  if (pullsQuery.error != null)
    return <ErrorState error={pullsQuery.error} onRetry={() => pullsQuery.refetch()} />;
  if (entries.length === 0 && decidedPulls.length === 0) {
    return tab === "in-progress" ? (
      <TabEmpty
        title="Nothing in progress"
        description="Requests that are still being generated, approved or downloaded appear here."
      />
    ) : (
      <TabEmpty
        title="Nothing finished yet"
        description="Requests that are on their channel, or were declined, appear here."
      />
    );
  }
  // A finished request links to the channel it became; its titles are part of that channel,
  // which is why there is no separate "ready" list.
  return (
    <div className="max-w-reading mx-auto w-full">
      <ListGroup aria-label={tab === "done" ? "Finished requests" : "Requests in progress"}>
        {entries.map(rowFor)}
        {decidedPulls.map(pullRowFor)}
      </ListGroup>
    </div>
  );
};

// Needs you: what is waiting on THIS viewer. An admin's approvals come first (they block other
// people); everyone's own failed requests follow, each with its reason and one way to fix it.
// Members never see approvals — deciding is admin-only (§11) — but they read everything else
// (§342).
const NeedsYouList = () => {
  const { isAdmin } = useAuth();
  const { inTab, pendingApprovals, error, refetch } = useRequests();
  // ⚠ Kept true while a bulk review is open: approving the last pending items drops the count to
  // zero, and unmounting the queue then would discard the result the operator is about to read.
  const [reviewing, setReviewing] = useState(false);
  const failed = inTab("needs-you");
  if (error != null) return <ErrorState error={error} onRetry={refetch} />;
  const showApprovals = isAdmin && (pendingApprovals > 0 || reviewing);
  if (!showApprovals && failed.length === 0) {
    return (
      <TabEmpty
        title="Nothing needs you"
        description="Approvals waiting on you, and requests that couldn't be built, appear here."
      />
    );
  }
  return (
    <div className="max-w-reading mx-auto flex w-full flex-col gap-6">
      {showApprovals && (
        <section aria-labelledby="needs-approvals" className="flex flex-col gap-3">
          <h2 id="needs-approvals" className="text-lg font-semibold">
            Waiting for your approval
          </h2>
          <ApprovalQueue onReviewingChange={setReviewing} />
        </section>
      )}
      {failed.length > 0 && (
        <section aria-labelledby="needs-failed" className="flex flex-col gap-3">
          <h2 id="needs-failed" className="text-lg font-semibold">
            Couldn't be built
          </h2>
          <ListGroup aria-labelledby="needs-failed">
            {failed.map((e) =>
              failedRowFor(
                e,
                requestFailureHint(e.status.detail, e.journey.failure?.guidance),
                requestFixLabel(e.journey) || undefined,
              ),
            )}
          </ListGroup>
        </section>
      )}
    </div>
  );
};

export { NeedsYouList, RequestList };
