import type { FillerReadinessDTO, MeBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame, withRouter } from "@/test/story-utils";
import { FillerOverview } from "./filler-overview";

const readiness: FillerReadinessDTO = {
  ready: true,
  nextAction: "none",
  repairs: { count: 0 },
  fetch: { enabled: true, catalogClips: 48 },
  storage: {
    automatic: true,
    state: "healthy",
    totalBytes: 500 * 1024 ** 3,
    freeBytes: 200 * 1024 ** 3,
    managedBytes: 2 * 1024 ** 3,
    reservedBytes: 0,
    filesystemReservedBytes: 0,
    softBudgetBytes: 20 * 1024 ** 3,
    hardReserveBytes: 10 * 1024 ** 3,
    availableBytes: 18 * 1024 ** 3,
  },
  pipeline: {
    runnable: 0,
    scheduled: 0,
    inProgress: 0,
    needsDecision: 0,
    recoverable: 0,
    ready: 48,
    complete: 0,
    rejected: 22,
    dismissed: 0,
  },
  pool: {
    clips: 48,
    breakBody: 38,
    eligible: 42,
    untagged: 0,
    channels: [
      {
        channelId: "ch-7",
        name: "Saturday Mornings",
        number: 7,
        level: "exact",
        total: 31,
        durationMs: 780_000,
        categories: 6,
        brands: 19,
      },
    ],
  },
  acquisitions: [],
};

// The Overview reads readiness and, since #1908, who is looking: a member sees the state but no
// admin-only button. Each read gets its own typed body.
const me = (role: MeBody["role"]): MeBody => ({
  id: `${role}-1`,
  name: role === "admin" ? "Admin" : "Member",
  role,
  disabled: false,
  local: true,
  offlineLogin: false,
  quota: 0,
  autoApprove: false,
});

const withReadiness =
  (workspace: FillerReadinessDTO = readiness, role: MeBody["role"] = "admin"): Decorator =>
  (Story) => {
    window.fetch = ((input: RequestInfo | URL) => {
      const body = String(input).includes("/auth/me") ? me(role) : workspace;
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    }) as typeof fetch;
    return (
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Story />
      </QueryClientProvider>
    );
  };

const meta = {
  title: "Filler/Overview",
  component: FillerOverview,
  tags: ["route"],
  decorators: [widthFrame(960), withRouter("/filler")],
} satisfies Meta<typeof FillerOverview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const HealthyZeroWork: Story = {
  decorators: [withReadiness()],
};

export const RecoverableFailure: Story = {
  decorators: [
    withReadiness({ ...readiness, ready: false, nextAction: "retry_failed_work", actionCount: 2 }),
  ],
};

export const ClipsNeedAChoice: Story = {
  decorators: [withReadiness({ ...readiness, ready: false, nextAction: "review_incoming", actionCount: 3 })],
};

// A member sees the exception and who can act on it, with no button that would bounce them.
export const ClipsNeedAChoiceMember: Story = {
  decorators: [
    withReadiness({ ...readiness, ready: false, nextAction: "review_incoming", actionCount: 3 }, "member"),
  ],
};

export const EmptyLibrary: Story = {
  decorators: [
    withReadiness({
      ...readiness,
      ready: false,
      nextAction: "add_filler",
      fetch: { enabled: true, catalogClips: 0 },
      pipeline: { ...readiness.pipeline, ready: 0, rejected: 0 },
      pool: { clips: 0, breakBody: 0, eligible: 0, untagged: 0, channels: [] },
    }),
  ],
};
