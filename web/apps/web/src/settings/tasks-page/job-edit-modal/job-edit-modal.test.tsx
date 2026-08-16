import type { JobView } from "@loomarr/api";
import { getSettingsPatchMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { JobEditModal } from "./job-edit-modal";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// A job whose current schedule is a known preset ("Every 5 minutes"), so the dropdown opens
// on that preset and no Advanced field is shown.
const job: JobView = {
  name: "reconcile",
  group: "acquisitions",
  title: "Reconcile acquisitions",
  description: "Checks in-flight downloads.",
  paused: false,
  schedule: "0 */5 * * * *",
  scheduleKey: "job.reconcile.schedule",
  lastResult: "ok",
  running: false,
};

// Captures every PATCH /v1/settings body so the test can assert the exact edit.
//
// ⚠ This replaced a local `stubFetch` that swapped out global `fetch` (V53d). Two things changed
// and both matter:
//
//   1. The ROUTES are no longer written here. `getSettingsPatchMockHandler` carries the method and
//      path from the spec, so a renamed endpoint is fixed by a regenerate. The old stub matched on
//      `init?.method === "PATCH"` alone — it would have accepted a PATCH to ANY url, including one
//      the component should never have called.
//   2. Request capture survives, because the generated handler's override may be a FUNCTION of the
//      request. That was the capability worth checking before migrating: 12 of the 31 files assert
//      on what was sent, not just on what came back.
//
// ⚠ ONLY the PATCH is stubbed, and the migration is what revealed that. The old `stubFetch`
// answered every non-PATCH call with `{ entries: [] }`, which read as "the modal loads the
// settings list on mount" — it does not. Dropping that branch changes nothing, and the
// unhandled-request guard proves it rather than leaving it to inspection: an unmatched GET would
// now fail this test by name. A catch-all stub cannot tell "handled" from "never asked for".
const captureSettings = () => {
  const patches: unknown[] = [];
  server.use(
    getSettingsPatchMockHandler(async ({ request }) => {
      patches.push(await request.json());
      // ⚠ `results` is an ARRAY. The stub this replaced returned `{ results: {} }` — an object the
      // API never produces — and nothing caught it, because a hand-rolled stub is untyped by
      // construction. The generated handler is typed against SettingsPatchOutputBody, so the
      // wrong shape is now a compile error rather than a fiction the test agrees with.
      return { results: [] };
    }),
  );
  return { patches };
};

describe("JobEditModal", () => {
  it("PATCHes the job's scheduleKey with the chosen preset cron", async () => {
    const user = userEvent.setup();
    const { patches } = captureSettings();
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
    const { patches } = captureSettings();
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
    captureSettings();
    const custom: JobView = { ...job, schedule: "0 15 2 * * 3" };
    render(<JobEditModal job={custom} open onOpenChange={() => {}} />, { wrapper: makeWrapper() });

    // A non-preset schedule seeds the dropdown to Custom, so the raw field is visible with it.
    const cronField = await screen.findByRole("textbox", { name: "Cron expression" });
    expect(cronField).toHaveValue("0 15 2 * * 3");
  });
});
