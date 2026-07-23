import { type JobView, jobsApi } from "@loomarr/api";
import { formatRelative, formatUntil } from "@loomarr/core";
import { Loader2, Pencil, Play } from "lucide-react";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import { describeCron } from "./cron-presets";
import { JobEditModal } from "./job-edit-modal/job-edit-modal";

// Settings → Tasks (§18.1): the scheduler's named background jobs, like Sonarr's System →
// Tasks. Each row shows the job, its schedule (human label, cron in the tooltip), the BE's
// last/next-run, a status dot, and a Run-now button; a Modify button opens the cron editor.
// All timing is server-authored — the page renders `lastRun`/`nextRun`/`running` verbatim and
// refetches on the `job` SSE frame (wired in core), never computing countdowns client-side.

// statusDot colors a job by its last result: running (amber) > error (red) > ok (green) >
// never-run (muted). A tiny, at-a-glance health signal.
const statusDot = (job: JobView): { cls: string; label: string } => {
  if (job.running) return { cls: "bg-signal", label: "Running" };
  if (job.lastResult === "error") return { cls: "bg-onair", label: "Failed last run" };
  if (job.lastResult === "ok") return { cls: "bg-lock", label: "OK" };
  return { cls: "bg-static-500", label: "Not run yet" };
};

const zero = "0001-01-01T00:00:00Z"; // the Go zero time the BE omits/sends for "never"
const isZero = (t?: string) => !t || t.startsWith("0001-01-01");

const TasksPage = () => {
  const jobs = jobsApi.useJobsList({ query: { retry: false } });
  const run = jobsApi.useJobsRun();
  const [editing, setEditing] = useState<JobView | undefined>();

  if (jobs.error) {
    return (
      <div className="p-6">
        <ErrorState error={jobs.error} onRetry={() => jobs.refetch()} />
      </div>
    );
  }
  const list = jobs.data?.status === 200 ? (jobs.data.data.jobs ?? []) : [];

  return (
    <div className="flex flex-col gap-4 p-6">
      <div>
        <h1 className="font-semibold text-2xl">Tasks</h1>
        <p className="text-muted-foreground text-sm">
          Loomarr's scheduled background jobs. Change how often each runs, or run one now.
        </p>
      </div>

      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-border border-b text-left text-muted-foreground text-xs">
              <th className="px-4 py-2.5 font-medium">Task</th>
              <th className="px-4 py-2.5 font-medium">Frequency</th>
              <th className="px-4 py-2.5 font-medium">Last run</th>
              <th className="px-4 py-2.5 font-medium">Next run</th>
              <th className="px-4 py-2.5 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {list.map((job) => {
              const dot = statusDot(job);
              const running = run.isPending && run.variables?.name === job.name;
              return (
                <tr key={job.name} className="border-border border-b last:border-0">
                  <td className="px-4 py-3">
                    <span className="flex items-center gap-2">
                      <span
                        role="img"
                        className={cn("size-2 shrink-0 rounded-full", dot.cls)}
                        title={dot.label}
                        aria-label={dot.label}
                      />
                      <span className="font-medium">{job.title}</span>
                    </span>
                    {job.lastResult === "error" && job.lastError && (
                      <p className="mt-0.5 text-onair-300 text-xs">{job.lastError}</p>
                    )}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground" title={job.schedule}>
                    {describeCron(job.schedule)}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {isZero(job.lastRun) ? "—" : formatRelative(job.lastRun ?? zero)}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {job.running ? "running…" : isZero(job.nextRun) ? "—" : formatUntil(job.nextRun ?? zero)}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-muted-foreground"
                        onClick={() => setEditing(job)}
                      >
                        <Pencil className="size-4" aria-hidden />
                        Modify
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={running || job.running}
                        onClick={() => run.mutate({ name: job.name })}
                      >
                        {running ? (
                          <Loader2 className="size-4 animate-spin" aria-hidden />
                        ) : (
                          <Play className="size-4" aria-hidden />
                        )}
                        Run now
                      </Button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {list.length === 0 && !jobs.isLoading && (
              <tr>
                <td colSpan={5} className="px-4 py-6 text-center text-muted-foreground">
                  No scheduled tasks.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {editing && (
        <JobEditModal
          job={editing}
          open={editing !== undefined}
          onOpenChange={(open) => !open && setEditing(undefined)}
        />
      )}
    </div>
  );
};

export { TasksPage };
