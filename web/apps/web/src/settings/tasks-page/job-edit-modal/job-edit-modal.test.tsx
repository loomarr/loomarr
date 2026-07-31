import type { JobView } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { JobEditModal } from "./job-edit-modal";

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

// A job whose current schedule is a known preset ("Every 5 minutes"), so the dropdown opens
// on that preset and no Advanced field is shown.
const job: JobView = {
  name: "reconcile",
  title: "Reconcile acquisitions",
  description: "Checks in-flight downloads.",
  paused: false,
  schedule: "0 */5 * * * *",
  scheduleKey: "job.reconcile.schedule",
  lastResult: "ok",
  running: false,
};

// Captures every PATCH /v1/settings body so the test can assert the exact edit.
const stubFetch = () => {
  const patches: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((_url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (method === "PATCH") {
        patches.push(init?.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(jsonResponse(200, { results: {} }));
      }
      return Promise.resolve(jsonResponse(200, { entries: [] }));
    }),
  );
  return { patches };
};

afterEach(() => vi.restoreAllMocks());

describe("JobEditModal", () => {
  it("PATCHes the job's scheduleKey with the chosen preset cron", async () => {
    const user = userEvent.setup();
    const { patches } = stubFetch();
    render(<JobEditModal job={job} open onOpenChange={() => {}} />, { wrapper: makeWrapper() });

    // Pick a different preset, then save.
    await user.click(await screen.findByRole("combobox", { name: "Frequency" }));
    await user.click(await screen.findByRole("option", { name: "Every hour" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    // The edit targets THIS job's settings key, with the preset's canonical cron.
    expect(patches[0]).toEqual({ edits: { "job.reconcile.schedule": "0 0 * * * *" } });
  });

  it("reveals the raw cron field only for the Custom (advanced) choice, and saves its value", async () => {
    const user = userEvent.setup();
    const { patches } = stubFetch();
    render(<JobEditModal job={job} open onOpenChange={() => {}} />, { wrapper: makeWrapper() });

    // No advanced field until Custom is chosen.
    expect(screen.queryByRole("textbox", { name: "Cron expression" })).not.toBeInTheDocument();

    await user.click(await screen.findByRole("combobox", { name: "Frequency" }));
    await user.click(await screen.findByRole("option", { name: /Custom/ }));

    const cronField = await screen.findByRole("textbox", { name: "Cron expression" });
    await user.clear(cronField);
    await user.type(cronField, "0 30 4 * * 1");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ edits: { "job.reconcile.schedule": "0 30 4 * * 1" } });
  });

  it("opens straight into the advanced field when the current schedule matches no preset", async () => {
    stubFetch();
    const custom: JobView = { ...job, schedule: "0 15 2 * * 3" };
    render(<JobEditModal job={custom} open onOpenChange={() => {}} />, { wrapper: makeWrapper() });

    // A non-preset schedule seeds the dropdown to Custom, so the raw field is visible with it.
    const cronField = await screen.findByRole("textbox", { name: "Cron expression" });
    expect(cronField).toHaveValue("0 15 2 * * 3");
  });
});
