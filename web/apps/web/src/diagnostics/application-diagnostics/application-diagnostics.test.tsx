import {
  getGetDiagnosticVerboseCaptureMockHandler,
  getListDiagnosticEventsMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
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

    const errors = await screen.findByRole("button", { name: "1 error" });
    expect(errors).toHaveClass("text-onair");
    expect(errors).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText("Playback failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1 warning" })).toHaveClass("text-caution");
    const info = screen.getByRole("button", { name: "0 info" });
    expect(info).toHaveClass("text-lock");
    expect(screen.getByRole("button", { name: "All 2" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.hover(info);
    expect(await screen.findByText("Show informational logs")).toBeInTheDocument();
    expect(screen.getByText("2 events dropped since startup")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Older" })).toBeEnabled();
    expect(screen.getByRole("combobox", { name: "Time range" })).toHaveTextContent("All retained");
    expect(screen.getByRole("combobox", { name: "Log order" })).toHaveTextContent("Newest first");
    expect(screen.queryByRole("button", { name: /pause|resume|refresh now/i })).not.toBeInTheDocument();

    await userEvent.click(errors);
    expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ level: "error" }));
    expect(screen.getByRole("button", { name: "1 error" })).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(errors);
    expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ level: "error" }));
    expect(screen.getByRole("button", { name: "1 error" })).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(screen.getByRole("button", { name: "Hide details" }));
    expect(screen.getByRole("button", { name: "Show details" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Playback failed/ }));
    expect(screen.getByRole("button", { name: "Hide details" })).toBeInTheDocument();
    await userEvent.click(screen.getAllByText("Technical details")[0]!);
    expect(screen.getAllByText("player.media_error")[0]).toBeInTheDocument();
    expect(screen.getAllByText(/hls_js/)[0]).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Copy log details" })[0]!);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      expect.stringContaining('"event": "player.media_error"'),
    );
    expect(screen.getAllByRole("button", { name: "Copy Request id" })[0]).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Open process output" })[0]!);
    expect(onOpenProcess).toHaveBeenCalledWith("process-1");
    await userEvent.click(screen.getAllByRole("button", { name: "Open related Channel" })[0]!);
    expect(onOpenRelated).toHaveBeenCalledWith("channel", "channel-1");

    await userEvent.click(screen.getByRole("button", { name: "More filters" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Subsystem" }), "api");
    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...DEFAULT_APPLICATION_FILTERS,
      level: "error",
      subsystem: "api",
    });

    await userEvent.click(screen.getByRole("combobox", { name: "Log order" }));
    await userEvent.click(await screen.findByRole("option", { name: "Oldest first" }));
    expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ order: "oldest" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Newer" })).toBeEnabled());
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();
  });

  it("requests a small server page and refreshes without replacing the current rows", async () => {
    const requests: URL[] = [];
    server.use(
      getGetDiagnosticVerboseCaptureMockHandler({ active: false }),
      http.get("/v1/diagnostics/events", ({ request }) => {
        requests.push(new URL(request.url));
        return HttpResponse.json({
          items: [
            {
              id: "stable-event",
              occurredAt: 1_780_000_000_000,
              receivedAt: 1_780_000_000_100,
              level: "info",
              source: "server",
              subsystem: "app",
              event: "app.ready",
              message: "Loomarr is ready",
              attributes: {},
            },
          ],
        });
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <ApplicationDiagnostics filters={DEFAULT_APPLICATION_FILTERS} onFiltersChange={vi.fn()} />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Loomarr is ready")).toBeInTheDocument();
    await waitFor(() => expect(requests[0]?.searchParams.get("limit")).toBe("50"));
    expect(requests[0]?.searchParams.get("from")).toBe("1");
    expect(requests[0]?.searchParams.get("order")).toBe("newest");
    expect(screen.getByText("Page 1")).toBeInTheDocument();
    expect(screen.getByText("Up to 50 logs")).toBeInTheDocument();
    const pager = screen.getByRole("navigation", { name: "Log pages" });
    const list = document.querySelector('section[aria-label="Logs"]');
    if (!list) throw new Error("log viewport missing");
    expect(pager.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
