import { createFileRoute } from "@tanstack/react-router";
import { ApprovalQueue } from "@/suggest";

// Needs approval — admin-only (§11); the layout (`route.tsx`) only offers this tab's link to
// an admin. `ApprovalQueue` itself fetches with `retry: false` so a 403 (a member hitting this
// path directly) resolves as a definite error rather than retrying.
const ApprovalScreen = () => <ApprovalQueue />;

const Route = createFileRoute("/_authed/queue/approval")({
  component: ApprovalScreen,
});

export { Route };
