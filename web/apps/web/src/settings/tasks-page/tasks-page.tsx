import { type JobView, jobsApi, toProblem, unwrap } from "@loomarr/api";
import { formatRelative, formatUntil } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { Pause, Pencil, Play } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { ErrorDetails, ErrorState, RunButton, useRunFeedback } from "@/components/loomarr";
import { Button, StatusDot, type StatusTone } from "@/components/ui";
import { cn } from "@/lib";
import { describeCron } from "./cron-presets";
import { JobEditModal } from "./job-edit-modal/job-edit-modal";

// Settings → Tasks (§18.1): the scheduler's named background jobs, like Sonarr's System →
// Tasks. Each row shows the job, what it does, its schedule (human label, cron in the tooltip),
// the BE's last/next-run, a status dot, and Run/Pause/Modify controls.
// All timing is server-authored — the page renders `lastRun`/`nextRun`/`running` verbatim and
// refetches on the `job` SSE frame (wired in core), never computing countdowns client-side.

// statusDot picks a StatusDot tone for a job: disabled (off, first) > paused (off) > running
// (pending) > error > ok > never-run (off).
//
// ⚠ A failed run is `error`, NOT `live`. The two render the same red and differ only in the
// pulse, and a job that failed hours ago has nothing happening — this row is why the primitive
// grew an `error` tone rather than the page borrowing `live` to get the colour.
//
// ⚠ Disabled is checked FIRST and has its own label. Falling through to "Not run yet" would
// reproduce, one cell over, exactly the ambiguity a disabled row exists to remove: a job
// that cannot run here looking like one that simply has not run yet.
//
// ⚠ Paused is checked SECOND, before `running`. The two are not exclusive — Run now still
// works on a paused job — and a row whose schedule is stopped should say so rather than report
// the state of a one-off the operator triggered themselves.
const statusDot = (job: JobView): { tone: StatusTone; label: string } => {
  if (job.disabledReason) return { tone: "off", label: "Not available on this backend" };
  if (job.paused) return { tone: "off", label: "Paused" };
  if (job.running) return { tone: "pending", label: "Running" };
  if (job.lastResult === "error") return { tone: "error", label: "Failed last run" };
  if (job.lastResult === "ok") return { tone: "ok", label: "OK" };
  return { tone: "off", label: "Not run yet" };
};

// The columns, declared ONCE and rendered into both tables' <colgroup>.
//
// ⚠ The header lives in a separate, non-scrolling <table> from the rows, so nothing makes the
// two agree automatically — a drift here shows as misaligned columns rather than an error.
// ⚠ Keyed by `key`, not by width: two columns share "7rem", and keying on the width produced
// duplicate React keys (a console error, and silently unstable reconciliation).
const COLUMNS = [
  { key: "task", label: "Task", width: "auto", align: "" },
  { key: "frequency", label: "Frequency", width: "10rem", align: "" },
  { key: "lastRun", label: "Last run", width: "7rem", align: "" },
  { key: "nextRun", label: "Next run", width: "7rem", align: "" },
  { key: "actions", label: "Actions", width: "22rem", align: "text-right" },
] as const;

const zero = "0001-01-01T00:00:00Z"; // the Go zero time the BE omits/sends for "never"
const isZero = (t?: string) => !t || t.startsWith("0001-01-01");

const TasksPage = () => {
  const queryClient = useQueryClient();
  const jobs = jobsApi.useJobsList({ query: { retry: false } });
  const refresh = () => queryClient.invalidateQueries({ queryKey: jobsApi.getJobsListQueryKey() });

  // After a Run-now, the job flips to running then records a fresh last-run/next-run. The
  // scheduler also emits a `job` SSE frame that refetches the list, but that races a fast
  // job (start→finish in ms); invalidating on the mutation's own success guarantees the
  // table re-reads the outcome even when the frame is missed.
  // The run indicator is held client-side — see `useRunFeedback` for why a server flag alone
  // shows nothing here.
  const feedback = useRunFeedback();

  const run = jobsApi.useJobsRun({
    mutation: {
      onMutate: ({ name }) => feedback.start(name),
      onSuccess: async (_data, { name }) => {
        // The task's own words, not its id: "Ran library-full-scan" makes an operator match a
        // slug against a row they are looking at.
        const job = list.find((j) => j.name === name);
        toast.success(`Ran ${job?.title ?? name}`);
        await feedback.settle(name, refresh());
      },
      onError: (e, { name }) => {
        feedback.finish(name);
        toast.error(toProblem(e).title ?? "Couldn't run the task");
      },
    },
  });
  const pause = jobsApi.useJobsPause({
    mutation: {
      onSuccess: () => refresh(),
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't change the task"),
    },
  });
  const [editing, setEditing] = useState<JobView | undefined>();

  if (jobs.error) {
    return (
      <div className="p-6">
        <ErrorState error={jobs.error} onRetry={() => jobs.refetch()} />
      </div>
    );
  }
  const list = unwrap(jobs.data, (b) => b.jobs) ?? [];

  return (
    // ⚠ `min-h-0` on a flex COLUMN is what makes the table below able to scroll. A flex item's
    // default `min-height:auto` refuses to shrink past its content, so without it the table
    // grows the page instead of scrolling inside it — which is what cut the last rows off.
    <div className="flex h-full min-h-0 flex-col gap-4 p-6">
      <div>
        <h1 className="font-semibold text-2xl">Tasks</h1>
        <p className="text-muted-foreground text-sm">
          Loomarr's scheduled background jobs. Change how often each runs, pause one, or run it now.
        </p>
      </div>

      {/* ⚠ **The column header sits OUTSIDE the scroll box, not sticky inside it.**
          A sticky `<thead>` inside a scrolling table was three failed attempts: under
          `border-collapse: collapse` the browser paints cell backgrounds over the section's, so
          a background on `<thead>` does nothing; moving it to the `<th>` covers the cells but
          not the gaps between them, and rows kept bleeding through the band — the top row read
          as sliced in half, which is what "the header bounces" looks like. Each fix was
          verified by measurement and each time the pixels disagreed.
          Taking the header out of the scroller removes the whole class of problem: there is no
          overlap to paint, because nothing scrolls past it. Column widths are shared with the
          body via an explicit grid on both, so they cannot drift. */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
        {/* ⚠ A real <table> with a real <thead>, in its own non-scrolling wrapper — NOT divs
            wearing `role="columnheader"`. The div version tripped biome's a11y rule
            (a non-interactive element with a table role is not reachable by keyboard) and it
            was right: hand-rolled table semantics are worse than the element that has them.
            The two tables share COLUMN_WIDTHS via <colgroup>, which is also what keeps their
            columns aligned. */}
        <table className="w-full shrink-0 text-sm">
          <colgroup>
            {COLUMNS.map((c) => (
              <col key={c.key} style={{ width: c.width }} />
            ))}
          </colgroup>
          <thead>
            <tr className="border-border border-b text-left text-muted-foreground text-xs">
              {COLUMNS.map((c) => (
                <th key={c.key} className={cn("px-4 py-2.5 font-medium", c.align)}>
                  {c.label}
                </th>
              ))}
            </tr>
          </thead>
        </table>

        {/* The rows scroll; the header table above them does not. */}
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full text-sm">
            <colgroup>
              {COLUMNS.map((c) => (
                <col key={c.key} style={{ width: c.width }} />
              ))}
            </colgroup>
            <tbody>
              {list.map((job) => {
                const dot = statusDot(job);
                // `running` is the LOCAL mutation being in flight; `job.running` is the server
                // saying a worker has it. Either one means the button should not invite a
                // second click — the gap between them is exactly the queue round trip.
                // Busy = we triggered it and the follow-up refetch has not landed yet, OR the
                // server currently reports a worker holding it. The first term is what makes the
                // state visible at all (see the note on `triggered`); the second covers a run
                // someone else started, or a genuinely long job.
                const busy = feedback.isBusy(job.name) || job.running;
                const pausing = pause.isPending && pause.variables?.name === job.name;
                return (
                  <tr
                    key={job.name}
                    className={cn(
                      "border-border border-b last:border-0",
                      // ⚠ Greyed, not hidden. A paused task must stay legible — the operator
                      // needs to find it to resume it — so this dims without dropping contrast
                      // to the point the text fails to read.
                      job.paused && "opacity-60",
                    )}
                  >
                    <td className="px-4 py-3">
                      <span className="flex items-center gap-2">
                        {/* ⚠ label="" when a visible badge already states the same thing, or a
                          screen reader announces "Paused" twice for one row. The empty string is
                          StatusDot's documented opt-out: it drops role="img" and marks the dot
                          aria-hidden. `title` survives either way, so a sighted user can still
                          hover it. */}
                        <StatusDot tone={dot.tone} label={job.paused ? "" : dot.label} title={dot.label} />
                        <span className="font-medium">{job.title}</span>
                        {job.paused && (
                          <span className="rounded bg-muted px-1.5 py-0.5 font-medium text-[10px] text-muted-foreground uppercase tracking-wide">
                            Paused
                          </span>
                        )}
                      </span>
                      {/* What the task actually does, in the operator's terms. Required on every
                        job (the registry panics without it) precisely so this line is never
                        missing for the row someone is trying to decide about. */}
                      <p className="mt-0.5 max-w-prose text-muted-foreground text-xs">{job.description}</p>
                      {/* The full failure text, collapsed by default (feedback/ErrorDetails). */}
                      {job.lastResult === "error" && (
                        <ErrorDetails message={job.lastError} className="mt-1" />
                      )}
                      {/* The reason sits under the title, where lastError sits for a failed
                        job — it is the same kind of fact: why this row is not doing work. */}
                      {job.disabledReason && (
                        <p className="mt-0.5 text-muted-foreground text-xs">{job.disabledReason}</p>
                      )}
                    </td>
                    <td
                      className="px-4 py-3 text-muted-foreground"
                      title={job.disabledReason ? undefined : job.schedule}
                    >
                      {/* A disabled job has a cron in the registry but will never fire on it.
                        Rendering "Daily at 3 am" next to a job that never runs is the lie
                        this whole concept exists to stop. */}
                      {job.disabledReason ? "Not scheduled" : describeCron(job.schedule)}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {isZero(job.lastRun) ? "—" : formatRelative(job.lastRun ?? zero)}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {/* ⚠ A paused job shows "Paused", never a time. The server zeroes nextRun
                        for one, because the claim skips it — a countdown that will not fire is
                        a promise the page cannot keep. */}
                      {job.disabledReason
                        ? "—"
                        : job.paused
                          ? "Paused"
                          : job.running
                            ? "running…"
                            : isZero(job.nextRun)
                              ? "—"
                              : formatUntil(job.nextRun ?? zero)}
                    </td>
                    <td className="px-4 py-3">
                      {/* ⚠ ABSENT, not disabled. A greyed-out "Run now" invites the operator
                        to hover it looking for a tooltip; the reason is already stated in
                        the row. Modify goes too — editing the cron of a job that never
                        fires changes nothing. The server refuses the call either way
                        (409), so this is presentation, not the gate. */}
                      {job.disabledReason ? null : (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-muted-foreground"
                            disabled={pausing}
                            aria-label={job.paused ? `Resume ${job.title}` : `Pause ${job.title}`}
                            onClick={() => pause.mutate({ name: job.name, data: { paused: !job.paused } })}
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
                          {/* ⚠ Run now stays available on a PAUSED job: pause stops the
                            schedule, not the task. (feedback/RunButton owns the busy state and
                            the indeterminate-progress reasoning.) */}
                          <RunButton busy={busy} onRun={() => run.mutate({ name: job.name })} />
                        </div>
                      )}
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
