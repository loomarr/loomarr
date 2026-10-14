import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { requestFailureHint, requestFixLabel } from "@loomarr/core/requests";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { RequestCard } from "@/components/loomarr/ai/request-card";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { buttonVariants } from "@/components/ui/button";
import { ListGroup, ListRow } from "@/components/ui/list-row";
import type { StatusTone } from "@/components/ui/status-dot";
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
        onClick: () => void navigate({ to: "/guide", search: { new: "1" } }),
      }}
    />
  );
};

// A failed request in Needs you stays a decision card: it carries its reason and the one way to
// fix it, and the viewer has to act on it (#1817 Q11).
const cardFor = (
  { journey, status }: RequestEntry,
  extras?: { action?: React.ReactNode; hint?: string | undefined },
) => (
  <li key={journey.jobId}>
    <RequestCard
      jobId={journey.jobId}
      title={journey.intent.description}
      line={status.line}
      tone={status.tone}
      createdAt={journey.createdAt}
      // The badge is a short label; the sentence behind it is the explanation line.
      hint={extras?.hint ?? status.detail}
      action={extras?.action}
    />
  </li>
);

// A request's badge tone, as a row's status dot. Home's Your requests uses the same map, so the
// same status draws the same dot wherever a request is a row (#1817 Q11).
const REQUEST_ROW_TONE: Record<RequestEntry["status"]["tone"], StatusTone> = {
  suggest: "attention",
  lock: "ok",
  onair: "error",
  caution: "progress",
};

// The row's trailing link, quiet like Home's "View channel →".
const ROW_LINK =
  "shrink-0 rounded-sm text-sm text-static-400 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

// The date reads relative, like the rest of the app; the full date shows on hover and the `<time>`
// carries it machine-readable.
const When = ({ at, verb }: { at: string; verb: string }) => (
  <time dateTime={at} title={new Date(at).toLocaleString()}>
    {verb} {formatRelative(at)}
  </time>
);

// Requests › In progress and Done: each request is a row you scan (#1817 Q11), the same shape as
// Home's Your requests. The sub line reads status · date · the sentence behind the status.
//
// Every row reaches the request's detail. A finished request that became a channel has a second
// destination, the channel itself, so its title is the detail link and the one trailing slot goes
// to the channel. ⚠ The trailing links name their row ("Open Westerns", "Open channel for Westerns"):
// a list of identical "Open" links is a list a screen reader cannot tell apart.
const rowFor = ({ journey, status }: RequestEntry) => {
  const title = journey.intent.description;
  const channel = status.tab === "done" ? journey.channel : undefined;
  return (
    <ListRow
      key={journey.jobId}
      tone={REQUEST_ROW_TONE[status.tone]}
      title={
        channel ? (
          <Link
            to="/requests/$jobId"
            params={{ jobId: journey.jobId }}
            className="rounded-sm hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {title}
          </Link>
        ) : (
          title
        )
      }
      sub={
        <>
          <span>{status.line}</span> · <When at={journey.createdAt} verb="Requested" />
          {status.detail && ` · ${status.detail}`}
        </>
      }
      action={
        channel ? (
          <Link
            to="/channels/$id"
            params={{ id: channel.id }}
            aria-label={`Open channel for ${title}`}
            className={ROW_LINK}
          >
            Open channel <span aria-hidden="true">→</span>
          </Link>
        ) : (
          <Link
            to="/requests/$jobId"
            params={{ jobId: journey.jobId }}
            aria-label={`Open ${title}`}
            className={ROW_LINK}
          >
            Open <span aria-hidden="true">→</span>
          </Link>
        )
      }
    />
  );
};

// Decided filler pulls, for the Done tab (#1430): the server KEEPS a pull once an admin approves
// or dismisses it — exactly so History can answer what was agreed to — but no screen ever asked
// for anything but the pending ones. There is no pull detail route (a pull id is not a job id), so
// its row has no link.
const pullDecisionLine = (status: PullDTO["status"]): { line: string; tone: "lock" | "onair" } =>
  status === "approved" ? { line: "Downloaded", tone: "lock" } : { line: "Declined", tone: "onair" };

const pullRowFor = (pull: PullDTO) => {
  const { line, tone } = pullDecisionLine(pull.status);
  return (
    <ListRow
      key={pull.id}
      tone={REQUEST_ROW_TONE[tone]}
      title={pull.title}
      sub={
        <>
          <span>{line}</span>
          {pull.decidedAt && (
            <>
              {" · "}
              <When at={pull.decidedAt} verb="Decided" />
            </>
          )}
          {pull.note && ` · ${pull.note}`}
        </>
      }
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
    <ListGroup
      aria-label={tab === "done" ? "Finished requests" : "Requests in progress"}
      className="mx-auto w-full max-w-reading"
    >
      {entries.map(rowFor)}
      {decidedPulls.map(pullRowFor)}
    </ListGroup>
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
    <div className="mx-auto flex w-full max-w-reading flex-col gap-6">
      {showApprovals && (
        <section aria-labelledby="needs-approvals" className="flex flex-col gap-3">
          <h2 id="needs-approvals" className="font-semibold text-lg">
            Waiting for your approval
          </h2>
          <ApprovalQueue onReviewingChange={setReviewing} />
        </section>
      )}
      {failed.length > 0 && (
        <section aria-labelledby="needs-failed" className="flex flex-col gap-3">
          <h2 id="needs-failed" className="font-semibold text-lg">
            Couldn't be built
          </h2>
          <ul className="flex flex-col gap-3">
            {failed.map((e) => {
              const fix = requestFixLabel(e.journey);
              return cardFor(e, {
                hint: requestFailureHint(e.status.detail, e.journey.failure?.guidance),
                action: fix && (
                  <Link
                    to="/guide"
                    search={{ job: e.journey.jobId }}
                    className={buttonVariants({ variant: "outline", size: "sm" })}
                  >
                    {fix}
                  </Link>
                ),
              });
            })}
          </ul>
        </section>
      )}
    </div>
  );
};

export { NeedsYouList, REQUEST_ROW_TONE, RequestList };
