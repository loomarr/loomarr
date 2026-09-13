import type {
  FillerDecisionActivityOutputBody,
  FillerDecisionDiagnosticDTO,
  FillerDecisionDiagnosticsOutputBody,
  FillerIncomingOutputBody,
  MeBody,
} from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { withRouter } from "@/test/story-utils";
import { FillerManage } from "./filler-manage";

const admin: MeBody = {
  id: "admin-1",
  name: "Admin",
  role: "admin",
  disabled: false,
  local: true,
  offlineLogin: false,
  quota: 0,
  autoApprove: false,
};
const incoming: FillerIncomingOutputBody = {
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
  rejected: [],
  rejectedTotal: 0,
  stageOrder: [],
  total: 0,
};

const withManage = (
  activity: FillerDecisionActivityOutputBody,
  diagnostics: FillerDecisionDiagnosticsOutputBody,
  retryOutcome: "unchanged" | "recovered" | "failure" = "unchanged",
): Decorator => {
  let currentDiagnostics = diagnostics;
  return (Story) => {
    window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? (input instanceof Request ? input.method : "GET")).toUpperCase();
      if (method === "POST" && url.includes("/decisions/diagnostics/")) {
        if (retryOutcome === "failure") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                status: 409,
                title: "Retry unavailable",
                detail: "Loomarr couldn’t restart this clip.",
              }),
              { status: 409, headers: { "content-type": "application/problem+json" } },
            ),
          );
        }
        if (retryOutcome === "recovered") currentDiagnostics = { rows: [], total: 0 };
        return Promise.resolve(
          new Response(JSON.stringify({ id: "diagnostic-retry" }), {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
        );
      }
      const body = url.includes("/auth/me")
        ? admin
        : url.includes("/decisions/activity")
          ? activity
          : url.includes("/decisions/diagnostics")
            ? currentDiagnostics
            : url.includes("/filler/incoming")
              ? incoming
              : { settings: [], features: { filler: true } };
      return Promise.resolve(
        new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } }),
      );
    }) as typeof fetch;
    return (
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Story />
      </QueryClientProvider>
    );
  };
};

const fillerFrame: Decorator = (Story) => (
  <div style={{ width: "100%", maxWidth: 960 }}>
    <Story />
  </div>
);

const meta = {
  title: "Filler/Manage",
  component: FillerManage,
  args: { onEditTags: () => {} },
  decorators: [fillerFrame, withRouter("/filler/manage")],
} satisfies Meta<typeof FillerManage>;

export default meta;
type Story = StoryObj<typeof meta>;

const manualDiagnostic = (id: string): FillerDecisionDiagnosticDTO => ({
  id,
  clipHash: "9".repeat(64),
  code: "extraction_failed",
  recovery: { action: "retry_extraction", mode: "manual_retry" },
  retryable: true,
  createdAt: new Date().toISOString(),
});

export const AuditAndCorrection: Story = {
  decorators: [
    withManage(
      {
        rows: [
          {
            id: "event-1",
            decisionId: "decision-1",
            clipHash: "abcdef012345",
            kind: "automatic_admit",
            applicationMode: "shadow",
            createdAt: new Date().toISOString(),
          },
          {
            id: "event-2",
            decisionId: "decision-2",
            actionId: "action-2",
            clipHash: "123456abcdef",
            kind: "correction",
            applicationMode: "shadow",
            createdAt: new Date().toISOString(),
          },
          {
            id: "event-3",
            decisionId: "decision-3",
            actionId: "action-3",
            clipHash: "fedcba654321",
            kind: "reversal",
            applicationMode: "shadow",
            createdAt: new Date().toISOString(),
          },
          {
            id: "event-4",
            decisionId: "decision-4",
            actionId: "action-4",
            clipHash: "987654abcdef",
            kind: "review_abandoned",
            applicationMode: "shadow",
            createdAt: new Date().toISOString(),
          },
        ],
        total: 4,
      },
      { rows: [], total: 0 },
    ),
  ],
};

export const RecoverableDiagnostics: Story = {
  decorators: [
    withManage(
      { rows: [], total: 0 },
      {
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
            createdAt: new Date().toISOString(),
          },
          {
            id: "hold-2",
            clipHash: "123456abcdef",
            code: "budget_exhausted",
            recovery: {
              action: "adjust_budget",
              mode: "configuration",
              destination: "/filler/settings",
            },
            retryable: false,
            createdAt: new Date().toISOString(),
          },
          {
            id: "hold-3",
            clipHash: "fedcba654321",
            code: "extraction_failed",
            recovery: {
              action: "retry_extraction",
              mode: "automatic_retry",
              retryAt: new Date(Date.now() + 30 * 60_000).toISOString(),
            },
            retryable: true,
            createdAt: new Date().toISOString(),
          },
          {
            id: "hold-4",
            clipHash: "987654abcdef",
            code: "extraction_failed",
            recovery: { action: "retry_extraction", mode: "manual_retry" },
            retryable: true,
            createdAt: new Date().toISOString(),
          },
          {
            id: "hold-5",
            clipHash: "a".repeat(64),
            code: "extraction_failed",
            recovery: {
              action: "inspect_media",
              mode: "inspection",
              destination: `/v1/filler/media/${"a".repeat(64)}`,
            },
            retryable: false,
            createdAt: new Date().toISOString(),
          },
          {
            id: "hold-6",
            clipHash: "b".repeat(64),
            code: "schema_invalid",
            recovery: {
              action: "update_policy",
              mode: "configuration",
              destination: "/filler/settings",
            },
            retryable: false,
            createdAt: new Date().toISOString(),
          },
        ],
        total: 6,
      },
    ),
  ],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Show issues" }));
    await canvas.findByText("Connect your AI service");
  },
};

export const ManualRetryFailure: Story = {
  decorators: [
    withManage({ rows: [], total: 0 }, { rows: [manualDiagnostic("failed-hold")], total: 1 }, "failure"),
  ],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Show issues" }));
    await userEvent.click(await canvas.findByRole("button", { name: "Try again" }));
    await canvas.findByRole("alert");
  },
};

export const RecoveredAfterRetry: Story = {
  decorators: [
    withManage({ rows: [], total: 0 }, { rows: [manualDiagnostic("recovered-hold")], total: 1 }, "recovered"),
  ],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Show issues" }));
    await userEvent.click(await canvas.findByRole("button", { name: "Try again" }));
    await canvas.findByText("Everything is working. Nothing needs your attention.");
  },
};
