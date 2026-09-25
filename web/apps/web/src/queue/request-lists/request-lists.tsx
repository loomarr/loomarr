import { Link, useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { RequestCard } from "@/components/loomarr/ai/request-card";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { buttonVariants } from "@/components/ui/button";
import { requestFixLabel } from "@/queue/request-status";
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

// The server's message sometimes already ends with its own guidance ("…this channel. Try again
// later." + "Try again later."), so the guidance is appended only when the message lacks it.
const failureHint = (message?: string, guidance?: string): string => {
  const said = message?.trim().toLowerCase() ?? "";
  const extra = guidance && !said.includes(guidance.trim().toLowerCase()) ? guidance : undefined;
  return [message, extra].filter(Boolean).join(" ");
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

const RequestList = ({ tab }: { tab: "in-progress" | "done" }) => {
  const { inTab, error, refetch } = useRequests();
  const entries = inTab(tab);
  if (error != null) return <ErrorState error={error} onRetry={refetch} />;
  if (entries.length === 0) {
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
                hint: failureHint(e.status.detail, e.journey.failure?.guidance),
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
