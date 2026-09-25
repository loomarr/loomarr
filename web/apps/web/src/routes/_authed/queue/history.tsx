import { createFileRoute } from "@tanstack/react-router";
import { ApprovalHistory } from "@/queue/approval-history";

// History — the decided requests, readable by every member: read visibility is global (design
// §342), re-confirmed by the maintainer for #1429. It is a record, not a decision surface, so it
// carries no admin-only action.
const HistoryScreen = () => <ApprovalHistory />;

const Route = createFileRoute("/_authed/queue/history")({
  component: HistoryScreen,
});

export { Route };
