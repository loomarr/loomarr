import {
  getFillerDecisionActivityMockHandler,
  getFillerDecisionDiagnosticsMockHandler,
  getMeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
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
  it("distinguishes shadow and applied automatic outcomes", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerDecisionActivityMockHandler({
        rows: [
          {
            id: "event-1",
            decisionId: "decision-1",
            clipHash: "abcdef012345",
            kind: "automatic_admit",
            applicationMode: "shadow",
            createdAt: "2026-08-25T12:00:00Z",
          },
          {
            id: "event-2",
            decisionId: "decision-2",
            clipHash: "123456abcdef",
            kind: "automatic_reject",
            applicationMode: "shadow",
            createdAt: "2026-08-25T12:00:01Z",
          },
          {
            id: "event-3",
            decisionId: "decision-3",
            clipHash: "fedcba654321",
            kind: "automatic_admit",
            applicationMode: "applied",
            createdAt: "2026-08-25T12:00:02Z",
          },
          {
            id: "event-4",
            decisionId: "decision-4",
            clipHash: "987654abcdef",
            kind: "automatic_reject",
            applicationMode: "applied",
            createdAt: "2026-08-25T12:00:03Z",
          },
        ],
        total: 4,
      }),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 0 }),
    );
    render(<FillerManage />, { wrapper });

    expect(await screen.findByText("Would add (preview)")).toHaveClass("text-caution");
    expect(screen.getByText("Would skip (preview)")).toHaveClass("text-caution");
    expect(screen.getByText("Added automatically")).toHaveClass("text-signal");
    expect(screen.getByText("Skipped automatically")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show issues" })).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Processing queue")).not.toBeInTheDocument();
  });

  it("does not claim an applied effect when decision mode is unavailable", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      http.get("*/v1/filler/decisions/activity", () =>
        HttpResponse.json({
          rows: [
            {
              id: "event-unknown",
              decisionId: "decision-unknown",
              clipHash: "abcdef012345",
              kind: "automatic_admit",
              applicationMode: "automatic",
              createdAt: "2026-08-25T12:00:00Z",
            },
            {
              id: "event-omitted",
              decisionId: "decision-omitted",
              clipHash: "123456abcdef",
              kind: "automatic_reject",
              createdAt: "2026-08-25T12:00:01Z",
            },
          ],
          total: 2,
        }),
      ),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 0 }),
    );
    render(<FillerManage />, { wrapper });

    const unavailable = await screen.findAllByText("Status unavailable");
    expect(unavailable).toHaveLength(2);
    for (const badge of unavailable) expect(badge).toHaveClass("text-caution");
    expect(screen.queryByText("Added automatically")).not.toBeInTheDocument();
    expect(screen.queryByText("Skipped automatically")).not.toBeInTheDocument();
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
              destination: "/filler/settings",
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
              destination: "/filler/settings",
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
      "/filler/settings",
    );
    expect(screen.getByRole("link", { name: "Open filler settings" })).toHaveAttribute(
      "href",
      "/filler/settings",
    );
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
