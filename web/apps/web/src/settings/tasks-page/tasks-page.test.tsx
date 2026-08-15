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
    group: "acquisitions",
    title: "Reconcile acquisitions",
    description: "Advances approved requests through download, retry, and completion states.",
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
    group: "filler",
    title: "Sync the filler catalogue",
    description: "Scans configured folders and libraries so new filler becomes available for channel breaks.",
    paused: false,
    schedule: "0 */15 * * * *",
    scheduleKey: "job.filler_sync.schedule",
    lastResult: "error",
    lastError: "no FILLER_DIR configured",
    running: false,
  },
];

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

const expandGroup = async (label: string) => {
  const button = (await screen.findByText(label)).closest("button") as HTMLElement;
  if (button.getAttribute("aria-expanded") === "false") await userEvent.click(button);
};

const expandTask = async (group: string, title: string) => {
  await expandGroup(group);
  const button = await screen.findByRole("button", { name: `Expand ${title}` });
  await userEvent.click(button);
};

const PG_REASON = "Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule.";
const disabledJobs: JobView[] = [
  jobs[0] as JobView,
  {
    name: "backup",
    group: "backup",
    title: "Back up the database",
    description: "Writes a database snapshot and prunes backups beyond the configured limit.",
    paused: false,
    schedule: "0 30 3 * * *",
    scheduleKey: "backup.schedule",
    lastResult: "",
    running: false,
    disabledReason: PG_REASON,
  },
];

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

describe("TasksPage", () => {
  it("starts with outcome groups collapsed and reports aggregate health", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });

    const acquisitions = (await screen.findByText("Acquisitions")).closest("button");
    expect(acquisitions).toHaveAttribute("aria-expanded", "false");
    const filler = screen.getByText("Filler").closest("button");
    expect(filler).toHaveTextContent("1 task · 1 failed");
    expect(filler).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Reconcile acquisitions")).not.toBeInTheDocument();
  });

  it("reveals compact task rows within a group", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandGroup("Acquisitions");

    expect(screen.getByText("Reconcile acquisitions")).toBeInTheDocument();
    expect(screen.getByText("Every 5 minutes")).toBeInTheDocument();
  });

  it("keeps descriptions and controls hidden until the task expands", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandGroup("Acquisitions");

    expect(screen.queryByText(/Advances approved requests/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Run now/ })).not.toBeInTheDocument();

    await expandTask("Acquisitions", "Reconcile acquisitions");
    expect(screen.getByText(/Advances approved requests/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Run now/ })).toBeInTheDocument();
  });

  it("runs a task and refetches its status", async () => {
    const { runs, listCount } = stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Acquisitions", "Reconcile acquisitions");
    const initialGets = listCount();

    await userEvent.click(screen.getByRole("button", { name: /Run now/ }));
    await waitFor(() => expect(runs).toEqual(["reconcile"]));
    await waitFor(() => expect(listCount()).toBeGreaterThan(initialGets));
  });

  it("opens the schedule editor from expanded details", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Acquisitions", "Reconcile acquisitions");

    await userEvent.click(screen.getByRole("button", { name: "Modify" }));
    expect(await screen.findByText("Modify Reconcile acquisitions")).toBeInTheDocument();
  });

  it("expands full failure detail only after the task expands", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Filler", "Sync the filler catalogue");

    const summary = screen.getByText("Show error");
    expect(screen.getByText("no FILLER_DIR configured")).not.toBeVisible();
    await userEvent.click(summary);
    expect(screen.getByText("no FILLER_DIR configured")).toBeVisible();
  });

  it("pauses and resumes a task from expanded details", async () => {
    const { pauses } = stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Acquisitions", "Reconcile acquisitions");

    await userEvent.click(screen.getByRole("button", { name: "Pause Reconcile acquisitions" }));
    await waitFor(() => expect(pauses).toEqual([{ name: "reconcile", paused: true }]));
  });

  it("keeps Run now available when the schedule is paused", async () => {
    const paused = [{ ...(jobs[0] as JobView), paused: true }];
    const { runs } = stubJobs(paused);
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Acquisitions", "Reconcile acquisitions");

    expect(screen.getByRole("button", { name: "Resume Reconcile acquisitions" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Run now/ }));
    await waitFor(() => expect(runs).toEqual(["reconcile"]));
  });

  it("lists a disabled task but offers no controls or false schedule", async () => {
    stubJobs(disabledJobs);
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Backup", "Back up the database");

    const detail = screen.getByText(PG_REASON).closest("tr") as HTMLElement;
    expect(screen.getByText("Not scheduled")).toBeInTheDocument();
    expect(within(detail).queryByRole("button", { name: /Run now|Modify/ })).not.toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Not available on this backend" })).toBeInTheDocument();
  });

  it("shows an overdue task without calling it expired", async () => {
    stubJobs([{ ...(jobs[0] as JobView), overdue: true }]);
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandGroup("Acquisitions");

    const row = screen.getByText("Reconcile acquisitions").closest("tr") as HTMLElement;
    expect(within(row).getByText("overdue")).toBeInTheDocument();
    expect(within(row).queryByText("expired")).not.toBeInTheDocument();
  });

  it("shows server and client initiated running states", async () => {
    stubJobs([{ ...(jobs[0] as JobView), running: true }]);
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Acquisitions", "Reconcile acquisitions");
    expect(screen.getByText("Running…").closest("button")).toBeDisabled();
  });

  it("names the task by title in run confirmation", async () => {
    stubJobs();
    render(<TasksPage />, { wrapper: makeWrapper() });
    await expandTask("Acquisitions", "Reconcile acquisitions");

    await userEvent.click(screen.getByRole("button", { name: /Run now/ }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Ran Reconcile acquisitions"));
  });
});
