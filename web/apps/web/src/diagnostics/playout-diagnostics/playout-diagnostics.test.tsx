import { getGetDiagnosticProcessMockHandler, getListDiagnosticProcessesMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import type { DiagnosticsSearch } from "../diagnostics-page";
import { PlayoutDiagnostics } from "./playout-diagnostics";

const run = {
  id: "process-1",
  purpose: "channel-segment",
  status: "running" as const,
  startedAt: 1_780_000_000_000,
  updatedAt: 1_780_000_001_000,
  channelId: "channel-7",
  target: "segment-42.ts",
  outputBytes: 128,
  discardedLines: 3,
};

const initial: DiagnosticsSearch = {
  view: "process",
  range: "1h",
  order: "newest",
  level: "all",
  source: "all",
  subsystem: "",
  text: "",
  processRange: "1h",
  processStatus: "all",
  processPurpose: "",
  processChannelId: "",
  processJobId: "",
};

// ⚠ A running process's output is polled. The test ended with a poll in flight, which reached the
// server after its handlers were reset and failed the NEXT test as an unhandled request (about one
// run in four). Each test waits for its client to go quiet and unmounts in the same tick, before
// the next poll can start, as app-router.test.tsx does since #2049.
const clients: QueryClient[] = [];
afterEach(async () => {
  await Promise.all(clients.splice(0).map((c) => vi.waitFor(() => expect(c.isFetching()).toBe(0))));
  cleanup();
});

describe("PlayoutDiagnostics", () => {
  it("shows live progress, searches bounded output, and updates shareable filters", async () => {
    const onFiltersChange = vi.fn();
    server.use(
      getListDiagnosticProcessesMockHandler({ items: [run], nextCursor: "older" }),
      getGetDiagnosticProcessMockHandler({
        run,
        progress: [{ frame: 240, occurredAt: run.updatedAt, outTimeMs: 10_000, speed: 1.01 }],
        progressTruncated: true,
      }),
      http.get(
        "/v1/diagnostics/processes/process-1/output",
        () =>
          new HttpResponse("[2026-08-24T01:00:00Z] frame=120 healthy\n[2026-08-24T01:00:01Z] warning retry", {
            headers: {
              "Content-Type": "text/plain",
              "X-Diagnostic-Truncated": "true",
              "X-Diagnostic-Discarded-Lines": "3",
            },
          }),
      ),
    );
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    clients.push(client);
    const Harness = () => {
      const [filters, setFilters] = useState(initial);
      return (
        <PlayoutDiagnostics
          filters={filters}
          onFiltersChange={(next) => {
            onFiltersChange(next);
            setFilters(next);
          }}
        />
      );
    };
    const view = render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>,
    );

    expect(await screen.findAllByText("channel-segment")).toHaveLength(2);
    expect(screen.getByRole("heading", { name: "Media processes" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Refresh" })).not.toBeInTheDocument();
    expect(await screen.findByText(/Frame/)).toHaveTextContent("240");
    expect(await screen.findByText(/3 earlier lines/)).toBeInTheDocument();
    await userEvent.hover(screen.getByRole("button", { name: "Follow tail" }));
    expect(await screen.findByText("Keep the newest output line in view")).toBeInTheDocument();
    await userEvent.type(screen.getByRole("textbox", { name: "Search Process output" }), "warning");
    expect(screen.getByText(/warning retry/)).toBeInTheDocument();
    expect(screen.queryByText(/frame=120 healthy/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Timestamps" }));
    expect(screen.getByText("warning retry")).toBeInTheDocument();
    const output = view.container.querySelector("pre");
    expect(output).not.toBeNull();
    Object.defineProperties(output, {
      scrollHeight: { configurable: true, value: 600 },
      clientHeight: { configurable: true, value: 200 },
      scrollTop: { configurable: true, value: 100, writable: true },
    });
    fireEvent.scroll(output!);
    expect(screen.getByRole("button", { name: "Follow tail" })).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(screen.getByRole("combobox", { name: "Process status" }));
    await userEvent.click(await screen.findByRole("option", { name: "Failed" }));
    expect(onFiltersChange).toHaveBeenLastCalledWith(expect.objectContaining({ processStatus: "failed" }));
  });

  it("requests 50 Process runs per server page", async () => {
    let requestedLimit: string | null = null;
    server.use(
      http.get("/v1/diagnostics/processes", ({ request }) => {
        requestedLimit = new URL(request.url).searchParams.get("limit");
        return HttpResponse.json({ items: [] });
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    clients.push(client);
    render(
      <QueryClientProvider client={client}>
        <PlayoutDiagnostics filters={initial} onFiltersChange={vi.fn()} />
      </QueryClientProvider>,
    );
    expect(await screen.findByText(/No media processes/)).toBeInTheDocument();
    await waitFor(() => expect(requestedLimit).toBe("50"));
    expect(screen.getByText("Page 1")).toBeInTheDocument();
    expect(screen.getByText("Up to 50 processes")).toBeInTheDocument();
  });
});
