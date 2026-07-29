import type { JobView } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TasksPage } from "./tasks-page";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const jobs: JobView[] = [
  {
    name: "reconcile",
    title: "Reconcile acquisitions",
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
    schedule: "0 */15 * * * *",
    scheduleKey: "job.filler_sync.schedule",
    lastResult: "error",
    lastError: "no FILLER_DIR configured",
    running: false,
  },
];

// Dispatches GET /v1/jobs (the list) and POST /v1/jobs/{name}/run (Run now); captures the
// names that were triggered so the test can assert the right job was run.
const stubFetch = (list: JobView[] = jobs) => {
  const runs: string[] = [];
  let listGets = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const u = String(url);
      const method = init?.method ?? "GET";
      const run = u.match(/\/v1\/jobs\/([^/]+)\/run$/);
      if (method === "POST" && run) {
        runs.push(run[1] as string);
        return Promise.resolve(jsonResponse(202, {}));
      }
      if (method === "GET" && /\/v1\/jobs$/.test(u)) listGets += 1;
      return Promise.resolve(jsonResponse(200, { jobs: list }));
    }),
  );
  return { runs, listCount: () => listGets };
};

// A job this backend cannot run — `backup` on Postgres. It carries a cron and a schedule
// key like any other job, because it IS registered; only DisabledReason distinguishes it.
const PG_REASON = "Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule.";
const disabledJobs: JobView[] = [
  jobs[0] as JobView,
  {
    name: "backup",
    title: "Back up the database",
    schedule: "0 30 3 * * *",
    scheduleKey: "backup.schedule",
    lastResult: "",
    running: false,
    disabledReason: PG_REASON,
  },
];

afterEach(() => vi.restoreAllMocks());

describe("TasksPage", () => {
  it("lists each job with its human-readable frequency and last error", async () => {
    stubFetch();
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
    const { runs, listCount } = stubFetch();
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
    stubFetch();
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
    stubFetch(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Back up the database")).toBeInTheDocument();
    expect(screen.getByText(PG_REASON)).toBeInTheDocument();
  });

  // Its schedule reads "Not scheduled", not "Daily at 3 am". The cron exists in the
  // registry but will never fire, and rendering it is the lie this concept exists to stop.
  it("does not advertise a schedule a disabled job will never run on", async () => {
    stubFetch(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Back up the database");
    expect(screen.getByText("Not scheduled")).toBeInTheDocument();
    expect(screen.queryByText("Daily at 3 am")).not.toBeInTheDocument();
  });

  // ⚠ The controls are ABSENT, not disabled — a greyed-out button invites a hunt for a
  // tooltip when the reason is already on the row. The enabled job keeps both, so this
  // asserts a real distinction rather than an empty table.
  it("offers neither Run now nor Modify for a disabled job", async () => {
    stubFetch(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Back up the database");
    expect(screen.getAllByRole("button", { name: /Run now/ })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Modify" })).toHaveLength(1);
  });

  // The status dot must not read "Not run yet" — that is the same ambiguity one cell over.
  it("marks a disabled job as unavailable rather than merely never-run", async () => {
    stubFetch(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Back up the database");
    expect(screen.getByRole("img", { name: "Not available on this backend" })).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Not run yet" })).not.toBeInTheDocument();
  });
});
