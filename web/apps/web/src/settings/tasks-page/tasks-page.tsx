import { type JobHistoryView, type JobView, jobsApi, toProblem, unwrap } from "@loomarr/api";
import { formatClipDuration, formatDuration, formatRelative, formatUntil } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Pause, Pencil, Play } from "lucide-react";
import { Fragment, useState } from "react";
import { toast } from "sonner";
import { ErrorDetails, ErrorState, RunButton, useRunFeedback } from "@/components/loomarr";
import { Button, StatusDot, type StatusTone } from "@/components/ui";
import { cn } from "@/lib";
import { describeCron } from "./cron-presets";
import { JobEditModal } from "./job-edit-modal/job-edit-modal";

const GROUPS: { id: JobView["group"]; label: string }[] = [
  { id: "acquisitions", label: "Acquisitions" },
  { id: "channels", label: "Channels" },
  { id: "filler", label: "Filler" },
  { id: "artwork", label: "Artwork" },
  { id: "playout", label: "Playout" },
  { id: "system", label: "System" },
  { id: "backup", label: "Backup" },
];

const statusDot = (job: JobView): { tone: StatusTone; label: string } => {
  if (job.disabledReason) return { tone: "off", label: "Not available on this backend" };
  if (job.paused) return { tone: "off", label: "Paused" };
  if (job.running) return { tone: "pending", label: "Running" };
  if (job.lastResult === "error") return { tone: "error", label: "Failed last run" };
  if (job.lastResult === "ok") return { tone: "ok", label: "OK" };
  return { tone: "off", label: "Not run yet" };
};

const groupSummary = (jobs: JobView[]) => {
  const running = jobs.filter((job) => job.running).length;
  const failed = jobs.filter((job) => job.lastResult === "error").length;
  const paused = jobs.filter((job) => job.paused).length;
  const unavailable = jobs.filter((job) => job.disabledReason).length;
  const details = [
    running && `${running} running`,
    failed && `${failed} failed`,
    paused && `${paused} paused`,
    unavailable && `${unavailable} unavailable`,
  ].filter(Boolean);
  return `${jobs.length} ${jobs.length === 1 ? "task" : "tasks"}${details.length ? ` · ${details.join(" · ")}` : ""}`;
};

const COLUMNS = [
  { key: "task", label: "Task", width: "auto" },
  { key: "frequency", label: "Frequency", width: "10rem" },
  { key: "lastRun", label: "Last run", width: "7rem" },
  { key: "nextRun", label: "Next run", width: "7rem" },
  { key: "details", label: "", width: "3rem" },
] as const;

const zero = "0001-01-01T00:00:00Z";
const isZero = (t?: string) => !t || t.startsWith("0001-01-01");

const executionDuration = (ms: number) => (ms < 3_600_000 ? formatClipDuration(ms) : formatDuration(ms));

const ExecutionHistory = ({ name }: { name: string }) => {
  const query = jobsApi.useJobsHistory(name, { query: { retry: false } });
  const history = unwrap(query.data) as JobHistoryView | undefined;

  if (query.isLoading) {
    return <p className="mt-4 text-muted-foreground text-xs">Loading execution history…</p>;
  }
  if (query.error) {
    return (
      <div className="mt-4 flex items-center gap-2 text-destructive text-xs" role="alert">
        <span>Couldn’t load execution history.</span>
        <Button variant="ghost" size="sm" onClick={() => query.refetch()}>
          Retry
        </Button>
      </div>
    );
  }
  if (!history) return null;

  return (
    <section className="mt-4 border-border border-t pt-3" aria-label="Execution history">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h3 className="font-medium text-xs">Past 24 hours</h3>
        {history.runCount === 0 ? (
          <p className="text-muted-foreground text-xs">No completed runs.</p>
        ) : (
          <dl className="flex flex-wrap gap-x-4 text-xs">
            <div className="flex gap-1">
              <dt className="text-muted-foreground">Runs</dt>
              <dd className="font-mono">{history.truncated ? `${history.runCount}+` : history.runCount}</dd>
            </div>
            <div className="flex gap-1">
              <dt className="text-muted-foreground">Failed</dt>
              <dd className={cn("font-mono", history.failureCount > 0 && "text-destructive")}>
                {history.failureCount}
              </dd>
            </div>
            <div className="flex gap-1">
              <dt className="text-muted-foreground">Average</dt>
              <dd className="font-mono">{executionDuration(history.averageDurationMs)}</dd>
            </div>
          </dl>
        )}
      </div>

      {history.recent.length > 0 && (
        <ul className="mt-2 divide-y divide-border rounded-md border border-border bg-background">
          {history.recent.map((run) => {
            const failed = run.result === "error";
            return (
              <li key={`${run.startedAt}-${run.trigger}`} className="px-3 py-2">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                  <StatusDot tone={failed ? "error" : "ok"} label={failed ? "Failed" : "Succeeded"} />
                  <span className="font-medium">{run.trigger === "manual" ? "Manual" : "Scheduled"}</span>
                  <span className="text-muted-foreground">{formatRelative(run.startedAt)}</span>
                  <span className="font-mono text-muted-foreground">{executionDuration(run.durationMs)}</span>
                </div>
                {failed && run.error && <ErrorDetails message={run.error} className="mt-1" />}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
};

const TasksPage = () => {
  const queryClient = useQueryClient();
  const jobs = jobsApi.useJobsList({ query: { retry: false } });
  const refresh = () => queryClient.invalidateQueries({ queryKey: jobsApi.getJobsListQueryKey() });
  const feedback = useRunFeedback();
  const [editing, setEditing] = useState<JobView | undefined>();
  const [openGroups, setOpenGroups] = useState<Set<JobView["group"]>>(() => new Set());
  const [openJobs, setOpenJobs] = useState<Set<string>>(() => new Set());
  const list = unwrap(jobs.data, (body) => body.jobs) ?? [];

  const run = jobsApi.useJobsRun({
    mutation: {
      onMutate: ({ name }) => feedback.start(name),
      onSuccess: async (_data, { name }) => {
        const job = list.find((candidate) => candidate.name === name);
        toast.success(`Ran ${job?.title ?? name}`);
        await feedback.settle(name, refresh());
      },
      onError: (error, { name }) => {
        feedback.finish(name);
        toast.error(toProblem(error).title ?? "Couldn't run the task");
      },
    },
  });
  const pause = jobsApi.useJobsPause({
    mutation: {
      onSuccess: () => refresh(),
      onError: (error) => toast.error(toProblem(error).title ?? "Couldn't change the task"),
    },
  });

  const toggleGroup = (group: JobView["group"]) => {
    setOpenGroups((current) => {
      const next = new Set(current);
      if (next.has(group)) next.delete(group);
      else next.add(group);
      return next;
    });
  };
  const toggleJob = (name: string) => {
    setOpenJobs((current) => {
      const next = new Set(current);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  if (jobs.error) {
    return (
      <div className="p-6">
        <ErrorState error={jobs.error} onRetry={() => jobs.refetch()} />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-6">
      <div>
        <h1 className="font-semibold text-2xl">Tasks</h1>
        <p className="text-muted-foreground text-sm">
          Background work grouped by outcome. Expand a group to inspect or control its tasks.
        </p>
      </div>

      <div className="min-h-0 flex-1 space-y-2 overflow-auto">
        {GROUPS.map((group) => {
          const groupedJobs = list.filter((job) => job.group === group.id);
          if (groupedJobs.length === 0) return null;
          const groupOpen = openGroups.has(group.id);
          return (
            <section key={group.id} className="overflow-hidden rounded-lg border border-border bg-card">
              <button
                type="button"
                className="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-muted/40"
                aria-expanded={groupOpen}
                aria-controls={`task-group-${group.id}`}
                onClick={() => toggleGroup(group.id)}
              >
                {groupOpen ? (
                  <ChevronDown className="size-4 text-muted-foreground" aria-hidden />
                ) : (
                  <ChevronRight className="size-4 text-muted-foreground" aria-hidden />
                )}
                <span className="font-medium">{group.label}</span>
                <span className="ml-auto text-muted-foreground text-xs">{groupSummary(groupedJobs)}</span>
              </button>

              {groupOpen && (
                <div id={`task-group-${group.id}`} className="border-border border-t">
                  <table className="w-full text-sm">
                    <colgroup>
                      {COLUMNS.map((column) => (
                        <col key={column.key} style={{ width: column.width }} />
                      ))}
                    </colgroup>
                    <thead>
                      <tr className="border-border border-b text-left text-muted-foreground text-xs">
                        {COLUMNS.map((column) => (
                          <th key={column.key} className="px-4 py-2.5 font-medium">
                            {column.label}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {groupedJobs.map((job) => {
                        const dot = statusDot(job);
                        const busy = feedback.isBusy(job.name) || job.running;
                        const pausing = pause.isPending && pause.variables?.name === job.name;
                        const jobOpen = openJobs.has(job.name);
                        return (
                          <Fragment key={job.name}>
                            <tr className={cn("border-border border-b", job.paused && "opacity-60")}>
                              <td className="px-4 py-3">
                                <button
                                  type="button"
                                  className="flex items-center gap-2 text-left"
                                  aria-expanded={jobOpen}
                                  aria-controls={`task-${job.name}`}
                                  onClick={() => toggleJob(job.name)}
                                >
                                  <StatusDot
                                    tone={dot.tone}
                                    label={job.paused ? "" : dot.label}
                                    title={dot.label}
                                  />
                                  <span className="font-medium">{job.title}</span>
                                  {job.paused && (
                                    <span className="rounded bg-muted px-1.5 py-0.5 font-medium text-[10px] text-muted-foreground uppercase tracking-wide">
                                      Paused
                                    </span>
                                  )}
                                </button>
                              </td>
                              <td
                                className="px-4 py-3 text-muted-foreground"
                                title={job.disabledReason ? undefined : job.schedule}
                              >
                                {job.disabledReason ? "Not scheduled" : describeCron(job.schedule)}
                              </td>
                              <td className="px-4 py-3 text-muted-foreground">
                                {isZero(job.lastRun) ? "—" : formatRelative(job.lastRun ?? zero)}
                              </td>
                              <td className="px-4 py-3 text-muted-foreground">
                                {job.disabledReason
                                  ? "—"
                                  : job.paused
                                    ? "Paused"
                                    : job.running
                                      ? "running…"
                                      : isZero(job.nextRun)
                                        ? "—"
                                        : job.overdue
                                          ? "overdue"
                                          : formatUntil(job.nextRun ?? zero)}
                              </td>
                              <td className="px-4 py-3 text-right">
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  aria-label={`${jobOpen ? "Collapse" : "Expand"} ${job.title}`}
                                  onClick={() => toggleJob(job.name)}
                                >
                                  {jobOpen ? (
                                    <ChevronDown className="size-4" />
                                  ) : (
                                    <ChevronRight className="size-4" />
                                  )}
                                </Button>
                              </td>
                            </tr>
                            {jobOpen && (
                              <tr id={`task-${job.name}`} className="border-border border-b bg-muted/20">
                                <td colSpan={5} className="px-4 py-3">
                                  <p className="max-w-prose text-muted-foreground text-sm">
                                    {job.description}
                                  </p>
                                  {job.lastResult === "error" && (
                                    <ErrorDetails message={job.lastError} className="mt-2" />
                                  )}
                                  {job.disabledReason && (
                                    <p className="mt-2 text-muted-foreground text-sm">{job.disabledReason}</p>
                                  )}
                                  <ExecutionHistory name={job.name} />
                                  {!job.disabledReason && (
                                    <div className="mt-3 flex items-center gap-1">
                                      <Button
                                        variant="ghost"
                                        size="sm"
                                        className="text-muted-foreground"
                                        disabled={pausing}
                                        aria-label={job.paused ? `Resume ${job.title}` : `Pause ${job.title}`}
                                        onClick={() =>
                                          pause.mutate({ name: job.name, data: { paused: !job.paused } })
                                        }
                                      >
                                        {job.paused ? (
                                          <Play className="size-4" aria-hidden />
                                        ) : (
                                          <Pause className="size-4" aria-hidden />
                                        )}
                                        {job.paused ? "Resume" : "Pause"}
                                      </Button>
                                      <Button
                                        variant="ghost"
                                        size="sm"
                                        className="text-muted-foreground"
                                        onClick={() => setEditing(job)}
                                      >
                                        <Pencil className="size-4" aria-hidden />
                                        Modify
                                      </Button>
                                      <RunButton busy={busy} onRun={() => run.mutate({ name: job.name })} />
                                    </div>
                                  )}
                                </td>
                              </tr>
                            )}
                          </Fragment>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          );
        })}

        {list.length === 0 && !jobs.isLoading && (
          <div className="rounded-lg border border-border bg-card px-4 py-6 text-center text-muted-foreground">
            No scheduled tasks.
          </div>
        )}
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
