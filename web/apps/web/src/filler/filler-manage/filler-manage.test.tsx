import {
  getFillerDecisionActivityMockHandler,
  getFillerDecisionDiagnosticsMockHandler,
  getFillerIncomingMockHandler,
  getMeMockHandler,
  getSettingsListMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerManage } from "./filler-manage";

const emptyIncoming = {
  overview: {
    runnable: 0,
    inProgress: 0,
    scheduled: 0,
    needsDecision: 0,
    recoverable: 0,
    admitted: 0,
    rejected: 0,
    dismissed: 0,
  },
  clips: [],
  clipsTotal: 0,
  decisionsTotal: 0,
  reels: [],
  reelsTotal: 0,
  recentlyFiled: [],
  recentlyFiledTotal: 0,
  rejected: [],
  rejectedTotal: 0,
  stageOrder: [],
  total: 0,
};

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <RouterHarness content={children} initialPath="/filler/manage" />
    </QueryClientProvider>
  );
};

describe("FillerManage", () => {
  it("keeps normal outcomes in Activity and diagnostics collapsed", async () => {
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
        ],
        total: 1,
      }),
      getFillerDecisionDiagnosticsMockHandler({ rows: [], total: 0 }),
    );
    render(<FillerManage onEditTags={vi.fn()} />, { wrapper });

    expect(await screen.findByText("Admitted automatically")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show filler diagnostics" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
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
            recovery: "configure_provider",
            retryable: true,
            createdAt: "2026-08-25T12:00:00Z",
          },
        ],
        total: 1,
      }),
      getFillerIncomingMockHandler(emptyIncoming),
      getSettingsListMockHandler({ settings: [], features: { filler: true } }),
    );
    render(<FillerManage onEditTags={vi.fn()} />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Show filler diagnostics" }));
    expect(await screen.findByText("provider unavailable")).toBeInTheDocument();
    expect(screen.getByText(/Recovery: configure provider/)).toBeInTheDocument();
    expect(screen.getByText("Processing queue")).toBeInTheDocument();
  });
});
