import {
  getFillerDecisionActivityMockHandler,
  getFillerDecisionDiagnosticsMockHandler,
  getFillerReadinessMockHandler,
  getFillerWatchMockHandler,
  getMeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { readiness } from "@/test/fixtures/filler";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerManage } from "./filler-manage";

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <RouterHarness content={children} initialPath="/filler/manage" />
    </QueryClientProvider>
  );
};

describe("FillerManage", () => {
  // The hub reads the watch line and readiness on every visit. A quiet install by default; tests
  // about the hub override these.
  beforeEach(() => {
    server.use(
      getFillerWatchMockHandler({
        clips: 25,
        health: "healthy",
        held: 0,
        sourcesOn: 1,
        sourcesReady: 1,
        sourcesTotal: 1,
      }),
      getFillerReadinessMockHandler(readiness()),
    );
  });

  it("opens the diagnostics owner when Incoming links to it", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 0 }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <RouterHarness content={<FillerManage />} initialPath="/filler/manage#diagnostics" />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("button", { name: "Hide issues" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(await screen.findByText("Everything is working. Nothing needs your attention.")).toBeVisible();
  });

  it("opens the complete settings index instead of exposing one buried task", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 0 }),
    );
    render(<FillerManage />, { wrapper });

    expect(await screen.findByRole("link", { name: "Open Settings" })).toHaveAttribute(
      "href",
      "/filler/settings",
    );
    expect(screen.queryByRole("link", { name: "Automatic download settings" })).not.toBeInTheDocument();
  });

  // The web mock's Manage hub: one row per tool, each summary read from what the server serves.
  it("lists the admin's tools with their live state", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 2 }),
      // `held` is what the header's "N need you" counts; Incoming's row must say the same number.
      getFillerWatchMockHandler({
        clips: 40,
        health: "healthy",
        held: 3,
        sourcesOn: 1,
        sourcesReady: 1,
        sourcesTotal: 2,
      }),
      getFillerReadinessMockHandler(readiness({ pool: { ...readiness().pool, untagged: 5 } })),
    );
    render(<FillerManage />, { wrapper });

    const tools = await screen.findByRole("region", { name: "Filler tools" });
    expect(await within(tools).findByText("1 of 2 on")).toBeInTheDocument();
    expect(await within(tools).findByText("3 clips need a choice")).toBeInTheDocument();
    expect(
      within(tools).getByText("Product, format, season and audience · 5 clips not tagged yet"),
    ).toBeInTheDocument();
    expect(await within(tools).findByText("2 items can be tried again")).toBeInTheDocument();
    expect(within(tools).getByRole("link", { name: "Open Sources" })).toHaveAttribute(
      "href",
      "/filler/sources",
    );
    expect(within(tools).getByRole("link", { name: "Open Incoming" })).toHaveAttribute(
      "href",
      "/filler/incoming",
    );
    expect(within(tools).getByRole("link", { name: "Open Tags" })).toHaveAttribute(
      "href",
      "/filler/taxonomy",
    );

    await userEvent.click(within(tools).getByRole("button", { name: "Open Problems" }));
    expect(await screen.findByRole("button", { name: "Hide issues" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("shows a member only the tools they can use", async () => {
    server.use(
      getMeMockHandler(me({ name: "Viewer", role: "member" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
    );
    render(<FillerManage />, { wrapper });

    const tools = await screen.findByRole("region", { name: "Filler tools" });
    expect(await within(tools).findByText("Tags")).toBeInTheDocument();
    expect(within(tools).getByText("Settings")).toBeInTheDocument();
    expect(within(tools).queryByText("Sources")).not.toBeInTheDocument();
    expect(within(tools).queryByText("Incoming")).not.toBeInTheDocument();
    expect(within(tools).queryByText("Problems")).not.toBeInTheDocument();
    expect(within(tools).queryByRole("link", { name: "Open Settings" })).not.toBeInTheDocument();
  });

  it("shows automatic outcomes without exposing runtime modes", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({
        rows: [
          {
            id: "event-1",
            decisionId: "decision-1",
            clipHash: "abcdef012345",
            kind: "automatic_admit",
            createdAt: "2026-08-25T12:00:00Z",
          },
          {
            id: "event-2",
            decisionId: "decision-2",
            clipHash: "123456abcdef",
            kind: "automatic_reject",
            createdAt: "2026-08-25T12:00:01Z",
          },
          {
            id: "event-3",
            decisionId: "decision-3",
            clipHash: "fedcba654321",
            kind: "automatic_admit",
            createdAt: "2026-08-25T12:00:02Z",
          },
          {
            id: "event-4",
            decisionId: "decision-4",
            clipHash: "987654abcdef",
            kind: "automatic_reject",
            createdAt: "2026-08-25T12:00:03Z",
          },
        ],
        total: 4,
      }),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 0 }),
    );
    render(<FillerManage />, { wrapper });

    expect(await screen.findAllByText("Added automatically")).toHaveLength(2);
    // Asserted by the rendered colour (Badge's public output), not a Tailwind class: Badge
    // sources colour from @loomarr/design-system's semanticColors via inline style (#970 PR C).
    expect(screen.getAllByText("Added automatically")[0]).toHaveStyle({ color: "#FFB020" });
    expect(screen.getAllByText("Skipped automatically")).toHaveLength(2);
    expect(screen.queryByText(/preview/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show issues" })).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Processing queue")).not.toBeInTheDocument();
  });

  it("shows recoverable failures only after opening Diagnostics", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({
        rows: [
          {
            id: "hold-1",
            clipHash: "abcdef012345",
            code: "provider_unavailable",
            recovery: {
              action: "configure_provider",
              mode: "configuration",
              destination: "/settings/ai",
            },
            retryable: true,
            createdAt: "2026-08-25T12:00:00Z",
          },
        ],
        total: 1,
      }),
    );
    render(<FillerManage />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Show issues" }));
    expect(await screen.findByText("Connect your AI service")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Check AI connection" })).toHaveAttribute("href", "/settings/ai");
    expect(screen.queryByText("Processing queue")).not.toBeInTheDocument();
  });

  it("renders only the server-authored configuration and inspection destinations", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({
        rows: [
          {
            id: "budget",
            clipHash: "b".repeat(64),
            code: "budget_exhausted",
            recovery: {
              action: "adjust_budget",
              mode: "configuration",
              destination: "/filler/settings/limits",
            },
            retryable: false,
            createdAt: "2026-08-25T12:00:00Z",
          },
          {
            id: "policy",
            clipHash: "c".repeat(64),
            code: "schema_invalid",
            recovery: {
              action: "update_policy",
              mode: "configuration",
              destination: "/filler/settings/review",
            },
            retryable: false,
            createdAt: "2026-08-25T12:00:00Z",
          },
          {
            id: "inspect",
            clipHash: "d".repeat(64),
            code: "extraction_failed",
            recovery: {
              action: "inspect_media",
              mode: "inspection",
              destination: `/v1/filler/media/${"d".repeat(64)}`,
            },
            retryable: false,
            createdAt: "2026-08-25T12:00:00Z",
          },
        ],
        total: 3,
      }),
    );
    render(<FillerManage />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Show issues" }));
    expect(await screen.findByRole("link", { name: "Review limits" })).toHaveAttribute(
      "href",
      "/filler/settings/limits",
    );
    expect(
      screen
        .getAllByRole("link", { name: "Open filler settings" })
        .find((link) => link.getAttribute("href") === "/filler/settings/review"),
    ).toBeDefined();
    expect(screen.getByRole("link", { name: "Open clip" })).toHaveAttribute(
      "href",
      `/v1/filler/media/${"d".repeat(64)}`,
    );
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });

  it("explains an automatic retry and does not offer a competing action", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({
        rows: [
          {
            id: "automatic",
            clipHash: "a".repeat(64),
            code: "extraction_failed",
            recovery: {
              action: "retry_extraction",
              mode: "automatic_retry",
              retryAt: new Date(Date.now() + 30 * 60_000).toISOString(),
            },
            retryable: true,
            createdAt: "2026-08-25T12:00:00Z",
          },
        ],
        total: 1,
      }),
    );
    render(<FillerManage />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Show issues" }));
    expect(await screen.findByText(/Loomarr will try again/)).toHaveTextContent("Nothing you need to do.");
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });

  it("starts a manual retry and keeps the server response authoritative", async () => {
    const bodies: unknown[] = [];
    let recovered = false;
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      http.get("*/v1/filler/decisions/diagnostics", () =>
        HttpResponse.json(
          recovered
            ? { rows: [], total: 0 }
            : {
                rows: [
                  {
                    id: "manual",
                    clipHash: "a".repeat(64),
                    code: "extraction_failed",
                    recovery: { action: "retry_extraction", mode: "manual_retry" },
                    retryable: true,
                    createdAt: "2026-08-25T12:00:00Z",
                  },
                ],
                total: 1,
              },
        ),
      ),
      http.post("*/v1/filler/decisions/diagnostics/manual/actions", async ({ request }) => {
        bodies.push(await request.json());
        recovered = true;
        return HttpResponse.json({ id: "recovery-1" });
      }),
    );
    render(<FillerManage />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Show issues" }));
    await userEvent.click(await screen.findByRole("button", { name: "Try again" }));
    expect(
      await screen.findByText("Everything is working. Nothing needs your attention."),
    ).toBeInTheDocument();
    expect(bodies).toEqual([{ actionId: expect.any(String), action: "retry" }]);
  });

  it("keeps a failed retry held and explains the failure", async () => {
    const actionIDs: string[] = [];
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({ rows: [], total: 0 }),
      getFillerDecisionDiagnosticsMockHandler({
        rows: [
          {
            id: "manual",
            clipHash: "a".repeat(64),
            code: "extraction_failed",
            recovery: { action: "retry_extraction", mode: "manual_retry" },
            retryable: true,
            createdAt: "2026-08-25T12:00:00Z",
          },
        ],
        total: 1,
      }),
      http.post("*/v1/filler/decisions/diagnostics/manual/actions", async ({ request }) => {
        const body = (await request.json()) as { actionId: string };
        actionIDs.push(body.actionId);
        return HttpResponse.json(
          { title: "Retry unavailable", detail: "This issue changed before the retry could start." },
          { status: 409 },
        );
      }),
    );
    render(<FillerManage />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Show issues" }));
    await userEvent.click(await screen.findByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This clip is still on hold. This issue changed before the retry could start.",
    );
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(actionIDs).toHaveLength(2));
    expect(actionIDs[1]).toBe(actionIDs[0]);
    expect(screen.getByText("This clip couldn’t be processed")).toBeInTheDocument();
  });
});
