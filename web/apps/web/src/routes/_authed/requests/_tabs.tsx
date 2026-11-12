import type { RequestTab } from "@loomarr/core/requests";
import { Stack } from "@loomarr/design-system";
import { NavTabs } from "@loomarr/ui";
import { createFileRoute, Link, Outlet, useLocation, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { viewerIsAdmin } from "@/auth/me-query";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { ChannelIdeas } from "@/home/channel-ideas";
import { useDocumentTitle } from "@/lib/use-document-title";
import { useRouteContent } from "@/lib/use-route-content";
import { ensureRequests, useRequests } from "@/queue/use-requests";

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
  // The layout answers for the page when there is nothing to list; each tab's list answers for
  // itself when the viewer switches tabs and this layout stays mounted.
  useRouteContent(!isLoading && error == null);

  const { isAdmin } = useAuth();
  const [nowMs] = useState(() => Date.now());
  // Channel ideas sit above the list (#2763); they render nothing while there is no idea. As on
  // Home, a member's section: an admin builds channels directly.
  const ideas = isAdmin ? null : <ChannelIdeas empty={false} inset nowMs={nowMs} />;

  const activeId = TAB_IDS.find((id) => pathname.endsWith(`/${id}`)) ?? "in-progress";
  const nothingRequested = !isLoading && error == null && entries.length === 0 && pendingApprovals === 0;

  return (
    <Stack className="h-full">
      <PageHeader title="Requests" description="Channels you've asked for and where each one stands." />
      {error != null ? (
        <div className="p-6">
          <ErrorState error={error} onRetry={refetch} />
        </div>
      ) : nothingRequested ? (
        <div className="p-6">
          {ideas}
          <EmptyState
            title="You haven't requested a channel yet"
            description="Describe a channel and Loomarr will build it. You'll follow it here, from request to your guide."
            action={{
              label: "Request a channel",
              onClick: () => void navigate({ to: "/requests/new" }),
            }}
          />
        </div>
      ) : (
        <>
          {ideas}
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
            // Rows as tall as their content (`auto-rows-max`), so the panel scrolls. A flex column,
            // or plain auto rows, shrank an `overflow-hidden` list instead (its minimum height is
            // 0), hiding its last rows with nothing to scroll (#1785, 320×640).
            className="grid flex-1 auto-rows-max gap-6 overflow-auto p-6"
          >
            <Outlet />
          </div>
        </>
      )}
    </Stack>
  );
};

const Route = createFileRoute("/_authed/requests/_tabs")({
  component: RequestsLayout,
  // The lists and counts the tab bar and every tab are made from. The landing redirect already
  // read most of this, so arriving from /requests costs nothing more.
  loader: async ({ context: { queryClient } }) => {
    await ensureRequests(queryClient, await viewerIsAdmin(queryClient));
  },
});

export { Route };
