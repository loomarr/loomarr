import { getGetCurrentHealthMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
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

describe("StartupReportPage", () => {
  it("shows every current check in one continuously updated table", async () => {
    server.use(getGetCurrentHealthMockHandler(health));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <StartupReportPage />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: "Current Health" })).toBeInTheDocument();
    expect(await screen.findByText("Needs attention")).toBeInTheDocument();
    expect(screen.getByText(/Next check expected/)).toBeInTheDocument();
    const table = screen.getByRole("table", { name: "Current health checks" });
    expect(table).toHaveTextContent("Database and migrations");
    expect(table).toHaveTextContent("Media server");
    expect(within(table).getByRole("link", { name: "Open" })).toHaveAttribute(
      "href",
      "/settings/connections",
    );
    expect(screen.queryByRole("button", { name: /check again/i })).not.toBeInTheDocument();
    expect(screen.queryByText("Startup history")).not.toBeInTheDocument();
  });
});
