import {
  getGetDiagnosticVerboseCaptureMockHandler,
  getListDiagnosticEventsMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ApplicationDiagnostics, DEFAULT_APPLICATION_FILTERS } from "./application-diagnostics";

describe("ApplicationDiagnostics", () => {
  it("renders bounded events, expands structured correlation, and edits reproducible filters", async () => {
    const dropState = { dropped: 2 };
    server.use(
      getGetDiagnosticVerboseCaptureMockHandler({ active: false }),
      getListDiagnosticEventsMockHandler({
        items: [
          {
            id: "event-1",
            occurredAt: 1_780_000_000_000,
            receivedAt: 1_780_000_000_100,
            level: "error",
            source: "web",
            subsystem: "player",
            event: "player.media_error",
            message: "Playback failed",
            processRunId: "process-1",
            requestId: "request-1",
            channelId: "channel-1",
            attributes: { transport: "hls_js", fatal: true },
          },
          {
            id: "event-2",
            occurredAt: 1_779_999_999_000,
            receivedAt: 1_779_999_999_100,
            level: "warn",
            source: "server",
            subsystem: "playout",
            event: "playout.transition_late",
            attributes: {},
          },
        ],
        nextCursor: "older-page",
        ...dropState,
      }),
    );
    const onFiltersChange = vi.fn();
    const onOpenProcess = vi.fn();
    const onOpenRelated = vi.fn();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const Harness = () => {
      const [filters, setFilters] = useState(DEFAULT_APPLICATION_FILTERS);
      return (
        <ApplicationDiagnostics
          filters={filters}
          onFiltersChange={(next) => {
            onFiltersChange(next);
            setFilters(next);
          }}
          onOpenProcess={onOpenProcess}
          onOpenRelated={onOpenRelated}
        />
      );
    };
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("1 errors")).toBeInTheDocument();
    expect(screen.getByText("player.media_error")).toBeInTheDocument();
    expect(screen.getByText("1 warnings")).toBeInTheDocument();
    expect(screen.getByText("2 events dropped since startup")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Older" })).toBeEnabled();

    await userEvent.click(screen.getAllByRole("button", { name: /Details/ })[0]!);
    expect(screen.getByText("Structured attributes")).toBeInTheDocument();
    expect(screen.getByText(/hls_js/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy Request id" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Open Process run" }));
    expect(onOpenProcess).toHaveBeenCalledWith("process-1");
    await userEvent.click(screen.getByRole("button", { name: "Open Channel" }));
    expect(onOpenRelated).toHaveBeenCalledWith("channel", "channel-1");

    await userEvent.type(screen.getByRole("textbox", { name: "Subsystem" }), "api");
    expect(onFiltersChange).toHaveBeenLastCalledWith({ ...DEFAULT_APPLICATION_FILTERS, subsystem: "api" });
  });
});
