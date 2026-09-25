import { createFileRoute, Link, Outlet, useLocation, useNavigate } from "@tanstack/react-router";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { NavTabs } from "@/components/ui/nav-tabs";
import { useDocumentTitle } from "@/lib/use-document-title";
import type { RequestTab } from "@/queue/request-status";
import { useRequests } from "@/queue/use-requests";

// Requests — what you've asked for, and anything that needs you (#1405; this was "Queue"). Three
// tabs, each its own PATH so a tab can be linked and reopened:
//   Needs you    — approvals waiting on an admin, and your own requests that couldn't be built
//   In progress  — every unfinished request, with one status line
//   Done         — finished requests (a link to their channel) and declined ones
// The tab bar is the v2 mock's underline style (`NavTabs variant="underline"`).
//
// ⚠ Every member sees all three tabs. Approving is admin-only (§11), but that gates the approval
// ACTIONS inside Needs you, not the tab: a member's own failed requests live there too, and read
// visibility is global (§342).
const TAB_IDS: RequestTab[] = ["needs-you", "in-progress", "done"];

const RequestsLayout = () => {
  useDocumentTitle("Requests");
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { entries, inTab, needsYouCount, pendingApprovals, isLoading, error, refetch } = useRequests();

  const activeId = TAB_IDS.find((id) => pathname.endsWith(`/${id}`)) ?? "in-progress";
  const nothingRequested = !isLoading && error == null && entries.length === 0 && pendingApprovals === 0;

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Requests" description="Channels you've asked for and where each one stands." />
      {error != null ? (
        <div className="p-6">
          <ErrorState error={error} onRetry={refetch} />
        </div>
      ) : nothingRequested ? (
        <div className="p-6">
          <EmptyState
            title="You haven't requested a channel yet"
            description="Describe a channel and Loomarr will build it. You'll follow it here, from request to your guide."
            action={{
              label: "Request a channel",
              onClick: () => void navigate({ to: "/guide", search: { new: "1" } }),
            }}
          />
        </div>
      ) : (
        <>
          <div className="px-6">
            <NavTabs
              label="Requests sections"
              variant="underline"
              linkComponent={Link}
              activeId={activeId}
              tabs={[
                {
                  id: "needs-you",
                  label: "Needs you",
                  to: "/requests/needs-you",
                  count: needsYouCount,
                  attention: true,
                },
                {
                  id: "in-progress",
                  label: "In progress",
                  to: "/requests/in-progress",
                  count: inTab("in-progress").length,
                },
                { id: "done", label: "Done", to: "/requests/done", count: inTab("done").length },
              ]}
            />
          </div>
          <div
            id={`panel-${activeId}`}
            role="tabpanel"
            aria-labelledby={`tab-${activeId}`}
            className="flex flex-1 flex-col gap-6 overflow-auto p-6"
          >
            <Outlet />
          </div>
        </>
      )}
    </div>
  );
};

const Route = createFileRoute("/_authed/requests/_tabs")({
  component: RequestsLayout,
});

export { Route };
