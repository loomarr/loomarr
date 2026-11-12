import * as jobsApi from "@loomarr/api/endpoints/jobs";
import type { JobHistoryView } from "@loomarr/api/models/jobHistoryView";
import type { JobView } from "@loomarr/api/models/jobView";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, formatDuration, formatRelative, formatUntil } from "@loomarr/core/format";
import {
  Action,
  Icon,
  icons,
  Stack,
  StatusDot,
  type StatusTone,
  StatusWord,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Fragment, useState } from "react";
import { toast } from "sonner";
import { ErrorDetails } from "@/components/loomarr/feedback/error-details";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { RunButton } from "@/components/loomarr/feedback/run-button";
import { useRunFeedback } from "@/components/loomarr/feedback/use-run-feedback";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { cn } from "@/lib/utils";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { describeCron } from "./cron-presets";
import { JobEditModal } from "./job-edit-modal/job-edit-modal";
import { isOverdueTask, overdueTaskCount } from "./overdue-tasks";

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
  // An overdue task is never green: its last run went fine, but it is waiting on a worker now.
  if (isOverdueTask(job)) return { tone: "warn", label: "Overdue" };
  if (job.lastResult === "ok") return { tone: "ok", label: "OK" };
  return { tone: "off", label: "Not run yet" };
};

const groupSummary = (jobs: JobView[]) => {
  const running = jobs.filter((job) => job.running).length;
  const failed = jobs.filter((job) => job.lastResult === "error").length;
  const overdue = overdueTaskCount(jobs);
  const paused = jobs.filter((job) => job.paused).length;
  const unavailable = jobs.filter((job) => job.disabledReason).length;
  const details = [
    running && `${running} running`,
    failed && `${failed} failed`,
    overdue && `${overdue} overdue`,
    paused && `${paused} paused`,
    unavailable && `${unavailable} unavailable`,
  ].filter(Boolean);
  return `${jobs.length} ${jobs.length === 1 ? "task" : "tasks"}${details.length ? ` · ${details.join(" · ")}` : ""}`;
};

const COLUMNS = [
  { key: "task", label: "Task", width: undefined },
  { key: "frequency", label: "Frequency", width: 160 },
  { key: "lastRun", label: "Last run", width: 112 },
  { key: "nextRun", label: "Next run", width: 112 },
  { key: "details", label: "", width: 48 },
] as const;

const zero = "0001-01-01T00:00:00Z";
const isZero = (t?: string) => !t || t.startsWith("0001-01-01");
// A next run that has just passed is a task waiting for its claim, which is ordinary queueing;
// only `overdue` (late by more than twice its interval) is worth a warning.
const isPast = (t?: string) => !isZero(t) && Date.parse(t ?? zero) <= Date.now();

const executionDuration = (ms: number) => (ms < 3_600_000 ? formatClipDuration(ms) : formatDuration(ms));

const ExecutionHistory = ({ name }: { name: string }) => {
  const query = jobsApi.useJobsHistory(name, { query: { retry: false } });
  const history = unwrap(query.data) as JobHistoryView | undefined;

  if (query.isLoading) {
    return <p className="text-muted-foreground mt-4 text-xs">Loading execution history…</p>;
  }
  if (query.error) {
    return (
      <Stack
        align="center"
        className="text-destructive mt-4 text-xs"
        direction="row"
        gap="inline"
        role="alert"
      >
        <span>Couldn’t load execution history.</span>
        <Action onPress={() => query.refetch()} variant="ghost">
          Retry
        </Action>
      </Stack>
    );
  }
  if (!history) return null;

  return (
    <section className="border-border mt-4 border-t pt-3" aria-label="Execution history">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h3 className="text-xs font-medium">Past 24 hours</h3>
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
        <ul className="divide-border border-border bg-background mt-2 divide-y rounded-md border">
          {history.recent.map((run) => {
            const failed = run.result === "error";
            return (
              <li key={`${run.startedAt}-${run.trigger}`} className="px-3 py-2">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                  <StatusDot tone={failed ? "error" : "ok"} label={failed ? "Failed" : "Succeeded"} />
                  <span className="font-medium">{run.trigger === "manual" ? "Manual" : "Scheduled"}</span>
                  <span className="text-muted-foreground">{formatRelative(run.startedAt)}</span>
                  <span className="text-muted-foreground font-mono">{executionDuration(run.durationMs)}</span>
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
  // A job's schedule choices are the registry's presets on the setting the dialog edits.
  const entries = useSettingsEntries();
  const presetsFor = (job: JobView) => entries.find((entry) => entry.key === job.scheduleKey)?.presets ?? [];

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
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Tasks"
        description="Background work grouped by outcome. Expand a group to inspect or control its tasks."
      />

      <div className="min-h-0 flex-1 space-y-2 overflow-auto p-6">
        {GROUPS.map((group) => {
          const groupedJobs = list.filter((job) => job.group === group.id);
          if (groupedJobs.length === 0) return null;
          const groupOpen = openGroups.has(group.id);
          return (
            <section key={group.id} className="border-border bg-card overflow-hidden rounded-lg border">
              <button
                type="button"
                className="hover:bg-muted/40 flex w-full items-center gap-3 px-4 py-3 text-left"
                aria-expanded={groupOpen}
                aria-controls={`task-group-${group.id}`}
                onClick={() => toggleGroup(group.id)}
              >
                <Icon
                  decorative
                  glyph={groupOpen ? icons.rowCollapse : icons.rowExpand}
                  size="compact"
                  tone="muted"
                />
                <span className="font-medium">{group.label}</span>
                <span
                  className="text-muted-foreground ml-auto text-xs"
                  style={{ display: "flex", alignItems: "center", gap: 8 }}
                >
                  {/* A collapsed group still says it holds an overdue task, with the row's amber dot. */}
                  {overdueTaskCount(groupedJobs) > 0 && <StatusDot tone="warn" label="Overdue" />}
                  {groupSummary(groupedJobs)}
                </span>
              </button>

              {groupOpen && (
                <div id={`task-group-${group.id}`} className="border-border border-t">
                  <Table caption={`${group.label} tasks`} framed={false}>
                    <TableHeader>
                      <TableRow>
                        {COLUMNS.map((column) => (
                          <TableHeaderCell key={column.key} width={column.width}>
                            {column.label}
                          </TableHeaderCell>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {groupedJobs.map((job) => {
                        const dot = statusDot(job);
                        const busy = feedback.isBusy(job.name) || job.running;
                        const pausing = pause.isPending && pause.variables?.name === job.name;
                        const jobOpen = openJobs.has(job.name);
                        return (
                          <Fragment key={job.name}>
                            <TableRow dimmed={Boolean(job.paused)}>
                              <TableCell rowHeader>
                                <button
                                  type="button"
                                  className="flex items-center gap-2 text-left"
                                  aria-expanded={jobOpen}
                                  aria-controls={`task-${job.name}`}
                                  onClick={() => toggleJob(job.name)}
                                >
                                  <span style={{ display: "flex", flexShrink: 0 }} title={dot.label}>
                                    <StatusDot tone={dot.tone} label={job.paused ? "" : dot.label} />
                                  </span>
                                  <span className="font-medium">{job.title}</span>
                                  {job.paused && (
                                    <span className="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px] font-medium">
                                      Paused
                                    </span>
                                  )}
                                </button>
                              </TableCell>
                              <TableCell>
                                <span
                                  className="text-muted-foreground"
                                  title={job.disabledReason ? undefined : job.schedule}
                                >
                                  {job.disabledReason
                                    ? "Not scheduled"
                                    : describeCron(job.schedule, presetsFor(job))}
                                </span>
                              </TableCell>
                              <TableCell>
                                <span className="text-muted-foreground">
                                  {isZero(job.lastRun) ? "—" : formatRelative(job.lastRun ?? zero)}
                                </span>
                              </TableCell>
                              <TableCell>
                                <span className="text-muted-foreground">
                                  {job.disabledReason ? (
                                    "—"
                                  ) : job.paused ? (
                                    "Paused"
                                  ) : job.running ? (
                                    "running…"
                                  ) : isZero(job.nextRun) ? (
                                    "—"
                                  ) : isOverdueTask(job) ? (
                                    <span title="Due, but waiting for a free worker">
                                      <StatusWord kind="overdue" />
                                    </span>
                                  ) : isPast(job.nextRun) ? (
                                    "due now"
                                  ) : (
                                    formatUntil(job.nextRun ?? zero)
                                  )}
                                </span>
                              </TableCell>
                              <TableCell align="end">
                                <Action
                                  icon={jobOpen ? "rowCollapse" : "rowExpand"}
                                  layout="icon"
                                  onPress={() => toggleJob(job.name)}
                                  variant="ghost"
                                >
                                  {`${jobOpen ? "Collapse" : "Expand"} ${job.title}`}
                                </Action>
                              </TableCell>
                            </TableRow>
                            {jobOpen && (
                              <TableRow id={`task-${job.name}`} tone="raised">
                                <TableCell colSpan={5} verticalAlign="top">
                                  <p className="text-muted-foreground max-w-prose text-sm">
                                    {job.description}
                                  </p>
                                  {job.lastResult === "error" && (
                                    <ErrorDetails message={job.lastError} className="mt-2" />
                                  )}
                                  {job.disabledReason && (
                                    <p className="text-muted-foreground mt-2 text-sm">{job.disabledReason}</p>
                                  )}
                                  <ExecutionHistory name={job.name} />
                                  {!job.disabledReason && (
                                    <div className="mt-3 flex items-center gap-1">
                                      <Action
                                        accessibilityLabel={
                                          job.paused ? `Resume ${job.title}` : `Pause ${job.title}`
                                        }
                                        disabled={pausing}
                                        icon={job.paused ? "play" : "pause"}
                                        onPress={() =>
                                          pause.mutate({ name: job.name, data: { paused: !job.paused } })
                                        }
                                        variant="ghost"
                                      >
                                        {job.paused ? "Resume" : "Pause"}
                                      </Action>
                                      <Action icon="edit" onPress={() => setEditing(job)} variant="ghost">
                                        Modify
                                      </Action>
                                      <RunButton busy={busy} onRun={() => run.mutate({ name: job.name })} />
                                    </div>
                                  )}
                                </TableCell>
                              </TableRow>
                            )}
                          </Fragment>
                        );
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
            </section>
          );
        })}

        {list.length === 0 && !jobs.isLoading && (
          <div className="border-border bg-card text-muted-foreground rounded-lg border px-4 py-6 text-center">
            No scheduled tasks.
          </div>
        )}
      </div>

      {editing && (
        <JobEditModal
          job={editing}
          presets={presetsFor(editing)}
          open={editing !== undefined}
          onOpenChange={(open) => !open && setEditing(undefined)}
        />
      )}
    </div>
  );
};

export { TasksPage };
