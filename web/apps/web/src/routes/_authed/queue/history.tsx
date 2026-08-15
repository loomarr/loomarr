import { createFileRoute } from "@tanstack/react-router";
import { ApprovalHistory } from "@/queue/approval-history";

// History — a record of other people's decisions is not a member's to read (§11), so the
// layout only offers this tab's link to an admin. As before the split, `ApprovalHistory`'s own
// query carries no role gate of its own — that has always been the layout's job (the tab bar),
// unchanged by moving from `?tab=history` to its own path.
const HistoryScreen = () => <ApprovalHistory />;

const Route = createFileRoute("/_authed/queue/history")({
  component: HistoryScreen,
});

export { Route };
