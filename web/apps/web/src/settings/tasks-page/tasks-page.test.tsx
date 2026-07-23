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
const stubFetch = () => {
  const runs: string[] = [];
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
      return Promise.resolve(jsonResponse(200, { jobs }));
    }),
  );
  return { runs };
};

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

  it("fires POST /v1/jobs/{name}/run when Run now is clicked", async () => {
    const user = userEvent.setup();
    const { runs } = stubFetch();
    render(<TasksPage />, { wrapper: makeWrapper() });

    await screen.findByText("Reconcile acquisitions");
    const runButtons = screen.getAllByRole("button", { name: /Run now/ });
    await user.click(runButtons[0] as HTMLElement);

    await waitFor(() => expect(runs).toEqual(["reconcile"]));
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
});
