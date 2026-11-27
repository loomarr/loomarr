import * as jobsApi from "@loomarr/api/endpoints/jobs";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { TasksPage } from "@/settings/tasks-page";

// Settings → Tasks (§18.1): the scheduler's named background jobs, like Sonarr's
// System → Tasks. Lists each job with its interval, last/next run, status, and a Run-now
// button. All timing is server-authored; the page refetches on the `job` SSE frame.
const Tasks = () => <TasksPage />;

const Route = createFileRoute("/_authed/settings/system/tasks")({
  component: Tasks,
  // The job list the page is made of, so it opens on its rows and a failed read is the route's
  // error view. Admin-only like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await queryClient.ensureQueryData(jobsApi.getJobsListQueryOptions({ query: { retry: false } }));
  },
});

export { Route };
