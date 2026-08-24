import type { HealthReport } from "@loomarr/api/models/healthReport";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { CurrentHealthCard } from "./startup-report-page";

const observedAt = Date.UTC(2026, 7, 23, 18, 42);
const base: HealthReport = {
  generationId: "startup-story",
  generation: 4,
  version: "v0.2.1",
  processStartedAt: observedAt - 3_600_000,
  generationStartedAt: observedAt - 3_000,
  updatedAt: observedAt,
  nextRefreshAt: observedAt + 180_000,
  state: "healthy",
  checks: [
    {
      key: "configuration",
      label: "Configuration",
      required: true,
      mode: "startup",
      status: "passed",
      observedAt: observedAt - 2_900,
      detail: "Valid",
    },
    {
      key: "database",
      label: "Database and migrations",
      required: true,
      mode: "continuous",
      status: "passed",
      observedAt,
      freshUntil: observedAt + 180_000,
      detail: "Available",
    },
    {
      key: "media_server",
      label: "Media server",
      required: false,
      mode: "continuous",
      status: "passed",
      observedAt,
      freshUntil: observedAt + 180_000,
      detail: "Available",
    },
    {
      key: "tmdb",
      label: "TMDB",
      required: false,
      mode: "continuous",
      status: "skipped",
      observedAt,
      detail: "Not configured",
      remediationRoute: "/settings/connections",
    },
  ],
};

const meta = {
  title: "Diagnostics/CurrentHealth",
  component: CurrentHealthCard,
  decorators: [widthFrame(920)],
  args: { report: base },
  render: (args) => <CurrentHealthCard {...args} onRefresh={() => undefined} />,
} satisfies Meta<typeof CurrentHealthCard>;

type Story = StoryObj<typeof meta>;

const Healthy: Story = {};
const Degraded: Story = {
  args: {
    report: {
      ...base,
      state: "degraded",
      checks: base.checks.map((check) =>
        check.key === "media_server"
          ? {
              ...check,
              status: "warning" as const,
              detail: "Configured but unavailable",
              remediationRoute: "/settings/connections",
            }
          : check,
      ),
    },
  },
};
const Unhealthy: Story = {
  args: {
    report: {
      ...base,
      state: "unhealthy",
      checks: base.checks.map((check) =>
        check.key === "database"
          ? {
              ...check,
              status: "stale" as const,
              detail: "No fresh database observation arrived",
              remediationRoute: "/settings/system/database",
            }
          : check,
      ),
    },
  },
};

export default meta;
export { Degraded, Healthy, Unhealthy };
