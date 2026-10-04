import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { requestFailureHint, requestFixLabel } from "@loomarr/core/requests";
import { Link, useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { RequestCard } from "@/components/loomarr/ai/request-card";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
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

// Decided filler pulls, for the Done tab (#1430): the server KEEPS a pull once an admin approves
// or dismisses it — exactly so History can answer what was agreed to — but no screen ever asked
// for anything but the pending ones. There is no pull detail route, so this renders inline rather
// than through RequestCard's link-to-detail: that card always links to `/requests/$jobId`, and a
// pull id is not a job id.
const pullDecisionLine = (status: PullDTO["status"]): { line: string; tone: "lock" | "onair" } =>
  status === "approved" ? { line: "Downloaded", tone: "lock" } : { line: "Declined", tone: "onair" };

const pullCardFor = (pull: PullDTO) => {
  const { line, tone } = pullDecisionLine(pull.status);
  return (
    <li key={pull.id}>
      <Card className="flex flex-col gap-1.5 p-4">
        <p className="font-medium">{pull.title}</p>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={tone}>{line}</Badge>
          {pull.decidedAt && (
            <time
              dateTime={pull.decidedAt}
              title={new Date(pull.decidedAt).toLocaleString()}
              className="text-muted-foreground text-xs"
            >
              Decided {formatRelative(pull.decidedAt)}
            </time>
          )}
        </div>
        {pull.note && <p className="text-muted-foreground text-sm">{pull.note}</p>}
      </Card>
    </li>
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
  return (
    <ul className="flex max-w-3xl flex-col gap-3">
      {entries.map((e) =>
        // A finished request links to the channel it became; its titles are part of that channel,
        // which is why there is no separate "ready" list.
        cardFor(e, {
          action: tab === "done" && e.journey.channel && (
            <Link
              to="/channels/$id"
              params={{ id: e.journey.channel.id }}
              className={buttonVariants({ variant: "outline", size: "sm" })}
            >
              Open channel
            </Link>
          ),
        }),
      )}
      {decidedPulls.map(pullCardFor)}
    </ul>
  );
};

// Needs you: what is waiting on THIS viewer. An admin's approvals come first (they block other
// people); everyone's own failed requests follow, each with its reason and one way to fix it.
// Members never see approvals — deciding is admin-only (§11) — but they read everything else
// (§342).
const NeedsYouList = () => {
  const { isAdmin } = useAuth();
  const { inTab, pendingApprovals, error, refetch } = useRequests();
  const failed = inTab("needs-you");
  if (error != null) return <ErrorState error={error} onRetry={refetch} />;
  const showApprovals = isAdmin && pendingApprovals > 0;
  if (!showApprovals && failed.length === 0) {
    return (
      <TabEmpty
        title="Nothing needs you"
        description="Approvals waiting on you, and requests that couldn't be built, appear here."
      />
    );
  }
  return (
    <div className="flex max-w-3xl flex-col gap-6">
      {showApprovals && (
        <section aria-labelledby="needs-approvals" className="flex flex-col gap-3">
          <h2 id="needs-approvals" className="font-semibold text-lg">
            Waiting for your approval
          </h2>
          <ApprovalQueue />
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

export { NeedsYouList, RequestList };
