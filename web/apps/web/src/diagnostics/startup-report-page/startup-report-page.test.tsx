import {
  getGetCurrentHealthMockHandler,
  getListStartupReportsMockHandler,
  getRefreshCurrentHealthMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { StartupReportPage } from "./startup-report-page";

const health = {
  generationId: "startup-2",
  generation: 2,
  version: "v1.2.3",
  processStartedAt: 1_780_000_000_000,
  generationStartedAt: 1_780_000_001_000,
  updatedAt: 1_780_000_002_000,
  nextRefreshAt: 1_780_000_062_000,
  state: "degraded" as const,
  checks: [
    {
      key: "database",
      label: "Database and migrations",
      required: true,
      mode: "continuous" as const,
      status: "passed" as const,
      observedAt: 1_780_000_002_000,
      freshUntil: 1_780_000_182_000,
      detail: "Available",
    },
    {
      key: "media_server",
      label: "Media server",
      required: false,
      mode: "continuous" as const,
      status: "warning" as const,
      observedAt: 1_780_000_002_000,
      freshUntil: 1_780_000_182_000,
      detail: "Configured but unavailable",
      remediationRoute: "/settings/connections",
    },
  ],
};

const startup = (id: string, generation: number) => ({
  id,
  generation,
  version: "v1.2.3",
  processStartedAt: 1_780_000_000_000,
  generationStartedAt: 1_780_000_001_000 - generation * 1_000,
  generationEndedAt: 1_780_000_002_000,
  durationMillis: 1_250,
  state: "ready" as const,
  checks: [
    {
      key: "database",
      label: "Database and migrations",
      required: true,
      mode: "continuous" as const,
      status: "passed" as const,
      durationMillis: 1_200,
      detail: "SQLite ready",
    },
  ],
});

describe("StartupReportPage", () => {
  it("leads with live Current Health and keeps prior startups as history", async () => {
    const current = startup("startup-2", 2);
    const previous = startup("startup-1", 1);
    const refresh = vi.fn(() => health);
    server.use(
      getGetCurrentHealthMockHandler(health),
      getListStartupReportsMockHandler({ current, items: [current, previous] }),
      getRefreshCurrentHealthMockHandler(refresh),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <StartupReportPage />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: "App Health" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Current Health" })).toBeInTheDocument();
    expect(screen.getByText("Needs attention")).toBeInTheDocument();
    expect(screen.getByText(/Next check expected/)).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Current Loomarr health checks" })).toBeInTheDocument();
    expect(
      screen.getByRole("rowheader", { name: /Database and migrationsRequired · Monitored/ }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Configured but unavailable")).toHaveLength(2);
    expect(screen.getAllByRole("link", { name: "Open" })[0]).toHaveAttribute("href", "/settings/connections");

    expect(screen.getByRole("heading", { name: "Previous Startups" })).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: "Startup checks for application generation 1" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("table", { name: "Startup checks for application generation 2" }),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Check again" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
  });
});
