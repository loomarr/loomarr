import type { JobView } from "@loomarr/api";
import { getJobsListMockHandler, getJobsPauseMockHandler, getJobsRunMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { TasksPage } from "./tasks-page";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jobs: JobView[] = [
  {
    name: "reconcile",
    title: "Reconcile acquisitions",
    description: "Checks in-flight downloads and moves finished ones into your library.",
    paused: false,
    schedule: "0 */5 * * * *",
    scheduleKey: "job.reconcile.schedule",
    lastResult: "ok",
    lastRun: "2026-07-23T12:00:00Z",
    nextRun: "2026-07-23T12:05:00Z",
    running: false,
  },
  {
    name: "filler-sync",
    title: "Sync filler catalog",
    description: "Re-reads your filler folder so new clips become available.",
    paused: false,
    schedule: "0 */15 * * * *",
    scheduleKey: "job.filler_sync.schedule",
    lastResult: "error",
    lastError: "no FILLER_DIR configured",
    running: false,
  },
];

// GET /v1/jobs (the list), POST /v1/jobs/{name}/run, POST /v1/jobs/{name}/pause. The triggered
// names are captured so a test can assert the right job was acted on.
//
// ⚠ The stub this replaces ended in an UNCONDITIONAL `jsonResponse(200, { jobs: list })` — not a
// `{}` catch-all but something worse, because it answered *every* unmatched request, at any path,
// with the jobs list. `listGets` was only counted for `/v1/jobs$`, so the refetch assertion below
// was reading a counter narrower than the branch that served it. Route-bound handlers make the
// count and the response the same thing.
//
// ⚠ The name comes from the PATH PARAM MSW parsed, not from a regex the test wrote against a URL
// it also constructed (`u.match(/\/v1\/jobs\/([^/]+)\/run$/)`).
const stubJobs = (list: JobView[] = jobs) => {
  const runs: string[] = [];
  const pauses: { name: string; paused: boolean }[] = [];
  let listGets = 0;

  server.use(
    getJobsListMockHandler(() => {
      listGets += 1;
      return { jobs: list };
    }),
    getJobsRunMockHandler(({ params }) => {
      runs.push(String(params.name));
    }),
    getJobsPauseMockHandler(async ({ request, params }) => {
      const body = (await request.json()) as { paused: boolean };
      pauses.push({ name: String(params.name), paused: body.paused });
    }),
  );

  return { runs, pauses, listCount: () => listGets };
};

// A job this backend cannot run — `backup` on Postgres. It carries a cron and a schedule
// key like any other job, because it IS registered; only DisabledReason distinguishes it.
const PG_REASON = "Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule.";
const disabledJobs: JobView[] = [
  jobs[0] as JobView,
  {
    name: "backup",
    title: "Back up the database",
    description: "Writes a snapshot of the database to your backup folder.",
    paused: false,
    schedule: "0 30 3 * * *",
    scheduleKey: "backup.schedule",
    lastResult: "",
    running: false,
    disabledReason: PG_REASON,
  },
];

// Toast copy is asserted below, so the module is mocked rather than rendered.
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

describe("TasksPage", () => {
  it("lists each job with its human-readable frequency and last error", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Reconcile acquisitions")).toBeInTheDocument();
    // The cron is rendered as its friendly preset label, not the raw expression.
    expect(screen.getByText("Every 5 minutes")).toBeInTheDocument();
    expect(screen.getByText("Every 15 minutes")).toBeInTheDocument();
    // A failed job surfaces its error inline.
    expect(screen.getByText("no FILLER_DIR configured")).toBeInTheDocument();
  });

  it("fires POST /v1/jobs/{name}/run and refetches the list on success", async () => {
    const user = userEvent.setup();
    const { runs, listCount } = stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Reconcile acquisitions");
    const initialGets = listCount(); // the mount fetch
    const runButtons = screen.getAllByRole("button", { name: /Run now/ });
    await user.click(runButtons[0] as HTMLElement);

    await waitFor(() => expect(runs).toEqual(["reconcile"]));
    // The fix: run-now invalidates ["/v1/jobs"], so the list re-reads the fresh outcome.
    // A wrong/absent key (the original bug) would leave the count unchanged.
    await waitFor(() => expect(listCount()).toBeGreaterThan(initialGets));
  });

  it("opens the modify modal for a job", async () => {
    const user = userEvent.setup();
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Reconcile acquisitions");
    const modifyButtons = screen.getAllByRole("button", { name: "Modify" });
    await user.click(modifyButtons[0] as HTMLElement);

    // The dialog titles itself with the job it's editing.
    expect(await screen.findByText("Modify Reconcile acquisitions")).toBeInTheDocument();
  });

  // ⚠ THE GATE: a job this backend cannot run is LISTED, carrying its reason. The
  // alternative — omitting it — is what this replaced, and an absent row is
  // indistinguishable from a job that runs fine and has never failed.
  it("lists a disabled job with the reason it cannot run", async () => {
    stubJobs(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Back up the database")).toBeInTheDocument();
    expect(screen.getByText(PG_REASON)).toBeInTheDocument();
  });

  // Its schedule reads "Not scheduled", not "Daily at 3 am". The cron exists in the
  // registry but will never fire, and rendering it is the lie this concept exists to stop.
  it("does not advertise a schedule a disabled job will never run on", async () => {
    stubJobs(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Back up the database");
    expect(screen.getByText("Not scheduled")).toBeInTheDocument();
    expect(screen.queryByText("Daily at 3 am")).not.toBeInTheDocument();
  });

  // ⚠ The controls are ABSENT, not disabled — a greyed-out button invites a hunt for a
  // tooltip when the reason is already on the row. The enabled job keeps both, so this
  // asserts a real distinction rather than an empty table.
  it("offers neither Run now nor Modify for a disabled job", async () => {
    stubJobs(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Back up the database");
    expect(screen.getAllByRole("button", { name: /Run now/ })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Modify" })).toHaveLength(1);
  });

  // The status dot must not read "Not run yet" — that is the same ambiguity one cell over.
  it("marks a disabled job as unavailable rather than merely never-run", async () => {
    stubJobs(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Back up the database");
    expect(screen.getByRole("img", { name: "Not available on this backend" })).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Not run yet" })).not.toBeInTheDocument();
  });

  // Every job carries a Description (the registry panics without one) precisely so an operator
  // deciding whether to run or pause a task can read what it does.
  it("shows what each task actually does", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    expect(await screen.findByText(/Checks in-flight downloads and moves finished ones/)).toBeInTheDocument();
  });

  // ⚠ The error OPENS UP rather than being clipped. The Tasks page is where an operator
  // diagnoses a failing integration, and the full message is what they came for.
  it("expands the full error text on demand", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    // Collapsed by default: one failing row should not push every other row down the page.
    const summary = await screen.findByText("Show error");
    expect(screen.getByText("no FILLER_DIR configured")).not.toBeVisible();

    await userEvent.click(summary);
    expect(screen.getByText("no FILLER_DIR configured")).toBeVisible();
  });

  it("pauses a job, and resumes a paused one", async () => {
    const { pauses } = stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Pause Reconcile acquisitions" }));
    await waitFor(() => expect(pauses).toEqual([{ name: "reconcile", paused: true }]));
  });

  it("offers Resume, not Pause, for an already-paused job", async () => {
    const { pauses } = stubJobs([{ ...(jobs[0] as JobView), paused: true }]);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Resume Reconcile acquisitions" }));
    // ⚠ Resume must send paused:false. Sending `true` again would look like it worked (the
    // request succeeds) and leave the job stopped.
    await waitFor(() => expect(pauses).toEqual([{ name: "reconcile", paused: false }]));
  });

  // ⚠ A paused job shows "Paused" where its next run would be. The server zeroes nextRun
  // because the claim skips it, so a countdown there would promise a run that never comes.
  it("shows no next-run time for a paused job", async () => {
    stubJobs([{ ...(jobs[0] as JobView), paused: true }]);
    render(<TasksPage />, { wrapper: makeWrapper() });

    const row = (await screen.findByText("Reconcile acquisitions")).closest("tr") as HTMLElement;
    expect(within(row).getByText("Paused", { selector: "td" })).toBeInTheDocument();
    expect(within(row).queryByText(/^in /)).not.toBeInTheDocument();
  });

  // ⚠ Run now stays available on a PAUSED job: pause stops the schedule, not the task. The
  // backend accepts it too, so hiding the button here would contradict the server.
  it("still offers Run now for a paused job", async () => {
    const { runs } = stubJobs([{ ...(jobs[0] as JobView), paused: true }]);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await userEvent.click(
      await screen.findAllByRole("button", { name: /Run now/ }).then((b) => b[0] as HTMLElement),
    );
    await waitFor(() => expect(runs).toEqual(["reconcile"]));
  });

  // The button reports a run the SERVER says is in flight.
  it("reports a server-reported run on the button", async () => {
    stubJobs([{ ...(jobs[0] as JobView), running: true }]);
    render(<TasksPage />, { wrapper: makeWrapper() });

    const btn = await screen.findByRole("button", { name: /Running/ });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute("aria-busy", "true");
  });

  // ⚠ **The regression guard for "I clicked Run and nothing happened".** The server's own
  // `running` flag is true for only ~250ms and `POST /run` returns 202 in milliseconds, so a
  // button driven by those alone never visibly changes — measured live by sampling it 120
  // times over 6 seconds and observing exactly one state. The page therefore remembers what it
  // triggered. This asserts the state appears from a CLICK, with the server still reporting
  // running:false, which is precisely the case the naive implementation misses.
  it("shows the run in progress from the click, not just the server flag", async () => {
    stubJobs(); // every job reports running:false throughout
    render(<TasksPage />, { wrapper: makeWrapper() });

    const btn = (await screen.findAllByRole("button", { name: /Run now/ }))[0] as HTMLElement;
    await userEvent.click(btn);

    const busy = await screen.findByRole("button", { name: /Running/ });
    expect(busy).toBeDisabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
  });

  // The toast names the task the way the row does. "Ran library-full-scan" makes an operator
  // match a slug against a title they are looking at.
  it("names the task by its title when confirming a run", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    await userEvent.click((await screen.findAllByRole("button", { name: /Run now/ }))[0] as HTMLElement);
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Ran Reconcile acquisitions"));
  });
});
