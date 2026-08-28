import type {
  FillerDecisionActivityOutputBody,
  FillerDecisionDiagnosticsOutputBody,
  FillerIncomingOutputBody,
  MeBody,
} from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame, withRouter } from "@/test/story-utils";
import { FillerManage } from "./filler-manage";

const admin: MeBody = {
  id: "admin-1",
  name: "Admin",
  role: "admin",
  disabled: false,
  local: true,
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
  recentlyFiled: [],
  recentlyFiledTotal: 0,
  rejected: [],
  rejectedTotal: 0,
  stageOrder: [],
  total: 0,
};

const withManage =
  (activity: FillerDecisionActivityOutputBody, diagnostics: FillerDecisionDiagnosticsOutputBody): Decorator =>
  (Story) => {
    window.fetch = ((input: RequestInfo | URL) => {
      const url = String(input);
      const body = url.includes("/auth/me")
        ? admin
        : url.includes("/decisions/activity")
          ? activity
          : url.includes("/decisions/diagnostics")
            ? diagnostics
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

const meta = {
  title: "Filler/Manage",
  component: FillerManage,
  args: { onEditTags: () => {} },
  decorators: [widthFrame(960), withRouter("/filler/manage")],
} satisfies Meta<typeof FillerManage>;

export default meta;
type Story = StoryObj<typeof meta>;

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
            recovery: "configure_provider",
            retryable: true,
            createdAt: new Date().toISOString(),
          },
          {
            id: "hold-2",
            clipHash: "123456abcdef",
            code: "budget_exhausted",
            recovery: "adjust_budget",
            retryable: false,
            createdAt: new Date().toISOString(),
          },
        ],
        total: 2,
      },
    ),
  ],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Show filler diagnostics" }));
    await canvas.findByText("provider unavailable");
  },
};
