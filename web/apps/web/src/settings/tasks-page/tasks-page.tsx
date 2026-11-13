import * as jobsApi from "@loomarr/api/endpoints/jobs";
import type { JobHistoryView } from "@loomarr/api/models/jobHistoryView";
import type { JobView } from "@loomarr/api/models/jobView";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, formatDuration, formatRelative, formatUntil } from "@loomarr/core/format";
import {
  Action,
  Badge,
  Card,
  Icon,
  icons,
  PressableRow,
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
  Text,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Fragment, useState } from "react";
import { toast } from "sonner";
import { ErrorDetails } from "@/components/loomarr/feedback/error-details";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { RunButton } from "@/components/loomarr/feedback/run-button";
import { useRunFeedback } from "@/components/loomarr/feedback/use-run-feedback";
import { PageHeader } from "@/components/loomarr/shell/page-header";
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
    return (
      <Stack marginTop={16}>
        <Text as="p" textRole="hint" tone="secondary">
          Loading execution history…
        </Text>
      </Stack>
    );
  }
  if (query.error) {
    return (
      <Stack align="center" direction="row" gap="inline" marginTop={16} role="alert">
        <Text as="span" textRole="hint" tone="danger">
          Couldn’t load execution history.
        </Text>
        <Action onPress={() => query.refetch()} variant="ghost">
          Retry
        </Action>
      </Stack>
    );
  }
  if (!history) return null;

  return (
    <Stack
      aria-label="Execution history"
      borderColor="borderDecorative"
      borderTopWidth={1}
      marginTop={16}
      paddingTop={12}
      render="section"
    >
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <Text as="h3" textRole="label" tone="primary">
          Past 24 hours
        </Text>
        {history.runCount === 0 ? (
          <Text as="p" textRole="hint" tone="secondary">
            No completed runs.
          </Text>
        ) : (
          <Stack direction="row" gap="4" render="dl" wrap>
            <Stack direction="row" gap="1">
              <Text as="dt" textRole="hint" tone="secondary">
                Runs
              </Text>
              <Text as="dd" textRole="mono" tone="primary">
                {history.truncated ? `${history.runCount}+` : history.runCount}
              </Text>
            </Stack>
            <Stack direction="row" gap="1">
              <Text as="dt" textRole="hint" tone="secondary">
                Failed
              </Text>
              <Text as="dd" textRole="mono" tone={history.failureCount > 0 ? "danger" : "primary"}>
                {history.failureCount}
              </Text>
            </Stack>
            <Stack direction="row" gap="1">
              <Text as="dt" textRole="hint" tone="secondary">
                Average
              </Text>
              <Text as="dd" textRole="mono" tone="primary">
                {executionDuration(history.averageDurationMs)}
              </Text>
            </Stack>
          </Stack>
        )}
      </div>

      {history.recent.length > 0 && (
        <Card className="divide-border divide-y" marginTop={8} render="ul">
          {history.recent.map((run) => {
            const failed = run.result === "error";
            return (
              <Stack
                flexShrink={0}
                key={`${run.startedAt}-${run.trigger}`}
                paddingHorizontal={12}
                paddingVertical={8}
                render="li"
              >
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <StatusDot tone={failed ? "error" : "ok"} label={failed ? "Failed" : "Succeeded"} />
                  <Text as="span" textRole="label" tone="primary">
                    {run.trigger === "manual" ? "Manual" : "Scheduled"}
                  </Text>
                  <Text as="span" textRole="hint" tone="secondary">
                    {formatRelative(run.startedAt)}
                  </Text>
                  <Text as="span" textRole="mono">
                    {executionDuration(run.durationMs)}
                  </Text>
                </div>
                {failed && run.error && <ErrorDetails message={run.error} className="mt-1" />}
              </Stack>
            );
          })}
        </Card>
      )}
    </Stack>
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
      <Stack padding={24}>
        <ErrorState error={jobs.error} onRetry={() => jobs.refetch()} />
      </Stack>
    );
  }

  return (
    <Stack height="100%">
      <PageHeader
        title="Tasks"
        description="Background work grouped by outcome. Expand a group to inspect or control its tasks."
      />

      <Stack className="overflow-auto" flexBasis="0%" flexGrow={1} gap="2" padding={24}>
        {GROUPS.map((group) => {
          const groupedJobs = list.filter((job) => job.group === group.id);
          if (groupedJobs.length === 0) return null;
          const groupOpen = openGroups.has(group.id);
          return (
            <Card key={group.id} overflow="hidden" render="section">
              <PressableRow
                aria-controls={`task-group-${group.id}`}
                aria-expanded={groupOpen}
                onPress={() => toggleGroup(group.id)}
              >
                <Icon
                  decorative
                  glyph={groupOpen ? icons.rowCollapse : icons.rowExpand}
                  size="compact"
                  tone="muted"
                />
                <Text as="span" textRole="cardLabel">
                  {group.label}
                </Text>
                <Stack align="center" direction="row" flexGrow={1} gap="inline" justify="end">
                  {/* A collapsed group still says it holds an overdue task, with the row's amber dot. */}
                  {overdueTaskCount(groupedJobs) > 0 && <StatusDot tone="warn" label="Overdue" />}
                  <Text as="span" textRole="hint" tone="secondary">
                    {groupSummary(groupedJobs)}
                  </Text>
                </Stack>
              </PressableRow>

              {groupOpen && (
                <Stack borderColor="borderDecorative" borderTopWidth={1} id={`task-group-${group.id}`}>
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
                                  <Text as="span" textRole="cardLabel">
                                    {job.title}
                                  </Text>
                                  {job.paused && (
                                    <Badge shape="square" tone="neutral">
                                      Paused
                                    </Badge>
                                  )}
                                </button>
                              </TableCell>
                              <TableCell>
                                <span title={job.disabledReason ? undefined : job.schedule}>
                                  <Text as="span" textRole="body" tone="secondary">
                                    {job.disabledReason
                                      ? "Not scheduled"
                                      : describeCron(job.schedule, presetsFor(job))}
                                  </Text>
                                </span>
                              </TableCell>
                              <TableCell>
                                <Text as="span" textRole="body" tone="secondary">
                                  {isZero(job.lastRun) ? "—" : formatRelative(job.lastRun ?? zero)}
                                </Text>
                              </TableCell>
                              <TableCell>
                                <Text as="span" textRole="body" tone="secondary">
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
                                </Text>
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
                                  <Text as="p" className="max-w-prose" textRole="compact" tone="secondary">
                                    {job.description}
                                  </Text>
                                  {job.lastResult === "error" && (
                                    <ErrorDetails message={job.lastError} className="mt-2" />
                                  )}
                                  {job.disabledReason && (
                                    <Stack marginTop={8}>
                                      <Text as="p" textRole="compact" tone="secondary">
                                        {job.disabledReason}
                                      </Text>
                                    </Stack>
                                  )}
                                  <ExecutionHistory name={job.name} />
                                  {!job.disabledReason && (
                                    <Stack align="center" direction="row" gap="1" marginTop={12}>
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
                                    </Stack>
                                  )}
                                </TableCell>
                              </TableRow>
                            )}
                          </Fragment>
                        );
                      })}
                    </TableBody>
                  </Table>
                </Stack>
              )}
            </Card>
          );
        })}

        {list.length === 0 && !jobs.isLoading && (
          <Card paddingHorizontal={16} paddingVertical={24}>
            <Text as="p" className="text-center" textRole="body" tone="secondary">
              No scheduled tasks.
            </Text>
          </Card>
        )}
      </Stack>

      {editing && (
        <JobEditModal
          job={editing}
          presets={presetsFor(editing)}
          open={editing !== undefined}
          onOpenChange={(open) => !open && setEditing(undefined)}
        />
      )}
    </Stack>
  );
};

export { TasksPage };
