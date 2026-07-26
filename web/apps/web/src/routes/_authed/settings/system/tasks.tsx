import { createFileRoute } from "@tanstack/react-router";
import { TasksPage } from "@/settings";

// Settings → Tasks (§18.1): the scheduler's named background jobs, like Sonarr's
// System → Tasks. Lists each job with its interval, last/next run, status, and a Run-now
// button. All timing is server-authored; the page refetches on the `job` SSE frame.
const Tasks = () => <TasksPage />;

const Route = createFileRoute("/_authed/settings/system/tasks")({
  component: Tasks,
});

export { Route };
